package server

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

// ---- bringing places in ----
//
// Google Maps is what most people keep their places in, so its Takeout
// files come in as they are: "Saved Places.json" (starred places),
// "Reviews.json" (places you rated, with the stars and the text) and the
// CSV of each saved list ("Want to go.csv", "Favourites.csv"…, one list
// each). Any other CSV with a header row works too (name, address, city,
// lat, lng, rating… — the columns the CSV export writes). And a Pocket
// Places export. Importing the same file twice adds nothing.

// csvColumns are the export's columns, which the import reads back.
var csvColumns = []string{"name", "kind", "status", "rating", "price", "cuisine", "address", "city", "country", "lat", "lng", "website", "phone", "map_url",
	"tags", "favourite", "notes", "visits", "last_visit"}

// handleImport takes a JSON file: a Pocket Places export, or Google
// Takeout's Saved Places / Reviews (GeoJSON).
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	body, err := s.readBody(r)
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "the file is too big")
		return
	}
	var probe struct {
		App  string `json:"app"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		writeErr(w, http.StatusBadRequest, "this is not a JSON file")
		return
	}
	switch {
	case probe.App == "pocket-places":
		var doc exportDoc
		if err := json.Unmarshal(body, &doc); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid Pocket Places export")
			return
		}
		writeJSON(w, http.StatusOK, s.importPocket(ctx, uid, doc))
	case strings.EqualFold(probe.Type, "FeatureCollection"):
		counts, err := s.importGeoJSON(r, uid, body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, counts)
	case probe.App != "":
		writeErr(w, http.StatusBadRequest, "this is a "+probe.App+" export, not one from Pocket Places")
	default:
		writeErr(w, http.StatusBadRequest, "Pocket Places reads its own export and Google Takeout's Saved Places.json and Reviews.json")
	}
}

// googleFeature covers both shapes Google Takeout has used for Maps.
type googleFeature struct {
	Geometry struct {
		Coordinates []float64 `json:"coordinates"`
	} `json:"geometry"`
	Properties struct {
		// 2023 and later
		Date     string `json:"date"`
		MapsURL  string `json:"google_maps_url"`
		Location struct {
			Name        string `json:"name"`
			Address     string `json:"address"`
			CountryCode string `json:"country_code"`
		} `json:"location"`
		Stars  float64 `json:"five_star_rating_published"`
		Review string  `json:"review_text_published"`
		// before
		Title     string  `json:"Title"`
		OldURL    string  `json:"Google Maps URL"`
		Published string  `json:"Published"`
		OldStars  float64 `json:"Star Rating"`
		OldReview string  `json:"Review Comment"`
		OldLoc    struct {
			Business string `json:"Business Name"`
			Address  string `json:"Address"`
			Country  string `json:"Country Code"`
			Geo      struct {
				Lat string `json:"Latitude"`
				Lng string `json:"Longitude"`
			} `json:"Geo Coordinates"`
		} `json:"Location"`
	} `json:"properties"`
}

func (s *Server) importGeoJSON(r *http.Request, uid int64, body []byte) (map[string]int, error) {
	ctx := r.Context()
	var fc struct {
		Features []googleFeature `json:"features"`
	}
	if err := json.Unmarshal(body, &fc); err != nil {
		return nil, fmt.Errorf("invalid GeoJSON")
	}
	listName := strings.TrimSpace(r.URL.Query().Get("list"))
	var listID int64
	if listName != "" {
		listID = s.listNamed(r, uid, listName)
	}
	counts := map[string]int{}
	for _, f := range fc.Features {
		pr := f.Properties
		name := firstNonEmpty(pr.Location.Name, pr.OldLoc.Business, pr.Title)
		mapsURL := firstNonEmpty(pr.MapsURL, pr.OldURL)
		p := store.Place{Name: name, Address: firstNonEmpty(pr.Location.Address, pr.OldLoc.Address), Country: firstNonEmpty(pr.Location.CountryCode, pr.OldLoc.Country),
			MapURL: mapsURL, Kind: "other", Status: store.StatusWant, Source: "google"}
		if len(f.Geometry.Coordinates) >= 2 && (f.Geometry.Coordinates[0] != 0 || f.Geometry.Coordinates[1] != 0) {
			lng, lat := f.Geometry.Coordinates[0], f.Geometry.Coordinates[1]
			p.Lat, p.Lng = &lat, &lng
		} else if lat, err := strconv.ParseFloat(pr.OldLoc.Geo.Lat, 64); err == nil {
			if lng, err := strconv.ParseFloat(pr.OldLoc.Geo.Lng, 64); err == nil {
				p.Lat, p.Lng = &lat, &lng
			}
		}
		if p.Name == "" {
			if p.Address == "" {
				counts["skipped"]++
				continue
			}
			p.Name = strings.SplitN(p.Address, ",", 2)[0]
		}
		p.City = cityFromAddress(p.Address)
		stars := firstPosF(pr.Stars, pr.OldStars)
		review := strings.TrimSpace(firstNonEmpty(pr.Review, pr.OldReview))
		if stars > 0 {
			p.Status, p.Rating = store.StatusBeen, stars
		}
		if review != "" {
			p.Notes = review
		}
		p.SourceRef = googleRef(mapsURL, p)
		created, id := s.importPlace(r, uid, &p)
		if created {
			counts["places"]++
		} else {
			counts["already there"]++
		}
		if listID > 0 {
			_ = s.store.AddToList(ctx, uid, listID, id, "")
		}
		// a review is a visit on the day it was written
		if stars > 0 && created {
			day := dateOf(firstNonEmpty(pr.Date, pr.Published))
			if day != "" {
				v := store.Visit{PlaceID: id, Date: day, Rating: stars, Notes: review, Source: "google", SourceRef: "review:" + p.SourceRef}
				if err := s.store.CreateVisit(ctx, uid, &v); err == nil {
					counts["visits"]++
				}
			}
		}
	}
	return counts, nil
}

func firstPosF(v ...float64) float64 {
	for _, x := range v {
		if x > 0 {
			return x
		}
	}
	return 0
}

func dateOf(ts string) string {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02"} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t.Local().Format("2006-01-02")
		}
	}
	if len(ts) >= 10 && validDate(ts[:10]) {
		return ts[:10]
	}
	return ""
}

var reCID = regexp.MustCompile(`[?&]cid=(\d+)`)

// googleRef is what identifies a Google place across files: its cid, else
// the link, else the name and spot.
func googleRef(mapsURL string, p store.Place) string {
	if m := reCID.FindStringSubmatch(mapsURL); m != nil {
		return "cid:" + m[1]
	}
	if mapsURL != "" {
		return mapsURL
	}
	ref := store.Fold(p.Name)
	if p.HasCoords() {
		ref += fmt.Sprintf("@%.4f,%.4f", *p.Lat, *p.Lng)
	}
	return ref
}

// cityFromAddress guesses the town from "Ferhadija 1, 71000 Sarajevo, Bosnia
// and Herzegovina": the part before the country, without the postcode.
func cityFromAddress(addr string) string {
	parts := strings.Split(addr, ",")
	if len(parts) < 2 {
		return ""
	}
	c := strings.TrimSpace(parts[len(parts)-2])
	fields := strings.Fields(c)
	out := []string{}
	for _, f := range fields {
		if strings.IndexFunc(f, func(r rune) bool { return r >= '0' && r <= '9' }) >= 0 {
			continue // postcodes
		}
		out = append(out, f)
	}
	return strings.Join(out, " ")
}

// importPlace adds a place unless the same one came in before (by source),
// or the person already has a place of that name within 80 m. It returns
// whether it was new, and its id.
func (s *Server) importPlace(r *http.Request, uid int64, p *store.Place) (bool, int64) {
	ctx := r.Context()
	if p.SourceRef != "" {
		if old, err := s.store.PlaceBySource(ctx, uid, p.Source, p.SourceRef); err == nil {
			return false, old.ID
		}
	}
	if p.HasCoords() {
		near, _ := s.store.ListPlaces(ctx, uid, store.PlaceFilter{Near: &[2]float64{*p.Lat, *p.Lng}, Radius: 80})
		for _, x := range near {
			if store.Fold(x.Name) == store.Fold(p.Name) {
				return false, x.ID
			}
		}
	}
	if err := s.store.CreatePlace(ctx, uid, p); err != nil {
		return false, 0
	}
	return true, p.ID
}

// listNamed is the person's list with this name, made when missing.
func (s *Server) listNamed(r *http.Request, uid int64, name string) int64 {
	ctx := r.Context()
	ls, _ := s.store.ListLists(ctx, uid)
	for _, l := range ls {
		if strings.EqualFold(l.Name, name) {
			return l.ID
		}
	}
	l := store.List{Name: name}
	if err := s.store.CreateList(ctx, uid, &l); err != nil {
		return 0
	}
	return l.ID
}

var reGooglePlace = regexp.MustCompile(`/maps/place/([^/]+)`)

// handleImportCSV takes a CSV: a Google Maps saved list (Title, Note, URL…)
// — it becomes a list named after the file (?list=Want to go) — or any CSV
// with a header row naming its columns.
func (s *Server) handleImportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	body, err := s.readBody(r)
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "the file is too big")
		return
	}
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	cr := csv.NewReader(bytes.NewReader(body))
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	head, err := cr.Read()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "this is not a CSV file")
		return
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	get := func(row []string, names ...string) string {
		for _, n := range names {
			if i, ok := col[n]; ok && i < len(row) {
				if v := strings.TrimSpace(row[i]); v != "" {
					return v
				}
			}
		}
		return ""
	}
	_, google := col["title"]
	_, hasName := col["name"]
	if !google && !hasName {
		writeErr(w, http.StatusBadRequest, "the first row should name the columns: name, address, city, lat, lng… (or a Google Maps list: Title, Note, URL)")
		return
	}
	listName := strings.TrimSpace(strings.TrimSuffix(r.URL.Query().Get("list"), ".csv"))
	status := store.StatusWant
	fav := false
	switch strings.ToLower(listName) {
	case "favourites", "favorites", "favourite places", "favorite places", "omiljeno", "omiljena mjesta":
		fav, status = true, store.StatusBeen
	}
	var listID int64
	if listName != "" && !fav {
		listID = s.listNamed(r, uid, listName)
	}
	counts := map[string]int{}
	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}
		var p store.Place
		note := ""
		if google {
			p = store.Place{Name: get(row, "title"), MapURL: get(row, "url"), Kind: "other", Status: status, Favourite: fav, Source: "google"}
			note = strings.TrimSpace(strings.Join(nonEmpty(get(row, "note"), get(row, "comment")), "\n"))
			p.Tags = get(row, "tags")
			if p.Name == "" && p.MapURL != "" {
				if m := reGooglePlace.FindStringSubmatch(p.MapURL); m != nil {
					n, _ := url.PathUnescape(strings.ReplaceAll(m[1], "+", " "))
					p.Name = n
				}
			}
			if f := parseMapLink(p.MapURL); f.Lat != nil {
				p.Lat, p.Lng = f.Lat, f.Lng
			}
			p.Notes = note
		} else {
			p = store.Place{Name: get(row, "name", "title"), Kind: get(row, "kind", "type", "category"), Status: get(row, "status"), Cuisine: get(row, "cuisine"),
				Address: get(row, "address"), City: get(row, "city", "town"), Country: get(row, "country", "country_code"), Website: get(row, "website", "url"),
				Phone: get(row, "phone"), MapURL: get(row, "map_url", "maps_url", "google_maps_url"), Tags: get(row, "tags"), Notes: get(row, "notes", "note", "comment"), Source: "import"}
			if !store.ValidKind(p.Kind) {
				p.Kind = guessKind(p.Kind)
			}
			if rt, err := strconv.ParseFloat(strings.ReplaceAll(get(row, "rating", "stars"), ",", "."), 64); err == nil && rt > 0 {
				p.Rating, p.Status = rt, store.StatusBeen
			}
			if n, err := strconv.Atoi(get(row, "price")); err == nil {
				p.Price = n
			}
			if v := strings.ToLower(get(row, "favourite", "favorite")); v == "yes" || v == "1" || v == "true" {
				p.Favourite = true
			}
			if lat, err := strconv.ParseFloat(get(row, "lat", "latitude"), 64); err == nil {
				if lng, err := strconv.ParseFloat(get(row, "lng", "lon", "longitude"), 64); err == nil {
					p.Lat, p.Lng = &lat, &lng
				}
			}
			if p.City == "" {
				p.City = cityFromAddress(p.Address)
			}
		}
		if p.Name == "" {
			counts["skipped"]++
			continue
		}
		if google {
			p.SourceRef = googleRef(p.MapURL, p)
		} else {
			p.SourceRef = store.Fold(p.Name + " " + p.City)
		}
		created, id := s.importPlace(r, uid, &p)
		if id == 0 {
			counts["skipped"]++
			continue
		}
		if created {
			counts["places"]++
		} else {
			counts["already there"]++
		}
		if listID > 0 {
			_ = s.store.AddToList(ctx, uid, listID, id, note)
		}
	}
	writeJSON(w, http.StatusOK, counts)
}

// guessKind maps a free-text category to a kind ("Coffee shop" → café).
func guessKind(v string) string {
	f := store.Fold(v)
	for _, w := range []struct{ word, kind string }{{"coffee", "cafe"}, {"cafe", "cafe"}, {"kafic", "cafe"}, {"restaurant", "restaurant"}, {"restoran", "restaurant"},
		{"bistro", "restaurant"}, {"pizz", "restaurant"}, {"bar", "bar"}, {"pub", "bar"}, {"wine", "bar"}, {"baker", "bakery"}, {"pekar", "bakery"},
		{"ice cream", "dessert"}, {"gelat", "dessert"}, {"cake", "dessert"}, {"fast food", "street_food"}, {"takeaway", "street_food"}, {"market", "market"},
		{"shop", "shop"}, {"store", "shop"}, {"museum", "museum"}, {"gallery", "museum"}, {"park", "park"}, {"garden", "park"}, {"view", "viewpoint"},
		{"beach", "beach"}, {"lake", "beach"}, {"hik", "nature"}, {"mountain", "nature"}, {"hotel", "hotel"}, {"hostel", "hotel"}, {"apartment", "hotel"}} {
		if strings.Contains(f, w.word) {
			return w.kind
		}
	}
	return "other"
}

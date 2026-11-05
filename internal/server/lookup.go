package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

// ---- finding a place on OpenStreetMap, reading a map link ----
//
// Typing a name looks it up on OpenStreetMap (Nominatim: one request a
// second, results kept 30 days), and a pasted Google Maps, Apple Maps or
// OpenStreetMap link is read for the name and the spot. Both only when the
// person's "Look places up online" is on and the box is not showing demo data.

// Found is a place found online, ready to fill the form.
type Found struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"`
	Cuisine string   `json:"cuisine"`
	Address string   `json:"address"`
	City    string   `json:"city"`
	Country string   `json:"country"`
	Lat     *float64 `json:"lat"`
	Lng     *float64 `json:"lng"`
	Website string   `json:"website"`
	Phone   string   `json:"phone"`
	Hours   string   `json:"hours"`
	MapURL  string   `json:"map_url"`
	Label   string   `json:"label"` // the full line to show in a picker
}

const nominatim = "https://nominatim.openstreetmap.org"

var (
	nomMu   sync.Mutex
	nomLast time.Time
)

// online reports whether this person may use lookups on the internet.
func (s *Server) online(ctx context.Context, uid int64) bool {
	if s.cfg.DemoData {
		return false
	}
	st, _ := s.store.GetSettings(ctx, uid)
	return st.Online
}

// nomGet asks Nominatim, one request a second for the whole box, cached.
func (s *Server) nomGet(ctx context.Context, path string, q url.Values, v any) error {
	key := "nominatim:" + path + "?" + q.Encode()
	if raw, at, ok := s.store.CacheGet(ctx, key); ok {
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(t) < 30*24*time.Hour {
			return json.Unmarshal([]byte(raw), v)
		}
	}
	nomMu.Lock()
	if wait := time.Second - time.Since(nomLast); wait > 0 {
		time.Sleep(wait)
	}
	nomLast = time.Now()
	nomMu.Unlock()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nominatim+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	req.Header.Set("Accept-Language", "en")
	res, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("OpenStreetMap answered %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return err
	}
	_ = s.store.CachePut(ctx, key, string(b))
	return json.Unmarshal(b, v)
}

type nomPlace struct {
	Lat         string            `json:"lat"`
	Lon         string            `json:"lon"`
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	Category    string            `json:"category"`
	Class       string            `json:"class"`
	Type        string            `json:"type"`
	OSMType     string            `json:"osm_type"`
	OSMID       int64             `json:"osm_id"`
	Address     map[string]string `json:"address"`
	Extra       map[string]string `json:"extratags"`
}

// osmKind maps OpenStreetMap's tags to a kind of place.
func osmKind(class, typ string) string {
	switch class + "=" + typ {
	case "amenity=restaurant":
		return "restaurant"
	case "amenity=cafe":
		return "cafe"
	case "amenity=bar", "amenity=pub", "amenity=biergarten", "amenity=nightclub":
		return "bar"
	case "amenity=fast_food", "amenity=food_court":
		return "street_food"
	case "amenity=ice_cream", "shop=confectionery", "shop=pastry", "shop=chocolate":
		return "dessert"
	case "shop=bakery":
		return "bakery"
	case "amenity=marketplace":
		return "market"
	case "tourism=museum", "tourism=gallery", "amenity=arts_centre":
		return "museum"
	case "tourism=viewpoint":
		return "viewpoint"
	case "tourism=hotel", "tourism=hostel", "tourism=guest_house", "tourism=apartment", "tourism=motel", "tourism=chalet", "tourism=camp_site":
		return "hotel"
	case "leisure=park", "leisure=garden", "leisure=nature_reserve":
		return "park"
	case "natural=beach", "leisure=beach_resort", "natural=water":
		return "beach"
	case "natural=peak", "natural=waterfall", "natural=cave_entrance", "route=hiking":
		return "nature"
	case "amenity=cinema", "amenity=theatre", "leisure=sports_centre", "leisure=water_park", "tourism=zoo", "tourism=theme_park", "leisure=bowling_alley":
		return "activity"
	}
	switch class {
	case "tourism", "historic":
		return "sight"
	case "shop":
		return "shop"
	}
	return "other"
}

func (n nomPlace) found() Found {
	f := Found{Name: n.Name, Label: n.DisplayName, Kind: osmKind(firstNonEmpty(n.Category, n.Class), n.Type)}
	if lat, err := strconv.ParseFloat(n.Lat, 64); err == nil {
		if lng, err := strconv.ParseFloat(n.Lon, 64); err == nil {
			f.Lat, f.Lng = &lat, &lng
		}
	}
	a := n.Address
	street := strings.TrimSpace(strings.TrimSpace(a["road"] + " " + a["house_number"]))
	f.City = firstNonEmpty(a["city"], a["town"], a["village"], a["municipality"], a["suburb"])
	f.Country = strings.ToUpper(a["country_code"])
	f.Address = strings.Join(nonEmpty(street, strings.TrimSpace(a["postcode"]+" "+f.City)), ", ")
	if f.Name == "" {
		f.Name = firstNonEmpty(street, strings.SplitN(n.DisplayName, ",", 2)[0])
	}
	f.Website = firstNonEmpty(n.Extra["website"], n.Extra["contact:website"])
	f.Phone = firstNonEmpty(n.Extra["phone"], n.Extra["contact:phone"])
	f.Hours = n.Extra["opening_hours"]
	f.Cuisine = strings.ReplaceAll(strings.ReplaceAll(n.Extra["cuisine"], ";", ", "), "_", " ")
	if n.OSMType != "" && n.OSMID > 0 {
		f.MapURL = fmt.Sprintf("https://www.openstreetmap.org/%s/%d", n.OSMType, n.OSMID)
	}
	return f
}

func nonEmpty(v ...string) []string {
	out := []string{}
	for _, x := range v {
		if strings.TrimSpace(x) != "" {
			out = append(out, strings.TrimSpace(x))
		}
	}
	return out
}

// search looks a name up on OpenStreetMap, near a spot when one is given.
func (s *Server) search(ctx context.Context, q string, near *[2]float64) ([]Found, error) {
	v := url.Values{"q": {q}, "format": {"jsonv2"}, "addressdetails": {"1"}, "extratags": {"1"}, "limit": {"8"}}
	if near != nil {
		d := 0.25 // about 25 km around
		v.Set("viewbox", fmt.Sprintf("%.4f,%.4f,%.4f,%.4f", near[1]-d, near[0]+d, near[1]+d, near[0]-d))
	}
	var res []nomPlace
	if err := s.nomGet(ctx, "/search", v, &res); err != nil {
		return nil, err
	}
	out := make([]Found, 0, len(res))
	for _, n := range res {
		out = append(out, n.found())
	}
	return out, nil
}

// reverse is the address at a spot.
func (s *Server) reverse(ctx context.Context, lat, lng float64) (*Found, error) {
	var n nomPlace
	v := url.Values{"lat": {strconv.FormatFloat(lat, 'f', 6, 64)}, "lon": {strconv.FormatFloat(lng, 'f', 6, 64)}, "format": {"jsonv2"}, "addressdetails": {"1"}, "extratags": {"1"}, "zoom": {"18"}}
	if err := s.nomGet(ctx, "/reverse", v, &n); err != nil {
		return nil, err
	}
	f := n.found()
	f.Lat, f.Lng = &lat, &lng
	return &f, nil
}

func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 3 || len(q) > 200 {
		writeJSON(w, http.StatusOK, map[string]any{"items": []Found{}})
		return
	}
	if !s.online(ctx, actorID(ctx)) {
		writeJSON(w, http.StatusOK, map[string]any{"items": []Found{}, "offline": true})
		return
	}
	var near *[2]float64
	if lat, lng, ok := latLng(r.URL.Query().Get("near")); ok {
		near = &[2]float64{lat, lng}
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	items, err := s.search(cctx, q, near)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []Found{}, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleReverse(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lat, lng, ok := latLng(r.URL.Query().Get("at"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "at is lat,lng")
		return
	}
	if !s.online(ctx, actorID(ctx)) {
		writeJSON(w, http.StatusOK, Found{Lat: &lat, Lng: &lng})
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	f, err := s.reverse(cctx, lat, lng)
	if err != nil {
		writeJSON(w, http.StatusOK, Found{Lat: &lat, Lng: &lng})
		return
	}
	writeJSON(w, http.StatusOK, f)
}

var (
	reBang   = regexp.MustCompile(`!3d(-?\d+(?:\.\d+)?)!4d(-?\d+(?:\.\d+)?)`)
	reAt     = regexp.MustCompile(`@(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)`)
	rePair   = regexp.MustCompile(`^\s*(-?\d{1,2}(?:\.\d+)?)\s*,\s*(-?\d{1,3}(?:\.\d+)?)\s*$`)
	reOSMMap = regexp.MustCompile(`map=\d+/(-?\d+(?:\.\d+)?)/(-?\d+(?:\.\d+)?)`)
	reOSMObj = regexp.MustCompile(`openstreetmap\.org/(node|way|relation)/(\d+)`)
	reURL    = regexp.MustCompile(`https?://\S+`)
)

// parseMapLink reads the name and the spot out of a map link, without the
// internet: Google Maps (…/place/Name/@lat,lng…!3dlat!4dlng, ?q=lat,lng),
// Apple Maps (?q=Name&ll=lat,lng) and OpenStreetMap (#map=z/lat/lng).
func parseMapLink(raw string) Found {
	f := Found{}
	raw = strings.TrimSpace(raw)
	if m := rePair.FindStringSubmatch(raw); m != nil {
		lat, _ := strconv.ParseFloat(m[1], 64)
		lng, _ := strconv.ParseFloat(m[2], 64)
		f.Lat, f.Lng = &lat, &lng
		return f
	}
	if m := reURL.FindString(raw); m != "" {
		raw = m
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return f
	}
	f.MapURL = raw
	setLL := func(a, b string) {
		lat, err1 := strconv.ParseFloat(a, 64)
		lng, err2 := strconv.ParseFloat(b, 64)
		if err1 == nil && err2 == nil && lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 {
			f.Lat, f.Lng = &lat, &lng
		}
	}
	full, _ := url.PathUnescape(u.Path)
	switch {
	case strings.Contains(u.Host, "google.") || strings.Contains(u.Host, "goo.gl"):
		if i := strings.Index(full, "/place/"); i >= 0 {
			name := strings.SplitN(full[i+7:], "/", 2)[0]
			f.Name = strings.TrimSpace(strings.ReplaceAll(name, "+", " "))
		}
		if m := reBang.FindStringSubmatch(raw); m != nil {
			setLL(m[1], m[2])
		} else if m := reAt.FindStringSubmatch(raw); m != nil {
			setLL(m[1], m[2])
		}
		for _, k := range []string{"q", "query", "destination", "ll"} {
			v := u.Query().Get(k)
			if m := rePair.FindStringSubmatch(v); m != nil {
				if f.Lat == nil {
					setLL(m[1], m[2])
				}
			} else if v != "" && f.Name == "" && k != "ll" {
				f.Name = v
			}
		}
	case strings.Contains(u.Host, "apple.com"):
		f.Name = firstNonEmpty(u.Query().Get("q"), u.Query().Get("name"))
		for _, k := range []string{"ll", "coordinate", "sll"} {
			if m := rePair.FindStringSubmatch(u.Query().Get(k)); m != nil {
				setLL(m[1], m[2])
				break
			}
		}
		if f.Name == "" {
			f.Address = u.Query().Get("address")
		}
	case strings.Contains(u.Host, "openstreetmap.org"):
		if m := reOSMMap.FindStringSubmatch(raw); m != nil {
			setLL(m[1], m[2])
		}
		if m := u.Query().Get("mlat"); m != "" {
			setLL(m, u.Query().Get("mlon"))
		}
	}
	return f
}

// expandLink follows a short link (maps.app.goo.gl, goo.gl/maps) to the long one.
func (s *Server) expandLink(ctx context.Context, raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Host != "maps.app.goo.gl" && u.Host != "goo.gl" && u.Host != "g.co" && !strings.HasSuffix(u.Host, ".page.link")) {
		return raw
	}
	final := raw
	cl := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		final = req.URL.String()
		if len(via) > 8 {
			return errors.New("too many redirects")
		}
		if strings.Contains(req.URL.Host, "google.") && strings.Contains(req.URL.Path, "/maps") {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return raw
	}
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	if res, err := cl.Do(req); err == nil {
		res.Body.Close()
		if loc := res.Header.Get("Location"); loc != "" && res.StatusCode >= 300 && res.StatusCode < 400 {
			final = loc
		}
	}
	return final
}

// handleResolve reads a pasted map link or "lat,lng": the name, the spot and,
// online, the address from OpenStreetMap.
func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Text string `json:"text"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Text) == "" || len(in.Text) > 4000 {
		writeErr(w, http.StatusBadRequest, "paste a map link")
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	on := s.online(ctx, actorID(ctx))
	text := in.Text
	if on {
		if m := reURL.FindString(text); m != "" {
			text = s.expandLink(cctx, m)
		}
	}
	f := parseMapLink(text)
	if f.MapURL == "" {
		if m := reURL.FindString(in.Text); m != "" {
			f.MapURL = m
		}
	}
	if m := reOSMObj.FindStringSubmatch(text); m != nil && on {
		var res []nomPlace
		if err := s.nomGet(cctx, "/lookup", url.Values{"osm_ids": {strings.ToUpper(m[1][:1]) + m[2]}, "format": {"jsonv2"}, "addressdetails": {"1"}, "extratags": {"1"}}, &res); err == nil && len(res) > 0 {
			got := res[0].found()
			got.MapURL = f.MapURL
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "place": got})
			return
		}
	}
	if f.Lat == nil && f.Name == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "No place found in that link. Copy the link from the place's Share button, or type the name."})
		return
	}
	if on {
		var got *Found
		switch {
		case f.Lat != nil && f.Name != "":
			// the named place near the spot, else just the address there
			if list, err := s.search(cctx, f.Name, &[2]float64{*f.Lat, *f.Lng}); err == nil {
				for i := range list {
					if list[i].Lat != nil && store.Meters(*list[i].Lat, *list[i].Lng, *f.Lat, *f.Lng) < 250 {
						got = &list[i]
						break
					}
				}
			}
			if got == nil {
				got, _ = s.reverse(cctx, *f.Lat, *f.Lng)
				if got != nil {
					got.Name, got.Kind = f.Name, ""
				}
			}
		case f.Lat != nil:
			got, _ = s.reverse(cctx, *f.Lat, *f.Lng)
		default:
			if list, err := s.search(cctx, f.Name, nil); err == nil && len(list) > 0 {
				got = &list[0]
			}
		}
		if got != nil {
			if f.Lat != nil {
				got.Lat, got.Lng = f.Lat, f.Lng
			}
			if f.Name != "" {
				got.Name = f.Name
			}
			got.MapURL = firstNonEmpty(f.MapURL, got.MapURL)
			f = *got
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "place": f, "offline": !on})
}

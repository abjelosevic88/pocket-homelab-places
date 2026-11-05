package server

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

// placeInput is a place as the forms and other apps send it; nil = not sent.
type placeInput struct {
	Name          *string  `json:"name"`
	Kind          *string  `json:"kind"`
	Cuisine       *string  `json:"cuisine"`
	Status        *string  `json:"status"`
	Favourite     *bool    `json:"favourite"`
	Closed        *bool    `json:"closed"`
	Rating        *float64 `json:"rating"`
	Price         *int     `json:"price"`
	Address       *string  `json:"address"`
	City          *string  `json:"city"`
	Country       *string  `json:"country"`
	Lat           *float64 `json:"lat"`
	Lng           *float64 `json:"lng"`
	Phone         *string  `json:"phone"`
	Website       *string  `json:"website"`
	MapURL        *string  `json:"map_url"`
	Hours         *string  `json:"hours"`
	Tags          *string  `json:"tags"`
	Notes         *string  `json:"notes"`
	RecommendedBy *string  `json:"recommended_by"`
	ContactID     *int64   `json:"contact_id"`
	Currency      *string  `json:"currency"`
	SourceRef     *string  `json:"source_ref"`
	ListIDs       []int64  `json:"list_ids"`
}

// apply copies what was sent onto p and says what is wrong, if anything.
func (in placeInput) apply(p *store.Place, keys map[string]bool) string {
	trimP(&p.Name, in.Name)
	if in.Kind != nil {
		if !store.ValidKind(*in.Kind) {
			return "unknown kind of place"
		}
		p.Kind = *in.Kind
	}
	trimP(&p.Cuisine, in.Cuisine)
	if in.Status != nil {
		if *in.Status != store.StatusWant && *in.Status != store.StatusBeen {
			return "the status is want or been"
		}
		p.Status = *in.Status
	}
	if in.Favourite != nil {
		p.Favourite = *in.Favourite
	}
	if in.Closed != nil {
		p.Closed = *in.Closed
	}
	if in.Rating != nil {
		if *in.Rating < 0 || *in.Rating > 5 {
			return "the rating is 0 to 5"
		}
		p.Rating = *in.Rating
	}
	if in.Price != nil {
		if *in.Price < 0 || *in.Price > 4 {
			return "the price is 0 to 4"
		}
		p.Price = *in.Price
	}
	trimP(&p.Address, in.Address)
	trimP(&p.City, in.City)
	trimP(&p.Country, in.Country)
	if keys["lat"] || keys["lng"] {
		p.Lat, p.Lng = in.Lat, in.Lng
	}
	trimP(&p.Phone, in.Phone)
	for _, u := range []struct {
		dst *string
		v   *string
	}{{&p.Website, in.Website}, {&p.MapURL, in.MapURL}} {
		if u.v != nil {
			clean, ok := cleanURL(*u.v)
			if !ok {
				return "links start with http:// or https://"
			}
			*u.dst = clean
		}
	}
	trimP(&p.Hours, in.Hours)
	trimP(&p.Tags, in.Tags)
	trimP(&p.Notes, in.Notes)
	trimP(&p.RecommendedBy, in.RecommendedBy)
	if keys["contact_id"] {
		p.ContactID = in.ContactID
	}
	if in.Currency != nil {
		c := strings.TrimSpace(*in.Currency)
		if c != "" && cleanCurrency(c) == "" {
			return "the currency is a three-letter code like EUR"
		}
		p.Currency = cleanCurrency(c)
	}
	if p.Name == "" {
		return "give the place a name"
	}
	if len(p.Name) > 200 || len(p.Notes) > 20000 {
		return "that is too long"
	}
	if p.Rating > 0 && p.Status == store.StatusWant && in.Status == nil && in.Rating != nil {
		p.Status = store.StatusBeen // rating it means you have been
	}
	return ""
}

func placePayload(p *store.Place) map[string]any {
	out := map[string]any{"place_id": p.ID, "name": p.Name, "kind": p.Kind, "status": p.Status, "city": p.City, "country": p.Country,
		"rating": p.Rating, "favourite": p.Favourite, "path": "/places/" + itoa(p.ID), "title": p.Name}
	if p.HasCoords() {
		out["lat"], out["lng"] = *p.Lat, *p.Lng
	}
	return out
}

// linkMayChange: a link token may only change what its own app added.
func linkMayChange(ctx context.Context, source string) bool {
	app := linkApp(ctx)
	return app == "" || app == source
}

// placeFilter reads the listing's query string.
func placeFilter(r *http.Request) store.PlaceFilter {
	q := r.URL.Query()
	f := store.PlaceFilter{Status: q.Get("status"), Kind: q.Get("kind"), Group: q.Get("group"), City: q.Get("city"), Country: q.Get("country"),
		Tag: q.Get("tag"), Q: strings.TrimSpace(q.Get("q")), Favourite: queryBool(r, "favourite"), OpenOnly: queryBool(r, "open"), Sort: q.Get("sort"), Limit: queryInt(r, "limit")}
	f.ListID, _ = strconv.ParseInt(q.Get("list_id"), 10, 64)
	if lat, lng, ok := latLng(q.Get("near")); ok {
		f.Near = &[2]float64{lat, lng}
		if km, err := strconv.ParseFloat(q.Get("radius_km"), 64); err == nil && km > 0 {
			f.Radius = km * 1000
		}
	}
	return f
}

// latLng parses "43.85,18.41".
func latLng(v string) (float64, float64, bool) {
	parts := strings.Split(strings.TrimSpace(v), ",")
	if len(parts) != 2 {
		return 0, 0, false
	}
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lng, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return 0, 0, false
	}
	return lat, lng, true
}

func (s *Server) handleListPlaces(w http.ResponseWriter, r *http.Request) {
	ps, err := s.store.ListPlaces(r.Context(), actorID(r.Context()), placeFilter(r))
	if err != nil {
		s.fail(w, err, "places")
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

func (s *Server) handleCreatePlace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in placeInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	p := store.Place{Kind: "restaurant", Status: store.StatusWant, Source: linkApp(ctx)}
	if msg := in.apply(&p, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if in.SourceRef != nil && p.Source != "" {
		p.SourceRef = strings.TrimSpace(*in.SourceRef)
		// another app sending the same place twice gets the one it added
		if old, err := s.store.PlaceBySource(ctx, uid, p.Source, p.SourceRef); err == nil {
			writeJSON(w, http.StatusOK, old)
			return
		}
	}
	if err := s.store.CreatePlace(ctx, uid, &p); err != nil {
		s.fail(w, err, "add place")
		return
	}
	for _, l := range in.ListIDs {
		_ = s.store.AddToList(ctx, uid, l, p.ID, "")
	}
	np, _ := s.store.GetPlace(ctx, uid, p.ID)
	s.publish(ctx, uid, "places.place_saved", "place:"+itoa(p.ID), placePayload(np))
	writeJSON(w, http.StatusCreated, np)
}

// placeDetail is one place with everything around it.
type placeDetail struct {
	Place    *store.Place       `json:"place"`
	Visits   []store.Visit      `json:"visits"`
	Lists    []store.List       `json:"lists"`
	Notes    map[string]string  `json:"list_notes"`
	Nearby   []store.Place      `json:"nearby"`
	Inbox    []store.InboxItem  `json:"inbox"`
	Spent    []store.SpentRow   `json:"spent"`
	Movement *movementSummary   `json:"movement"`
	Links    map[string]string  `json:"links"`
	Raw      map[string]float64 `json:"-"`
}

func (s *Server) handleGetPlace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPlace(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "place")
		return
	}
	out := placeDetail{Place: p, Lists: []store.List{}, Notes: map[string]string{}, Nearby: []store.Place{}, Links: map[string]string{}}
	out.Visits, _, _ = s.store.ListVisits(ctx, uid, store.VisitFilter{PlaceID: p.ID})
	lists, _ := s.store.ListLists(ctx, uid)
	in := map[int64]bool{}
	for _, id := range p.Lists {
		in[id] = true
	}
	for _, l := range lists {
		if in[l.ID] {
			out.Lists = append(out.Lists, l)
		}
	}
	entries, _ := s.store.ListEntries(ctx, uid, 0)
	for _, e := range entries {
		if e.PlaceID == p.ID && e.Note != "" {
			out.Notes[itoa(e.ListID)] = e.Note
		}
	}
	if p.HasCoords() {
		near, _ := s.store.ListPlaces(ctx, uid, store.PlaceFilter{Near: &[2]float64{*p.Lat, *p.Lng}, Radius: 600, Sort: "near", Limit: 7})
		for _, x := range near {
			if x.ID != p.ID && len(out.Nearby) < 6 {
				out.Nearby = append(out.Nearby, x)
			}
		}
	}
	all, _ := s.store.ListInbox(ctx, uid, store.InboxOpen, 0)
	out.Inbox = []store.InboxItem{}
	for _, it := range all {
		if it.PlaceID == p.ID {
			out.Inbox = append(out.Inbox, it)
		}
	}
	out.Spent = spentOf(out.Visits)
	if linkApp(ctx) == "" && p.HasCoords() {
		out.Movement = s.movementAt(ctx, uid, p)
	}
	if out.Movement != nil && out.Movement.URL != "" {
		out.Links["movement"] = out.Movement.URL
	}
	writeJSON(w, http.StatusOK, out)
}

// spentOf sums the visits' costs per currency.
func spentOf(vs []store.Visit) []store.SpentRow {
	by := map[string]*store.SpentRow{}
	for _, v := range vs {
		if v.Planned || v.Spent == 0 {
			continue
		}
		r := by[v.Currency]
		if r == nil {
			r = &store.SpentRow{Currency: v.Currency}
			by[v.Currency] = r
		}
		r.Total += v.Spent
		r.Visits++
	}
	out := []store.SpentRow{}
	for _, r := range by {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	return out
}

func (s *Server) handleUpdatePlace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPlace(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "place")
		return
	}
	if !linkMayChange(ctx, p.Source) {
		writeErr(w, http.StatusForbidden, "this app may only change the places it added")
		return
	}
	var in placeInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := in.apply(p, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdatePlace(ctx, uid, p); err != nil {
		s.fail(w, err, "save place")
		return
	}
	np, _ := s.store.GetPlace(ctx, uid, p.ID)
	s.publish(ctx, uid, "places.place_saved", "place:"+itoa(p.ID), placePayload(np))
	writeJSON(w, http.StatusOK, np)
}

func (s *Server) handleDeletePlace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPlace(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "place")
		return
	}
	if !linkMayChange(ctx, p.Source) {
		writeErr(w, http.StatusForbidden, "this app may only remove the places it added")
		return
	}
	visits, _, _ := s.store.ListVisits(ctx, uid, store.VisitFilter{PlaceID: p.ID})
	if err := s.store.DeletePlace(ctx, uid, p.ID); err != nil {
		s.fail(w, err, "delete place")
		return
	}
	// planned visits leave the calendar; costs Places booked leave Pocket Money
	for i := range visits {
		v := visits[i]
		go func() {
			cctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			s.dropVisitLinks(cctx, uid, &v)
		}()
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleAddToList puts a place on one of the person's lists.
func (s *Server) handlePlaceAddToList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		ListID int64  `json:"list_id"`
		Note   string `json:"note"`
	}
	if err := decode(r, &in); err != nil || in.ListID <= 0 {
		writeErr(w, http.StatusBadRequest, "choose a list")
		return
	}
	if err := s.store.AddToList(ctx, actorID(ctx), in.ListID, idParam(r, "id"), in.Note); err != nil {
		s.fail(w, err, "add to list")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePlaceRemoveFromList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.RemoveFromList(ctx, actorID(ctx), idParam(r, "list"), idParam(r, "id")); err != nil {
		s.fail(w, err, "remove from list")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleKinds(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, store.Kinds)
}

func (s *Server) handleCities(w http.ResponseWriter, r *http.Request) {
	cs, err := s.store.Cities(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "cities")
		return
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	ts, err := s.store.Tags(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "tags")
		return
	}
	writeJSON(w, http.StatusOK, ts)
}

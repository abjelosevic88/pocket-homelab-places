package server

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

// tripView is a Pocket Travel trip with the places on your list there.
type tripView struct {
	TripID      int64    `json:"trip_id"`
	Name        string   `json:"name"`
	Destination string   `json:"destination"`
	Start       string   `json:"start"`
	End         string   `json:"end"`
	DaysUntil   int      `json:"days_until"`
	Lat         *float64 `json:"lat"`
	Lng         *float64 `json:"lng"`
	Want        int      `json:"want"`
	Been        int      `json:"been"`
	Visits      int      `json:"visits"`
	ListIDs     []int64  `json:"list_ids"`
	URL         string   `json:"url"`
}

// tripRadius is how far from a trip's destination its places can be.
const tripRadius = 40000.0

// placesForTrip are the places at a trip's destination: within 40 km of its
// spot, or in a town named in its destination.
func placesForTrip(ps []store.Place, lat, lng *float64, destination string) []store.Place {
	dest := " " + store.Fold(destination) + " "
	out := []store.Place{}
	for _, p := range ps {
		near := lat != nil && lng != nil && p.HasCoords() && store.Meters(*lat, *lng, *p.Lat, *p.Lng) <= tripRadius
		named := strings.TrimSpace(dest) != "" && p.City != "" && strings.Contains(dest, " "+store.Fold(p.City)+" ")
		if near || named {
			out = append(out, p)
		}
	}
	return out
}

// tripsAhead asks Pocket Travel for trips under way or starting within days
// (and those that ended within back days). Not connected = nothing, quietly;
// the second value says why when it failed.
func (s *Server) tripsAhead(ctx context.Context, uid int64, days, back int) ([]tripView, string) {
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	conn, err := optional(s.clients(cctx, uid).conn(cctx, appTravel, scopeTripsRead))
	if err != nil {
		return []tripView{}, err.Error()
	}
	if conn == nil {
		return []tripView{}, ""
	}
	trips, err := conn.client.Trips(cctx)
	if err != nil {
		return []tripView{}, "Pocket Travel: " + err.Error()
	}
	today := store.Today()
	places, _ := s.store.ListPlaces(ctx, uid, store.PlaceFilter{})
	lists, _ := s.store.ListLists(ctx, uid)
	out := []tripView{}
	for _, t := range trips {
		if t.Archived || t.StartDate == "" || !validDate(t.StartDate) || !validDate(t.EndDate) || strings.EqualFold(t.Status, "cancelled") {
			continue
		}
		end := firstNonEmpty(t.EndDate, t.StartDate)
		if end < store.AddDays(today, -back) || store.DaysBetween(today, t.StartDate) > days {
			continue
		}
		v := tripView{TripID: t.ID, Name: t.Name, Destination: t.Destination, Start: t.StartDate, End: end, DaysUntil: store.DaysBetween(today, t.StartDate),
			Lat: t.Lat, Lng: t.Lng, ListIDs: []int64{}, URL: strings.TrimRight(conn.public, "/") + "/trips/" + itoa(t.ID)}
		for _, p := range placesForTrip(places, t.Lat, t.Lng, t.Destination) {
			if p.Status == store.StatusWant {
				v.Want++
			} else {
				v.Been++
			}
		}
		for _, l := range lists {
			if l.TripID != nil && *l.TripID == t.ID {
				v.ListIDs = append(v.ListIDs, l.ID)
			}
		}
		v.Visits = s.store.CountVisits(ctx, uid, t.StartDate, end)
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out, ""
}

// handleOverview is the first page: what to rate, what is planned, where
// you went lately, favourites, places waiting for you, trips coming up.
func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	today := store.Today()
	all, err := s.store.ListPlaces(ctx, uid, store.PlaceFilter{})
	if err != nil {
		s.fail(w, err, "overview")
		return
	}
	counts := map[string]int{"places": len(all), "want": 0, "been": 0, "favourites": 0, "lists": 0}
	for _, p := range all {
		counts[p.Status]++
		if p.Favourite {
			counts["favourites"]++
		}
	}
	lists, _ := s.store.ListLists(ctx, uid)
	counts["lists"] = len(lists)
	inbox, _ := s.store.ListInbox(ctx, uid, store.InboxOpen, 6)
	past := s.pastPlans(ctx, uid)
	counts["to_rate"] = len(inbox) + len(past)
	planned, _, _ := s.store.ListVisits(ctx, uid, store.VisitFilter{Planned: ptr(true), From: today, Oldest: true, Limit: 6})
	recent, _, _ := s.store.ListVisits(ctx, uid, store.VisitFilter{Planned: ptr(false), Limit: 8})
	favs := []store.Place{}
	want := []store.Place{}
	byAdded := append([]store.Place(nil), all...)
	sort.SliceStable(byAdded, func(i, j int) bool { return byAdded[i].CreatedAt > byAdded[j].CreatedAt })
	for _, p := range byAdded {
		if p.Status == store.StatusWant && !p.Closed && len(want) < 6 {
			want = append(want, p)
		}
	}
	byRating := append([]store.Place(nil), all...)
	sort.SliceStable(byRating, func(i, j int) bool {
		if byRating[i].Rating != byRating[j].Rating {
			return byRating[i].Rating > byRating[j].Rating
		}
		return byRating[i].Visits > byRating[j].Visits
	})
	for _, p := range byRating {
		if p.Favourite && len(favs) < 6 {
			favs = append(favs, p)
		}
	}
	trips, tripsErr := s.tripsAhead(ctx, uid, 60, 0)
	year := today[:4] + "-01-01"
	spent, _ := s.store.Spending(ctx, uid, year, today)
	cities, _ := s.store.Cities(ctx, uid)
	if len(cities) > 6 {
		cities = cities[:6]
	}
	writeJSON(w, http.StatusOK, map[string]any{"today": today, "counts": counts, "inbox": inbox, "past_plans": past, "planned": planned, "recent": recent,
		"favourites": favs, "want": want, "trips": trips, "trips_error": tripsErr, "spent_year": spent, "visits_year": s.store.CountVisits(ctx, uid, year, today),
		"cities": cities, "lists": lists})
}

// handleSummary is the short version for dashboards and Pocket AI.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	today := store.Today()
	all, err := s.store.ListPlaces(ctx, uid, store.PlaceFilter{})
	if err != nil {
		s.fail(w, err, "summary")
		return
	}
	out := map[string]any{"places": len(all), "want": 0, "been": 0, "favourites": 0}
	for _, p := range all {
		out[p.Status] = out[p.Status].(int) + 1
		if p.Favourite {
			out["favourites"] = out["favourites"].(int) + 1
		}
	}
	inbox, _ := s.store.ListInbox(ctx, uid, store.InboxOpen, 0)
	out["to_rate"] = len(inbox) + len(s.pastPlans(ctx, uid))
	planned, _, _ := s.store.ListVisits(ctx, uid, store.VisitFilter{Planned: ptr(true), From: today, Oldest: true, Limit: 5})
	out["planned"] = planned
	month := today[:8] + "01"
	out["visits_this_month"] = s.store.CountVisits(ctx, uid, month, today)
	out["spent_this_month"], _ = s.store.Spending(ctx, uid, month, today)
	last, _, _ := s.store.ListVisits(ctx, uid, store.VisitFilter{Planned: ptr(false), Limit: 1})
	if len(last) > 0 {
		out["last_visit"] = last[0]
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDay is one day with places: where you went and what was planned —
// what Pocket Day writes into the journal.
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	date := chi.URLParam(r, "date")
	if !validDate(date) || date == "" {
		writeErr(w, http.StatusBadRequest, "the date is YYYY-MM-DD")
		return
	}
	vs, _, err := s.store.ListVisits(ctx, uid, store.VisitFilter{From: date, To: date, Oldest: true})
	if err != nil {
		s.fail(w, err, "day")
		return
	}
	done, planned := []store.Visit{}, []store.Visit{}
	for _, v := range vs {
		if v.Planned {
			planned = append(planned, v)
		} else {
			done = append(done, v)
		}
	}
	added := []store.Place{}
	ps, _ := s.store.ListPlaces(ctx, uid, store.PlaceFilter{})
	for _, p := range ps {
		if len(p.CreatedAt) >= 10 && dayOf(p.CreatedAt) == date && p.Source != "import" && p.Source != "google" {
			added = append(added, p)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": date, "visits": done, "planned": planned, "added": added, "spent": spentOf(done)})
}

// dayOf is the local day of a stored UTC timestamp.
func dayOf(ts string) string {
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.Local().Format("2006-01-02")
	}
	return ts[:10]
}

// handleSearch looks through places, visits and lists.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || len(q) > 200 {
		writeErr(w, http.StatusBadRequest, "search for 1 to 200 characters")
		return
	}
	limit := queryInt(r, "limit")
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	ps, _ := s.store.ListPlaces(ctx, uid, store.PlaceFilter{Q: q, Limit: limit})
	vs, _, _ := s.store.ListVisits(ctx, uid, store.VisitFilter{Q: q, Limit: limit})
	ls := []store.List{}
	all, _ := s.store.ListLists(ctx, uid)
	fq := store.Fold(q)
	for _, l := range all {
		if strings.Contains(store.Fold(l.Name+" "+l.Description+" "+l.TripName), fq) {
			ls = append(ls, l)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": q, "places": ps, "visits": vs, "lists": ls})
}

// handleTrips lists Pocket Travel trips (coming and of the last months)
// with the places on your list there.
func (s *Server) handleTrips(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	days := queryInt(r, "days")
	if days <= 0 || days > 730 {
		days = 365
	}
	back := queryInt(r, "back")
	if back < 0 || back > 3650 {
		back = 0
	}
	trips, why := s.tripsAhead(ctx, actorID(ctx), days, back)
	writeJSON(w, http.StatusOK, map[string]any{"trips": trips, "error": why})
}

// handleTripGuide is what Places knows for a trip: the lists made for it,
// the places at its destination (waiting for you, and where you have been)
// and the visits during it. Pocket Travel shows it on the trip's page.
func (s *Server) handleTripGuide(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := r.URL.Query()
	tripID, _ := strconv.ParseInt(q.Get("trip_id"), 10, 64)
	var lat, lng *float64
	if a, b, ok := latLng(q.Get("at")); ok {
		lat, lng = &a, &b
	} else if a, err := strconv.ParseFloat(q.Get("lat"), 64); err == nil {
		if b, err := strconv.ParseFloat(q.Get("lng"), 64); err == nil {
			lat, lng = &a, &b
		}
	}
	dest := strings.TrimSpace(q.Get("destination"))
	from, to := q.Get("from"), q.Get("to")
	if !validDate(from) || !validDate(to) {
		writeErr(w, http.StatusBadRequest, "from and to are YYYY-MM-DD")
		return
	}
	all, err := s.store.ListPlaces(ctx, uid, store.PlaceFilter{})
	if err != nil {
		s.fail(w, err, "trip guide")
		return
	}
	lists := []store.List{}
	onList := map[int64]bool{}
	if tripID > 0 {
		ls, _ := s.store.ListLists(ctx, uid)
		for _, l := range ls {
			if l.TripID != nil && *l.TripID == tripID {
				lists = append(lists, l)
				for _, p := range all {
					for _, id := range p.Lists {
						if id == l.ID {
							onList[p.ID] = true
						}
					}
				}
			}
		}
	}
	there := map[int64]bool{}
	for _, p := range placesForTrip(all, lat, lng, dest) {
		there[p.ID] = true
	}
	want, been := []store.Place{}, []store.Place{}
	for _, p := range all {
		if !there[p.ID] && !onList[p.ID] {
			continue
		}
		if p.Status == store.StatusWant {
			want = append(want, p)
		} else {
			been = append(been, p)
		}
	}
	sort.SliceStable(been, func(i, j int) bool { return been[i].Rating > been[j].Rating })
	visits := []store.Visit{}
	if from != "" {
		visits, _, _ = s.store.ListVisits(ctx, uid, store.VisitFilter{From: from, To: firstNonEmpty(to, from), Oldest: true})
	} else if tripID > 0 {
		visits, _, _ = s.store.ListVisits(ctx, uid, store.VisitFilter{TripID: tripID, Oldest: true})
	}
	writeJSON(w, http.StatusOK, map[string]any{"trip_id": tripID, "lists": lists, "want": want, "been": been, "visits": visits, "spent": spentOf(visits),
		"url": s.cfg.BaseURL + "/places?trip=" + itoa(tripID)})
}

// handleSpending is what visits cost between two days, per currency and month.
func (s *Server) handleSpending(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	if to == "" {
		to = store.Today()
	}
	if from == "" {
		from = to[:4] + "-01-01"
	}
	if !validDate(from) || !validDate(to) {
		writeErr(w, http.StatusBadRequest, "from and to are YYYY-MM-DD")
		return
	}
	rows, err := s.store.Spending(ctx, uid, from, to)
	if err != nil {
		s.fail(w, err, "spending")
		return
	}
	vs, _, _ := s.store.ListVisits(ctx, uid, store.VisitFilter{From: from, To: to, Planned: ptr(false), Oldest: true})
	type month struct {
		Month  string           `json:"month"`
		Visits int              `json:"visits"`
		Spent  []store.SpentRow `json:"spent"`
	}
	by := map[string][]store.Visit{}
	order := []string{}
	for _, v := range vs {
		m := v.Date[:7]
		if _, ok := by[m]; !ok {
			order = append(order, m)
		}
		by[m] = append(by[m], v)
	}
	months := []month{}
	for _, m := range order {
		months = append(months, month{Month: m, Visits: len(by[m]), Spent: spentOf(by[m])})
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "totals": rows, "months": months})
}

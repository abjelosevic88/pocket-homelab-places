package server

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

// The four routes behind `capabilities` (links.go): /api/v1/pocket/search,
// /day/{date}, /widget and /capture. Each runs as the request's person and
// sees only their own places, visits and lists, as the app's own routes do.
// The SDK's handlers check and cap what goes in and out. Nothing here asks
// another app or the internet.

func placeURL(id int64) string { return "/places/" + itoa(id) }

// visitURL opens a visit on its place's page.
func visitURL(v *store.Visit) string { return placeURL(v.PlaceID) + "?visit=" + itoa(v.ID) }

func listURL(id int64) string { return "/lists/" + itoa(id) }

// dots joins the parts that say something: "Restaurant · Italian · Sarajevo".
func dots(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// count is "1 place", "12 places".
func count(n int, one, many string) string {
	return strconv.Itoa(n) + " " + plural(n, one, many)
}

// starsText is a rating in words: "4.5 stars", "1 star"; "" for none.
func starsText(r float64) string {
	if r <= 0 {
		return ""
	}
	if r == 1 {
		return "1 star"
	}
	return strconv.FormatFloat(r, 'f', -1, 64) + " stars"
}

// withText is "with Ana", "" when nobody came along.
func withText(who string) string {
	if who = strings.TrimSpace(who); who != "" {
		return "with " + who
	}
	return ""
}

// visitWhat is what a visit was, from the kind of place and the time:
// dinner at a restaurant in the evening, coffee at a café; "" when the kind
// and time do not say. Pocket Day words its journal lines the same way.
func visitWhat(kind, clock string) string {
	switch kind {
	case "restaurant", "street_food":
		h, err := strconv.Atoi(strings.SplitN(clock, ":", 2)[0])
		switch {
		case clock == "" || err != nil || h < 5:
			return ""
		case h < 11:
			return "breakfast"
		case h < 16:
			return "lunch"
		}
		return "dinner"
	case "cafe":
		return "coffee"
	case "bar":
		return "drinks"
	case "dessert":
		return "dessert"
	}
	return ""
}

// visitLine is a visit in a few words: "Dinner at Pirpa", "Visited Stari
// Most"; a plan says what it will be: "Dinner at Pirpa", "Vrelo Bosne".
func visitLine(v *store.Visit) string {
	name := firstNonEmpty(strings.TrimSpace(v.PlaceName), "a place")
	if what := visitWhat(v.PlaceKind, v.Time); what != "" {
		return strings.ToUpper(what[:1]) + what[1:] + " at " + name
	}
	if v.Planned {
		return name
	}
	switch v.PlaceKind {
	case "restaurant", "street_food":
		return "Ate at " + name
	case "bakery":
		return "Stopped by " + name
	case "market", "shop":
		return "Shopped at " + name
	case "museum", "sight", "park", "viewpoint", "beach", "nature":
		return "Visited " + name
	case "hotel":
		return "Stayed at " + name
	}
	return "Went to " + name
}

// statusText says where a place stands: "Want to try", "Been there,
// 4.5 stars", "Closed for good".
func statusText(p *store.Place) string {
	if p.Closed {
		return "Closed for good"
	}
	s := "Want to try"
	if p.Status == store.StatusBeen {
		s = "Been there"
	}
	if r := starsText(p.Rating); r != "" {
		s += ", " + r
	}
	if p.Favourite {
		s += ", a favourite"
	}
	return s
}

// placeSnippet is a place in one line: kind · cuisine · city · status.
func placeSnippet(p *store.Place) string {
	return dots(store.KindLabel(p.Kind), p.Cuisine, p.City, statusText(p))
}

// dayWords is a day as people say it from today: "today", "tomorrow",
// "Friday" within the week, else "12 Dec" (with the year when not this one).
func dayWords(date string) string {
	d, err := store.ParseDate(date)
	if err != nil {
		return date
	}
	today := store.Today()
	switch n := store.DaysBetween(today, date); {
	case n == 0:
		return "today"
	case n == 1:
		return "tomorrow"
	case n == -1:
		return "yesterday"
	case n > 1 && n < 7:
		return d.Weekday().String()
	case date[:4] == today[:4]:
		return d.Format("2 Jan")
	}
	return d.Format("2 Jan 2006")
}

// ---------- search ----------

// searchWords are the words of a query worth looking for: the ones with a
// letter or a digit in them ("??" and "*" find nothing), at most eight.
func searchWords(q string) []string {
	var out []string
	for _, w := range strings.Fields(q) {
		if strings.IndexFunc(w, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) < 0 {
			continue
		}
		if out = append(out, w); len(out) == 8 {
			break
		}
	}
	return out
}

// foldedHas reports whether every word is in the text, as the app's own
// search compares them (no case, no accents).
func foldedHas(text string, words []string) bool {
	hay := " " + store.Fold(text) + " "
	for _, w := range words {
		if f := store.Fold(w); f != "" && !strings.Contains(hay, f) {
			return false
		}
	}
	return true
}

// pocketSearch is GET /search in the contract's shape: places by name,
// kind, cuisine, town, address, tags, notes and who recommended them; lists
// by name and description; visits by their notes. Places whose name holds
// the words come first, then other places, lists, visits.
func (s *Server) pocketSearch(r *http.Request, q string, limit int) ([]core.SearchItem, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	words := searchWords(q)
	if len(words) == 0 {
		return nil, nil
	}
	q = strings.Join(words, " ")
	out := []core.SearchItem{}

	ps, err := s.store.ListPlaces(ctx, uid, store.PlaceFilter{Q: q})
	if err != nil {
		return nil, err
	}
	for i := range ps {
		p := &ps[i]
		it := core.SearchItem{ID: "place:" + itoa(p.ID), Title: p.Name, Snippet: placeSnippet(p), Kind: "place", Date: p.LastVisit, URL: placeURL(p.ID), Link: p.Website, Score: 3}
		if foldedHas(p.Name, words) {
			it.Score = 4
		} else if shown := p.Name + " " + it.Snippet; !foldedHas(shown, words) {
			// say where the words were found when the line does not show it
			switch {
			case p.Tags != "" && foldedHas(shown+" "+p.Tags, words):
				it.Snippet = dots(it.Snippet, "Tags: "+p.Tags)
			case p.Notes != "" && foldedHas(shown+" "+p.Notes, words):
				it.Snippet = dots(it.Snippet, p.Notes)
			case p.RecommendedBy != "" && foldedHas(shown+" "+p.RecommendedBy, words):
				it.Snippet = dots(it.Snippet, "Recommended by "+p.RecommendedBy)
			case p.Address != "":
				it.Snippet = dots(it.Snippet, p.Address)
			}
		}
		out = append(out, it)
		if len(out) == limit {
			break
		}
	}

	ls, err := s.store.ListLists(ctx, uid)
	if err != nil {
		return nil, err
	}
	for i := range ls {
		l := &ls[i]
		if !foldedHas(l.Name+" "+l.Description+" "+l.TripName, words) {
			continue
		}
		trip := ""
		if l.TripName != "" {
			trip = "For " + l.TripName
		}
		out = append(out, core.SearchItem{ID: "list:" + itoa(l.ID), Title: strings.TrimSpace(l.Emoji + " " + l.Name), Kind: "list", URL: listURL(l.ID), Score: 2,
			Snippet: dots("List", count(l.Count, "place", "places"), trip, l.Description), Date: l.UpdatedAt})
	}

	// the app's own visit search looks for the words together; the longest
	// word finds the candidates, then every word must be in the notes
	longest := words[0]
	for _, w := range words {
		if len([]rune(w)) > len([]rune(longest)) {
			longest = w
		}
	}
	vs, _, err := s.store.ListVisits(ctx, uid, store.VisitFilter{Q: longest})
	if err != nil {
		return nil, err
	}
	found := 0
	for i := range vs {
		v := &vs[i]
		if v.Notes == "" || !foldedHas(v.Notes, words) {
			continue
		}
		when := dayWords(v.Date)
		if v.Planned {
			when = "Planned for " + when
		}
		out = append(out, core.SearchItem{ID: "visit:" + itoa(v.ID), Title: visitLine(v), Kind: "visit", Date: v.Date, URL: visitURL(v), Score: 1,
			Snippet: dots(when, withText(v.WithWhom), starsText(v.Rating), v.Notes)})
		if found++; found == limit {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out, nil
}

// ---------- day ----------

// pocketDay is GET /day/{date} in the contract's shape: the visits of the
// day, the visits planned for it and the places saved that day (a place
// with a visit or a plan that day shows once, as the visit). Visits keep the
// day and time the person gave them; "saved that day" is in the caller's
// time zone (?tz=), else the server's, as GET /day does.
func (s *Server) pocketDay(r *http.Request, date string, tz *time.Location) (*core.Day, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	loc := time.Local
	if tz != nil {
		loc = tz
	}
	vs, _, err := s.store.ListVisits(ctx, uid, store.VisitFilter{From: date, To: date, Oldest: true})
	if err != nil {
		return nil, err
	}
	ps, err := s.store.ListPlaces(ctx, uid, store.PlaceFilter{})
	if err != nil {
		return nil, err
	}
	day := &core.Day{Date: date}
	seen := map[int64]bool{}
	var done, planned []*store.Visit
	for i := range vs {
		v := &vs[i]
		seen[v.PlaceID] = true
		kind := "visit"
		if v.Planned {
			kind = "planned"
			planned = append(planned, v)
		} else {
			done = append(done, v)
		}
		it := core.DayItem{ID: "visit:" + itoa(v.ID), Time: v.Time, Title: visitLine(v), Kind: kind, URL: visitURL(v),
			Sub: dots(v.City, withText(v.WithWhom), starsText(v.Rating)), Note: strings.TrimSpace(v.Notes)}
		if !v.Planned {
			it.Minutes = float64(v.Minutes)
			if v.Spent > 0 {
				it.Amount, it.Currency = v.Spent, v.Currency
			}
		}
		day.Items = append(day.Items, it)
	}
	added := 0
	for i := range ps {
		p := &ps[i]
		if seen[p.ID] || p.Source == "import" || p.Source == "google" {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, p.CreatedAt)
		if err != nil || at.In(loc).Format("2006-01-02") != date {
			continue
		}
		added++
		day.Items = append(day.Items, core.DayItem{ID: "place:" + itoa(p.ID), Time: at.In(loc).Format("15:04"), Title: p.Name, Kind: "added", URL: placeURL(p.ID),
			Sub: dots(store.KindLabel(p.Kind), p.Cuisine, p.City, statusText(p))})
	}
	if len(day.Items) == 0 {
		return nil, nil
	}

	var parts []string
	switch len(done) {
	case 0:
	case 1:
		parts = append(parts, visitLine(done[0])+commaThen(starsText(done[0].Rating)))
	default:
		parts = append(parts, count(len(done), "visit", "visits"))
	}
	if len(done) > 0 {
		day.Metrics = append(day.Metrics, core.Metric{Label: "Visits", Value: strconv.Itoa(len(done))})
	}
	switch len(planned) {
	case 0:
	case 1:
		parts = append(parts, "Planned: "+lowerFirst(visitLine(planned[0])))
	default:
		parts = append(parts, count(len(planned), "plan", "plans"))
	}
	if added > 0 {
		parts = append(parts, count(added, "place", "places")+" saved")
	}
	var spent []store.Visit
	for _, v := range done {
		spent = append(spent, *v)
	}
	if rows := spentOf(spent); len(rows) == 1 {
		day.Metrics = append(day.Metrics, core.Metric{Label: "Spent", Value: moneyText(rows[0].Total, rows[0].Currency)})
	}
	day.Summary = strings.Join(parts, "; ")
	return day, nil
}

// commaThen is ", " + s, or nothing when s is empty.
func commaThen(s string) string {
	if s == "" {
		return ""
	}
	return ", " + s
}

// lowerFirst lowers the first letter of a line that starts with a word of
// ours ("Dinner at…"), never of a place's name.
func lowerFirst(s string) string {
	for _, w := range []string{"Breakfast ", "Lunch ", "Dinner ", "Coffee ", "Drinks ", "Dessert "} {
		if strings.HasPrefix(s, w) {
			return strings.ToLower(s[:1]) + s[1:]
		}
	}
	return s
}

// ---------- widget ----------

// widgetRows is how many rows the widget shows: the plans coming up first
// (at most four), then the newest places to try.
const widgetRows = 6

// pocketWidget is Places in a few lines: the plans coming up, then the
// places waiting to be tried. It reads the counts and a few rows.
func (s *Server) pocketWidget(r *http.Request) (*core.WidgetData, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	plans, nPlans, err := s.store.ListVisits(ctx, uid, store.VisitFilter{Planned: ptr(true), From: store.Today(), Oldest: true, Limit: 4})
	if err != nil {
		return nil, err
	}
	want, nWant, err := s.store.WantToTry(ctx, uid, widgetRows)
	if err != nil {
		return nil, err
	}
	if nPlans == 0 && nWant == 0 {
		return nil, nil
	}
	out := &core.WidgetData{URL: "/places?status=want", Line: count(nWant, "place", "places") + " to try"}
	if nPlans > 0 {
		out.Metrics = append(out.Metrics, core.Metric{Label: "Planned", Value: strconv.Itoa(nPlans)})
	}
	out.Metrics = append(out.Metrics, core.Metric{Label: "To try", Value: strconv.Itoa(nWant)})
	for i := range plans {
		v := &plans[i]
		when := dayWords(v.Date)
		if i == 0 {
			on := when
			if n := store.DaysBetween(store.Today(), v.Date); n > 1 {
				on = "on " + when
			}
			out.Line, out.URL = visitLine(v)+" "+on, "/"
		}
		if v.Time != "" {
			when += " " + v.Time
		}
		out.Items = append(out.Items, core.WidgetItem{ID: "visit:" + itoa(v.ID), Title: visitLine(v), Detail: dots(upperFirst(when), withText(v.WithWhom)), URL: visitURL(v)})
	}
	for i := range want {
		if len(out.Items) == widgetRows {
			break
		}
		p := &want[i]
		out.Items = append(out.Items, core.WidgetItem{ID: "place:" + itoa(p.ID), Title: p.Name, Detail: dots(store.KindLabel(p.Kind), p.Cuisine, p.City), URL: placeURL(p.ID)})
	}
	return out, nil
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ---------- capture ----------

// capturePlace splits a quick line into a place's name and notes: the title
// when one was given (the text is then the notes), else the text's first
// line, the rest being the notes.
func capturePlace(in core.CaptureInput) (name, notes string) {
	name, notes = in.Title, in.Text
	if name == "" {
		name, notes, _ = strings.Cut(notes, "\n")
	}
	return strings.Join(strings.Fields(name), " "), strings.TrimSpace(notes)
}

// pocketCapture keeps a quick line as a place to try, as POST /places makes
// one from a name alone (a restaurant you want to try, nothing looked up).
// A name the person already has is refused, so a place is not kept twice.
// The place carries no source: the person typed it, the app that passed the
// line on may not change or remove it later. The same request_id again
// answers the same place and adds nothing: the receipt is written in the
// transaction that adds the place, and lines are kept one at a time.
func (s *Server) pocketCapture(r *http.Request, in core.CaptureInput) (*core.CaptureResult, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	op, err := core.NewOperation(fmt.Sprintf("places:user:%d:%s", uid, core.CapturePath), in.RequestID, in)
	if err != nil {
		return nil, err
	}
	s.captureMu.Lock()
	defer s.captureMu.Unlock()
	var res core.CaptureResult
	found, err := op.Replay(ctx, s.store.DB, &res)
	switch {
	case errors.Is(err, core.ErrOperationConflict):
		return nil, fmt.Errorf("%w: this was already kept with other words", core.ErrCapture)
	case err != nil:
		return nil, err
	case found:
		return &res, nil
	}

	name, notes := capturePlace(in)
	p := store.Place{Kind: "restaurant", Status: store.StatusWant}
	if msg := (placeInput{Name: &name, Notes: &notes}).apply(&p, nil); msg != "" {
		return nil, fmt.Errorf("%w: %s", core.ErrCapture, msg)
	}
	all, err := s.store.ListPlaces(ctx, uid, store.PlaceFilter{Q: name})
	if err != nil {
		return nil, err
	}
	for _, x := range all {
		if store.Fold(x.Name) == store.Fold(p.Name) {
			return nil, fmt.Errorf("%w: %s is already in your places", core.ErrCapture, x.Name)
		}
	}
	err = s.store.CreatePlaceThen(ctx, uid, &p, func(tx *sql.Tx) error {
		res = core.CaptureResult{ID: itoa(p.ID), Title: p.Name, URL: placeURL(p.ID)}
		return op.Save(ctx, tx, res)
	})
	if err != nil {
		return nil, err
	}
	if np, err := s.store.GetPlace(ctx, uid, p.ID); err == nil {
		s.publish(ctx, uid, "places.place_saved", "place:"+itoa(p.ID), placePayload(np))
	}
	return &res, nil
}

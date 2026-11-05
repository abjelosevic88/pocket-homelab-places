package server

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-places/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-places/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

// ---- noticing visits ----
//
// Every half hour Places asks Pocket Movement where each person stayed in
// the last days and Pocket Money what they paid, and matches both against
// their places: a stay of 20 minutes or more within 100 m of a place, a
// payment at the place's spot or under its name. Each place and day becomes
// one "how was it?" in the inbox (stay and payment together when both are
// there), with a push. Nothing is logged until the person says so; a
// dismissed one never comes back. Everything is the person's own: their
// own Movement and Money, through their own connections.

// paymentAt reports whether a payment was made at a place, and why.
func paymentAt(p store.Place, pay pocket.Payment, meters float64) (bool, string) {
	if meters <= 0 {
		meters = 100
	}
	if p.HasCoords() && pay.Lat != nil && pay.Lng != nil {
		d := store.Meters(*p.Lat, *p.Lng, *pay.Lat, *pay.Lng)
		if d <= meters*1.5 {
			return true, "paid there"
		}
		if d > 2000 {
			return false, "" // the same name somewhere else
		}
	}
	if store.NameMatches(p.Name, pay.Payee) || store.NameMatches(p.Name, pay.Description) {
		return true, "the name matches"
	}
	return false, ""
}

// nearestPlace is the place closest to a spot within meters, if any.
func nearestPlace(ps []store.Place, lat, lng, meters float64) *store.Place {
	var best *store.Place
	bestD := math.MaxFloat64
	for i := range ps {
		if !ps[i].HasCoords() || ps[i].Closed {
			continue
		}
		d := store.Meters(lat, lng, *ps[i].Lat, *ps[i].Lng)
		if d <= meters && d < bestD {
			best, bestD = &ps[i], d
		}
	}
	return best
}

// scanResult says what one scan found.
type scanResult struct {
	Added    int      `json:"added"`
	Stays    int      `json:"stays"`
	Payments int      `json:"payments"`
	Errors   []string `json:"errors"`
}

// scanUser matches one person's stays and payments of the last days.
func (s *Server) scanUser(ctx context.Context, uid int64, days int) scanResult {
	res := scanResult{Errors: []string{}}
	st, err := s.store.GetSettings(ctx, uid)
	if err != nil || (!st.MatchVisits && !st.MatchPayments) {
		return res
	}
	places, err := s.store.ListPlaces(ctx, uid, store.PlaceFilter{OpenOnly: true})
	if err != nil || len(places) == 0 {
		return res
	}
	meters := float64(max(st.MatchMeters, 30))
	minutes := max(st.MatchMinutes, 5)
	now := time.Now()
	from := now.AddDate(0, 0, -days)
	c := s.clients(ctx, uid)
	if st.MatchVisits {
		conn, err := optional(c.conn(ctx, appMovement, scopeMovementRead))
		if err != nil {
			res.Errors = append(res.Errors, err.Error())
		} else if conn != nil {
			stays, err := conn.client.Stays(ctx, from, now)
			if err != nil {
				res.Errors = append(res.Errors, "Pocket Movement: "+err.Error())
			}
			for _, sv := range stays {
				if sv.Status == "declined" || sv.EndTS <= sv.StartTS {
					continue
				}
				mins := int((sv.EndTS - sv.StartTS) / 60)
				if mins < minutes {
					continue
				}
				p := nearestPlace(places, sv.Lat, sv.Lng, math.Max(meters, math.Min(sv.Radius, 250)))
				if p == nil {
					continue
				}
				at := time.Unix(sv.StartTS, 0)
				date := at.Format("2006-01-02")
				res.Stays++
				if s.store.HasVisitOn(ctx, uid, p.ID, date) {
					continue
				}
				id := sv.ID
				made, err := s.store.NoteInbox(ctx, uid, store.InboxItem{PlaceID: p.ID, Date: date, Time: at.Format("15:04"), Minutes: mins, MovementID: &id})
				if err == nil && made {
					res.Added++
				}
			}
		}
	}
	if st.MatchPayments {
		conn, err := optional(c.conn(ctx, appMoney, scopeTxRead))
		if err != nil {
			res.Errors = append(res.Errors, err.Error())
		} else if conn != nil {
			pays, err := conn.client.Payments(ctx, from.Format("2006-01-02"), now.Format("2006-01-02"))
			if err != nil {
				res.Errors = append(res.Errors, "Pocket Money: "+err.Error())
			}
			for _, pay := range pays {
				if pay.Source == "places" || pay.Kind == "transfer" {
					continue // what Places booked itself is already a visit
				}
				var p *store.Place
				for i := range places {
					if ok, _ := paymentAt(places[i], pay, meters); ok {
						p = &places[i]
						break
					}
				}
				if p == nil {
					continue
				}
				res.Payments++
				if s.store.HasVisitOn(ctx, uid, p.ID, pay.Date) {
					continue
				}
				amt, cur := pay.Paid()
				id := pay.ID
				made, err := s.store.NoteInbox(ctx, uid, store.InboxItem{PlaceID: p.ID, Date: pay.Date, Time: pay.Time, Amount: amt, Currency: cur, MoneyTxID: &id,
					Payee: firstNonEmpty(pay.Payee, pay.Description)})
				if err == nil && made {
					res.Added++
				}
			}
		}
	}
	s.pushInbox(ctx, uid, st, now)
	return res
}

// pushInbox sends one message for the new "how was it?" items, outside
// the night (22:00–08:00 they wait for the morning), and publishes them.
func (s *Server) pushInbox(ctx context.Context, uid int64, st store.Settings, now time.Time) {
	items, err := s.store.ListInbox(ctx, uid, store.InboxOpen, 0)
	if err != nil {
		return
	}
	night := now.Hour() >= 22 || now.Hour() < 8
	var fresh []store.InboxItem
	for _, it := range items {
		if !it.Notified && it.Date >= store.AddDays(store.Today(), -2) {
			fresh = append(fresh, it)
		}
	}
	if len(fresh) == 0 || night {
		return
	}
	var lines []string
	for _, it := range fresh {
		s.store.MarkInboxNotified(ctx, uid, it.ID)
		s.publish(ctx, uid, "places.to_rate", "inbox:"+itoa(it.ID), map[string]any{"inbox_id": it.ID, "place_id": it.PlaceID, "place": it.PlaceName, "date": it.Date,
			"minutes": it.Minutes, "amount": it.Amount, "currency": it.Currency, "title": "How was " + it.PlaceName + "?", "path": "/?inbox=" + itoa(it.ID)})
		lines = append(lines, "• "+inboxLine(it))
	}
	if !st.NotifyToRate || st.NotifyURL == "" {
		return
	}
	title := "How was " + fresh[0].PlaceName + "?"
	if len(fresh) > 1 {
		title = fmt.Sprintf("%d visits to rate", len(fresh))
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: title, Body: strings.Join(lines, "\n"), Priority: "info", Tags: []string{"round_pushpin"}, Link: s.cfg.BaseURL + "/"}); err != nil {
		s.log.Warn("notify", "user", uid, "err", err)
	}
}

// inboxLine is "Café Tito — 1 h 20 min on Tuesday, paid 12.50 BAM".
func inboxLine(it store.InboxItem) string {
	parts := []string{}
	if it.Minutes > 0 {
		parts = append(parts, "you were there "+durationText(it.Minutes))
	}
	if it.Amount > 0 {
		parts = append(parts, "paid "+moneyText(it.Amount, it.Currency))
	}
	day := it.Date
	if d, err := store.ParseDate(it.Date); err == nil {
		switch it.Date {
		case store.Today():
			day = "today"
		case store.AddDays(store.Today(), -1):
			day = "yesterday"
		default:
			day = "on " + d.Format("Monday 2 January")
		}
	}
	return it.PlaceName + " " + day + ": " + strings.Join(parts, ", ")
}

func durationText(m int) string {
	if m < 60 {
		return fmt.Sprintf("%d min", m)
	}
	if m%60 == 0 {
		return fmt.Sprintf("%d h", m/60)
	}
	return fmt.Sprintf("%d h %d min", m/60, m%60)
}

func moneyText(minor int64, cur string) string {
	return strings.TrimSpace(fmt.Sprintf("%.2f %s", float64(minor)/100, cur))
}

// scanAll runs the scan for everybody (the janitor, every half hour).
func (s *Server) scanAll(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	s.store.ExpireInbox(ctx, 21)
	for _, uid := range ids {
		days := 3
		if s.once(ctx, fmt.Sprintf("scanned:%d", uid)) {
			days = 14 // the first time: the last two weeks
		}
		cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		r := s.scanUser(cctx, uid, days)
		cancel()
		if len(r.Errors) > 0 {
			s.log.Debug("scan", "user", uid, "errors", r.Errors)
		}
	}
}

// once reports whether key is new, and remembers it.
func (s *Server) once(ctx context.Context, key string) bool {
	if _, _, done := s.store.CacheGet(ctx, key); done {
		return false
	}
	_ = s.store.CachePut(ctx, key, "1")
	return true
}

// ---------- the inbox API ----------

// handleInbox is what waits for an answer: visits noticed in Movement and
// Money, and plans whose day has passed ("did you go?").
func (s *Server) handleInbox(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	items, err := s.store.ListInbox(ctx, uid, store.InboxOpen, 50)
	if err != nil {
		s.fail(w, err, "inbox")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "past_plans": s.pastPlans(ctx, uid)})
}

// pastPlans are planned visits whose day is over.
func (s *Server) pastPlans(ctx context.Context, uid int64) []store.Visit {
	vs, _, _ := s.store.ListVisits(ctx, uid, store.VisitFilter{Planned: ptr(true), To: store.AddDays(store.Today(), -1), From: store.AddDays(store.Today(), -60)})
	return vs
}

// handleInboxAccept turns an inbox item into a visit: when, how long, what
// was paid (the payment linked, not booked twice) and the person's rating.
func (s *Server) handleInboxAccept(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	it, err := s.store.GetInbox(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "inbox")
		return
	}
	var in struct {
		Rating   float64 `json:"rating"`
		Notes    string  `json:"notes"`
		WithWhom string  `json:"with_whom"`
		Spent    *int64  `json:"spent"`
	}
	_ = decode(r, &in)
	p, err := s.store.GetPlace(ctx, uid, it.PlaceID)
	if err != nil {
		s.fail(w, err, "place")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	v := store.Visit{PlaceID: it.PlaceID, Date: it.Date, Time: it.Time, Minutes: it.Minutes, Rating: in.Rating, Notes: strings.TrimSpace(in.Notes),
		WithWhom: strings.TrimSpace(in.WithWhom), Spent: it.Amount, Currency: firstNonEmpty(it.Currency, currencyFor(p, st)), MoneyTxID: it.MoneyTxID}
	if in.Spent != nil && *in.Spent >= 0 {
		v.Spent = *in.Spent
	}
	if !s.store.HasVisitOn(ctx, uid, p.ID, it.Date) {
		s.fileWithTrip(ctx, uid, &v)
		if err := s.store.CreateVisit(ctx, uid, &v); err != nil {
			s.fail(w, err, "add visit")
			return
		}
		s.publish(ctx, uid, "places.visit_logged", "visit:"+itoa(v.ID), visitPayload(&v))
	}
	_ = s.store.SetInboxStatus(ctx, uid, it.ID, store.InboxDone)
	writeJSON(w, http.StatusOK, map[string]any{"visit": v})
}

func (s *Server) handleInboxDismiss(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.SetInboxStatus(ctx, actorID(ctx), idParam(r, "id"), store.InboxDismissed); err != nil {
		s.fail(w, err, "inbox")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleInboxScan looks for visits now (the last two weeks).
func (s *Server) handleInboxScan(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.scanUser(ctx, actorID(ctx), 14))
}

// ---------- Pocket Movement on a place's page ----------

type movementSummary struct {
	Visits  int    `json:"visits"`
	Minutes int    `json:"minutes"`
	First   string `json:"first"`
	Last    string `json:"last"`
	URL     string `json:"url"`
	Error   string `json:"error,omitempty"`
}

// movementAt is how often Pocket Movement saw the person at a place in the
// last two years (cached an hour). nil when Movement is not connected.
func (s *Server) movementAt(ctx context.Context, uid int64, p *store.Place) *movementSummary {
	key := fmt.Sprintf("mvat:%d:%d:%.5f,%.5f", uid, p.ID, *p.Lat, *p.Lng)
	if raw, at, ok := s.store.CacheGet(ctx, key); ok {
		if t, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(t) < time.Hour {
			var m movementSummary
			if remarshal(jsonRaw(raw), &m) == nil {
				return &m
			}
		}
	}
	cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	conn, err := optional(s.clients(cctx, uid).conn(cctx, appMovement, scopeMovementRead))
	if err != nil {
		return &movementSummary{Error: err.Error()}
	}
	if conn == nil {
		return nil
	}
	stays, err := conn.client.Stays(cctx, time.Now().AddDate(-2, 0, 0), time.Now())
	if err != nil {
		return &movementSummary{Error: "Pocket Movement: " + err.Error()}
	}
	st, _ := s.store.GetSettings(ctx, uid)
	m := movementSummary{}
	days := map[string]bool{}
	for _, sv := range stays {
		if sv.Status == "declined" || store.Meters(*p.Lat, *p.Lng, sv.Lat, sv.Lng) > math.Max(float64(st.MatchMeters), 60) {
			continue
		}
		day := time.Unix(sv.StartTS, 0).Format("2006-01-02")
		m.Minutes += int((sv.EndTS - sv.StartTS) / 60)
		if !days[day] {
			days[day] = true
			m.Visits++
		}
		if m.First == "" || day < m.First {
			m.First = day
		}
		if day > m.Last {
			m.Last = day
		}
	}
	if m.Last != "" {
		m.URL = strings.TrimRight(conn.public, "/") + "/timeline/" + m.Last
	}
	if b, err := jsonString(m); err == nil {
		_ = s.store.CachePut(ctx, key, b)
	}
	return &m
}

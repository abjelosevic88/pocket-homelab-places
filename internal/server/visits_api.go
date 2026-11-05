package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-places/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

// visitInput is a visit as the forms and other apps send it; nil = not sent.
type visitInput struct {
	PlaceID   *int64   `json:"place_id"`
	Date      *string  `json:"date"`
	Time      *string  `json:"time"`
	Planned   *bool    `json:"planned"`
	Rating    *float64 `json:"rating"`
	Notes     *string  `json:"notes"`
	WithWhom  *string  `json:"with_whom"`
	Minutes   *int     `json:"minutes"`
	Spent     *int64   `json:"spent"`
	Currency  *string  `json:"currency"`
	TripID    *int64   `json:"trip_id"`
	TripName  *string  `json:"trip_name"`
	SourceRef *string  `json:"source_ref"`

	// What to do in the other apps on create.
	Money      *bool  `json:"money"`       // book what it cost in Pocket Money
	MoneyTxID  *int64 `json:"money_tx_id"` // or link a payment already there
	AccountID  int64  `json:"account_id"`
	CategoryID int64  `json:"category_id"`
	Calendar   *bool  `json:"calendar"` // a planned visit on Pocket Calendar
}

func (in visitInput) apply(v *store.Visit, keys map[string]bool) string {
	if in.PlaceID != nil {
		v.PlaceID = *in.PlaceID
	}
	if in.Date != nil {
		v.Date = strings.TrimSpace(*in.Date)
	}
	if in.Time != nil {
		v.Time = strings.TrimSpace(*in.Time)
		if v.Time != "" {
			if _, err := time.Parse("15:04", v.Time); err != nil {
				return "the time is HH:MM"
			}
		}
	}
	if in.Planned != nil {
		v.Planned = *in.Planned
	}
	if in.Rating != nil {
		if *in.Rating < 0 || *in.Rating > 5 {
			return "the rating is 0 to 5"
		}
		v.Rating = *in.Rating
	}
	trimP(&v.Notes, in.Notes)
	trimP(&v.WithWhom, in.WithWhom)
	if in.Minutes != nil {
		v.Minutes = *in.Minutes
	}
	if in.Spent != nil {
		if *in.Spent < 0 || *in.Spent > 1e11 {
			return "what you spent is a positive amount in cents"
		}
		v.Spent = *in.Spent
	}
	if in.Currency != nil {
		c := strings.TrimSpace(*in.Currency)
		if c != "" && cleanCurrency(c) == "" {
			return "the currency is a three-letter code like EUR"
		}
		v.Currency = cleanCurrency(c)
	}
	if keys["trip_id"] {
		v.TripID = in.TripID
	}
	trimP(&v.TripName, in.TripName)
	if v.PlaceID <= 0 {
		return "choose the place"
	}
	if v.Date == "" || !validDate(v.Date) {
		return "the date is YYYY-MM-DD"
	}
	if len(v.Notes) > 20000 {
		return "the notes are too long"
	}
	return ""
}

func visitPayload(v *store.Visit) map[string]any {
	return map[string]any{"visit_id": v.ID, "place_id": v.PlaceID, "place": v.PlaceName, "kind": v.PlaceKind, "city": v.City, "date": v.Date, "time": v.Time,
		"planned": v.Planned, "rating": v.Rating, "spent": v.Spent, "currency": v.Currency, "with": v.WithWhom, "title": visitTitle(v), "path": "/places/" + itoa(v.PlaceID)}
}

func visitTitle(v *store.Visit) string {
	if v.Planned {
		return "Planned: " + v.PlaceName
	}
	return v.PlaceName
}

// currencyFor is the currency of a visit at a place: the place's, else the person's.
func currencyFor(p *store.Place, st store.Settings) string {
	return firstNonEmpty(p.Currency, st.Currency, "EUR")
}

func (s *Server) handleListVisits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	f := store.VisitFilter{From: q.Get("from"), To: q.Get("to"), Q: strings.TrimSpace(q.Get("q")), Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset")}
	f.PlaceID, _ = strconv.ParseInt(q.Get("place_id"), 10, 64)
	f.TripID, _ = strconv.ParseInt(q.Get("trip_id"), 10, 64)
	if v := q.Get("planned"); v != "" {
		f.Planned = ptr(queryBool(r, "planned"))
	}
	if !validDate(f.From) || !validDate(f.To) {
		writeErr(w, http.StatusBadRequest, "from and to are YYYY-MM-DD")
		return
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 200
	}
	vs, total, err := s.store.ListVisits(ctx, actorID(ctx), f)
	if err != nil {
		s.fail(w, err, "visits")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": vs, "total": total, "spent": spentOf(vs)})
}

func (s *Server) handleGetVisit(w http.ResponseWriter, r *http.Request) {
	v, err := s.store.GetVisit(r.Context(), actorID(r.Context()), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "visit")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) handleCreateVisit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in visitInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	v := store.Visit{Date: store.Today(), Source: linkApp(ctx)}
	if msg := in.apply(&v, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	p, err := s.store.GetPlace(ctx, uid, v.PlaceID)
	if err != nil {
		s.fail(w, err, "place")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	if v.Currency == "" {
		v.Currency = currencyFor(p, st)
	}
	if in.Planned == nil && v.Date > store.Today() {
		v.Planned = true
	}
	if in.SourceRef != nil && v.Source != "" {
		v.SourceRef = strings.TrimSpace(*in.SourceRef)
		if old, err := s.store.VisitBySource(ctx, uid, v.Source, v.SourceRef); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"visit": old})
			return
		}
	}
	if in.MoneyTxID != nil && *in.MoneyTxID > 0 {
		v.MoneyTxID = in.MoneyTxID
	}
	if v.TripID == nil && !v.Planned {
		s.fileWithTrip(ctx, uid, &v)
	}
	if err := s.store.CreateVisit(ctx, uid, &v); err != nil {
		s.fail(w, err, "add visit")
		return
	}
	out := map[string]any{}
	if !v.Planned {
		s.store.CloseInboxFor(ctx, uid, v.PlaceID, v.Date)
		if v.Spent > 0 && v.MoneyTxID == nil && s.wantsMoney(ctx, uid, in.Money) {
			out["money"] = s.bookVisit(ctx, uid, &v, in.AccountID, in.CategoryID)
		}
	} else if (in.Calendar == nil && st.CalendarPlans) || (in.Calendar != nil && *in.Calendar) {
		out["calendar"] = s.syncVisitCalendar(ctx, uid, &v, p)
	}
	nv, _ := s.store.GetVisit(ctx, uid, v.ID)
	out["visit"] = nv
	topic := "places.visit_logged"
	if nv.Planned {
		topic = "places.visit_planned"
	}
	s.publish(ctx, uid, topic, "visit:"+itoa(v.ID), visitPayload(nv))
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdateVisit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	v, err := s.store.GetVisit(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "visit")
		return
	}
	if !linkMayChange(ctx, v.Source) {
		writeErr(w, http.StatusForbidden, "this app may only change the visits it added")
		return
	}
	var in visitInput
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	before := *v
	if msg := in.apply(v, keys); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if err := s.store.UpdateVisit(ctx, uid, v, before.PlaceID); err != nil {
		s.fail(w, err, "save visit")
		return
	}
	out := map[string]any{}
	if v.MoneyBooked && (v.Spent != before.Spent || v.Date != before.Date || v.Currency != before.Currency || v.PlaceID != before.PlaceID) {
		out["money"] = s.syncVisitMoney(ctx, uid, v)
	}
	if v.CalEventID != nil || (v.Planned && !before.Planned) {
		if p, err := s.store.GetPlace(ctx, uid, v.PlaceID); err == nil && (v.Planned || before.Planned) {
			out["calendar"] = s.syncVisitCalendar(ctx, uid, v, p)
		}
	}
	if before.Planned && !v.Planned {
		s.store.CloseInboxFor(ctx, uid, v.PlaceID, v.Date)
	}
	nv, _ := s.store.GetVisit(ctx, uid, v.ID)
	out["visit"] = nv
	topic := "places.visit_logged"
	if nv.Planned {
		topic = "places.visit_planned"
	}
	s.publish(ctx, uid, topic, "visit:"+itoa(v.ID), visitPayload(nv))
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteVisit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	v, err := s.store.GetVisit(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "visit")
		return
	}
	if !linkMayChange(ctx, v.Source) {
		writeErr(w, http.StatusForbidden, "this app may only remove the visits it added")
		return
	}
	if err := s.store.DeleteVisit(ctx, uid, v.ID); err != nil {
		s.fail(w, err, "delete visit")
		return
	}
	cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	s.dropVisitLinks(cctx, uid, v)
	w.WriteHeader(http.StatusNoContent)
}

// dropVisitLinks removes what Places made for a visit that is gone: the
// transaction it booked and the calendar event of a plan. A bill split in
// Pocket Split stays there (people may have settled it already).
func (s *Server) dropVisitLinks(ctx context.Context, uid int64, v *store.Visit) {
	if v.MoneyBooked && v.MoneyTxID != nil {
		if conn, err := s.clients(ctx, uid).conn(ctx, appMoney, scopeTxWrite); err == nil {
			_ = conn.client.DeleteTx(ctx, *v.MoneyTxID)
		}
	}
	if v.CalEventID != nil {
		if conn, err := s.clients(ctx, uid).conn(ctx, appCalendar, scopeCalendarWrite); err == nil {
			_ = conn.client.DeleteEvent(ctx, *v.CalEventID)
		}
	}
}

// fileWithTrip files a visit during a Pocket Travel trip with that trip.
func (s *Server) fileWithTrip(ctx context.Context, uid int64, v *store.Visit) {
	cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	conn, err := optional(s.clients(cctx, uid).conn(cctx, appTravel, scopeTripsRead))
	if err != nil || conn == nil {
		return
	}
	trips, err := conn.client.Trips(cctx)
	if err != nil {
		return
	}
	for _, t := range trips {
		if t.StartDate != "" && v.Date >= t.StartDate && v.Date <= firstNonEmpty(t.EndDate, t.StartDate) && !strings.EqualFold(t.Status, "cancelled") {
			id := t.ID
			v.TripID, v.TripName = &id, firstNonEmpty(t.Name, t.Destination)
			return
		}
	}
}

// ---------- Pocket Money ----------

type moneyResult struct {
	OK      bool   `json:"ok"`
	TxID    int64  `json:"tx_id,omitempty"`
	Account string `json:"account,omitempty"`
	URL     string `json:"url,omitempty"`
	Via     string `json:"via,omitempty"`
	Error   string `json:"error,omitempty"`
	Skipped bool   `json:"skipped,omitempty"`
}

// wantsMoney is the person's choice for this cost: the form's tick, else
// the setting — and only when Money is connected at all.
func (s *Server) wantsMoney(ctx context.Context, uid int64, asked *bool) bool {
	st, _ := s.store.GetSettings(ctx, uid)
	mode, _, token, _ := connSettings(st, appMoney)
	if s.connMode(mode, token) == modeOff {
		return false
	}
	if asked != nil {
		return *asked
	}
	return st.MoneyBook
}

// eatingOutWords match a Money category for eating out by name.
var eatingOutWords = []string{"restaurants", "restaurant", "eating out", "dining out", "dining", "food & drink", "food and drink", "cafes", "café", "coffee",
	"restorani", "restoran", "izlasci", "kafići", "kafici", "hrana van kuće", "going out", "food"}

func pickAccount(accts []pocket.Account, want, setting int64) *pocket.Account {
	for _, id := range []int64{want, setting} {
		for i := range accts {
			if id > 0 && accts[i].ID == id {
				return &accts[i]
			}
		}
	}
	for i := range accts {
		if accts[i].Everyday {
			return &accts[i]
		}
	}
	if len(accts) > 0 {
		return &accts[0]
	}
	return nil
}

func pickCategory(cats []pocket.Category, want, setting int64) int64 {
	for _, id := range []int64{want, setting} {
		for _, c := range cats {
			if id > 0 && c.ID == id {
				return c.ID
			}
		}
	}
	for _, w := range eatingOutWords {
		for _, c := range cats {
			if strings.EqualFold(c.Name, w) {
				return c.ID
			}
		}
	}
	for _, w := range eatingOutWords {
		for _, c := range cats {
			if strings.Contains(strings.ToLower(c.FullName+" "+c.Name), w) {
				return c.ID
			}
		}
	}
	return 0
}

func (s *Server) moneyTx(ctx context.Context, uid int64, v *store.Visit) pocket.Tx {
	tx := pocket.Tx{Date: v.Date, Time: v.Time, Amount: v.Spent, Currency: v.Currency, Description: v.PlaceName, Payee: v.PlaceName,
		Notes: strings.TrimSpace("Pocket Places. " + v.Notes), ExternalID: fmt.Sprintf("places:visit:%d", v.ID)}
	if p, err := s.store.GetPlace(ctx, uid, v.PlaceID); err == nil {
		tx.Lat, tx.Lng = p.Lat, p.Lng
	}
	return tx
}

// bookVisit creates the transaction for what a visit cost, as the person,
// and remembers it. Errors come back as text; the visit itself is kept.
func (s *Server) bookVisit(ctx context.Context, uid int64, v *store.Visit, accountID, categoryID int64) moneyResult {
	s.moneyMu.Lock()
	defer s.moneyMu.Unlock()
	if v.MoneyTxID != nil {
		return moneyResult{OK: true, TxID: *v.MoneyTxID, Skipped: true}
	}
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	c := s.clients(cctx, uid)
	conn, err := c.conn(cctx, appMoney, scopeAccountsRead, scopeTxWrite)
	if err != nil {
		return moneyResult{Error: err.Error()}
	}
	accts, err := conn.client.Accounts(cctx)
	if err != nil {
		return moneyResult{Error: "Pocket Money: " + err.Error(), Via: conn.via}
	}
	acct := pickAccount(accts, accountID, c.st.MoneyAccountID)
	if acct == nil {
		return moneyResult{Error: "Pocket Money has no account you can book on", Via: conn.via}
	}
	cats, _ := conn.client.Categories(cctx, acct.ID)
	tx := s.moneyTx(cctx, uid, v)
	tx.AccountID, tx.CategoryID = acct.ID, pickCategory(cats, categoryID, c.st.MoneyCategoryID)
	id, err := conn.client.CreateTx(cctx, tx)
	if err != nil {
		return moneyResult{Error: "Pocket Money: " + err.Error(), Via: conn.via}
	}
	_ = s.store.SetVisitMoney(ctx, uid, v.ID, &id, true)
	return moneyResult{OK: true, TxID: id, Account: acct.Name, Via: conn.via, URL: strings.TrimRight(conn.public, "/") + "/transactions/" + itoa(id)}
}

// syncVisitMoney carries an edited visit over to the transaction Places
// booked; a cost of 0 removes it.
func (s *Server) syncVisitMoney(ctx context.Context, uid int64, v *store.Visit) moneyResult {
	if v.MoneyTxID == nil || !v.MoneyBooked {
		return moneyResult{Skipped: true}
	}
	s.moneyMu.Lock()
	defer s.moneyMu.Unlock()
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	conn, err := s.clients(cctx, uid).conn(cctx, appMoney, scopeTxWrite)
	if err != nil {
		return moneyResult{Error: err.Error()}
	}
	if v.Spent == 0 {
		if err := conn.client.DeleteTx(cctx, *v.MoneyTxID); err != nil {
			return moneyResult{Error: "Pocket Money: " + err.Error(), Via: conn.via}
		}
		_ = s.store.SetVisitMoney(ctx, uid, v.ID, nil, false)
		return moneyResult{OK: true, Via: conn.via}
	}
	err = conn.client.UpdateTx(cctx, *v.MoneyTxID, s.moneyTx(cctx, uid, v))
	if errors.Is(err, pocket.ErrNotFound) {
		_ = s.store.SetVisitMoney(ctx, uid, v.ID, nil, false) // deleted in Money meanwhile
		return moneyResult{OK: true, Skipped: true}
	}
	if err != nil {
		return moneyResult{Error: "Pocket Money: " + err.Error(), Via: conn.via}
	}
	return moneyResult{OK: true, TxID: *v.MoneyTxID, Via: conn.via}
}

// handleVisitMoney books the visit's cost, or links a payment already in
// Pocket Money ({"tx_id": 12}), or ({"unlink": true}) forgets the link (a
// transaction Places booked itself is removed).
func (s *Server) handleVisitMoney(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	v, err := s.store.GetVisit(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "visit")
		return
	}
	var in struct {
		TxID       int64 `json:"tx_id"`
		Unlink     bool  `json:"unlink"`
		AccountID  int64 `json:"account_id"`
		CategoryID int64 `json:"category_id"`
	}
	_ = decode(r, &in)
	switch {
	case in.Unlink:
		if v.MoneyBooked && v.MoneyTxID != nil {
			cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
			defer cancel()
			if conn, err := s.clients(cctx, uid).conn(cctx, appMoney, scopeTxWrite); err == nil {
				if err := conn.client.DeleteTx(cctx, *v.MoneyTxID); err != nil {
					writeJSON(w, http.StatusOK, map[string]any{"money": moneyResult{Error: "Pocket Money: " + err.Error()}})
					return
				}
			}
		}
		_ = s.store.SetVisitMoney(ctx, uid, v.ID, nil, false)
		writeJSON(w, http.StatusOK, map[string]any{"money": moneyResult{OK: true}})
	case in.TxID > 0:
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		conn, err := s.clients(cctx, uid).conn(cctx, appMoney, scopeTxRead)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"money": moneyResult{Error: err.Error()}})
			return
		}
		pay, err := conn.client.GetPayment(cctx, in.TxID)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"money": moneyResult{Error: "Pocket Money: " + err.Error()}})
			return
		}
		_ = s.store.SetVisitMoney(ctx, uid, v.ID, &pay.ID, false)
		if v.Spent == 0 {
			v.Spent, v.Currency = pay.Paid()
			_ = s.store.UpdateVisit(ctx, uid, v, v.PlaceID)
		}
		writeJSON(w, http.StatusOK, map[string]any{"money": moneyResult{OK: true, TxID: pay.ID, Via: conn.via, URL: strings.TrimRight(conn.public, "/") + "/transactions/" + itoa(pay.ID)}})
	default:
		if v.Spent <= 0 {
			writeErr(w, http.StatusBadRequest, "enter what you spent first")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"money": s.bookVisit(ctx, uid, v, in.AccountID, in.CategoryID)})
	}
}

// handleVisitPayments lists Pocket Money payments around the visit's day,
// the likely ones (the place's name or spot) first — to link instead of
// booking twice.
func (s *Server) handleVisitPayments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	v, err := s.store.GetVisit(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "visit")
		return
	}
	p, err := s.store.GetPlace(ctx, uid, v.PlaceID)
	if err != nil {
		s.fail(w, err, "place")
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	conn, err := s.clients(cctx, uid).conn(cctx, appMoney, scopeTxRead)
	if err != nil {
		pick(w, nil, err, appMoney)
		return
	}
	pays, err := conn.client.Payments(cctx, store.AddDays(v.Date, -1), store.AddDays(v.Date, 1))
	if err != nil {
		pick(w, nil, err, appMoney)
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	type cand struct {
		pocket.Payment
		Likely bool   `json:"likely"`
		Why    string `json:"why"`
	}
	out := []cand{}
	for _, pay := range pays {
		c := cand{Payment: pay}
		if ok, why := paymentAt(*p, pay, float64(st.MatchMeters)); ok {
			c.Likely, c.Why = true, why
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Likely && !out[j].Likely })
	if len(out) > 40 {
		out = out[:40]
	}
	pick(w, out, nil, appMoney)
}

// ---------- Pocket Calendar: a planned visit as an event ----------

type calResult struct {
	OK      bool   `json:"ok"`
	EventID int64  `json:"event_id,omitempty"`
	Error   string `json:"error,omitempty"`
	Removed bool   `json:"removed,omitempty"`
}

// syncVisitCalendar puts a planned visit on the person's calendar (or moves
// it); a visit that is no longer a plan in the future leaves its event
// where it was, since the day happened.
func (s *Server) syncVisitCalendar(ctx context.Context, uid int64, v *store.Visit, p *store.Place) calResult {
	s.calMu.Lock()
	defer s.calMu.Unlock()
	if !v.Planned && v.CalEventID == nil {
		return calResult{OK: true}
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	c := s.clients(cctx, uid)
	conn, err := c.conn(cctx, appCalendar, scopeCalendarWrite)
	if err != nil {
		return calResult{Error: err.Error()}
	}
	if !v.Planned {
		return calResult{OK: true}
	}
	desc := strings.TrimSpace(strings.Join(nonEmpty(placeLine(p), p.Phone, v.WithWhom, v.Notes), "\n"))
	ev := pocket.Event{CalendarID: c.st.CalendarID, Title: p.Name, Description: desc + "\n\nPocket Places", Location: strings.Join(nonEmpty(p.Address, p.City), ", "),
		URL: s.cfg.BaseURL + "/places/" + itoa(p.ID), Date: v.Date, Time: v.Time, Minutes: 120}
	if v.CalEventID != nil {
		err := conn.client.UpdateEvent(cctx, *v.CalEventID, ev)
		if err == nil {
			return calResult{OK: true, EventID: *v.CalEventID}
		}
		if !errors.Is(err, pocket.ErrNotFound) {
			return calResult{Error: "Pocket Calendar: " + err.Error()}
		}
	}
	id, err := conn.client.CreateEvent(cctx, ev)
	if err != nil || id == 0 {
		msg := "Pocket Calendar gave no event"
		if err != nil {
			msg = "Pocket Calendar: " + err.Error()
		}
		return calResult{Error: msg}
	}
	_ = s.store.SetVisitCalendar(ctx, uid, v.ID, &id)
	return calResult{OK: true, EventID: id}
}

// placeLine is "Restaurant · Bosnian · €€".
func placeLine(p *store.Place) string {
	parts := []string{store.KindLabel(p.Kind)}
	if p.Cuisine != "" {
		parts = append(parts, p.Cuisine)
	}
	if p.Price > 0 {
		parts = append(parts, strings.Repeat("€", p.Price))
	}
	return strings.Join(parts, " · ")
}

func (s *Server) handleVisitCalendar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	v, err := s.store.GetVisit(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "visit")
		return
	}
	if !v.Planned {
		writeErr(w, http.StatusBadRequest, "only a planned visit goes on the calendar")
		return
	}
	p, err := s.store.GetPlace(ctx, uid, v.PlaceID)
	if err != nil {
		s.fail(w, err, "place")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"calendar": s.syncVisitCalendar(ctx, uid, v, p)})
}

// ---------- Pocket Split: share the bill ----------

func (s *Server) handleVisitSplit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	v, err := s.store.GetVisit(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "visit")
		return
	}
	var in struct {
		GroupID   int64   `json:"group_id"`
		MemberIDs []int64 `json:"member_ids"`
		Amount    int64   `json:"amount"`
	}
	if err := decode(r, &in); err != nil || in.GroupID <= 0 {
		writeErr(w, http.StatusBadRequest, "choose the group")
		return
	}
	amount := firstPos(in.Amount, v.Spent)
	if amount <= 0 {
		writeErr(w, http.StatusBadRequest, "enter what the bill came to")
		return
	}
	if v.SplitExpenseID != nil {
		writeErr(w, http.StatusConflict, "this bill is already split in Pocket Split")
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	conn, err := s.clients(cctx, uid).conn(cctx, appSplit, scopeGroupsRead, scopeExpensesWrite)
	if err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	groups, err := conn.client.SplitGroups(cctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Pocket Split: "+err.Error())
		return
	}
	var g *pocket.SplitGroup
	for i := range groups {
		if groups[i].ID == in.GroupID {
			g = &groups[i]
		}
	}
	if g == nil {
		writeErr(w, http.StatusNotFound, "that group is not one of yours in Pocket Split")
		return
	}
	members := in.MemberIDs
	if len(members) == 0 {
		for _, m := range g.Members {
			members = append(members, m.ID)
		}
	}
	title := v.PlaceName
	if v.WithWhom != "" {
		title += " (" + v.WithWhom + ")"
	}
	id, err := conn.client.CreateSplitExpense(cctx, pocket.SplitExpense{GroupID: g.ID, Title: title, Amount: amount, Curr: v.Currency, Date: v.Date,
		Notes: "Pocket Places", PaidBy: g.MyMemberID, Members: members})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Pocket Split: "+err.Error())
		return
	}
	gid := g.ID
	_ = s.store.SetVisitSplit(ctx, uid, v.ID, &gid, &id)
	st, _ := s.store.GetSettings(ctx, uid)
	if st.SplitGroupID != gid {
		st.SplitGroupID = gid
		_ = s.store.SaveUserSettings(ctx, uid, st)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "expense_id": id, "group": g.Name, "url": strings.TrimRight(conn.public, "/") + "/groups/" + itoa(g.ID)})
}

func firstPos(v ...int64) int64 {
	for _, x := range v {
		if x > 0 {
			return x
		}
	}
	return 0
}

package store

import (
	"context"
	"database/sql"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DaysBetween is the number of days from a to b (both YYYY-MM-DD).
func DaysBetween(a, b string) int {
	ta, err1 := ParseDate(a)
	tb, err2 := ParseDate(b)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(math.Round(tb.Sub(ta).Hours() / 24))
}

func nullF(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func fptr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

func iptr(n sql.NullInt64) *int64 {
	if !n.Valid || n.Int64 == 0 {
		return nil
	}
	v := n.Int64
	return &v
}

// ---------- places ----------

// PlaceFilter narrows a listing of places. Text search, sorting and
// distance are done in Go so accents and "near me" work.
type PlaceFilter struct {
	Status    string // want | been | ""
	Kind      string
	Group     string // eat | shop | see | stay | other
	City      string
	Country   string
	Tag       string
	Q         string
	ListID    int64
	Favourite bool
	OpenOnly  bool // leave out places closed for good
	Sort      string
	Near      *[2]float64
	Radius    float64 // metres, with Near
	Limit     int
}

const placeCols = `p.id, p.name, p.kind, p.cuisine, p.status, p.favourite, p.closed, p.rating, p.price, p.address, p.city, p.country, p.lat, p.lng,
	p.phone, p.website, p.map_url, p.hours, p.tags, p.notes, p.recommended_by, p.contact_id, p.currency, p.source, p.source_ref, p.created_at, p.updated_at,
	(SELECT COUNT(1) FROM visits v WHERE v.place_id = p.id AND v.planned = 0),
	COALESCE((SELECT MAX(date) FROM visits v WHERE v.place_id = p.id AND v.planned = 0), ''),
	COALESCE((SELECT MIN(date) FROM visits v WHERE v.place_id = p.id AND v.planned = 1 AND v.date >= ?), ''),
	COALESCE((SELECT SUM(spent) FROM visits v WHERE v.place_id = p.id AND v.planned = 0), 0),
	COALESCE((SELECT group_concat(list_id) FROM list_places lp WHERE lp.place_id = p.id), '')`

type scanner interface{ Scan(...any) error }

func scanPlace(sc scanner) (Place, error) {
	var p Place
	var fav, closed int
	var lat, lng sql.NullFloat64
	var contact sql.NullInt64
	var lists string
	err := sc.Scan(&p.ID, &p.Name, &p.Kind, &p.Cuisine, &p.Status, &fav, &closed, &p.Rating, &p.Price, &p.Address, &p.City, &p.Country, &lat, &lng,
		&p.Phone, &p.Website, &p.MapURL, &p.Hours, &p.Tags, &p.Notes, &p.RecommendedBy, &contact, &p.Currency, &p.Source, &p.SourceRef, &p.CreatedAt, &p.UpdatedAt,
		&p.Visits, &p.LastVisit, &p.NextPlan, &p.Spent, &lists)
	if err != nil {
		return p, err
	}
	p.Favourite, p.Closed = fav == 1, closed == 1
	p.Lat, p.Lng, p.ContactID = fptr(lat), fptr(lng), iptr(contact)
	p.Lists = []int64{}
	for _, x := range strings.Split(lists, ",") {
		if id, err := strconv.ParseInt(x, 10, 64); err == nil {
			p.Lists = append(p.Lists, id)
		}
	}
	sort.Slice(p.Lists, func(i, j int) bool { return p.Lists[i] < p.Lists[j] })
	return p, nil
}

// Haystack is everything a search looks through, folded.
func (p Place) Haystack() string {
	return Fold(strings.Join([]string{p.Name, p.Cuisine, KindLabel(p.Kind), p.City, p.Address, p.Country, p.Tags, p.Notes, p.RecommendedBy}, " "))
}

// ListPlaces lists one person's places.
func (s *Store) ListPlaces(ctx context.Context, uid int64, f PlaceFilter) ([]Place, error) {
	q := `SELECT ` + placeCols + ` FROM places p WHERE p.user_id = ?`
	args := []any{Today(), uid}
	if f.Status != "" {
		q += ` AND p.status = ?`
		args = append(args, f.Status)
	}
	if f.Kind != "" {
		q += ` AND p.kind = ?`
		args = append(args, f.Kind)
	}
	if f.Country != "" {
		q += ` AND p.country = ?`
		args = append(args, strings.ToUpper(f.Country))
	}
	if f.City != "" {
		q += ` AND lower(p.city) = lower(?)`
		args = append(args, f.City)
	}
	if f.Favourite {
		q += ` AND p.favourite = 1`
	}
	if f.OpenOnly {
		q += ` AND p.closed = 0`
	}
	if f.ListID > 0 {
		q += ` AND EXISTS (SELECT 1 FROM list_places lp WHERE lp.place_id = p.id AND lp.list_id = ?)`
		args = append(args, f.ListID)
	}
	q += ` ORDER BY p.name COLLATE NOCASE, p.id`
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Place{}
	terms := strings.Fields(Fold(f.Q))
	tag := Fold(f.Tag)
	for rows.Next() {
		p, err := scanPlace(rows)
		if err != nil {
			return nil, err
		}
		if f.Group != "" && KindGroup(p.Kind) != f.Group {
			continue
		}
		if tag != "" && !containsTag(p.Tags, tag) {
			continue
		}
		if len(terms) > 0 {
			hay := " " + p.Haystack() + " "
			miss := false
			for _, t := range terms {
				if !strings.Contains(hay, t) {
					miss = true
					break
				}
			}
			if miss {
				continue
			}
		}
		if f.Near != nil && p.HasCoords() {
			d := math.Round(Meters(f.Near[0], f.Near[1], *p.Lat, *p.Lng))
			p.Distance = &d
		}
		if f.Near != nil && f.Radius > 0 && (p.Distance == nil || *p.Distance > f.Radius) {
			continue
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortPlaces(out, f.Sort)
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func containsTag(tags, want string) bool {
	for _, t := range SplitTags(tags) {
		if Fold(t) == want {
			return true
		}
	}
	return false
}

func sortPlaces(ps []Place, by string) {
	switch by {
	case "rating":
		sort.SliceStable(ps, func(i, j int) bool { return ps[i].Rating > ps[j].Rating })
	case "recent":
		sort.SliceStable(ps, func(i, j int) bool {
			a, b := firstNon(ps[i].LastVisit, ps[i].CreatedAt[:10]), firstNon(ps[j].LastVisit, ps[j].CreatedAt[:10])
			return a > b
		})
	case "added":
		sort.SliceStable(ps, func(i, j int) bool { return ps[i].CreatedAt > ps[j].CreatedAt })
	case "visits":
		sort.SliceStable(ps, func(i, j int) bool { return ps[i].Visits > ps[j].Visits })
	case "near":
		sort.SliceStable(ps, func(i, j int) bool {
			if ps[i].Distance == nil || ps[j].Distance == nil {
				return ps[j].Distance == nil && ps[i].Distance != nil
			}
			return *ps[i].Distance < *ps[j].Distance
		})
	}
}

func firstNon(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// GetPlace reads one of the person's places.
func (s *Store) GetPlace(ctx context.Context, uid, id int64) (*Place, error) {
	p, err := scanPlace(s.DB.QueryRowContext(ctx, `SELECT `+placeCols+` FROM places p WHERE p.user_id = ? AND p.id = ?`, Today(), uid, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &p, nil
}

// PlaceBySource finds the place another app (or an import) added.
func (s *Store) PlaceBySource(ctx context.Context, uid int64, source, ref string) (*Place, error) {
	if ref == "" {
		return nil, ErrNotFound
	}
	p, err := scanPlace(s.DB.QueryRowContext(ctx, `SELECT `+placeCols+` FROM places p WHERE p.user_id = ? AND p.source = ? AND p.source_ref = ?`, Today(), uid, source, ref))
	if err != nil {
		return nil, notFound(err)
	}
	return &p, nil
}

func (p *Place) clean() {
	p.Name = strings.TrimSpace(p.Name)
	if !ValidKind(p.Kind) {
		p.Kind = "other"
	}
	if p.Status != StatusBeen {
		p.Status = StatusWant
	}
	p.Rating = CleanRating(p.Rating)
	if p.Price < 0 || p.Price > 4 {
		p.Price = 0
	}
	p.Country = strings.ToUpper(strings.TrimSpace(p.Country))
	if len(p.Country) != 2 {
		p.Country = ""
	}
	p.City = strings.TrimSpace(p.City)
	p.Tags = JoinTags(p.Tags)
	p.Currency = strings.ToUpper(strings.TrimSpace(p.Currency))
	if p.Lat != nil && p.Lng != nil && (math.Abs(*p.Lat) > 90 || math.Abs(*p.Lng) > 180 || (*p.Lat == 0 && *p.Lng == 0)) {
		p.Lat, p.Lng = nil, nil
	}
	if p.Lat == nil || p.Lng == nil {
		p.Lat, p.Lng = nil, nil
	}
}

// CleanRating keeps a rating between 1 and 5 in halves (0 = none).
func CleanRating(r float64) float64 {
	if r <= 0 || math.IsNaN(r) {
		return 0
	}
	if r > 5 {
		r = 5
	}
	return math.Max(1, math.Round(r*2)/2)
}

// CreatePlace adds a place.
func (s *Store) CreatePlace(ctx context.Context, uid int64, p *Place) error {
	p.clean()
	now := Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO places (user_id, name, kind, cuisine, status, favourite, closed, rating, price, address, city, country, lat, lng,
		phone, website, map_url, hours, tags, notes, recommended_by, contact_id, currency, source, source_ref, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		uid, p.Name, p.Kind, p.Cuisine, p.Status, boolInt(p.Favourite), boolInt(p.Closed), p.Rating, p.Price, p.Address, p.City, p.Country, nullF(p.Lat), nullF(p.Lng),
		p.Phone, p.Website, p.MapURL, p.Hours, p.Tags, p.Notes, p.RecommendedBy, nullI(p.ContactID), p.Currency, p.Source, p.SourceRef, now, now)
	if err != nil {
		return conflict(err)
	}
	p.ID, _ = res.LastInsertId()
	p.UserID, p.CreatedAt, p.UpdatedAt = uid, now, now
	if p.Lists == nil {
		p.Lists = []int64{}
	}
	return nil
}

// UpdatePlace saves a place.
func (s *Store) UpdatePlace(ctx context.Context, uid int64, p *Place) error {
	p.clean()
	p.UpdatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `UPDATE places SET name=?, kind=?, cuisine=?, status=?, favourite=?, closed=?, rating=?, price=?, address=?, city=?, country=?, lat=?, lng=?,
		phone=?, website=?, map_url=?, hours=?, tags=?, notes=?, recommended_by=?, contact_id=?, currency=?, updated_at=? WHERE user_id = ? AND id = ?`,
		p.Name, p.Kind, p.Cuisine, p.Status, boolInt(p.Favourite), boolInt(p.Closed), p.Rating, p.Price, p.Address, p.City, p.Country, nullF(p.Lat), nullF(p.Lng),
		p.Phone, p.Website, p.MapURL, p.Hours, p.Tags, p.Notes, p.RecommendedBy, nullI(p.ContactID), p.Currency, p.UpdatedAt, uid, p.ID)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeletePlace removes a place with its visits and list entries.
func (s *Store) DeletePlace(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM places WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AfterVisit brings a place in line with its visits: been once a visit has
// happened, and the rating the average of the rated visits (when any).
func (s *Store) AfterVisit(ctx context.Context, uid, placeID int64) error {
	var n int
	var avg sql.NullFloat64
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1), AVG(CASE WHEN rating > 0 THEN rating END) FROM visits WHERE user_id = ? AND place_id = ? AND planned = 0`, uid, placeID).Scan(&n, &avg); err != nil {
		return err
	}
	if n > 0 {
		if _, err := s.DB.ExecContext(ctx, `UPDATE places SET status = 'been' WHERE user_id = ? AND id = ? AND status <> 'been'`, uid, placeID); err != nil {
			return err
		}
	}
	if avg.Valid {
		_, err := s.DB.ExecContext(ctx, `UPDATE places SET rating = ? WHERE user_id = ? AND id = ?`, CleanRating(avg.Float64), uid, placeID)
		return err
	}
	return nil
}

// CityCount is how many places a person keeps in a town.
type CityCount struct {
	City    string `json:"city"`
	Country string `json:"country"`
	Count   int    `json:"count"`
	Want    int    `json:"want"`
}

// Cities lists the towns with places, most first.
func (s *Store) Cities(ctx context.Context, uid int64) ([]CityCount, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT city, country, COUNT(1), SUM(CASE WHEN status = 'want' THEN 1 ELSE 0 END) FROM places
		WHERE user_id = ? AND city <> '' GROUP BY lower(city), country ORDER BY COUNT(1) DESC, city`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CityCount{}
	for rows.Next() {
		var c CityCount
		if err := rows.Scan(&c.City, &c.Country, &c.Count, &c.Want); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Tags lists every tag a person used, most used first.
func (s *Store) Tags(ctx context.Context, uid int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT tags FROM places WHERE user_id = ? AND tags <> ''`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	n := map[string]int{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		for _, x := range SplitTags(t) {
			n[x]++
		}
	}
	out := make([]string, 0, len(n))
	for t := range n {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if n[out[i]] != n[out[j]] {
			return n[out[i]] > n[out[j]]
		}
		return out[i] < out[j]
	})
	return out, rows.Err()
}

// ---------- visits ----------

// VisitFilter narrows a listing of visits.
type VisitFilter struct {
	PlaceID int64
	From    string
	To      string
	Planned *bool
	TripID  int64
	Q       string
	Limit   int
	Offset  int
	Oldest  bool
}

const visitCols = `v.id, v.place_id, v.date, v.time, v.planned, v.rating, v.notes, v.with_whom, v.minutes, v.spent, v.currency, v.money_tx_id, v.money_booked,
	v.split_group_id, v.split_expense_id, v.cal_event_id, v.trip_id, v.trip_name, v.source, v.source_ref, v.created_at, v.updated_at, p.name, p.kind, p.city`

func scanVisit(sc scanner) (Visit, error) {
	var v Visit
	var planned, booked int
	var tx, sg, se, cal, trip sql.NullInt64
	err := sc.Scan(&v.ID, &v.PlaceID, &v.Date, &v.Time, &planned, &v.Rating, &v.Notes, &v.WithWhom, &v.Minutes, &v.Spent, &v.Currency, &tx, &booked,
		&sg, &se, &cal, &trip, &v.TripName, &v.Source, &v.SourceRef, &v.CreatedAt, &v.UpdatedAt, &v.PlaceName, &v.PlaceKind, &v.City)
	if err != nil {
		return v, err
	}
	v.Planned, v.MoneyBooked = planned == 1, booked == 1
	v.MoneyTxID, v.SplitGroupID, v.SplitExpenseID, v.CalEventID, v.TripID = iptr(tx), iptr(sg), iptr(se), iptr(cal), iptr(trip)
	return v, nil
}

// ListVisits lists visits, newest first; the total is before the limit.
func (s *Store) ListVisits(ctx context.Context, uid int64, f VisitFilter) ([]Visit, int, error) {
	where := ` FROM visits v JOIN places p ON p.id = v.place_id WHERE v.user_id = ?`
	args := []any{uid}
	if f.PlaceID > 0 {
		where += ` AND v.place_id = ?`
		args = append(args, f.PlaceID)
	}
	if f.From != "" {
		where += ` AND v.date >= ?`
		args = append(args, f.From)
	}
	if f.To != "" {
		where += ` AND v.date <= ?`
		args = append(args, f.To)
	}
	if f.Planned != nil {
		where += ` AND v.planned = ?`
		args = append(args, boolInt(*f.Planned))
	}
	if f.TripID > 0 {
		where += ` AND v.trip_id = ?`
		args = append(args, f.TripID)
	}
	if f.Q != "" {
		where += ` AND (p.name LIKE ? OR v.notes LIKE ? OR v.with_whom LIKE ? OR p.city LIKE ?)`
		like := "%" + f.Q + "%"
		args = append(args, like, like, like, like)
	}
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1)`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := ` ORDER BY v.date DESC, v.time DESC, v.id DESC`
	if f.Oldest {
		order = ` ORDER BY v.date, v.time, v.id`
	}
	q := `SELECT ` + visitCols + where + order
	if f.Limit > 0 {
		q += ` LIMIT ? OFFSET ?`
		args = append(args, f.Limit, f.Offset)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Visit{}
	for rows.Next() {
		v, err := scanVisit(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}

// GetVisit reads one visit.
func (s *Store) GetVisit(ctx context.Context, uid, id int64) (*Visit, error) {
	v, err := scanVisit(s.DB.QueryRowContext(ctx, `SELECT `+visitCols+` FROM visits v JOIN places p ON p.id = v.place_id WHERE v.user_id = ? AND v.id = ?`, uid, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &v, nil
}

// VisitBySource finds a visit another app (or an import) added.
func (s *Store) VisitBySource(ctx context.Context, uid int64, source, ref string) (*Visit, error) {
	v, err := scanVisit(s.DB.QueryRowContext(ctx, `SELECT `+visitCols+` FROM visits v JOIN places p ON p.id = v.place_id WHERE v.user_id = ? AND v.source = ? AND v.source_ref = ?`, uid, source, ref))
	if err != nil {
		return nil, notFound(err)
	}
	return &v, nil
}

// HasVisitOn reports whether the person logged a visit at the place that day.
func (s *Store) HasVisitOn(ctx context.Context, uid, placeID int64, date string) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM visits WHERE user_id = ? AND place_id = ? AND date = ? AND planned = 0`, uid, placeID, date).Scan(&n)
	return n > 0
}

func (v *Visit) clean() {
	v.Rating = CleanRating(v.Rating)
	v.Time = strings.TrimSpace(v.Time)
	if _, err := time.Parse("15:04", v.Time); err != nil {
		v.Time = ""
	}
	if v.Spent < 0 {
		v.Spent = 0
	}
	if v.Minutes < 0 {
		v.Minutes = 0
	}
	v.Currency = strings.ToUpper(strings.TrimSpace(v.Currency))
	v.WithWhom = strings.TrimSpace(v.WithWhom)
}

// CreateVisit adds a visit (the place must be the person's own).
func (s *Store) CreateVisit(ctx context.Context, uid int64, v *Visit) error {
	v.clean()
	if _, err := s.GetPlace(ctx, uid, v.PlaceID); err != nil {
		return err
	}
	now := Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO visits (user_id, place_id, date, time, planned, rating, notes, with_whom, minutes, spent, currency, money_tx_id, money_booked,
		split_group_id, split_expense_id, cal_event_id, trip_id, trip_name, source, source_ref, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		uid, v.PlaceID, v.Date, v.Time, boolInt(v.Planned), v.Rating, v.Notes, v.WithWhom, v.Minutes, v.Spent, v.Currency, nullI(v.MoneyTxID), boolInt(v.MoneyBooked),
		nullI(v.SplitGroupID), nullI(v.SplitExpenseID), nullI(v.CalEventID), nullI(v.TripID), v.TripName, v.Source, v.SourceRef, now, now)
	if err != nil {
		return conflict(err)
	}
	v.ID, _ = res.LastInsertId()
	v.UserID, v.CreatedAt, v.UpdatedAt = uid, now, now
	if err := s.AfterVisit(ctx, uid, v.PlaceID); err != nil {
		return err
	}
	if got, err := s.GetVisit(ctx, uid, v.ID); err == nil {
		*v = *got
	}
	return nil
}

// UpdateVisit saves a visit; it may move to another of the person's places.
func (s *Store) UpdateVisit(ctx context.Context, uid int64, v *Visit, oldPlace int64) error {
	v.clean()
	if _, err := s.GetPlace(ctx, uid, v.PlaceID); err != nil {
		return err
	}
	v.UpdatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `UPDATE visits SET place_id=?, date=?, time=?, planned=?, rating=?, notes=?, with_whom=?, minutes=?, spent=?, currency=?, trip_id=?, trip_name=?, updated_at=?
		WHERE user_id = ? AND id = ?`, v.PlaceID, v.Date, v.Time, boolInt(v.Planned), v.Rating, v.Notes, v.WithWhom, v.Minutes, v.Spent, v.Currency, nullI(v.TripID), v.TripName, v.UpdatedAt, uid, v.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_ = s.AfterVisit(ctx, uid, v.PlaceID)
	if oldPlace > 0 && oldPlace != v.PlaceID {
		_ = s.AfterVisit(ctx, uid, oldPlace)
	}
	if got, err := s.GetVisit(ctx, uid, v.ID); err == nil {
		*v = *got
	}
	return nil
}

// DeleteVisit removes a visit.
func (s *Store) DeleteVisit(ctx context.Context, uid, id int64) error {
	v, err := s.GetVisit(ctx, uid, id)
	if err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM visits WHERE user_id = ? AND id = ?`, uid, id); err != nil {
		return err
	}
	return s.AfterVisit(ctx, uid, v.PlaceID)
}

// SetVisitMoney remembers the Pocket Money transaction of a visit.
func (s *Store) SetVisitMoney(ctx context.Context, uid, id int64, tx *int64, booked bool) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE visits SET money_tx_id = ?, money_booked = ? WHERE user_id = ? AND id = ?`, nullI(tx), boolInt(booked && tx != nil), uid, id)
	return err
}

// SetVisitSplit remembers the Pocket Split expense of a visit.
func (s *Store) SetVisitSplit(ctx context.Context, uid, id int64, group, expense *int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE visits SET split_group_id = ?, split_expense_id = ? WHERE user_id = ? AND id = ?`, nullI(group), nullI(expense), uid, id)
	return err
}

// SetVisitCalendar remembers the Pocket Calendar event of a planned visit.
func (s *Store) SetVisitCalendar(ctx context.Context, uid, id int64, ev *int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE visits SET cal_event_id = ? WHERE user_id = ? AND id = ?`, nullI(ev), uid, id)
	return err
}

// SpentRow is the total spent in one currency.
type SpentRow struct {
	Currency string `json:"currency"`
	Total    int64  `json:"total"`
	Visits   int    `json:"visits"`
}

// Spending sums what was spent on visits between two days, per currency.
func (s *Store) Spending(ctx context.Context, uid int64, from, to string) ([]SpentRow, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT currency, SUM(spent), COUNT(1) FROM visits WHERE user_id = ? AND planned = 0 AND date >= ? AND date <= ? AND spent > 0
		GROUP BY currency ORDER BY SUM(spent) DESC`, uid, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SpentRow{}
	for rows.Next() {
		var r SpentRow
		if err := rows.Scan(&r.Currency, &r.Total, &r.Visits); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CountVisits counts the visits that happened between two days.
func (s *Store) CountVisits(ctx context.Context, uid int64, from, to string) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM visits WHERE user_id = ? AND planned = 0 AND date >= ? AND date <= ?`, uid, from, to).Scan(&n)
	return n
}

// ---------- lists ----------

const listCols = `l.id, l.name, l.emoji, l.description, l.trip_id, l.trip_name, l.sort, l.created_at, l.updated_at,
	(SELECT COUNT(1) FROM list_places lp WHERE lp.list_id = l.id),
	(SELECT COUNT(1) FROM list_places lp JOIN places p ON p.id = lp.place_id WHERE lp.list_id = l.id AND p.status = 'been')`

func scanList(sc scanner) (List, error) {
	var l List
	var trip sql.NullInt64
	err := sc.Scan(&l.ID, &l.Name, &l.Emoji, &l.Description, &trip, &l.TripName, &l.Sort, &l.CreatedAt, &l.UpdatedAt, &l.Count, &l.Been)
	l.TripID = iptr(trip)
	return l, err
}

// ListLists lists a person's lists.
func (s *Store) ListLists(ctx context.Context, uid int64) ([]List, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+listCols+` FROM lists l WHERE l.user_id = ? ORDER BY l.sort, l.name COLLATE NOCASE`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []List{}
	for rows.Next() {
		l, err := scanList(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetList reads one list.
func (s *Store) GetList(ctx context.Context, uid, id int64) (*List, error) {
	l, err := scanList(s.DB.QueryRowContext(ctx, `SELECT `+listCols+` FROM lists l WHERE l.user_id = ? AND l.id = ?`, uid, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &l, nil
}

// CreateList adds a list.
func (s *Store) CreateList(ctx context.Context, uid int64, l *List) error {
	l.Name = strings.TrimSpace(l.Name)
	now := Now()
	res, err := s.DB.ExecContext(ctx, `INSERT INTO lists (user_id, name, emoji, description, trip_id, trip_name, sort, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		uid, l.Name, l.Emoji, l.Description, nullI(l.TripID), l.TripName, l.Sort, now, now)
	if err != nil {
		return err
	}
	l.ID, _ = res.LastInsertId()
	l.UserID, l.CreatedAt, l.UpdatedAt = uid, now, now
	return nil
}

// UpdateList saves a list.
func (s *Store) UpdateList(ctx context.Context, uid int64, l *List) error {
	l.Name = strings.TrimSpace(l.Name)
	l.UpdatedAt = Now()
	res, err := s.DB.ExecContext(ctx, `UPDATE lists SET name=?, emoji=?, description=?, trip_id=?, trip_name=?, sort=?, updated_at=? WHERE user_id = ? AND id = ?`,
		l.Name, l.Emoji, l.Description, nullI(l.TripID), l.TripName, l.Sort, l.UpdatedAt, uid, l.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteList removes a list (its places stay).
func (s *Store) DeleteList(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM lists WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AddToList puts a place on a list (both the person's own); twice is fine.
func (s *Store) AddToList(ctx context.Context, uid, listID, placeID int64, note string) error {
	if _, err := s.GetList(ctx, uid, listID); err != nil {
		return err
	}
	if _, err := s.GetPlace(ctx, uid, placeID); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO list_places (list_id, place_id, user_id, note, added_at) VALUES (?,?,?,?,?)
		ON CONFLICT(list_id, place_id) DO UPDATE SET note = CASE WHEN excluded.note <> '' THEN excluded.note ELSE list_places.note END`, listID, placeID, uid, strings.TrimSpace(note), Now())
	if err == nil {
		_, _ = s.DB.ExecContext(ctx, `UPDATE lists SET updated_at = ? WHERE id = ?`, Now(), listID)
	}
	return err
}

// RemoveFromList takes a place off a list.
func (s *Store) RemoveFromList(ctx context.Context, uid, listID, placeID int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM list_places WHERE user_id = ? AND list_id = ? AND place_id = ?`, uid, listID, placeID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListEntries are the notes kept on a list's places.
func (s *Store) ListEntries(ctx context.Context, uid, listID int64) ([]ListEntry, error) {
	q := `SELECT list_id, place_id, note, added_at FROM list_places WHERE user_id = ?`
	args := []any{uid}
	if listID > 0 {
		q += ` AND list_id = ?`
		args = append(args, listID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` ORDER BY added_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ListEntry{}
	for rows.Next() {
		var e ListEntry
		if err := rows.Scan(&e.ListID, &e.PlaceID, &e.Note, &e.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------- the inbox: "you were there — how was it?" ----------

const inboxCols = `i.id, i.place_id, i.date, i.time, i.minutes, i.movement_id, i.amount, i.currency, i.money_tx_id, i.payee, i.status, i.notified, i.created_at, p.name, p.kind, p.city`

func scanInbox(sc scanner) (InboxItem, error) {
	var it InboxItem
	var mv, tx sql.NullInt64
	var notified int
	err := sc.Scan(&it.ID, &it.PlaceID, &it.Date, &it.Time, &it.Minutes, &mv, &it.Amount, &it.Currency, &tx, &it.Payee, &it.Status, &notified, &it.CreatedAt, &it.PlaceName, &it.PlaceKind, &it.City)
	it.MovementID, it.MoneyTxID, it.Notified = iptr(mv), iptr(tx), notified == 1
	return it, err
}

// ListInbox lists inbox items in a state ("" = open), newest first.
func (s *Store) ListInbox(ctx context.Context, uid int64, status string, limit int) ([]InboxItem, error) {
	if status == "" {
		status = InboxOpen
	}
	q := `SELECT ` + inboxCols + ` FROM inbox i JOIN places p ON p.id = i.place_id WHERE i.user_id = ? AND i.status = ? ORDER BY i.date DESC, i.time DESC, i.id DESC`
	args := []any{uid, status}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InboxItem{}
	for rows.Next() {
		it, err := scanInbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetInbox reads one inbox item.
func (s *Store) GetInbox(ctx context.Context, uid, id int64) (*InboxItem, error) {
	it, err := scanInbox(s.DB.QueryRowContext(ctx, `SELECT `+inboxCols+` FROM inbox i JOIN places p ON p.id = i.place_id WHERE i.user_id = ? AND i.id = ?`, uid, id))
	if err != nil {
		return nil, notFound(err)
	}
	return &it, nil
}

// NoteInbox records a stay or a payment at a place on a day: a new item,
// or the missing half (the stay, the payment) added to an open one. Done and
// dismissed items are left alone. It reports whether a new item was made.
func (s *Store) NoteInbox(ctx context.Context, uid int64, it InboxItem) (bool, error) {
	var id int64
	var status string
	err := s.DB.QueryRowContext(ctx, `SELECT id, status FROM inbox WHERE user_id = ? AND place_id = ? AND date = ?`, uid, it.PlaceID, it.Date).Scan(&id, &status)
	if err == sql.ErrNoRows {
		now := Now()
		_, err := s.DB.ExecContext(ctx, `INSERT INTO inbox (user_id, place_id, date, time, minutes, movement_id, amount, currency, money_tx_id, payee, status, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,'open',?,?)`, uid, it.PlaceID, it.Date, it.Time, it.Minutes, nullI(it.MovementID), it.Amount, it.Currency, nullI(it.MoneyTxID), it.Payee, now, now)
		return err == nil, err
	}
	if err != nil || status != InboxOpen {
		return false, err
	}
	if it.MovementID != nil {
		_, err = s.DB.ExecContext(ctx, `UPDATE inbox SET movement_id = ?, minutes = MAX(minutes, ?), time = CASE WHEN time = '' THEN ? ELSE time END, updated_at = ? WHERE id = ? AND movement_id IS NULL`,
			*it.MovementID, it.Minutes, it.Time, Now(), id)
	}
	if err == nil && it.MoneyTxID != nil {
		_, err = s.DB.ExecContext(ctx, `UPDATE inbox SET money_tx_id = ?, amount = ?, currency = ?, payee = ?, time = CASE WHEN time = '' THEN ? ELSE time END, updated_at = ? WHERE id = ? AND money_tx_id IS NULL`,
			*it.MoneyTxID, it.Amount, it.Currency, it.Payee, it.Time, Now(), id)
	}
	return false, err
}

// SetInboxStatus marks an item done or dismissed (or open again).
func (s *Store) SetInboxStatus(ctx context.Context, uid, id int64, status string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE inbox SET status = ?, updated_at = ? WHERE user_id = ? AND id = ?`, status, Now(), uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CloseInboxFor marks the open item for a place and day done (a visit was logged by hand).
func (s *Store) CloseInboxFor(ctx context.Context, uid, placeID int64, date string) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE inbox SET status = 'done', updated_at = ? WHERE user_id = ? AND place_id = ? AND date = ? AND status = 'open'`, Now(), uid, placeID, date)
}

// MarkInboxNotified remembers that a push went out for an item.
func (s *Store) MarkInboxNotified(ctx context.Context, uid, id int64) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE inbox SET notified = 1 WHERE user_id = ? AND id = ?`, uid, id)
}

// ExpireInbox dismisses open items older than days, so the inbox never piles up.
func (s *Store) ExpireInbox(ctx context.Context, days int) {
	_, _ = s.DB.ExecContext(ctx, `UPDATE inbox SET status = 'dismissed', updated_at = ? WHERE status = 'open' AND date < ?`, Now(), AddDays(Today(), -days))
}

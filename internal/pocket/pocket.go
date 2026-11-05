// Package pocket talks to sibling Pocket apps: Pocket Movement (where you
// stayed), Pocket Money (what you paid, and booking what a visit cost),
// Pocket Calendar (planned visits as events), Pocket Contacts (who
// recommended a place, who came along), Pocket Travel (trips) and Pocket
// Split (splitting the bill). Same bearer-token JSON API everywhere.
package pocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a bearer-token JSON client for one Pocket app.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New creates a client.
func New(baseURL, token string, timeout time.Duration) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: timeout}}
}

// Configured reports whether URL and token are set.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" && c.Token != "" }

// ErrNotFound is a 404 from the sibling app.
var ErrNotFound = errors.New("not found")

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any, v any) error {
	if !c.Configured() {
		return errors.New("not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errors.New("token rejected (401)")
	}
	if res.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2000))
		_ = json.Unmarshal(b, &e)
		if e.Error != "" {
			return fmt.Errorf("HTTP %d: %s", res.StatusCode, e.Error)
		}
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if v == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Health is the connection test for any Pocket app: /healthz plus a token check.
func (c *Client) Health(ctx context.Context) (string, error) {
	var out struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if !c.Configured() {
		return "", errors.New("not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/healthz", nil)
	if err != nil {
		return "", err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	var me map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v1/auth/me", nil, nil, &me); err != nil {
		return out.Version, err
	}
	return out.Version, nil
}

// Do is a raw JSON call (for a connection test).
func (c *Client) Do(ctx context.Context, method, path string, q url.Values, body, v any) error {
	return c.do(ctx, method, path, q, body, v)
}

// list decodes a list that may be bare or wrapped in {key:[…]} / {results:[…]}.
func list[T any](raw json.RawMessage, keys ...string) ([]T, error) {
	var out []T
	if err := json.Unmarshal(raw, &out); err == nil {
		return out, nil
	}
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	for _, k := range append(keys, "results", "items") {
		if b, ok := wrap[k]; ok {
			if err := json.Unmarshal(b, &out); err != nil {
				return nil, err
			}
			return out, nil
		}
	}
	return []T{}, nil
}

// ---------------- Pocket Money ----------------

// Account is a Money account the person can see.
type Account struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Currency string `json:"currency"`
	Everyday bool   `json:"everyday"`
	Archived bool   `json:"archived"`
	Role     string `json:"role"` // owner | editor | viewer
}

// Accounts lists the accounts the person can book on.
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	var all []Account
	if err := c.do(ctx, http.MethodGet, "/api/v1/accounts", nil, nil, &all); err != nil {
		return nil, err
	}
	out := []Account{}
	for _, a := range all {
		if !a.Archived && a.Role != "viewer" {
			out = append(out, a)
		}
	}
	return out, nil
}

// Category is a Money category.
type Category struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Kind     string `json:"kind"`
	Archived bool   `json:"archived"`
}

// Categories lists the expense categories usable on an account.
func (c *Client) Categories(ctx context.Context, accountID int64) ([]Category, error) {
	var all []Category
	path := "/api/v1/categories"
	if accountID > 0 {
		path = fmt.Sprintf("/api/v1/accounts/%d/categories", accountID)
	}
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &all); err != nil {
		return nil, err
	}
	out := []Category{}
	for _, ct := range all {
		if !ct.Archived && ct.Kind != "income" {
			out = append(out, ct)
		}
	}
	return out, nil
}

// Tx is what Places books: always an expense, in the visit's own currency
// (Money converts to the account's).
type Tx struct {
	AccountID   int64
	Date        string
	Time        string
	Amount      int64 // minor units of Currency
	Currency    string
	Description string
	Payee       string
	CategoryID  int64
	Notes       string
	ExternalID  string
	Lat, Lng    *float64
}

// CreateTx books a new expense and returns its id.
func (c *Client) CreateTx(ctx context.Context, t Tx) (int64, error) {
	body := map[string]any{"kind": "expense", "account_id": t.AccountID, "date": t.Date, "amount": t.Amount, "currency": t.Currency,
		"description": t.Description, "payee": t.Payee, "tags": []string{}, "notes": t.Notes, "source": "places", "external_id": t.ExternalID}
	if t.CategoryID > 0 {
		body["category_id"] = t.CategoryID
	}
	if t.Time != "" {
		body["time"] = t.Time
	}
	if t.Lat != nil && t.Lng != nil {
		body["lat"], body["lng"] = *t.Lat, *t.Lng
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/transactions", nil, body, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// UpdateTx changes what Places owns on the transaction: date, amount,
// description and payee. Account, category and tags stay as the person left them.
func (c *Client) UpdateTx(ctx context.Context, id int64, t Tx) error {
	body := map[string]any{"date": t.Date, "amount": t.Amount, "currency": t.Currency, "description": t.Description, "payee": t.Payee}
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/transactions/%d", id), nil, body, nil)
}

// DeleteTx removes a transaction; one already gone counts as deleted.
func (c *Client) DeleteTx(ctx context.Context, id int64) error {
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/transactions/%d", id), nil, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// Payment is an expense in Pocket Money, as Places matches it to a place.
type Payment struct {
	ID              int64    `json:"id"`
	Kind            string   `json:"kind"`
	Date            string   `json:"date"`
	Time            string   `json:"time"`
	Amount          int64    `json:"amount"`   // minor units of Currency (the account's)
	Currency        string   `json:"currency"` // the account's
	ForeignAmount   int64    `json:"foreign_amount"`
	ForeignCurrency string   `json:"foreign_currency"`
	Payee           string   `json:"payee"`
	Description     string   `json:"description"`
	Lat             *float64 `json:"lat"`
	Lng             *float64 `json:"lng"`
	Source          string   `json:"source"`
}

// Paid is what was paid in the currency it was paid in.
func (p Payment) Paid() (int64, string) {
	amt, cur := p.Amount, p.Currency
	if p.ForeignAmount != 0 && p.ForeignCurrency != "" {
		amt, cur = p.ForeignAmount, p.ForeignCurrency
	}
	if amt < 0 {
		amt = -amt
	}
	return amt, cur
}

// Payments lists the expenses between two days (YYYY-MM-DD).
func (c *Client) Payments(ctx context.Context, from, to string) ([]Payment, error) {
	var raw json.RawMessage
	q := url.Values{"from": {from}, "to": {to}, "kind": {"expense"}, "limit": {"500"}}
	if err := c.do(ctx, http.MethodGet, "/api/v1/transactions", q, nil, &raw); err != nil {
		return nil, err
	}
	return list[Payment](raw, "items", "transactions")
}

// GetPayment reads one transaction.
func (c *Client) GetPayment(ctx context.Context, id int64) (*Payment, error) {
	var out Payment
	return &out, c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/transactions/%d", id), nil, nil, &out)
}

// ---------------- Pocket Calendar ----------------

// Calendar is one of the person's calendars.
type Calendar struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Writable bool   `json:"writable"`
	Default  bool   `json:"is_default"`
}

// Calendars lists the calendars the person can write to.
func (c *Client) Calendars(ctx context.Context) ([]Calendar, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/calendars", nil, nil, &raw); err != nil {
		return nil, err
	}
	all, err := list[Calendar](raw, "calendars")
	if err != nil {
		return nil, err
	}
	out := []Calendar{}
	for _, cal := range all {
		if cal.Writable {
			out = append(out, cal)
		}
	}
	return out, nil
}

// Event is a planned visit on the calendar: a timed event at the place, or
// an all-day one when no time was set.
type Event struct {
	CalendarID  int64
	Title       string
	Description string
	Location    string
	URL         string
	Date        string // YYYY-MM-DD
	Time        string // HH:MM, "" = all day
	Minutes     int
}

func (e Event) body() map[string]any {
	b := map[string]any{"title": e.Title, "description": e.Description, "location": e.Location, "url": e.URL, "categories": []string{"Places"}}
	if e.Time == "" {
		b["all_day"], b["start"], b["busy"] = true, e.Date, false
	} else {
		start, err := time.Parse("2006-01-02T15:04", e.Date+"T"+e.Time)
		if err != nil {
			b["all_day"], b["start"] = true, e.Date
		} else {
			m := e.Minutes
			if m <= 0 {
				m = 120
			}
			b["all_day"], b["start"], b["end"] = false, start.Format("2006-01-02T15:04"), start.Add(time.Duration(m)*time.Minute).Format("2006-01-02T15:04")
		}
	}
	if e.CalendarID > 0 {
		b["calendar_id"] = e.CalendarID
	}
	return b
}

// CreateEvent adds the event and returns its id.
func (c *Client) CreateEvent(ctx context.Context, e Event) (int64, error) {
	var out struct {
		EventID int64 `json:"event_id"`
		ID      any   `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/events", nil, e.body(), &out); err != nil {
		return 0, err
	}
	if out.EventID > 0 {
		return out.EventID, nil
	}
	// a bare {"id": 12} or an occurrence id like "12:20261104"
	var id int64
	_, _ = fmt.Sscan(strings.SplitN(fmt.Sprint(out.ID), ":", 2)[0], &id)
	return id, nil
}

// UpdateEvent moves or renames the event.
func (c *Client) UpdateEvent(ctx context.Context, id int64, e Event) error {
	b := e.body()
	delete(b, "calendar_id")
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/events/%d", id), nil, b, nil)
}

// DeleteEvent removes the event; one already gone counts as deleted.
func (c *Client) DeleteEvent(ctx context.Context, id int64) error {
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/events/%d", id), nil, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// ---------------- Pocket Contacts ----------------

// Contact is a card in Pocket Contacts.
type Contact struct {
	ID     int64    `json:"id"`
	FN     string   `json:"fn"`
	Org    string   `json:"org"`
	Title  string   `json:"title"`
	Emails []string `json:"emails"`
	Phones []string `json:"phones"`
	URLs   []string `json:"urls"`
	Note   string   `json:"note"`
}

// SearchContacts finds the person's contacts.
func (c *Client) SearchContacts(ctx context.Context, q string, limit int) ([]Contact, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/search", url.Values{"q": {q}, "limit": {fmt.Sprint(limit)}}, nil, &raw); err != nil {
		return nil, err
	}
	out, err := list[Contact](raw, "contacts")
	if len(out) > limit {
		out = out[:limit]
	}
	return out, err
}

// GetContact reads one contact.
func (c *Client) GetContact(ctx context.Context, id int64) (*Contact, error) {
	var out Contact
	return &out, c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/contacts/%d", id), nil, nil, &out)
}

// ---------------- Pocket Travel ----------------

// Trip is a trip in Pocket Travel.
type Trip struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Destination string   `json:"destination"`
	StartDate   string   `json:"start_date"`
	EndDate     string   `json:"end_date"`
	Status      string   `json:"status"`
	Lat         *float64 `json:"lat"`
	Lng         *float64 `json:"lng"`
	Archived    bool     `json:"archived"`
}

// Trips lists the person's trips.
func (c *Client) Trips(ctx context.Context) ([]Trip, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/trips", nil, nil, &raw); err != nil {
		return nil, err
	}
	return list[Trip](raw, "trips")
}

// ---------------- Pocket Movement ----------------

// Stay is a visit Pocket Movement noticed: somewhere you stopped for a while.
type Stay struct {
	ID       int64   `json:"id"`
	StartTS  int64   `json:"start_ts"`
	EndTS    int64   `json:"end_ts"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	Radius   float64 `json:"radius"`
	Duration int     `json:"duration"` // minutes
	Name     string  `json:"name"`
	Status   string  `json:"status"` // suggested | confirmed | declined
}

// Stays lists the visits Pocket Movement recorded between two moments.
func (c *Client) Stays(ctx context.Context, from, to time.Time) ([]Stay, error) {
	var raw json.RawMessage
	q := url.Values{"from": {fmt.Sprint(from.Unix())}, "to": {fmt.Sprint(to.Unix())}, "order": {"desc"}, "limit": {"500"}}
	if err := c.do(ctx, http.MethodGet, "/api/v1/visits", q, nil, &raw); err != nil {
		return nil, err
	}
	return list[Stay](raw, "visits")
}

// ---------------- Pocket Split ----------------

// SplitMember is someone in a Split group.
type SplitMember struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	UserID *int64 `json:"user_id"`
}

// SplitGroup is a Pocket Split group the person is in.
type SplitGroup struct {
	ID         int64         `json:"id"`
	Name       string        `json:"name"`
	Currency   string        `json:"currency"`
	Archived   bool          `json:"archived"`
	Members    []SplitMember `json:"members"`
	MyMemberID int64         `json:"my_member_id"`
}

// SplitGroups lists the person's open groups.
func (c *Client) SplitGroups(ctx context.Context) ([]SplitGroup, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/groups", nil, nil, &raw); err != nil {
		return nil, err
	}
	all, err := list[SplitGroup](raw, "groups")
	if err != nil {
		return nil, err
	}
	out := []SplitGroup{}
	for _, g := range all {
		if !g.Archived && g.MyMemberID > 0 {
			out = append(out, g)
		}
	}
	return out, nil
}

// SplitExpense is a bill paid by the person, shared evenly by members.
type SplitExpense struct {
	GroupID int64
	Title   string
	Amount  int64
	Curr    string
	Date    string
	Notes   string
	PaidBy  int64
	Members []int64
}

// CreateSplitExpense adds the bill to the group and returns its id.
func (c *Client) CreateSplitExpense(ctx context.Context, e SplitExpense) (int64, error) {
	shares := make([]map[string]any, 0, len(e.Members))
	for _, m := range e.Members {
		shares = append(shares, map[string]any{"member_id": m, "share": 1})
	}
	body := map[string]any{"title": e.Title, "amount": e.Amount, "currency": e.Curr, "paid_by": e.PaidBy, "date": e.Date, "notes": e.Notes,
		"split_mode": "even", "shares": shares}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/api/v1/groups/%d/expenses", e.GroupID), nil, body, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// DeleteSplitExpense removes a bill Places added; one already gone counts as deleted.
func (c *Client) DeleteSplitExpense(ctx context.Context, group, id int64) error {
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/groups/%d/expenses/%d", group, id), nil, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

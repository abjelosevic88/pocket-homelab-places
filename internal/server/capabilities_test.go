package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-places/internal/config"
	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

// fakeCapCore is a Pocket Core that vouches for link tokens
// pcx_<alice|bob>_<search|day|widget|capture|read|write> from Pocket Hub
// and remembers the manifest Places registers with.
type fakeCapCore struct {
	*httptest.Server
	mu       sync.Mutex
	manifest map[string]any
}

func newCapApp(t *testing.T) (*app, *fakeCapCore) {
	t.Helper()
	fc := &fakeCapCore{}
	scopes := map[string]string{"search": core.ScopeSearch, "day": core.ScopeDay, "widget": core.ScopeWidget, "capture": core.ScopeCapture,
		"read": "places:read", "write": "places:write"}
	fc.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/apps/register":
			var m map[string]any
			_ = json.NewDecoder(r.Body).Decode(&m)
			fc.mu.Lock()
			fc.manifest = m
			fc.mu.Unlock()
			io.WriteString(w, `{}`)
		case "/api/v1/auth/introspect":
			var in struct{ Token string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			parts := strings.Split(in.Token, "_")
			if len(parts) != 3 || parts[0] != "pcx" || scopes[parts[2]] == "" {
				io.WriteString(w, `{"active":false}`)
				return
			}
			id := map[string]int{"alice": 1, "bob": 2}[parts[1]]
			_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "kind": "delegated", "audience": "places", "scope": scopes[parts[2]],
				"user": map[string]any{"id": id, "email": parts[1] + "@example.com", "name": parts[1]},
				"app":  map[string]any{"id": 9, "slug": "hub", "name": "Pocket Hub"}})
		default:
			io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(fc.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL, c.CoreRegistrationKey = fc.URL, "key" }), fc
}

// items are the rows of a capability answer.
func items(t *testing.T, m map[string]any) []map[string]any {
	t.Helper()
	l, ok := m["items"].([]any)
	if !ok {
		t.Fatalf("no items list in %v", m)
	}
	out := []map[string]any{}
	for _, v := range l {
		out = append(out, v.(map[string]any))
	}
	return out
}

func titles(rows []map[string]any) string {
	var out []string
	for _, r := range rows {
		out = append(out, r["title"].(string))
	}
	return strings.Join(out, ", ")
}

func capture(a *app, tok, requestID, title string) (int, map[string]any) {
	a.t.Helper()
	code, m, _ := a.call(tok, "POST", "/api/v1/pocket/capture", map[string]any{"request_id": requestID, "kind": "place", "title": title})
	return code, m
}

func TestPocketSearch(t *testing.T) {
	a := newApp(t, nil)
	ids := a.seed(a.alice)
	a.ok(a.alice, "PUT", "/api/v1/places/"+ids["rest"], map[string]any{"cuisine": "Bosnian", "notes": "Ask for the table by the window"})
	search := func(tok, q string) []map[string]any {
		return items(t, a.ok(tok, "GET", "/api/v1/pocket/search?q="+q, nil))
	}

	got := search(a.alice, "kafeterija")
	if len(got) != 1 || got[0]["id"] != "place:"+ids["cafe"] || got[0]["kind"] != "place" || got[0]["url"] != "/places/"+ids["cafe"] ||
		got[0]["snippet"] != "Café · Sarajevo · Been there, 4.5 stars" || got[0]["date"] != store.AddDays(store.Today(), -1) {
		t.Fatalf("by name: %v", got)
	}
	// by cuisine, kind, town, tags and notes; the line says where it was found
	if got := search(a.alice, "bosnian"); titles(got) != "Kibe Mahala" || got[0]["snippet"] != "Restaurant · Bosnian · Sarajevo · Want to try" {
		t.Fatalf("by cuisine: %v", got)
	}
	if got := search(a.alice, "sarajevo"); len(got) != 2 {
		t.Fatalf("by town: %v", got)
	}
	if got := search(a.alice, "work"); titles(got) != "Kafeterija" || !strings.Contains(got[0]["snippet"].(string), "Tags: coffee, work") {
		t.Fatalf("by tag: %v", got)
	}
	if got := search(a.alice, "window"); titles(got) != "Kibe Mahala" || !strings.Contains(got[0]["snippet"].(string), "by the window") {
		t.Fatalf("by notes: %v", got)
	}
	a.ok(a.alice, "POST", "/api/v1/visits", map[string]any{"place_id": n(ids["rest"]), "date": store.AddDays(store.Today(), -9), "time": "20:00", "rating": 4,
		"with_whom": "Ana", "notes": "The lamb was perfect", "money": false})
	// lists by name, visits by their notes
	if got := search(a.alice, "date+night"); len(got) != 1 || got[0]["id"] != "list:"+ids["list"] || got[0]["url"] != "/lists/"+ids["list"] || got[0]["snippet"] != "List · 1 place" {
		t.Fatalf("list: %v", got)
	}
	got = search(a.alice, "lamb+perfect")
	if len(got) != 1 || got[0]["kind"] != "visit" || got[0]["title"] != "Dinner at Kibe Mahala" || !strings.HasPrefix(got[0]["id"].(string), "visit:") ||
		got[0]["url"] != "/places/"+ids["rest"]+"?visit="+strings.TrimPrefix(got[0]["id"].(string), "visit:") ||
		!strings.Contains(got[0]["snippet"].(string), "with Ana · 4 stars · The lamb was perfect") {
		t.Fatalf("visit by notes: %v", got)
	}
	// a place's name alone does not bring its visits along
	if got := search(a.alice, "kibe"); titles(got) != "Kibe Mahala" {
		t.Fatalf("kibe: %v", got)
	}
	// the place whose name holds the words comes before one that only mentions them
	a.ok(a.alice, "POST", "/api/v1/places", map[string]any{"name": "Vrelo", "kind": "park", "notes": "After lunch at Kibe"})
	if got := search(a.alice, "kibe"); titles(got) != "Kibe Mahala, Vrelo" {
		t.Fatalf("order: %v", got)
	}
	// words without a letter or digit find nothing; the limit holds
	for _, q := range []string{"%3F%3F", "*", "-"} {
		if got := search(a.alice, q); len(got) != 0 {
			t.Fatalf("%q finds %v", q, got)
		}
	}
	if got := items(t, a.ok(a.alice, "GET", "/api/v1/pocket/search?q=a&limit=2", nil)); len(got) != 2 {
		t.Fatalf("limit: %v", got)
	}
	// Bob finds none of it
	for _, q := range []string{"kafeterija", "date", "lamb", "sarajevo"} {
		if got := search(a.bob, q); len(got) != 0 {
			t.Fatalf("bob finds %q: %v", q, got)
		}
	}
}

func TestPocketDay(t *testing.T) {
	a := newApp(t, nil)
	ids := a.seed(a.alice)
	day := func(tok, date string) map[string]any { return a.ok(tok, "GET", "/api/v1/pocket/day/"+date, nil) }
	byKind := func(d map[string]any, kind string) []map[string]any {
		var out []map[string]any
		for _, it := range items(t, d) {
			if it["kind"] == kind {
				out = append(out, it)
			}
		}
		return out
	}

	// yesterday: coffee at Kafeterija, 4.5 stars, 9.50
	y := day(a.alice, store.AddDays(store.Today(), -1))
	v := byKind(y, "visit")
	if y["summary"] != "Coffee at Kafeterija, 4.5 stars" || len(v) != 1 || v[0]["title"] != "Coffee at Kafeterija" || v[0]["sub"] != "Sarajevo · 4.5 stars" ||
		num(v[0], "amount") != 950 || v[0]["currency"] != "EUR" || v[0]["id"] != "visit:"+ids["visit"] || v[0]["url"] != "/places/"+ids["cafe"]+"?visit="+ids["visit"] {
		t.Fatalf("yesterday: %v", y)
	}
	// the plan in three days
	p := day(a.alice, store.AddDays(store.Today(), 3))
	pl := byKind(p, "planned")
	if p["summary"] != "Planned: dinner at Kibe Mahala" || len(pl) != 1 || pl[0]["time"] != "19:30" || pl[0]["title"] != "Dinner at Kibe Mahala" || pl[0]["amount"] != nil {
		t.Fatalf("plan: %v", p)
	}
	// today: both places were saved
	td := day(a.alice, store.Today())
	if added := byKind(td, "added"); td["summary"] != "2 places saved" || len(added) != 2 || added[0]["url"] != "/places/"+ids["cafe"] ||
		!strings.HasPrefix(added[0]["id"].(string), "place:") {
		t.Fatalf("today: %v", td)
	}
	// two visits with someone: counted, spent summed, notes kept
	a.ok(a.alice, "POST", "/api/v1/visits", map[string]any{"place_id": n(ids["rest"]), "date": store.AddDays(store.Today(), -1), "time": "20:15", "rating": 5,
		"with_whom": "Ana", "notes": "Lamb", "spent": 4200, "money": false})
	y = day(a.alice, store.AddDays(store.Today(), -1))
	var dinner map[string]any
	for _, it := range byKind(y, "visit") {
		if it["title"] == "Dinner at Kibe Mahala" {
			dinner = it
		}
	}
	if y["summary"] != "2 visits" || dinner == nil || dinner["sub"] != "Sarajevo · with Ana · 5 stars" || dinner["note"] != "Lamb" || dinner["time"] != "20:15" {
		t.Fatalf("two visits: %v", y)
	}
	if fmt.Sprint(y["metrics"]) != "[map[label:Visits value:2] map[label:Spent value:51.50 EUR]]" {
		t.Fatalf("metrics: %v", y["metrics"])
	}
	// a day with nothing, and Bob's days
	if d := day(a.alice, "2020-01-01"); d["summary"] != nil || len(items(t, d)) != 0 {
		t.Fatalf("empty day: %v", d)
	}
	for _, date := range []string{store.Today(), store.AddDays(store.Today(), -1), store.AddDays(store.Today(), 3)} {
		if d := day(a.bob, date); len(items(t, d)) != 0 {
			t.Fatalf("bob's %s: %v", date, d)
		}
	}
	a.expect(400, a.alice, "GET", "/api/v1/pocket/day/2026-13-01", nil)
}

// "Saved that day" follows the caller's time zone.
func TestPocketDayTimeZone(t *testing.T) {
	a := newApp(t, nil)
	p := a.ok(a.alice, "POST", "/api/v1/places", map[string]any{"name": "Pirpa"})
	if _, err := a.st.DB.Exec(`UPDATE places SET created_at = ? WHERE id = ?`, "2026-03-10T23:30:00Z", num(p, "id")); err != nil {
		t.Fatal(err)
	}
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/2026-03-11?tz=Europe/Sarajevo", nil); d["summary"] != "1 place saved" || items(t, d)[0]["time"] != "00:30" {
		t.Fatalf("in Sarajevo: %v", d)
	}
	if d := a.ok(a.alice, "GET", "/api/v1/pocket/day/2026-03-10?tz=UTC", nil); d["summary"] != "1 place saved" {
		t.Fatalf("in UTC: %v", d)
	}
}

func TestPocketWidget(t *testing.T) {
	a := newApp(t, nil)
	ids := a.seed(a.alice)
	w := a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)
	rows := items(t, w)
	when := dayWords(store.AddDays(store.Today(), 3))
	if w["line"] != "Dinner at Kibe Mahala on "+when || w["url"] != "/" || len(rows) != 2 {
		t.Fatalf("widget: %v", w)
	}
	if rows[0]["title"] != "Dinner at Kibe Mahala" || rows[0]["detail"] != when+" 19:30" || rows[0]["id"] != "visit:"+ids["plan"] {
		t.Fatalf("plan row: %v", rows[0])
	}
	if rows[1]["title"] != "Kibe Mahala" || rows[1]["detail"] != "Restaurant · Sarajevo" || rows[1]["url"] != "/places/"+ids["rest"] {
		t.Fatalf("to try row: %v", rows[1])
	}
	if fmt.Sprint(w["metrics"]) != "[map[label:Planned value:1] map[label:To try value:1]]" {
		t.Fatalf("metrics: %v", w["metrics"])
	}
	// tomorrow, with someone
	a.ok(a.alice, "POST", "/api/v1/visits", map[string]any{"place_id": n(ids["cafe"]), "date": store.AddDays(store.Today(), 1), "with_whom": "Ana", "calendar": false})
	w = a.ok(a.alice, "GET", "/api/v1/pocket/widget", nil)
	if r := items(t, w)[0]; w["line"] != "Coffee at Kafeterija tomorrow" || r["detail"] != "Tomorrow · with Ana" {
		t.Fatalf("tomorrow: %v", w)
	}
	// no plans: the places to try; closed ones and places been to left out
	for i := range 3 {
		a.ok(a.bob, "POST", "/api/v1/places", map[string]any{"name": fmt.Sprintf("Spot %d", i), "kind": "cafe", "city": "Mostar"})
	}
	a.ok(a.bob, "POST", "/api/v1/places", map[string]any{"name": "Gone", "closed": true})
	a.ok(a.bob, "POST", "/api/v1/places", map[string]any{"name": "Old friend", "status": "been"})
	w = a.ok(a.bob, "GET", "/api/v1/pocket/widget", nil)
	if w["line"] != "3 places to try" || w["url"] != "/places?status=want" || titles(items(t, w)) != "Spot 2, Spot 1, Spot 0" {
		t.Fatalf("bob: %v", w)
	}
	// nothing to show
	a2 := newApp(t, nil)
	if w := a2.ok(a2.alice, "GET", "/api/v1/pocket/widget", nil); w["line"] != nil || len(items(t, w)) != 0 {
		t.Fatalf("empty: %v", w)
	}
}

func TestPocketCapture(t *testing.T) {
	a := newApp(t, nil)
	a.seed(a.alice)
	code, m := capture(a, a.alice, "req-0000000000000001", "  Pirpa  ")
	if code != 201 || m["title"] != "Pirpa" || m["url"] != "/places/"+m["id"].(string) {
		t.Fatalf("capture: %d %v", code, m)
	}
	p := a.ok(a.alice, "GET", "/api/v1/places/"+m["id"].(string), nil)["place"].(map[string]any)
	if p["status"] != "want" || p["kind"] != "restaurant" || p["source"] != "" || p["notes"] != "" {
		t.Fatalf("the place: %v", p)
	}
	// the same request again answers the same and adds nothing
	if code, again := capture(a, a.alice, "req-0000000000000001", "  Pirpa  "); code != 201 || again["id"] != m["id"] {
		t.Fatalf("retry: %d %v", code, again)
	}
	// the same request with other words is refused
	if code, r := capture(a, a.alice, "req-0000000000000001", "Klopa"); code != 400 || !strings.Contains(r["error"].(string), "other words") {
		t.Fatalf("other words: %d %v", code, r)
	}
	// a name she already has, however it is written
	for i, name := range []string{"pirpa", "KAFETERIJA", "Kibe  Mahala"} {
		if code, r := capture(a, a.alice, fmt.Sprintf("req-000000000000010%d", i), name); code != 400 || !strings.Contains(r["error"].(string), "already in your places") {
			t.Fatalf("%q again: %d %v", name, code, r)
		}
	}
	// the text: its first line is the name, the rest the notes
	code, m = capture(a, a.alice, "req-0000000000000002", "")
	if code != 400 {
		t.Fatalf("nothing: %d %v", code, m)
	}
	code, m, _ = a.call(a.alice, "POST", "/api/v1/pocket/capture", map[string]any{"request_id": "req-0000000000000003", "text": "Klopa\nLunch menu, ask Ana"})
	if code != 201 || m["title"] != "Klopa" {
		t.Fatalf("text: %d %v", code, m)
	}
	if p := a.ok(a.alice, "GET", "/api/v1/places/"+m["id"].(string), nil)["place"].(map[string]any); p["notes"] != "Lunch menu, ask Ana" {
		t.Fatalf("notes: %v", p)
	}
	if code, r := capture(a, a.alice, "req-0000000000000004", strings.Repeat("x", 201)); code != 400 || r["error"] != "that is too long" {
		t.Fatalf("long: %d %v", code, r)
	}
	if code, r, _ := a.call(a.alice, "POST", "/api/v1/pocket/capture", map[string]any{"request_id": "req-0000000000000005", "kind": "visit", "title": "x"}); code != 400 {
		t.Fatalf("unknown kind: %d %v", code, r)
	}
	// Bob keeps his own Pirpa, with the same request id
	if code, r := capture(a, a.bob, "req-0000000000000001", "Pirpa"); code != 201 || r["id"] == m["id"] {
		t.Fatalf("bob: %d %v", code, r)
	}
	if n := len(a.list(a.alice, "/api/v1/places")); n != 4 {
		t.Fatalf("alice has %d places", n)
	}
	if n := len(a.list(a.bob, "/api/v1/places")); n != 1 {
		t.Fatalf("bob has %d places", n)
	}
}

func TestPocketCaptureAtOnce(t *testing.T) {
	a := newApp(t, nil)
	var wg sync.WaitGroup
	ids := make([]string, 6)
	for i := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, m := capture(a, a.alice, "req-0000000000000001", "Pirpa"); code == 201 {
				ids[i] = m["id"].(string)
			}
		}()
	}
	wg.Wait()
	for _, id := range ids {
		if id == "" || id != ids[0] {
			t.Fatalf("answers: %v", ids)
		}
	}
	if n := len(a.list(a.alice, "/api/v1/places")); n != 1 {
		t.Fatalf("%d places kept", n)
	}
}

// A kept line tells the Core bus once, as adding a place does; a retry not again.
func TestPocketCaptureEvent(t *testing.T) {
	cfg := testConfig(t.TempDir())
	cfg.CoreURL = "http://core.invalid"
	st := openTestStore(t, cfg)
	s := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	ctx := context.Background()
	if err := s.core.Init(ctx); err != nil {
		t.Fatal(err)
	}
	u := &store.User{Email: "ana@example.com", Name: "Ana", Role: "admin"}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	tok, _, _ := st.CreateAPIToken(ctx, u.ID, "t")
	ts := httptest.NewServer(s.Router())
	t.Cleanup(ts.Close)
	a := &app{t: t, url: ts.URL, s: s, st: st}
	for range 2 {
		if code, m := capture(a, tok, "req-0000000000000001", "Pirpa"); code != 201 {
			t.Fatalf("capture → %d %v", code, m)
		}
	}
	var topics []string
	rows, err := st.DB.QueryContext(ctx, `SELECT body FROM pocket_outbox ORDER BY created_at, rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			t.Fatal(err)
		}
		var ev struct {
			Topic   string
			Payload map[string]any
		}
		_ = json.Unmarshal(b, &ev)
		if ev.Payload["name"] != "Pirpa" || ev.Payload["user_email"] != "ana@example.com" {
			t.Errorf("payload = %v", ev.Payload)
		}
		topics = append(topics, ev.Topic)
	}
	if strings.Join(topics, " ") != "places.place_saved" {
		t.Fatalf("topics: %v", topics)
	}
}

// A link token with one pocket:* scope reaches its one route and nothing else.
func TestPocketLinkTokens(t *testing.T) {
	a, _ := newCapApp(t)
	ids := a.seed(a.alice)
	today := store.Today()
	line := func(requestID string) map[string]any {
		return map[string]any{"request_id": requestID, "kind": "place", "title": "Pirpa"}
	}
	for _, c := range []struct {
		tok, method, path string
		body              any
		want              int
	}{
		{"pcx_alice_search", "GET", "/api/v1/pocket/search?q=kibe", nil, 200},
		{"pcx_alice_day", "GET", "/api/v1/pocket/day/" + today, nil, 200},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/widget", nil, 200},
		{"pcx_alice_capture", "POST", "/api/v1/pocket/capture", line("req-0000000000000001"), 201},
		{"pcx_alice_widget", "GET", "/api/v1/auth/me", nil, 200},
		// another capability's route
		{"pcx_alice_search", "GET", "/api/v1/pocket/day/" + today, nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_search", "POST", "/api/v1/pocket/capture", line("req-0000000000000002"), 403},
		{"pcx_alice_day", "GET", "/api/v1/pocket/search?q=kibe", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/search?q=kibe", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/pocket/day/" + today, nil, 403},
		{"pcx_alice_widget", "POST", "/api/v1/pocket/capture", line("req-0000000000000003"), 403},
		{"pcx_alice_capture", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_capture", "GET", "/api/v1/pocket/search?q=kibe", nil, 403},
		// the rest of the app
		{"pcx_alice_search", "GET", "/api/v1/search?q=kibe", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/places?q=kibe", nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/places/" + ids["rest"], nil, 403},
		{"pcx_alice_search", "GET", "/api/v1/lists", nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/day/" + today, nil, 403},
		{"pcx_alice_day", "GET", "/api/v1/visits", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/overview", nil, 403},
		{"pcx_alice_widget", "GET", "/api/v1/summary", nil, 403},
		{"pcx_alice_capture", "POST", "/api/v1/places", map[string]any{"name": "x"}, 403},
		{"pcx_alice_capture", "POST", "/api/v1/visits", map[string]any{"place_id": n(ids["rest"])}, 403},
		{"pcx_alice_capture", "PUT", "/api/v1/places/" + ids["rest"], map[string]any{"name": "x"}, 403},
		{"pcx_alice_capture", "DELETE", "/api/v1/places/" + ids["rest"], nil, 403},
		{"pcx_alice_capture", "POST", "/api/v1/lists", map[string]any{"name": "x"}, 403},
		{"pcx_alice_capture", "GET", "/api/v1/lookup?q=x", nil, 403},
		{"pcx_alice_capture", "GET", "/api/v1/settings", nil, 403},
		// the app's own scopes do not open the new routes
		{"pcx_alice_read", "GET", "/api/v1/pocket/search?q=kibe", nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/pocket/day/" + today, nil, 403},
		{"pcx_alice_read", "GET", "/api/v1/pocket/widget", nil, 403},
		{"pcx_alice_write", "POST", "/api/v1/pocket/capture", line("req-0000000000000004"), 403},
		{"pcx_alice_read", "GET", "/api/v1/places?q=kibe", nil, 200},
		{"pcx_alice_read", "GET", "/api/v1/day/" + today, nil, 200},
		{"pcx_nobody", "GET", "/api/v1/pocket/widget", nil, 401},
	} {
		if code, m, _ := a.call(c.tok, c.method, c.path, c.body); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%v)", c.tok, c.method, c.path, code, c.want, m)
		}
	}
	// the token acts for its person only
	if got := items(t, a.ok("pcx_alice_search", "GET", "/api/v1/pocket/search?q=kibe", nil)); titles(got) != "Kibe Mahala" {
		t.Fatalf("alice through the link: %v", got)
	}
	if got := items(t, a.ok("pcx_bob_search", "GET", "/api/v1/pocket/search?q=kibe", nil)); len(got) != 0 {
		t.Fatalf("bob through the link finds alice's place: %v", got)
	}
	if w := a.ok("pcx_bob_widget", "GET", "/api/v1/pocket/widget", nil); len(items(t, w)) != 0 {
		t.Fatalf("bob's widget through the link: %v", w)
	}
	if d := a.ok("pcx_bob_day", "GET", "/api/v1/pocket/day/"+today, nil); len(items(t, d)) != 0 {
		t.Fatalf("bob's day through the link: %v", d)
	}
	// the one allowed capture landed, without Hub as its source (Hub may not
	// change it later); a retry adds nothing
	code, m := capture(a, "pcx_alice_capture", "req-0000000000000001", "Pirpa")
	if code != 201 {
		t.Fatalf("retry through the link: %d %v", code, m)
	}
	if p := a.ok(a.alice, "GET", "/api/v1/places/"+m["id"].(string), nil)["place"].(map[string]any); p["name"] != "Pirpa" || p["source"] != "" {
		t.Fatalf("the captured place: %v", p)
	}
	if n := len(a.list(a.alice, "/api/v1/places")); n != 3 {
		t.Fatalf("alice has %d places", n)
	}
	if code, m := capture(a, "pcx_bob_capture", "req-0000000000000001", "Pirpa"); code != 201 || len(a.list(a.bob, "/api/v1/places")) != 1 {
		t.Fatalf("bob's line through the link: %d %v", code, m)
	}
}

func TestPocketLinkScopes(t *testing.T) {
	req := func(m, p string) *http.Request { return httptest.NewRequest(m, p, nil) }
	for _, c := range []struct {
		scope string
		r     *http.Request
		want  bool
	}{
		{core.ScopeSearch, req("GET", "/api/v1/pocket/search?q=x"), true},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/2026-01-02"), true},
		{core.ScopeWidget, req("GET", "/api/v1/pocket/widget"), true},
		{core.ScopeCapture, req("POST", "/api/v1/pocket/capture"), true},
		{core.ScopeWidget, req("GET", "/api/v1/auth/me"), true},
		{core.ScopeSearch, req("POST", "/api/v1/pocket/search"), false},
		{core.ScopeSearch, req("GET", "/api/v1/search"), false},
		{core.ScopeDay, req("GET", "/api/v1/pocket/day/today"), false},
		{core.ScopeDay, req("GET", "/api/v1/day/2026-01-02"), false},
		{core.ScopeWidget, req("GET", "/api/v1/overview"), false},
		{core.ScopeCapture, req("GET", "/api/v1/pocket/capture"), false},
		{core.ScopeCapture, req("POST", "/api/v1/places"), false},
		{"places:read", req("GET", "/api/v1/pocket/widget"), false},
		{"places:read", req("GET", "/api/v1/pocket/search"), false},
		{"places:write", req("POST", "/api/v1/pocket/capture"), false},
	} {
		if got := linkAllows([]string{c.scope}, c.r); got != c.want {
			t.Errorf("%s %s %s = %v, want %v", c.scope, c.r.Method, c.r.URL.Path, got, c.want)
		}
	}
}

// Places tells Core what it can do, so Search, Day and Hub find it on their own.
func TestManifestCapabilities(t *testing.T) {
	a, fc := newCapApp(t)
	if err := a.s.registerWithCore(context.Background()); err != nil {
		t.Fatal(err)
	}
	fc.mu.Lock()
	m := fc.manifest
	fc.mu.Unlock()
	if m["slug"] != "places" || m["category"] != "Places" {
		t.Fatalf("manifest: %v %v", m["slug"], m["category"])
	}
	search, _ := m["search"].(map[string]any)
	day, _ := m["day"].(map[string]any)
	widget, _ := m["widget"].(map[string]any)
	cap, _ := m["capture"].(map[string]any)
	if search == nil || fmt.Sprint(search["bangs"]) != "[places place eat]" || fmt.Sprint(search["kinds"]) != "[place visit list]" || day == nil || day["label"] != "Places" ||
		day["one"] != "visit" || day["many"] != "visits" || widget == nil || widget["title"] != "Places" || cap == nil {
		t.Fatalf("capabilities: %v %v %v %v", search, day, widget, cap)
	}
	if k := cap["kinds"].([]any)[0].(map[string]any); k["kind"] != "place" || fmt.Sprint(k["fields"]) != "[title]" {
		t.Fatalf("capture kinds: %v", cap)
	}
	offered := map[string]bool{}
	for _, o := range m["offers"].([]any) {
		offered[o.(map[string]any)["scope"].(string)] = true
	}
	for _, sc := range []string{"places:read", "places:write", core.ScopeMoveExport, core.ScopeMoveImport, core.ScopeSearch, core.ScopeDay, core.ScopeWidget, core.ScopeCapture} {
		if !offered[sc] {
			t.Errorf("%s is not offered", sc)
		}
	}
}

func TestDayWords(t *testing.T) {
	today := store.Today()
	d, _ := store.ParseDate(store.AddDays(today, 4))
	for in, want := range map[string]string{today: "today", store.AddDays(today, 1): "tomorrow", store.AddDays(today, -1): "yesterday",
		store.AddDays(today, 4): d.Weekday().String(), "2019-05-03": "3 May 2019"} {
		if got := dayWords(in); got != want {
			t.Errorf("dayWords(%s) = %q, want %q", in, got, want)
		}
	}
	if got := visitLine(&store.Visit{PlaceName: "Stari Most", PlaceKind: "sight"}); got != "Visited Stari Most" {
		t.Errorf("sight: %q", got)
	}
	if got := visitLine(&store.Visit{PlaceName: "Vrelo Bosne", PlaceKind: "park", Planned: true}); got != "Vrelo Bosne" {
		t.Errorf("plan: %q", got)
	}
	if got := visitLine(&store.Visit{PlaceName: "Pirpa", PlaceKind: "restaurant", Time: "12:30"}); got != "Lunch at Pirpa" {
		t.Errorf("lunch: %q", got)
	}
}

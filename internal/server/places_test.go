package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-places/internal/config"
	"github.com/abjelosevic88/pocket-homelab-places/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

// seed gives a person a café they have been to, a restaurant they want to
// try, a visit, a plan and a list.
func (a *app) seed(tok string) map[string]string {
	a.t.Helper()
	cafe := a.ok(tok, "POST", "/api/v1/places", map[string]any{"name": "Kafeterija", "kind": "cafe", "city": "Sarajevo", "country": "ba", "lat": 43.85925, "lng": 18.42838, "tags": "Coffee, work"})
	rest := a.ok(tok, "POST", "/api/v1/places", map[string]any{"name": "Kibe Mahala", "kind": "restaurant", "city": "Sarajevo", "price": 3})
	v := a.ok(tok, "POST", "/api/v1/visits", map[string]any{"place_id": num(cafe, "id"), "date": store.AddDays(store.Today(), -1), "rating": 4.5, "spent": 950, "money": false})
	plan := a.ok(tok, "POST", "/api/v1/visits", map[string]any{"place_id": num(rest, "id"), "date": store.AddDays(store.Today(), 3), "time": "19:30", "calendar": false})
	l := a.ok(tok, "POST", "/api/v1/lists", map[string]any{"name": "Date night", "emoji": "🕯️", "place_ids": []float64{num(rest, "id")}})
	return map[string]string{"cafe": id(cafe), "rest": id(rest), "visit": id(v["visit"].(map[string]any)), "plan": id(plan["visit"].(map[string]any)), "list": id(l)}
}

func TestPerPersonIsolation(t *testing.T) {
	a := newApp(t, nil)
	ids := a.seed(a.alice)
	uid := a.ids["Alice"]
	if _, err := a.st.NoteInbox(context.Background(), uid, store.InboxItem{PlaceID: n(ids["rest"]), Date: store.Today(), Minutes: 60}); err != nil {
		t.Fatal(err)
	}
	inbox := a.ok(a.alice, "GET", "/api/v1/inbox", nil)["items"].([]any)
	if len(inbox) != 1 {
		t.Fatalf("alice's inbox: %v", inbox)
	}
	inboxID := id(inbox[0].(map[string]any))
	// Bob sees none of it and can touch none of it
	if l := a.list(a.bob, "/api/v1/places"); len(l) != 0 {
		t.Fatalf("bob sees alice's places: %v", l)
	}
	if l := a.list(a.bob, "/api/v1/lists"); len(l) != 0 {
		t.Fatalf("bob sees alice's lists: %v", l)
	}
	if v := a.ok(a.bob, "GET", "/api/v1/visits", nil)["items"].([]any); len(v) != 0 {
		t.Fatalf("bob sees alice's visits: %v", v)
	}
	if v := a.ok(a.bob, "GET", "/api/v1/inbox", nil)["items"].([]any); len(v) != 0 {
		t.Fatalf("bob sees alice's inbox: %v", v)
	}
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/v1/places/" + ids["cafe"]}, {"PUT", "/api/v1/places/" + ids["cafe"]}, {"DELETE", "/api/v1/places/" + ids["cafe"]},
		{"GET", "/api/v1/visits/" + ids["visit"]}, {"PUT", "/api/v1/visits/" + ids["visit"]}, {"DELETE", "/api/v1/visits/" + ids["visit"]},
		{"GET", "/api/v1/lists/" + ids["list"]}, {"PUT", "/api/v1/lists/" + ids["list"]}, {"DELETE", "/api/v1/lists/" + ids["list"]},
		{"POST", "/api/v1/inbox/" + inboxID + "/accept"}, {"POST", "/api/v1/inbox/" + inboxID + "/dismiss"},
	} {
		a.expect(404, a.bob, c.method, c.path, map[string]any{"name": "x"})
	}
	// nor use Alice's place in his own visits and lists
	a.expect(404, a.bob, "POST", "/api/v1/visits", map[string]any{"place_id": n(ids["cafe"]), "date": store.Today()})
	mine := a.ok(a.bob, "POST", "/api/v1/lists", map[string]any{"name": "Mine"})
	a.expect(404, a.bob, "POST", "/api/v1/lists/"+id(mine)+"/places", map[string]any{"place_id": n(ids["cafe"])})
	a.expect(404, a.bob, "POST", "/api/v1/places/"+ids["cafe"]+"/lists", map[string]any{"list_id": num(mine, "id")})
	// and Alice's things are still there
	a.ok(a.alice, "GET", "/api/v1/places/"+ids["cafe"], nil)
	if d := a.ok(a.bob, "GET", "/api/v1/day/"+store.AddDays(store.Today(), -1), nil); len(d["visits"].([]any)) != 0 {
		t.Fatalf("bob's day has alice's visit: %v", d)
	}
}

func TestVisitsMakeBeenAndRating(t *testing.T) {
	a := newApp(t, nil)
	p := a.ok(a.alice, "POST", "/api/v1/places", map[string]any{"name": "Dveri", "kind": "restaurant"})
	pid := num(p, "id")
	if p["status"] != "want" || num(p, "rating") != 0 {
		t.Fatalf("new place: %v", p)
	}
	// a plan changes nothing
	a.ok(a.alice, "POST", "/api/v1/visits", map[string]any{"place_id": pid, "date": store.AddDays(store.Today(), 5), "calendar": false})
	if got := a.ok(a.alice, "GET", "/api/v1/places/"+id(p), nil)["place"].(map[string]any); got["status"] != "want" {
		t.Fatalf("a plan made it been: %v", got)
	}
	a.ok(a.alice, "POST", "/api/v1/visits", map[string]any{"place_id": pid, "date": store.AddDays(store.Today(), -20), "rating": 4})
	a.ok(a.alice, "POST", "/api/v1/visits", map[string]any{"place_id": pid, "date": store.AddDays(store.Today(), -2), "rating": 5, "spent": 4200})
	got := a.ok(a.alice, "GET", "/api/v1/places/"+id(p), nil)
	pl := got["place"].(map[string]any)
	if pl["status"] != "been" || num(pl, "rating") != 4.5 || num(pl, "visits") != 2 || pl["next_plan"] == "" {
		t.Fatalf("after two visits: %v", pl)
	}
	if spent := got["spent"].([]any); len(spent) != 1 || num(spent[0].(map[string]any), "total") != 4200 {
		t.Fatalf("spent: %v", spent)
	}
	// rating a place by hand says you have been
	q := a.ok(a.alice, "POST", "/api/v1/places", map[string]any{"name": "Klopa", "rating": 4})
	if q["status"] != "been" {
		t.Fatalf("rated but still want: %v", q)
	}
	a.expect(400, a.alice, "POST", "/api/v1/places", map[string]any{"name": "", "kind": "cafe"})
	a.expect(400, a.alice, "POST", "/api/v1/places", map[string]any{"name": "X", "kind": "spaceport"})
	a.expect(400, a.alice, "POST", "/api/v1/visits", map[string]any{"place_id": pid, "date": "yesterday"})
}

func TestFiltersAndNearby(t *testing.T) {
	a := newApp(t, nil)
	a.seed(a.alice)
	a.ok(a.alice, "POST", "/api/v1/places", map[string]any{"name": "Ćevabdžinica Željo", "kind": "restaurant", "city": "Sarajevo", "lat": 43.85972, "lng": 18.43069, "favourite": true})
	if l := a.list(a.alice, "/api/v1/places?q=cevabdzinica"); len(l) != 1 {
		t.Fatalf("search without the accents: %v", l)
	}
	if l := a.list(a.alice, "/api/v1/places?status=want"); len(l) != 2 {
		t.Fatalf("want: %d", len(l))
	}
	if l := a.list(a.alice, "/api/v1/places?group=eat&favourite=1"); len(l) != 1 {
		t.Fatalf("favourites: %d", len(l))
	}
	if l := a.list(a.alice, "/api/v1/places?tag=coffee"); len(l) != 1 {
		t.Fatalf("tag: %d", len(l))
	}
	near := a.list(a.alice, "/api/v1/places?near=43.8593,18.4290&radius_km=1&sort=near")
	if len(near) != 2 || near[0].(map[string]any)["name"] != "Kafeterija" || num(near[0].(map[string]any), "distance_m") > 100 {
		t.Fatalf("near: %v", near)
	}
}

func TestLinkScopes(t *testing.T) {
	req := func(m, p string) *http.Request { return httptest.NewRequest(m, p, nil) }
	read, write := []string{"places:read"}, []string{"places:write"}
	for _, c := range []struct {
		scopes []string
		r      *http.Request
		want   bool
	}{
		{nil, req("GET", "/api/v1/auth/me"), true},
		{read, req("GET", "/api/v1/overview"), true},
		{read, req("GET", "/api/v1/places"), true},
		{read, req("GET", "/api/v1/places/3"), true},
		{read, req("GET", "/api/v1/visits"), true},
		{read, req("GET", "/api/v1/lists/2"), true},
		{read, req("GET", "/api/v1/trip-guide"), true},
		{read, req("GET", "/api/v1/day/2026-01-02"), true},
		{read, req("POST", "/api/v1/places"), false},
		{write, req("GET", "/api/v1/places"), false},
		{write, req("POST", "/api/v1/places"), true},
		{write, req("PUT", "/api/v1/places/4"), true},
		{write, req("POST", "/api/v1/visits"), true},
		{write, req("DELETE", "/api/v1/visits/4"), true},
		{write, req("POST", "/api/v1/lists"), false},
		{write, req("POST", "/api/v1/visits/4/money"), false},
		{append(read, write...), req("GET", "/api/v1/settings"), false},
		{append(read, write...), req("POST", "/api/v1/tokens"), false},
		{append(read, write...), req("GET", "/api/v1/links/money/accounts"), false},
		{append(read, write...), req("POST", "/api/v1/import"), false},
		{append(read, write...), req("GET", "/api/v1/lookup"), false},
	} {
		if got := linkAllows(c.scopes, c.r); got != c.want {
			t.Errorf("%v %s %s = %v, want %v", c.scopes, c.r.Method, c.r.URL.Path, got, c.want)
		}
	}
}

func TestExportImportIdempotent(t *testing.T) {
	a := newApp(t, nil)
	a.seed(a.alice)
	code, doc, _ := a.call(a.alice, "GET", "/api/v1/export.json", nil)
	if code != 200 || doc["app"] != "pocket-places" {
		t.Fatalf("export: %d %v", code, doc["app"])
	}
	first := a.ok(a.bob, "POST", "/api/v1/import", doc)
	if num(first, "places") != 2 || num(first, "visits") != 2 || num(first, "lists") != 1 {
		t.Fatalf("first import: %v", first)
	}
	second := a.ok(a.bob, "POST", "/api/v1/import", doc)
	for k, v := range second {
		if v.(float64) != 0 {
			t.Fatalf("second import added %v %s", v, k)
		}
	}
	lists := a.list(a.bob, "/api/v1/lists")
	if len(lists) != 1 || num(lists[0].(map[string]any), "count") != 1 {
		t.Fatalf("bob's imported list: %v", lists)
	}
	a.expect(400, a.bob, "POST", "/api/v1/import", map[string]any{"app": "pocket-home"})
}

func (a *app) post(auth, path, ctype string, body []byte) (int, map[string]any) {
	a.t.Helper()
	req, _ := http.NewRequest("POST", a.url+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("Authorization", "Bearer "+auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	raw, _ := io.ReadAll(res.Body)
	_ = json.Unmarshal(raw, &m)
	return res.StatusCode, m
}

func TestGoogleTakeoutImport(t *testing.T) {
	a := newApp(t, nil)
	saved := `{"type":"FeatureCollection","features":[
	 {"type":"Feature","geometry":{"type":"Point","coordinates":[18.4307,43.8597]},"properties":{"date":"2024-03-01T10:00:00Z","google_maps_url":"http://maps.google.com/?cid=111","location":{"name":"Ćevabdžinica Željo","address":"Kundurdžiluk 19, Sarajevo 71000, Bosnia and Herzegovina","country_code":"BA"}}},
	 {"type":"Feature","geometry":{"type":"Point","coordinates":[16.3654,48.2104]},"properties":{"Title":"Café Central","Google Maps URL":"http://maps.google.com/?cid=222","Location":{"Business Name":"Café Central","Address":"Herrengasse 14, 1010 Wien, Austria","Country Code":"AT"}}}]}`
	code, r := a.post(a.alice, "/api/v1/import", "application/json", []byte(saved))
	if code != 200 || num(r, "places") != 2 {
		t.Fatalf("saved places: %d %v", code, r)
	}
	ps := a.list(a.alice, "/api/v1/places?sort=name")
	if ps[0].(map[string]any)["city"] != "Wien" || ps[1].(map[string]any)["city"] != "Sarajevo" || ps[1].(map[string]any)["country"] != "BA" {
		t.Fatalf("towns from the address: %v", ps)
	}
	// a review of the same place rates it and logs the visit; nothing doubles
	reviews := `{"type":"FeatureCollection","features":[{"type":"Feature","geometry":{"type":"Point","coordinates":[18.4307,43.8597]},
	 "properties":{"date":"2024-05-10T19:00:00Z","five_star_rating_published":5,"review_text_published":"Best ćevapi.","google_maps_url":"http://maps.google.com/?cid=111","location":{"name":"Ćevabdžinica Željo"}}}]}`
	_, r = a.post(a.alice, "/api/v1/import", "application/json", []byte(reviews))
	if num(r, "places") != 0 || num(r, "already there") != 1 {
		t.Fatalf("review import: %v", r)
	}
	if l := a.list(a.alice, "/api/v1/places"); len(l) != 2 {
		t.Fatalf("places after the review: %d", len(l))
	}
	// a saved list: Google's CSV becomes a list named after the file
	csv := "Title,Note,URL,Tags,Comment\n,,,,\nKibe Mahala,Window table,\"https://www.google.com/maps/place/Kibe+Mahala/@43.8629,18.4422,17z/data=!3d43.86292!4d18.44221\",,\nFigl,,https://www.google.com/maps/search/Figlm%C3%BCller,,\n"
	code, r = a.post(a.alice, "/api/v1/import/csv?list=Want%20to%20go", "text/csv", []byte(csv))
	if code != 200 || num(r, "places") != 2 {
		t.Fatalf("csv list: %d %v", code, r)
	}
	_, r = a.post(a.alice, "/api/v1/import/csv?list=Want%20to%20go", "text/csv", []byte(csv))
	if num(r, "places") != 0 {
		t.Fatalf("second csv import added places: %v", r)
	}
	lists := a.list(a.alice, "/api/v1/lists")
	if len(lists) != 1 || lists[0].(map[string]any)["name"] != "Want to go" || num(lists[0].(map[string]any), "count") != 2 {
		t.Fatalf("the list: %v", lists)
	}
	kibe := a.list(a.alice, "/api/v1/places?q=kibe")[0].(map[string]any)
	if num(kibe, "lat") != 43.86292 || kibe["notes"] != "Window table" {
		t.Fatalf("kibe from the link: %v", kibe)
	}
}

func TestCSVRoundTrip(t *testing.T) {
	a := newApp(t, nil)
	a.seed(a.alice)
	req, _ := http.NewRequest("GET", a.url+"/api/v1/export.csv", nil)
	req.Header.Set("Authorization", "Bearer "+a.alice)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if !strings.HasPrefix(string(body), "name,kind,status") {
		t.Fatalf("csv: %s", body)
	}
	code, r := a.post(a.bob, "/api/v1/import/csv", "text/csv", body)
	if code != 200 || num(r, "places") != 2 {
		t.Fatalf("csv import: %d %v", code, r)
	}
	cafe := a.list(a.bob, "/api/v1/places?q=kafeterija")[0].(map[string]any)
	if cafe["kind"] != "cafe" || cafe["status"] != "been" || num(cafe, "rating") != 4.5 || cafe["tags"] != "coffee, work" {
		t.Fatalf("round trip: %v", cafe)
	}
	code, _ = a.post(a.bob, "/api/v1/import/csv", "text/csv", []byte("foo,bar\n1,2\n"))
	if code != 400 {
		t.Fatalf("a csv without names: %d", code)
	}
}

func TestNameMatching(t *testing.T) {
	for _, c := range []struct {
		place, other string
		want         bool
	}{
		{"Kafeterija", "KAFETERIJA SARAJEVO", true},
		{"Café Tito", "CAFFE TITO 71000", true},
		{"Ćevabdžinica Željo", "CEVABDZINICA ZELJO 2", true},
		{"Café Tito", "Titova pekara", false},
		{"Bar", "BAR BAR", false}, // nothing distinctive
		{"Zlatna Ribica", "Zlatna", false},
		{"Ministry of Ćejf", "MINISTRY OF CEJF D.O.O.", true},
		{"Klopa", "KLOPA DOO", true},
	} {
		if got := store.NameMatches(c.place, c.other); got != c.want {
			t.Errorf("NameMatches(%q, %q) = %v", c.place, c.other, got)
		}
	}
	lat, lng := 43.85925, 18.42838
	p := store.Place{Name: "Kafeterija", Lat: &lat, Lng: &lng}
	near, far := 43.8594, 18.4284
	if ok, _ := paymentAt(p, pocket.Payment{Payee: "Konzum", Lat: &near, Lng: &far}, 100); !ok {
		t.Error("a payment at the spot")
	}
	if ok, _ := paymentAt(p, pocket.Payment{Payee: "Konzum"}, 100); ok {
		t.Error("a payment elsewhere under another name")
	}
	vienna1, vienna2 := 48.2104, 16.3654
	if ok, _ := paymentAt(p, pocket.Payment{Payee: "KAFETERIJA", Lat: &vienna1, Lng: &vienna2}, 100); ok {
		t.Error("the same name in another town")
	}
}

func TestParseMapLink(t *testing.T) {
	for _, c := range []struct {
		in       string
		name     string
		lat, lng float64
	}{
		{"https://www.google.com/maps/place/Kibe+Mahala/@43.86,18.44,17z/data=!3m1!4b1!4m6!3m5!1s0x0:0x0!8m2!3d43.86292!4d18.44221", "Kibe Mahala", 43.86292, 18.44221},
		{"Look: https://www.google.com/maps/place/Caf%C3%A9+Tito/@43.8536,18.4008,18z", "Café Tito", 43.8536, 18.4008},
		{"https://maps.google.com/?q=43.8597,18.4307", "", 43.8597, 18.4307},
		{"https://maps.apple.com/?q=Kafeterija&ll=43.85925,18.42838", "Kafeterija", 43.85925, 18.42838},
		{"https://www.openstreetmap.org/#map=19/43.85972/18.43069", "", 43.85972, 18.43069},
		{"43.85972, 18.43069", "", 43.85972, 18.43069},
		{"https://www.google.com/maps/search/Kafeterija/@43.85925,18.42838,17z", "Kafeterija", 43.85925, 18.42838},
		{"https://maps.apple.com/place?address=Sarajevo&coordinate=43.85925,18.42838&name=Kafeterija", "Kafeterija", 43.85925, 18.42838},
	} {
		f := parseMapLink(c.in)
		if f.Name != c.name || f.Lat == nil || *f.Lat != c.lat || *f.Lng != c.lng {
			t.Errorf("parseMapLink(%q) = %q %v %v", c.in, f.Name, f.Lat, f.Lng)
		}
	}
	// a share.google link ends on a Google search for the place: only the name
	if f := parseMapLink("https://www.google.com/search?kgmid=/g/1yghbv_df&q=San+Remo&shndl=30"); f.Name != "San Remo" || f.Lat != nil {
		t.Errorf("google search: %q %v", f.Name, f.Lat)
	}
	if !isGoogle("share.google") || !isGoogle("www.google.ba") || !isGoogle("maps.app.goo.gl") || isGoogle("example.com") {
		t.Error("isGoogle")
	}
	if f := parseMapLink("not a link"); f.Lat != nil || f.Name != "" {
		t.Errorf("text: %v", f)
	}
}

func TestInboxMergeAcceptDismiss(t *testing.T) {
	a := newApp(t, nil)
	ids := a.seed(a.alice)
	ctx := context.Background()
	uid := a.ids["Alice"]
	day := store.AddDays(store.Today(), -2)
	mv, tx := int64(7), int64(9)
	if made, _ := a.st.NoteInbox(ctx, uid, store.InboxItem{PlaceID: n(ids["rest"]), Date: day, Time: "19:40", Minutes: 95, MovementID: &mv}); !made {
		t.Fatal("first stay not noted")
	}
	if made, _ := a.st.NoteInbox(ctx, uid, store.InboxItem{PlaceID: n(ids["rest"]), Date: day, Time: "21:20", Amount: 8800, Currency: "BAM", MoneyTxID: &tx, Payee: "KIBE"}); made {
		t.Fatal("the payment the same day should join the stay")
	}
	items := a.ok(a.alice, "GET", "/api/v1/inbox", nil)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("inbox: %v", items)
	}
	it := items[0].(map[string]any)
	if num(it, "minutes") != 95 || num(it, "amount") != 8800 || it["time"] != "19:40" {
		t.Fatalf("merged: %v", it)
	}
	v := a.ok(a.alice, "POST", "/api/v1/inbox/"+id(it)+"/accept", map[string]any{"rating": 5, "with_whom": "Ana"})["visit"].(map[string]any)
	if num(v, "minutes") != 95 || num(v, "spent") != 8800 || num(v, "money_tx_id") != 9 || v["money_booked"] != false || v["currency"] != "BAM" {
		t.Fatalf("visit from the inbox: %v", v)
	}
	if l := a.ok(a.alice, "GET", "/api/v1/inbox", nil)["items"].([]any); len(l) != 0 {
		t.Fatalf("still open: %v", l)
	}
	// a dismissed one never comes back
	other := store.AddDays(store.Today(), -4)
	_, _ = a.st.NoteInbox(ctx, uid, store.InboxItem{PlaceID: n(ids["cafe"]), Date: other, Minutes: 30, MovementID: &mv})
	items = a.ok(a.alice, "GET", "/api/v1/inbox", nil)["items"].([]any)
	a.expect(204, a.alice, "POST", "/api/v1/inbox/"+id(items[0].(map[string]any))+"/dismiss", nil)
	if made, _ := a.st.NoteInbox(ctx, uid, store.InboxItem{PlaceID: n(ids["cafe"]), Date: other, Minutes: 30, MovementID: &mv}); made {
		t.Fatal("a dismissed visit came back")
	}
	// past plans ask "did you go?"
	a.ok(a.alice, "POST", "/api/v1/visits", map[string]any{"place_id": n(ids["cafe"]), "date": store.AddDays(store.Today(), -1), "planned": true})
	if pp := a.ok(a.alice, "GET", "/api/v1/inbox", nil)["past_plans"].([]any); len(pp) != 1 {
		t.Fatalf("past plans: %v", pp)
	}
}

func TestPlacesForTrip(t *testing.T) {
	lat, lng := 38.7071, -9.1458
	far1, far2 := 43.8597, 18.4307
	ps := []store.Place{{ID: 1, Name: "Time Out", Lat: &lat, Lng: &lng}, {ID: 2, Name: "Ramiro", City: "Lisbon"}, {ID: 3, Name: "Željo", Lat: &far1, Lng: &far2, City: "Sarajevo"}}
	tl, tg := 38.7223, -9.1393
	got := placesForTrip(ps, &tl, &tg, "Lisbon, Portugal")
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 {
		t.Fatalf("trip places: %v", got)
	}
}

func TestSettingsPerPerson(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"currency": "BAM", "notify_to_rate": false, "distance_unit": "mi", "travel_mode": "off", "match_minutes": 30})
	al := a.ok(a.alice, "GET", "/api/v1/settings", nil)
	bo := a.ok(a.bob, "GET", "/api/v1/settings", nil)
	if al["currency"] != "BAM" || al["notify_to_rate"] != false || al["distance_unit"] != "mi" || al["travel_mode"] != "off" || num(al, "match_minutes") != 30 {
		t.Fatalf("alice: %v", al)
	}
	if bo["currency"] != "EUR" || bo["notify_to_rate"] != true || bo["distance_unit"] != "km" || bo["travel_mode"] == "off" {
		t.Fatalf("bob got alice's settings: %v", bo)
	}
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"match_meters": 5})
	// a member can't rename the box
	a.ok(a.bob, "PUT", "/api/v1/settings", map[string]any{"title": "Bob's"})
	if a.ok(a.alice, "GET", "/api/v1/settings", nil)["title"] != "Pocket Places" {
		t.Fatal("member renamed the box")
	}
}

func TestDemoSeeds(t *testing.T) {
	a := newApp(t, func(c *config.Config) {
		c.DemoData = true
		c.AuthMode = "local"
		c.AdminEmail = "demo@example.com"
		c.AdminPassword = "demo-password"
	})
	ctx := context.Background()
	owner := a.s.store.OwnerID(ctx)
	ps, err := a.s.store.ListPlaces(ctx, owner, store.PlaceFilter{})
	if err != nil || len(ps) != len(demoPlaces) {
		t.Fatalf("demo places: %d %v", len(ps), err)
	}
	_, total, _ := a.s.store.ListVisits(ctx, owner, store.VisitFilter{Limit: 1})
	if total < 80 {
		t.Fatalf("demo visits: %d", total)
	}
	items, _ := a.s.store.ListInbox(ctx, owner, store.InboxOpen, 0)
	if len(items) != 2 || items[0].Minutes == 0 || items[0].Amount == 0 {
		t.Fatalf("demo inbox: %+v", items)
	}
	ls, _ := a.s.store.ListLists(ctx, owner)
	if len(ls) != 5 {
		t.Fatalf("demo lists: %d", len(ls))
	}
}

// Saving a place queues places.place_saved on the Core outbox, with the
// owner's email.
func TestPlaceSavedEvent(t *testing.T) {
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
	p := a.ok(tok, "POST", "/api/v1/places", map[string]any{"name": "Kafeterija", "kind": "cafe", "lat": 43.85925, "lng": 18.42838})
	a.ok(tok, "POST", "/api/v1/visits", map[string]any{"place_id": num(p, "id"), "date": store.Today(), "rating": 5})
	rows, err := st.DB.QueryContext(ctx, `SELECT body FROM pocket_outbox ORDER BY created_at, rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var topics []string
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			t.Fatal(err)
		}
		var ev struct {
			Topic   string         `json:"topic"`
			Payload map[string]any `json:"payload"`
		}
		_ = json.Unmarshal([]byte(body), &ev)
		if ev.Payload["user_email"] != "ana@example.com" {
			t.Fatalf("event without the email: %s", body)
		}
		topics = append(topics, ev.Topic)
	}
	if strings.Join(topics, " ") != "places.place_saved places.visit_logged" {
		t.Fatalf("topics: %v", topics)
	}
}

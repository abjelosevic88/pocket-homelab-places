package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-places/internal/config"
)

// newMoveApp is newApp with a Pocket Core that vouches for Pocket Move's
// link tokens: pcx_<alice|bob>_<in|out> (move:import / move:export).
func newMoveApp(t *testing.T) *app {
	t.Helper()
	coreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/auth/introspect" {
			http.NotFound(w, r)
			return
		}
		var in struct{ Token string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		parts := strings.Split(in.Token, "_")
		if len(parts) != 3 || parts[0] != "pcx" {
			_, _ = w.Write([]byte(`{"active":false}`))
			return
		}
		id := map[string]int{"alice": 1, "bob": 2}[parts[1]]
		_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "kind": "delegated", "audience": "places",
			"scope": map[string]string{"in": "move:import", "out": "move:export"}[parts[2]],
			"user":  map[string]any{"id": id, "email": parts[1] + "@example.com", "name": parts[1]},
			"app":   map[string]any{"id": 9, "slug": "move", "name": "Pocket Move"}})
	}))
	t.Cleanup(coreSrv.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL = coreSrv.URL })
}

// upload posts a file as the multipart field "file", with form fields.
func (a *app) upload(auth, path, filename string, body []byte, fields map[string]string) (int, map[string]any) {
	a.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filename)
	_, _ = fw.Write(body)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()
	return a.post(auth, path, mw.FormDataContentType(), buf.Bytes())
}

func (a *app) status(auth, method, path string) int {
	a.t.Helper()
	req, _ := http.NewRequest(method, a.url+path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return res.StatusCode
}

const savedPlaces = `{"type":"FeatureCollection","features":[
 {"type":"Feature","geometry":{"type":"Point","coordinates":[18.4307,43.8597]},"properties":{"date":"2024-03-01T10:00:00Z","google_maps_url":"http://maps.google.com/?cid=111","location":{"name":"Ćevabdžinica Željo","address":"Kundurdžiluk 19, Sarajevo 71000, Bosnia and Herzegovina","country_code":"BA"}}},
 {"type":"Feature","geometry":{"type":"Point","coordinates":[16.3654,48.2104]},"properties":{"Title":"Café Central","Google Maps URL":"http://maps.google.com/?cid=222","Published":"2023-01-01T00:00:00Z","Location":{"Business Name":"Café Central","Address":"Herrengasse 14, 1010 Wien, Austria","Country Code":"AT"}}},
 {"type":"Feature","geometry":{"type":"Point","coordinates":[2.2945,48.8584]},"properties":{"Title":"Tour Eiffel","Location":{"Address":"Champ de Mars, 75007 Paris, France"}}}]}`

func TestMoveLinksAndImports(t *testing.T) {
	a := newMoveApp(t)
	a.seed(a.alice)
	aliceOut, aliceIn, bobIn, bobOut := "pcx_alice_out", "pcx_alice_in", "pcx_bob_in", "pcx_bob_out"

	// move:export: the description and the exports, nothing else
	code, desc, _ := a.call(aliceOut, "GET", core.PortablePath, nil)
	if code != 200 || len(desc["exports"].([]any)) != 2 || len(desc["imports"].([]any)) != 3 {
		t.Fatalf("portable %d %v", code, desc)
	}
	for _, p := range []string{"/api/v1/export.json", "/api/v1/export.csv"} {
		if c := a.status(aliceOut, "GET", p); c != 200 {
			t.Fatalf("export %s = %d", p, c)
		}
	}
	for _, c := range []struct{ m, p string }{{"POST", "/api/v1/import"}, {"POST", "/api/v1/import/csv"}, {"GET", "/api/v1/places"}, {"GET", "/api/v1/settings"}, {"POST", "/api/v1/places"}, {"GET", "/api/v1/backup"}} {
		if got := a.status(aliceOut, c.m, c.p); got != 403 {
			t.Errorf("move:export %s %s = %d, want 403", c.m, c.p, got)
		}
	}
	// move:import: the description and the imports, nothing else
	if c := a.status(aliceIn, "GET", core.PortablePath); c != 200 {
		t.Fatalf("move:import reads the description: %d", c)
	}
	for _, c := range []struct{ m, p string }{{"GET", "/api/v1/export.json"}, {"GET", "/api/v1/export.csv"}, {"GET", "/api/v1/places"}, {"PUT", "/api/v1/settings"}, {"POST", "/api/v1/places"}, {"POST", "/api/v1/lists"}} {
		if got := a.status(aliceIn, c.m, c.p); got != 403 {
			t.Errorf("move:import %s %s = %d, want 403", c.m, c.p, got)
		}
	}

	// Alice's own export into Bob's account through Move, twice
	req, _ := http.NewRequest("GET", a.url+"/api/v1/export.json", nil)
	req.Header.Set("Authorization", "Bearer "+aliceOut)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	dump, _ := io.ReadAll(res.Body)
	res.Body.Close()
	for run, want := range []float64{2, 0} {
		code, r := a.upload(bobIn, "/api/v1/import", "places.json", dump, nil)
		if code != 200 || num(r, "places") != want {
			t.Fatalf("pocket run %d: %d %v", run+1, code, r)
		}
	}
	// and Alice's own file back into her own account adds nothing
	if code, r := a.upload(aliceIn, "/api/v1/import", "places.json", dump, nil); code != 200 || num(r, "places") != 0 || num(r, "visits") != 0 || num(r, "lists") != 0 {
		t.Fatalf("alice's own file: %d %v", code, r)
	}
	if n := len(a.list(a.alice, "/api/v1/places")); n != 2 {
		t.Fatalf("alice has %d places", n)
	}
	if n := len(a.list(a.alice, "/api/v1/lists")); n != 1 {
		t.Fatalf("alice has %d lists", n)
	}

	// Google Maps saved places: into "Google Maps", twice; then into a named list
	for run, want := range []float64{3, 0} {
		code, r := a.upload(bobIn, "/api/v1/import", "Saved Places.json", []byte(savedPlaces), nil)
		if code != 200 || num(r, "places") != want {
			t.Fatalf("saved places run %d: %d %v", run+1, code, r)
		}
	}
	code, r := a.upload(bobIn, "/api/v1/import", "Saved Places.json", []byte(savedPlaces), map[string]string{"list": "Starred"})
	if code != 200 || num(r, "places") != 0 || num(r, "already there") != 3 {
		t.Fatalf("named list: %d %v", code, r)
	}
	// a Google Maps saved list CSV: the list from the form field, else the file name
	listCSV := "Title,Note,URL,Comment\nKibe Mahala,Window table,\"https://www.google.com/maps/place/Kibe+Mahala/@43.8629,18.4422,17z/data=!3d43.86292!4d18.44221\",Great view\nFiglmüller,,https://www.google.com/maps/search/Figlm%C3%BCller,\n"
	for run, want := range []float64{2, 0} {
		code, r := a.upload(bobIn, "/api/v1/import/csv", "Saved/Want to go.csv", []byte(listCSV), nil)
		if code != 200 || num(r, "places") != want {
			t.Fatalf("list csv run %d: %d %v", run+1, code, r)
		}
	}
	if code, r := a.upload(bobIn, "/api/v1/import/csv", "list.csv", []byte(listCSV), map[string]string{"list": "Vienna"}); code != 200 || num(r, "places") != 0 {
		t.Fatalf("named csv list: %d %v", code, r)
	}
	lists := map[string]float64{}
	for _, l := range a.list(a.bob, "/api/v1/lists") {
		m := l.(map[string]any)
		lists[m["name"].(string)] = num(m, "count")
	}
	if lists["Google Maps"] != 3 || lists["Starred"] != 3 || lists["Want to go"] != 2 || lists["Vienna"] != 2 || lists["Date night"] != 1 {
		t.Fatalf("bob's lists: %v", lists)
	}
	var kibe map[string]any
	for _, p := range a.list(a.bob, "/api/v1/places?q=kibe") {
		if m := p.(map[string]any); m["source"] == "google" {
			kibe = m
		}
	}
	if kibe == nil || num(kibe, "lat") != 43.86292 || !strings.Contains(kibe["notes"].(string), "Window table") {
		t.Fatalf("kibe from the list: %v", kibe)
	}
	// Bob's places stay Bob's
	if n := len(a.list(a.alice, "/api/v1/places")); n != 2 {
		t.Fatalf("alice now has %d places", n)
	}
	if n := len(a.list(a.bob, "/api/v1/places")); n != 7 {
		t.Fatalf("bob has %d places, want 7", n)
	}
	code, bobDump, _ := a.call(bobOut, "GET", "/api/v1/export.json", nil)
	if code != 200 || len(bobDump["places"].([]any)) != 7 {
		t.Fatalf("bob's export %d", code)
	}
}

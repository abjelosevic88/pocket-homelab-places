package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Places as
// the target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "places:read", Name: "See your places", Description: "Read your places (restaurants, cafés, sights) with your ratings, lists and notes, your visits with what they cost, plans and what waits to be rated — for your journal, your trips, dashboards and Pocket AI."},
	{Scope: "places:write", Name: "Add places and visits", Description: "Add a place to your list and log or plan a visit (\"save this restaurant\", \"we had dinner there\"). It may change or remove only the places and visits that app added itself."},
}

// portable is what Pocket Move may copy out and bring in (docs/move.md in
// pocket-homelab-core). Imports add; what came in before is skipped, so a
// second run of the same file adds nothing.
var portable = &core.Portable{
	Exports: []core.PortableExport{
		{Path: "/api/v1/export.json", File: "places.json", Kind: core.KindPocket, Title: "Every place with its visits and lists"},
		{Path: "/api/v1/export.csv", File: "places.csv", Kind: core.KindCSV, Title: "Every place as a table"},
	},
	Imports: []core.PortableImport{
		{Path: "/api/v1/import", Kind: core.KindPocket, Form: "file", Title: "Adds the places, visits and lists from another box; what came in before is left alone"},
		{Path: "/api/v1/import", Kind: core.KindPlaces, Form: "file", Title: "Adds Google Maps saved places (Saved Places.json, Reviews.json) to the list named in the form field \"list\" (default \"Google Maps\"); places already here are skipped"},
		{Path: "/api/v1/import/csv", Kind: core.KindCSV, Form: "file", Title: "Adds a Google Maps saved list (Title, Note, URL, Comment) as the list named in the form field \"list\" (else the file name), or any CSV with a name column; places already here are skipped"},
	},
}

// capabilities is what the box-wide apps may do here (docs/capabilities.md
// in pocket-homelab-core): Pocket Search finds places, lists and visits,
// Pocket Day reads a day's visits, plans and saved places, Pocket Hub shows
// what is planned and what waits to be tried, and keeps a quick line as a
// place to try. Each pocket:* scope opens its one route only.
var capabilities = core.Capabilities{
	Search: &core.Searchable{What: "Your places by name, kind, cuisine, town, tags and notes, your lists by name, and your visits by their notes",
		Kinds: []string{"place", "visit", "list"}, Bangs: []string{"places", "place", "eat"}},
	Day:    &core.DayFeed{Label: "Places", One: "visit", Many: "visits", What: "Where you went that day, with whom, how you liked it and what it cost, what you planned for the day and the places you saved"},
	Widget: &core.Widget{Title: "Places", What: "Your plans coming up, then the places you want to try"},
	Capture: &core.QuickCapture{Kinds: []core.CaptureKind{
		{Kind: "place", Label: "Place to try", What: "A place you want to try, by its name, in Pocket Places", Fields: []string{"title"}},
	}},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(overview|summary|places|visits|lists|inbox|search|cities|tags|kinds|spending|trips|trip-guide|day/\d{4}-\d{2}-\d{2})$`), "places:read"},
	{"GET", regexp.MustCompile(`^/api/v1/(places|visits|lists)/\d+$`), "places:read"},
	{"POST", regexp.MustCompile(`^/api/v1/(places|visits)$`), "places:write"},
	{"PUT DELETE", regexp.MustCompile(`^/api/v1/(places|visits)/\d+$`), "places:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	if portable.LinkAllows(scopes, r) || capabilities.LinkAllows(scopes, r) {
		return true
	}
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}

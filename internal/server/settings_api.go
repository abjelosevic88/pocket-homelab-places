package server

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-places/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-places/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-places/internal/pocket"
	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

// ---- settings ----

func mask(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "••••"
	}
	return s[:4] + "••••" + s[len(s)-4:]
}

// publicSettings is what one person sees: their settings with the secrets
// masked and, per Pocket app, the mode in effect (<app>_mode), whether it
// works and how (<app>_via: core | direct | "") and why not (<app>_detail).
func (s *Server) publicSettings(ctx context.Context, u *store.User, st store.Settings) map[string]any {
	out := map[string]any{
		"core_configured": s.core.Configured(), "core_registered": s.coreStatus()["registered"],
		"title": st.Title, "date_format": st.DateFormat, "time_format": st.TimeFormat, "week_start": st.WeekStart,
		"currency": st.Currency, "distance_unit": st.DistanceUnit, "online": st.Online, "online_blocked": s.cfg.DemoData,
		"notify_to_rate": st.NotifyToRate, "notify_planned": st.NotifyPlanned, "notify_trips": st.NotifyTrips, "notify_hour": st.NotifyHour,
		"notify_url": st.NotifyURL, "notify_format": st.NotifyFormat, "notify_configured": st.NotifyURL != "",
		"match_visits": st.MatchVisits, "match_payments": st.MatchPayments, "match_minutes": st.MatchMinutes, "match_meters": st.MatchMeters,
		"money_book": st.MoneyBook, "money_account_id": st.MoneyAccountID, "money_category_id": st.MoneyCategoryID,
		"calendar_plans": st.CalendarPlans, "calendar_id": st.CalendarID, "split_group_id": st.SplitGroupID,
		"ai_provider": st.AIProvider, "ai_pocket_model": st.AIPocketModel, "openai_api_key": mask(st.OpenAIKey), "openai_model": st.OpenAIModel, "openai_base_url": st.OpenAIBaseURL,
		"setup_complete": st.SetupComplete, "base_url": s.cfg.BaseURL,
	}
	for _, a := range pocketApps {
		mode, via, detail := s.connStatus(ctx, u, st, a)
		out[a.slug+"_mode"], out[a.slug+"_via"], out[a.slug+"_detail"] = mode, via, detail
		out[a.slug+"_configured"] = via != ""
		_, url, token, public := connSettings(st, a)
		out[a.slug+"_url"], out[a.slug+"_token"], out[a.slug+"_public_url"] = url, mask(token), public
		if public == "" && via != "" {
			public = s.resolvePublic(ctx, a.slug)
		}
		out[a.slug+"_open_url"] = public
	}
	return out
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	st, err := s.store.GetSettings(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	writeJSON(w, http.StatusOK, s.publicSettings(r.Context(), userFrom(r.Context()), st))
}

// handlePutSettings saves the caller's personal settings. The box-wide title
// is only taken from an admin.
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r.Context())
	isAdmin := me.Role == "admin"
	cur, err := s.store.GetSettings(r.Context(), me.ID)
	if err != nil {
		s.fail(w, err, "settings")
		return
	}
	raw := map[string]any{}
	var in store.Settings
	keys, err := decodeKeys(r, &raw)
	if err != nil || remarshal(raw, &in) != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	bad := func(msg string) { writeErr(w, http.StatusBadRequest, msg) }
	if keys["title"] && isAdmin {
		cur.Title = strings.TrimSpace(in.Title)
		if cur.Title == "" {
			cur.Title = store.DefaultTitle
		}
	}
	if keys["date_format"] {
		cur.DateFormat = in.DateFormat
	}
	if keys["time_format"] {
		cur.TimeFormat = in.TimeFormat
	}
	if keys["week_start"] && (in.WeekStart == 1 || in.WeekStart == 7 || in.WeekStart == 6) {
		cur.WeekStart = in.WeekStart
	}
	if keys["currency"] {
		if c := cleanCurrency(in.Currency); c != "" {
			cur.Currency = c
		} else {
			bad("currency is a three-letter code like EUR")
			return
		}
	}
	if keys["distance_unit"] && (in.DistanceUnit == "km" || in.DistanceUnit == "mi") {
		cur.DistanceUnit = in.DistanceUnit
	}
	if keys["match_minutes"] {
		if in.MatchMinutes < 5 || in.MatchMinutes > 600 {
			bad("a visit lasts 5 to 600 minutes")
			return
		}
		cur.MatchMinutes = in.MatchMinutes
	}
	if keys["match_meters"] {
		if in.MatchMeters < 30 || in.MatchMeters > 1000 {
			bad("the distance is 30 to 1000 metres")
			return
		}
		cur.MatchMeters = in.MatchMeters
	}
	if keys["notify_hour"] {
		if in.NotifyHour < 0 || in.NotifyHour > 23 {
			bad("the hour is 0 to 23")
			return
		}
		cur.NotifyHour = in.NotifyHour
	}
	secret := func(k string, dst *string, v string) {
		if keys[k] && !strings.Contains(v, "••") {
			*dst = strings.TrimSpace(v)
		}
	}
	url := func(k string, dst *string, v string) {
		if keys[k] {
			*dst = strings.TrimRight(strings.TrimSpace(v), "/")
		}
	}
	text := func(k string, dst *string, v string) {
		if keys[k] {
			*dst = strings.TrimSpace(v)
		}
	}
	boolIn := func(k string, dst *bool, v bool) {
		if keys[k] {
			*dst = v
		}
	}
	idIn := func(k string, dst *int64, v int64) {
		if keys[k] && v >= 0 {
			*dst = v
		}
	}
	boolIn("online", &cur.Online, in.Online)
	boolIn("notify_to_rate", &cur.NotifyToRate, in.NotifyToRate)
	boolIn("notify_planned", &cur.NotifyPlanned, in.NotifyPlanned)
	boolIn("notify_trips", &cur.NotifyTrips, in.NotifyTrips)
	boolIn("match_visits", &cur.MatchVisits, in.MatchVisits)
	boolIn("match_payments", &cur.MatchPayments, in.MatchPayments)
	text("notify_url", &cur.NotifyURL, in.NotifyURL)
	if keys["notify_format"] && in.NotifyFormat != "" {
		cur.NotifyFormat = in.NotifyFormat
	}
	boolIn("money_book", &cur.MoneyBook, in.MoneyBook)
	idIn("money_account_id", &cur.MoneyAccountID, in.MoneyAccountID)
	idIn("money_category_id", &cur.MoneyCategoryID, in.MoneyCategoryID)
	boolIn("calendar_plans", &cur.CalendarPlans, in.CalendarPlans)
	idIn("calendar_id", &cur.CalendarID, in.CalendarID)
	idIn("split_group_id", &cur.SplitGroupID, in.SplitGroupID)
	// each Pocket app: mode, URL, token, public URL
	inMap := map[string]string{}
	for k, v := range raw {
		if sv, ok := v.(string); ok {
			inMap[k] = sv
		}
	}
	for _, a := range pocketApps {
		c := cur.Conns[a.slug]
		if v := inMap[a.slug+"_mode"]; keys[a.slug+"_mode"] && (v == modeCore || v == modeDirect || v == modeOff) {
			c.Mode = v
		}
		url(a.slug+"_url", &c.URL, inMap[a.slug+"_url"])
		secret(a.slug+"_token", &c.Token, inMap[a.slug+"_token"])
		url(a.slug+"_public_url", &c.PublicURL, inMap[a.slug+"_public_url"])
		if c != (store.Conn{}) {
			cur.Conns[a.slug] = c
		}
	}
	if keys["ai_provider"] {
		if !validAIProvider(in.AIProvider) {
			bad("AI is automatic, Pocket AI or OpenAI")
			return
		}
		cur.AIProvider = in.AIProvider
	}
	if keys["ai_pocket_model"] {
		if !validPocketModel(in.AIPocketModel) {
			bad("unknown model")
			return
		}
		cur.AIPocketModel = in.AIPocketModel
	}
	secret("openai_api_key", &cur.OpenAIKey, in.OpenAIKey)
	text("openai_model", &cur.OpenAIModel, in.OpenAIModel)
	url("openai_base_url", &cur.OpenAIBaseURL, in.OpenAIBaseURL)
	if err := s.store.SaveUserSettings(r.Context(), me.ID, cur); err != nil {
		s.fail(w, err, "save settings")
		return
	}
	if isAdmin && keys["title"] {
		if err := s.store.SaveBoxSettings(r.Context(), cur); err != nil {
			s.fail(w, err, "save settings")
			return
		}
	}
	for k := range keys {
		if isModeKey(k) {
			s.reportModesSoon()
			break
		}
	}
	writeJSON(w, http.StatusOK, s.publicSettings(r.Context(), me, cur))
}

// handleIntegrationTest checks a connection with the caller's own saved
// settings (or the URL/key typed into the form).
func (s *Server) handleIntegrationTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Which  string `json:"which"`
		URL    string `json:"url"`
		Key    string `json:"key"`
		Format string `json:"format"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	uid := actorID(r.Context())
	st, _ := s.store.GetSettings(r.Context(), uid)
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	keyOr := func(saved string) string {
		if strings.Contains(in.Key, "••") || in.Key == "" {
			return saved
		}
		return in.Key
	}
	urlOr := func(saved string) string {
		if strings.TrimSpace(in.URL) == "" {
			return saved
		}
		return strings.TrimRight(strings.TrimSpace(in.URL), "/")
	}
	var msg string
	var err error
	if a, ok := appBySlug(in.Which); ok {
		mode, savedURL, savedTok, _ := connSettings(st, a)
		typed := in.Key != "" && !strings.Contains(in.Key, "••")
		var conn *siblingConn
		if tok := keyOr(savedTok); tok != "" && (typed || s.connMode(mode, savedTok) == modeDirect) {
			base := urlOr(savedURL)
			if base == "" {
				base = s.resolveSibling(ctx, a.slug)
			}
			conn = &siblingConn{client: pocket.New(base, tok, 15*time.Second), via: modeDirect}
		} else {
			conn, err = s.clients(ctx, uid).conn(ctx, a, a.scopes...)
		}
		if err == nil {
			how := "direct connection"
			if conn.via == modeCore {
				how = "through Pocket Core"
			}
			var v string
			if v, err = conn.client.Health(ctx); err == nil {
				msg = fmt.Sprintf("%s %s, %s", a.name, v, how)
				switch a.slug {
				case "money":
					var accts []pocket.Account
					if accts, err = conn.client.Accounts(ctx); err == nil {
						msg += fmt.Sprintf(", %d accounts you can book on", len(accts))
					}
				case "calendar":
					var cals []pocket.Calendar
					if cals, err = conn.client.Calendars(ctx); err == nil {
						msg += fmt.Sprintf(", %d calendars you can write to", len(cals))
					}
				case "travel":
					var trips []pocket.Trip
					if trips, err = conn.client.Trips(ctx); err == nil {
						msg += fmt.Sprintf(", %d trips", len(trips))
					}
				case "contacts":
					_, err = conn.client.SearchContacts(ctx, "a", 1)
				case "movement":
					var stays []pocket.Stay
					if stays, err = conn.client.Stays(ctx, time.Now().AddDate(0, 0, -7), time.Now()); err == nil {
						msg += fmt.Sprintf(", %d stops in the last week", len(stays))
					}
				case "split":
					var gs []pocket.SplitGroup
					if gs, err = conn.client.SplitGroups(ctx); err == nil {
						msg += fmt.Sprintf(", %d groups", len(gs))
					}
				}
			}
		}
	} else if in.Which == "openai" {
		c := ai.New(urlOr(st.OpenAIBaseURL), keyOr(st.OpenAIKey), st.OpenAIModel, 20*time.Second)
		var ids []string
		if ids, err = c.Models(ctx); err == nil {
			msg = fmt.Sprintf("The key works (%d models)", len(ids))
		}
	} else if in.Which == "notify" {
		f := in.Format
		if f == "" {
			f = st.NotifyFormat
		}
		n := notify.New(urlOr(st.NotifyURL), f)
		err = n.Send(ctx, notify.Message{Title: "Pocket Places", Body: notifyTestBody, Priority: "info", Tags: []string{"round_pushpin"}})
		msg = "Test message sent via " + n.Format
	} else {
		writeErr(w, http.StatusBadRequest, "unknown integration")
		return
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

// handleCheckNow runs the daily check for the caller now and says what it sent.
func (s *Server) handleCheckNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st, _ := s.store.GetSettings(ctx, uid)
	body, sent := s.dailyCheck(ctx, uid, st, time.Now())
	writeJSON(w, http.StatusOK, map[string]any{"sent": sent, "body": body})
}

// ---- system / backup / export ----

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	var size int64
	if fi, err := os.Stat(s.cfg.DBPath); err == nil {
		size = fi.Size()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"core":      s.coreStatus(),
		"auth_mode": s.cfg.AuthMode, "data_dir": s.cfg.DataDir, "db_path": s.cfg.DBPath, "db_bytes": size,
		"uptime": time.Since(s.started).Round(time.Second).String(), "timezone": time.Now().Location().String(),
		"counts": s.store.CountsFor(r.Context(), actorID(r.Context())),
	})
}

// handleBackup downloads the whole database, an admin-only box operation.
func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	dir := filepath.Join(s.cfg.DataDir, "backups")
	name := fmt.Sprintf("pocket-places-%s.db", time.Now().Format("20060102-150405"))
	dst := filepath.Join(dir, name)
	if err := s.store.Backup(r.Context(), dst); err != nil {
		s.fail(w, err, "backup")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, dst)
}

// exportDoc is the JSON export of one person's data.
type exportDoc struct {
	App      string            `json:"app"`
	Version  string            `json:"version"`
	Exported string            `json:"exported_at"`
	Places   []store.Place     `json:"places"`
	Visits   []store.Visit     `json:"visits"`
	Lists    []store.List      `json:"lists"`
	Entries  []store.ListEntry `json:"list_places"`
}

func (s *Server) export(ctx context.Context, uid int64) exportDoc {
	doc := exportDoc{App: "pocket-places", Version: Version, Exported: store.Now()}
	doc.Places, _ = s.store.ListPlaces(ctx, uid, store.PlaceFilter{})
	doc.Visits, _, _ = s.store.ListVisits(ctx, uid, store.VisitFilter{Oldest: true})
	doc.Lists, _ = s.store.ListLists(ctx, uid)
	doc.Entries, _ = s.store.ListEntries(ctx, uid, 0)
	return doc
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-places-export.json"`)
	writeJSON(w, http.StatusOK, s.export(r.Context(), actorID(r.Context())))
}

// handleExportCSV writes the places as CSV rows (the same columns the CSV
// import reads back).
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ps, err := s.store.ListPlaces(ctx, actorID(ctx), store.PlaceFilter{})
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-places.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write(csvColumns)
	f := func(v *float64) string {
		if v == nil {
			return ""
		}
		return strconv.FormatFloat(*v, 'f', 6, 64)
	}
	for _, p := range ps {
		fav := ""
		if p.Favourite {
			fav = "yes"
		}
		_ = cw.Write([]string{p.Name, p.Kind, p.Status, strconv.FormatFloat(p.Rating, 'f', -1, 64), strconv.Itoa(p.Price), p.Cuisine, p.Address, p.City, p.Country,
			f(p.Lat), f(p.Lng), p.Website, p.Phone, p.MapURL, p.Tags, fav, p.Notes, strconv.Itoa(p.Visits), p.LastVisit})
	}
	cw.Flush()
}

// handleImportJSON merges a Pocket Places export into the caller's own
// data. Rows keep their identity (the export's ids, remembered per person),
// so importing the same file twice adds nothing. Money, calendar and split
// links are not carried over: they belong to the account the file came from.
func (s *Server) importPocket(ctx context.Context, uid int64, doc exportDoc) map[string]int {
	counts := map[string]int{}
	idOf := func(kind string, old int64) (int64, bool) {
		v, _, ok := s.store.CacheGet(ctx, fmt.Sprintf("import:%d:%s:%d", uid, kind, old))
		if !ok {
			return 0, false
		}
		n, _ := strconv.ParseInt(v, 10, 64)
		return n, n > 0
	}
	remember := func(kind string, old, id int64) {
		_ = s.store.CachePut(ctx, fmt.Sprintf("import:%d:%s:%d", uid, kind, old), strconv.FormatInt(id, 10))
	}
	exists := func(table string, id int64) bool {
		var n int
		_ = s.store.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM `+table+` WHERE id = ? AND user_id = ?`, id, uid).Scan(&n)
		return n > 0
	}
	for _, p := range doc.Places {
		if id, ok := idOf("place", p.ID); ok && exists("places", id) {
			continue
		}
		old := p.ID
		p.Source, p.SourceRef, p.ContactID = "import", fmt.Sprintf("pocket:%d", old), nil
		if prev, err := s.store.PlaceBySource(ctx, uid, p.Source, p.SourceRef); err == nil {
			remember("place", old, prev.ID)
			continue
		}
		if err := s.store.CreatePlace(ctx, uid, &p); err == nil {
			remember("place", old, p.ID)
			counts["places"]++
		}
	}
	for _, l := range doc.Lists {
		if id, ok := idOf("list", l.ID); ok && exists("lists", id) {
			continue
		}
		old := l.ID
		if err := s.store.CreateList(ctx, uid, &l); err == nil {
			remember("list", old, l.ID)
			counts["lists"]++
		}
	}
	for _, e := range doc.Entries {
		lid, ok1 := idOf("list", e.ListID)
		pid, ok2 := idOf("place", e.PlaceID)
		if ok1 && ok2 {
			_ = s.store.AddToList(ctx, uid, lid, pid, e.Note)
		}
	}
	for _, v := range doc.Visits {
		if id, ok := idOf("visit", v.ID); ok && exists("visits", id) {
			continue
		}
		pid, ok := idOf("place", v.PlaceID)
		if !ok || !exists("places", pid) {
			counts["skipped"]++
			continue
		}
		old := v.ID
		v.PlaceID, v.Source, v.SourceRef = pid, "import", fmt.Sprintf("pocket:%d", old)
		v.MoneyTxID, v.MoneyBooked, v.SplitGroupID, v.SplitExpenseID, v.CalEventID = nil, false, nil, nil, nil
		if err := s.store.CreateVisit(ctx, uid, &v); err == nil {
			remember("visit", old, v.ID)
			counts["visits"]++
		}
	}
	return counts
}

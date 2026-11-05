package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-places/internal/store"
)

// Router builds the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	if s.cfg.TrustProxy {
		r.Use(middleware.RealIP)
	}
	r.Use(middleware.RequestID)
	r.Use(s.logging)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	r.Get("/pocket-ask.js", s.core.AskWidget("places"))
	r.Get("/healthz", s.handleHealth)
	r.Get("/readyz", s.handleReady)
	r.Get("/api/health", s.handleHealth)
	if s.cfg.MetricsEnabled {
		r.Get("/metrics", s.handleMetrics)
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.Timeout(120 * time.Second))
		r.Post("/auth/login", s.handleLogin)
		r.Post("/auth/setup", s.handleSetup)
		r.Get("/auth/status", s.handleAuthStatus)
		r.Get("/auth/oidc/start", s.handleOIDCStart)
		r.Get("/auth/oidc/callback", s.handleOIDCCallback)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Post("/auth/logout", s.handleLogout)
			r.Get("/auth/me", s.handleMe)
			r.Put("/auth/me", s.handleUpdateMe)

			// overview
			r.Get("/overview", s.handleOverview)
			r.Get("/summary", s.handleSummary)
			r.Get("/day/{date}", s.handleDay)
			r.Get("/search", s.handleSearch)
			r.Get("/spending", s.handleSpending)
			r.Get("/trips", s.handleTrips)
			r.Get("/trip-guide", s.handleTripGuide)

			// places
			r.Get("/places", s.handleListPlaces)
			r.Post("/places", s.handleCreatePlace)
			r.Get("/places/{id}", s.handleGetPlace)
			r.Put("/places/{id}", s.handleUpdatePlace)
			r.Delete("/places/{id}", s.handleDeletePlace)
			r.Post("/places/{id}/lists", s.handlePlaceAddToList)
			r.Delete("/places/{id}/lists/{list}", s.handlePlaceRemoveFromList)
			r.Get("/kinds", s.handleKinds)
			r.Get("/cities", s.handleCities)
			r.Get("/tags", s.handleTags)
			r.Get("/lookup", s.handleLookup)
			r.Get("/reverse", s.handleReverse)
			r.Post("/resolve", s.handleResolve)

			// visits and plans
			r.Get("/visits", s.handleListVisits)
			r.Post("/visits", s.handleCreateVisit)
			r.Get("/visits/{id}", s.handleGetVisit)
			r.Put("/visits/{id}", s.handleUpdateVisit)
			r.Delete("/visits/{id}", s.handleDeleteVisit)
			r.Post("/visits/{id}/money", s.handleVisitMoney)
			r.Get("/visits/{id}/payments", s.handleVisitPayments)
			r.Post("/visits/{id}/calendar", s.handleVisitCalendar)
			r.Post("/visits/{id}/split", s.handleVisitSplit)

			// your lists
			r.Get("/lists", s.handleListLists)
			r.Post("/lists", s.handleCreateList)
			r.Get("/lists/{id}", s.handleGetList)
			r.Put("/lists/{id}", s.handleUpdateList)
			r.Delete("/lists/{id}", s.handleDeleteList)
			r.Post("/lists/{id}/places", s.handleListAddPlace)
			r.Delete("/lists/{id}/places/{place}", s.handleListRemovePlace)

			// "how was it?"
			r.Get("/inbox", s.handleInbox)
			r.Post("/inbox/scan", s.handleInboxScan)
			r.Post("/inbox/{id}/accept", s.handleInboxAccept)
			r.Post("/inbox/{id}/dismiss", s.handleInboxDismiss)

			// the other Pocket apps
			r.Get("/links/money/accounts", s.handlePickMoneyAccounts)
			r.Get("/links/money/categories", s.handlePickMoneyCategories)
			r.Get("/links/calendar/calendars", s.handlePickCalendars)
			r.Get("/links/contacts", s.handlePickContacts)
			r.Get("/links/split/groups", s.handlePickSplitGroups)

			// AI
			r.Get("/ai/status", s.handleAIStatus)
			r.Get("/ai/models", s.handleAIModels)
			r.Post("/ai/pick", s.handlePick)

			r.Post("/integrations/test", s.handleIntegrationTest)
			r.Post("/notify/test", s.handleNotifyTest)
			r.Post("/notify/check", s.handleCheckNow)

			r.Get("/users", s.handleListUsers)
			r.Post("/users", s.handleCreateUser)
			r.Put("/users/{id}", s.handleUpdateUser)
			r.Delete("/users/{id}", s.handleDeleteUser)

			r.Get("/settings", s.handleGetSettings)
			r.Put("/settings", s.handlePutSettings)
			r.Get("/settings/system", s.handleSystemInfo)

			r.Get("/tokens", s.handleListTokens)
			r.Post("/tokens", s.handleCreateToken)
			r.Delete("/tokens/{id}", s.handleDeleteToken)

			r.Get("/backup", s.handleBackup)
			r.Get("/export.json", s.handleExportJSON)
			r.Get("/export.csv", s.handleExportCSV)
			r.Post("/import", s.handleImport)
			r.Post("/import/csv", s.handleImportCSV)
		})
	})

	r.NotFound(s.handleSPA)
	return r
}

// ---------- helpers ----------

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.requests.Add(1)
		if ww.Status() >= 500 {
			s.errors.Add(1)
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			return
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start).String(), "ip", r.RemoteAddr)
	})
}

type apiError struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

func (s *Server) fail(w http.ResponseWriter, err error, context string) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
		return
	case errors.Is(err, store.ErrConflict), errors.Is(err, core.ErrOperationConflict):
		writeErr(w, http.StatusConflict, "already exists")
		return
	}
	s.log.Error(context, "err", err)
	writeErr(w, http.StatusInternalServerError, context+": "+err.Error())
}

func (s *Server) readBody(r *http.Request) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(nil, r.Body, s.cfg.MaxBodyBytes))
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 32<<20))
	return dec.Decode(v)
}

// decodeKeys decodes JSON into v and also reports which top-level keys were
// present, so PUT handlers can tell "not sent" from "sent empty".
func decodeKeys(r *http.Request, v any) (map[string]bool, error) {
	b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(raw))
	for k := range raw {
		keys[k] = true
	}
	return keys, json.Unmarshal(b, v)
}

func idParam(r *http.Request, name string) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	return n
}

func queryInt(r *http.Request, name string) int {
	n, _ := strconv.Atoi(r.URL.Query().Get(name))
	return n
}

func queryBool(r *http.Request, name string) bool {
	v := strings.ToLower(r.URL.Query().Get(name))
	return v == "1" || v == "true" || v == "yes"
}

func validDate(s string) bool {
	if s == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// ---------- health / metrics ----------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"core_outbox": s.coreOutboxHealth(r.Context()), "status": "ok", "version": Version, "uptime": time.Since(s.started).Round(time.Second).String()})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DB.PingContext(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	counts := s.store.Counts(ctx)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP pocket_places_info Build info.\n# TYPE pocket_places_info gauge\npocket_places_info{version=%q} 1\n", Version)
	fmt.Fprintf(w, "# HELP pocket_places_uptime_seconds Process uptime.\n# TYPE pocket_places_uptime_seconds gauge\npocket_places_uptime_seconds %d\n", int(time.Since(s.started).Seconds()))
	fmt.Fprintf(w, "# HELP pocket_places_http_requests_total HTTP requests served.\n# TYPE pocket_places_http_requests_total counter\npocket_places_http_requests_total %d\n", s.requests.Load())
	fmt.Fprintf(w, "# HELP pocket_places_http_errors_total HTTP 5xx responses.\n# TYPE pocket_places_http_errors_total counter\npocket_places_http_errors_total %d\n", s.errors.Load())
	fmt.Fprintf(w, "# HELP pocket_places_rows Rows per table.\n# TYPE pocket_places_rows gauge\n")
	for k, v := range counts {
		fmt.Fprintf(w, "pocket_places_rows{table=%q} %d\n", k, v)
	}
}

// ---------- static / SPA ----------

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if s.webFS == nil {
		http.Error(w, "web UI not built (run `make web`)", http.StatusNotFound)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if f, err := s.webFS.Open(path); err == nil {
		f.Close()
		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.FileServerFS(s.webFS).ServeHTTP(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	r.URL.Path = "/"
	http.FileServerFS(s.webFS).ServeHTTP(w, r)
}

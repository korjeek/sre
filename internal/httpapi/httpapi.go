package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sre-incident-tracker/internal/metrics"
	"strconv"
	"strings"
	"time"

	"sre-incident-tracker/internal/models"
	"sre-incident-tracker/internal/store"
)

type Server struct {
	store     *store.Store
	log       *slog.Logger
	staticDir string
	metrics   *metrics.Metrics
}

func New(st *store.Store, log *slog.Logger, staticDir string, m *metrics.Metrics) http.Handler {
	s := &Server{store: st, log: log, staticDir: staticDir, metrics: m}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /api/services", s.listServices)
	mux.HandleFunc("POST /api/services", s.createService)
	mux.HandleFunc("GET /api/services/{id}", s.getService)
	mux.HandleFunc("PUT /api/services/{id}", s.updateService)
	mux.HandleFunc("DELETE /api/services/{id}", s.deleteService)

	mux.HandleFunc("GET /api/incidents", s.listIncidents)
	mux.HandleFunc("POST /api/incidents", s.createIncident)
	mux.HandleFunc("GET /api/incidents/{id}", s.getIncident)
	mux.HandleFunc("PUT /api/incidents/{id}", s.updateIncident)
	mux.HandleFunc("DELETE /api/incidents/{id}", s.deleteIncident)

	mux.Handle("GET /", s.staticHandler())

	return s.requestLog(mux)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listServices(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListServices(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) getService(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := s.store.GetService(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) createService(w http.ResponseWriter, r *http.Request) {
	var in models.Service
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := validateService(in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	item, err := s.store.CreateService(r.Context(), in)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.metrics.ServiceCreated()
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) updateService(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in models.Service
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := validateService(in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	in.ID = id
	item, err := s.store.UpdateService(r.Context(), in)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) deleteService(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteService(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	s.metrics.ServiceDeleted()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listIncidents(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListIncidents(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) getIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, err := s.store.GetIncident(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) createIncident(w http.ResponseWriter, r *http.Request) {
	var in models.Incident
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := validateIncident(in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	item, err := s.store.CreateIncident(r.Context(), in)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.metrics.IncidentCreated(item.Severity)
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) updateIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in models.Incident
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if err := validateIncident(in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	in.ID = id
	prev, err := s.store.GetIncident(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	item, err := s.store.UpdateIncident(r.Context(), in)
	if err != nil {
		s.fail(w, err)
		return
	}
	if prev.Status != item.Status {
		s.metrics.IncidentStatusChanged(prev.Status, item.Status, item.Severity,
			item.UpdatedAt.Sub(prev.CreatedAt))
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) deleteIncident(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteIncident(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	s.metrics.IncidentDeleted()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) staticHandler() http.Handler {
	fs := http.FileServer(http.Dir(s.staticDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		path := filepath.Join(s.staticDir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(s.staticDir, "index.html"))
	})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	s.log.Error("request failed", "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
}

func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = newRequestID()
		}
		w.Header().Set("X-Request-ID", reqID)

		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		s.metrics.HTTPInFlightInc()
		next.ServeHTTP(rw, r)
		s.metrics.HTTPInFlightDec()
		elapsed := time.Since(start)
		s.metrics.ObserveHTTP(r.Method, routeLabel(r.Pattern), rw.status, elapsed)

		level := slog.LevelInfo
		switch {
		case r.URL.Path == "/healthz":
			level = slog.LevelDebug
		case rw.status >= 500:
			level = slog.LevelError
		case rw.status >= 400:
			level = slog.LevelWarn
		}

		s.log.LogAttrs(r.Context(), level, "http request",
			slog.String("request_id", reqID),
			slog.String("http.method", r.Method),
			slog.String("http.path", r.URL.Path),
			slog.String("http.route", r.Pattern),
			slog.Int("http.status_code", rw.status),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
	})
}

func routeLabel(pattern string) string {
	if pattern == "" {
		return "unmatched"
	}
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		return pattern[i+1:]
	}
	return pattern
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return 0, false
	}
	return id, true
}

func validateService(in models.Service) error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(in.Owner) == "" {
		return errors.New("owner is required")
	}
	return nil
}

func validateIncident(in models.Incident) error {
	if in.ServiceID < 1 {
		return errors.New("service_id is required")
	}
	if strings.TrimSpace(in.Title) == "" {
		return errors.New("title is required")
	}
	switch in.Severity {
	case "low", "medium", "high", "critical":
	default:
		return errors.New("severity must be low, medium, high or critical")
	}
	switch in.Status {
	case "open", "investigating", "resolved":
	default:
		return errors.New("status must be open, investigating or resolved")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

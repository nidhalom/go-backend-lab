// Package server wires the HTTP API of the URL shortener.
package server

import (
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nidhalom/go-url-shortener/internal/ratelimit"
	"github.com/nidhalom/go-url-shortener/internal/store"
)

const maxURLLen = 2048

// Config holds the server dependencies.
type Config struct {
	Store   store.Store
	Limiter *ratelimit.Limiter // optional
	BaseURL string             // public base, e.g. http://localhost:8080
	Now     func() time.Time   // optional, defaults to time.Now
}

type server struct {
	cfg Config
	now func() time.Time
}

// New returns the HTTP handler.
func New(cfg Config) http.Handler {
	s := &server{cfg: cfg, now: cfg.Now}
	if s.now == nil {
		s.now = time.Now
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/links", s.create)
	mux.HandleFunc("GET /api/links/{id}", s.stats)
	mux.HandleFunc("GET /{id}", s.redirect)
	var h http.Handler = mux
	if cfg.Limiter != nil {
		h = s.limit(h)
	}
	return h
}

type createRequest struct {
	URL        string `json:"url"`
	TTLSeconds int64  `json:"ttl_seconds,omitempty"`
}

type linkResponse struct {
	store.Link
	ShortURL string `json:"short_url"`
	Expired  bool   `json:"expired"`
}

func (s *server) respond(l store.Link) linkResponse {
	return linkResponse{Link: l, ShortURL: strings.TrimRight(s.cfg.BaseURL, "/") + "/" + l.ID, Expired: l.Expired(s.now())}
}

func (s *server) create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateURL(req.URL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.TTLSeconds < 0 {
		writeError(w, http.StatusBadRequest, "ttl_seconds must be positive")
		return
	}
	var ttl *time.Duration
	if req.TTLSeconds > 0 {
		d := time.Duration(req.TTLSeconds) * time.Second
		ttl = &d
	}
	l, err := s.cfg.Store.Create(req.URL, ttl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not store link")
		return
	}
	w.Header().Set("Location", "/api/links/"+l.ID)
	writeJSON(w, http.StatusCreated, s.respond(l))
}

func (s *server) stats(w http.ResponseWriter, r *http.Request) {
	l, err := s.cfg.Store.Get(r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "link not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, s.respond(l))
}

func (s *server) redirect(w http.ResponseWriter, r *http.Request) {
	l, err := s.cfg.Store.Hit(r.PathValue("id"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "link not found")
	case errors.Is(err, store.ErrExpired):
		writeError(w, http.StatusGone, "link expired")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal error")
	default:
		http.Redirect(w, r, l.URL, http.StatusFound)
	}
}

func (s *server) limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, retry := s.cfg.Limiter.Allow(clientIP(r))
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientIP uses the TCP peer address; X-Forwarded-For is deliberately ignored
// because it is client-controlled unless a trusted proxy rewrites it.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func validateURL(raw string) error {
	if raw == "" {
		return errors.New("url is required")
	}
	if len(raw) > maxURLLen {
		return errors.New("url too long")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("url must be an absolute http(s) URL")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nidhalom/go-url-shortener/internal/ratelimit"
	"github.com/nidhalom/go-url-shortener/internal/store"
)

func newTestServer(t *testing.T, cfg Config) *httptest.Server {
	t.Helper()
	if cfg.Store == nil {
		cfg.Store = store.NewMemory()
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://short.test"
	}
	ts := httptest.NewServer(New(cfg))
	t.Cleanup(ts.Close)
	return ts
}

// noFollow returns a client that surfaces redirects instead of following them.
func noFollow() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func post(t *testing.T, ts *httptest.Server, body string) (*http.Response, linkResponse) {
	t.Helper()
	resp, err := http.Post(ts.URL+"/api/links", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var lr linkResponse
	json.NewDecoder(resp.Body).Decode(&lr)
	return resp, lr
}

func TestCreateRedirectAndStats(t *testing.T) {
	ts := newTestServer(t, Config{})
	resp, lr := post(t, ts, `{"url":"https://example.com/a?b=1"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if lr.ShortURL != "http://short.test/"+lr.ID {
		t.Errorf("short_url = %q", lr.ShortURL)
	}

	c := noFollow()
	for i := 0; i < 2; i++ {
		r, err := c.Get(ts.URL + "/" + lr.ID)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != http.StatusFound || r.Header.Get("Location") != "https://example.com/a?b=1" {
			t.Fatalf("redirect: %d %q", r.StatusCode, r.Header.Get("Location"))
		}
	}

	r, err := http.Get(ts.URL + "/api/links/" + lr.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var stats linkResponse
	json.NewDecoder(r.Body).Decode(&stats)
	if stats.Clicks != 2 {
		t.Errorf("clicks = %d, want 2", stats.Clicks)
	}
}

func TestValidation(t *testing.T) {
	ts := newTestServer(t, Config{})
	bodies := []string{`{}`, `{"url":"ftp://x.com"}`, `{"url":"javascript:alert(1)"}`, `{"url":"/relative"}`, `not json`, `{"url":"https://x.com","ttl_seconds":-5}`}
	for _, body := range bodies {
		resp, _ := post(t, ts, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", body, resp.StatusCode)
		}
	}
}

func TestNotFound(t *testing.T) {
	ts := newTestServer(t, Config{})
	r, err := http.Get(ts.URL + "/doesnotexist")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusNotFound {
		t.Errorf("status %d", r.StatusCode)
	}
}

func TestExpiredLinkIsGone(t *testing.T) {
	var offset atomic.Int64 // seconds added to the shared fake clock
	clock := func() time.Time {
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(offset.Load()) * time.Second)
	}
	ts := newTestServer(t, Config{Store: store.NewMemory(store.WithClock(clock)), Now: clock})
	_, lr := post(t, ts, `{"url":"https://example.com","ttl_seconds":60}`)
	if lr.ExpiresAt == nil || lr.Expired {
		t.Fatalf("fresh link should have expiry and not be expired: %+v", lr)
	}

	c := noFollow()
	r, _ := c.Get(ts.URL + "/" + lr.ID)
	r.Body.Close()
	if r.StatusCode != http.StatusFound {
		t.Fatalf("before expiry: status %d", r.StatusCode)
	}

	offset.Store(61)
	r, _ = c.Get(ts.URL + "/" + lr.ID)
	r.Body.Close()
	if r.StatusCode != http.StatusGone {
		t.Errorf("after expiry: status %d, want 410", r.StatusCode)
	}
	r, err := http.Get(ts.URL + "/api/links/" + lr.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var stats linkResponse
	json.NewDecoder(r.Body).Decode(&stats)
	if !stats.Expired || stats.Clicks != 1 {
		t.Errorf("stats after expiry: %+v", stats)
	}
}

func TestRateLimitPerIP(t *testing.T) {
	ts := newTestServer(t, Config{Limiter: ratelimit.New(0.001, 3, nil)})
	want := []int{200, 200, 200, 429, 429}
	for i, w := range want {
		r, err := http.Get(ts.URL + "/healthz")
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != w {
			t.Fatalf("request %d: status %d, want %d", i, r.StatusCode, w)
		}
		if w == 429 && r.Header.Get("Retry-After") == "" {
			t.Error("missing Retry-After")
		}
	}
}

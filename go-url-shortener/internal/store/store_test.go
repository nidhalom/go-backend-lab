package store

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }

func TestCreateGetHit(t *testing.T) {
	m := NewMemory()
	l, err := m.Create("https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.ID) < 4 {
		t.Errorf("id %q too short", l.ID)
	}
	for i := 0; i < 3; i++ {
		if _, err := m.Hit(l.ID); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := m.Get(l.ID)
	if got.Clicks != 3 {
		t.Errorf("clicks = %d, want 3", got.Clicks)
	}
	if _, err := m.Get("nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestExpiry(t *testing.T) {
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	m := NewMemory(WithClock(clk.Now))
	ttl := time.Minute
	l, _ := m.Create("https://example.com", &ttl)
	if _, err := m.Hit(l.ID); err != nil {
		t.Fatal(err)
	}
	clk.t = clk.t.Add(time.Minute)
	if _, err := m.Hit(l.ID); !errors.Is(err, ErrExpired) {
		t.Fatalf("want ErrExpired, got %v", err)
	}
	got, _ := m.Get(l.ID)
	if got.Clicks != 1 {
		t.Errorf("expired hit must not count, clicks = %d", got.Clicks)
	}
}

func TestConcurrentHits(t *testing.T) {
	m := NewMemory()
	l, _ := m.Create("https://example.com", nil)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); m.Hit(l.ID) }()
	}
	wg.Wait()
	if got, _ := m.Get(l.ID); got.Clicks != 50 {
		t.Errorf("clicks = %d, want 50", got.Clicks)
	}
}

func TestJSONFilePersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "links.json")
	a, err := NewJSONFile(path)
	if err != nil {
		t.Fatal(err)
	}
	l1, _ := a.Create("https://a.example", nil)
	a.Hit(l1.ID)

	b, err := NewJSONFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.Get(l1.ID)
	if err != nil || got.Clicks != 1 || got.URL != "https://a.example" {
		t.Fatalf("reload mismatch: %+v err=%v", got, err)
	}
	l2, _ := b.Create("https://b.example", nil)
	if l2.ID == l1.ID {
		t.Error("sequence must continue after reload")
	}
}

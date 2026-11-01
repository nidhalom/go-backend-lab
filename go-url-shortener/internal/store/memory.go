package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nidhalom/go-url-shortener/internal/base62"
)

// idOffset keeps generated IDs at least four characters long.
const idOffset = 238328 // 62^3

// Memory is a Store held in memory, optionally mirrored to a JSON file.
type Memory struct {
	mu    sync.Mutex
	links map[string]*Link
	seq   uint64
	now   func() time.Time
	path  string // empty: no persistence
}

// Option configures a Memory store.
type Option func(*Memory)

// WithClock overrides the time source (used by tests).
func WithClock(now func() time.Time) Option { return func(m *Memory) { m.now = now } }

// NewMemory returns an empty in-memory store.
func NewMemory(opts ...Option) *Memory {
	m := &Memory{links: map[string]*Link{}, now: time.Now}
	for _, o := range opts {
		o(m)
	}
	return m
}

type snapshot struct {
	Seq   uint64  `json:"seq"`
	Links []*Link `json:"links"`
}

// NewJSONFile returns a store that loads path if it exists and rewrites it
// atomically after every mutation.
func NewJSONFile(path string, opts ...Option) (*Memory, error) {
	m := NewMemory(opts...)
	m.path = path
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return m, nil
	case err != nil:
		return nil, err
	}
	var s snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("store: parse %s: %w", path, err)
	}
	m.seq = s.Seq
	for _, l := range s.Links {
		m.links[l.ID] = l
	}
	return m, nil
}

func (m *Memory) Create(url string, ttl *time.Duration) (Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	now := m.now().UTC()
	l := &Link{ID: base62.Encode(idOffset + m.seq), URL: url, CreatedAt: now}
	if ttl != nil {
		exp := now.Add(*ttl)
		l.ExpiresAt = &exp
	}
	m.links[l.ID] = l
	if err := m.persistLocked(); err != nil {
		delete(m.links, l.ID)
		m.seq--
		return Link{}, err
	}
	return *l, nil
}

func (m *Memory) Get(id string) (Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.links[id]
	if !ok {
		return Link{}, ErrNotFound
	}
	return *l, nil
}

func (m *Memory) Hit(id string) (Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	l, ok := m.links[id]
	if !ok {
		return Link{}, ErrNotFound
	}
	if l.Expired(m.now()) {
		return *l, ErrExpired
	}
	l.Clicks++
	if err := m.persistLocked(); err != nil {
		l.Clicks--
		return Link{}, err
	}
	return *l, nil
}

func (m *Memory) persistLocked() error {
	if m.path == "" {
		return nil
	}
	s := snapshot{Seq: m.seq, Links: make([]*Link, 0, len(m.links))}
	for _, l := range m.links {
		s.Links = append(s.Links, l)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(m.path), ".links-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp.Name())
		return errors.Join(werr, cerr)
	}
	if err := os.Rename(tmp.Name(), m.path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

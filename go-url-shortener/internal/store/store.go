// Package store defines the link storage interface and two implementations:
// an in-memory store and one that persists to a JSON file.
package store

import (
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("store: link not found")
	ErrExpired  = errors.New("store: link expired")
)

// Link is a shortened URL.
type Link struct {
	ID        string     `json:"id"`
	URL       string     `json:"url"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Clicks    int64      `json:"clicks"`
}

// Expired reports whether the link is past its expiry at time now.
func (l Link) Expired(now time.Time) bool {
	return l.ExpiresAt != nil && !now.Before(*l.ExpiresAt)
}

// Store is the persistence boundary of the service. Implementations must be
// safe for concurrent use.
type Store interface {
	// Create stores url under a freshly generated ID. A nil ttl means the link never expires.
	Create(url string, ttl *time.Duration) (Link, error)
	// Get returns a link without counting a click; expired links are still returned.
	Get(id string) (Link, error)
	// Hit resolves a link for redirecting: it returns ErrNotFound or ErrExpired,
	// otherwise increments the click counter and returns the updated link.
	Hit(id string) (Link, error)
}

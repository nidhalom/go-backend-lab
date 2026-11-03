package ratelimit

import (
	"testing"
	"time"
)

func TestBurstThenRefill(t *testing.T) {
	now := time.Unix(1000, 0)
	l := New(1, 3, func() time.Time { return now })
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("ip"); !ok {
			t.Fatalf("request %d should pass", i)
		}
	}
	ok, retry := l.Allow("ip")
	if ok || retry <= 0 || retry > time.Second {
		t.Fatalf("expected denial with retry<=1s, got ok=%v retry=%v", ok, retry)
	}
	now = now.Add(1500 * time.Millisecond)
	if ok, _ := l.Allow("ip"); !ok {
		t.Error("token should have refilled")
	}
	if ok, _ := l.Allow("ip"); ok {
		t.Error("only one token should have refilled")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	now := time.Unix(0, 0)
	l := New(1, 1, func() time.Time { return now })
	l.Allow("a")
	if ok, _ := l.Allow("a"); ok {
		t.Error("a should be limited")
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Error("b should be independent")
	}
}

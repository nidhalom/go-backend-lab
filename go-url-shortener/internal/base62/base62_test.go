package base62

import (
	"math"
	"testing"
)

func TestKnownValues(t *testing.T) {
	cases := map[uint64]string{0: "0", 9: "9", 10: "A", 35: "Z", 36: "a", 61: "z", 62: "10", 3844: "100"}
	for n, want := range cases {
		if got := Encode(n); got != want {
			t.Errorf("Encode(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	for _, n := range []uint64{0, 1, 61, 62, 12345, 1 << 32, math.MaxUint64} {
		got, err := Decode(Encode(n))
		if err != nil || got != n {
			t.Errorf("round trip %d: got %d, err %v", n, got, err)
		}
	}
}

func TestDecodeErrors(t *testing.T) {
	// "zzzzzzzzzzz" (11 chars) is 62^11-1 > MaxUint64, so it overflows.
	for _, s := range []string{"", "a-b", "!", "zzzzzzzzzzzz", "zzzzzzzzzzz"} {
		if _, err := Decode(s); err == nil {
			t.Errorf("Decode(%q) should fail", s)
		}
	}
}

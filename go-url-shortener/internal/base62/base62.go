// Package base62 encodes unsigned integers using the alphabet 0-9A-Za-z.
package base62

import (
	"errors"
	"strings"
)

const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// ErrInvalid is returned by Decode for empty, invalid or overflowing input.
var ErrInvalid = errors.New("base62: invalid input")

// Encode returns the base62 representation of n.
func Encode(n uint64) string {
	if n == 0 {
		return string(alphabet[0])
	}
	var buf [11]byte // 62^11 > 2^64
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = alphabet[n%62]
		n /= 62
	}
	return string(buf[i:])
}

// Decode parses a string produced by Encode.
func Decode(s string) (uint64, error) {
	if s == "" || len(s) > 11 {
		return 0, ErrInvalid
	}
	var n uint64
	for i := 0; i < len(s); i++ {
		d := strings.IndexByte(alphabet, s[i])
		if d < 0 {
			return 0, ErrInvalid
		}
		if n > (^uint64(0)-uint64(d))/62 {
			return 0, ErrInvalid
		}
		n = n*62 + uint64(d)
	}
	return n, nil
}

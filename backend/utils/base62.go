// Package utils provides shared helpers used across the URL shortener.
//
// Base62 encoding maps positive int64 values to a compact alphanumeric string
// using digits (0-9), lowercase (a-z), and uppercase (A-Z) — 62 symbols total.
//
// Why Base62 instead of Base64?
//   - No special characters (+ / =) that cause issues in URLs.
//   - Case-sensitive but still human-readable.
//   - 62^6 ≈ 56.8 billion — more than enough capacity for a URL shortener.
package utils

import (
	"math/big"
	"strings"
)

// charset defines the Base62 alphabet. The ordering is intentional:
// digits first, then lowercase, then uppercase. This must never change
// once URLs are in production or existing short keys will break.
const charset = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

const base = int64(len(charset)) // 62

const (
	offsetVal  = int64(56800235584)   // 62^6
	limitVal   = int64(3521614606208) // 62^7
	rangeVal   = int64(3464814370624) // 62^7 - 62^6
	primeVal   = int64(100000000003)  // Large prime coprime to rangeVal
	inverseVal = int64(2864984937195) // Modular inverse of primeVal mod rangeVal
	shiftVal   = int64(1234567890123) // Additive shift to avoid encoding 0 to '0'
)

// mulMod calculates (a * b) % m using math/big to prevent overflow of int64.
func mulMod(a, b, m int64) int64 {
	var abig, bbig, mbig big.Int
	abig.SetInt64(a)
	bbig.SetInt64(b)
	mbig.SetInt64(m)

	var prod big.Int
	prod.Mul(&abig, &bbig)

	var mod big.Int
	mod.Mod(&prod, &mbig)

	return mod.Int64()
}

// scramble maps a sequential ID in the range [offsetVal, limitVal - 1] to a pseudo-random ID in the same range.
func scramble(id int64) int64 {
	if id < offsetVal || id >= limitVal {
		return id
	}
	offset := id - offsetVal
	prodMod := mulMod(offset, primeVal, rangeVal)
	scrambledRange := (prodMod + shiftVal) % rangeVal
	return offsetVal + scrambledRange
}

// unscramble reverses the scramble mapping.
func unscramble(scrambled int64) int64 {
	if scrambled < offsetVal || scrambled >= limitVal {
		return scrambled
	}
	offset := scrambled - offsetVal
	diff := offset - shiftVal
	for diff < 0 {
		diff += rangeVal
	}
	prodMod := mulMod(diff, inverseVal, rangeVal)
	return offsetVal + prodMod
}

// Encode converts a non-negative int64 to its Base62 string representation.
// If the number falls in the sequential range [62^6, 62^7-1], it is first scrambled.
// Encode(0) returns "0" (the first character in the charset).
func Encode(num int64) string {
	num = scramble(num)
	if num == 0 {
		return string(charset[0])
	}

	// Build the result in reverse order (least-significant digit first),
	// then reverse at the end. Using a strings.Builder avoids repeated
	// string concatenation allocations.
	var sb strings.Builder
	for num > 0 {
		remainder := num % base
		sb.WriteByte(charset[remainder])
		num /= base
	}

	// Reverse the bytes to get most-significant digit first.
	encoded := []byte(sb.String())
	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}

	return string(encoded)
}

// Decode converts a Base62 string back to the original int64.
// Returns -1 if the string contains characters outside the charset.
func Decode(s string) int64 {
	var num int64
	for _, c := range s {
		idx := strings.IndexRune(charset, c)
		if idx < 0 {
			return -1 // invalid character
		}
		num = num*base + int64(idx)
	}
	return unscramble(num)
}


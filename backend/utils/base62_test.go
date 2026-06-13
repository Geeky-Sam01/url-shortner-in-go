package utils

import (
	"fmt"
	"testing"
)

// TestEncodeKnownValues verifies Base62 encoding against pre-computed values.
func TestEncodeKnownValues(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{9, "9"},
		{10, "a"},
		{35, "z"},
		{36, "A"},
		{61, "Z"},
		{62, "10"},
		{63, "11"},
		{3844, "100"},        // 62^2
		{238328, "1000"},     // 62^3
		{14776336, "10000"},  // 62^4
		{916132832, "100000"}, // 62^5
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Encode(%d)", tt.input), func(t *testing.T) {
			got := Encode(tt.input)
			if got != tt.expected {
				t.Errorf("Encode(%d) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestDecodeKnownValues verifies Base62 decoding against pre-computed values.
func TestDecodeKnownValues(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"0", 0},
		{"1", 1},
		{"a", 10},
		{"z", 35},
		{"A", 36},
		{"Z", 61},
		{"10", 62},
		{"100", 3844},
		{"1000", 238328},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Decode(%q)", tt.input), func(t *testing.T) {
			got := Decode(tt.input)
			if got != tt.expected {
				t.Errorf("Decode(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

// TestDecodeInvalidCharacter ensures Decode returns -1 for bad input.
func TestDecodeInvalidCharacter(t *testing.T) {
	invalids := []string{"hello!", "abc-def", "foo bar", "#tag"}
	for _, s := range invalids {
		t.Run(fmt.Sprintf("Decode(%q)", s), func(t *testing.T) {
			got := Decode(s)
			if got != -1 {
				t.Errorf("Decode(%q) = %d, want -1", s, got)
			}
		})
	}
}

// TestSequenceStartValue verifies that the sequence start value (62^6)
// produces a 7-character short key.
//
// Math: 62^6 = 56 800 235 584. All IDs from this point forward will be
// scrambled but still result in exactly 7 characters until we reach 62^7.
func TestSequenceStartValue(t *testing.T) {
	const sequenceStart int64 = 56800235584 // 62^6

	encoded := Encode(sequenceStart)

	// Since we scramble, it should not be the raw "1000000" anymore
	if encoded == "1000000" {
		t.Errorf("Encode(%d) = %q, expected scrambled string, not raw base62", sequenceStart, encoded)
	}
	if len(encoded) != 7 {
		t.Errorf("Encode(%d) = %q (len %d), want 7 characters",
			sequenceStart, encoded, len(encoded))
	}

	decoded := Decode(encoded)
	if decoded != sequenceStart {
		t.Errorf("round-trip failed: Encode(%d) = %q, Decode(%q) = %d",
			sequenceStart, encoded, encoded, decoded)
	}
}

// TestRoundTrip tests that Encode → Decode is the identity function
// for a variety of values.
func TestRoundTrip(t *testing.T) {
	values := []int64{
		0, 1, 10, 61, 62, 100, 999, 3843, 3844,
		1000000, 56800235584, // sequence start (62^6)
		56800235585,          // sequence start + 1
		999999999999,         // large number
	}

	for _, v := range values {
		t.Run(fmt.Sprintf("RoundTrip(%d)", v), func(t *testing.T) {
			encoded := Encode(v)
			decoded := Decode(encoded)
			if decoded != v {
				t.Errorf("RoundTrip failed: Encode(%d) = %q, Decode(%q) = %d",
					v, encoded, encoded, decoded)
			}
		})
	}
}

// TestEncodeZero ensures the zero case is handled correctly.
func TestEncodeZero(t *testing.T) {
	got := Encode(0)
	if got != "0" {
		t.Errorf("Encode(0) = %q, want %q", got, "0")
	}
	if Decode("0") != 0 {
		t.Errorf("Decode(\"0\") = %d, want 0", Decode("0"))
	}
}

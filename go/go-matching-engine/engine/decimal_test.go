package engine

import "testing"

func TestParseDecimal(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"0", 0},
		{"1", 1 * Scale},
		{"50000.50", 5_000_050_000_000},
		{"50000.5", 5_000_050_000_000},
		{"0.5", 50_000_000},
		{"0.00000001", 1},
		{".5", 50_000_000},
		{"123.45678901", 12_345_678_901}, // 123 * 1e8 + 45678901
	}
	for _, tc := range cases {
		got, err := ParseDecimal(tc.in)
		if err != nil {
			t.Errorf("ParseDecimal(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseDecimal(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseDecimalErrors(t *testing.T) {
	bad := []string{
		"",
		"-1",
		"1.2.3",
		"1.234567890",   // 9 fractional digits
		"abc",
		"1.a",
	}
	for _, s := range bad {
		if _, err := ParseDecimal(s); err == nil {
			t.Errorf("ParseDecimal(%q) expected error, got nil", s)
		}
	}
}

func TestFormatDecimal(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{Scale, "1"},
		{50_000_000, "0.5"},
		{5_000_050_000_000, "50000.5"},
		{1, "0.00000001"},
		{12_345_678_901, "123.45678901"},
	}
	for _, tc := range cases {
		got := FormatDecimal(tc.in)
		if got != tc.want {
			t.Errorf("FormatDecimal(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	// FormatDecimal(ParseDecimal(s)) is canonical (trailing zeros removed),
	// but should be value-stable.
	cases := []string{"0", "1", "50000.5", "0.00000001", "12345.6789"}
	for _, s := range cases {
		v, err := ParseDecimal(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		out := FormatDecimal(v)
		if out != s {
			t.Errorf("round-trip %q → %d → %q (canonical form differs)", s, v, out)
		}
	}
}

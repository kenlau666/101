package engine

import (
	"errors"
	"strings"
)

// Scale is the fixed-point scale factor: 10^8.
// All prices and quantities are stored as int64 = decimal_value * Scale.
const Scale int64 = 100_000_000

const scaleDigits = 8

var (
	errEmpty       = errors.New("decimal: empty string")
	errBadChar     = errors.New("decimal: invalid character")
	errMultipleDot = errors.New("decimal: multiple decimal points")
	errTooPrecise  = errors.New("decimal: more than 8 fractional digits")
	errOverflow    = errors.New("decimal: integer part overflows int64")
	errNegative    = errors.New("decimal: negative values not allowed")
)

// ParseDecimal converts a non-negative decimal string like "50000.50"
// into a fixed-point int64 at scale 10^8 (e.g. 5_000_050_000_000).
// No floats are used.
func ParseDecimal(s string) (int64, error) {
	if len(s) == 0 {
		return 0, errEmpty
	}
	if s[0] == '-' {
		return 0, errNegative
	}
	dot := strings.IndexByte(s, '.')
	intPart := s
	fracPart := ""
	if dot >= 0 {
		if strings.IndexByte(s[dot+1:], '.') >= 0 {
			return 0, errMultipleDot
		}
		intPart = s[:dot]
		fracPart = s[dot+1:]
	}
	if len(fracPart) > scaleDigits {
		return 0, errTooPrecise
	}
	var intVal int64
	if intPart != "" {
		v, err := parseDigits(intPart)
		if err != nil {
			return 0, err
		}
		// Check overflow before multiplying by Scale.
		const maxInt = int64(1<<63 - 1)
		if v > maxInt/Scale {
			return 0, errOverflow
		}
		intVal = v * Scale
	}
	var fracVal int64
	if fracPart != "" {
		v, err := parseDigits(fracPart)
		if err != nil {
			return 0, err
		}
		// Pad to 8 digits: e.g. ".5" → 5 → 50_000_000.
		mult := int64(1)
		for i := 0; i < scaleDigits-len(fracPart); i++ {
			mult *= 10
		}
		fracVal = v * mult
	}
	return intVal + fracVal, nil
}

func parseDigits(s string) (int64, error) {
	var v int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, errBadChar
		}
		v = v*10 + int64(c-'0')
	}
	return v, nil
}

// FormatDecimal converts a fixed-point int64 back to its canonical
// decimal string. Trailing zeros after the decimal point are trimmed.
// 5_000_050_000_000 → "50000.5", 50_000_000 → "0.5", 100_000_000 → "1".
func FormatDecimal(x int64) string {
	if x < 0 {
		return "-" + FormatDecimal(-x)
	}
	intPart := x / Scale
	fracPart := x % Scale

	var b strings.Builder
	b.Grow(24)
	writeUint(&b, uint64(intPart))

	if fracPart == 0 {
		return b.String()
	}
	// Render fracPart left-padded to 8 digits, then trim trailing zeros.
	var frac [scaleDigits]byte
	for i := scaleDigits - 1; i >= 0; i-- {
		frac[i] = byte('0' + fracPart%10)
		fracPart /= 10
	}
	end := scaleDigits
	for end > 0 && frac[end-1] == '0' {
		end--
	}
	b.WriteByte('.')
	b.Write(frac[:end])
	return b.String()
}

func writeUint(b *strings.Builder, v uint64) {
	if v == 0 {
		b.WriteByte('0')
		return
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	b.Write(buf[i:])
}

// Package half converts decimal strings into IEEE 754 binary16 (half
// precision) bit patterns using only arbitrary-precision integer and
// rational arithmetic. Host floating point is never consulted, so every
// rounding decision is exact.
package half

import (
	"math/big"
	"strings"
)

// Class of a binary16 encoding.
type Class string

const (
	ClassZero      Class = "zero"
	ClassSubnormal Class = "subnormal"
	ClassNormal    Class = "normal"
	ClassInfinity  Class = "infinity"
)

const (
	// MaxSigDigits is the maximum number of significant decimal digits
	// accepted in one value.
	MaxSigDigits = 30
	// MinDecExp / MaxDecExp bound the decimal exponent of an input value,
	// defined as the exponent of its leading significant digit in
	// scientific notation (so 0.001 is 10^-3).
	MinDecExp = -50
	MaxDecExp = 50
)

// parseError identifies a rejected decimal string. All other failure modes
// inside Convert are reported as parseError as well (they all mean the
// value did not meet the input contract).
func parseError() *Result { return &Result{reject: true} }

// decimal is a parsed decimal literal:
//
//	sign * coeff * 10^exp
//
// where coeff holds the significant decimal digits with leading and trailing
// zeros removed (so len(coeff) is the significant-digit count). exp is the
// decimal exponent of the least significant digit.
type decimal struct {
	negative bool
	coeff    *big.Int
	exp      int // 10 exponent attached to the last digit of coeff
}

// ParseDecimal parses a strict decimal literal.
//
// Accepted grammar (no whitespace, no NaN, no Infinity):
//
//	[+-]? ( [0-9]+ ('.' [0-9]*)? | '.' [0-9]+ ) ( [eE] [+-]? [0-9]{1,2} )?
//
// The number of significant digits must not exceed 30 and the decimal
// exponent of the leading significant digit must be in [-50, 50].
func ParseDecimal(s string) (decimal, bool) {
	if s == "" {
		return decimal{}, false
	}
	negative := false
	switch s[0] {
	case '+', '-':
		negative = s[0] == '-'
		s = s[1:]
	}
	if s == "" {
		return decimal{}, false
	}

	// Split off optional exponent.
	mant := s
	rawExp := ""
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mant = s[:i]
		rawExp = s[i+1:]
		if rawExp == "" || strings.IndexAny(rawExp, "eE") >= 0 {
			return decimal{}, false // trailing e/E or more than one e/E
		}
	}

	if strings.Count(mant, ".") > 1 {
		return decimal{}, false
	}

	intPart, fracPart, hasDot := strings.Cut(mant, ".")
	if intPart == "" && (!hasDot || fracPart == "") {
		return decimal{}, false // ".", "" or ".e3" style
	}
	if !hasDot && intPart == "" {
		return decimal{}, false
	}
	for _, r := range intPart + fracPart {
		if r < '0' || r > '9' {
			return decimal{}, false
		}
	}

	exp, ok := parseExponent(rawExp)
	if !ok {
		return decimal{}, false
	}

	digits := intPart + fracPart
	withoutLeading := strings.TrimLeft(digits, "0")
	// Significant digits exclude both leading zeros and trailing zeros, so
	// "0.0012300" has the same three significant digits as "123".
	significant := strings.TrimRight(withoutLeading, "0")
	if significant == "" {
		// Signed zero of any shape is accepted with value zero. The explicit
		// exponent still has to satisfy its own bound.
		return decimal{negative: negative, coeff: new(big.Int), exp: 0}, true
	}
	if len(significant) > MaxSigDigits {
		return decimal{}, false
	}
	coeff, ok := new(big.Int).SetString(significant, 10)
	if !ok {
		return decimal{}, false
	}

	// Before stripping, the last digit has weight 10^(exp-fracDigits). Each
	// removed trailing zero moves the retained last digit up one exponent.
	exp += len(withoutLeading) - len(significant) - len(fracPart)

	// Decimal exponent of the leading significant digit.
	leadExp := exp + len(significant) - 1
	if leadExp < MinDecExp || leadExp > MaxDecExp {
		return decimal{}, false
	}
	return decimal{negative: negative, coeff: coeff, exp: exp}, true
}

// parseExponent parses [+-]? [0-9]{1,2} and enforces the explicit exponent
// bound.
func parseExponent(s string) (int, bool) {
	if s == "" {
		return 0, true
	}
	negative := false
	if s[0] == '+' || s[0] == '-' {
		negative = s[0] == '-'
		s = s[1:]
	}
	if len(s) < 1 || len(s) > 2 {
		return 0, false
	}
	exp := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		exp = exp*10 + int(c-'0')
	}
	if negative {
		exp = -exp
	}
	if exp < MinDecExp || exp > MaxDecExp {
		return 0, false
	}
	return exp, true
}

// Rat returns sign * coeff * 10^exp exactly.
func (d decimal) Rat() *big.Rat {
	r := new(big.Rat).SetInt(d.coeff)
	if d.exp >= 0 {
		r.Mul(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(d.exp)), nil)))
	} else {
		den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-int64(d.exp))), nil)
		r.Quo(r, new(big.Rat).SetInt(den))
	}
	if d.negative {
		r.Neg(r)
	}
	return r
}

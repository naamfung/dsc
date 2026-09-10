package reader

import (
	"errors"
	"math/big"
	"strings"

	"github.com/jig/scanner"

	"github.com/jig/lisp/lisperror"
	. "github.com/jig/lisp/types"
)

// mergeFractionTokens rewrites the token stream produced by the scanner so that
// a no-whitespace adjacent "3/4" (scanned as Int("3") + Ident("/4")) becomes a
// single fractional token "3/4". The scanner keeps writing "/4" as an identifier
// because '/' is a legal identifier rune, so without this pass "3/4" would be
// misparsed as a symbol '/' on a list starting with 3.
//
// Only pure positive-integer numerators/denominators are merged, mirroring the
// no-whitespace grammar of Scheme-style fraction literals; negative fractions
// are written as (/ -3 4).
func mergeFractionTokens(tokens []Token) []Token {
	out := make([]Token, 0, len(tokens))
	for i := 0; i < len(tokens); i++ {
		cur := tokens[i]
		if cur.Type == scanner.Int && plainDigits(cur.Value) &&
			i+1 < len(tokens) && isFractionTail(tokens[i+1]) &&
			adjacentTokens(cur.Cursor, tokens[i+1].Cursor, tokens[i+1].Value) {
			nxt := tokens[i+1]
			out = append(out, Token{
				Value: cur.Value + nxt.Value,
				Type:  scanner.Int,
				Cursor: Position{
					Module:   cur.Cursor.Module,
					BeginRow: cur.Cursor.BeginRow,
					BeginCol: cur.Cursor.BeginCol,
					Row:      nxt.Cursor.Row,
					Col:      nxt.Cursor.Col,
				},
			})
			i++
			continue
		}
		out = append(out, cur)
	}
	return out
}

// plainDigits reports whether s is a non-empty string of decimal digits,
// optionally preceded by a minus sign (fraction numerators may be negative).
func plainDigits(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '-' {
		i = 1
		if i == len(s) {
			return false
		}
	}
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// isFractionTail reports whether the token is an identifier of the form "/<digits>".
func isFractionTail(nxt Token) bool {
	if nxt.Type != scanner.Ident {
		return false
	}
	v := nxt.Value
	if len(v) < 2 || v[0] != '/' {
		return false
	}
	return plainDigits(v[1:])
}

// adjacentTokens reports whether two tokens appear on the same line with no
// whitespace between them. tokenize records BeginCol as the position just after
// the token, so a and b are adjacent iff b's end column equals a's end column
// plus the length of b's text.
func adjacentTokens(a, b Position, bValue string) bool {
	return a.BeginRow == b.BeginRow && a.BeginCol+len(bValue) == b.BeginCol
}

// parseRatLiteral parses a fraction literal "num/den" into an exact *Rat.
// Both parts must be plain decimal digits and the denominator must be non-zero.
func parseRatLiteral(s string, cursor *Position) (MalType, error) {
	parts := strings.SplitN(s, "/", 2)
	num, ok := new(big.Int).SetString(parts[0], 10)
	if !ok {
		return nil, lisperror.NewLispError(errors.New("invalid fraction numerator: "+parts[0]), cursor)
	}
	if len(parts) != 2 {
		return nil, lisperror.NewLispError(errors.New("invalid fraction literal: "+s), cursor)
	}
	den, ok := new(big.Int).SetString(parts[1], 10)
	if !ok {
		return nil, lisperror.NewLispError(errors.New("invalid fraction denominator: "+parts[1]), cursor)
	}
	if den.Sign() == 0 {
		return nil, lisperror.NewLispError(errors.New("division by zero in literal: "+s), cursor)
	}
	return &Rat{R: new(big.Rat).SetFrac(num, den)}, nil
}

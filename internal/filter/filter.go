package filter

import (
	"fmt"
	"strings"
)

// The server's filter DSL (admiral/internal/querybuilder/parser.peg) reads
// single-quoted strings as: "'" ( "\'" / !"'" . )* "'". The only escape is
// \' for a literal quote; a backslash before anything else is itself
// literal. Two consequences drive the helpers below: every user-supplied
// value must have its quotes escaped or it can break out of the literal,
// and a value ending in a backslash cannot be expressed at all, because
// its closing "\'" would read as an escaped quote.

// Quote renders v as a single-quoted DSL string literal.
func Quote(v string) (string, error) {
	if strings.HasSuffix(v, `\`) {
		return "", fmt.Errorf("value %q cannot be used in a filter: it ends with a backslash", v)
	}
	return "'" + strings.ReplaceAll(v, "'", `\'`) + "'", nil
}

// Eq builds a single equality predicate, field['<field>'] = '<value>',
// with both sides quoted for the DSL. field is normally a constant but is
// quoted anyway, since label keys are user-supplied.
func Eq(field, value string) (string, error) {
	f, err := Quote(field)
	if err != nil {
		return "", err
	}
	v, err := Quote(value)
	if err != nil {
		return "", err
	}
	return "field[" + f + "] = " + v, nil
}

// And joins predicates with AND, skipping empty ones.
func And(preds ...string) string {
	out := preds[:0:0]
	for _, p := range preds {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " AND ")
}

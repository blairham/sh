// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// Dialect.ArithUnbalancedBracketIsRefused: a `))` that arrives with a `[`
// the source opened still open is `)' unexpected, at the arithmetic command
// and at the substitution, quoted or not. Measured 2026-10-03 on ksh93u+.
// Balanced brackets, a bracket inside quotes, and a group that holds one are
// the controls, and without the flag every refused row parses but the last,
// whose stray `]))` no reading takes.
func TestAnArithmeticCloserInsideAnOpenBracket(t *testing.T) {
	t.Parallel()
	refusing := syntax.Core()
	refusing.ArithUnbalancedBracketIsRefused = true
	for _, src := range []string{
		"(( a[1 ))",
		"echo $(( a[1 ))",
		`echo "$(( a[$k ))"`,
		"echo $(( 1 + [ ))",
		"echo $(( a[1 )) ]))",
	} {
		_, err := syntax.Parse(src, refusing)
		var se *syntax.Error
		if !errors.As(err, &se) || se.Kind != syntax.ErrUnexpected || se.Token != ")" {
			t.Errorf("%s: err %v, want `)' unexpected", src, err)
		}
		if _, err := syntax.Parse(src, syntax.Core()); err != nil && src != "echo $(( a[1 )) ]))" {
			t.Errorf("%s without the flag: %v, want it parsed", src, err)
		}
	}
	for _, src := range []string{
		"(( a[1] ))",
		"echo $(( a[(1)] ))",
		`echo $(( a["["] ))`,
		"echo $(( a[1] + b[2] ))",
	} {
		if _, err := syntax.Parse(src, refusing); err != nil {
			t.Errorf("%s: %v, want it parsed", src, err)
		}
	}
}

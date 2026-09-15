// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A `$'…'` written as the subscript of an assignment decodes its escapes, the
// way it does anywhere else a word is expanded.
//
// It did not. The shortcut in expandKeyQuoted takes a word that is one literal
// span as its own text, and a `$'…'` is a literal span whose value still holds
// the source of its escapes — the decoding is the expansion's. So
// `m[$'\t']=v` stored a key spelled backslash-then-t: two characters,
// reachable only by writing the same escape again, and invisible to `${m[$k]}`
// for a `k` holding the character the script meant. Status 0 throughout, which
// is what made it a table with a key nobody could name (#2749).
func TestADollarSingleQuotedSubscriptDecodesItsEscapes(t *testing.T) {
	for _, tc := range []struct{ why, src, want string }{
		{
			"the key is the character, not the escape",
			`typeset -A m; m[$'\t']=v; for k in "${!m[@]}"; do printf "len=%d" "${#k}"; done`,
			"len=1",
		},
		{
			// The half a length alone cannot see: the same character
			// arriving through a variable has to find the key already there.
			"a variable holding the character reaches the same key",
			`typeset -A m; m[$'\t']=v; k=$'\t'; printf "[%s]" "${m[$k]}"`,
			"[v]",
		},
		{
			"and a multi-character escape sequence likewise",
			`typeset -A m; m[$'a\nb']=v; for k in "${!m[@]}"; do printf "len=%d" "${#k}"; done`,
			"len=3",
		},
		{
			// The control that keeps the shortcut doing its job: the other
			// quoting kinds really are their text, and a double-quoted
			// backslash that escapes nothing stays a backslash.
			"a single-quoted subscript keeps its backslash",
			`typeset -A m; m['a\nb']=v; for k in "${!m[@]}"; do printf "len=%d" "${#k}"; done`,
			"len=4",
		},
		{
			"and a double-quoted one does too",
			`typeset -A m; m["a\nb"]=v; for k in "${!m[@]}"; do printf "len=%d" "${#k}"; done`,
			"len=4",
		},
	} {
		sem := permissive()
		sem.SubscriptIsAQuotingContext = Yes
		out, st := runGrammar(t, tc.src, func(d *syntax.Dialect) {
			d.ParamIndirection = true
			d.DollarSingleQuote = true
		}, withSem(sem))
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s: %s gave %q, want %q", tc.why, tc.src, strings.TrimSpace(out), tc.want)
		}
		if st != 0 {
			t.Errorf("%s: status %d, want 0", tc.why, st)
		}
	}
}

// The two words that mean the *whole array* in brackets are listed quoted,
// where every other ordinary character in a key is listed bare.
//
// A listing is meant to be read back, and `[@]=at` read back is the
// whole-array subscript rather than the key the table is holding. They were
// listed bare, so the output of `typeset -p` named a subscript the table did
// not have (#2749).
func TestAWholeArraySubscriptIsQuotedAsAKey(t *testing.T) {
	sem := permissive()
	sem.DeclareValueQuoting = ListingQuoteAlwaysDouble
	sem.DeclareListing = DeclareListingClustered
	// The one column that will store such a key at all reads the spelling as
	// an ordinary key, which is what makes the listing question askable.
	sem.WholeArraySubscriptAssigningATable = WholeArraySubscriptIsAnOrdinaryKey
	for _, tc := range []struct{ src, want string }{
		{`typeset -A m; m[@]=at; typeset -p m`, `declare -A m=(["@"]="at" )`},
		{`typeset -A m; m[*]=st; typeset -p m`, `declare -A m=(["*"]="st" )`},
		// The whole key and not a character in it. `a@b` is ordinary and
		// stays bare; a rule written about the character would quote it too.
		{`typeset -A m; m[a@b]=x; typeset -p m`, `declare -A m=([a@b]="x" )`},
	} {
		out, st := run(t, tc.src, withSem(sem))
		if strings.TrimSpace(out) != tc.want {
			t.Errorf("%s gave %q, want %q", tc.src, strings.TrimSpace(out), tc.want)
		}
		if st != 0 {
			t.Errorf("%s: status %d, want 0", tc.src, st)
		}
	}
}

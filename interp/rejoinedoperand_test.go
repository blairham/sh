// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A utility that reads its own `name=( … )` operand is handed the assignment
// written out again — as **words**, because the text is an ordinary word and
// an ordinary word is field-split.
//
// The literal halves of the assignment are not split points and the fields an
// expansion produced are, so the trailing `)` of the last element joins the
// last field rather than standing alone. See interp/rejoinedoperand.go for
// the panel this was measured against.
func TestARejoinedArrayOperandIsTheFieldsOfItsExpansions(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		// One field each, so the rejoin is one word.
		{"a quoted element holds its blank", `x="p q"; show a=("p q" r)`, `<a=(p q r)>`},
		// The break the expansion made, and nothing else.
		{"an expansion that split", `x="p q"; show a=($x)`, `<a=(p><q)>`},
		// The discriminator: ` r)` is the assignment's own text, so it is
		// not a split point and joins the expansion's last field. A rule
		// that split one word per element would give three words here.
		{"literal text after it joins the last field", `x="p q"; show a=($x r)`, `<a=(p><q r)>`},
		{"and before it joins the first", `x="p q"; show a=(r $x)`, `<a=(r p><q)>`},
		{"two elements that both split", `x="p q"; y="s t"; show a=($x $y)`, `<a=(p><q s><t)>`},
		// The separator is written per element, not before each field: an
		// element that expanded to nothing still had a blank after it.
		{"a separator with no field beside it", `x=""; show a=($x r)`, `<a=( r)>`},
		{"an element list that came to nothing", `x=""; show a=($x)`, `<a=()>`},
		{"no elements at all", `show a=()`, `<a=()>`},
		{"the appending spelling", `x="p q"; show a+=($x r)`, `<a+=(p><q r)>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			src := `show() { printf "<%s>" "$@"; echo; }; ` + c.src
			out, st := runGrammar(t, src, func(d *syntax.Dialect) {
				d.ArrayLiteral = true
				d.DeclarationUtilities = map[string]bool{"show": true}
			}, func(r *Runner) {
				r.RejoinArrayOperand("show")
			})
			if got := strings.TrimSpace(out); got != c.want {
				t.Errorf("words %s, want %s", got, c.want)
			}
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
		})
	}
}

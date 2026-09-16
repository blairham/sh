// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// One shell, one single-quoting rule, and this column held it in four places
// with one of them wrong.
//
// dash writes every listed value single-quoted and spells an embedded quote
// by closing, doubling and reopening — `'a'"'"'b'`. It has to: there is no
// `$'…'` here and a backslash inside single quotes is a backslash, so the
// escaped spelling — quote, a, quote, backslash, quote, quote, b, quote —
// reads back as five characters, and a listing that cannot be read back is
// not a listing. (It is written out in the assertion below rather than here,
// because gofmt rewrites a doubled apostrophe in a doc comment into a
// typographic quote.) AliasQuoting, TrapQuoting and
// SetListingQuoting all said so; DeclareValueQuoting, which is what `export
// -p` and `readonly -p` go through, said the opposite.
//
// Measured 2026-09-16 under `env -i PATH=/usr/bin:/bin LC_ALL=C`, on Apple's
// dash-16 (macOS /bin/dash) and on upstream dash 0.5.12 built from source on
// the same machine — byte-identical on both:
//
//	v="a'b"; export v; export -p       export v='a'"'"'b'
//	r="a'b"; readonly r; readonly -p   readonly r='a'"'"'b'
//	set                                v='a'"'"'b'
//
// The bare `set` row is the control and it was already right, which is the
// whole shape of the defect: the two spellings sat in one file, three lines
// apart, and nothing asked them the same question until
// share/suite/dash/variables.tests did.
func TestTheTwoListingsQuoteAnEmbeddedQuoteTheWayABareSetDoes(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"export -p", `v="a'b"; export v; export -p`, `export v='a'"'"'b'`},
		{"readonly -p", `r="a'b"; readonly r; readonly -p`, `readonly r='a'"'"'b'`},
		{"bare set", `v="a'b"; set`, `v='a'"'"'b'`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{
				Name: "dash", Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
			}, c.src)
			if err != nil {
				t.Fatalf("run %q: %v", c.src, err)
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("got %q, want a line %q", strings.TrimSpace(out), c.want)
			}
			// The escaped spelling is the one this column wrote before, and
			// naming it here is what makes the assertion above about dash's
			// rule rather than about any quoting at all.
			if strings.Contains(out, `'\''`) {
				t.Errorf("got %q — that is the escaped spelling, which this shell "+
					"cannot read back: a backslash inside single quotes is a backslash", out)
			}
		})
	}
}

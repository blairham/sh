// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/dialect/ash"
)

// A backslash inside a bracket expression, which is a member here and an
// escape in the other six columns (#3271).
//
// The axis had two values and this shell holds a third. It is not "the
// escape is refused" and it is not zsh's "the escape protects *and* joins the
// set": the backslash joins the set and protects nothing, so the character
// behind it keeps whatever meaning it has inside a bracket — including being
// the range operator.
//
// Measured 2026-09-16 against BusyBox v1.37.0 in the digest-pinned Alpine
// image internal/oracle reaches, under `env -i PATH=/usr/bin:/bin LC_ALL=C`,
// `--init`, and stdin from /dev/null.
func TestABackslashInABracketIsAMember(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The row the issue was filed on. `[\]` is the one-character
			// set holding a backslash and the second `]` is a literal, so
			// the pattern wants four characters and `a]c` is three.
			"an escaped terminator does not admit the terminator",
			`case 'a]c' in a[\]]c) echo yes;; *) echo no;; esac`,
			"no\n",
		},
		{
			// And the subject that reading *does* admit. The other six
			// columns answer no here, which is what makes this row and the
			// one above a pair rather than one assertion twice.
			"it admits a backslash followed by the terminator",
			`case 'a\]c' in a[\]]c) echo yes;; *) echo no;; esac`,
			"yes\n",
		},
		{
			// The range. Under the escaping reading `[a\-z]` is the three
			// members a, `-` and z; here it is `a` together with the range
			// from `\` — 0x5c — to `z`, so a `-` at 0x2d is outside it.
			"an escaped dash is still the range operator",
			`case 'a-c' in a[a\-z]c) echo yes;; *) echo no;; esac`,
			"no\n",
		},
		{
			// 0x62 is inside 0x5c..0x7a, so the range is real rather than
			// the bracket having been rejected. A shell that refused the
			// pattern would answer no to this and to the row below, which
			// is the reading these two rule out.
			"and the range it opens is live",
			`case 'abc' in a[a\-z]c) echo yes;; *) echo no;; esac`,
			"yes\n",
		},
		{
			"whose left bound is the backslash itself",
			`case 'a\c' in a[a\-z]c) echo yes;; *) echo no;; esac`,
			"yes\n",
		},
		{
			// The control, and it is unanimous across all seven columns:
			// outside a bracket the escape is spent here too. Without it
			// every row above would also be satisfied by a shell that had
			// simply stopped reading backslashes in patterns.
			"outside a bracket the escape is spent",
			`case 'beta' in bet\a) echo yes;; *) echo no;; esac`,
			"yes\n",
		},
		{
			// A second control, at the same site and about the bracket
			// rather than the backslash: an ordinary set still works.
			"an ordinary bracket is unaffected",
			`case 'abc' in a[b]c) echo yes;; *) echo no;; esac`,
			"yes\n",
		},
		{
			// The mark our own pipeline puts on quoted text. `escapePattern-
			// Meta` marks every character that is a metacharacter in *some*
			// dialect, and under this reading a surplus mark is a surplus
			// member — so the set it writes is narrowed to what this
			// dialect reads. A `(` is ordinary here, so no mark and no
			// backslash in the set.
			"a quoted parenthesis admits no backslash",
			`case 'a\c' in a["("]c) echo yes;; *) echo no;; esac`,
			"no\n",
		},
		{
			"and the parenthesis itself is the member",
			`case 'a(c' in a["("]c) echo yes;; *) echo no;; esac`,
			"yes\n",
		},
		{
			// A quoted *metacharacter* is marked, here as in every dialect,
			// and the mark is then a member. Measured: BusyBox answers yes
			// to both of these.
			"a quoted star admits the backslash as well",
			`case 'a\c' in a["*"]c) echo yes;; *) echo no;; esac`,
			"yes\n",
		},
		{
			"and the star is a member too",
			`case 'a*c' in a["*"]c) echo yes;; *) echo no;; esac`,
			"yes\n",
		},
		{
			// The other route a bracket can arrive on. A value's backslash
			// was never a mark, so it is a member before *any* character —
			// where the source route above has already spent it before an
			// ordinary one.
			"a backslash from a value is a member before an ordinary character",
			`p='a[\z]c'; case 'a\c' in $p) echo yes;; *) echo no;; esac`,
			"yes\n",
		},
		{
			"and the source route has spent that one",
			`case 'a\c' in a[\z]c) echo yes;; *) echo no;; esac`,
			"no\n",
		},
		{
			"where both routes still admit the character behind it",
			`case 'azc' in a[\z]c) echo yes;; *) echo no;; esac`,
			"yes\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runIn(t, tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// And the value itself, so a preset mutant dies here even where a row above
// could be satisfied by some other change.
func TestBracketEscapeAnswer(t *testing.T) {
	if got, want := ash.Semantics().BracketEscape, interp.BracketEscapeIsOnlyAMember; got != want {
		t.Errorf("BracketEscape = %v, want %v", got, want)
	}
}

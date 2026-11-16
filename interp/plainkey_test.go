// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// plainKeyScript puts a subscript of one bare `$name` in front of every state
// the single read must either answer exactly as the two readings do or leave
// to them: keys read and written, in and out of double quotes, through a
// positional parameter, a function's local and a nested subscript, under an
// IFS holding the key's own letters, and as a search flag's argument; then
// values the single read declines — a space, a glob character, quotes, a
// backslash, brackets, a tilde, an empty or unset name, `set -u`, a produced
// parameter, an array and a reference.
//
// Shapes a vector refuses are run through `command eval`, so a refusal is
// part of the output both runs must agree on and the script carries on.
const plainKeyScript = `p() { printf '[%s]' "$@"; echo; }
typeset -A m n
m[a]=1; m[b.c]=2; m[1]=pos; m[x/y]=3; m[k-+:,]=4
k=a; p "${m[$k]}" ${m[$k]} "${m[${k}]}"
k=b.c; p "${m[$k]}"; m[$k]=22; p "${m[b.c]}"
k=x/y; m[$k]+=9; p "${m[x/y]}"
k=k-+:,; p "${m[$k]}"
k=nosuch; p "${m[$k]}" "${#m[$k]}"
set -- 1 b.c; p "${m[$1]}" "${m[$2]}"; command eval 'p "${m[$3]}"'; echo "<$?>"
f() { local k=a; p "${m[$k]}"; m[$k]=fa; }; f; p "${m[a]}"
k=a; n[a]=b.c; p "${m[${n[$k]}]}"
IFS=a.; k=a; p "${m[$k]}"; k=b.c; p "${m[$k]}"; unset IFS
k=a; command eval 'p "${m[(k)$k]}" "${m[(r)$k]}" "${m[(i)$k]}"'
k='a b'; m[$k]=sp; p "${m[$k]}" "${m[a b]}"
k='*'; m[$k]=star; p "${m[$k]}" "${m[x]}"
k='a"b'; command eval 'm[$k]=q; p "${m[$k]}" "${m[a\"b]}"'
k="a'b"; command eval 'm[$k]=s; p "${m[$k]}"'
k='a\b'; command eval 'm[$k]=bs; p "${m[$k]}"'
k='[x]'; m[$k]=br; p "${m[$k]}"; k='x]'; m[$k]=cb; p "${m[$k]}"
k='~x'; m[$k]=ti; p "${m[$k]}"
k=; command eval 'm[$k]=e; p "${m[$k]}"'; echo "<$?>"
unset k; command eval 'p "${m[$k]}"'; echo "<$?>"
(set -u; p "${m[$nosuch]}"); echo "<$?>"
m[1]=c1; m[11]=c2; p "${m[$COUNT]}" "${m[$COUNT]}"
command eval 'a=(a b.c)'; m['a b.c']=whole; p "${m[$a]}"
k=a; command eval 'typeset -n ref=k'; command eval 'p "${m[$ref]}"'
k=a; p "${m["$k"]}" "${m[${k}x]}" "${m[$k$k]}" "${m[${k-z}]}"
echo key-end
`

// plainKeyVectors are the answers the script is run under: a subscript as a
// quoting context and not, which are the two readings the full road compares;
// the question left open, so a key whose readings differ refuses; and an
// array name read as the whole list.
func plainKeyVectors() map[string]Semantics {
	base := testSemantics()
	base.DeclareOptions = "aAilnprux"
	quoting := base
	quoting.SubscriptIsAQuotingContext = Yes
	asWritten := base
	asWritten.SubscriptIsAQuotingContext = No
	open := base
	open.SubscriptIsAQuotingContext = Unspecified
	// A bare array name read as the whole list, so that `${m[$a]}` is a key
	// the single read would get wrong if it took an array for a scalar.
	whole := quoting
	whole.ArrayScalarIsTheWholeArray = Yes
	whole.ArrayNameWithoutSubscriptIsTheList = Yes
	return map[string]Semantics{
		"quoting context": quoting,
		"as written":      asWritten,
		"left open":       open,
		"whole array":     whole,
	}
}

// plainKeyRun runs plainKeyScript under sem with the single read held on or
// off, with subscript flags in the grammar where flags says so, and returns
// what it printed, its status and how many keys came by the single read.
func plainKeyRun(t *testing.T, sem Semantics, flags, on bool) (string, int, int) {
	t.Helper()
	d := syntax.Core()
	d.ArraySubscriptFlags = flags
	f, err := syntax.Parse(plainKeyScript, d)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	was := SetPlainKeyShortcutForTest(on)
	defer SetPlainKeyShortcutForTest(was)
	var out strings.Builder
	r := plainParamRunner(t, sem, &out)
	r.Dialect = &d
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		return out.String() + "unsupported: " + rerr.Error(), -1, PlainKeyReadsForTest(r)
	}
	return out.String(), st, PlainKeyReadsForTest(r)
}

// TestAPlainKeyAnswersAsBothReadingsDo runs plainKeyScript with the single
// read of a subscript of one bare `$name` on and off, under each vector with
// and without subscript flags, and requires byte-identical output and status
// (#6423). It also requires the single read to be taken with it on and never
// with it off, and the script to have reached the rows the comparison leans
// on.
//
// The switch is a package variable and these rows hold it, so they run one
// after another rather than side by side.
func TestAPlainKeyAnswersAsBothReadingsDo(t *testing.T) {
	for name, sem := range plainKeyVectors() {
		for _, flags := range []bool{false, true} {
			label := name
			if flags {
				label += ", subscript flags"
			}
			t.Run(label, func(t *testing.T) {
				full, fullStatus, fullReads := plainKeyRun(t, sem, flags, false)
				short, shortStatus, reads := plainKeyRun(t, sem, flags, true)
				if short != full || shortStatus != fullStatus {
					t.Errorf("with the single read:\n%s(status %d)\nwithout it:\n%s(status %d)", short, shortStatus, full, fullStatus)
				}
				if fullReads != 0 {
					t.Errorf("the single read was taken %d times with it off; want none", fullReads)
				}
				// Measured per line: the single read is taken 18 times
				// without subscript flags and 17 with them, where the nested
				// subscript's inner key is read once rather than three times
				// and the `(k)` argument adds one.
				if reads < 17 {
					t.Errorf("the single read was taken %d times; want at least 17", reads)
				}
				for _, row := range []string{"[1][1][1]\n", "[22]\n", "[39]\n", "[4]\n", "[pos][22]\n", "[fa]\n", "[sp][sp]\n", "[br]\n", "[ti]\n", "key-end\n"} {
					if !strings.Contains(full, row) {
						t.Errorf("the script did not reach %q:\n%s", row, full)
					}
				}
				if name == "whole array" && !strings.Contains(full, "[whole]\n") {
					t.Errorf("an array key was not read as the whole list:\n%s", full)
				}
			})
		}
	}
}

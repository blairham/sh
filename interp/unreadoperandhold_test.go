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

// unreadHoldScript declares operands in a `function` body where nothing behind
// them reads the hold, beside every shape that must keep it: a later operand
// reading the name, a substitution, an array literal, a tilde, a word split by
// the IFS held, a prefix, a trace whose PS4 reads it, a POSIX-form body that
// makes no locals, and the top level, where a walk stopped by a frozen name
// keeps what it held for the EXIT trap. Two walks stop inside a body as well,
// on a frozen name and on an unset positional under `set -u`.
const unreadHoldScript = `set -- x y
a=ga b=gb c=gc
function decl3 { typeset a=$1 b=$2 c; c=$a$b; echo "[$a][$b][$c]"; }
decl3 p q; echo "out [$a][$b][$c]"
function twice { typeset a=$1 a=$2; echo "[$a]"; }; twice p q; echo "out [$a]"
function quoted { typeset a=$1 "b=$2" 'c=$a' d="$1"; echo "[$a][$b][$c][$d]"; }; quoted p q
function reads { typeset a=$1 b=$a; echo "[$b]"; }; reads p; echo "out [$a][$b]"
function again { typeset a=o; typeset a=$1 b=$2; echo "[$a][$b]"; }; again p q; echo "out [$a]"
function ints { typeset -i a=$1 b=$2; echo "$((a+b))"; }; ints 3 4
function exported { typeset -x a=$1; echo "[$a]"; }; exported p
function redirected { typeset a=$1 b=$2 >/dev/null 2>&1; echo "[$a][$b]"; }; redirected p q
readonly ro=r
function frozen { typeset a=$1 ro=$2 c=z; echo "[$a][$ro][$c]"; }; (frozen p q); echo "frozen $? [$a][$c]"
function unsetpos { set -u; typeset a=$1 b=$3; echo "[$a]"; }; (unsetpos p); echo "unset $? [$a]"
posixf() { typeset a=$1 b=$2; }; posixf p q; echo "posix [$a][$b]"
(trap 'echo "trap [$a][$c]"' EXIT; typeset a=$1 ro=$2 c=z); echo "top stop $?"
function prefixed { v=$a typeset a=$1 b=$2; echo "[$v][$a][$b]"; }; prefixed p q
function nested { typeset a=$1 b=$(echo $a); echo "[$b]"; }; nested p
function lit { typeset a=$1 b=($a); echo "[$b]"; }; lit p
HOME=/h; function home { typeset HOME=$1 d=~; echo "[$d]"; }; home /p
function split { typeset IFS=: $1; echo "[$m][$n]"; }; split m:n
PS4='+[$a] '
function traced { set -x; typeset a=$1 b=$2; set +x; }; traced p q 2>&1
typeset a=$1 b=$2; echo "top [$a][$b]"
`

// unreadHoldRows are lines unreadHoldScript writes from its first call to its
// last, which the run with every operand held must reach for the comparison
// to have compared the whole script.
var unreadHoldRows = []string{"[p][q][pq]\nout [ga][gb][gc]\n", "[q]\nout [ga]\n", "[p]\nout [ga][gb]\n", "7\n", "top [x][y]\n"}

// TestAnUnreadHoldAnswersAsTheHeldOneDoes runs unreadHoldScript with the route
// that leaves out an operand's hold on and off, in the column that stores a
// declaration's operands in turn, and requires byte-identical output and
// status, the route taken with it on and never with it off (#6373).
func TestAnUnreadHoldAnswersAsTheHeldOneDoes(t *testing.T) {
	prog, err := syntax.Parse(unreadHoldScript, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	base := emptyShadowVectors()["base"]
	base.DeclarationOperandsAreStoredInTurn = Yes
	base.DeclareOptions = "aAgilnprux"
	base.TypesetLocalNeedsKeywordFunction = Yes
	shadows := base
	shadows.DeclarationMayShadowAReadonly = Yes
	everyBody := base
	everyBody.TypesetLocalNeedsKeywordFunction = No
	for name, sem := range map[string]Semantics{
		"keyword bodies": base, "shadows readonly": shadows, "every body": everyBody,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			run := func(on bool) (string, int, int) {
				was := SetUnreadHoldShortcutForTest(on)
				defer SetUnreadHoldShortcutForTest(was)
				var out strings.Builder
				dg := PosixDiagnostics()
				r := newTestRunner(t, &Runner{
					Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &dg, Name: "testsh", Dir: dir,
					Env: []string{"PATH=/usr/bin:/bin"},
				})
				// PS4 expanded, so a trace line reads the hold.
				r.SetPromptStyle(PromptStyle{Expand: PromptExpandsAlways})
				st, err := r.Run(context.Background(), prog)
				if err != nil {
					t.Fatalf("route %v: %v", on, err)
				}
				return out.String(), st, UnreadHoldsForTest(r)
			}
			full, fullStatus, fullTaken := run(false)
			short, shortStatus, taken := run(true)
			if short != full || shortStatus != fullStatus {
				t.Errorf("with the route:\n%s(status %d)\nwithout it:\n%s(status %d)", short, shortStatus, full, fullStatus)
			}
			if fullTaken != 0 {
				t.Errorf("the route left out %d holds with it off; want none", fullTaken)
			}
			if taken < 12 {
				t.Errorf("the route left out %d holds; want at least 12", taken)
			}
			for _, row := range unreadHoldRows {
				if !strings.Contains(full, row) {
					t.Errorf("the script did not reach %q:\n%s", row, full)
				}
			}
		})
	}
}

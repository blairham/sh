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

// emptyShadowScript declares `local` operands over every state the route for
// a name with nothing to shadow must either answer exactly as the full road
// does or leave to it: fresh names with and without a value, outer names that
// are set, exported, typed, frozen, compound, produced, tied, in the
// environment or the shell's own, `set -a`, a redeclaration on the same line
// and on a later one, a name the body then retypes, exports and freezes, a
// name `unset` inside the scope, an array literal and an append, a prefix
// holding the name, nested calls declaring the same name, a second call
// reusing the scope's records, and the return putting every one of them back.
// A reference comes last and in a subshell, because one standing anywhere
// turns the route off.
//
// Shapes a vector may refuse are run through `command eval`, so a refusal is
// part of the output both runs must agree on and the script carries on.
const emptyShadowScript = `p() { printf '[%s]' "$@"; echo; }
show() { for n; do typeset -p "$n" 2>&1 || echo "$n:absent"; done; }
ov=outer; ex=exp; export ex; typeset -i ti=3; typeset -u tu=abc; command eval 'ta=(1 2)'
command eval 'readonly ro=r'; typeset -l tl=XY
f() {
  local a b=$1 c=lit d
  p "<${a-unset}>" "<${b-unset}>" "<$c>" "<${d-unset}>"
  show a b c d
  local ov ex ti tu tl ta BORN COUNT LIST TIED tied IFS OPTIND
  p "<${ov-unset}>" "<${ex-unset}>" "<${ti-unset}>" "<${tu-unset}>" "<${ta-unset}>" "<${BORN-unset}>" "<${TIED-unset}>"
  show ov ex ti tu tl ta BORN
  command eval 'local ro'; echo "<ro $?>"
  a=set; command eval 'a=(x y)'; command eval 'typeset -i d; d=2+3'; export b; command eval 'readonly c'
  local a; local b=again; local e e=twice
  show a b c d e
  case "$(export -p)" in *" b="*) echo b-exported;; *) echo b-not;; esac
  set -a; local sa=1 sb; set +a
  case "$(export -p)" in *sa=*) echo sa-exported;; *) echo sa-not;; esac
  command eval 'local arr=(1 2)'; local app+=x; show arr app
  g
  p "<${gi-unset}>"
}
g() { local gi=inner a; p "<$a>" "<$gi>"; h; }
h() { local hz; hz=deep; p "$hz"; command eval 'typeset -u hz; hz=up'; show hz; }
f one; f two
show a b c d e ov ex ti tu tl ta sa sb arr app gi hz
p "<${a-unset}>" "<${c-unset}>" "<${ov-unset}>"
a=after; c=after; p "$a" "$c"
export -p
k() { local x=1; unset x; local x; show x; local y; unset y; y=2; show y; local z=1 z; show z; }; k; show x y z
m() { local q=$1; m2; }; m2() { local q; p "<${q-unset}>"; q=m2; }; m outer; show q
pre() { local pz; show pz; pz=in; }; pz=1 pre; show pz
fi2() { local cnt; cnt=$((cnt + 1)); p "$cnt"; }; fi2; fi2; show cnt
(typeset -n ref=ov; r() { local ov; ov=x; p "$ref"; }; r; p "$ov")
command eval 'local top'; echo "<top $?>"
`

// emptyShadowVectors are the answers the route's store and shadow lean on,
// each moved away from the base: whether a valueless declaration sets the
// name, whether a local inherits the export attribute, whether a valueless
// one hides the outer value, and whether a declaration may shadow a frozen
// name.
func emptyShadowVectors() map[string]Semantics {
	base := plainParamVectors()["first element"]
	base.LocalOptions = "aAilnprux"
	empty := base
	empty.DeclaredNameWithoutValueIsEmpty = Yes
	inherits := base
	inherits.LocalInheritsTheExportAttribute = Yes
	hides := base
	hides.ValuelessDeclarationHidesTheOuterValue = Yes
	shadows := empty
	shadows.DeclarationMayShadowAReadonly = Yes
	return map[string]Semantics{
		"base": base, "empty": empty, "inherits export": inherits, "hides outer": hides,
		"shadows readonly": shadows,
	}
}

// TestAnEmptyShadowAnswersAsTheFullRoadDoes runs emptyShadowScript with the
// route for a `local` operand over a name with nothing to shadow on and off,
// and requires the same output and status (#6373).
func TestAnEmptyShadowAnswersAsTheFullRoadDoes(t *testing.T) {
	f, err := syntax.Parse(emptyShadowScript, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	for name, sem := range emptyShadowVectors() {
		t.Run(name, func(t *testing.T) {
			// One directory and one TMPDIR for both runs, which `export -p`
			// lists.
			dir, tmp := t.TempDir(), t.TempDir()
			run := func(on bool) (string, int, int) {
				was := SetEmptyShadowShortcutForTest(on)
				defer SetEmptyShadowShortcutForTest(was)
				var out strings.Builder
				dg := PosixDiagnostics()
				r := newTestRunner(t, &Runner{
					Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &dg, Name: "testsh", Dir: dir,
					Env: []string{"BORN=env", "PATH=/usr/bin:/bin", "TMPDIR=" + tmp},
				})
				count := 0
				r.SetDynamic("COUNT", func(*Runner) string { count++; return strings.Repeat("1", count) })
				r.SetDynamicArray("LIST", func(*Runner) []string { return []string{"l1", "l2"} })
				r.Tie("TIED", "tied", ":")
				st, err := r.Run(context.Background(), f)
				if err != nil {
					t.Fatalf("route %v: %v", on, err)
				}
				return out.String(), st, EmptyShadowsForTest(r)
			}
			full, fullStatus, fullTaken := run(false)
			short, shortStatus, taken := run(true)
			if short != full || shortStatus != fullStatus {
				t.Errorf("with the route:\n%s(status %d)\nwithout it:\n%s(status %d)", short, shortStatus, full, fullStatus)
			}
			// The comparison compares something: the route answered operands
			// with it on and none with it off.
			if fullTaken != 0 || taken < 20 {
				t.Errorf("the route took %d operands with it on and %d with it off; want at least 20 and 0", taken, fullTaken)
			}
			// And the rows it leans on, so a script that stopped early
			// compares nothing.
			for _, row := range []string{"[<one>][<lit>]", "[<two>][<lit>]", "[deep]\n", "[after][after]\n", "[1]\n[1]\n"} {
				if !strings.Contains(full, row) {
					t.Errorf("the script did not reach %q:\n%s", row, full)
				}
			}
		})
	}
}

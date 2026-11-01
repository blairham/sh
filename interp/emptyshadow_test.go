// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"regexp"
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
ou=outer; k2() { local ou; unset ou; local ou; ou=in; show ou; }; k2; show ou
lt() { local -i li; li=2+3; local -x lx=1; local -r lr=1; local -a la; local -u lu=abc; show li lx lr la lu
  case "$(export -p)" in *lx=*) echo lx-exported;; *) echo lx-not;; esac; }; lt; show li lx lr la lu; lr=w; p "$lr"
ap() { local an+=x; show an; local an+=y; show an; }; ap; show an
tt() { local TIED; TIED=a:b; show TIED tied; local REFUSED=bad; echo "<refused $?>"; }; tt; show TIED tied REFUSED
pe() { local BORN; printenv BORN || echo no-born; BORN=in; printenv BORN || echo no-born; }; pe; printenv BORN
`

// emptyShadowVectors are the answers the route's store and shadow lean on,
// each moved away from the base: whether a valueless declaration sets the
// name, whether a local inherits the export attribute (the base says it
// does), whether a valueless
// one hides the outer value, and whether a declaration may shadow a frozen
// name. The last one is the base with locals inheriting the outer value,
// which the runner is told rather than the vector.
func emptyShadowVectors() map[string]Semantics {
	base := plainParamVectors()["first element"]
	base.LocalOptions = "aAilnprux"
	base.DeclarationTakesAnAppendOperand = Yes
	empty := base
	empty.DeclaredNameWithoutValueIsEmpty = Yes
	drops := base
	drops.LocalInheritsTheExportAttribute = No
	hides := base
	hides.ValuelessDeclarationHidesTheOuterValue = Yes
	shadows := empty
	shadows.DeclarationMayShadowAReadonly = Yes
	return map[string]Semantics{
		"base": base, "empty": empty, "drops export": drops, "hides outer": hides,
		"shadows readonly": shadows, "inherits outer": base,
	}
}

// emptyShadowRun runs prog under sem with the route on or off, and gives back
// the output, the status and how many operands took the route. Both runs share
// one directory and one TMPDIR, which `export -p` lists.
func emptyShadowRun(
	t *testing.T, prog *syntax.File, sem Semantics, inheritsOuter bool,
) func(on bool) (string, int, int) {
	t.Helper()
	dir, tmp := t.TempDir(), t.TempDir()
	return func(on bool) (string, int, int) {
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
		r.RefuseValues("REFUSED", func(v string) (string, bool) { return "refused " + v, v == "bad" })
		// `declare` as `typeset` under a second name, which is how the
		// dialect that has the word gets it.
		if typeset, ok := r.Builtin("typeset"); ok {
			r.Register("declare", typeset)
			r.SetDeclaring("declare")
		}
		// A local starting as the outer binding, which the route leaves to
		// the full road.
		r.SetLocalInheritsTheOuterValue(inheritsOuter)
		st, err := r.Run(context.Background(), prog)
		if err != nil {
			t.Fatalf("route %v: %v", on, err)
		}
		return out.String(), st, EmptyShadowsForTest(r)
	}
}

// emptyShadowCompare runs prog both ways and requires the same output and
// status, the route to have taken at least atLeast operands with it on (none
// where atLeast is 0) and none with it off, and the full road's output to hold
// every row, so a script that stopped early compares nothing.
func emptyShadowCompare(
	t *testing.T, prog *syntax.File, sem Semantics, inheritsOuter bool, atLeast int, rows []string,
) {
	t.Helper()
	run := emptyShadowRun(t, prog, sem, inheritsOuter)
	full, fullStatus, fullTaken := run(false)
	short, shortStatus, taken := run(true)
	if short != full || shortStatus != fullStatus {
		t.Errorf("with the route:\n%s(status %d)\nwithout it:\n%s(status %d)", short, shortStatus, full, fullStatus)
	}
	switch {
	case fullTaken != 0:
		t.Errorf("the route took %d operands with it off; want none", fullTaken)
	case atLeast == 0 && taken != 0:
		t.Errorf("the route took %d operands; want none", taken)
	case taken < atLeast:
		t.Errorf("the route took %d operands; want at least %d", taken, atLeast)
	}
	for _, row := range rows {
		if !strings.Contains(full, row) {
			t.Errorf("the script did not reach %q:\n%s", row, full)
		}
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
			atLeast := 20
			if name == "inherits outer" {
				atLeast = 0
			}
			emptyShadowCompare(t, f, sem, name == "inherits outer", atLeast,
				[]string{"[<one>][<lit>]", "[<two>][<lit>]", "[deep]\n", "[after][after]\n", "[1]\n[1]\n"})
		})
	}
}

// emptyShadowGlobalLines are declarations the route must leave alone however
// they are spelled: `-g`, which makes no binding, beside a valueless and a
// valued plain operand on lines of their own, read back after the return.
const emptyShadowGlobalLines = `gl() { local -g ga gb=1; local gc=2; show ga gb gc; }; gl; show ga gb gc
`

// emptyShadowSpelled is emptyShadowScript with every `local` written as word,
// and every function defined with the `function` word where keyword is set.
func emptyShadowSpelled(t *testing.T, word string, keyword bool) *syntax.File {
	t.Helper()
	src := strings.ReplaceAll(emptyShadowScript+emptyShadowGlobalLines, "local ", word+" ")
	if keyword {
		src = posixFunctionHead.ReplaceAllString(src, "${1}function $2 {")
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// posixFunctionHead is a `name() {` at the start of a line or after a `;` or
// a `(`, which is every definition emptyShadowScript writes.
var posixFunctionHead = regexp.MustCompile(`(^|[;(]\s*|\n)([a-z][a-z0-9]*)\(\) \{`)

// TestAnEmptyTypesetShadowAnswersAsTheFullRoadDoes runs the same declarations
// written `typeset` and `declare` — declareNames, not biLocal — with the route
// on and off, under every vector the route's line question reads: a dialect
// where a function of either form makes locals, one where only a
// `function`-form body does (so a POSIX-form `f()` assigns the caller's names
// and must not take it), one that has not answered that question, and locals
// that inherit the outer value. Byte-identical output and status, and the
// route taken where the line makes locals and nowhere else (#6373).
func TestAnEmptyTypesetShadowAnswersAsTheFullRoadDoes(t *testing.T) {
	type form struct {
		keyword bool
		name    string
	}
	forms := []form{{false, "f()"}, {true, "function f"}}
	for _, word := range []string{"typeset", "declare"} {
		for _, fm := range forms {
			prog := emptyShadowSpelled(t, word, fm.keyword)
			for vname, sem := range emptyShadowVectors() {
				// `-g` is a letter of the word here, as it is of `typeset` in
				// every shell that has it.
				sem.DeclareOptions = "aAgilnprux"
				sem.TypesetLocalNeedsKeywordFunction = No
				t.Run(word+"/"+fm.name+"/"+vname, func(t *testing.T) {
					atLeast := 20
					if vname == "inherits outer" {
						atLeast = 0
					}
					emptyShadowCompare(t, prog, sem, vname == "inherits outer", atLeast,
						[]string{"[<one>][<lit>]", "[<two>][<lit>]", "[deep]\n", "[after][after]\n", "[1]\n[1]\n"})
				})
			}
			for _, kw := range []struct {
				name   string
				answer Answer
			}{{"keyword functions only", Yes}, {"keyword axis open", Unspecified}} {
				sem := emptyShadowVectors()["base"]
				sem.DeclareOptions = "aAgilnprux"
				sem.TypesetLocalNeedsKeywordFunction = kw.answer
				t.Run(word+"/"+fm.name+"/"+kw.name, func(t *testing.T) {
					// Only a `function`-form body makes locals here, and an
					// open axis is left to the full road whichever form asks.
					atLeast := 0
					if fm.keyword && kw.answer == Yes {
						atLeast = 20
					}
					rows := []string{"[<one>][<lit>]", "[deep]\n"}
					if !fm.keyword && kw.answer == Unspecified {
						// The full road refuses each declaration here, and
						// the report is what both runs must write alike.
						rows = []string{"needing a keyword-defined function to declare a local", "[deep]\n"}
					}
					emptyShadowCompare(t, prog, sem, false, atLeast, rows)
				})
			}
		}
	}
}

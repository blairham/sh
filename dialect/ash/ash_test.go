// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The measured answers for BusyBox ash live here, in ash's own package, so
// that adding a shell never touches the substrate. Every expectation below was
// taken from BusyBox v1.37.0 on 2026-09-12; docs/spec/ash.md records how, and
// records that nothing in `make check` re-checks them.

func parses(t *testing.T, src string) bool {
	t.Helper()
	_, err := syntax.Parse(src, ash.Dialect())
	return err == nil
}

func run(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "ash", Env: []string{"PATH=/usr/bin:/bin"},
	}, src)
	if err != nil {
		return out + "unsupported: " + err.Error(), -1
	}
	return out, st
}

// TestTheHelperRunsUnderThisDialectAndNotTheCore guards the field run() sets.
// The omission is silent — nil means the core, the snippet still runs — so it
// is asserted with a construct the core has and this shell does not.
func TestTheHelperRunsUnderThisDialectAndNotTheCore(t *testing.T) {
	out, _ := run(t, `eval 'a=(1 2)'; echo st=$?`)
	if strings.Contains(out, "st=0") {
		t.Errorf("run = %q, want a failure: this shell has no array literal, "+
			"so the runner was not told the dialect", out)
	}
}

// TestGrammarTakesWhatWasMeasured is the half of the substrate claim that a
// parser can answer. The shape of the list is the finding: the issue that
// asked for this dialect predicted a near-copy of dash's variant set, and the
// first six rows are constructs dash refuses.
func TestGrammarTakesWhatWasMeasured(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want bool
		why  string
	}{
		{`echo $'a\tb'`, true, "$'…' writes a real tab"},
		{`[[ -n x ]]`, true, "[[ ]] is here, as a builtin"},
		{`cat <(echo hi)`, true, "process substitution writes hi"},
		{`function f { echo x; }`, true, "the keyword, brace body"},
		{`function f() { echo x; }`, true, "and with parens too"},
		{`echo $((2**3))`, true, "8, where dash refuses the operator"},
		{`echo $((10#08))`, true, "8, where dash refuses the base"},
		{`v=abcdef; echo ${v:1:3}`, true, "bcd"},
		{`v=abc; echo ${v/b/X}`, true, "aXc"},
		{`a.b() { echo hi; }`, true, "a name may carry punctuation"},

		{`a=(x y)`, false, "no arrays: unexpected \"(\""},
		{`for ((i=0;i<2;i++)); do :; done`, false, "no C-style for"},
		{`select x in a; do break; done`, false, "no select"},
		{`cat <<< hello`, false, "no herestring"},
		{`echo ${!v}`, false, "no indirection"},
		{`echo hi |& cat`, false, "no |&"},
		{`case a in a) :;;& b) :;; esac`, false, "no ;;&"},
	} {
		if got := parses(t, tc.src); got != tc.want {
			t.Errorf("%q: parses = %v, want %v (%s)", tc.src, got, tc.want, tc.why)
		}
	}
}

// TestAliasesExpandOnEveryRouteButTheCommandString is the grammar difference
// between the two ash descendants that a reader is most likely to trip over.
// dash expands on all three routes; this shell expands in a file and on
// standard input and not under `-c`, where `ash -c 'alias foo=echo; foo'`
// answers `foo: not found`.
func TestAliasesExpandOnEveryRouteButTheCommandString(t *testing.T) {
	if !ash.Dialect().AliasesExpandUnlessTold {
		t.Error("AliasesExpandUnlessTold = false, want true")
	}
	got := ash.Dialect().ExpandAliasesInProgramText
	if got&syntax.RouteFromCommandString != 0 {
		t.Errorf("ExpandAliasesInProgramText = %v, want the command-string route excluded", got)
	}
	if got&syntax.RouteFromScriptFile == 0 || got&syntax.RouteOnStandardInput == 0 {
		t.Errorf("ExpandAliasesInProgramText = %v, want a file and standard input included", got)
	}
	if !ash.Dialect().AliasBodyCountsLines {
		t.Error("AliasBodyCountsLines = false, want true")
	}
}

// TestTheAnswersThatSideWithBashRatherThanDash is the finding this dialect was
// added to obtain, written down as a test so a later edit cannot quietly undo
// it. The issue predicted ash would join dash's side of every non-POSIX
// question and thereby end dash's status as the panel's sole dissenter.
// Measured, it does the opposite on all five.
func TestTheAnswersThatSideWithBashRatherThanDash(t *testing.T) {
	s := ash.Semantics()
	d := ash.Dialect()
	// `echo 'a\tb'` writes the backslash here and a tab in dash.
	if s.EchoInterpretsEscapes != interp.No {
		t.Errorf("EchoInterpretsEscapes = %v, want No", s.EchoInterpretsEscapes)
	}
	if !strings.Contains(s.EchoOptions, "e") {
		t.Errorf("EchoOptions = %q, want the e letter", s.EchoOptions)
	}
	// `x=outer; f(){ local x; echo "[$x]"; }; f` is `[]` here, `[outer]` in
	// dash. Two fields, because the pairing is what makes the answer.
	if s.DeclaredNameWithoutValueIsEmpty != interp.No {
		t.Errorf("DeclaredNameWithoutValueIsEmpty = %v, want No", s.DeclaredNameWithoutValueIsEmpty)
	}
	if s.ValuelessDeclarationHidesTheOuterValue != interp.Yes {
		t.Errorf("ValuelessDeclarationHidesTheOuterValue = %v, want Yes", s.ValuelessDeclarationHidesTheOuterValue)
	}
	// And the two arithmetic constructs dash refuses outright.
	if !d.ArithExponent || !d.ArithExplicitBase {
		t.Errorf("ArithExponent/ArithExplicitBase = %v/%v, want both", d.ArithExponent, d.ArithExplicitBase)
	}
	// `case x in [^a])` negates here, and dash is the panel's only shell that
	// reads the caret as an ordinary character. The axis lives in the core,
	// which is why this reads the value the core's own reading implies rather
	// than a field this dialect sets: the preset leaves it where POSIX has
	// it, and the run below is the thing that settles it.
	out, _ := run(t, `case x in [^a]) echo negates ;; *) echo plain ;; esac`)
	if !strings.Contains(out, "negates") {
		t.Errorf("[^a] = %q, want it to negate", out)
	}
	// A sixth, measured 2026-09-12 in the container: a character class name
	// this shell has not got is *inert* — bash's and zsh's answer — where
	// dash stops the bracket's scan at it. `[a[:nope:]b]` matches `b` here
	// and does not in dash (#2383).
	if got, want := s.UnknownCharacterClass, interp.UnknownClassIsInert; got != want {
		t.Errorf("UnknownCharacterClass = %v, want %v", got, want)
	}
	out, _ = run(t, `case b in [a[:nope:]b]) echo in ;; *) echo out ;; esac`)
	if !strings.Contains(out, "in") {
		t.Errorf("[a[:nope:]b] against b = %q, want the unknown name inert", out)
	}
	// A seventh, and the one that was wrong here until #2441 built something
	// that reads the record: `shift` past the end is survivable, as it is in
	// bash, where dash — the preset this one starts from — ends the script
	// over it. Measured 2026-09-12 in the pinned image: nothing is printed at
	// all, 1 is left behind, `$#` is untouched and the next command runs.
	if s.ShiftPastEndFatal != interp.No {
		t.Errorf("ShiftPastEndFatal = %v, want No", s.ShiftPastEndFatal)
	}
	// Both halves, because the axis alone would leave dash's sentence being
	// printed where BusyBox says nothing — a survivable overshoot that
	// complains is not this shell either.
	out, st := run(t, `set -- a b; shift 5; echo "st=$? n=$#"`)
	if !strings.Contains(out, "st=1 n=2") || st != 0 {
		t.Errorf("shift past the end = %q at %d, want `st=1 n=2` at 0", out, st)
	}
	if strings.Contains(out, "shift") {
		t.Errorf("shift past the end = %q, want it to say nothing at all", out)
	}
}

// TestDiagnosticsAreWordedThisShellsWay pins the two shapes that run through
// the whole table, and that account for more corpus rows than every other
// difference from dash put together.
func TestDiagnosticsAreWordedThisShellsWay(t *testing.T) {
	d := ash.Diagnostics()
	// The complaint first and the token after it, lower case: `syntax error:
	// unexpected "("`, against dash's `Syntax error: "(" unexpected`.
	if got, want := d.SyntaxUnexpected, `syntax error: unexpected "%[1]s"`; got != want {
		t.Errorf("SyntaxUnexpected = %q, want %q", got, want)
	}
	if got, want := d.BadSubstitution, "syntax error: bad substitution"; got != want {
		t.Errorf("BadSubstitution = %q, want %q", got, want)
	}
	// No line under `-c` and a `line N` in a script, which is the split
	// neither of the other four makes this way.
	if got, want := d.Location, interp.LocationNameOnly; got != want {
		t.Errorf("Location = %v, want %v", got, want)
	}
	if got, want := d.ScriptLocation, interp.LocationLineWord; got != want {
		t.Errorf("ScriptLocation = %v, want %v", got, want)
	}
}

// TestTheDialectRefusesNothingItWasNotMeasuredRefusing is a guard against the
// hazard this package was written under: for its first day there was no
// oracle column, so an answer added by eye was indistinguishable from one
// that was run. There is a column now (#2263) and `make
// conformance-dialects` grades this dialect against it, which is the wider
// net; these rows stay because they are the behaviors a reader is most likely
// to assume from dash, each measured the other way, and a named assertion
// says *which* assumption is wrong where a conformance number only says how
// many are.
func TestTheDialectRefusesNothingItWasNotMeasuredRefusing(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want string
	}{
		// A bare `read` fills REPLY here; dash wants a name.
		{`printf 'x\n' | { read; echo "[$REPLY]"; }`, "[x]"},
		// `$(( ))` is 0 rather than an error.
		{`echo "[$(( ))]"`, "[0]"},
		// `break` in a function reaches the caller's loop, so this writes
		// nothing at all; dash writes both rounds.
		{`for i in 1 2; do echo "i=$i"; done`, "i=1"},
		// And the three expansions this shell refuses when it reaches them
		// rather than when it parses them — measured, `false && echo
		// "${v[0]}"` runs to the end and prints what follows, so the
		// refusal is not the parser's.
		{`echo "st=$?"; false && echo "${a[0]}"; echo after`, "after"},
	} {
		out, _ := run(t, tc.src)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%q = %q, want it to contain %q", tc.src, out, tc.want)
		}
	}
}

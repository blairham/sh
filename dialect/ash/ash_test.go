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

// TestAliasesExpandOnEveryRoute pins ash beside dash rather than beside zsh,
// which is where #2338 moved it. The test that stood here asserted the
// opposite — that `-c` was excluded — and it was a test of our own answer
// wearing another shell's name: the measurement behind it was the one-liner
// `ash -c 'alias foo=echo; foo'`, which answers `not found` in every shell in
// the panel because an alias never expands on the line that defines it. Given
// two lines, BusyBox v1.37.0 expands under `-c` as well.
func TestAliasesExpandOnEveryRoute(t *testing.T) {
	if !ash.Dialect().AliasesExpandUnlessTold {
		t.Error("AliasesExpandUnlessTold = false, want true")
	}
	if got, want := ash.Dialect().ExpandAliasesInProgramText, syntax.RouteOnEveryRoute; got != want {
		t.Errorf("ExpandAliasesInProgramText = %v, want %v", got, want)
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
	// The delimiters are read and no body is ever an element, which is this
	// column alone: measured 2026-09-18 in BusyBox v1.37.0 in the pinned
	// image, `[[.a.]]` matches nothing at all while `[[.a.]x]` matches `x`
	// (#3379).
	if got, want := s.CollatingElements, interp.CollatingElementsHoldNothing; got != want {
		t.Errorf("CollatingElements = %v, want %v", got, want)
	}
	// Unmeasured for the same reason, and the same treatment: the answer the
	// shell already gave, which is dash's (#3379).
	if got, want := s.UnterminatedBracketAfterASubExpression, interp.BracketNoMatch; got != want {
		t.Errorf("UnterminatedBracketAfterASubExpression = %v, want %v", got, want)
	}
	// And no parameter for the order an expansion comes back in.
	if got, want := s.SortOrderVariable, ""; got != want {
		t.Errorf("SortOrderVariable = %q, want %q", got, want)
	}
	// And the `[:` that nothing closes, which is the axis beside it
	// rather than a corner of it — see #1431.
	if got, want := s.UnterminatedCharacterClass, interp.UnterminatedClassSwallowsTheClosingBracket; got != want {
		t.Errorf("UnterminatedCharacterClass = %v, want %v", got, want)
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

// TestTypeHasNoLettersAtAll. `type -- cd` reads the `--` as a name here, so
// no letter of its own is reachable — `-t` included, which the empty
// optstring says on its own since #2180.
func TestTypeHasNoLettersAtAll(t *testing.T) {
	if got := ash.Semantics().TypeOptions; got != "" {
		t.Errorf("TypeOptions = %q, want none: this shell's type has no options", got)
	}
}

// TestPrintfAbsentNumberIsAnEmptyOne is ash's own answer, and the whole of
// #2648: BusyBox does not separate an absent numeric operand from one that is
// present and empty. `printf '[%d]\n'` writes `ash: invalid number ”` and
// ends at 1, exactly as `printf '[%d]\n' ”` does, where bash, zsh, ksh93 and
// dash all write the zero in silence at 0.
func TestPrintfAbsentNumberIsAnEmptyOne(t *testing.T) {
	s := ash.Semantics()
	if got, want := s.PrintfAbsentNumberIsAnEmptyOne, interp.Yes; got != want {
		t.Errorf("PrintfAbsentNumberIsAnEmptyOne = %v, want %v", got, want)
	}
	// The pair it depends on. Were this No, the axis above would never be
	// reached and the row would be silently inert.
	if got, want := s.PrintfEmptyIsNotANumber, interp.Yes; got != want {
		t.Errorf("PrintfEmptyIsNotANumber = %v, want %v", got, want)
	}
}

// TestPrintfStarComplaintCostsTheStatus is ash's alone in the other
// direction: the complaint about a `*` operand it cannot read goes out and
// the status stays 0. `printf '[%*s]\n' abc hi` writes `ash: invalid number
// 'abc'` to stderr, `[hi]` to stdout and reports success, where bash and dash
// report 1 for the same line.
func TestPrintfStarComplaintCostsTheStatus(t *testing.T) {
	s := ash.Semantics()
	if got, want := s.PrintfStarComplaintCostsTheStatus, interp.No; got != want {
		t.Errorf("PrintfStarComplaintCostsTheStatus = %v, want %v", got, want)
	}
	// An infinity goes through the conversion here too, with one cell that
	// is musl rather than BusyBox: `printf '%+f' nan` is `+nan` in the
	// image and `nan` in every other column, because musl takes the `+`
	// flag before it looks at the value and BSD clears the sign of a
	// not-a-number outright. A dialect is not a libc, so this shell writes
	// `nan` unsigned and the corpus row records the difference (#2707).
	if got, want := s.PrintfNonFiniteIsConverted, interp.Yes; got != want {
		t.Errorf("PrintfNonFiniteIsConverted = %v, want %v", got, want)
	}
	// The whole operand or nothing: `printf '%d' 1.5` is 0 here where four
	// columns read a 1, and `printf '%d' 99999999999999999999` is 0 where
	// three saturate — its integer reader returns nothing on `ERANGE`,
	// while its `strtod` returns the infinity and `printf '%f' 1e400` is
	// `inf` all the same (#2731, #2727).
	if got, want := s.PrintfNumberOperand, interp.PrintfNumberWholeOperand; got != want {
		t.Errorf("PrintfNumberOperand = %v, want %v", got, want)
	}
	// None of C99's three: `printf '%F' 1.5` is `%F]: invalid format` at 1
	// in BusyBox ash 1.37 (#2726).
	if got, want := s.PrintfC99FloatConversions, interp.No; got != want {
		t.Errorf("PrintfC99FloatConversions = %v, want %v", got, want)
	}
	// And an absent star operand is a silent zero even here, which is what
	// keeps it a separate question from the absent *conversion* operand
	// above: `printf 'a%*db'` writes one complaint in BusyBox and not two.
	if got, want := s.PrintfStarWithoutOperandIsRefused, interp.No; got != want {
		t.Errorf("PrintfStarWithoutOperandIsRefused = %v, want %v", got, want)
	}
}

// TestPrintfGroupingFlag: BusyBox ash has no `'` flag, as dash has none. The
// character reaches the scan as the conversion this shell does not have, so
// `printf "[%'d]" 1234567` writes `[`, `%'d]: invalid format` and reports 1
// — measured 2026-09-13 on BusyBox 1.37.0 in the pinned Alpine image, where
// the five columns that do have the flag write `[1234567]` at 0 under the
// harness's `LC_ALL=C` (#2665).
func TestPrintfGroupingFlag(t *testing.T) {
	s := ash.Semantics()
	if got, want := s.PrintfGroupingFlag, interp.No; got != want {
		t.Errorf("PrintfGroupingFlag = %v, want %v", got, want)
	}
	// Answered rather than left open, because an unanswered axis is
	// indistinguishable from one nobody thought about — and this shell never
	// reaches the question, having refused the flag one level up.
	if got, want := s.PrintfGroupingFlagAfterTheWidth, interp.No; got != want {
		t.Errorf("PrintfGroupingFlagAfterTheWidth = %v, want %v", got, want)
	}
}

// There is no `disown` here, which is what the unanswered verdict on
// Semantics.DisownAlwaysFails stands on: the name resolves to nothing at all,
// so there is no status for the axis to be about.
func TestDisownIsNotABuiltin(t *testing.T) {
	out, st := run(t, `disown; echo st=$?`)
	if !strings.Contains(out, "disown: not found") || !strings.Contains(out, "st=127") {
		t.Errorf("got %q status %d, want the name unresolved at 127", out, st)
	}
}

// This shell numbers a backquoted body from one and a `$( … )` body from the
// file — two answers for the two spellings of one construct, and the same
// pair dash gives. Measured 2026-09-18 on BusyBox 1.37.0 in the pinned image,
// on both routes to the body: a refusal and a `not found` inside “ ` ` “
// both name `line 1` where the `$( … )` spelling names `line 2` (#2471).
func TestABackquotedBodyIsNumberedFromOne(t *testing.T) {
	if !ash.Diagnostics().BackquotedSubstitutionRestartsLines {
		t.Error("a backquoted body is numbered from one here")
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `functions -M`: a shell function registered under a name arithmetic can
// call. Here rather than in interp because the whole thing is reached through
// this dialect's tables — the word `functions`, the `M` in its letter set, the
// `(` after a name being a call at all — and a case built on a permissive base
// would see none of them (#1493).
//
// Every expectation below was measured on zsh 5.9.2 (Homebrew, aarch64) and
// zsh 5.9 (`/bin/zsh`, macOS 26), which agree.

// mathValue registers `mf` over the given body and returns what `$(( mf(5) ))`
// evaluates to, which is the one question the whole facility turns on.
func mathValue(t *testing.T, body, call string) string {
	t.Helper()
	out, st := runZsh(t, t.TempDir(),
		"g(){ "+body+" }\nfunctions -M -- mf 0 9 g\necho $(( "+call+" ))\n")
	if st != 0 {
		t.Fatalf("body %q call %q: status %d, output %q", body, call, st, out)
	}
	return strings.TrimSuffix(out, "\n")
}

// TestAMathFunctionReturnsTheLastArithmeticValueAndNotREPLY is the measurement
// the design rests on, and the reason `-M` was refused rather than guessed at
// when the two names landed (#1487).
//
// The documented convention is that the implementation sets `REPLY`. It is not
// what the shell does: `REPLY` is never read, and the value is the last
// arithmetic evaluation performed anywhere during the call. The two readings
// agree on `REPLY=$(( … ))` — the idiom every documented caller writes, whose
// right-hand side is the last evaluation — and part company on every row here
// that does not write one, which is where the plugin manager lives.
func TestAMathFunctionReturnsTheLastArithmeticValueAndNotREPLY(t *testing.T) {
	for _, c := range []struct{ body, call, want string }{
		// A plain string assignment evaluates no arithmetic, so the last
		// value is the argument — the reading that separates this from
		// REPLY, and the one a REPLY implementation gets wrong.
		{"REPLY=7;", "mf(5)", "5"},
		{"REPLY=\"7\";", "mf(5)", "5"},
		// The idiom. Right by the same rule rather than by a second one.
		{"REPLY=$((7));", "mf(5)", "7"},
		{"REPLY=$(($1+100));", "mf(5)", "105"},
		// Arithmetic that has nothing to do with REPLY still decides it,
		// before the assignment and after it.
		{": $((123)); REPLY=7;", "mf(5)", "123"},
		{"REPLY=7; : $((123));", "mf(5)", "123"},
		{"(( x = 77 ));", "mf(5)", "77"},
		// `return` takes an expression in this shell, so it is an
		// arithmetic evaluation like any other — including when it is
		// written as a plain number, which is the branch the plugin
		// manager's scheduler takes.
		{"return 42;", "mf(5)", "42"},
		{"local i=9; return i;", "mf(5)", "9"},
		{"return $(( $1 * 3 ));", "mf(5)", "15"},
		// A function the implementation calls counts too: it is "anywhere
		// during the call" and not "in this body".
		{"h(){ : $((55)); }; h;", "mf(5)", "55"},
		// Nothing at all, so the arguments are the last thing evaluated —
		// and it is the *last* argument, not the first.
		{":;", "mf(5)", "5"},
		{":;", "mf(3,4)", "4"},
		{":;", "mf(1,2,3)", "3"},
	} {
		if got := mathValue(t, c.body, c.call); got != c.want {
			t.Errorf("body %q call %q = %s, want %s", c.body, c.call, got, c.want)
		}
	}
}

// TestTheLastArithmeticValueOutlivesTheRegistration — the record is the
// shell's and not the call's, which is what the last row of the table
// measured: an evaluation from before the math function existed is still what
// an implementation that evaluates nothing hands back.
func TestTheLastArithmeticValueOutlivesTheRegistration(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"g(){ :; }\n: $((123))\nfunctions -M -- mf 0 0 g\necho $(( mf() ))\n")
	if out != "123\n" || st != 0 {
		t.Errorf("output = %q status %d, want 123", out, st)
	}
}

// TestAMathFunctionRuns is the seam itself: an expression that runs a shell
// function, with everything a call brings.
func TestAMathFunctionRuns(t *testing.T) {
	// The body really runs, and its output lands where the command's does.
	out, st := runZsh(t, t.TempDir(),
		"g(){ echo IN; return 7; }\nfunctions -M mf 1 1 g\necho $(( mf(5) ))\necho st=$?\n")
	if out != "IN\n7\nst=0\n" || st != 0 {
		t.Errorf("output = %q status %d, want the body to run and 7 to come back", out, st)
	}
	// A call is an operand like any other, so it composes.
	out, _ = runZsh(t, t.TempDir(),
		"g(){ return 7; }\nfunctions -M mf 1 1 g\nx=5\necho $(( mf(x) + 1 ))\n")
	if out != "8\n" {
		t.Errorf("output = %q, want 8", out)
	}
	// And it is reachable from every place an expression is written, not
	// only from `$(( ))`.
	out, _ = runZsh(t, t.TempDir(),
		"g(){ return 7; }\nfunctions -M mf 1 1 g\n(( y = mf(1) ))\n"+
			"for ((i=0;i<mf(1);i++)); do :; done\necho $y $i\n")
	if out != "7 7\n" {
		t.Errorf("output = %q, want 7 7", out)
	}
}

// TestAMathFunctionSeesItsArgumentsEvaluatedAndAsStrings. They arrive already
// evaluated — `mf(1+1,2*3)` passes `2` and `6` — written the way this shell
// writes a number, and `$0` is the *registered* name rather than the
// implementation's.
func TestAMathFunctionSeesItsArgumentsEvaluatedAndAsStrings(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"g(){ echo \"0=$0 #=$# *=$*\"; }\nfunctions -M mf 0 3 g\n: $(( mf(1,2,3) ))\n")
	if out != "0=mf #=3 *=1 2 3\n" {
		t.Errorf("output = %q, want the registered name and the three arguments", out)
	}
	out, _ = runZsh(t, t.TempDir(),
		"g(){ echo \"*=$*\"; }\nfunctions -M mf 0 3 g\n: $(( mf(1+1,2*3) ))\n: $(( mf(3/2) ))\n: $(( mf(1.5,2.0) ))\n")
	if out != "*=2 6\n*=1\n*=1.5 2.\n" {
		t.Errorf("output = %q, want evaluated arguments written as this shell writes numbers", out)
	}
	// A diagnostic from inside the body still names the implementation, not
	// the registered name — the two names go to different places and this is
	// the half that is not `$0`.
	out, _ = runZsh(t, t.TempDir(),
		"g(){ nosuchcmd_xyz; }\nfunctions -M mf 1 1 g\necho $(( mf(5) ))\n")
	if !strings.Contains(out, "g: command not found: nosuchcmd_xyz") {
		t.Errorf("output = %q, want the implementation named", out)
	}
	// The trace says the implementation too, and it is a separate record
	// from the frame the diagnostic used: `+g:0>` and not `+mf:0>`. Without
	// this, setting the trace's name to the registered one changed nothing
	// any test could see.
	out, _ = runZsh(t, t.TempDir(),
		"g(){ echo hi; }\nfunctions -M mf 1 1 g\nset -x\n: $(( mf(5) ))\n")
	if !strings.Contains(out, "+g:0> echo hi") {
		t.Errorf("output = %q, want the trace to name the implementation", out)
	}
}

// TestAMathFunctionRecursesAndCallsAnother — the seam re-enters itself.
func TestAMathFunctionRecursesAndCallsAnother(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"g(){ if (( $1 <= 1 )); then return 1; else return $(( $1 * mf($1-1) )); fi }\n"+
			"functions -M mf 1 1 g\necho $(( mf(5) ))\n")
	if out != "120\n" || st != 0 {
		t.Errorf("output = %q status %d, want 120", out, st)
	}
	out, _ = runZsh(t, t.TempDir(),
		"a(){ return $(($1*2)); }\nb(){ return $(( ma($1) + 1 )); }\n"+
			"functions -M ma 1 1 a\nfunctions -M mb 1 1 b\necho $(( mb(5) ))\n")
	if out != "11\n" {
		t.Errorf("output = %q, want 11", out)
	}
	// A call inside a call's own argument, which is the same seam twice on
	// one expression rather than once.
	out, _ = runZsh(t, t.TempDir(),
		"g(){ return 3; }\nfunctions -M mf 1 1 g\necho $(( mf(mf(1)) ))\n")
	if out != "3\n" {
		t.Errorf("output = %q, want 3", out)
	}
}

// TestTheArityOperandsAreNameMinMaxImpl, each measured.
func TestTheArityOperandsAreNameMinMaxImpl(t *testing.T) {
	// A minimum on its own is also the maximum.
	for _, c := range []struct{ spec, call, want string }{
		{"mf 1", "mf(1)", "8\n"},
		{"mf 1", "mf()", "wrong number of arguments: mf()"},
		{"mf 1", "mf(1,2)", "wrong number of arguments: mf(1,2)"},
		{"mf 0", "mf()", "8\n"},
		{"mf 0", "mf(1)", "wrong number of arguments: mf(1)"},
		{"mf 2", "mf(1,2)", "8\n"},
		{"mf 2", "mf(1)", "wrong number of arguments: mf(1)"},
		// No arity at all is no bound, and the implementation defaults to
		// the registered name — which is why `mf` must be the function here.
		{"mf", "mf(1,2,3,4,5)", "8\n"},
		// A maximum of -1 is unbounded above the minimum.
		{"mf 1 -1", "mf(1,2,3,4)", "8\n"},
	} {
		out, _ := runZsh(t, t.TempDir(),
			"mf(){ return 8; }\nfunctions -M "+c.spec+"\necho $(( "+c.call+" ))\n")
		if !strings.Contains(out, c.want) {
			t.Errorf("functions -M %s; $(( %s )) = %q, want %q", c.spec, c.call, out, c.want)
		}
	}
	// The count complaint quotes the call as it was *written*, spaces and
	// all, rather than rendering the tree back.
	out, _ := runZsh(t, t.TempDir(),
		"g(){ :; }\nfunctions -M mf 1 1 g\necho $(( mf( 5 , 6 ) ))\n")
	if !strings.Contains(out, "wrong number of arguments: mf( 5 , 6 )") {
		t.Errorf("output = %q, want the call quoted as written", out)
	}
}

// TestABadArityOperandIsRefusedAndNothingIsRegistered. Silence is the failure
// mode this whole issue guards against, so each refusal is checked for leaving
// no registration behind as well as for its sentence.
func TestABadArityOperandIsRefusedAndNothingIsRegistered(t *testing.T) {
	for _, c := range []struct{ spec, want string }{
		{"mf x 1 g", "-M: invalid min number of arguments: x"},
		{"mf -1 1 g", "-M: invalid min number of arguments: -1"},
		{"mf 1 x g", "-M: invalid max number of arguments: x"},
		{"mf 3 1 g", "-M: invalid max number of arguments: 1"},
		{"mf 1 2 g extra", "-M: too many arguments"},
		{"1bad 1 1 g", "-M 1bad: bad math function name"},
		{"'a b' 1 1 g", "-M a b: bad math function name"},
	} {
		out, _ := runZsh(t, t.TempDir(),
			"g(){ :; }\nfunctions -M "+c.spec+"\necho st=$?\nfunctions -M\n")
		if !strings.Contains(out, c.want) {
			t.Errorf("functions -M %s: output = %q, want %q", c.spec, out, c.want)
		}
		if !strings.Contains(out, "st=1\n") {
			t.Errorf("functions -M %s: output = %q, want status 1", c.spec, out)
		}
		if strings.Contains(out, "functions -M mf") || strings.Contains(out, "functions -M 1bad") {
			t.Errorf("functions -M %s: output = %q, want nothing registered", c.spec, out)
		}
		// And the builtin names itself in the location, as its other
		// complaints do.
		if !strings.Contains(out, ":functions:") {
			t.Errorf("functions -M %s: output = %q, want the invoked name in the location", c.spec, out)
		}
	}
}

// TestRegistrationDoesNotCheckTheImplementation — measured status 0 with a
// name nobody defined, and the failure arrives at the call instead. Three
// separate sentences for three separate questions.
func TestRegistrationDoesNotCheckTheImplementation(t *testing.T) {
	// The failing call ends the script the way any arithmetic failure in
	// this shell does, so only the registration's own status is 0 here.
	out, _ := runZsh(t, t.TempDir(),
		"functions -M mf 1 1 nodef\necho st=$?\necho $(( mf(5) ))\n")
	if !strings.Contains(out, "st=0\n") {
		t.Errorf("output = %q, want the registration taken", out)
	}
	if !strings.Contains(out, "no such function: nodef") {
		t.Errorf("output = %q, want the implementation named at the call", out)
	}
	// A name nobody registered at all is a different sentence from a
	// registration whose implementation is missing.
	out, _ = runZsh(t, t.TempDir(), "echo $(( nosuchmf(1) ))\n")
	if !strings.Contains(out, "unknown function: nosuchmf") {
		t.Errorf("output = %q, want the unknown-function sentence", out)
	}
	// And an implementation removed between the registration and the call
	// fails at the call, not before.
	out, _ = runZsh(t, t.TempDir(),
		"g(){ :; }\nfunctions -M mf 1 1 g\nunfunction g\necho $(( mf(1) ))\n")
	if !strings.Contains(out, "no such function: g") {
		t.Errorf("output = %q, want the call to fail once the implementation is gone", out)
	}
}

// TestTheListingSaysRegistrationsBackInTheFormThatWouldMakeThem, most recently
// registered first, with the operands a re-registration would default to left
// out. Row for row as measured.
func TestTheListingSaysRegistrationsBackInTheFormThatWouldMakeThem(t *testing.T) {
	for _, c := range []struct{ spec, want string }{
		{"mf", "functions -M mf\n"},
		{"mf 0", "functions -M mf 0 0\n"},
		{"mf 1", "functions -M mf 1\n"},
		{"mf 2 2", "functions -M mf 2\n"},
		{"mf 0 1", "functions -M mf 0 1\n"},
		{"mf 1 2", "functions -M mf 1 2\n"},
		{"mf 0 -1", "functions -M mf\n"},
		{"mf 1 -1", "functions -M mf 1 -1\n"},
		{"mf 0 0 g", "functions -M mf 0 0 g\n"},
		{"mf 1 1 g", "functions -M mf 1 1 g\n"},
		{"mf 2 2 g", "functions -M mf 2 2 g\n"},
		{"mf 0 -1 g", "functions -M mf 0 -1 g\n"},
	} {
		out, st := runZsh(t, t.TempDir(), "g(){ :; }\nfunctions -M "+c.spec+"\nfunctions -M\n")
		if out != c.want || st != 0 {
			t.Errorf("functions -M %s: listing = %q status %d, want %q", c.spec, out, st, c.want)
		}
	}
	// Most recent first, which is the table's order and not the alphabet's:
	// registering the names backwards lists them forwards.
	out, _ := runZsh(t, t.TempDir(),
		"g(){ :; }\nfor n in aa bb cc; do functions -M $n 1 1 g; done\nfunctions -M\n")
	if out != "functions -M cc 1 1 g\nfunctions -M bb 1 1 g\nfunctions -M aa 1 1 g\n" {
		t.Errorf("listing = %q, want the most recent first", out)
	}
	// A second registration of one name replaces it rather than adding a
	// second row — and moves it to the front, which is what says the order
	// is arrival and not first-seen.
	out, _ = runZsh(t, t.TempDir(),
		"g(){ return 1; }\nh(){ return 2; }\nfunctions -M mf 1 1 g\nfunctions -M mf 1 1 h\n"+
			"functions -M\necho $(( mf(0) ))\n")
	if out != "functions -M mf 1 1 h\n2\n" {
		t.Errorf("output = %q, want one row and the later implementation", out)
	}
	out, _ = runZsh(t, t.TempDir(),
		"g(){ :; }\nfunctions -M a 1 1 g\nfunctions -M b 1 1 g\nfunctions -M a 2 2 g\nfunctions -M\n")
	if out != "functions -M a 2 2 g\nfunctions -M b 1 1 g\n" {
		t.Errorf("listing = %q, want the re-registered name back at the front", out)
	}
	// And a name removed and registered again is at the front too, which is
	// the same rule reached through the removal.
	out, _ = runZsh(t, t.TempDir(),
		"g(){ :; }\nfunctions -M a 1 1 g\nfunctions -M b 1 1 g\nfunctions +M a\nfunctions -M a 1 1 g\nfunctions -M\n")
	if out != "functions -M a 1 1 g\nfunctions -M b 1 1 g\n" {
		t.Errorf("listing = %q, want the re-registered name back at the front", out)
	}
}

// TestOperandsAlwaysRegisterAndNeverFilterTheListing. `functions -M mf` looks
// like a filtered listing and is not one: it registers `mf` over whatever was
// there, with the default arity and itself as the implementation. Measured,
// and it is why a `*` in the operand is a bad name rather than a pattern.
func TestOperandsAlwaysRegisterAndNeverFilterTheListing(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"g(){ :; }\nfunctions -M mf 0 3 g\nfunctions -M mf\necho $(( mf(1,2,3) ))\n")
	if !strings.Contains(out, "no such function: mf") {
		t.Errorf("output = %q, want the registration replaced", out)
	}
	if strings.Contains(out, "functions -M mf 0 3 g") {
		t.Errorf("output = %q, want no listing from an operand", out)
	}
	out, _ = runZsh(t, t.TempDir(), "g(){ :; }\nfunctions -M 'mf*'\necho st=$?\n")
	if !strings.Contains(out, "-M mf*: bad math function name") || !strings.Contains(out, "st=1\n") {
		t.Errorf("output = %q, want a pattern refused as a name", out)
	}
}

// TestPlusMRemovesARegistration, which is the spelling — `unfunction -M` is
// not, and stays a bad option under that word.
func TestPlusMRemovesARegistration(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"g(){ :; }\nfunctions -M mf 1 1 g\nfunctions +M mf\nfunctions -M\necho $(( mf(1) ))\n")
	if !strings.Contains(out, "unknown function: mf") {
		t.Errorf("output = %q, want the registration gone", out)
	}
	if strings.Contains(out, "functions -M mf") {
		t.Errorf("output = %q, want nothing left in the listing", out)
	}
	// Several at once, a name never registered among them, and it is silent
	// at 0 — unlike `unfunction`, which reports a name it does not hold.
	out, _ = runZsh(t, t.TempDir(),
		"g(){ :; }\nfunctions -M a 1 1 g\nfunctions -M b 1 1 g\nfunctions +M a nosuch b\necho st=$?\nfunctions -M\n")
	if out != "st=0\n" {
		t.Errorf("output = %q, want both gone quietly at 0", out)
	}
	// `+M` with no operand removes nothing and says nothing.
	out, _ = runZsh(t, t.TempDir(), "g(){ :; }\nfunctions -M mf 1 1 g\nfunctions +M\nfunctions -M\n")
	if out != "functions -M mf 1 1 g\n" {
		t.Errorf("output = %q, want the registration untouched", out)
	}
}

// TestARegistrationIsNotAFunction. It is in neither the `functions` listing
// nor the `$functions` associative array, which is what says the two tables
// are separate rather than one with a flag.
func TestARegistrationIsNotAFunction(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"g(){ :; }\nfunctions -M mf 1 1 g\nfunctions\necho ${+functions[mf]} ${+functions[g]}\n")
	if strings.Contains(out, "mf () {") {
		t.Errorf("output = %q, want no math function in the function listing", out)
	}
	if !strings.Contains(out, "0 1\n") {
		t.Errorf("output = %q, want the math name absent from $functions", out)
	}
}

// TestARegistrationMadeInASubshellIsNotTheParents — the table is inherited and
// owned, the way the function table is.
func TestARegistrationMadeInASubshellIsNotTheParents(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(),
		"g(){ return 5; }\nfunctions -M mf 1 1 g\n( functions -M x 1 1 g; functions +M mf )\nfunctions -M\n")
	if out != "functions -M mf 1 1 g\n" {
		t.Errorf("output = %q, want the subshell's registration and removal both left behind", out)
	}
	// And the parent's is the subshell's to call.
	out, _ = runZsh(t, t.TempDir(),
		"g(){ return 5; }\nfunctions -M mf 1 1 g\n( echo $(( mf(1) )) )\n")
	if out != "5\n" {
		t.Errorf("output = %q, want the registration inherited", out)
	}
}

// TestTheCallFormNeedsTheParenthesisTouchingTheName. A space between them is
// not a call in the shell that has one either, which is what makes the grammar
// additive: turning it on changes nothing about text the other five accept.
func TestTheCallFormNeedsTheParenthesisTouchingTheName(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "g(){ :; }\nfunctions -M mf 1 1 g\necho $(( mf ( 5 ) ))\n")
	if !strings.Contains(out, "operator expected") {
		t.Errorf("output = %q, want the space to break the call", out)
	}
	// And a registered name written without parentheses is an ordinary
	// variable reference, which is 0 when nothing is stored under it.
	out, _ = runZsh(t, t.TempDir(), "g(){ return 7; }\nfunctions -M mf 1 1 g\necho $(( mf ))\n")
	if out != "0\n" {
		t.Errorf("output = %q, want a bare name read as a variable", out)
	}
}

// TestTheLetterIsExclusive. `-M` with any letter but `-m` is refused; with
// `-m` the shell does nothing at all, quietly — which is its own answer and
// not a registration, because registering there is the plausible wrong answer
// and nothing would say it had happened.
func TestTheLetterIsExclusive(t *testing.T) {
	for _, word := range []string{"functions -Mm", "functions -mM", "functions -M -m"} {
		out, st := runZsh(t, t.TempDir(), "g(){ :; }\n"+word+" mf 1 1 g\necho st=$?\nfunctions -M\n")
		if out != "st=0\n" || st != 0 {
			t.Errorf("%s: output = %q status %d, want nothing done at 0", word, out, st)
		}
	}
	// Any other letter with it is refused, and no registration is made.
	out, _ := runZsh(t, t.TempDir(), "g(){ :; }\nfunctions -Mt mf 1 1 g\nfunctions -M\n")
	if strings.Contains(out, "functions -M mf") {
		t.Errorf("output = %q, want no registration from a refused invocation", out)
	}
	if !strings.Contains(out, "not implemented yet") {
		t.Errorf("output = %q, want the other letter refused by name", out)
	}
}

// TestTheDoubleDashSpellingTheLoaderUses. The plugin manager on the rc file
// this shell has to run writes `functions -M -- name min max impl`, and
// silences it — so the `--` has to be stepped over rather than read as the
// name.
func TestTheDoubleDashSpellingTheLoaderUses(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"-zi_scheduler_add_sh(){ local idx=\"$1\"\n"+
			"  if [[ $idx -gt 2 ]]; then return 1; else return idx; fi\n}\n"+
			"functions -M -- zi_scheduler_add 1 1 -zi_scheduler_add_sh 2>/dev/null\n"+
			"functions -M\n"+
			"echo $(( zi_scheduler_add(2) )) $(( zi_scheduler_add(5) ))\n")
	if !strings.Contains(out, "functions -M zi_scheduler_add 1 1 -zi_scheduler_add_sh\n") {
		t.Errorf("output = %q, want the registration to have happened", out)
	}
	// The two branches must differ, and neither is the argument: a reading
	// that returned the last argument would give 2 and 5 here, and the
	// scheduler would pick the wrong task.
	if !strings.Contains(out, "2 1\n") || st != 0 {
		t.Errorf("output = %q status %d, want 2 and 1 from the two branches", out, st)
	}
}

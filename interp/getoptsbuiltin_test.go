// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"

	. "github.com/blairham/sh/interp"
)

func getoptsSem() Semantics {
	s := CoreSemantics()
	s.GetoptsClearsOptarg = No
	s.GetoptsEmptiesOptargForAnArgumentlessOption = No
	s.GetoptsUnsetsOptargAtEndOfOptions = No
	s.GetoptsClearingOptargIsARealUnset = No
	s.GetoptsAssignmentRestartsWord = Yes
	return s
}

// The parts every shell agrees on, which is nearly all of getopts — and the
// shape that reported success while doing nothing when this was a separate
// program that could not reach the shell's variables.
func TestGetoptsTheUnanimousParts(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the loop runs its body",
			`set -- -a -b x; while getopts "ab:" o; do echo "[$o:${OPTARG-}]"; done; echo "ind=$OPTIND"`,
			"[a:]\n[b:x]\nind=4\n",
		},
		{"clustered", `set -- -ab; while getopts "ab" o; do printf "[%s]" "$o"; done; echo " ind=$OPTIND"`, "[a][b] ind=2\n"},
		{"attached argument", `set -- -bval; getopts "b:" o; echo "[$o][$OPTARG]"`, "[b][val]\n"},
		{"separate argument", `set -- -b val; getopts "b:" o; echo "[$o][$OPTARG]"`, "[b][val]\n"},
		{"double dash ends it", `set -- -a -- -b; while getopts "ab" o; do printf "[%s]" "$o"; done; echo " ind=$OPTIND"`, "[a] ind=3\n"},
		{"a non-option ends it", `set -- -a f -b; while getopts "ab" o; do printf "[%s]" "$o"; done; echo " ind=$OPTIND"`, "[a] ind=2\n"},
		{"explicit operands", `getopts "ab" o -a; echo "[$o]"`, "[a]\n"},
		{"silent unknown", `set -- -z; getopts ":ab" o; echo "[$o][$OPTARG]"`, "[?][z]\n"},
		{"silent missing argument", `set -- -b; getopts ":b:" o; echo "[$o][$OPTARG]"`, "[:][b]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsSem()
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestOptindHasItsStartupValueBeforeGetoptsRuns. The index the next scan will
// read is 1 from the moment the shell starts, not from the first call to the
// builtin — unanimous, so it is a starting value rather than an axis.
//
// The two halves are separate failures. A shell that leaves the variable
// unset answers a script that tests it before its loop with an empty string
// where a number belongs, and one that lets the *environment* answer starts
// the scan wherever its caller had got to. Only the second needs an
// environment to see, which is why it is asserted here rather than left to
// the first assertion passing for the wrong reason.
func TestOptindHasItsStartupValueBeforeGetoptsRuns(t *testing.T) {
	const src = `echo "[${OPTIND-unset}]"`
	for _, tc := range []struct {
		name string
		env  []string
	}{
		{"nothing inherited", nil},
		{"an inherited value is overwritten", []string{"OPTIND=7"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsSem()
			out, _ := run(t, src, func(r *Runner) {
				r.Semantics = &sem
				r.Env = append(r.Env, tc.env...)
			})
			if out != "[1]\n" {
				t.Errorf("got %q, want %q", out, "[1]\n")
			}
		})
	}
}

// TestTheStartupOptindIsAStartingValueAndNotAFloor. A starting value is
// written once, before anything has run, and a chunk that arrives afterwards
// must not have it written again — or a front end reading its input a line at
// a time would reset the scan between the two lines of `getopts … ; echo`.
//
// Every corpus case is one chunk, so nothing outside this can tell a value
// supplied at startup from one re-supplied on every chunk; the whole
// difference is only visible through RunPart.
func TestTheStartupOptindIsAStartingValueAndNotAFloor(t *testing.T) {
	var out bytes.Buffer
	sem := getoptsSem()
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out, Semantics: &sem})
	for _, src := range []string{`set -- -a -b; getopts ab o`, `getopts ab o`, `echo "[$OPTIND][$o]"`} {
		f, err := syntax.Parse(src, syntax.Core())
		if err != nil {
			t.Fatal(err)
		}
		if err := r.RunPart(context.Background(), f); err != nil {
			t.Fatal(err)
		}
	}
	if got := out.String(); got != "[3][b]\n" {
		t.Errorf("got %q, want the scan to have carried across the chunks", got)
	}
}

// A bad option leaves OPTARG unset in three of the four and empty in one,
// which a script testing `${OPTARG-}` can tell apart.
func TestGetoptsClearsOptargIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"unset", No, "[gone]\n"},
		{"emptied", Yes, "[]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsSem()
			sem.GetoptsClearsOptarg = tc.answer
			dg := Diagnostics{GetoptsBadOption: "bad -%[1]s"}
			out, _ := run(t, `set -- -z; getopts "ab" o 2>/dev/null; echo "[${OPTARG-gone}]"`,
				func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The two complaints, and the three amounts of prefix they are printed with.
func TestGetoptsComplaintsAreLocatedThreeWays(t *testing.T) {
	sem := getoptsSem()
	for _, tc := range []struct {
		name string
		dg   Diagnostics
		want string
	}{
		{"the shell's usual location", Diagnostics{
			Location: LocationColonLine, GetoptsBadOption: "bad -%[1]s",
		}, "testsh: 1: bad -z\n"},
		{"its name and no line", Diagnostics{
			Location: LocationColonLine, GetoptsBadOption: "bad -%[1]s", GetoptsNamesNoLine: true,
		}, "testsh: bad -z\n"},
		{"nothing in front at all", Diagnostics{
			Location: LocationColonLine, GetoptsBadOption: "bad -%[1]s", GetoptsUnprefixed: true,
		}, "bad -z\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dg := tc.dg
			out, _ := run(t, `set -- -z; getopts "ab" o`, func(r *Runner) {
				r.Semantics, r.Diagnostics, r.Name = &sem, &dg, "testsh"
			})
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}

	// A missing argument is the other complaint and uses the other wording.
	dg := Diagnostics{GetoptsMissingArgument: "needs an argument: %[1]s", GetoptsUnprefixed: true}
	out, _ := run(t, `set -- -b; getopts "b:" o`, func(r *Runner) {
		r.Semantics, r.Diagnostics = &sem, &dg
	})
	if !strings.Contains(out, "needs an argument: b") {
		t.Errorf("got %q, want the missing-argument wording", out)
	}
}

// Resetting OPTIND is how a script starts a second scan — and it is the
// *assignment* that restarts the word, not the number, since the value it
// writes is often the one OPTIND already held.
func TestGetoptsAssignmentRestartsWordIsAnAxis(t *testing.T) {
	const src = `set -- -ab; getopts "ab" o; OPTIND=1; getopts "ab" o; echo "[$o]"`
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"restarts", Yes, "[a]\n"},
		{"carries on", No, "[b]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsSem()
			sem.GetoptsAssignmentRestartsWord = tc.answer
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestALeadingDashIsTheOptstringOrAnOption. `getopts` has no options at all
// here, so any leading `-` word is the question — refused as an option under
// one answer, taken as the optstring under the other.
func TestALeadingDashIsTheOptstringOrAnOption(t *testing.T) {
	refuse := func(r *Runner) {
		s := *r.Semantics
		s.GetoptsRejectsUnknownOption = Yes
		r.Semantics = &s
		dg := Diagnostics{BuiltinBadOption: "getopts: %[2]s: invalid option"}
		r.Diagnostics = &dg
	}
	out, _ := run(t, `getopts -a x`, refuse)
	if !strings.Contains(out, "-a: invalid option") {
		t.Errorf("refused: got %q", out)
	}
	// A bundle is named by its first letter here too.
	out, _ = run(t, `getopts --version x`, refuse)
	if !strings.Contains(out, "--: invalid option") {
		t.Errorf("bundle: got %q, want the first letter named", out)
	}

	take := func(r *Runner) {
		s := *r.Semantics
		s.GetoptsRejectsUnknownOption = No
		r.Semantics = &s
	}
	out, _ = run(t, `getopts -a x`, take)
	if strings.Contains(out, "invalid option") {
		t.Errorf("taken as the optstring: got %q, want no option complaint", out)
	}
}

// TestAnOrdinaryGetoptsAsksNothing, so a shell with no answer still parses
// options — the question is only about a leading dash.
func TestAnOrdinaryGetoptsAsksNothing(t *testing.T) {
	out, _ := run(t, `set -- -a; getopts ab o; echo "[$o]"`, func(r *Runner) {
		s := CoreSemantics()
		r.Semantics = &s
	})
	if !strings.Contains(out, "[a]") {
		t.Errorf("got %q, want an ordinary use to need no answer", out)
	}
	if strings.Contains(out, "refused as an option") {
		t.Errorf("got %q, want the leading-dash question not asked", out)
	}
}

// TestDashDashEndsGetoptsOptions, so `getopts -- ab o` reads `ab` as the
// optstring rather than refusing it — the control for the case above, and
// the shape a script uses when its optstring might begin with a dash.
func TestDashDashEndsGetoptsOptions(t *testing.T) {
	out, _ := run(t, `set -- -a; getopts -- ab o; echo "[$o]"`, func(r *Runner) {
		s := *r.Semantics
		s.GetoptsRejectsUnknownOption = Yes
		r.Semantics = &s
	})
	if !strings.Contains(out, "[a]") {
		t.Errorf("got %q, want -- taken as the end of the options", out)
	}
}

// getoptsFrozenSem is getoptsSem plus the four answers a freeze on one of
// `getopts`'s own names reaches, set to the shape bash measures: the freeze is
// consulted, the refusal is reported, and neither the builtin nor the script
// gives anything up — wherever inside the builtin the refusal was reached.
func getoptsFrozenSem() Semantics {
	s := getoptsSem()
	s.GetoptsOwnParametersIgnoreAFreeze = No
	s.GetoptsRefusedWriteEndsTheBuiltin = No
	s.GetoptsFrozenNameAtTheEndOfTheOptionsIsFatal = No
	s.ReadonlyRefusalInABuiltinIsFatal = No
	s.ReadonlyReassignmentFatal = No
	s.FatalErrorStatusIsOne = Yes
	return s
}

// TestGetoptsRefusedWriteKeepsTheRestOfTheLine. A builtin filling in its own
// output parameter is neither a bare assignment nor a declaration, and it was
// taking the bare assignment's answer: the refusal gave up what the shell was
// running, so the `echo` after the `;` never happened (#3147).
func TestGetoptsRefusedWriteKeepsTheRestOfTheLine(t *testing.T) {
	sem := getoptsFrozenSem()
	dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
	out, _ := run(t, `set -- -a val; readonly OPTARG; getopts "a:" o; echo "reached [$o]"; echo after`,
		func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
	if want := "sh: OPTARG: frozen\nreached [a]\nafter\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestGetoptsOwnParametersIgnoreAFreezeIsAnAxis. OPTARG and OPTIND are the
// builtin's own in ksh93 and zsh, and a `readonly` on either is not consulted
// there at all — where bash, dash and BusyBox ash refuse the write and say so.
func TestGetoptsOwnParametersIgnoreAFreezeIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"consulted", No, "sh: OPTARG: frozen\n[a][gone]\n"},
		{"written through", Yes, "[a][val]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsFrozenSem()
			sem.GetoptsOwnParametersIgnoreAFreeze = tc.answer
			dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
			out, _ := run(t, `set -- -a val; readonly OPTARG; getopts "a:" o; echo "[$o][${OPTARG-gone}]"`,
				func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// And the freeze is put back afterwards: writing through one is a single
// store's exemption and not the end of the name being frozen.
func TestGetoptsWritingThroughAFreezeLeavesItOn(t *testing.T) {
	sem := getoptsFrozenSem()
	sem.GetoptsOwnParametersIgnoreAFreeze = Yes
	dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
	out, _ := run(t, `set -- -a val; readonly OPTARG; getopts "a:" o
OPTARG=mine
echo "[$OPTARG]"`,
		func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
	if want := "sh: OPTARG: frozen\n[val]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestGetoptsRefusedWriteEndsTheBuiltinIsAnAxis. dash and BusyBox ash stop at
// the refusal, with the name unwritten and OPTIND still where it was; bash
// reports it and writes the rest anyway. OPTIND is the half that says which
// of the two happened.
func TestGetoptsRefusedWriteEndsTheBuiltinIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"reports only", No, "sh: OPTARG: frozen\nst=0 [a] ind=3\n"},
		{"ends it", Yes, "sh: OPTARG: frozen\nst=2 [none] ind=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsFrozenSem()
			sem.GetoptsRefusedWriteEndsTheBuiltin = tc.answer
			dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
			out, _ := run(t, `set -- -a val; o=none; readonly OPTARG; getopts "a:" o; echo "st=$? [$o] ind=$OPTIND"`,
				func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestReadonlyRefusalInABuiltinIsFatalIsAnAxis. zsh alone ends the script over
// a builtin's refused write, and it is the same zsh that ends one for a plain
// assignment — which is what keeps this apart from ReadonlyReassignmentFatal,
// answered Yes by three shells that carry on here.
func TestReadonlyRefusalInABuiltinIsFatalIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"reports and carries on", No, "sh: OPTARG: frozen\nafter\n"},
		{"ends the script", Yes, "sh: OPTARG: frozen\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsFrozenSem()
			sem.ReadonlyRefusalInABuiltinIsFatal = tc.answer
			dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
			out, _ := run(t, `set -- -a val; readonly OPTARG; getopts "a:" o; echo after`,
				func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestGetoptsRefusedNameCostsTheBuiltinItsStatus. The name is the one write
// whose refusal ends the builtin wherever it is reached — there is a letter it
// cannot report — except at the end of the options, where there is no letter
// and the 1 that says so stands.
func TestGetoptsRefusedNameCostsTheBuiltinItsStatus(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a letter it cannot report",
			`set -- -a val; o=kept; readonly o; getopts "a:" o; echo "st=$? [$o]"`,
			"sh: o: frozen\nst=2 [kept]\n",
		},
		{
			"no letter to report",
			`set -- operand; o=kept; readonly o; getopts "a:" o; echo "st=$? [$o]"`,
			"sh: o: frozen\nst=1 [kept]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsFrozenSem()
			dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// And where the dialect stops the builtin over a refusal it stops it here too,
// so the end of the options is 2 rather than 1.
func TestGetoptsRefusedNameAtTheEndFollowsTheAxis(t *testing.T) {
	sem := getoptsFrozenSem()
	sem.GetoptsRefusedWriteEndsTheBuiltin = Yes
	dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
	out, _ := run(t, `set -- operand; o=kept; readonly o; getopts "a:" o; echo "st=$? [$o]"`,
		func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
	if want := "sh: o: frozen\nst=2 [kept]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestGetoptsClearingOptargMeetsAFreeze. Taking the value away is a write as
// far as a freeze is concerned: the clearing after an option that takes no
// argument is refused, reported, and leaves the value standing — where the
// same line over an unfrozen name leaves OPTARG unset.
func TestGetoptsClearingOptargMeetsAFreeze(t *testing.T) {
	sem := getoptsFrozenSem()
	dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
	out, _ := run(t, `set -- -b; OPTARG=PRE; readonly OPTARG; getopts "a:b" o; echo "[$o][${OPTARG-gone}]"`,
		func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
	if want := "sh: OPTARG: frozen\n[b][PRE]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestGetoptsBadOptionComplainsBeforeRefusingTheName. Two sentences and an
// order: every column that writes both writes the option's first. The one that
// reverses it is the one whose refusal ends the script, and it never writes
// the second at all.
func TestGetoptsBadOptionComplainsBeforeRefusingTheName(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"the complaint first", No, "sh: bad -z\nsh: o: frozen\nafter\n"},
		{"the fatal refusal instead", Yes, "sh: o: frozen\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsFrozenSem()
			sem.ReadonlyRefusalInABuiltinIsFatal = tc.answer
			dg := Diagnostics{ReadonlyVariable: "%s: frozen", GetoptsBadOption: "bad -%[1]s"}
			out, _ := run(t, `set -- -z; o=kept; readonly o; getopts "a:" o; echo after`,
				func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestGetoptsRefusalNamesTheBuiltin. A builtin filling in its own output
// parameter names itself in the sentence where the dialect names one — `dash`
// writes `getopts: OPTARG: is read only` through the same wording it writes
// `export: x: is read only` with — which a bare assignment's form never
// reaches, because that wording is a declaration's.
func TestGetoptsRefusalNamesTheBuiltin(t *testing.T) {
	sem := getoptsFrozenSem()
	dg := Diagnostics{
		ReadonlyVariable:              "%s: frozen",
		ReadonlyVariableInDeclaration: "%[2]s: %[1]s: frozen",
		ReadonlyRefusalNamesBuiltin:   map[string]bool{"getopts": true},
	}
	out, _ := run(t, `set -- -a val; readonly OPTARG; getopts "a:" o; echo "[$o]"`,
		func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
	if want := "sh: getopts: OPTARG: frozen\n[a]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestGetoptsUnsetsOptargAtEndOfOptionsIsAnAxis. The call that reports "no
// more options" is also the one that clears OPTARG in five of the seven
// columns; the other two leave the last option's argument standing, so a
// script reading `$OPTARG` after its `while getopts` loop gets the previous
// option's value there (#3146).
func TestGetoptsUnsetsOptargAtEndOfOptionsIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"the last argument stands", No, "st=1 [?] OPTARG=[val]\n"},
		{"cleared with the scan", Yes, "st=1 [?] OPTARG=[gone]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsSem()
			sem.GetoptsUnsetsOptargAtEndOfOptions = tc.answer
			out, _ := run(t, `set -- -a val x; getopts "a:" o; getopts "a:" o; echo "st=$? [$o] OPTARG=[${OPTARG-gone}]"`,
				func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// And it happens on every call that runs out, not only the first: a second
// `getopts` after the loop has ended finds nothing to report and clears again.
func TestGetoptsClearsOptargEveryTimeItRunsOut(t *testing.T) {
	sem := getoptsSem()
	sem.GetoptsUnsetsOptargAtEndOfOptions = Yes
	out, _ := run(t, `set -- -a val x
getopts "a:" o
getopts "a:" o
OPTARG=again
getopts "a:" o
echo "st=$? OPTARG=[${OPTARG-gone}]"`, func(r *Runner) { r.Semantics = &sem })
	if want := "st=1 OPTARG=[gone]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestGetoptsClearingOptargIsARealUnsetIsAnAxis. A `delete` from the value map
// is not an unset: it leaves the name's attributes where they were, so a
// `readonly OPTARG` neither stopped the clearing nor was taken away by it.
// bash removes the name, and removing a name removes what was recorded about
// it — so an assignment made after the loop is taken where it was refused
// before.
func TestGetoptsClearingOptargIsARealUnsetIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"an ordinary write the freeze refuses", No, "sh: OPTARG: frozen\nend OPTARG=[PRESET]\nsh: OPTARG: frozen\nafter OPTARG=[PRESET]\n"},
		{"the name is taken away", Yes, "end OPTARG=[gone]\nafter OPTARG=[written]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsFrozenSem()
			sem.GetoptsUnsetsOptargAtEndOfOptions = Yes
			sem.GetoptsClearingOptargIsARealUnset = tc.answer
			dg := Diagnostics{ReadonlyVariable: "%s: frozen"}
			out, _ := run(t, `OPTARG=PRESET
set -- operand
readonly OPTARG
getopts "a:" o
echo "end OPTARG=[${OPTARG-gone}]"
OPTARG=written
echo "after OPTARG=[${OPTARG-gone}]"`, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The error paths ask the same question, and the one clearing that does not is
// the option that takes no argument: bash goes through the ordinary refusal
// there, reports it and leaves the value standing, where the same shell is
// silent on the three rows above.
func TestGetoptsRealUnsetDoesNotReachTheArgumentlessClearing(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a bad option", `set -- -z; getopts "a:b" o`, "sh: bad -z\nOPTARG=[gone]\n"},
		{"a missing argument", `set -- -a; getopts "a:b" o`, "sh: need -a\nOPTARG=[gone]\n"},
		{"no argument wanted", `set -- -b; getopts "a:b" o`, "sh: OPTARG: frozen\nOPTARG=[PRESET]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsFrozenSem()
			sem.GetoptsClearingOptargIsARealUnset = Yes
			dg := Diagnostics{
				ReadonlyVariable:       "%s: frozen",
				GetoptsBadOption:       "bad -%[1]s",
				GetoptsMissingArgument: "need -%[1]s",
			}
			out, _ := run(t, "OPTARG=PRESET\nreadonly OPTARG\n"+tc.src+"\necho \"OPTARG=[${OPTARG-gone}]\"",
				func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestGetoptsClearingComesBeforeTheWordCount. The order the two writes happen
// in is invisible until one of them is refused, and then it is the whole of
// the difference between "reported" and "stopped": dash and BusyBox ash refuse
// a frozen OPTARG with OPTIND still at its old value, so neither had counted
// past the word.
func TestGetoptsClearingComesBeforeTheWordCount(t *testing.T) {
	sem := getoptsFrozenSem()
	sem.GetoptsRefusedWriteEndsTheBuiltin = Yes
	dg := Diagnostics{ReadonlyVariable: "%s: frozen", GetoptsBadOption: "bad -%[1]s"}
	out, _ := run(t, `set -- -z -z; OPTIND=1; readonly OPTARG; getopts "a:b" o; echo "st=$? ind=$OPTIND"`,
		func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
	if want := "sh: bad -z\nsh: OPTARG: frozen\nst=2 ind=1\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestGetoptsWritesOptargBeforeTheWordCount. One order on every path, and it
// is invisible until a freeze refuses the OPTARG half: dash and BusyBox ash
// then report and stop with OPTIND still where it was, which is what says the
// builtin never counted past the word. Each of the four paths writes OPTARG
// its own way, so each is its own row.
func TestGetoptsWritesOptargBeforeTheWordCount(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an option that takes one",
			`set -- -a val`,
			"sh: OPTARG: frozen\nst=2 ind=1\n",
		},
		{
			"an option that takes none",
			`set -- -b`,
			"sh: OPTARG: frozen\nst=2 ind=1\n",
		},
		{
			"a bad option in silent mode",
			`set -- -z; spec=':a:b'`,
			"sh: OPTARG: frozen\nst=2 ind=1\n",
		},
		{
			"the end of the options, past a --",
			`set -- -- operand`,
			"sh: OPTARG: frozen\nst=2 ind=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := getoptsFrozenSem()
			sem.GetoptsRefusedWriteEndsTheBuiltin = Yes
			sem.GetoptsUnsetsOptargAtEndOfOptions = Yes
			dg := Diagnostics{ReadonlyVariable: "%s: frozen", GetoptsBadOption: "bad -%[1]s"}
			src := "spec='a:b'\nOPTIND=1\nOPTARG=PRESET\n" + tc.src + "\nreadonly OPTARG\n" +
				`getopts "$spec" o; echo "st=$? ind=$OPTIND"`
			out, _ := run(t, src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, &dg })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

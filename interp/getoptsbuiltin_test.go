// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

func getoptsSem() Semantics {
	s := CoreSemantics()
	s.GetoptsClearsOptarg = No
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

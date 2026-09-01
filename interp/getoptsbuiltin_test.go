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

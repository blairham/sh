// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a shell function call does to the `getopts` scan position, which is
// three answers and not two: the position has two halves — `OPTIND`, which
// counts words, and how far into a clustered word the letters have been read —
// and the panel splits over which of them a call gets to itself.
//
// See Semantics.GetoptsFunctionPosition for the measurements.

func positionSem(p GetoptsFunctionPositionPolicy) Semantics {
	s := getoptsSem()
	s.GetoptsFunctionPosition = p
	// The neighbor these cases walk past. A declaration is what that axis is
	// about and none of these makes one, so the answer only has to be *an*
	// answer — an unanswered one would print beside the output under test.
	s.GetoptsLocalOptindRestoresTheCursor = No
	return s
}

// The row the third answer was added for: a helper function that parses its
// own options and is called twice.
//
// Under the shared answer the second call starts where the first stopped and
// reads nothing, which is why every such helper in those shells begins by
// resetting OPTIND. Under either local answer it reads its arguments again.
// The failure is silent — the options are simply not seen the second time —
// which is what makes it worth an axis rather than a note.
func TestAnOptionParsingFunctionCalledTwice(t *testing.T) {
	const src = `g() { while getopts "ab" o "$@" >/dev/null 2>&1; do printf "%s " "$o"; done; ` +
		`printf "end=%s " "$OPTIND"; }` + "\n" + `OPTIND=1` + "\n" + `g -a -b` + "\n" + `g -a -b` + "\n" + `echo`
	for _, tc := range []struct {
		position GetoptsFunctionPositionPolicy
		want     string
	}{
		{GetoptsFunctionPositionIsShared, "a b end=3 end=3 \n"},
		{GetoptsFunctionPositionIsTheCallsOwn, "a b end=3 a b end=3 \n"},
		{GetoptsFunctionPositionIsLocal, "a b end=3 a b end=3 \n"},
	} {
		t.Run(tc.position.String(), func(t *testing.T) {
			sem := positionSem(tc.position)
			out, st := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// And the probe that separates the two answers which both re-read: whether
// `OPTIND` is the call's as well as the scan.
//
// No `getopts` in the callee at all, which is what makes this about the
// parameter rather than about the builtin: the call reads the name on the way
// in and writes it, and the caller reads it afterwards.
func TestWhetherOptindItselfIsTheCallsOwn(t *testing.T) {
	const src = `g() { printf "entry=%s " "$OPTIND"; OPTIND=7; }` + "\n" +
		`OPTIND=3` + "\n" + `g` + "\n" + `printf "after=%s\n" "$OPTIND"`
	for _, tc := range []struct {
		position GetoptsFunctionPositionPolicy
		want     string
	}{
		{GetoptsFunctionPositionIsShared, "entry=3 after=7\n"},
		// The scan is the call's and the parameter is the shell's, so this
		// row is the shared one's word for word — which is the whole of what
		// separates it from the answer below.
		{GetoptsFunctionPositionIsTheCallsOwn, "entry=3 after=7\n"},
		{GetoptsFunctionPositionIsLocal, "entry=1 after=3\n"},
	} {
		t.Run(tc.position.String(), func(t *testing.T) {
			sem := positionSem(tc.position)
			out, st := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// What the *caller* comes back to, with a callee that scanned words of its
// own. This is the half a reset-on-empty-scan would get wrong: the caller had
// read one word, the callee left OPTIND at 3, and the two local answers hand
// the caller its own word 2 back where the shared one reads word 3.
func TestTheCallerComesBackToItsOwnWord(t *testing.T) {
	const src = `set -- -a -b -c` + "\n" +
		`getopts abc o; printf "top1=%s,%s " "$o" "$OPTIND"` + "\n" +
		`g() { getopts abc q "$@" 2>/dev/null; getopts abc q "$@" 2>/dev/null; }` + "\n" +
		`g -b -c` + "\n" +
		`getopts abc o; printf "top2=%s,%s\n" "$o" "$OPTIND"`
	for _, tc := range []struct {
		position GetoptsFunctionPositionPolicy
		want     string
	}{
		{GetoptsFunctionPositionIsShared, "top1=a,2 top2=c,4\n"},
		{GetoptsFunctionPositionIsTheCallsOwn, "top1=a,2 top2=b,3\n"},
		{GetoptsFunctionPositionIsLocal, "top1=a,2 top2=b,3\n"},
	} {
		t.Run(tc.position.String(), func(t *testing.T) {
			sem := positionSem(tc.position)
			out, st := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// A dialect that has not answered is told so rather than guessed at, and told
// so where a call can be distinguished by it.
func TestAnUnansweredFunctionPositionIsReported(t *testing.T) {
	sem := positionSem(GetoptsFunctionPositionUnspecified)
	const src = `set -- -a -b` + "\n" + `getopts ab o` + "\n" + `g() { :; }` + "\n" + `g`
	out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
	// The sentence and not the status: the refusal is the *call's*, and the
	// call is not the last command a script runs — `g` returns 0 over it,
	// exactly as a refusal inside any other command would be overwritten by
	// what ran after it.
	if want := "the `getopts` cursor being local to a function"; !contains(out, want) {
		t.Errorf("out = %q, want %q in it", out, want)
	}
	// And a cursor the caller never moved is not distinguishable, so nothing
	// is said: an axis asked on the common path is one every script pays for.
	quiet, _ := run(t, "g() { :; }\ng", func(r *Runner) { r.Semantics = &sem })
	if quiet != "" {
		t.Errorf("out = %q, want nothing — a fresh cursor tells the answers apart nowhere", quiet)
	}
}

// OPTARG after an option the string *has* and that takes no argument is its
// own axis, because the columns line up differently from the bad-option row:
// reading one answer for both put one column on the wrong side of each.
func TestOptargAfterAnOptionThatTakesNoneIsItsOwnAxis(t *testing.T) {
	const src = `OPTARG=PRESET; OPTIND=1; getopts "ab:" o -a; ` +
		`echo "o=[$o] optarg=[${OPTARG-UNSET}] set=[${OPTARG+yes}]"`
	for _, tc := range []struct {
		empties Answer
		want    string
	}{
		{Yes, "o=[a] optarg=[] set=[yes]\n"},
		{No, "o=[a] optarg=[UNSET] set=[]\n"},
	} {
		t.Run(tc.empties.String(), func(t *testing.T) {
			sem := getoptsSem()
			sem.GetoptsEmptiesOptargForAnArgumentlessOption = tc.empties
			out, st := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// And the bad-option row is not moved by it: the two axes are independent, so
// a dialect can empty for one and unset for the other. That pairing is the one
// two columns actually have, and it is what a single axis could not express.
func TestTheTwoOptargRowsAreIndependent(t *testing.T) {
	sem := getoptsSem()
	sem.GetoptsClearsOptarg = No
	sem.GetoptsEmptiesOptargForAnArgumentlessOption = Yes
	out, st := run(t, `OPTARG=PRESET; OPTIND=1; getopts "a:" o -z 2>/dev/null; `+
		`echo "bad=[${OPTARG-UNSET}]"; `+
		`OPTARG=PRESET; OPTIND=1; getopts "ab:" o -a; echo "none=[${OPTARG-UNSET}]"`,
		func(r *Runner) { r.Semantics = &sem })
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
	if want := "bad=[UNSET]\nnone=[]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
}

// The answer keyed on the definition form, asked of the substrate with both
// forms in every row: a row with only the keyword form in it passes under
// GetoptsFunctionPositionIsLocal too, which scopes both (#3321).
func TestAKeywordFunctionsCursorIsItsOwnAndAPosixOnesIsShared(t *testing.T) {
	for _, tc := range []struct{ name, def, want string }{
		{"the keyword form", `function g { printf "entry=%s " "$OPTIND"; OPTIND=7; }`, "entry=1 after=3\n"},
		{"the POSIX form", `g() { printf "entry=%s " "$OPTIND"; OPTIND=7; }`, "entry=3 after=7\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := positionSem(GetoptsFunctionPositionIsLocalToAKeywordFunction)
			src := tc.def + "\n" + `OPTIND=3` + "\n" + `g` + "\n" + `printf "after=%s\n" "$OPTIND"`
			out, st := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if st != 0 {
				t.Errorf("status %d, want 0", st)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

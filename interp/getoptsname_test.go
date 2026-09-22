// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// getoptsNameRun is optRun with this builtin's own fatality turned off, so a
// row that is not about the fatality gets to see what happened after the
// refusal.
func getoptsNameRun(t *testing.T, tweak func(*Semantics), dg Diagnostics, src string) (string, int) {
	t.Helper()
	return optRun(t, func(s *Semantics) {
		s.BadNameToGetoptsFatal = No
		// The neighboring axes a successful scan reaches, answered flat so
		// that a row varies the name operand alone: the arrays a subscripted
		// operand lands in, and what OPTARG becomes after a letter that
		// takes no argument.
		arraySemantics(s)
		s.GetoptsEmptiesOptargForAnArgumentlessOption = No
		if tweak != nil {
			tweak(s)
		}
	}, dg, src)
}

// The bug: `getopts` took a word that is not an identifier as the name it
// writes into and said nothing about it (#3555).
//
// `getopts x 1bad -x` stored under a word no expansion can read back, moved
// OPTIND and reported 0, so the loop that went on to read the parameter saw an
// empty value and a status that says an option was found. It is the shape
// #3515 found at `printf -v` and #1440 at `read`, one builtin further along,
// and it is the last of them: `read`, `printf -v`, `unset`, `export`,
// `readonly` and `local` all refuse the same operands today.
//
// Measured 2026-09-18, `env -i PATH=/usr/bin:/bin LC_ALL=C`, stdin /dev/null,
// a script file, with `echo A` in front and `echo "B st=$?"` behind so that
// the fatality is visible. All seven columns refuse — bash 5.3.20, bash
// 3.2.57, bash as `sh`, ksh93u+ 2012-08-01, zsh 5.9.2, dash 0.5.12 and
// BusyBox ash 1.37.0 — in four wordings and two statuses, and `a-b` answers
// as `1bad` does in every one of them. So the refusal is the core's, and only
// the wording, the status, the fatality and how far the set of names reaches
// are the dialect's.
func TestAGetoptsNameOperandThatIsNotANameIsRefused(t *testing.T) {
	for _, operand := range []string{"1bad", "a-b", "a b", "a.b", ""} {
		src := `getopts x "` + operand + `" -x; echo "st=$?"`
		out, _ := getoptsNameRun(t, nil, Diagnostics{}, src)
		if !strings.Contains(out, "identifier") {
			t.Errorf("getopts %q: said %q, want the operand refused as a name", operand, out)
		}
		if !strings.Contains(out, "getopts") {
			t.Errorf("getopts %q: said %q, want the builtin named", operand, out)
		}
		if strings.Contains(out, "st=0") {
			t.Errorf("getopts %q: said %q, want a failure rather than an option found", operand, out)
		}
	}
}

// And nothing is stored under the word that was refused, which is the half a
// caller can see: the parameter the loop goes on to read stays as it was.
//
// **OPTIND is the other half and it is a dialect's**, which this case claimed
// it was not until 2026-09-21. It asserted `OPTIND=1` under "the scan never
// ran", and the scan does run in three of the five columns — see
// Semantics.GetoptsRefusedNameStillScans, which holds the re-measurement. The
// probe that had been taken as settling it printed the status and never
// printed OPTIND, so it could not have.
func TestARefusedGetoptsNameStoresNothing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scans Answer
		want  string
	}{
		{"the scan runs anyway", Yes, "OPTIND=2"},
		{"the name is judged first", No, "OPTIND=1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `bad=PRE; getopts x "1bad" -x; echo "st=$? bad=$bad OPTIND=$OPTIND"`
			out, _ := getoptsNameRun(t, func(s *Semantics) {
				s.GetoptsRefusedNameStillScans = tc.scans
			}, Diagnostics{}, src)
			if !strings.Contains(out, "bad=PRE") {
				t.Errorf("out %q, want the parameter left standing", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("out %q, want %s", out, tc.want)
			}
		})
	}
}

// The cursor moves because the *scan* ran, not because a refusal adds one.
//
// The discriminating case, and the reason this is not a fixed increment:
// with nothing for the option to be, bash leaves OPTIND where it was.
// Measured 2026-09-21, `getopts x 1bad foo` is `OPTIND=1` there while
// `getopts x 1bad -x` is `OPTIND=2`.
func TestARefusedGetoptsNameMovesNothingWithNoOptionToRead(t *testing.T) {
	src := `getopts x "1bad" foo; echo "st=$? OPTIND=$OPTIND"`
	out, _ := getoptsNameRun(t, func(s *Semantics) {
		s.GetoptsRefusedNameStillScans = Yes
	}, Diagnostics{}, src)
	if !strings.Contains(out, "OPTIND=1") {
		t.Errorf("out %q, want OPTIND unmoved — there was no option to consume", out)
	}
}

// Whether a *subscripted* operand is a name here is an axis of its own, and it
// is not the one `read` and `printf -v` ask.
//
// Measured 2026-09-18, a script file, with `o[1]` beside `o[0]` so that a
// refusal about the *position* is told apart from one about the brackets:
//
//	ksh93u+       getopts x 'o[1]' -x   0, and ${o[1]} holds the letter
//	zsh 5.9.2     the same              0, and $o holds the letter
//	bash 5.3.20   not a valid identifier, at 1
//	dash, ash     o[1]: bad variable name, at 2
//
// bash has arrays and fills `read 'a[1]'`, so StoreOperandTakesASubscript —
// which is Yes there — is the wrong gate for this builtin and would take the
// operand in the one column that refuses it. zsh's `o[0]` is `assignment to
// invalid subscript range`, this shell's one-based arrays refusing the
// position rather than the brackets: a probe that asked only `o[0]` would have
// read that column as a refusal.
func TestWhetherAGetoptsOperandTakesASubscriptIsItsOwnAxis(t *testing.T) {
	const src = `getopts x 'o[1]' -x; echo "st=$? one=[${o[1]}]"`
	out, _ := getoptsNameRun(t, func(s *Semantics) {
		s.GetoptsOperandTakesASubscript = Yes
	}, Diagnostics{}, src)
	if !strings.Contains(out, "st=0 one=[x]") {
		t.Errorf("taken: said %q, want the element filled at 0", out)
	}
	out, _ = getoptsNameRun(t, func(s *Semantics) {
		s.GetoptsOperandTakesASubscript = No
	}, Diagnostics{}, src)
	if !strings.Contains(out, "identifier") {
		t.Errorf("refused: said %q, want the brackets refused as a name", out)
	}
	if strings.Contains(out, "st=0") {
		t.Errorf("refused: said %q, want a failure", out)
	}
	// And the store is the one `read 'a[1]'` goes through rather than a
	// second copy: a parameter literally named `o[1]` is what this builtin
	// used to leave behind, and no expansion in a dialect with arrays can
	// read it back.
	out, _ = getoptsNameRun(t, func(s *Semantics) {
		s.GetoptsOperandTakesASubscript = Yes
	}, Diagnostics{}, `a=(p q r); getopts x 'a[1]' -x; echo "[${a[*]}]"`)
	if !strings.Contains(out, "[p x r]") {
		t.Errorf("said %q, want the element replaced in place", out)
	}
}

// What the refusal costs the script is the dialect's, and one column stops.
//
// zsh 5.9.2 ends the script on it, exactly as it does at `read` and at
// `printf -v`; the other six reach the line after it. Measured 2026-09-18 with
// the probe above.
func TestWhatARefusedGetoptsNameCostsTheScript(t *testing.T) {
	const src = `getopts x "1bad" -x` + "\n" + `echo after`
	out, _ := optRun(t, func(s *Semantics) {
		s.BadNameToGetoptsFatal = No
	}, Diagnostics{}, src)
	if !strings.Contains(out, "after") {
		t.Errorf("not fatal: said %q, want the next line to run", out)
	}
	out, _ = optRun(t, func(s *Semantics) {
		s.BadNameToGetoptsFatal = Yes
	}, Diagnostics{}, src)
	if strings.Contains(out, "after") {
		t.Errorf("fatal: said %q, want the script to stop", out)
	}
}

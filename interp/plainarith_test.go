// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// plainArithScript reads arithmetic expansions through every place a word is
// read whole — an assignment's value, quoted and not, a case subject — over
// the operators, the numbers each reading writes, a name read again as an
// expression, an unset name, and the failures. A division by zero runs in a
// subshell, so a refusal is part of the output both runs must agree on.
const plainArithScript = `p() { printf '<%s>' "$@"; echo; }
i=5; n=-3
x=$((i + 1)); p "$x"; x="$((i * n))"; p "$x"; x=$((i > 2 ? i : n)); p "$x"
x=$((i / 2)); p "$x"; x=$((-i)); p "$x"; x=$((~i)); p "$x"; x=$((!i)); p "$x"
x=$((i << 3 | 1)); p "$x"; x=$((i == 5 && n < 0)); p "$x"; x=$(( (i + n) * 2 )); p "$x"
x=$((9223372036854775807 + 1)); p "$x"; x=$((0x1F + 010)); p "$x"; x=$((z9 = 4, i)); p "$x"
x=$((nosuch + 1)); p "$x"; s='i + 1'; x=$((s * 2)); p "$x"; t=s; x=$((t)); p "$x"
case $((i % 3)) in 2) p two ;; *) p other ;; esac
(x=$((i / 0)); p "$x"); p "$?"
p end
`

// plainArithFailureKept is the one place a failed expansion's text is seen
// afterwards: where the shell gives up the line and carries on at the next,
// the name the failed assignment named is read there, and it still holds
// what it held before (#6544).
const plainArithFailureKept = `dz=4
dz=$((1 / 0))
printf '<%s>\n' "$dz"
`

// plainArithVectors are the readings the script runs under: the substrate's,
// a leading zero read as decimal, floats, and a failed expansion that gives
// up only its line.
func plainArithVectors() map[string]struct {
	script string
	rows   []string
	enable func(*syntax.Dialect)
	setup  func(*Runner)
} {
	with := func(set func(*Semantics)) func(*Runner) {
		return func(r *Runner) {
			sem := *r.Semantics
			set(&sem)
			r.Semantics = &sem
		}
	}
	common := []string{
		"<6>\n<-15>\n<5>\n<2>\n<-5>\n<-6>\n<0>\n<41>\n<1>\n<4>\n",
		"<5>\n<1>\n<12>\n<6>\n<two>\n",
		"<end>\n",
	}
	return map[string]struct {
		script string
		rows   []string
		enable func(*syntax.Dialect)
		setup  func(*Runner)
	}{
		"standard": {plainArithScript, append([]string{"<39>\n"}, common...), nil, nil},
		"leading zero decimal": {
			plainArithScript, append([]string{"<41>\n"}, common...), nil,
			with(func(s *Semantics) { s.ArithLeadingZeroIsOctal = No }),
		},
		"floats": {
			plainArithScript + "f=1.5; x=$((f * 2)); p \"$x\"; x=$((1.0 / 4)); p \"$x\"\n", common,
			func(d *syntax.Dialect) { d.ArithFloat = true }, nil,
		},
		"the line given up": {
			plainArithFailureKept,
			[]string{"division by zero\n<4>\n"},
			nil,
			with(func(s *Semantics) { s.FailedExpansionAbandonsTheLine = Yes }),
		},
	}
}

// TestAPlainArithmeticValueAnswersAsTheWordRoadDoes runs each of
// plainArithVectors with plainArithValue on and off, and requires the same
// output and status (#6496).
func TestAPlainArithmeticValueAnswersAsTheWordRoadDoes(t *testing.T) {
	for name, v := range plainArithVectors() {
		t.Run(name, func(t *testing.T) {
			run := func(on bool) (string, int) {
				was := SetPlainArithShortcutForTest(on)
				defer SetPlainArithShortcutForTest(was)
				return runGrammar(t, v.script, v.enable, v.setup)
			}
			full, fullStatus := run(false)
			short, shortStatus := run(true)
			if short != full || shortStatus != fullStatus {
				t.Errorf("with the shortcut:\n%s(status %d)\nwithout it:\n%s(status %d)", short, shortStatus, full, fullStatus)
			}
			for _, row := range v.rows {
				if !strings.Contains(full, row) {
					t.Errorf("the script did not reach %q:\n%s", row, full)
				}
			}
		})
	}
}

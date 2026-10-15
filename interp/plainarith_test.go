// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	. "github.com/blairham/sh/interp"
)

// plainArithScript reads arithmetic expansions through every place a word is
// read whole — an assignment's value, quoted and not, a case subject — over
// the operators, the numbers each dialect writes, a name read again as an
// expression, an unset name, and the failures. Floats and a division by zero
// run in subshells, so a refusal is part of the output both runs must agree on.
const plainArithScript = `p() { printf '<%s>' "$@"; echo; }
i=5; n=-3
x=$((i + 1)); p "$x"; x="$((i * n))"; p "$x"; x=$((i > 2 ? i : n)); p "$x"
x=$((i ** 2)); p "$x"; x=$((i / 2)); p "$x"; x=$((-i)); p "$x"; x=$((~i)); p "$x"; x=$((!i)); p "$x"
x=$((i << 3 | 1)); p "$x"; x=$((i == 5 && n < 0)); p "$x"; x=$(( (i + n) * 2 )); p "$x"
x=$((9223372036854775807 + 1)); p "$x"; x=$((0x1F + 010)); p "$x"; x=$((16#ff)); p "$x"
x=$((nosuch + 1)); p "$x"; s='i + 1'; x=$((s * 2)); p "$x"; t=s; x=$((t)); p "$x"
case $((i % 3)) in 2) p two ;; *) p other ;; esac
(f=1.5; x=$((f * 2)); p "$x"); (x=$((1.0 / 4)); p "$x")
(x=$((i / 0)); p "$x"); p "$?"
p end
`

// TestAPlainArithmeticValueAnswersAsTheWordRoadDoes runs plainArithScript with
// plainArithValue on and off, and requires the same output and status (#6496).
func TestAPlainArithmeticValueAnswersAsTheWordRoadDoes(t *testing.T) {
	for _, p := range []dialecttest.Preset{
		{Name: "bash", Dialect: bash.Dialect, Semantics: bash.Semantics, Diagnostics: bash.Diagnostics, Apply: bash.Apply},
		{Name: "zsh", Dialect: zsh.Dialect, Semantics: zsh.Semantics, Diagnostics: zsh.Diagnostics, Apply: zsh.Apply},
	} {
		t.Run(p.Name, func(t *testing.T) {
			run := func(on bool) (string, int) {
				was := SetPlainArithShortcutForTest(on)
				defer SetPlainArithShortcutForTest(was)
				out, st, err := p.Combined(t, dialecttest.Base{Dir: t.TempDir()}, plainArithScript)
				if err != nil {
					t.Fatalf("shortcut %v: %v", on, err)
				}
				return out, st
			}
			full, fullStatus := run(false)
			short, shortStatus := run(true)
			if short != full || shortStatus != fullStatus {
				t.Errorf("with the shortcut:\n%s(status %d)\nwithout it:\n%s(status %d)", short, shortStatus, full, fullStatus)
			}
			for _, row := range []string{"<6>\n<-15>\n<5>\n<25>\n<2>\n<-5>\n<-6>\n<0>\n<41>\n<1>\n<4>\n", "<255>\n<1>\n<12>\n<6>\n<two>\n", "<end>\n"} {
				if !strings.Contains(full, row) {
					t.Errorf("the script did not reach %q:\n%s", row, full)
				}
			}
		})
	}
}

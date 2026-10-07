// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// The same refusal at a dash prompt, which names what it expected where the
// word ended the body's read, and counts no refused line: `$LINENO` and the
// next refusal are numbered as the refused line was. Measured 2026-10-07 on
// dash 0.5.12 through a pseudo-terminal (#6319).
func TestADashPromptRefusesASubstitutionBodyAtTheToken(t *testing.T) {
	_, errs, _ := prompt(t, "true\necho $(\nfi\n)\n", "dash", "-i")
	for _, want := range []string{
		"dash: 3: Syntax error: \"fi\" unexpected (expecting \")\")\n",
		"dash: 3: Syntax error: \")\" unexpected\n",
	} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr %q, want %q", errs, want)
		}
	}
}

func TestADashPromptDoesNotCountARefusedLine(t *testing.T) {
	out, errs, _ := prompt(t, "fi\nfi\necho $LINENO\nif true\nfi fi\necho $LINENO\n", "dash", "-i")
	if strings.Count(errs, "dash: 1: Syntax error: \"fi\" unexpected\n") != 2 {
		t.Errorf("stderr %q, want both refusals at line 1", errs)
	}
	if out != "1\n3\n" {
		t.Errorf("stdout %q, want 1 and 3", out)
	}
}

// And a backquoted body ends, quietly, at a word that ends a list.
// Measured 2026-10-07 under `-c`: `echo `echo a; fi; echo b“ writes `a`, and
// `echo `echo a; ;“ is refused at the `;`.
func TestABackquotedBodyEndsAtAStopWord(t *testing.T) {
	out, errs, _ := prompt(t, "echo `echo a; fi; echo b`x\necho `echo c; ;`\n", "dash", "-i")
	if !strings.Contains(out, "ax\n") || strings.Contains(out, "b") {
		t.Errorf("stdout %q, want the body ended at `fi'", out)
	}
	if !strings.Contains(errs, "Syntax error: \";\" unexpected\n") {
		t.Errorf("stderr %q, want the `;' refused", errs)
	}
}

// And a closed body refused when it runs keeps the closer it was expecting,
// though the closer is put back for the second read that locates it.
// Measured 2026-10-07, dash 0.5.12: `echo $(fi)` under `-c` is `"fi"
// unexpected (expecting ")")`, and `v=$(echo a; ;)` names nothing.
func TestAClosedBodyRefusedWhenItRunsExpectsTheCloser(t *testing.T) {
	_, errs, _ := prompt(t, "", "dash", "-c", "echo $(fi); v=$(echo a; ;)")
	if !strings.Contains(errs, "Syntax error: \"fi\" unexpected (expecting \")\")\n") {
		t.Errorf("stderr %q, want the closer named", errs)
	}
	_, errs, _ = prompt(t, "", "dash", "-c", "v=$(echo a; ;)")
	if !strings.Contains(errs, "Syntax error: \";\" unexpected\n") {
		t.Errorf("stderr %q, want no closer named for a refused token", errs)
	}
}

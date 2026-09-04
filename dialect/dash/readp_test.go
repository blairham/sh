// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// dash has bash's -p, not ksh93's: the letter takes a prompt as its argument
// and shows it only to a terminal. Measured (2026-09-04): the word after -p
// goes to the option — it is not a variable name — and piped input reads as
// though the flag were absent, the prompt appearing nowhere.
func TestReadPromptTakesTheNextWord(t *testing.T) {
	out, _ := runDash(t, t.TempDir(), `printf 'a b\n' | { read -p PR0MPT v; echo "st=$? PR0MPT=[$PR0MPT] v=[$v]"; }`)
	if !strings.Contains(out, "st=0 PR0MPT=[] v=[a b]") {
		t.Errorf("got %q, want the word consumed as the prompt and the line in v", out)
	}
	if strings.Contains(out, "PR0MPT\n") || strings.HasPrefix(out, "PR0MPT") {
		t.Errorf("got %q, want no prompt off a terminal", out)
	}
}

// A -p whose prompt never arrives is refused in dash's own words — the same
// sentence its kill already had for the case — with status 2 and no usage
// line, dash having none for its builtins. Measured 2026-09-04.
func TestReadPromptMissingItsArgument(t *testing.T) {
	out, _ := runDash(t, t.TempDir(), `read -p </dev/null; echo "st=$?"`)
	if !strings.Contains(out, "read: No arg for -p option") {
		t.Errorf("got %q, want dash's missing-argument sentence", out)
	}
	if !strings.Contains(out, "st=2") {
		t.Errorf("got %q, want status 2", out)
	}
}

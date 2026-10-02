// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The text `eval` reads is parsed with the runner's own answer about how long
// a character is, as the front end's parse is: a flag's delimiter of more
// than one byte is taken whole there too, and one character is not taken for
// another that shares its first byte. Measured 2026-10-02 on zsh 5.9.2 under
// `LC_ALL=en_US.UTF-8`: the first writes `barXX`, the second `error in flags
// near position 10` (#5153).
func TestEvalReadsAWideFlagDelimiter(t *testing.T) {
	out, _, errs := runZshUTF8(t, "foo=bar; eval 'print ${(r£5££X£)foo}'")
	if out != "barXX\n" || errs != "" {
		t.Errorf("got %q, %q, want %q", out, errs, "barXX\n")
	}
	out, _, errs = runZshUTF8(t, "foo=bar; eval 'print ${(r£5£«X«)foo}'")
	if want := "error in flags near position 10 in '${(r£5£«X«)foo}'"; out != "" || !strings.Contains(errs, want) {
		t.Errorf("got %q, %q, want the refusal %q", out, errs, want)
	}
}

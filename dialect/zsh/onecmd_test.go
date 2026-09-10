// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// `set -t` is a letter this shell has and will not move, which is a third
// answer beside having it and never having heard of it — and it is the same
// answer it gives `set -o singlecommand`, the option the letter abbreviates.
//
// Measured 2026-09-10 against zsh 5.9.2. It matters here because the option
// behind the letter was built for the two shells that do move it, and a letter
// riding `UnimplementedOptionLetters` said `-t is not implemented yet`: this
// implementation confessing to something the shell itself refuses (#1716).

// zshWriting is zshShell with the two streams attached.
func zshWriting(out, errs *bytes.Buffer) driver.Shell {
	sh := zshShell()
	sh.Stdout, sh.Stderr = out, errs
	return sh
}

func TestZshRefusesTheOneCommandLetterInItsOwnWords(t *testing.T) {
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-c", "set -t\necho NO"})
	// The letter's own spelling, not the name it abbreviates; the status and
	// the fatality the same shell gives `set -o singlecommand`.
	if want := "zsh:set:1: can't change option: -t\n"; errs.String() != want {
		t.Errorf("said %q, want %q", errs.String(), want)
	}
	if code != 1 || out.String() != "" {
		t.Errorf("status %d and ran %q, want 1 and the script stopped", code, out.String())
	}
	// And the name it abbreviates says the same thing, which is what makes
	// the letter's wording a pairing rather than a coincidence.
	out.Reset()
	errs.Reset()
	code = driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-c", "set -o singlecommand\necho NO"})
	if want := "zsh:set:1: can't change option: singlecommand\n"; errs.String() != want || code != 1 {
		t.Errorf("the name said %q status %d, want %q status 1", errs.String(), code, want)
	}
}

// TestZshGrantsTheStateItAlreadyHolds is the other half, and the half a plain
// refusal gets wrong: asking for the state the option is already in succeeds,
// silently, exactly as it does for every long name.
func TestZshGrantsTheStateItAlreadyHolds(t *testing.T) {
	var out, errs bytes.Buffer
	code := driver.MainArgs(zshWriting(&out, &errs), []string{"zsh", "-c", "set +t; echo B"})
	if out.String() != "B\n" || errs.String() != "" || code != 0 {
		t.Errorf("set +t = %q / %q status %d, want B and nothing said", out.String(), errs.String(), code)
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `bash --badopt` writes its own sentence and then the whole shell usage
// block, exactly as `bash -q` does — #2298.
//
// Measured 2026-09-16 on bash 5.3.20 (Homebrew) with standard input on
// /dev/null: both spellings print one sentence and the same twenty-two lines
// under it, twenty-three in all, at status 2. This shell had the block and
// printed it for the letter alone, so the long spelling was a single
// `unknown option "--badopt"` — forty-four missing lines in two refusals, and
// the largest single cause in `invocation.tests`.
//
// The pairing is what this asserts and not the block's contents: the two
// spellings are compared against **each other**, so the row cannot be
// satisfied by a block copied into the long path and left to drift from the
// one the letter writes.
func TestABadLongOptionWritesTheSameBlockTheLetterDoes(t *testing.T) {
	letterOut, letterCode := refuse(t, "-q")
	longOut, longCode := refuse(t, "--badopt")
	if letterCode != 2 || longCode != 2 {
		t.Fatalf("status: letter %d, long %d — want 2 and 2", letterCode, longCode)
	}
	if !strings.HasPrefix(longOut, "bash: --badopt: invalid option\n") {
		t.Errorf("stderr = %q, want it to open with the word and `invalid option`", longOut)
	}
	if !strings.HasPrefix(letterOut, "bash: -q: invalid option\n") {
		t.Errorf("the letter's stderr = %q, want it to open with `-q: invalid option`", letterOut)
	}
	letterBlock := strings.SplitN(letterOut, "\n", 2)[1]
	longBlock := strings.SplitN(longOut, "\n", 2)[1]
	if longBlock != letterBlock {
		t.Errorf("the long spelling's block and the letter's differ:\nlong:   %q\nletter: %q",
			longBlock, letterBlock)
	}
	if n := strings.Count(longOut, "\n"); n != 23 {
		t.Errorf("wrote %d lines, want 23 — the sentence and bash's twenty-two", n)
	}
}

// The word is echoed whole and is never trimmed towards an option this shell
// has: `--initfile` is one character from `--init-file`, and bash refuses it
// as written rather than reading it as the option it nearly is.
func TestABadLongOptionIsEchoedAsItWasWritten(t *testing.T) {
	out, code := refuse(t, "--initfile")
	if code != 2 {
		t.Fatalf("status %d, want 2", code)
	}
	if !strings.HasPrefix(out, "bash: --initfile: invalid option\n") {
		t.Errorf("stderr = %q, want the word as it was written", out)
	}
}

// refuse invokes the binary's own shell value with one option word and
// returns what it wrote to standard error.
func refuse(t *testing.T, word string) (string, int) {
	t.Helper()
	var o, e bytes.Buffer
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &o, &e
	code := driver.MainArgs(sh, []string{"bash", word})
	if o.Len() > 0 {
		t.Errorf("stdout = %q, want nothing", o.String())
	}
	return e.String(), code
}

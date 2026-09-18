// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// `bash --help` writes a version line, the shell's usage block and a six-line
// trailer, on **standard output**, at **0**.
//
// Before #3268 it was `--help: invalid option` and the same block on standard
// error at 2 — the right block under the wrong sentence, which is the answer
// for a word this shell does not have and the wrong one for a word it does.
//
// Measured 2026-09-18 on bash 5.3.20 (Homebrew) with standard input on
// /dev/null. `bash --help` and `bash --badopt` were diffed line by line and
// the twenty-two lines between the version line and the trailer are
// byte-identical, which is why the block is drawn from Diagnostics rather
// than written out again here.
//
// The end-to-end row: the block, the trailer and the stream are three
// different values and only a whole binary can be wrong about all three at
// once. driver/helpoption_test.go asks whether the front end reads the option
// at all, with a vector of its own.
func TestTheHelpOptionWritesTheBlockOnStandardOutput(t *testing.T) {
	out, errs, code := captureArgs(t, "", "--help")
	if code != 0 {
		t.Fatalf("status %d, want 0 — out %q, stderr %q", code, out, errs)
	}
	if errs != "" {
		t.Errorf("stderr %q, want nothing — this is an answer and not a complaint", errs)
	}
	// The version line, then the block, then the trailer.
	if !strings.HasPrefix(out, "GNU bash, version ") {
		t.Errorf("stdout opens %q, want a version line", firstLine(out))
	}
	// The hyphen before the parenthesis is bash's own and parts this line
	// from `--version`'s, which writes a space there — measured, one binary
	// one run. The tag between them is this build's rather than bash's, so
	// the assertion is on the punctuation and not on the whole line.
	version, _, _ := captureArgs(t, "", "--version")
	help, plain := firstLine(out), firstLine(version)
	if help == plain {
		t.Errorf("--help and --version wrote the same line %q, want them to differ", help)
	}
	if strings.Contains(help, " (") || !strings.Contains(help, "-(") {
		t.Errorf("--help version line %q, want the machine in `-(…)` and no space before it", help)
	}
	if !strings.Contains(plain, " (") {
		t.Errorf("--version line %q, want the machine in ` (…)`", plain)
	}
	// And the two are one line with one character changed, which is what
	// says they are the same version said twice rather than two versions.
	if strings.ReplaceAll(help, "-(", " (") != plain {
		t.Errorf("--help %q and --version %q differ by more than the separator", help, plain)
	}
	// The block itself, taken from the refusal so the two cannot drift: the
	// same twenty-two lines, in the same order, under a different opening.
	block := strings.TrimPrefix(invalidOptionZ, "bash: -Z: invalid option\n")
	if !strings.Contains(out, block) {
		t.Errorf("stdout %q, want the usage block in it", out)
	}
	for _, want := range []string{
		"Type `bash -c \"help set\"' for more information about shell options.\n",
		"Type `bash -c help' for more information about shell builtin commands.\n",
		"Use the `bashbug' command to report bugs.\n",
		"\nbash home page: <http://www.gnu.org/software/bash>\n",
		"General help using GNU software: <http://www.gnu.org/gethelp/>\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout %q, want %q in the trailer", out, want)
		}
	}
	if !strings.HasSuffix(out, "General help using GNU software: <http://www.gnu.org/gethelp/>\n") {
		t.Errorf("stdout ends %q, want the last trailer line", out[max(0, len(out)-60):])
	}
}

// And the word ends the invocation, while a refused word behind it still wins
// — which is why it is recorded where it stands rather than answered there.
// Measured in the same run: `bash --help -c 'echo RAN'` writes the help and
// runs nothing, and `bash --help --badopt` is the refusal at 2.
func TestTheHelpOptionEndsTheInvocationAndYieldsToARefusal(t *testing.T) {
	out, _, code := captureArgs(t, "", "--help", "-c", "echo RAN")
	if code != 0 || strings.Contains(out, "RAN") {
		t.Errorf("--help -c: status %d, out %q — want the help at 0 and nothing run", code, out)
	}
	out, errs, code := captureArgs(t, "", "--help", "--badopt")
	if code != 2 || out != "" {
		t.Errorf("--help --badopt: status %d, out %q — want the refusal at 2", code, out)
	}
	if !strings.HasPrefix(errs, "bash: --badopt: invalid option\n") {
		t.Errorf("--help --badopt said %q, want the refusal to name the bad word", firstLine(errs))
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

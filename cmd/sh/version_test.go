// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// #1713: `sh --version` was an unknown option, which is what an installer,
// an editor or an agent probing this binary would have concluded was a broken
// shell. The answer is the core dialect's, and it can only be filled in here:
// interp holds no build's version.
func TestTheBinaryNamesItsOwnVersion(t *testing.T) {
	t.Parallel()
	out, errs, code := helped(t, "--version")
	if code != 0 {
		t.Errorf("status %d, want 0 (stderr %q)", code, errs)
	}
	if errs != "" {
		t.Errorf("stderr = %q, want the version on standard output alone", errs)
	}
	if !strings.Contains(out, version) {
		t.Errorf("output = %q, want the build's version %q in it", out, version)
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("output = %q, want one line", out)
	}
}

// And a dialect answers for itself, including the one that refuses. This is
// what keeps `sh -dialect bash` a usable column in the conformance harness:
// the invocation surface is graded through this binary, so an answer of the
// binary's own here would be the wrong shell's answer.
func TestADialectsVersionIsTheDialectsAnswer(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		dialect string
		want    string
		stderr  bool
		code    int
	}{
		{dialect: "bash", want: "GNU bash, version 5.3.15"},
		{dialect: "zsh", want: "zsh 5.9.2"},
		{dialect: "ksh", want: "93u+", stderr: true, code: 2},
		{dialect: "dash", want: "unknown option", stderr: true, code: 2},
	} {
		t.Run(c.dialect, func(t *testing.T) {
			t.Parallel()
			out, errs, code := helped(t, "-dialect", c.dialect, "--version")
			if code != c.code {
				t.Errorf("status %d, want %d (out %q, err %q)", code, c.code, out, errs)
			}
			got := out
			if c.stderr {
				got = errs
				if out != "" {
					t.Errorf("stdout = %q, want the answer on standard error alone", out)
				}
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("answer = %q, want %q in it", got, c.want)
			}
		})
	}
}

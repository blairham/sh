// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A `{name}` redirection whose number the *shell* picks is refused in two
// sentences when the process cannot hold that number: one naming the move,
// with no line in it, and then the ordinary failed-open refusal with the
// target and the same errno.
//
// Measured 2026-09-22 on bash 5.3.20 under `ulimit -n 8`, from a script file:
//
//	exec {v}</dev/null      redirection error: cannot duplicate fd: Invalid argument
//	                        line 2: /dev/null: Invalid argument              1
//	{ echo x; } {w}>out     the same pair, naming `out`                      1
//	exec {a}>&1             the same pair, naming `1`                        1
//	cat {b}<<EOF            the same pair, naming `file descriptor out of range`
//
// A number the *script* wrote is a different event and keeps its own sentence
// — `exec 20>f` is `20: Bad file descriptor` — which is the control below.
//
// The limit is the hook's answer rather than this process's: lowering the real
// one would change the state every other test runs in, and what is being
// checked is the pair of sentences.
func TestAPickedDescriptorOverTheLimitIsRefusedInTwoSentences(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{
			"an open", `exec {v}</dev/null`,
			[]string{
				"sh: redirection error: cannot duplicate fd: Invalid argument\n",
				"/dev/null: Invalid argument",
			},
		},
		{
			"a create", `{ echo x; } {w}>out`,
			[]string{
				"sh: redirection error: cannot duplicate fd: Invalid argument\n",
				"out: Invalid argument",
			},
		},
		{
			"a duplication", `exec {a}>&1`,
			[]string{
				"sh: redirection error: cannot duplicate fd: Invalid argument\n",
				"1: Invalid argument",
			},
		},
		{
			// A here-document has a body where a filename would be, and the
			// shell names one anyway.
			"a here-document", "cat {b}<<EOF\nhi\nEOF",
			[]string{
				"sh: redirection error: cannot duplicate fd: Invalid argument\n",
				"file descriptor out of range: Invalid argument",
			},
		},
		{
			// The control: a number the script wrote keeps the other
			// sentence, and gets no preamble at all.
			"a number the script wrote", `exec 20>f`,
			[]string{"20: Bad file descriptor"},
		},
	} {
		out, st := runUnderFdLimit(t, tc.src)
		for _, want := range tc.want {
			if !strings.Contains(out, want) {
				t.Errorf("%s: said %q, want %q in it", tc.name, out, want)
			}
		}
		if st != 1 {
			t.Errorf("%s: status = %d, want a failed redirection", tc.name, st)
		}
	}
}

// And the preamble belongs to the picked number alone: a number the script
// wrote must not pick it up.
func TestAScriptWrittenNumberGetsNoDuplicationSentence(t *testing.T) {
	out, _ := runUnderFdLimit(t, `exec 20>f`)
	if strings.Contains(out, "cannot duplicate fd") {
		t.Errorf("said the picked number's sentence for a number the script wrote: %q", out)
	}
}

// runUnderFdLimit runs src in this dialect with the open-file limit answered
// at 8, which is below where the shell starts picking descriptors.
func runUnderFdLimit(t *testing.T, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, bash.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var buf bytes.Buffer
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Name: "sh", Dir: t.TempDir(),
		GetRlimit: func(res interp.Resource) (int64, int64, error) {
			if res != interp.ResourceOpenFiles {
				return interp.RlimitInfinity, interp.RlimitInfinity, nil
			}
			return 8, 8, nil
		},
		Dialect: presetDialect(),
	}
	bash.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return buf.String(), st
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// ksh93 refuses a descriptor number the process cannot hold, as bash does, and
// says something else about it: `ulimit -n 6; exec 8>f` is `bad file unit
// number [Invalid argument]` here where bash quotes the number and a different
// errno. The number is not in this shell's sentence at all, which is why the
// wording is a field rather than one message with the number substituted in.
//
// The limit is the hook's answer rather than this process's: lowering the real
// one would change the state every other test runs in, and what is being
// checked is the sentence.
func TestADescriptorNumberOverTheLimitIsRefusedInThisShellsWords(t *testing.T) {
	// A single digit, because this shell reads only one — the number has to
	// be reachable in the dialect that is refusing it.
	f, err := syntax.Parse(`exec 8>f`, ksh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem, diag := ksh.Semantics(), ksh.Diagnostics()
	if sem.FdNumberBoundedByOpenFileLimit != interp.Yes {
		t.Fatalf("the axis answers %v, want the refusal this test is about", sem.FdNumberBoundedByOpenFileLimit)
	}
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Name: "sh", Dir: t.TempDir(),
		GetRlimit: func(res interp.Resource) (int64, int64, error) {
			if res != interp.ResourceOpenFiles {
				return interp.RlimitInfinity, interp.RlimitInfinity, nil
			}
			return 6, 6, nil
		},
		Dialect: presetDialect(),
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if got := buf.String(); !strings.Contains(got, "bad file unit number [Invalid argument]") {
		t.Errorf("said %q, want this shell's own sentence", got)
	}
	if strings.Contains(buf.String(), "Bad file descriptor") {
		t.Errorf("said the other shell's sentence: %q", buf.String())
	}
	if st != 1 {
		t.Errorf("status = %d, want a failed redirection", st)
	}
}

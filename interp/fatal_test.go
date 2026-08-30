// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// TestFatalErrorsAbandonTheScript covers the three unrelated failures that
// share one status axis. Each is asserted under a dialect that calls it fatal
// and one that does not, because asserting only the fatal side would pass
// against an implementation that made everything fatal.
func TestFatalErrorsAbandonTheScript(t *testing.T) {
	tests := []struct {
		name, src string
		fatal     Semantics // aborts, and with which status
		status    int
		survives  Semantics // reaches the second command
	}{
		{
			"readonly reassignment",
			`readonly r=1; r=2; echo after`,
			PosixSemantics(), 2, bash.Semantics(),
		},
		{
			"shift past the end",
			`shift 5; echo after`,
			ksh.Semantics(), 1, bash.Semantics(),
		},
		{
			"arithmetic error",
			// Fatal in all four, so the surviving side is not asserted.
			`echo $((1/0)); echo after`,
			PosixSemantics(), 2,
			Semantics{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, withSem(tc.fatal))
			if strings.Contains(out, "after") {
				t.Errorf("the script continued: %q", out)
			}
			if st != tc.status {
				t.Errorf("status = %d, want %d", st, tc.status)
			}
			if tc.survives == (Semantics{}) {
				return
			}
			out, st = run(t, tc.src, withSem(tc.survives))
			if !strings.Contains(out, "after") {
				t.Errorf("the script stopped where the dialect survives: %q", out)
			}
			if st != 0 {
				t.Errorf("surviving status = %d, want 0", st)
			}
		})
	}
}

// TestFatalStatusIsOneAxis is the evidence for having one axis rather than
// three: unrelated errors, same split.
func TestFatalStatusIsOneAxis(t *testing.T) {
	for _, src := range []string{`readonly r=1; r=2`, `shift 5`, `echo $((1/0))`} {
		if _, st := run(t, src, withSem(PosixSemantics())); st != 2 {
			t.Errorf("%s under posix: status = %d, want 2", src, st)
		}
		if _, st := run(t, src, withSem(ksh.Semantics())); st != 1 {
			t.Errorf("%s under ksh93: status = %d, want 1", src, st)
		}
	}
}

func TestUnmatchedGlobIsFatalOnlyWhereTheDialectSaysSo(t *testing.T) {
	// zsh alone. Reporting the error and then passing the pattern through
	// was the bug: the diagnostic appeared and the command ran anyway.
	out, st := run(t, `echo /zzz_no_such_dir_*; echo after`, withSem(zsh.Semantics()))
	if strings.Contains(out, "after") {
		t.Errorf("zsh: the script continued: %q", out)
	}
	// The diagnostic names the pattern, so the check is that `echo` never
	// wrote it as a line of its own.
	for _, line := range strings.Split(out, "\n") {
		if line == "/zzz_no_such_dir_*" {
			t.Errorf("zsh: the pattern was passed through: %q", out)
		}
	}
	if st != 1 {
		t.Errorf("zsh: status = %d, want 1", st)
	}
	out, st = run(t, `echo /zzz_no_such_dir_*`, withSem(bash.Semantics()))
	if !strings.Contains(out, "/zzz_no_such_dir_*") {
		t.Errorf("bash: the pattern should pass through, got %q", out)
	}
	if st != 0 {
		t.Errorf("bash: status = %d, want 0", st)
	}
}

func TestUnterminatedBracketIsNotAGlob(t *testing.T) {
	// `[` is the test builtin's name. Treating it as a pattern reported
	// "no matches found: [" on every use of `test`, which went unnoticed
	// until an unmatched pattern became fatal and the builtin stopped
	// running. All four shells agree a lone `[` is literal.
	out, st := run(t, `[ a = a ] && echo yes`, withSem(zsh.Semantics()))
	if out != "yes\n" || st != 0 {
		t.Errorf("got %q status %d, want %q status 0", out, st, "yes\n")
	}
	if got, _ := run(t, `echo [`, withSem(bash.Semantics())); got != "[\n" {
		t.Errorf("echo [: got %q", got)
	}
	// A closed bracket is still a pattern.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	src := `cd ` + dir + `; echo [a]`
	if got, _ := run(t, src, withSem(bash.Semantics())); got != "a\n" {
		t.Errorf("[a] should have matched the file, got %q", got)
	}
}

func TestArithmeticAxesAreAsked(t *testing.T) {
	// A name-shaped value: re-evaluated in bash and zsh, an error in dash
	// and ksh93.
	if got, _ := run(t, `x=abc; echo $((x+1))`, withSem(bash.Semantics())); got != "1\n" {
		t.Errorf("bash: got %q, want %q", got, "1\n")
	}
	if _, st := run(t, `x=abc; echo $((x+1))`, withSem(PosixSemantics())); st == 0 {
		t.Error("posix: a name-shaped value should be an error")
	}
	// An invalid octal digit: an error in dash and bash, decimal in ksh93.
	if _, st := run(t, `echo $((08))`, withSem(bash.Semantics())); st == 0 {
		t.Error("bash: 08 should be an error")
	}
	if got, _ := run(t, `echo $((08))`, withSem(ksh.Semantics())); got != "8\n" {
		t.Errorf("ksh93: got %q, want %q", got, "8\n")
	}
	// ksh93 is octal *and* tolerant — the reason one bool could not say it.
	if got, _ := run(t, `echo $((0100))`, withSem(ksh.Semantics())); got != "64\n" {
		t.Errorf("ksh93 0100: got %q, want %q", got, "64\n")
	}
	if got, _ := run(t, `echo $((0100))`, withSem(zsh.Semantics())); got != "100\n" {
		t.Errorf("zsh 0100: got %q, want %q", got, "100\n")
	}
}

func TestCoreRefusesTheNewAxes(t *testing.T) {
	for _, src := range []string{`x=abc; echo $((x+1))`, `echo $((08))`} {
		if _, st := run(t, src, withSem(CoreSemantics())); st != 2 {
			t.Errorf("%s under the core: status = %d, want a refusal", src, st)
		}
	}
}

// TestIndirectionMeaningIsAnAxis is the semantics half of the three-way
// `${!x}` divergence; the grammar half is asserted in the syntax package.
// Together they express three answers with two binary questions.
func TestIndirectionMeaningIsAnAxis(t *testing.T) {
	const src = `x=y; y=V; printf "[%s]" "${!x}"`
	f, err := syntax.Parse(src, ksh.Dialect())
	if err != nil {
		t.Fatalf("ksh should parse ${!x}: %v", err)
	}
	var buf bytes.Buffer
	sem := ksh.Semantics()
	r := &Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "[x]" {
		t.Errorf("ksh93 yields the name: got %q, want %q", buf.String(), "[x]")
	}
	// bash reads through it, which is the whole disagreement.
	if got, _ := runBash(t, src); got != "[V]" {
		t.Errorf("bash indirects: got %q, want %q", got, "[V]")
	}
}

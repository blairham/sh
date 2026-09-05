// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `{name}>f` and its family: the shell picks the descriptor, the variable
// receives its number, and the number is what `>&$name` then addresses.
func TestFdVariableRedirectionAllocatesAndAssigns(t *testing.T) {
	dir := t.TempDir()
	out := runFdVar(t, dir, nil,
		`exec {fd}>f
[ "$fd" -ge 10 ] && echo high
echo hi >&$fd
exec {fd}>&-
cat f`)
	if got, want := out, "high\nhi\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The input direction picks a descriptor the same way.
func TestFdVariableRedirectionReads(t *testing.T) {
	dir := t.TempDir()
	out := runFdVar(t, dir, nil,
		`echo data > src
exec {i}<src
read -r x <&$i
echo "got=$x"`)
	if got, want := out, "got=data\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Whether the descriptor outlives the simple command that carried it — the
// axis one dialect answers No, taking it back with the command's other
// redirections.
func TestFdVariableMayOrMayNotOutliveItsCommand(t *testing.T) {
	for _, c := range []struct {
		name     string
		outlives Answer
		want     string
	}{
		// `echo one` still writes its stdout: the picked descriptor is a
		// new one, not a replacement for 1. Only the second write goes
		// through it, and only where it is still there to write through.
		{"kept open", Yes, "one\ntwo\n"},
		{"taken back", No, "one\ndead\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			out := runFdVar(t, dir, func(s *Semantics) {
				s.FdVariableOutlivesTheCommand = c.outlives
			},
				`echo one {fd}>pf
echo two >&$fd 2>/dev/null || echo dead
cat pf`)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// Closing through a name that holds no descriptor — an error in two of the
// three that have the grammar, and not worth a word in the third.
func TestFdVariableBadCloseMayBeAnError(t *testing.T) {
	for _, c := range []struct {
		name    string
		refuses Answer
		want    string
	}{
		{"refused", Yes, "st=1\n"},
		{"quietly accepted", No, "st=0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			out := runFdVar(t, dir, func(s *Semantics) {
				s.FdVariableBadCloseIsAnError = c.refuses
			},
				`exec {nofd}>&- 2>/dev/null
echo "st=$?"`)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// runFdVar parses src under the core grammar — which has the construct — and
// runs it in dir with the posix answers, adjusted by tweak.
func runFdVar(t *testing.T, dir string, tweak func(*Semantics), src string) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	if tweak != nil {
		tweak(&sem)
	}
	out := &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
		Stdout: out, Stderr: &strings.Builder{},
		Vars: map[string]string{"PATH": lookBinPath(t)},
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// lookBinPath is enough PATH to find cat on the platforms CI runs.
func lookBinPath(t *testing.T) string {
	t.Helper()
	return strings.Join([]string{"/bin", "/usr/bin"}, string(filepath.ListSeparator))
}

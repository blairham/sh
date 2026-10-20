// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `zcompile` writes a file, and every row here was measured on zsh 5.9.2
// before it was written down — see dialect/zsh/zcompile.go for the table and
// for why the product is the inputs' text rather than zsh's wordcode.
//
// powerlevel10k is what made this worth having: it calls `zcompile` while
// writing its instant-prompt cache, so a shell without the builtin abandoned
// that dump on every startup and said so twice (#1405).

// One operand compiles to `NAME.zwc` beside it.
func TestZcompileWithOneOperandWritesTheNameWithZwc(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.zsh", "f(){ echo hi; }\n")

	out, st := runZsh(t, dir, "zcompile a.zsh && echo done")
	if st != 0 || out != "done\n" {
		t.Fatalf("out = %q, status %d, want %q", out, st, "done\n")
	}
	if _, err := os.Stat(filepath.Join(dir, "a.zsh.zwc")); err != nil {
		t.Errorf("no a.zsh.zwc: %v", err)
	}
}

// With more than one operand the **first** is the output, which is the whole
// of how the two forms are told apart.
func TestZcompileWithSeveralOperandsTakesTheFirstAsTheOutput(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.zsh", "f(){ echo one; }\n")
	write(t, dir, "b.zsh", "g(){ echo two; }\n")

	if out, st := runZsh(t, dir, "zcompile out.zwc a.zsh b.zsh && echo done"); st != 0 || out != "done\n" {
		t.Fatalf("out = %q, status %d", out, st)
	}
	body := read(t, dir, "out.zwc")
	for _, want := range []string{"echo one", "echo two"} {
		if !strings.Contains(body, want) {
			t.Errorf("out.zwc = %q, want it to carry %q — the product is the inputs", body, want)
		}
	}
	// And no `a.zsh.zwc`: the first operand was the output, not an input.
	if _, err := os.Stat(filepath.Join(dir, "a.zsh.zwc")); err == nil {
		t.Error("a.zsh.zwc was written, so the first operand was read as an input")
	}
}

// The product is mode 0444, as zsh's is, and compiling twice still works —
// which it would not if the read-only mode were left in the way.
func TestZcompileWritesAReadOnlyFileAndCanReplaceIt(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.zsh", "f(){ echo hi; }\n")

	if out, st := runZsh(t, dir, "zcompile a.zsh && zcompile a.zsh && echo twice"); st != 0 || out != "twice\n" {
		t.Fatalf("out = %q, status %d, want %q", out, st, "twice\n")
	}
	info, err := os.Stat(filepath.Join(dir, "a.zsh.zwc"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o444 {
		t.Errorf("mode %04o, want 0444", got)
	}
}

// The four refusals, each measured, each with zsh's own sentence and status.
func TestZcompileRefusals(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.zsh", "f(){ echo hi; }\n")

	for _, tc := range []struct{ name, src, want string }{
		{"no operands", "zcompile", "too few arguments"},
		{"a file that is not there", "zcompile nosuch.zsh", "can't open file: nosuch.zsh"},
		{"a letter it has not got", "zcompile -q a.zsh", "bad option: -q"},
		{"-t on anything at all", "zcompile -t nosuch.zwc", "can't open zwc file: nosuch.zwc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if st != 1 {
				t.Errorf("status %d, want 1", st)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("said %q, want it to carry %q", out, tc.want)
			}
		})
	}
}

// A file that does not parse is refused, and — measured — **nothing is
// written**, not even for the operands that did parse before it.
func TestZcompileWritesNothingWhenAnInputDoesNotParse(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "ok.zsh", "f(){ echo hi; }\n")
	write(t, dir, "bad.zsh", "if then fi (((\n")

	out, st := runZsh(t, dir, "zcompile out.zwc ok.zsh bad.zsh")
	if st != 1 {
		t.Errorf("status %d, want 1", st)
	}
	if !strings.Contains(out, "can't read file: bad.zsh") {
		t.Errorf("said %q, want it to carry %q", out, "can't read file: bad.zsh")
	}
	if _, err := os.Stat(filepath.Join(dir, "out.zwc")); err == nil {
		t.Error("out.zwc was written, and zsh leaves nothing behind for a refused compile")
	}
}

// `--` ends the options, so an output whose name begins with `-` is reachable.
func TestZcompileStopsReadingOptionsAtDoubleDash(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.zsh", "f(){ echo hi; }\n")
	if out, st := runZsh(t, dir, "zcompile -R -- o.zwc a.zsh && echo done"); st != 0 || out != "done\n" {
		t.Errorf("out = %q, status %d, want %q", out, st, "done\n")
	}
}

// A write the kernel declines is reported, and in zsh's own sentence for it —
// which is a *different* sentence from the one for an input it cannot read.
// Measured: `zcompile rodir/x.zwc ok.zsh` with `rodir` unwritable says
// `can't write zwc file: rodir/x.zwc` at 1.
//
// It is a test because the first implementation was silent here: it returned
// fs.ErrPermission for a refusal by the sandbox *and* for one by the kernel,
// and suppressed the message for both — so a real failure said nothing at all.
func TestZcompileReportsAWriteItCouldNotMake(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.zsh", "f(){ echo hi; }\n")
	ro := filepath.Join(dir, "ro")
	if err := os.Mkdir(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })

	out, st := runZsh(t, dir, "zcompile ro/x.zwc a.zsh")
	if st != 1 {
		t.Errorf("status %d, want 1", st)
	}
	if want := "can't write zwc file: ro/x.zwc"; !strings.Contains(out, want) {
		t.Errorf("said %q, want it to carry %q", out, want)
	}
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

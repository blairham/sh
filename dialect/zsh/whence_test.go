// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `whence` and `where`, measured against zsh 5.9.2 on 2026-09-05. Every
// assertion here is a line that shell wrote, and several of them are the ones
// the other shell with a `whence` writes differently — the stream a miss goes
// to, the status of a usage error, and the letters that exist at all.

// whenceDir is a PATH directory holding one executable, so a test can name a
// file whose path it knows without depending on the machine.
func whenceDir(t *testing.T) (dir, tool string) {
	t.Helper()
	dir = t.TempDir()
	tool = filepath.Join(dir, "tool431")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir, tool
}

func TestWhenceAnswersInFourShapes(t *testing.T) {
	dir, tool := whenceDir(t)
	const setup = "f() { :; }\nalias al='ls -l'\n"
	for _, c := range []struct{ name, src, want string }{
		// Bare: the resolution and nothing around it. An alias is its
		// *value*, which is the one answer the core cannot give.
		{"bare alias", "whence al", "ls -l"},
		{"bare function", "whence f", "f"},
		{"bare builtin", "whence echo", "echo"},
		{"bare reserved word", "whence if", "if"},
		{"bare file", "whence tool431", tool},

		// -v: the sentence, which is this shell's `type` exactly.
		{"verbose alias", "whence -v al", "al is an alias for ls -l"},
		{"verbose function", "whence -v f", "f is a shell function from zsh"},
		{"verbose builtin", "whence -v echo", "echo is a shell builtin"},
		{"verbose reserved word", "whence -v if", "if is a reserved word"},
		{"verbose file", "whence -v tool431", "tool431 is " + tool},

		// -c: the csh listing, a shape of its own rather than the bare one
		// with words added.
		{"csh alias", "whence -c al", "al: aliased to ls -l"},
		{"csh builtin", "whence -c echo", "echo: shell built-in command"},
		{"csh reserved word", "whence -c if", "if: shell reserved word"},
		{"csh file", "whence -c tool431", tool},

		// -w: the bare kind, in this shell's own vocabulary — a file is
		// `command` here.
		{"kind alias", "whence -w al", "al: alias"},
		{"kind function", "whence -w f", "f: function"},
		{"kind builtin", "whence -w echo", "echo: builtin"},
		{"kind reserved word", "whence -w if", "if: reserved"},
		{"kind file", "whence -w tool431", "tool431: command"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, dir, setup+c.src+"\n")
			if strings.TrimSpace(out) != c.want {
				t.Errorf("output = %q, want %q", out, c.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// A function answers with its body under `-c` and `-f`, and with its name
// otherwise. The two letters differ in nothing else here.
func TestWhenceShowsAFunctionsBody(t *testing.T) {
	for _, src := range []string{"whence -c f", "whence -f f", "where f"} {
		out, st := runZsh(t, t.TempDir(), "f() { :; }\n"+src+"\n")
		if !strings.Contains(out, "f () {") || !strings.Contains(out, ":") {
			t.Errorf("%q gave %q, want the body", src, out)
		}
		if st != 0 {
			t.Errorf("%q: status = %d, want 0", src, st)
		}
	}
}

// A name that resolves to nothing: silence in the bare shape and a line in
// the rest — on *standard output*, which is the difference from the other
// shell with this builtin, and always 1.
func TestWhenceMissesOnStandardOutput(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"whence nosuchcmd431", ""},
		{"whence -v nosuchcmd431", "nosuchcmd431 not found"},
		{"whence -c nosuchcmd431", "nosuchcmd431 not found"},
		{"whence -w nosuchcmd431", "nosuchcmd431: none"},
		{"whence -p nosuchcmd431", ""},
	} {
		out, st, errs := runZshSplit(t, t.TempDir(), c.src+"\n")
		if strings.TrimSpace(out) != c.want {
			t.Errorf("%q: stdout = %q, want %q", c.src, out, c.want)
		}
		if errs != "" {
			t.Errorf("%q: stderr = %q, want the answer on standard output", c.src, errs)
		}
		if st != 1 {
			t.Errorf("%q: status = %d, want 1", c.src, st)
		}
	}
}

// -p is the PATH search with everything else invisible: a function and a
// builtin are nobody, and a builtin that also has a file answers with the
// file.
func TestWhencePSearchesPathAlone(t *testing.T) {
	dir, tool := whenceDir(t)
	out, st := runZsh(t, dir, "f() { :; }\nwhence -p f\n")
	if strings.TrimSpace(out) != "" || st != 1 {
		t.Errorf("a function under -p gave %q at %d, want silence and 1", out, st)
	}
	out, st = runZsh(t, dir, "whence -p tool431\n")
	if strings.TrimSpace(out) != tool || st != 0 {
		t.Errorf("a file under -p gave %q at %d, want %q and 0", out, st, tool)
	}
}

// -a is every resolution rather than the first, alias included, and in the
// order the shell resolves them.
func TestWhenceAListsEveryResolution(t *testing.T) {
	dir, tool := whenceDir(t)
	out, st := runZsh(t, dir, "alias tool431='x'\ntool431() { :; }\nwhence -a tool431\n")
	want := "x\ntool431\n" + tool
	if strings.TrimSpace(out) != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// A usage error is 1 with a bare `bad option` line, where the other shell
// with this builtin prints a usage line and exits 2. Nothing is answered
// after it.
func TestWhenceRefusesAnUnknownLetter(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "whence -z echo\n")
	if !strings.Contains(out, "bad option: -z") {
		t.Errorf("output = %q, want the letter refused", out)
	}
	if strings.Contains(out, "echo") {
		t.Errorf("output = %q, want the operand left unanswered", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// A letter this shell has and this build does not is refused as missing
// rather than as unknown, so a script can tell the two apart.
func TestWhenceRefusesTheLettersItDoesNotImplement(t *testing.T) {
	for _, letter := range []string{"-m", "-s", "-x"} {
		out, st := runZsh(t, t.TempDir(), "whence "+letter+" echo\n")
		if !strings.Contains(out, "not implemented") {
			t.Errorf("%s gave %q, want it refused as missing", letter, out)
		}
		if st != 1 {
			t.Errorf("%s: status = %d, want 1", letter, st)
		}
	}
}

// Nothing to ask about is a silent 1 here, where the other shell prints a
// usage line at 2. The quiet answer is the trap: a script reading the status
// sees a plain miss.
func TestWhenceWithNoOperandIsASilentFailure(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "whence\n")
	if strings.TrimSpace(out) != "" {
		t.Errorf("output = %q, want silence", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// `where` is `whence -ca`, and it is a name rather than a synonym with flags:
// it parses no options at all.
func TestWhereIsWhenceWithCAndA(t *testing.T) {
	dir, tool := whenceDir(t)
	out, st := runZsh(t, dir, "alias tool431='x'\nwhere tool431\n")
	want := "tool431: aliased to x\n" + tool
	if strings.TrimSpace(out) != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}

	out, st = runZsh(t, dir, "where -v echo\n")
	if !strings.Contains(out, "bad option: -v") {
		t.Errorf("output = %q, want the option refused", out)
	}
	if strings.Contains(out, "shell built-in") {
		t.Errorf("output = %q, want nothing answered", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// A command with several operands answers for all of them and reports 1 if
// any missed.
func TestWhenceAnswersEveryOperand(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "whence echo nosuchcmd431 if\n")
	if strings.TrimSpace(out) != "echo\nif" {
		t.Errorf("output = %q, want the two that resolved", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1 for the one that did not", st)
	}
}

// runZshSplit is runZsh with the two streams kept apart, for the assertions
// about which one an answer goes to.
func runZshSplit(t *testing.T, dir, src string) (out string, status int, errs string) {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	r := &interp.Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "zsh", Vars: map[string]string{"PATH": dir},
	}
	zsh.Apply(r)
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return o.String(), st, e.String()
}

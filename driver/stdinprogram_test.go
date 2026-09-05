// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// runStdinProgram invokes the front end with the program on standard input and
// nothing on the argument vector, which is the route `echo … | sh` takes.
//
// A real pipe rather than a strings.Reader, because the whole question is what
// is left on the descriptor: an in-memory reader would answer it too kindly,
// and an external command started by the script could not share one at all.
func runStdinProgram(t *testing.T, sh driver.Shell, program string) (out, errs string, code int) {
	t.Helper()
	sh.Stdin = pipeWith(t, program)
	var o, e bytes.Buffer
	sh.Stdout, sh.Stderr = &o, &e
	code = driver.MainArgs(sh, []string{"testsh"})
	return o.String(), e.String(), code
}

// The axis, from both sides. A program on standard input shares the descriptor
// with the script it is: what the front end has not read yet is what the
// script's own `read` finds, unless the dialect takes the input in blocks, in
// which case the block has already swallowed it.
//
// This is the whole of #470. Reading it all up front gave every dialect the
// block answer, so a data line piped after a script — the payload half of
// `curl … | sh` — was run as a command instead of being read.
func TestTheProgramOnStandardInputSharesTheDescriptor(t *testing.T) {
	const program = "read x\necho \"[$x]\"\nDATA-LINE\necho end\n"
	for _, c := range []struct {
		name   string
		blocks bool
		want   string
		errs   string
	}{
		{
			name: "a line at a time leaves the next line for the script",
			// `read` takes line 2, so `echo "[$x]"` is never parsed and
			// DATA-LINE is the next thing run — at line 2, because a line
			// handed to the script is never counted.
			want: "end\n",
			errs: "testsh: line 2: DATA-LINE: not found\n",
		},
		{
			name:   "a block takes it and the script finds end of input",
			blocks: true,
			want:   "[]\nend\n",
			errs:   "testsh: line 3: DATA-LINE: not found\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			sh := shell()
			sh.Semantics = interp.PosixSemantics()
			sh.Semantics.StdinProgramReadInBlocks = c.blocks
			sh.Diagnostics = interp.Diagnostics{Location: interp.LocationLineWord}
			out, errs, code := runStdinProgram(t, sh, program)
			if out != c.want {
				t.Errorf("output %q, want %q", out, c.want)
			}
			if errs != c.errs {
				t.Errorf("stderr %q, want %q", errs, c.errs)
			}
			if code != 0 {
				t.Errorf("status %d, want 0", code)
			}
		})
	}
}

// A `read` with nothing after it is the control: there is nothing to take, so
// both answers report end of input and leave the variable empty. It is what
// separates "the axis decides who gets the bytes" from "the axis decides how
// `read` behaves", which it does not.
func TestAReadAtTheEndOfAProgramOnStandardInputFindsNothing(t *testing.T) {
	for _, blocks := range []bool{false, true} {
		sh := shell()
		sh.Semantics = interp.PosixSemantics()
		sh.Semantics.StdinProgramReadInBlocks = blocks
		// One line, so that the report is not itself the next line for a
		// line-at-a-time reader to hand over.
		out, _, _ := runStdinProgram(t, sh, "read x; echo \"$? [$x]\"\n")
		if want := "1 []\n"; out != want {
			t.Errorf("blocks=%v: output %q, want %q", blocks, out, want)
		}
	}
}

// Reading a line at a time must not turn a construct into a line. A
// here-document's body, a continued line and a compound command all arrive on
// the same descriptor as the program and all belong to the command that opened
// them — and a reader taking one line at a time is exactly the thing that
// would run them as commands instead.
func TestAProgramOnStandardInputIsStillReadByTheConstruct(t *testing.T) {
	for _, c := range []struct {
		name    string
		program string
		want    string
	}{
		{
			name:    "a here-document body belongs to its command",
			program: "cat <<EOF\nbody\nEOF\necho after\n",
			want:    "body\nafter\n",
		},
		{
			name: "a trailing backslash joins the line to the next",
			// The parser cannot answer this — the same text at the end of the
			// input is a finished command — so the reader has to.
			program: "echo one \\\ntwo\necho three\n",
			want:    "one two\nthree\n",
		},
		{
			name:    "a construct is read until it finishes",
			program: "if true\nthen\n\techo yes\nfi\necho done\n",
			want:    "yes\ndone\n",
		},
		{
			name:    "several commands on one line still run in order",
			program: "echo one; echo two\necho three\n",
			want:    "one\ntwo\nthree\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, blocks := range []bool{false, true} {
				sh := shell()
				sh.Semantics = interp.PosixSemantics()
				sh.Semantics.StdinProgramReadInBlocks = blocks
				out, errs, code := runStdinProgram(t, sh, c.program)
				if out != c.want || code != 0 {
					t.Errorf("blocks=%v: output %q status %d, want %q status 0 (stderr %q)",
						blocks, out, code, c.want, errs)
				}
			}
		})
	}
}

// A shell runs what it has read before a later line fails to parse, on this
// route as on any other. The line-at-a-time reader is the one that could
// plausibly have lost this, by holding the whole program back until it parsed.
func TestAProgramOnStandardInputRunsWhatItReadBeforeAFailure(t *testing.T) {
	sh := shell()
	sh.Semantics = interp.PosixSemantics()
	out, errs, code := runStdinProgram(t, sh, "echo one\n{ fi; }\necho three\n")
	if want := "one\n"; out != want {
		t.Errorf("output %q, want %q", out, want)
	}
	if errs == "" {
		t.Error("no diagnostic for the line that would not parse")
	}
	if code == 0 {
		t.Error("status 0 for a program that failed to parse")
	}
}

// Input that ends part-way through a construct is a syntax error rather than a
// request for more, because there is no more coming. The reader has to tell
// the two apart, and it can only do so by having asked.
func TestAProgramOnStandardInputThatEndsUnfinishedIsAFailure(t *testing.T) {
	sh := shell()
	sh.Semantics = interp.PosixSemantics()
	out, errs, code := runStdinProgram(t, sh, "echo one\nif true\n")
	if want := "one\n"; out != want {
		t.Errorf("output %q, want %q", out, want)
	}
	if errs == "" {
		t.Error("no diagnostic for input that ran out inside a construct")
	}
	if code == 0 {
		t.Error("status 0 for input that ran out inside a construct")
	}
}

// The sharing is with everything the script points at the descriptor, not with
// `read` alone: an external command inherits it, so `cat` prints the rest of
// the program rather than the shell running it.
func TestACommandInheritsTheRestOfAProgramOnStandardInput(t *testing.T) {
	sh := shell()
	sh.Semantics = interp.PosixSemantics()
	out, _, _ := runStdinProgram(t, sh, "echo one\ncat\nNOT-A-COMMAND\necho end\n")
	if want := "one\nNOT-A-COMMAND\necho end\n"; out != want {
		t.Errorf("output %q, want %q", out, want)
	}
}

// And the sharing is what the route *is*: the program is the descriptor, so
// pointing descriptor 0 somewhere else half way through replaces the rest of
// the program with whatever is there.
func TestExecRepointsTheRestOfAProgramOnStandardInput(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/rest.sh"
	if err := os.WriteFile(path, []byte("echo from the file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sh := shell()
	sh.Semantics = interp.PosixSemantics()
	out, errs, _ := runStdinProgram(t, sh, "echo one\nexec 0< "+path+"\necho never reached\n")
	if want := "one\nfrom the file\n"; out != want {
		t.Errorf("output %q, want %q (stderr %q)", out, want, errs)
	}
}

// A block bigger than one read still finishes the line it stopped in the
// middle of. A producer may split its bytes anywhere, and half a command is
// not a command — so the block reader waits for the rest of the line rather
// than running what it happens to have.
func TestABlockReaderDoesNotRunHalfALine(t *testing.T) {
	sh := shell()
	sh.Semantics = interp.PosixSemantics()
	sh.Semantics.StdinProgramReadInBlocks = true
	// Long enough that no single read can hold it, so the boundary falls
	// inside a line rather than tidily between two.
	program := strings.Repeat("echo pad\n", 2000) + "echo last\n"
	out, errs, code := runStdinProgram(t, sh, program)
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if got, want := strings.Count(out, "pad\n"), 2000; got != want {
		t.Errorf("%d padding lines ran, want %d", got, want)
	}
	if !strings.HasSuffix(out, "last\n") {
		t.Errorf("output does not end with the last line: %q", out[max(0, len(out)-40):])
	}
}

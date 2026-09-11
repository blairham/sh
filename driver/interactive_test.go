// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// Deciding to prompt is part of reading the invocation, so it belongs to the
// shared front end rather than to one binary's main. It lived in one binary's
// main, which is how `sh` learned to prompt and `bash` answered
//
//	bash: unknown option "-i"
//
// — the same drift that once let `sh` run a script file and left the dialect
// binaries unable to. A test here is what makes it a property of the front
// end: every binary built on it has a prompt because this passes.
func TestTheFrontEndDecidesToPrompt(t *testing.T) {
	t.Run("-i prompts where standard input is not a terminal", func(t *testing.T) {
		out, errs, _ := runPiped(t, "echo prompted\n", "testsh", "-i")
		if !strings.Contains(out, "prompted\n") {
			t.Errorf("out = %q, want the typed line to have run", out)
		}
		// The prompt is on the error stream, so `sh -i < in > out` collects
		// the commands' output and nothing else.
		if !strings.Contains(errs, "$ ") {
			t.Errorf("err = %q, want a prompt on it", errs)
		}
		if strings.Contains(out, "$ ") {
			t.Errorf("out = %q, want no prompt on the output stream", out)
		}
	})

	t.Run("without -i the same input is a script", func(t *testing.T) {
		// Not a terminal and nothing named: `curl … | sh` arrives this way
		// and must not wait for a keystroke or print a prompt.
		out, errs, _ := runPiped(t, "echo scripted\n", "testsh")
		if !strings.Contains(out, "scripted\n") {
			t.Errorf("out = %q, want standard input read as a script", out)
		}
		if strings.Contains(errs, "$ ") {
			t.Errorf("err = %q, want no prompt for a script on standard input", errs)
		}
	})

	t.Run("an operand still wins", func(t *testing.T) {
		// `-i` is an option among options, not a mode: naming a script still
		// runs the script.
		path := writeScript(t, "echo from-the-script\n")
		out, _, _ := runPiped(t, "", "testsh", "-i", path)
		if !strings.Contains(out, "from-the-script\n") {
			t.Errorf("out = %q, want the named script to have run", out)
		}
	})

	t.Run("-c still wins", func(t *testing.T) {
		out, _, _ := runPiped(t, "", "testsh", "-i", "-c", "echo from-c")
		if !strings.Contains(out, "from-c\n") {
			t.Errorf("out = %q, want -c to have run", out)
		}
	})
}

// A prompt is the same shell as a script: the dialect's prelude is there, and
// so is what the session has already done to itself.
func TestAPromptIsTheSameShellAsAScript(t *testing.T) {
	sh := shell()
	sh.Prelude = "greet() { echo hello-from-the-prelude; }\n"
	out, _, _ := runPipedShell(t, sh, "greet\nx=5\necho \"x=$x\"\n", "testsh", "-i")
	if !strings.Contains(out, "hello-from-the-prelude") {
		t.Errorf("out = %q, want the dialect's prelude available at the prompt", out)
	}
	if !strings.Contains(out, "x=5") {
		t.Errorf("out = %q, want one line's state to reach the next", out)
	}
}

// The prompt is given the invocation, not the process's. A shell names itself
// by the path it was invoked by, in `$0` and in every diagnostic, and reading
// os.Args instead is invisible in a binary — where the two are the same — and
// wrong everywhere else, a test included.
func TestThePromptIsNamedByTheInvocation(t *testing.T) {
	out, _, _ := runPiped(t, "echo \"$0\"\n", "/some/where/myshell", "-i")
	if !strings.Contains(out, "/some/where/myshell\n") {
		t.Errorf("out = %q, want the shell named by argv[0]", out)
	}
}

// A login shell is one whose argv[0] begins with a dash, and the prompt has to
// ask *its* argv rather than the process's — reading os.Args here is invisible
// in a binary, where they are the same, and wrong in everything else.
func TestAPromptReadsLoginFromItsOwnArgv(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".profile"), []byte("echo from-profile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	// Which file a login shell reads is the dialect's, so the shell under
	// test has to name one: a vector nobody filled in reads nothing, which
	// is what keeps every other test in this package out of the developer's
	// own home directory.
	sh := shell()
	sh.Semantics.LoginStartupFiles = ".profile"
	for _, tc := range []struct {
		name, arg0 string
		want       bool
	}{
		{"a login shell reads it", "-testsh", true},
		{"an ordinary one does not", "testsh", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, _ := runPipedShell(t, sh, "", tc.arg0, "-i")
			if got := strings.Contains(out, "from-profile"); got != tc.want {
				t.Errorf("read .profile = %v, want %v (out %q)", got, tc.want, out)
			}
		})
	}
}

// A prompt has someone to tell about its jobs and a script does not, so the
// front end is what says so: no shell in the panel announces a background job
// to `sh -c`, and a Runner embedded in another program has nobody to tell.
func TestOnlyAPromptReportsItsJobs(t *testing.T) {
	sh := shell()
	sem := interp.PosixSemantics()
	sem.AnnouncesBackgroundJob = interp.Yes
	// And with the monitor off, which is what an `-i` on a pipe has: measured
	// 2026-09-11, `printf 'sleep 0.2 &\n' | bash -i` says `no job control in
	// this shell` and still prints `[1] <pid>`. This test is about the front
	// end having somebody to tell, so the monitor question is answered rather
	// than left to refuse (#1738).
	sem.AnnouncesBackgroundJobWithoutTheMonitor = interp.Yes
	sh.Semantics = sem

	t.Run("a prompt announces", func(t *testing.T) {
		_, errs, _ := runPipedShell(t, sh, "sleep 0.2 &\n", "testsh", "-i")
		if !strings.Contains(errs, "[1] ") {
			t.Errorf("err = %q, want the job announced", errs)
		}
	})

	t.Run("a script does not", func(t *testing.T) {
		_, errs, _ := runPipedShell(t, sh, "", "testsh", "-c", "sleep 0.2 &")
		if strings.Contains(errs, "[1] ") {
			t.Errorf("err = %q, want a script told nothing", errs)
		}
	})
}

// The dialect's prompt style has to reach the prompt.
//
// The whole arrangement rests on there being one front end: `./bash` and
// `sh -dialect bash` are the same code with a different value, so a dialect's
// answer that is never carried across is a difference between the two, which
// AGENTS.md calls a driver bug by construction. Nothing was checking that this
// one was carried — the repl tests set the field directly and the dialect
// tests read it back, and both passed with the wire cut.
func TestTheDialectsPromptStyleReachesThePrompt(t *testing.T) {
	for _, tc := range []struct {
		name  string
		style repl.PromptStyle
		want  string
	}{
		{"expanded", repl.PromptStyle{Expand: interp.PromptExpandsAlways}, "[someone]"},
		{"as it stands", repl.PromptStyle{}, "[$who]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := shell()
			sh.PromptStyle = tc.style
			_, errs, _ := runPipedShell(t, sh, "who=someone\nPS1='[$who]'\n:\n", "testsh", "-i")
			if !strings.Contains(errs, tc.want) {
				t.Errorf("prompts = %q, want one of them to be %q", errs, tc.want)
			}
		})
	}
}

// What a dialect says about the front end reaches the front end.
//
// The editor is only built where there is a terminal, so a test cannot get at
// it through a run — and two mutations of this wiring survived everything
// while it was written inline, because an answer dropped here looks exactly
// like a dialect that did not answer.
func TestTheDialectsAnswersReachTheFrontEnd(t *testing.T) {
	sh := shell()
	sh.Name = "testsh"
	sh.PromptStyle = repl.PromptStyle{Expand: interp.PromptExpandsAlways, Escape: '%'}
	sh.EditorStyle = repl.EditorStyle{Interrupt: "<int>"}
	sh.HistoryStyle = repl.HistoryStyle{SearchPrompt: "<search %s>", Ignore: "SOMEVAR"}
	front := sh.FrontEndForTest(nil, "testsh", interp.Diagnostics{})
	if front.Editor.Interrupt != "<int>" {
		t.Errorf("Interrupt = %q, want it carried across", front.Editor.Interrupt)
	}
	if front.History.SearchPrompt != "<search %s>" || front.History.Ignore != "SOMEVAR" {
		t.Errorf("History = %+v, want it carried across", front.History)
	}
	if front.Style.Expand == nil || front.Style.Escape != '%' {
		t.Errorf("Style = %+v, want it carried across", front.Style)
	}
	if front.Name != "testsh" {
		t.Errorf("Name = %q, want testsh", front.Name)
	}
}

// The shell's own name has to reach the prompt too, for the code that draws
// it — and by its basename, since a shell invoked as ./build/bash calls itself
// bash.
func TestTheShellsNameReachesThePrompt(t *testing.T) {
	sh := shell()
	sh.PromptStyle = repl.PromptStyle{
		Escape: '\\',
		Codes:  map[rune]repl.PromptField{'s': repl.FieldShellName},
	}
	_, errs, _ := runPipedShell(t, sh, "PS1='[\\s]'\n:\n", "/somewhere/testsh", "-i")
	if !strings.Contains(errs, "[testsh]") {
		t.Errorf("prompts = %q, want the shell to name itself testsh", errs)
	}
}

// `-s` is the explicit "read standard input" spelling, and standard input from
// a terminal is a person: all four shells prompt for `sh -s` there and read a
// script for `echo x | sh -s`.
//
// Reading it as a script either way meant waiting for an end-of-file nobody
// was going to type — which does not look like a shell waiting for a decision,
// it looks like a hang.
func TestDashSReadsAPersonOrAScript(t *testing.T) {
	t.Run("a script where standard input is not a terminal", func(t *testing.T) {
		out, errs, _ := runPiped(t, "echo scripted\n", "testsh", "-s")
		if !strings.Contains(out, "scripted\n") {
			t.Errorf("out = %q, want it read as a script", out)
		}
		if strings.Contains(errs, "$ ") {
			t.Errorf("err = %q, want no prompt", errs)
		}
	})

	t.Run("and a prompt where -i says so", func(t *testing.T) {
		// `-i` is what stands in for a terminal in a test, and it is the
		// same branch a terminal takes.
		_, errs, _ := runPiped(t, "echo prompted\n", "testsh", "-i", "-s")
		if !strings.Contains(errs, "$ ") {
			t.Errorf("err = %q, want a prompt", errs)
		}
	})

	t.Run("the operands are parameters either way", func(t *testing.T) {
		// Unanimous: `sh -s one two` sets `$1` in a script and at a prompt
		// alike, and neither takes an operand as `$0`.
		out, _, _ := runPiped(t, `echo "[$1][$2] n=$#"`+"\n", "testsh", "-s", "one", "two")
		if !strings.Contains(out, "[one][two] n=2") {
			t.Errorf("out = %q, want the operands as parameters", out)
		}
		out, _, _ = runPiped(t, `echo "[$1][$2] n=$#"`+"\n", "testsh", "-i", "-s", "one", "two")
		if !strings.Contains(out, "[one][two] n=2") {
			t.Errorf("out = %q, want the same at a prompt", out)
		}
	})
}

func runPiped(t *testing.T, typed string, argv ...string) (out, errs string, code int) {
	t.Helper()
	return runPipedShell(t, shell(), typed, argv...)
}

// runPipedShell invokes the front end with standard input that is not a
// terminal, which is what `-i` exists to override.
func runPipedShell(t *testing.T, sh driver.Shell, typed string, argv ...string) (out, errs string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	sh.Stdout, sh.Stderr = &o, &e
	sh.Stdin = pipeWith(t, typed)
	code = driver.MainArgs(sh, argv)
	return o.String(), e.String(), code
}

func pipeWith(t *testing.T, s string) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString(s)
		_ = w.Close()
	}()
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// The front end hands the prompt-escape table to *both* readers.
//
// `driver.Shell.PromptStyle` reaches the line editor, which every other test
// here is about, and it reaches the interpreter too — so a shell whose front
// end supplied a table answers `${(%)…}` from that table and not from an
// empty one. That is what makes "one table" true for a caller who assembles a
// dialect out of the three vectors rather than calling a dialect package's
// Apply, and it is the only assertion that fails if the front end stops
// handing it over (#1090).
//
// A table written here rather than taken from a dialect, because the subject
// is the wiring and not a shell: one row, and a row whose answer cannot come
// from anywhere else.
func TestThePromptTableReachesTheInterpreterToo(t *testing.T) {
	sh := shell()
	sh.Dialect.ParamExpansionFlags = true
	sh.PromptStyle = repl.PromptStyle{
		Escape: '%',
		Codes:  map[rune]repl.PromptField{'%': repl.FieldEscape},
	}
	out, errs, code := runArgs(t, sh, "testsh", "-c", `echo "[${(%):-100%%}]"`)
	if out != "[100%]\n" || errs != "" || code != 0 {
		t.Errorf("out=%q errs=%q code=%d, want %q clean", out, errs, code, "[100%]\n")
	}
	// And with no table the same expansion transforms nothing, which is what
	// says the answer above came from the table rather than from a built-in
	// reading of the escape.
	bare := shell()
	bare.Dialect.ParamExpansionFlags = true
	out, errs, code = runArgs(t, bare, "testsh", "-c", `echo "[${(%):-100%%}]"`)
	if out != "[100%%]\n" || errs != "" || code != 0 {
		t.Errorf("with no table: out=%q errs=%q code=%d, want %q clean", out, errs, code, "[100%%]\n")
	}
}

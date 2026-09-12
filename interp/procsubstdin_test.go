// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Which standard input a process substitution's body reads.
//
// The two candidates — the input of the command the word stands in, and the
// input of the shell — are the same stream almost everywhere, so the axis is
// visible in one place only: a pipeline element, whose input is the pipe.
// There the body either reads the shell's own input or eats the pipe the
// outer command was going to read, and the panel splits on which.
//
// `printf` fills the pipe and the shell's input holds something else, so the
// answer names itself.
const (
	procSubStdinPipeline = `printf "PIPE\n" | cat <(` + procSubStdinBody + `)`
	// The body is the shell's own `read` rather than an external `cat`, and
	// that is not a style choice — the corpus rows are written the same way
	// for the same reason. An external body is handed the stream as a
	// descriptor and os/exec starts a process with it, which under the
	// answer that leaves the body on the element's pipe is a process being
	// started from one goroutine while the pipeline closes that same pipe
	// from another. That is #2144, it is older than this axis, and the two
	// spellings are measured to answer identically in every column — so the
	// builtin loses nothing and reaches nothing this change owns.
	procSubStdinBody = `read -r v; printf "%s\n" "$v"`
)

// procSubStdinInput is a real file rather than a strings.Reader, and that is
// load-bearing too: os/exec copies a non-file reader on a goroutine of its
// own, so the *outer* command's inherited input would drain the same reader
// the body is reading and the answer would depend on which copier won. A
// descriptor is handed to a child directly, which is also the shape the
// measurement was made in — `<shell> -c … < outer.txt`.
func procSubStdinInput(t *testing.T, content string) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shell-input")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately not closed on cleanup. A substitution's body is a
	// goroutine that may still be starting a process with this descriptor
	// when the test function returns — the nested case is one — so closing
	// it here is the test racing the shell it is measuring rather than a
	// fault in the shell. The file is in a scratch directory the framework
	// removes, and the process ends.
	return f
}

func procSubStdinSetup(t *testing.T, answer, lastInThisShell Answer, content string) func(*Runner) {
	t.Helper()
	in := procSubStdinInput(t, content)
	return func(r *Runner) {
		sem := testSemantics()
		sem.ProcessSubstitutionBodyReadsTheShellsInput = answer
		sem.LastPipelineElementInCurrentShell = lastInThisShell
		r.Semantics = &sem
		r.Stdin = in
	}
}

func TestAProcessSubstitutionsInputInsideAPipelineIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"the shell's own input", Yes, "OUTER\n"},
		{"the input of the command it stands in", No, "PIPE\n"},
	} {
		// Both sides of the *other* pipeline axis, because they are two code
		// paths here: every element but the last runs on a copy of the
		// shell, and the last one runs on the shell itself where the answer
		// says so — so the stream the pipe replaced has to be recorded in
		// two places, and put back in one of them.
		for _, last := range []struct {
			name   string
			answer Answer
		}{
			{"last element on a copy", No},
			{"last element on the shell itself", Yes},
		} {
			t.Run(tc.name+", "+last.name, func(t *testing.T) {
				out, st := run(t, procSubStdinPipeline,
					procSubStdinSetup(t, tc.answer, last.answer, "OUTER\n"))
				if out != tc.want || st != 0 {
					t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
				}
			})
		}
	}
}

// The shell's own input is still the shell's afterwards.
//
// The sharp case is the last element running on the shell itself, where what
// the pipeline installed is the shell's own field: a record left behind would
// tell every later substitution to read a pipe that is closed. The second
// `read` is what says the shell can still reach its input, and that the
// substitution took one line of it rather than the stream.
func TestTheShellsInputSurvivesAPipelineThatUsedIt(t *testing.T) {
	out, st := run(t,
		`printf "PIPE\n" | cat <(read -r v; printf "[%s]" "$v"); read -r w; printf "(%s)" "$w"`,
		procSubStdinSetup(t, Yes, Yes, "OUTER\nREST\n"))
	if want := "[OUTER](REST)"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// Where the two candidates are the same stream the axis is not asked at all,
// which is what keeps an unanswered one off the common path: a Runner with no
// answer still runs `cat <(cmd)`, and refuses only the shape the shells
// actually disagree about.
func TestTheInputAxisIsAskedOnlyInsideAPipeline(t *testing.T) {
	unanswered := procSubStdinSetup(t, Unspecified, No, "OUTER\n")

	out, st := run(t, `cat <(`+procSubStdinBody+`)`, unanswered)
	if want := "OUTER\n"; out != want || st != 0 {
		t.Errorf("outside a pipeline: got %q (status %d), want %q at 0", out, st, want)
	}

	out, st = run(t, procSubStdinPipeline, procSubStdinSetup(t, Unspecified, No, "OUTER\n"))
	if st == 0 {
		t.Errorf("inside a pipeline: status 0 and %q, want the axis refused by name", out)
	}
	if !strings.Contains(out, "process substitution's body") {
		t.Errorf("inside a pipeline: %q does not name the axis", out)
	}
}

// How far the answer reaches, which is the half that makes it narrow rather
// than a blanket rule. Every row here is a shape where the shell that answers
// Yes agrees with the rest of the panel, so the axis must not move it: a
// compound body, a function body and an `eval`'s program all run once the
// element's redirections are in place, and a substitution written as a
// redirection *operand* is expanded with them rather than before them.
func TestTheInputAnswerReachesOneCommandsWordsAndNoFurther(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
	}{
		{"a grouped element", `printf "PIPE\n" | { cat <(` + procSubStdinBody + `); }`},
		{"a subshell element", `printf "PIPE\n" | ( cat <(` + procSubStdinBody + `) )`},
		{"a function the element calls", `f() { cat <(` + procSubStdinBody + `); }; printf "PIPE\n" | f`},
		{"a program the element evals", `printf "PIPE\n" | eval 'cat <(` + procSubStdinBody + `)'`},
		{"a redirection operand", `printf "PIPE\n" | cat < <(` + procSubStdinBody + `)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, procSubStdinSetup(t, Yes, No, "OUTER\n"))
			if want := "PIPE\n"; out != want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, want)
			}
		})
	}
}

// The body of a substitution is a shell of its own, so a substitution inside
// one inherits what the axis handed the outer body rather than reaching back
// to the pipe.
func TestANestedProcessSubstitutionInheritsTheInputTheOuterBodyGot(t *testing.T) {
	out, st := run(t, `printf "PIPE\n" | cat <(cat <(`+procSubStdinBody+`) < /dev/null)`,
		procSubStdinSetup(t, Yes, No, "OUTER\n"))
	if want := "OUTER\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

// All three spellings ask the one question, which is the reason the
// preparation is one helper: `=(cmd)` runs its body to completion into a
// regular file and moves with `<(cmd)`, while `>(cmd)` replaces its body's
// input with the reading end of its own pipe and so cannot move at all.
func TestEverySpellingOfASubstitutionAsksTheSameInputQuestion(t *testing.T) {
	fileForm := func(d *syntax.Dialect) { d.ProcessSubstitutionToFile = true }

	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"the shell's own input", Yes, "OUTER\n"},
		{"the input of the command it stands in", No, "PIPE\n"},
	} {
		t.Run("=(cmd), "+tc.name, func(t *testing.T) {
			out, st := runGrammar(t, `printf "PIPE\n" | cat =(`+procSubStdinBody+`)`, fileForm,
				procSubStdinSetup(t, tc.answer, No, "OUTER\n"))
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})

		// The writing direction answers the same under both, because what
		// its body reads is the pipe the outer command is writing into. A
		// row that could only pass under one answer would not say that.
		t.Run(">(cmd), "+tc.name, func(t *testing.T) {
			out, st := run(t, `printf "PIPE\n" | tee >(read -r v; printf "[%s]" "$v") >/dev/null`,
				procSubStdinSetup(t, tc.answer, No, "OUTER\n"))
			if want := "[PIPE]"; out != want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, want)
			}
		})
	}
}

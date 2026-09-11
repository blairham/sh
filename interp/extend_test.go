// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// This file is the extension story as a working example rather than as prose.
// It builds a miniature dialect on the core the way a real one would, using
// the three mechanisms the package offers and nothing else.

// prelude is the part of a dialect that needs no Go at all.
//
// Functions shadow builtins and external commands alike, so a dialect can
// define, replace or wrap anything the core provides. This is where most of a
// dialect belongs: it is portable across implementations, testable with any
// shell, and cannot break the substrate.
const prelude = `
basename() { printf '%s\n' "${1##*/}"; }
dirname()  { printf '%s\n' "${1%/*}"; }
greet()    { printf 'hello, %s\n' "$1"; }
`

// newDialect builds a shell the way something downstream would.
func newDialect(t *testing.T, out *bytes.Buffer) *interp.Runner {
	t.Helper()

	// 1. Choose the axes. This is the whole of "which shell am I", and it is
	//    a value rather than a fork of the code.
	//
	//    From the standard and the core, never from a sibling. This file is
	//    the documented example, and it used to build its miniature dialect
	//    on bash's two vectors — demonstrating borrowing where it meant to
	//    demonstrate building (#491). A preset that inherits from a shell
	//    inherits that shell's future mistakes, which is the rule
	//    dialect/doc.go states for the real ones and this is no different.
	sem := interp.PosixSemantics()
	dial := dialectGrammar()
	r := newTestRunner(t, &interp.Runner{
		Stdout: out, Stderr: out,
		Semantics: &sem, Dialect: &dial, Name: "mysh",
		Env: testPATH(),
	})

	// 2. Register what shell cannot express. `cd` has to change the working
	//    directory the runner itself uses; no function can say that.
	r.Register("cd", func(r *interp.Runner, _ context.Context, args []string) int {
		if len(args) == 0 {
			return 0
		}
		// Setting r.Dir is the whole of it. Calling os.Chdir as well —
		// which this used to do — moves the *process*, which is what the
		// core's own cd documents as the thing not to do: it would move
		// every Runner in the program, including ones another package owns.
		// It also broke an unrelated test once the temp directory it had
		// moved into was cleaned up and the process had no cwd left.
		if fi, err := os.Stat(args[0]); err != nil || !fi.IsDir() {
			return 1
		}
		r.Dir = args[0]
		return 0
	})

	// 3. Source the prelude. Everything expressible in shell lives here.
	f, err := syntax.Parse(prelude, dial)
	if err != nil {
		t.Fatalf("prelude: %v", err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("prelude: %v", err)
	}
	return r
}

// dialectGrammar is the miniature dialect's grammar, in one place because the
// prelude and the snippets have to be read the same way — a dialect is what
// parses as well as what it means.
func dialectGrammar() syntax.Dialect { return syntax.Core() }

func runDialect(t *testing.T, r *interp.Runner, out *bytes.Buffer, src string) string {
	t.Helper()
	out.Reset()
	f, err := syntax.Parse(src, dialectGrammar())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out.String()
}

func TestADialectExtendsTheCore(t *testing.T) {
	var out bytes.Buffer
	r := newDialect(t, &out)

	// Functions from the prelude, which needed no Go.
	if got := runDialect(t, r, &out, `basename /a/b/c.txt`); got != "c.txt\n" {
		t.Errorf("basename gave %q", got)
	}
	if got := runDialect(t, r, &out, `dirname /a/b/c.txt`); got != "/a/b\n" {
		t.Errorf("dirname gave %q", got)
	}
	if got := runDialect(t, r, &out, `greet world`); got != "hello, world\n" {
		t.Errorf("greet gave %q", got)
	}

	// The registered builtin, which could not have been a function.
	dir := t.TempDir()
	if got := runDialect(t, r, &out, `cd `+dir+`; printf '%s' "$PWD"`); got != "" {
		t.Logf("PWD is not tracked by the core, which is the dialect's job: %q", got)
	}
	if r.Dir != dir {
		t.Errorf("cd did not change the runner's directory: %q", r.Dir)
	}
}

func TestAPreludeFunctionReplacesABuiltin(t *testing.T) {
	// The reason most of a dialect can be shell: a function shadows a
	// builtin, so the core's answer can be replaced without touching it.
	var out bytes.Buffer
	r := newDialect(t, &out)
	if got := runDialect(t, r, &out, `echo() { printf 'replaced:%s\n' "$1"; }; echo hi`); got != "replaced:hi\n" {
		t.Errorf("a function did not shadow the builtin: %q", got)
	}
}

func TestADialectCanRemoveWhatItDoesNotHave(t *testing.T) {
	// A dialect without a command should not offer it — the same reasoning as
	// the parser refusing a construct rather than accepting it and meaning
	// something else.
	var out bytes.Buffer
	r := newDialect(t, &out)
	// Removing one that has no external equivalent leaves nothing to run.
	r.Unregister("shift")
	if got := runDialect(t, r, &out, `shift`); !strings.Contains(got, "not found") {
		t.Errorf("an unregistered builtin should not run, got %q", got)
	}

	// Removing one that *does* falls through to the external command, which
	// is what a real shell does and is worth asserting rather than assuming:
	// hiding a builtin is not the same as forbidding the name.
	r.Unregister("echo")
	if got := runDialect(t, r, &out, `echo viaPATH`); got != "viaPATH\n" {
		t.Errorf("expected the external echo to run, got %q", got)
	}
}

func TestTheAxesAreValuesNotForks(t *testing.T) {
	// Two dialects differing only in a field, which is the point of the
	// vector: "which shell am I" is data. Both start from the same base and
	// answer one axis differently.
	octal := testSemantics()
	octal.ArithLeadingZeroIsOctal = interp.Yes
	decimal := testSemantics()
	decimal.ArithLeadingZeroIsOctal = interp.No
	for _, tc := range []struct {
		name string
		sem  interp.Semantics
		want string
	}{
		{"octal", octal, "64\n"},
		{"decimal", decimal, "100\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			sem := tc.sem
			r := newTestRunner(t, &interp.Runner{Stdout: &out, Stderr: &out, Semantics: &sem})
			f, err := syntax.Parse(`echo $((0100))`, syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if out.String() != tc.want {
				t.Errorf("got %q, want %q", out.String(), tc.want)
			}
		})
	}
}

// TestReadFileGatedResolvesAgainstTheRunnersDirectory is the PATH rule applied
// to the seam a dialect's builtin reads whole files through.
//
// A Runner carries its own Dir so that two of them in one program do not fight
// over a single process-wide cwd, and os.ReadFile on a bare relative name
// quietly opts out of that: it resolves against the *process*, which no Runner
// owns. `.` and every redirection already go through the runner's own
// resolution and this did not, so a dialect whose function search path may
// hold a relative entry could not read one (#1968).
//
// The test turns on the two directories differing: the runner is put in a
// scratch directory holding the file, while the process stays where `go test`
// started it, where no such name exists. An implementation that reads the
// process's cwd finds nothing here and passes wherever the two happen to
// agree.
func TestReadFileGatedResolvesAgainstTheRunnersDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/relnote", []byte("contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	r := newTestRunner(t, &interp.Runner{Stdout: &out, Stderr: &out, Dir: dir})
	r.Register("readrel", func(r *interp.Runner, _ context.Context, args []string) int {
		b, err := r.ReadFileGated(args[0])
		if err != nil {
			r.Diagnosef("readrel: %v\n", err)
			return 1
		}
		_, _ = r.Out().Write(b)
		return 0
	})
	f, err := syntax.Parse(`readrel relnote`, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if out.String() != "contents\n" {
		t.Errorf("got %q, want %q", out.String(), "contents\n")
	}
}

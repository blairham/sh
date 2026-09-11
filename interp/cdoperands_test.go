// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `cd` decided whether it could move by asking whether the path was a
// directory. That answers a different question from the one a chdir asks: a
// directory the caller may not *enter* is a directory all the same.
//
// So a directory with no execute bit was entered, silently, at status 0 —
// leaving a working directory nothing can be resolved against, where every
// relative path afterwards fails with a reason that names the path rather than
// the `cd` that should have failed. The whole panel refuses (#1492).
//
// The three modes together are what make this the *execute* bit rather than
// "some permission": a directory that may be read and not entered is refused,
// and one that may be entered and not read is not.
func TestCdAsksWhetherTheDirectoryCanBeEntered(t *testing.T) {
	for _, c := range []struct {
		name    string
		mode    os.FileMode
		refused bool
	}{
		{"no permission at all", 0o000, true},
		{"readable but not enterable", 0o444, true},
		{"enterable but not readable", 0o111, false},
		{"the ordinary case", 0o755, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "d")
			if err := os.Mkdir(target, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(target, c.mode); err != nil {
				t.Fatal(err)
			}
			// Put back before the temporary directory is removed, or a mode
			// of 000 outlives the test as a directory nothing can clean up.
			t.Cleanup(func() { _ = os.Chmod(target, 0o755) })

			out, errs := &strings.Builder{}, &strings.Builder{}
			sem := PosixSemantics()
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: out, Stderr: errs,
			})
			// The status is read on the same line as the `cd`, because a
			// `pwd` after it would have left its own 0 behind and every row
			// would pass.
			runCd(t, r, "cd d; echo \"st=$?\"\npwd\n")

			st, where := cdResult(t, out.String())
			if moved := strings.HasSuffix(where, "/d"); moved == c.refused {
				t.Errorf("ended at %q, said %q; want refused=%v", where, errs, c.refused)
			}
			if said := strings.Contains(strings.ToLower(errs.String()), "permission denied"); said != c.refused {
				t.Errorf("said %q, want the reason the operating system gave, refused=%v", errs, c.refused)
			}
			if (st != 0) != c.refused {
				t.Errorf("status = %d, want refused=%v", st, c.refused)
			}
		})
	}
}

// An operand that is not a directory at all still reports what it is, which is
// the neighbor the change above must not have taken with it: the errno now
// comes from the kernel where it used to be written here by hand.
func TestCdOntoSomethingThatIsNotADirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, operand, want string }{
		{"a regular file", "f", "not a directory"},
		{"and a path that is not there", "nope", "no such file or directory"},
	} {
		t.Run(c.name, func(t *testing.T) {
			errs := &strings.Builder{}
			sem := PosixSemantics()
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: &strings.Builder{}, Stderr: errs,
			})
			runCd(t, r, "cd "+c.operand+"\n")
			if !strings.Contains(strings.ToLower(errs.String()), c.want) {
				t.Errorf("said %q, want %q in it", errs, c.want)
			}
		})
	}
}

// An empty operand is not the same thing as no operand, and it is not nothing
// either: three of the panel take it as the directory they are already in, and
// two refuse it in words neither "cannot change" nor "HOME not set" has.
//
// `biCd` read the operand and tested it against "", so `cd ""` went *home* —
// a move nobody asked for, from a shell that had been handed a destination
// (#1491).
func TestCdWithAnEmptyOperand(t *testing.T) {
	for _, c := range []struct {
		name     string
		isError  Answer
		wantSaid string
		status   int
	}{
		{"taken as where the shell already is", No, "", 0},
		{"or refused in a fifth set of words", Yes, "cd: null directory", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, _ := cdTree(t)
			out, errs := &strings.Builder{}, &strings.Builder{}
			sem := PosixSemantics()
			sem.CdEmptyOperandIsAnError = c.isError
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: out, Stderr: errs,
				// A home that is somewhere else, so that going there is
				// visible rather than indistinguishable from staying.
				Vars: map[string]string{"HOME": "/", "PATH": ""},
			})
			runCd(t, r, "cd \"\"; echo \"st=$?\"\npwd\n")
			st, where := cdResult(t, out.String())
			if where != dir {
				t.Errorf("ended at %q, want to have stayed at %q", where, dir)
			}
			if got := strings.TrimSpace(errs.String()); !strings.Contains(got, c.wantSaid) ||
				(c.wantSaid == "" && got != "") {
				t.Errorf("said %q, want %q", got, c.wantSaid)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}

// And it is a *move* rather than a no-op where it is accepted, which is what
// the previous directory says: the shell records where it was, exactly as it
// does for a `cd` that went somewhere else.
func TestAnEmptyOperandIsAMoveToWhereTheShellAlreadyIs(t *testing.T) {
	dir, _ := cdTree(t)
	out := &strings.Builder{}
	sem := PosixSemantics()
	sem.CdEmptyOperandIsAnError = No
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
		Stdout: out, Stderr: &strings.Builder{},
		Vars: map[string]string{"OLDPWD": "MARK", "PATH": ""},
	})
	runCd(t, r, "cd \"\"\necho \"old=$OLDPWD\"\n")
	if got := strings.TrimSpace(out.String()); got != "old="+dir {
		t.Errorf("left %q, want the previous directory recorded as %q", got, dir)
	}
}

// A HOME set to the empty string is an empty destination, not a missing one.
//
// `biCd` read HOME and tested the value against "", so the two reached the
// same branch and a shell whose HOME was set to nothing was told HOME was not
// set. Five of the six panel columns separate them.
func TestCdWithAnEmptyHome(t *testing.T) {
	for _, c := range []struct {
		name     string
		isError  Answer
		wantSaid string
		status   int
	}{
		{"taken as where the shell already is", No, "", 0},
		{"or refused, in the empty operand's words and not HOME's", Yes, "cd: null directory", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, _ := cdTree(t)
			out, errs := &strings.Builder{}, &strings.Builder{}
			sem := PosixSemantics()
			sem.CdEmptyHomeIsAnError = c.isError
			// The answer to the *absent* HOME question set the other way, so
			// that a row reading "HOME not set" could only have got there by
			// confusing empty with absent.
			sem.CdWithoutHomeIsAnError = Yes
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: out, Stderr: errs,
				Vars: map[string]string{"HOME": "", "PATH": ""},
			})
			runCd(t, r, "cd; echo \"st=$?\"\npwd\n")
			st, where := cdResult(t, out.String())
			if where != dir {
				t.Errorf("ended at %q, want to have stayed at %q", where, dir)
			}
			got := strings.TrimSpace(errs.String())
			if strings.Contains(got, "HOME not set") {
				t.Errorf("said %q, want an empty HOME told apart from an absent one", got)
			}
			if !strings.Contains(got, c.wantSaid) || (c.wantSaid == "" && got != "") {
				t.Errorf("said %q, want %q", got, c.wantSaid)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}

// A HOME that is absent is still the other question, and still answered by
// the axis that has always answered it.
func TestCdWithNoHomeAtAllIsStillTheOtherQuestion(t *testing.T) {
	dir, _ := cdTree(t)
	errs := &strings.Builder{}
	sem := PosixSemantics()
	sem.CdWithoutHomeIsAnError = Yes
	// Set the other way, so that anything reached through the empty-HOME
	// branch would be silent and this row would fail.
	sem.CdEmptyHomeIsAnError = No
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
		Stdout: &strings.Builder{}, Stderr: errs,
		Vars: map[string]string{"PATH": ""},
	})
	runCd(t, r, "cd\n")
	if !strings.Contains(errs.String(), "HOME not set") {
		t.Errorf("said %q, want an absent HOME reported as one", errs)
	}
}

// Two operands are a rewrite of the current directory in two of the panel, too
// many in one, and one operand plus something ignored in the rest. We gave the
// last of those to all four dialects.
func TestCdWithTwoOperands(t *testing.T) {
	for _, c := range []struct {
		name        string
		substitutes Answer
		refuses     Answer
		wantDir     string
		wantSaid    string
		status      int
	}{
		{
			// The rewrite: the first occurrence of the first operand in the
			// current directory's path, replaced by the second.
			"a rewrite of the current directory", Yes, No,
			"/beta/two", "", 0,
		},
		{
			// Too many, which is a usage error rather than a directory that
			// would not open.
			"too many operands", No, Yes,
			"/alpha/two", "cd: too many arguments", 2,
		},
		{
			// And the reading we gave everyone: take the first, ignore the
			// rest. It goes somewhere, which is the part that makes it a
			// silent wrong answer rather than a quiet one.
			"the first operand, and the rest ignored", No, No,
			"/alpha/two/alpha", "", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, p := range []string{"alpha/two/alpha", "beta/two"} {
				if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			out, errs := &strings.Builder{}, &strings.Builder{}
			sem := PosixSemantics()
			sem.CdSubstitutesTheOperands = c.substitutes
			sem.CdRefusesExtraOperands = c.refuses
			sem.CdSubstitutionPrintsTheDirectory = No
			dg := Diagnostics{
				CdTooManyOperands:       "cd: too many arguments",
				CdTooManyOperandsStatus: 2,
			}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &dg, Name: "sh",
				Dir: filepath.Join(dir, "alpha", "two"),
				// Reached through the same directory twice on purpose, so a
				// rewrite that replaced the *last* occurrence would land
				// somewhere this can tell apart.
				Stdout: out, Stderr: errs,
				Vars: map[string]string{"PATH": ""},
			})
			runCd(t, r, "cd alpha beta; echo \"st=$?\"\npwd\n")

			st, where := cdResult(t, out.String())
			if where != dir+c.wantDir {
				t.Errorf("ended at %q, want %q", where, dir+c.wantDir)
			}
			if got := strings.TrimSpace(errs.String()); !strings.Contains(got, c.wantSaid) ||
				(c.wantSaid == "" && got != "") {
				t.Errorf("said %q, want %q", got, c.wantSaid)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}

// The rewrite names the string it could not find, and leaves the shell where
// it was. Its own wording, because it is neither a directory that would not
// open nor an operand count.
func TestARewriteOfSomethingNotInTheCurrentDirectory(t *testing.T) {
	dir, _ := cdTree(t)
	out, errs := &strings.Builder{}, &strings.Builder{}
	sem := PosixSemantics()
	sem.CdSubstitutesTheOperands = Yes
	dg := Diagnostics{CdBadSubstitution: "cd: string not in pwd: %[1]s"}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "sh", Dir: dir,
		Stdout: out, Stderr: errs, Vars: map[string]string{"PATH": ""},
	})
	runCd(t, r, "cd nowhere_zz beta; echo \"st=$?\"\npwd\n")
	st, where := cdResult(t, out.String())
	if want := "cd: string not in pwd: nowhere_zz"; !strings.Contains(errs.String(), want) {
		t.Errorf("said %q, want %q in it", errs, want)
	}
	if where != dir {
		t.Errorf("ended at %q, want to have stayed at %q", where, dir)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// And a rewrite that arrived somewhere may be announced, the way `cd -` is —
// one of the two shells with the form prints and the other does not.
func TestARewriteMayAnnounceWhereItWent(t *testing.T) {
	for _, prints := range []Answer{Yes, No} {
		dir := t.TempDir()
		for _, p := range []string{"alpha", "beta"} {
			if err := os.MkdirAll(filepath.Join(dir, p), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		out := &strings.Builder{}
		sem := PosixSemantics()
		sem.CdSubstitutesTheOperands = Yes
		sem.CdSubstitutionPrintsTheDirectory = prints
		r := newTestRunner(t, &Runner{
			Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
			Dir:    filepath.Join(dir, "alpha"),
			Stdout: out, Stderr: &strings.Builder{},
			Vars: map[string]string{"PATH": ""},
		})
		runCd(t, r, "cd alpha beta\n")
		said := strings.TrimSpace(out.String())
		if want := filepath.Join(dir, "beta"); (said == want) != (prints == Yes) {
			t.Errorf("prints=%v: wrote %q, want the directory written only when asked", prints, said)
		}
	}
}

// A third operand is too many for the rewrite as well, and the two shells with
// the form say different things about it: one a sentence, one its usage block.
// Neither implies the other, which is why they are two fields.
func TestARewriteTakesExactlyTwoOperands(t *testing.T) {
	for _, c := range []struct {
		name  string
		dg    Diagnostics
		want  string
		unset string
	}{
		{
			"a sentence and no usage block",
			Diagnostics{CdTooManyOperands: "cd: too many arguments"},
			"cd: too many arguments", "Usage:",
		},
		{
			"or a usage block and no sentence",
			Diagnostics{
				CdTooManyOperandsShowsUsage: true,
				CdTooManyOperandsStatus:     2,
				BuiltinUsage:                map[string]string{"cd": "Usage: cd [-LP] [directory]"},
			},
			"Usage: cd [-LP] [directory]", "too many",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, _ := cdTree(t)
			errs := &strings.Builder{}
			sem := PosixSemantics()
			sem.CdSubstitutesTheOperands = Yes
			dg := c.dg
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &dg, Name: "sh", Dir: dir,
				Stdout: &strings.Builder{}, Stderr: errs,
				Vars: map[string]string{"PATH": ""},
			})
			runCd(t, r, "cd a b c\n")
			if got := errs.String(); !strings.Contains(got, c.want) || strings.Contains(got, c.unset) {
				t.Errorf("said %q, want %q in it and %q not", got, c.want, c.unset)
			}
		})
	}
}

// A shell with no answer refuses rather than guessing, and names the question.
func TestCdOperandAxesRefuseWhenUnanswered(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		set             func(*Semantics)
	}{
		{"an empty operand", "cd \"\"\n", "an empty operand", nil},
		{"a HOME set to nothing", "cd\n", "HOME set to the empty string", func(s *Semantics) {
			s.CdWithoutHomeIsAnError = No
		}},
		{"two operands", "cd a b\n", "rewriting the current directory", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir, _ := cdTree(t)
			errs := &strings.Builder{}
			sem := PosixSemantics()
			if c.set != nil {
				c.set(&sem)
			}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: &strings.Builder{}, Stderr: errs,
				Vars: map[string]string{"HOME": "", "PATH": ""},
			})
			runCd(t, r, c.src)
			if got := errs.String(); !strings.Contains(got, c.want) || !strings.Contains(got, "disagree") {
				t.Errorf("said %q, want the axis named", got)
			}
		})
	}
}

// cdResult reads the two lines the rows above print: the status the `cd` left,
// and where the shell ended up.
//
// The status is printed on the `cd`'s own line rather than read from the
// runner afterwards, because the `pwd` that follows leaves its own 0 behind —
// which is how a status assertion can pass for every answer at once.
func cdResult(t *testing.T, out string) (status int, where string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "st=") {
		t.Fatalf("output %q is not a status line and a directory", out)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(lines[0], "st="))
	if err != nil {
		t.Fatalf("output %q: %v", out, err)
	}
	return n, lines[1]
}

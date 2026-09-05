// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// profileShell is a shell whose home directory holds the given ~/.profile and
// whose streams are buffers, ready to be invoked through MainArgs.
//
// HOME is redirected rather than read: a test that consulted the real one
// would source whatever the person running it wrote, which is both a wrong
// answer and someone else's file.
func profileShell(t *testing.T, sem interp.Semantics, profile string) (Shell, *strings.Builder, *strings.Builder) {
	t.Helper()
	home := t.TempDir()
	if profile != "" {
		if err := os.WriteFile(filepath.Join(home, ".profile"), []byte(profile), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	var out, errs strings.Builder
	return Shell{
		Name:      "testsh",
		Dialect:   syntax.Core(),
		Semantics: sem,
		Stdout:    &out,
		Stderr:    &errs,
	}, &out, &errs
}

// scriptAt writes a program to a file and returns its path.
func scriptAt(t *testing.T, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "program.sh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// withProfile is PosixSemantics, which is the answer for a login shell with a
// script to run in dash, ksh93 and zsh; withoutProfile is bash's.
func withProfile() interp.Semantics { return interp.PosixSemantics() }

func withoutProfile() interp.Semantics {
	s := interp.PosixSemantics()
	s.LoginProfileWhenNonInteractive = false
	return s
}

// TestALoginShellReadsTheProfileWithAScriptToRun is #482: startup ran only
// from the prompt route, so `argv[0] = "-dash"` with a script to run sourced
// nothing and the dash and ksh binaries answered bash's question instead.
//
// Every route, because the answer is a fact about the shell rather than about
// the route: measured 2026-09-05, dash, ksh93 and zsh each read ~/.profile —
// zsh's own login files, in its case — for a script operand, for `-c`, for a
// program on standard input and for `-s` alike, and bash reads none of them on
// any of the four.
func TestALoginShellReadsTheProfileWithAScriptToRun(t *testing.T) {
	const profile = "FROM_PROFILE=yes\n"
	// Every route prints the same thing, so one expectation covers them all
	// and the route is the only variable.
	const probe = `echo "[${FROM_PROFILE-unset}]"`
	script := scriptAt(t, probe+"\n")

	for _, r := range []struct {
		name  string
		argv  func(argv0 string) []string
		stdin string
	}{
		{"a script operand", func(a string) []string { return []string{a, script} }, ""},
		{"-c", func(a string) []string { return []string{a, "-c", probe} }, ""},
		{"standard input", func(a string) []string { return []string{a} }, probe + "\n"},
		{"-s", func(a string) []string { return []string{a, "-s"} }, probe + "\n"},
	} {
		for _, c := range []struct {
			name  string
			argv0 string
			sem   interp.Semantics
			want  string
		}{
			{"a login shell that reads it", "-testsh", withProfile(), "[yes]\n"},
			{"a login shell that does not", "-testsh", withoutProfile(), "[unset]\n"},
			// argv[0] without the dash is not a login shell at all, so the
			// axis has nothing to answer and neither dialect reads it.
			{"not a login shell", "testsh", withProfile(), "[unset]\n"},
			{"not a login shell either way", "testsh", withoutProfile(), "[unset]\n"},
		} {
			t.Run(r.name+"/"+c.name, func(t *testing.T) {
				sh, out, errs := profileShell(t, c.sem, profile)
				if r.stdin != "" {
					sh.Stdin = fileHolding(t, r.stdin)
				}
				if code := MainArgs(sh, r.argv(c.argv0)); code != 0 {
					t.Fatalf("status %d, stderr %q", code, errs)
				}
				if out.String() != c.want {
					t.Errorf("got %q, want %q", out.String(), c.want)
				}
			})
		}
	}
}

// fileHolding puts a program in a regular file and opens it, which is the
// `sh < program` route.
func fileHolding(t *testing.T, src string) *os.File {
	t.Helper()
	f, err := os.Open(scriptAt(t, src))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

// TestTheProfileIsNotTheInteractiveFile keeps the two startup files apart.
//
// ~/.profile is what a person wants set for everything started from the
// session, and $ENV is what only makes sense at a prompt. Reaching the profile
// from a script route by calling startup would have brought $ENV with it, and
// a script inheriting the settings someone wrote for their keyboard is the
// thing $ENV exists not to do.
func TestTheProfileIsNotTheInteractiveFile(t *testing.T) {
	sh, out, errs := profileShell(t, withProfile(), "FROM_PROFILE=yes\n")
	env := scriptAt(t, "FROM_ENV=yes\n")
	t.Setenv("ENV", env)
	script := scriptAt(t, `echo "[${FROM_PROFILE-unset}][${FROM_ENV-unset}]"`+"\n")
	if code := MainArgs(sh, []string{"-testsh", script}); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "[yes][unset]\n"; out.String() != want {
		t.Errorf("got %q, want %q — $ENV is the prompt's file and a script must not inherit it", out.String(), want)
	}
}

// TestTheProfileRunsAsThisShell: the profile is run *by* the shell that is
// about to run the script, and sees what it sees. Measured, `$0` and `$#`
// inside ~/.profile are the script's own in dash and ksh93 — the profile of a
// shell invoked as `-dash script.sh A B` reports two parameters and the
// script's path.
func TestTheProfileRunsAsThisShell(t *testing.T) {
	sh, out, errs := profileShell(t, withProfile(), `echo "PROFILE [$0] n=$# [${1-}]"`+"\n")
	script := scriptAt(t, `echo "SCRIPT [$0] n=$# [${1-}]"`+"\n")
	if code := MainArgs(sh, []string{"-testsh", script, "A", "B"}); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	want := "PROFILE [" + script + "] n=2 [A]\nSCRIPT [" + script + "] n=2 [A]\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

// TestTheInvocationsOptionsComeBeforeTheProfile: `-x` given to the invocation
// is already on while the profile runs, which is measured — dash, ksh93 and
// zsh each trace the profile's own lines. It is the same order the prompt
// route uses, and it is why the profile is sourced after applyOptions rather
// than before it.
func TestTheInvocationsOptionsComeBeforeTheProfile(t *testing.T) {
	sh, out, errs := profileShell(t, withProfile(), `case $- in *x*) echo "PROFILE sees x";; *) echo "PROFILE does not";; esac`+"\n")
	script := scriptAt(t, "true\n")
	if code := MainArgs(sh, []string{"-testsh", "-x", script}); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "PROFILE sees x\n"; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

// TestAProfileThatExitsEndsTheShell: `exit 3` in ~/.profile exits 3 and the
// script never runs, measured in dash, ksh93 and zsh alike. Through Finish, so
// an EXIT trap the profile installed still fires.
func TestAProfileThatExitsEndsTheShell(t *testing.T) {
	sh, out, errs := profileShell(t, withProfile(), "trap 'echo TRAP' EXIT\necho PROFILE\nexit 3\n")
	script := scriptAt(t, "echo SCRIPT\n")
	code := MainArgs(sh, []string{"-testsh", script})
	if code != 3 {
		t.Errorf("status %d, want 3, stderr %q", code, errs)
	}
	if want := "PROFILE\nTRAP\n"; out.String() != want {
		t.Errorf("got %q, want %q — the script must not run", out.String(), want)
	}
}

// TestAMissingProfileIsNotAFailure. Every shell starts for the first time
// without one, and a machine with no HOME at all must not be told about
// `/.profile`, which is a real path and belongs to root.
func TestAMissingProfileIsNotAFailure(t *testing.T) {
	script := scriptAt(t, "echo ran\n")
	t.Run("no file", func(t *testing.T) {
		sh, out, errs := profileShell(t, withProfile(), "")
		if code := MainArgs(sh, []string{"-testsh", script}); code != 0 {
			t.Fatalf("status %d, stderr %q", code, errs)
		}
		if out.String() != "ran\n" {
			t.Errorf("got %q", out.String())
		}
	})
	t.Run("no home", func(t *testing.T) {
		sh, out, errs := profileShell(t, withProfile(), "FROM_PROFILE=yes\n")
		t.Setenv("HOME", "")
		if code := MainArgs(sh, []string{"-testsh", script}); code != 0 {
			t.Fatalf("status %d, stderr %q", code, errs)
		}
		if out.String() != "ran\n" {
			t.Errorf("got %q", out.String())
		}
	})
}

// TestRunDoesNotSourceAProfile: the exported entry points take no argument
// vector, so nothing they are given can make them a login shell. A program
// embedding a runner has an invocation of its own and this shell is not it —
// reading the embedder's ~/.profile because a field defaulted would be the
// library touching state it was never handed.
func TestRunDoesNotSourceAProfile(t *testing.T) {
	sh, out, errs := profileShell(t, withProfile(), "FROM_PROFILE=yes\n")
	const probe = `echo "[${FROM_PROFILE-unset}]"`
	if code := Run(sh, probe+"\n", "-testsh"); code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if want := "[unset]\n"; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

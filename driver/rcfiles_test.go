// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// The two shapes the panel splits into, as vectors rather than as names —
// nothing in this package may name a shell, and both of these are measured
// grids in docs/spec/invocation.md.
//
// bashLike reads one file when interactive, a chain of three when a login
// shell, and never both. zshLike reads a file on every invocation, another
// before the run-commands file and a third after it, all of them under a
// directory a variable may move.
func bashLike() interp.Semantics {
	s := interp.PosixSemantics()
	s.LoginProfileWhenNonInteractive = false
	s.LoginStartupFiles = ".a_profile .a_login .profile"
	s.InteractiveStartupFile = ".arc"
	s.InteractiveStartupFileWhenLogin = interp.No
	s.NonInteractiveStartupVariable = "A_ENV"
	// The letter the other shape spends on its escape hatch means globbing
	// here, which is what the last test in this file is about.
	s.SetFTurnsOffGlobbing = interp.Yes
	s.StartupFileOptions = interp.StartupFileOptions{
		Login:               "-l --login",
		SuppressLogin:       "--noprofile",
		SuppressInteractive: "--norc",
		NameInteractive:     "--rcfile --init-file",
	}
	return s
}

func zshLike() interp.Semantics {
	s := interp.PosixSemantics()
	s.StartupDirectoryVariable = "ZDOTDIR"
	s.UnconditionalStartupFile = ".zenv"
	s.LoginStartupFiles = ".zprofile"
	s.LateLoginStartupFile = ".zlogin"
	s.InteractiveStartupFile = ".zrc"
	s.InteractiveStartupFileWhenLogin = interp.Yes
	s.StartupFileOptions = interp.StartupFileOptions{
		Login:       "-l --login",
		SuppressAll: "-f --no-rcs",
	}
	return s
}

// startupHome fills a directory with a marker file under every name either
// shape reads, each one echoing its own name.
//
// A marker per name rather than one file per test: what a startup rule gets
// wrong is nearly always *which* file and in what order, and a test that wrote
// only the file it expected could not tell "read the right one" from "read
// everything".
func startupHome(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("echo "+n+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const everyStartupName = ".profile .a_profile .a_login .arc .zenv .zprofile .zrc .zlogin"

// cleanStartupEnv clears the startup inputs a developer's own shell may have
// exported into the test process.
//
// ZDOTDIR is the one that bites: a maintainer whose own shell sets it made
// every zsh-shaped case here read files out of *their* directory and report
// that nothing was read at all. t.Setenv registers the restore and the unset
// that follows is what the test actually wants — there is no t.Unsetenv.
func cleanStartupEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"ZDOTDIR", "ENV", "A_ENV"} {
		t.Setenv(name, "")
		_ = os.Unsetenv(name)
	}
}

// readFiles runs one invocation and reports which markers it read, in order.
func readFiles(t *testing.T, sem interp.Semantics, home string, argv ...string) []string {
	t.Helper()
	sh := shell()
	sh.Semantics = sem
	t.Setenv("HOME", home)
	out, errs, _ := runPipedShell(t, sh, "", argv...)
	if strings.Contains(errs, "panic") {
		t.Fatalf("the session panicked: %s", errs)
	}
	var read []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, ".") {
			read = append(read, line)
		}
	}
	return read
}

func equal(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestTheInteractiveStartupFileIsRead is the whole of #807: a person's
// run-commands file was never read, because the front end had a login path and
// no interactive one, and a terminal starts an interactive *non-login* shell.
//
// The grid is measured in docs/spec/invocation.md. Both shapes and all four
// combinations of login and interactive, in one table, because the four are not
// four independent facts — the one place the panel disagrees is whether an
// interactive login shell reads its run-commands file as well as its profile.
func TestTheInteractiveStartupFileIsRead(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t, strings.Fields(everyStartupName)...)
	for _, tc := range []struct {
		name string
		sem  interp.Semantics
		argv []string
		want []string
	}{
		{
			"interactive and not login reads the run-commands file",
			bashLike(),
			[]string{"testsh", "-i"},
			[]string{".arc"},
		},
		{
			"a script reads none of them",
			bashLike(),
			[]string{"testsh", "-c", ":"},
			nil,
		},
		{
			"interactive and login reads the profile and stops",
			bashLike(),
			[]string{"-testsh", "-i"},
			[]string{".a_profile"},
		},
		{
			"a login script reads nothing where the dialect says so",
			bashLike(),
			[]string{"-testsh", "-c", ":"},
			nil,
		},
		{
			// The other shape: every file, in the measured order, with the
			// run-commands file between the two login ones.
			"the four-file shape reads all four in order",
			zshLike(),
			[]string{"-testsh", "-i"},
			[]string{".zenv", ".zprofile", ".zrc", ".zlogin"},
		},
		{
			"and only two without login",
			zshLike(),
			[]string{"testsh", "-i"},
			[]string{".zenv", ".zrc"},
		},
		{
			// The one file any shell in the panel reads for a plain command
			// string, and the reason it is a slot of its own.
			"the unconditional file is read by a script too",
			zshLike(),
			[]string{"testsh", "-c", ":"},
			[]string{".zenv"},
		},
		{
			"and by a login script, with the two login files around it",
			zshLike(),
			[]string{"-testsh", "-c", ":"},
			[]string{".zenv", ".zprofile", ".zlogin"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := readFiles(t, tc.sem, home, tc.argv...); !equal(got, tc.want) {
				t.Errorf("read %v, want %v", got, tc.want)
			}
		})
	}
}

// `-i` makes a shell interactive on every route, not only the one that
// prompts, and the run-commands file follows it there: measured, `bash -i -c
// cmd` and `bash -i script.sh` both read `~/.bashrc`.
//
// This is the half the old split could not reach at all. The script routes
// called the profile directly and never went near the interactive file, so
// `sh -i script.sh` read nothing a person had written.
func TestAnInteractiveScriptReadsTheRunCommandsFile(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t, ".arc")
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{"with a command string", []string{"testsh", "-i", "-c", ":"}},
		{"with a script operand", []string{"testsh", "-i", writeScript(t, ":\n")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := readFiles(t, bashLike(), home, tc.argv...); !equal(got, []string{".arc"}) {
				t.Errorf("read %v, want the run-commands file", got)
			}
		})
	}
}

// The profile is a chain and exactly one link of it runs: the first name that
// can be read wins and the rest are not looked at.
//
// A `.profile` at the end of it is the reason a person who has only ever
// written that one still gets it.
func TestTheLoginProfileIsAChainAndOneLinkRuns(t *testing.T) {
	cleanStartupEnv(t)
	for _, tc := range []struct {
		name    string
		present []string
		want    string
	}{
		{"all three", []string{".a_profile", ".a_login", ".profile"}, ".a_profile"},
		{"the first missing", []string{".a_login", ".profile"}, ".a_login"},
		{"only the last", []string{".profile"}, ".profile"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := startupHome(t, tc.present...)
			if got := readFiles(t, bashLike(), home, "-testsh", "-i"); !equal(got, []string{tc.want}) {
				t.Errorf("read %v, want just %q", got, tc.want)
			}
		})
	}
	t.Run("none of them", func(t *testing.T) {
		if got := readFiles(t, bashLike(), startupHome(t), "-testsh", "-i"); len(got) != 0 {
			t.Errorf("read %v, want nothing", got)
		}
	})
}

// POSIX mode swaps the shell's own run-commands file for the standard's `$ENV`,
// and this is the interactive half of what #596 measured for the
// non-interactive one.
//
// It is a *replacement* rather than a suppression, which is the difference
// between the two halves and is measured: the non-interactive file is not read
// in the mode at all, because the standard has no such file, while the
// standard does have an interactive one.
func TestPosixModeReadsTheStandardsInteractiveFile(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t, ".arc", ".profile")
	env := filepath.Join(home, "env.sh")
	if err := os.WriteFile(env, []byte("echo .from-env\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ENV", env)

	t.Run("under its own name it reads its own file", func(t *testing.T) {
		if got := readFiles(t, bashLike(), home, "testsh", "-i"); !equal(got, []string{".arc"}) {
			t.Errorf("read %v, want the shell's own file and not $ENV", got)
		}
	})
	t.Run("called sh it reads $ENV", func(t *testing.T) {
		got := readFiles(t, bashLike(), home, "sh", "-i")
		if !equal(got, []string{".from-env"}) {
			t.Errorf("read %v, want $ENV and not the shell's own file", got)
		}
	})
	t.Run("a login sh reads the profile and then $ENV", func(t *testing.T) {
		// Both, and in that order — which is where the shape differs from the
		// non-POSIX one above, where a login shell reads no run-commands file
		// at all. Measured in dash, ksh93 and bash-as-`sh` alike.
		got := readFiles(t, bashLike(), home, "-sh", "-i")
		if !equal(got, []string{".profile", ".from-env"}) {
			t.Errorf("read %v, want the profile and then $ENV", got)
		}
	})
	t.Run("a dialect with no file of its own always reads $ENV", func(t *testing.T) {
		if got := readFiles(t, interp.PosixSemantics(), home, "testsh", "-i"); !equal(got, []string{".from-env"}) {
			t.Errorf("read %v, want $ENV", got)
		}
	})
}

// The non-interactive startup variable and the interactive file never both
// happen: a shell that is interactive reads the file and not the variable,
// whichever route it came by.
func TestTheNonInteractiveVariableStandsDownWhenInteractive(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t, ".arc")
	other := filepath.Join(home, "bashenv.sh")
	if err := os.WriteFile(other, []byte("echo .from-a-env\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("A_ENV", other)
	t.Run("a script reads the variable", func(t *testing.T) {
		if got := readFiles(t, bashLike(), home, "testsh", "-c", ":"); !equal(got, []string{".from-a-env"}) {
			t.Errorf("read %v, want the named file", got)
		}
	})
	t.Run("an interactive one reads its own file instead", func(t *testing.T) {
		if got := readFiles(t, bashLike(), home, "testsh", "-i", "-c", ":"); !equal(got, []string{".arc"}) {
			t.Errorf("read %v, want the run-commands file alone", got)
		}
	})
}

// The startup directory variable moves every file at once, and it is read
// afresh for each one — which is the whole reason a person's own unconditional
// file can set it and have the rest follow.
func TestTheStartupDirectoryMovesEveryFile(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t, strings.Fields(everyStartupName)...)
	elsewhere := startupHome(t, ".zenv", ".zprofile", ".zrc", ".zlogin")
	// A marker that says which directory answered.
	for _, n := range []string{".zenv", ".zprofile", ".zrc", ".zlogin"} {
		if err := os.WriteFile(filepath.Join(elsewhere, n), []byte("echo .moved"+n+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("set in the environment", func(t *testing.T) {
		t.Setenv("ZDOTDIR", elsewhere)
		got := readFiles(t, zshLike(), home, "-testsh", "-i")
		want := []string{".moved.zenv", ".moved.zprofile", ".moved.zrc", ".moved.zlogin"}
		if !equal(got, want) {
			t.Errorf("read %v, want %v", got, want)
		}
	})

	t.Run("set by the unconditional file itself", func(t *testing.T) {
		// The file that sets it is found under the home directory and every
		// file after it under the one it named, which only works if the
		// directory is asked for again per file.
		local := startupHome(t)
		if err := os.WriteFile(filepath.Join(local, ".zenv"),
			[]byte("echo .zenv\nexport ZDOTDIR="+elsewhere+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		got := readFiles(t, zshLike(), local, "-testsh", "-i")
		want := []string{".zenv", ".moved.zprofile", ".moved.zrc", ".moved.zlogin"}
		if !equal(got, want) {
			t.Errorf("read %v, want %v", got, want)
		}
	})
}

// A broken startup file has to be escapable, which is the point of modeling the
// options at all: a shell whose only run-commands file fails every time it
// starts is a shell a person cannot repair from.
func TestAStartupFileIsEscapable(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t, strings.Fields(everyStartupName)...)
	for _, tc := range []struct {
		name string
		sem  interp.Semantics
		argv []string
		want []string
	}{
		{
			"--norc drops the run-commands file",
			bashLike(),
			[]string{"testsh", "--norc", "-i"},
			nil,
		},
		{
			"and leaves the profile alone",
			bashLike(),
			[]string{"-testsh", "--norc", "-i"},
			[]string{".a_profile"},
		},
		{
			"--noprofile drops the profile",
			bashLike(),
			[]string{"-testsh", "--noprofile", "-i"},
			nil,
		},
		{
			"and leaves the run-commands file alone",
			bashLike(),
			[]string{"testsh", "--noprofile", "-i"},
			[]string{".arc"},
		},
		{
			"a one-letter option drops every file",
			zshLike(),
			[]string{"-testsh", "-f", "-i"},
			nil,
		},
		{
			"and bundles like any other letter",
			zshLike(),
			[]string{"-testsh", "-if"},
			nil,
		},
		{
			"spelled long it means the same",
			zshLike(),
			[]string{"-testsh", "--no-rcs", "-i"},
			nil,
		},
		{
			// It reaches the file the *script* routes read as well, which is
			// what makes it a whole escape rather than a prompt-only one.
			"and reaches a script's files too",
			zshLike(),
			[]string{"-testsh", "-f", "-c", ":"},
			nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := readFiles(t, tc.sem, home, tc.argv...); !equal(got, tc.want) {
				t.Errorf("read %v, want %v", got, tc.want)
			}
		})
	}
}

// A file that will not run does not make the shell unusable: the session still
// starts, and what a person types after it still runs.
//
// The complaint is not swallowed — a file somebody wrote and the shell declined
// has to say so — but it is not fatal either.
func TestABrokenStartupFileStillLeavesAShell(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t)
	if err := os.WriteFile(filepath.Join(home, ".arc"), []byte("if\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sh := shell()
	sh.Semantics = bashLike()
	t.Setenv("HOME", home)
	// Escaped, the session is clean.
	out, _, _ := runPipedShell(t, sh, "echo alive\n", "testsh", "--norc", "-i")
	if !strings.Contains(out, "alive") {
		t.Errorf("out = %q, want the escaped session to work", out)
	}
	// And unescaped it is still reported rather than silently skipped.
	_, errs, code := runPipedShell(t, sh, "", "testsh", "-i")
	if code == 0 {
		t.Error("a run-commands file that does not parse reported 0")
	}
	if !strings.Contains(errs, ".arc") {
		t.Errorf("err = %q, want the file named", errs)
	}
}

// `--rcfile` replaces the run-commands file, and it loses to everything that
// was already going to skip one.
func TestNamingTheRunCommandsFile(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t, strings.Fields(everyStartupName)...)
	alt := filepath.Join(home, "alternate")
	if err := os.WriteFile(alt, []byte("echo .alternate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		argv []string
		want []string
	}{
		{"it replaces the file", []string{"testsh", "--rcfile", alt, "-i"}, []string{".alternate"}},
		{"under either spelling", []string{"testsh", "--init-file", alt, "-i"}, []string{".alternate"}},
		{"--norc still wins", []string{"testsh", "--norc", "--rcfile", alt, "-i"}, nil},
		{"whichever order", []string{"testsh", "--rcfile", alt, "--norc", "-i"}, nil},
		{
			// A login shell was not going to read a run-commands file at all
			// in this shape, so naming one changes nothing.
			"and a login shell reads its profile regardless",
			[]string{"-testsh", "--rcfile", alt, "-i"},
			[]string{".a_profile"},
		},
		{"a script is not interactive, so nothing is read", []string{"testsh", "--rcfile", alt, "-c", ":"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := readFiles(t, bashLike(), home, tc.argv...); !equal(got, tc.want) {
				t.Errorf("read %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("with nothing after it, it is refused", func(t *testing.T) {
		sh := shell()
		sh.Semantics = bashLike()
		_, errs, code := runArgs(t, sh, "testsh", "--rcfile")
		if code == 0 {
			t.Error("a naming option with no name reported 0")
		}
		if !strings.Contains(errs, "--rcfile") {
			t.Errorf("err = %q, want the option named", errs)
		}
	})
}

// A dialect that names none of the options has none of them, and the words go
// back to meaning what they meant before — which for `-f` is a set option in
// three of the four shells.
func TestADialectWithoutTheOptionsDoesNotEatTheWords(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t, ".arc")
	// bashLike names no one-letter option, so `-f` is a set option and reaches
	// the runner, where the core turns globbing off rather than skipping a
	// file.
	sh := shell()
	sh.Semantics = bashLike()
	t.Setenv("HOME", home)
	out, _, _ := runPipedShell(t, sh, "", "testsh", "-f", "-i", "-c", `echo $-`)
	if !strings.Contains(out, ".arc") {
		t.Errorf("out = %q, want the run-commands file still read", out)
	}
	if !strings.Contains(out, "f") {
		t.Errorf("out = %q, want the letter to have reached the option set", out)
	}
}

// A login shell can be asked for, rather than only inferred from a dashed
// argv[0] — which is what `login` does and what a person at a keyboard cannot.
//
// The option is stronger than the inference in one measured respect: it reads
// the profile even where there is a script to run and the dialect says a login
// shell started by argv[0] would not.
func TestLoginCanBeAskedForOnTheCommandLine(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t, strings.Fields(everyStartupName)...)
	for _, tc := range []struct {
		name string
		sem  interp.Semantics
		argv []string
		want []string
	}{
		{"the short spelling", bashLike(), []string{"testsh", "-l", "-i"}, []string{".a_profile"}},
		{"the long one", bashLike(), []string{"testsh", "--login", "-i"}, []string{".a_profile"}},
		{"bundled with another letter", bashLike(), []string{"testsh", "-il"}, []string{".a_profile"}},
		{
			// The half the inference does not do in this shape.
			"and it reads the profile with a script to run",
			bashLike(),
			[]string{"testsh", "--login", "-c", ":"},
			[]string{".a_profile"},
		},
		{
			"where the dashed name alone reads nothing",
			bashLike(),
			[]string{"-testsh", "-c", ":"},
			nil,
		},
		{
			"--noprofile still wins",
			bashLike(),
			[]string{"testsh", "--login", "--noprofile", "-i"},
			nil,
		},
		{
			"and the other shape reads all four",
			zshLike(),
			[]string{"testsh", "-l", "-i"},
			[]string{".zenv", ".zprofile", ".zrc", ".zlogin"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := readFiles(t, tc.sem, home, tc.argv...); !equal(got, tc.want) {
				t.Errorf("read %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("a dialect that does not name it leaves the letter alone", func(t *testing.T) {
		// `-l` is a set option's letter to a shell with no such invocation
		// option, which is the same rule `-f` follows.
		sem := interp.PosixSemantics()
		sem.LoginStartupFiles = ".profile"
		if got := readFiles(t, sem, home, "testsh", "-l", "-i"); len(got) != 0 {
			t.Errorf("read %v, want nothing — the letter is not this dialect's", got)
		}
	})
}

// TestEveryStartupNameIsDistinct keeps the table above honest: two slots
// sharing a marker would make an ordering test pass for the wrong reason.
func TestEveryStartupNameIsDistinct(t *testing.T) {
	names := strings.Fields(everyStartupName)
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	for i := 1; i < len(sorted); i++ {
		if sorted[i] == sorted[i-1] {
			t.Errorf("%q appears twice", sorted[i])
		}
	}
}

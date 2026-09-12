// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// The startup files a machine's administrator owns, which every shell in the
// panel reads and this one read none of until #1717.
//
// It matters on macOS in particular, where the system-wide profile's whole job
// is to run `path_helper` and rebuild `$PATH` from `/etc/paths` and
// `/etc/paths.d`. A login shell that skips it keeps whatever order it was
// handed, which is how `command -v git` came to answer `/opt/homebrew/bin/git`
// where the reference shell answered `/usr/bin/git`.
//
// **Nothing in the oracle corpus can reach any of this**, and that is a
// property of the corpus rather than an omission here: every row is a `-c`
// snippet run by a shell that is neither a login shell nor interactive, so no
// row is ever in a slot where a startup file is read. The seven startup-file
// axes that predate this one are unreachable for the same reason. So these are
// driver-level tests, and they are the only instrument the rule has.
//
// The files are named without a leading dot, the way the real ones are —
// `/etc/profile` beside `~/.profile` — so a marker line says which of the pair
// was read without the test having to spell it out.

// systemDir fills a directory with a marker under every name either shape
// reads from the system-wide directory, each echoing its own name with a
// prefix that distinguishes it from the person's file of the same slot.
func systemDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		body := "echo etc/" + n + "\n"
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const everySystemName = "profile zenv zprofile zrc zlogin"

// bashLikeSystem is the bash-shaped vector with the one system-wide file that
// shape has: a profile, and nothing in the run-commands slot.
//
// The empty run-commands slot is measured rather than assumed. `/etc/bashrc`
// exists on the machine this was written on and sets `PS1` and `checkwinsize`;
// a `~/.bashrc` that reports `$PS1` sees bash's own default and `shopt
// checkwinsize` answers `off` in bash 3.2, so bash reaches that file only
// through `/etc/profile`, which sources it by hand.
func bashLikeSystem() interp.Semantics {
	s := bashLike()
	s.SystemStartupFiles = interp.SystemStartupFiles{Login: "profile"}
	return s
}

// zshLikeSystem is the four-slot shape with a system-wide file in front of
// every one of them, and the escape hatch that drops those and keeps the
// person's.
func zshLikeSystem() interp.Semantics {
	s := zshLike()
	s.SystemStartupFiles = interp.SystemStartupFiles{
		Unconditional: "zenv",
		Login:         "zprofile",
		Interactive:   "zrc",
		LateLogin:     "zlogin",
	}
	s.StartupFileOptions.SuppressSystem = "-d --no-globalrcs"
	return s
}

// readEveryFile is readFiles' counterpart that keeps the system markers too,
// so the *interleaving* of the two directories is what the test reads.
func readEveryFile(t *testing.T, sem interp.Semantics, home, etc string, argv ...string) []string {
	t.Helper()
	sh := shell()
	sh.Semantics = sem
	sh.SystemStartupDirectory = etc
	t.Setenv("HOME", home)
	out, errs, _ := runPipedShell(t, sh, "", argv...)
	if strings.Contains(errs, "panic") {
		t.Fatalf("the session panicked: %s", errs)
	}
	var read []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, ".") || strings.HasPrefix(line, "etc/") {
			read = append(read, line)
		}
	}
	return read
}

// TestASystemFileComesFirstInItsOwnSlot is the whole shape of #1717 in one
// table, and the interleaving is the claim.
//
// Measured 2026-09-12 with `zsh -o sourcetrace`, which names each file as it is
// read: an interactive login zsh writes `~/.zshenv`, `/etc/zprofile`,
// `~/.zprofile`, `/etc/zshrc`, `~/.zshrc`, `~/.zlogin` — so a system file is
// paired with the file it precedes rather than hoisted to the front. A shell
// that read all of root's files first would run `/etc/zshrc` before
// `~/.zprofile`, and `~/.zprofile` is where a person sets `$ZDOTDIR` and
// `$PATH`.
//
// The probe discriminates because both directories carry a marker for every
// slot: a hoisting shell and a pairing shell read the same six files, and only
// the order tells them apart.
func TestASystemFileComesFirstInItsOwnSlot(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t, strings.Fields(everyStartupName)...)
	etc := systemDir(t, strings.Fields(everySystemName)...)
	for _, tc := range []struct {
		name string
		sem  interp.Semantics
		argv []string
		want []string
	}{
		{
			// The bash shape, and the row #1717 is about: the person's
			// profile runs *after* the machine's, so what `path_helper` did
			// to `$PATH` is what their file sees.
			"a login shell reads the machine's profile before the person's",
			bashLikeSystem(),
			[]string{"-testsh", "-i"},
			[]string{"etc/profile", ".a_profile"},
		},
		{
			// And only in that slot. A shell that is not a login shell reads
			// the run-commands file and nothing of root's, because this shape
			// has no system-wide run-commands file at all.
			"a plain prompt reads none of it",
			bashLikeSystem(),
			[]string{"testsh", "-i"},
			[]string{".arc"},
		},
		{
			// The four-slot shape, interleaved. This is the row that could
			// not be written any other way: it is the only one where hoisting
			// and pairing differ.
			"the four-slot shape pairs each file with the slot it precedes",
			zshLikeSystem(),
			[]string{"-testsh", "-i"},
			[]string{
				"etc/zenv", ".zenv",
				"etc/zprofile", ".zprofile",
				"etc/zrc", ".zrc",
				"etc/zlogin", ".zlogin",
			},
		},
		{
			// The unconditional slot is the only one a plain command string
			// reaches, and root's file is in it too.
			"a command string reaches the unconditional slot and no other",
			zshLikeSystem(),
			[]string{"testsh", "-c", ":"},
			[]string{"etc/zenv", ".zenv"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := readEveryFile(t, tc.sem, home, etc, tc.argv...); !equal(got, tc.want) {
				t.Errorf("read %v, want %v", got, tc.want)
			}
		})
	}
}

// TestAShellWithNoSystemDirectoryReachesForNothing pins the default, which is
// the half of the design that keeps a test suite off the machine it runs on.
//
// `/etc/profile` is an absolute path with no environment variable in front of
// it, so a scratch `HOME` cannot redirect it and internal/testenv cannot see
// it. The empty default is what makes a Shell a test built by hand safe, and a
// preset that named the directory itself would have taken that away.
func TestAShellWithNoSystemDirectoryReachesForNothing(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t, strings.Fields(everyStartupName)...)
	// The vectors name every file; only the directory is missing.
	sh := shell()
	sh.Semantics = zshLikeSystem()
	t.Setenv("HOME", home)
	out, _, _ := runPipedShell(t, sh, "", "-testsh", "-i")
	if strings.Contains(out, "etc/") {
		t.Errorf("out = %q, want no system-wide file read with no directory named", out)
	}
	if !strings.Contains(out, ".zenv") {
		t.Errorf("out = %q, want the person's own files still read", out)
	}
}

// TestTheOptionsThatSuppressStartupFilesReachTheSystemOnes is the three-way
// split between them, and each answer is measured.
//
//   - `--noprofile` names the *slot*: `bash --noprofile -l -c 'echo
//     ${PATH%%:*}'` answers the inherited head rather than `path_helper`'s, so
//     it dropped `/etc/profile` along with `~/.bash_profile`.
//   - `-d` names root's files alone: `zsh -d -l -c` still reads `.zshenv`,
//     `.zprofile` and `.zlogin`, and each of them reports the inherited
//     `${PATH%%:*}`, so `/etc/zprofile` did not run.
//   - `-f` drops everything: the same probe reads nothing at all.
func TestTheOptionsThatSuppressStartupFilesReachTheSystemOnes(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t, strings.Fields(everyStartupName)...)
	etc := systemDir(t, strings.Fields(everySystemName)...)
	for _, tc := range []struct {
		name string
		sem  interp.Semantics
		argv []string
		want []string
	}{
		{
			// Nothing at all, where the same invocation without the option
			// reads `etc/profile` then `.a_profile` — that baseline is the
			// first row of the table above, and it is what makes this row
			// discriminating. This shape reads no run-commands file for a
			// login shell, which is why the profile was the whole of it.
			"suppressing the login profile suppresses the machine's too",
			bashLikeSystem(),
			[]string{"-testsh", "--noprofile", "-i"},
			nil,
		},
		{
			// And it reaches the login slot only: the same option on a shell
			// that was never going to read a profile still reads everything
			// else.
			"and it leaves the slots it does not name",
			bashLikeSystem(),
			[]string{"testsh", "--noprofile", "-i"},
			[]string{".arc"},
		},
		{
			"the system-only option leaves every file the person owns",
			zshLikeSystem(),
			[]string{"-testsh", "-d", "-i"},
			[]string{".zenv", ".zprofile", ".zrc", ".zlogin"},
		},
		{
			"and its long spelling is the same option",
			zshLikeSystem(),
			[]string{"-testsh", "--no-globalrcs", "-i"},
			[]string{".zenv", ".zprofile", ".zrc", ".zlogin"},
		},
		{
			"suppressing every file suppresses root's as well",
			zshLikeSystem(),
			[]string{"-testsh", "-f", "-i"},
			nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := readEveryFile(t, tc.sem, home, etc, tc.argv...); !equal(got, tc.want) {
				t.Errorf("read %v, want %v", got, tc.want)
			}
		})
	}
}

// TestNamingARunCommandsFileLeavesTheMachinesAlone is the one rule here that
// nothing measures, written down rather than left implicit.
//
// No shell in the panel has both a `--rcfile` and a system-wide run-commands
// file, so there is no run that could settle it. The reading is that the option
// names a file to be read *in place of the person's own*, and root's is not
// theirs.
func TestNamingARunCommandsFileLeavesTheMachinesAlone(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t, strings.Fields(everyStartupName)...)
	etc := systemDir(t, strings.Fields(everySystemName)...)
	sem := bashLikeSystem()
	sem.SystemStartupFiles.Interactive = "zrc"
	named := filepath.Join(t.TempDir(), "named")
	if err := os.WriteFile(named, []byte("echo .named\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := readEveryFile(t, sem, home, etc, "testsh", "--rcfile", named, "-i")
	if want := []string{"etc/zrc", ".named"}; !equal(got, want) {
		t.Errorf("read %v, want %v", got, want)
	}
}

// TestTheSystemDirectoryIsJoinedOnlyWhenBothHalvesAreThere keeps the join from
// producing `/profile`, which is a real path on a real machine and belongs to
// root.
//
// The same trap Semantics.StartupDirectoryVariable's join has, and it is worth
// its own case for the same reason: the failure is silent, and what it reads is
// the most privileged file on the box.
func TestTheSystemDirectoryIsJoinedOnlyWhenBothHalvesAreThere(t *testing.T) {
	cleanStartupEnv(t)
	home := startupHome(t, strings.Fields(everyStartupName)...)
	sem := bashLikeSystem()
	sem.SystemStartupFiles.Login = ""
	sh := driver.Shell{Name: "testsh", Semantics: sem, SystemStartupDirectory: "/"}
	t.Setenv("HOME", home)
	out, errs, _ := runPipedShell(t, sh, "", "-testsh", "-i")
	if strings.Contains(errs, "panic") {
		t.Fatalf("the session panicked: %s", errs)
	}
	// The person's own files are still read; nothing under the root is.
	if !strings.Contains(out, ".a_profile") {
		t.Errorf("out = %q, want the person's profile", out)
	}
	if strings.Contains(out, "etc/") {
		t.Errorf("out = %q, want nothing from the system directory", out)
	}
}

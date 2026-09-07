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

// The default a function search path arrives with — #1250.
//
// One dialect looks function definition files up on a parameter, and it starts
// with that installation's own directories on it rather than with nothing.
// Which directories is a fact about where the binary was installed, so the
// dialect names the parameter and this package supplies the value.
//
// Nothing here names a shell, for the reason nothing else in this package
// does: the test dialect declares a parameter of its own and ties it, which is
// the whole of what a dialect contributes to this.

// searchShell is a dialect with a function search path and the tie that makes
// its array half follow. The tie is installed by Register because that is
// where a real dialect installs its own, and the order matters: the seed has
// to come after it or it fills a scalar nothing mirrors.
func searchShell() driver.Shell {
	sh := shell()
	sh.Semantics.FunctionSearchVariable = "SH_TEST_SEARCH"
	sh.Register = func(r *interp.Runner) { r.Tie("SH_TEST_SEARCH", "sh_test_search", ":") }
	return sh
}

// TestTheInstallPrefixIsReadOffTheBinarysOwnPath pins the layouts.
//
// `make install` puts every binary in `$(PREFIX)/libexec/sh` and the Homebrew
// formula puts them in the keg's `libexec`, so that shape names a prefix two
// directories up. Anything else is an uninstalled build, whose installation is
// whatever contains the directory it sits in.
//
// The point of the second rule is that it is never *silently* nothing: a
// binary somewhere unexpected still names a prefix, and the directories under
// it are simply not there — which is a search that finds nothing rather than a
// shell with no search at all. Measured of the shell being imitated, an entry
// on the path that does not exist is carried anyway.
func TestTheInstallPrefixIsReadOffTheBinarysOwnPath(t *testing.T) {
	for _, tc := range []struct {
		name string
		exe  string
		want string
	}{
		{"make install", "/usr/local/libexec/sh/zsh", "/usr/local"},
		{"a keg", "/opt/homebrew/Cellar/sh/0.0.0/libexec/sh/zsh", "/opt/homebrew/Cellar/sh/0.0.0"},
		{"a staged tree", "/tmp/stage/usr/libexec/sh/bash", "/tmp/stage/usr"},
		// `libexec` without the `sh` under it is not the layout, and neither
		// is `sh` without `libexec` over it. Both fall back rather than
		// guessing, because a wrong prefix names two directories that are
		// somebody else's.
		{"libexec alone", "/usr/local/libexec/zsh", "/usr/local"},
		{"a directory called sh", "/opt/sh/zsh", "/opt"},
		{"an uninstalled build", "/home/me/checkout/build/sh-zsh", "/home/me/checkout"},
		{"a bare directory", "/opt/zsh", "/"},
		{"nothing at all", "", ""},
	} {
		if got := driver.InstallPrefixForTest(tc.exe); got != tc.want {
			t.Errorf("%s: prefix of %q = %q, want %q", tc.name, tc.exe, got, tc.want)
		}
	}
}

// TestTheSearchPathIsSiteFirst pins the two directories and their order.
//
// `site-functions` before `functions` is the order the shell being imitated
// has, and it is the useful one: what a package installed for this shell wins
// over what the shell shipped, so replacing one function does not mean
// replacing the file it came in.
func TestTheSearchPathIsSiteFirst(t *testing.T) {
	got := driver.FunctionSearchDirsForTest("/opt/pfx")
	want := []string{"/opt/pfx/share/sh/site-functions", "/opt/pfx/share/sh/functions"}
	if len(got) != len(want) {
		t.Fatalf("dirs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dirs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// And a prefix nobody could work out is no directories rather than two
	// rooted at the filesystem's top.
	if dirs := driver.FunctionSearchDirsForTest(""); len(dirs) != 0 {
		t.Errorf("dirs of no prefix = %v, want none", dirs)
	}
}

// TestAShellArrivesWithItsInstallationsFunctionDirectories is the whole of the
// bug: the parameter was empty, so no name could ever be autoloaded.
//
// Asserted on the *shape* rather than on the value, because the value under
// `go test` is derived from the test binary and naming it would be writing
// this machine's answer down. What is being checked is that a shell reaches
// its first line with the two directories on the parameter, in order, and with
// the array half agreeing — which is what `autoload` reads.
func TestAShellArrivesWithItsInstallationsFunctionDirectories(t *testing.T) {
	out, errs, code := runArgs(t, searchShell(), "testsh", "-c",
		`printf '%s\n' "$SH_TEST_SEARCH"; echo "n=${#sh_test_search[@]}"`)
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	scalar, count, _ := strings.Cut(strings.TrimSuffix(out, "\n"), "\n")
	dirs := strings.Split(scalar, ":")
	if len(dirs) != 2 {
		t.Fatalf("search path = %q, want two directories", scalar)
	}
	if !strings.HasSuffix(dirs[0], filepath.Join("share", "sh", "site-functions")) {
		t.Errorf("first entry %q does not end in share/sh/site-functions", dirs[0])
	}
	if !strings.HasSuffix(dirs[1], filepath.Join("share", "sh", "functions")) {
		t.Errorf("second entry %q does not end in share/sh/functions", dirs[1])
	}
	// The tie is what carries the scalar into the array a lookup walks, so a
	// seed the array did not see would be a default nothing reads.
	if count != "n=2" {
		t.Errorf("the array half says %q, want n=2", count)
	}
}

// TestTheEnvironmentReplacesTheDefaultRatherThanAddingToIt is the measured
// rule, and the empty case is the half that is easy to get wrong.
//
// Measured 2026-09-07 against zsh 5.9.2 under `env -i` and `-f`: `FPATH=/x/y`
// gives `$fpath` exactly one element rather than four, and `FPATH=` — present
// and empty — gives it one empty element rather than the three it starts with.
// So the question the seed asks is whether the environment *mentions* the
// name, not whether what it says is worth anything.
func TestTheEnvironmentReplacesTheDefaultRatherThanAddingToIt(t *testing.T) {
	const probe = `printf '%s\n' "$SH_TEST_SEARCH"; echo "n=${#sh_test_search[@]}"`

	t.Run("a value", func(t *testing.T) {
		t.Setenv("SH_TEST_SEARCH", "/x/y")
		out, errs, code := runArgs(t, searchShell(), "testsh", "-c", probe)
		if code != 0 {
			t.Fatalf("status %d, stderr %q", code, errs)
		}
		if out != "/x/y\nn=1\n" {
			t.Errorf("with a value in the environment = %q, want %q", out, "/x/y\nn=1\n")
		}
	})

	t.Run("present and empty", func(t *testing.T) {
		t.Setenv("SH_TEST_SEARCH", "")
		out, errs, code := runArgs(t, searchShell(), "testsh", "-c", probe)
		if code != 0 {
			t.Fatalf("status %d, stderr %q", code, errs)
		}
		if out != "\nn=1\n" {
			t.Errorf("with an empty value in the environment = %q, want %q", out, "\nn=1\n")
		}
	})
}

// TestADialectWithNoFunctionSearchGetsNoDefault keeps the seam narrow.
//
// Three of the four shells have no such search, and a front end that filled a
// parameter for them would be inventing one. The observable is that the name
// this suite's other tests read is not set at all.
func TestADialectWithNoFunctionSearchGetsNoDefault(t *testing.T) {
	if _, ok := os.LookupEnv("SH_TEST_SEARCH"); ok {
		t.Fatal("the environment already names SH_TEST_SEARCH; the suite's scrub has a hole")
	}
	out, errs, code := runArgs(t, shell(), "testsh", "-c",
		`echo "set=${SH_TEST_SEARCH+yes}${SH_TEST_SEARCH-no}"`)
	if code != 0 {
		t.Fatalf("status %d, stderr %q", code, errs)
	}
	if out != "set=no\n" {
		t.Errorf("a dialect with no function search = %q, want %q", out, "set=no\n")
	}
}

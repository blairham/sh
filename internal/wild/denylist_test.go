// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package wild_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/wild"
)

// The deny-list classifies from the path alone, because opening the file is
// exactly what CLEANROOM.md forbids. These are the paths the sweep must
// refuse and the paths it must not.
func TestDeniedClassifiesFromThePathAlone(t *testing.T) {
	for _, tc := range []struct {
		path, want string
	}{
		// Another shell's own distribution — the issue's real sightings.
		{"/src/bash-3.2/examples/scripts/adventure.sh", wild.ReasonShellSource},
		{"/src/bash-3.2/examples/scripts.v2/ren", wild.ReasonShellSource},
		{"/home/x/dash-0.5.12/src/mksignames.sh", wild.ReasonShellSource},
		{"/tmp/zsh-5.9/Functions/Misc/zargs", wild.ReasonShellSource},
		{"/opt/ksh-93u+/src/cmd/ksh93/foo.sh", wild.ReasonShellSource},
		{"/build/busybox-1.36.1/shell/ash_test/run", wild.ReasonShellSource},
		{"/go/pkg/mod/mvdan.cc/sh/v3@v3.8.0/syntax/parser.sh", wild.ReasonShellSource},
		// The marker wins wherever it sits in the path, and a shell tree's
		// own tests directory is still the shell tree.
		{"/src/bash-3.2/tests/run-all", wild.ReasonShellSource},

		// Another project's test data — vim's is the issue's real sighting.
		{"/usr/share/vim/vim91/syntax/testdir/input/sh_12.sh", wild.ReasonTestData},
		{"/usr/share/vim/vim91/syntax/testdir/input/sh_bash.bash", wild.ReasonTestData},
		{"/usr/lib/node_modules/pkg/testdata/hook.sh", wild.ReasonTestData},
		{"/opt/someproject/tests/regress.sh", wild.ReasonTestData},

		// What the sweep actually points at must keep flowing.
		{"/usr/bin/ldd", ""},
		{"/opt/homebrew/bin/brew", ""},
		{"/usr/local/sbin/backup.sh", ""},
		// A *file* whose name carries a marker is not inside anyone's tree —
		// only directories on the way to the file classify it.
		{"/usr/local/bin/bash-wrapper", ""},
		{"/usr/local/bin/tests", ""},
		// Markers match directory-name prefixes, not substrings.
		{"/usr/lib/rebash-2.0/lib.sh", ""},
		{"/opt/latest/run.sh", ""},
	} {
		if got := wild.Denied(tc.path, ""); got != tc.want {
			t.Errorf("Denied(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// This repository's own test files are ours to read — CLEANROOM.md bans
// *other* projects' test data — so the tree the sweep runs from is exempt.
func TestDeniedExemptsOurOwnTree(t *testing.T) {
	own := "/home/me/sh"
	for _, tc := range []struct {
		path, want string
	}{
		{"/home/me/sh/internal/oracle/testdata/record.sh", ""},
		{"/home/me/sh/tests/case.sh", ""},
		// A sibling whose name merely extends ours is not under our tree.
		{"/home/me/sh-other/testdata/case.sh", wild.ReasonTestData},
		{"/home/me/other/testdata/case.sh", wild.ReasonTestData},
	} {
		if got := wild.Denied(tc.path, own); got != tc.want {
			t.Errorf("Denied(%q, %q) = %q, want %q", tc.path, own, got, tc.want)
		}
	}
}

// TestDeniedRefusesAFetchedSuiteInsideOurOwnTree is the deliberate violation:
// it puts another shell's tree in the one place the exemption used to cover
// and asserts the guard still says no.
//
// It is the half that makes the test above mean something. Every path in this
// repository is allowed, so a check that only walked real paths would pass
// with the guard removed entirely — and would have passed while the hole was
// open, which it was: measured before the change, `bash-5.3/tests/case.sub`
// was refused everywhere on the machine except under the checkout, and under
// the checkout is where CLEANROOM.md's own carve-out sends a fetched suite.
// A guard whose exemption swallows the case it exists for reads exactly like a
// guard that works.
//
// The reason matters as much as the refusal. A fetched suite is refused for
// being another shell's *source tree*, which is the rule that has no business
// standing down inside our tree, and not for sitting in a directory called
// `tests`, which is the rule that does.
func TestDeniedRefusesAFetchedSuiteInsideOurOwnTree(t *testing.T) {
	for _, tc := range []struct {
		name, own, path, want string
	}{
		{
			// Where `make bash-suite` would fetch to: gitignored, inside the
			// tree, and still another shell's distribution.
			name: "an unpacked distribution under the build directory",
			own:  "/home/me/sh",
			path: "/home/me/sh/build/suites/bash-5.3/tests/case.sub",
			want: wild.ReasonShellSource,
		},
		{
			name: "the same for zsh",
			own:  "/home/me/sh",
			path: "/home/me/sh/build/suites/zsh-5.9/Test/A01grammar.ztst",
			want: wild.ReasonShellSource,
		},
		{
			name: "an installed distribution's layout, fetched in",
			own:  "/home/me/sh",
			path: "/home/me/sh/build/suites/zsh/5.9/functions/compinit",
			want: wild.ReasonShellSource,
		},
		{
			// Unchanged, and the reason the narrowing is a narrowing: our own
			// testdata is generated from oracle runs and is ours to read.
			name: "our own generated testdata is still ours",
			own:  "/home/me/sh",
			path: "/home/me/sh/internal/oracle/testdata/record.sh",
			want: "",
		},
		{
			name: "a directory of ours called tests is still ours",
			own:  "/home/me/sh",
			path: "/home/me/sh/tests/case.sh",
			want: "",
		},
		{
			// The path *above* the checkout is somebody's own naming. A
			// worktree branched for a bash fix is not bash, and refusing the
			// whole tree for its parent's name would take the sweep out with
			// it.
			name: "a checkout whose own directory name starts with a marker",
			own:  "/home/me/bash-fix",
			path: "/home/me/bash-fix/interp/case.sh",
			want: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := wild.Denied(tc.path, tc.own); got != tc.want {
				t.Errorf("Denied(%q, %q) = %q, want %q", tc.path, tc.own, got, tc.want)
			}
			// Asked of the directory too, because the walk stops on that
			// question and never reaches the file's: a rule that refused the
			// file and let the walk in would open every other file in the
			// tree on the way past.
			if got := wild.DeniedDir(filepath.Dir(tc.path), tc.own); got != tc.want {
				t.Errorf("DeniedDir(%q, %q) = %q, want %q", filepath.Dir(tc.path), tc.own, got, tc.want)
			}
		})
	}
}

// A denied file is counted and never surfaced: the sweep must not open it
// even to read the shebang, and the report must not hand anyone its path.
func TestFindCountsDeniedFilesWithoutOpeningThem(t *testing.T) {
	dir := t.TempDir()
	shellTree := filepath.Join(dir, "bash-3.2", "examples")
	testTree := filepath.Join(dir, "vim", "testdir")
	for _, d := range []string{shellTree, testTree} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// Unreadable on purpose: if Find tries to open them, the count breaks.
	write(t, shellTree, "adventure.sh", "#!/bin/sh\necho hi\n")
	write(t, testTree, "input.sh", "#!/bin/sh\necho hi\n")
	write(t, testTree, "not-even-a-script", "whether this is shell is unknowable without reading it\n")
	for _, p := range []string{
		filepath.Join(shellTree, "adventure.sh"),
		filepath.Join(testTree, "input.sh"),
		filepath.Join(testTree, "not-even-a-script"),
	} {
		if err := os.Chmod(p, 0o000); err != nil {
			t.Fatal(err)
		}
	}
	allowed := write(t, dir, "fine.sh", "#!/bin/sh\necho hi\n")

	paths, skipped := wild.Find(wild.Scope{Dirs: []string{dir}})
	if len(paths) != 1 || paths[0] != allowed {
		t.Errorf("paths = %v, want just %s", paths, allowed)
	}
	if skipped[wild.ReasonShellSource] != 1 {
		t.Errorf("skipped[%q] = %d, want 1", wild.ReasonShellSource, skipped[wild.ReasonShellSource])
	}
	if skipped[wild.ReasonTestData] != 2 {
		t.Errorf("skipped[%q] = %d, want 2", wild.ReasonTestData, skipped[wild.ReasonTestData])
	}
}

// TestFindRefusesAFetchedSuiteInTheTreeItRunsFrom is the same refusal end to
// end, through the walk, with the checkout being the directory the sweep is
// standing in.
//
// The unit test above asks Denied and DeniedDir directly, which leaves the one
// thing between them and the sweep untested: Find takes own from os.Getwd, so
// nothing in a table can put a suite inside it. This does, by standing the
// sweep in a temporary directory and unpacking a distribution below it.
//
// The file is made unreadable, which is how the whole file asserts "never
// opened" rather than "not reported": if the walk tries to read it the count
// breaks rather than quietly succeeding.
func TestFindRefusesAFetchedSuiteInTheTreeItRunsFrom(t *testing.T) {
	// Resolved before the sweep is stood in it. On this platform the scratch
	// directory is reached through a symbolic link, and the walk judges each
	// path *and* its resolved form — deliberately, so a link into a shell's
	// tree is refused — so an unresolved cwd would put our own files outside
	// our own tree and deny them. That is the safe direction to be wrong in
	// and it is not what this test is about.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	// build/ is gitignored, which is what makes it the place a suite is
	// fetched to and never committed — and, until this, the place the walk
	// would read it from.
	suite := filepath.Join(dir, "build", "suites", "bash-5.3", "tests")
	ours := filepath.Join(dir, "internal", "oracle", "testdata")
	for _, d := range []string{suite, ours} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	write(t, suite, "case.sub", "#!/bin/sh\necho hi\n")
	if err := os.Chmod(filepath.Join(suite, "case.sub"), 0o000); err != nil {
		t.Fatal(err)
	}
	// Ours, in a directory whose name the other rule would refuse, so the
	// same run proves the narrowing did not become a blanket denial.
	allowed := write(t, ours, "record.sh", "#!/bin/sh\necho hi\n")

	paths, skipped := wild.Find(wild.Scope{Dirs: []string{dir}, Depth: 6})
	if len(paths) != 1 || paths[0] != allowed {
		t.Errorf("paths = %v, want just %s", paths, allowed)
	}
	if skipped[wild.ReasonShellSource] != 1 {
		t.Errorf("skipped[%q] = %d, want 1", wild.ReasonShellSource, skipped[wild.ReasonShellSource])
	}
	if n := skipped[wild.ReasonTestData]; n != 0 {
		t.Errorf("skipped[%q] = %d, want 0 — our own testdata is ours to read", wild.ReasonTestData, n)
	}
}

// Naming a denied tree as a root is still naming a denied tree. The walk
// refuses it there too, and counts what it did not read — otherwise `-dirs`
// would be a way around the rule the walk enforces everywhere else.
func TestFindRefusesADeniedRoot(t *testing.T) {
	dir := t.TempDir()
	shellTree := filepath.Join(dir, "zsh-5.9", "Functions")
	if err := os.MkdirAll(shellTree, 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, shellTree, "zargs", "#!/bin/zsh\necho hi\n")

	paths, skipped := wild.Find(wild.Scope{Dirs: []string{shellTree}, Shells: wild.ZshScope})
	if len(paths) != 0 {
		t.Errorf("paths = %v, want none", paths)
	}
	if skipped[wild.ReasonShellSource] != 1 {
		t.Errorf("skipped[%q] = %d, want 1", wild.ReasonShellSource, skipped[wild.ReasonShellSource])
	}
}

// A shell's *installed* distribution is the same expression as its source. A
// package manager's per-formula directory and the data directory a shell
// installs its own functions into are both the bare shell name with the
// version below, which the source-tarball markers do not catch.
func TestDeniedCoversAnInstalledShellDistribution(t *testing.T) {
	for _, tc := range []struct {
		path, want string
	}{
		{"/opt/homebrew/Cellar/zsh/5.9.2/share/zsh/functions/zargs", wild.ReasonShellSource},
		{"/opt/homebrew/opt/bash/share/bashbug", wild.ReasonShellSource},
		{"/usr/share/zsh/5.9/functions/zed", wild.ReasonShellSource},
		{"/opt/homebrew/Cellar/dash/0.5.12/bin/wrapper.sh", wild.ReasonShellSource},
		// The name has to be the whole segment, as an installed package's is.
		{"/opt/homebrew/Cellar/zsh-completions/0.35/share/x.zsh", wild.ReasonShellSource},
		{"/opt/homebrew/Cellar/bashdb/5.0/bin/bashdb", ""},
		{"/usr/share/vim/vim91/ftplugin/zsh.vim", ""},
		// A directory the shell merely *looks* in is not the shell's own. What
		// a package installs into site-functions is that package's code,
		// written in zsh by somebody who is not the zsh project.
		{"/opt/homebrew/share/zsh/site-functions/_git", ""},
		{"/usr/share/zsh/site-functions/_brew", ""},
		{"/usr/local/share/bash/completions/git", ""},
	} {
		if got := wild.Denied(tc.path, ""); got != tc.want {
			t.Errorf("Denied(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestDeniedRefusesEveryShapeOfTestDirectory is the table the singular case
// went missing from.
//
// The rule was three names long — testdir, testdata, tests — and plain `test`
// was not one of them, so `<pkg>/share/ncurses/test` was swept like any other
// directory and nine of another project's test scripts were in the population
// on this machine every time `make wild` ran. One more entry would have fixed
// that sighting and left the next one, because the failure was never a missing
// name: it was that the list was written from memory, and nobody's memory
// holds every convention.
//
// So the table is layouts rather than names, and it carries its own opposite.
// Every allowed row is a word that *contains* a denied one — special, contest,
// attest, t9 — because a rule broad enough to catch the conventions is one
// substring away from refusing the whole machine, and a table with no allowed
// rows cannot tell the two apart.
func TestDeniedRefusesEveryShapeOfTestDirectory(t *testing.T) {
	for _, tc := range []struct {
		name, path, want string
	}{
		// The sighting: a singular test directory in the sweep's own default
		// roots, holding a package's own suite.
		{
			"a singular test directory, the miss this fixes",
			"/opt/homebrew/opt/ncurses/share/ncurses/test/listused.sh", wild.ReasonTestData,
		},
		// The same shape one directory deeper, which is where a plugin tree
		// keeps it and where the sweep is about to be pointed.
		{
			"a plugin's own test directory",
			"/home/me/plugins/theme/share/test/prompt.zsh", wild.ReasonTestData,
		},

		// The three that were already covered, kept so a rewrite of the rule
		// cannot drop them on the way past.
		{"tests", "/opt/proj/tests/regress.sh", wild.ReasonTestData},
		{"testdata", "/opt/proj/testdata/case.sh", wild.ReasonTestData},
		{"testdir", "/usr/share/vim/vim91/syntax/testdir/input/sh_12.sh", wild.ReasonTestData},

		// The rest of the family, which the stem catches without anyone
		// having had to think of them.
		{"testsuite", "/opt/proj/testsuite/run.sh", wild.ReasonTestData},
		{"testsuites", "/opt/proj/testsuites/run.sh", wild.ReasonTestData},
		{"testing", "/opt/proj/testing/helper.sh", wild.ReasonTestData},
		{"testcase", "/opt/proj/testcase/01.sh", wild.ReasonTestData},
		{"testcases", "/opt/proj/testcases/01.sh", wild.ReasonTestData},
		{"regress", "/opt/proj/regress/bin/run.sh", wild.ReasonTestData},
		{"regression", "/opt/proj/regression/run.sh", wild.ReasonTestData},

		// Two words, which no entry names and none has to: a name is matched
		// by its first word as well as whole.
		{"test-suite", "/opt/proj/test-suite/run.sh", wild.ReasonTestData},
		{"test_data", "/opt/proj/test_data/case.sh", wild.ReasonTestData},
		{"tests.old", "/opt/proj/tests.old/case.sh", wild.ReasonTestData},
		{"spec-helpers", "/opt/proj/spec-helpers/env.sh", wild.ReasonTestData},

		// Names with no stem, which have to be spelled out.
		{"spec", "/opt/proj/spec/x_spec.sh", wild.ReasonTestData},
		{"specs", "/opt/proj/specs/x.sh", wild.ReasonTestData},
		{"fixture", "/opt/proj/fixture/input.sh", wild.ReasonTestData},
		{"fixtures", "/opt/proj/fixtures/input.sh", wild.ReasonTestData},
		{"jest's __tests__", "/opt/proj/src/__tests__/hook.sh", wild.ReasonTestData},
		{"jest's __mocks__", "/opt/proj/src/__mocks__/git.sh", wild.ReasonTestData},

		// Case is a convention, not an identifier. zsh's own directory is
		// Test, singular and capitalized, and a project vendored from
		// elsewhere carries TestData.
		{"Test", "/opt/proj/Test/A01grammar.sh", wild.ReasonTestData},
		{"TESTS", "/opt/proj/TESTS/run.sh", wild.ReasonTestData},
		{"TestData", "/opt/proj/TestData/case.sh", wild.ReasonTestData},
		{"Spec", "/opt/proj/Spec/x.sh", wild.ReasonTestData},
		{"Fixtures", "/opt/proj/Fixtures/x.sh", wild.ReasonTestData},

		// A suite nested anywhere on the way down is still a suite.
		{"nested", "/opt/proj/lib/spec/support/env.sh", wild.ReasonTestData},

		// And the other half, which is what keeps the rule from being a
		// prefix. Every row here begins with a word the rule knows and is a
		// different word, and a prefix denied all of them: measured, that
		// version refused twelve of this package's own tests by denying the
		// scratch directories they sweep.
		{"testify is a library", "/opt/proj/testify/x.sh", ""},
		{"testuser is a person", "/home/testuser/bin/backup.sh", ""},
		{"tester likewise", "/home/tester/bin/backup.sh", ""},
		// What the Go tool names a scratch directory: the test function, and
		// every test function begins with Test.
		{
			"a scratch directory named after a test function",
			"/var/folders/5n/x/T/TestSweepFindsEveryScript1234/001/fine.sh", "",
		},
		{"special is not spec", "/opt/proj/special/x.sh", ""},
		{"contest ends with a word, it does not begin with one", "/opt/proj/contest/x.sh", ""},
		{"attest likewise", "/opt/proj/attest/x.sh", ""},
		// Perl's t, left readable on purpose: macOS names the per-user
		// temporary directory T, so denying one letter denied every path
		// under $TMPDIR — every scratch tree this package's own tests sweep
		// included.
		{
			"a one-letter name is macOS's temporary directory as often as Perl's suite",
			"/var/folders/5n/x/T/build/fine.sh", "",
		},
		{"tools", "/opt/proj/tools/x.sh", ""},
		// Examples are a project's own programs, written to be read. The red
		// list names test files, the same line that leaves a third-party
		// program in /usr/bin readable.
		{"examples are not test data", "/opt/homebrew/opt/krb5/share/examples/kdc.sh", ""},
		// Only directories classify a file. A file called test is a program.
		{"a file named test", "/usr/local/bin/test", ""},
		{"a file named spec", "/usr/local/bin/spec", ""},
		// What the sweep is for, still flowing.
		{"an ordinary script", "/usr/bin/ldd", ""},
		{"an ordinary helper", "/opt/homebrew/opt/git/libexec/git-core/git-sh-setup", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := wild.Denied(tc.path, ""); got != tc.want {
				t.Errorf("Denied(%q) = %q, want %q", tc.path, got, tc.want)
			}
			// Asked of the directory too. The walk stops on that question,
			// and a rule that refused the file while letting the walk in
			// would have opened every other file in the suite on the way to
			// deciding.
			if got := wild.DeniedDir(filepath.Dir(tc.path), ""); got != tc.want {
				t.Errorf("DeniedDir(%q) = %q, want %q", filepath.Dir(tc.path), got, tc.want)
			}
		})
	}
}

// TestFindRefusesASingularTestDirectory is the same refusal through the walk,
// with the files made unreadable so that "never opened" is asserted rather
// than "not reported".
//
// The unit table above cannot say this. Denied is a pure function of a string
// and would keep answering correctly while the walk read the file anyway —
// which is how the sighting happened: the classifier was asked, said nothing,
// and the sweep went in.
func TestFindRefusesASingularTestDirectory(t *testing.T) {
	dir := t.TempDir()
	suite := filepath.Join(dir, "pkg", "share", "test")
	if err := os.MkdirAll(suite, 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, suite, "listused.sh", "#!/bin/sh\necho hi\n")
	write(t, suite, "savescreen.sh", "#!/bin/sh\necho hi\n")
	for _, n := range []string{"listused.sh", "savescreen.sh"} {
		if err := os.Chmod(filepath.Join(suite, n), 0o000); err != nil {
			t.Fatal(err)
		}
	}
	// The control: a script of the same package, outside the suite, which the
	// sweep exists to read and must keep reading.
	allowed := write(t, filepath.Join(dir, "pkg"), "helper.sh", "#!/bin/sh\necho hi\n")

	paths, skipped := wild.Find(wild.Scope{Dirs: []string{dir}, Depth: 5})
	if len(paths) != 1 || paths[0] != allowed {
		t.Errorf("paths = %v, want just %s", paths, allowed)
	}
	if skipped[wild.ReasonTestData] != 2 {
		t.Errorf("skipped[%q] = %d, want 2", wild.ReasonTestData, skipped[wild.ReasonTestData])
	}
}

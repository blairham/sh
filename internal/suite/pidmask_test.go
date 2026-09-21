// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package suite_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/suite"
)

// A suite file that asks a shell to signal **itself** has to take the
// process id out of what is compared, because two of the references write it
// into a sentence and it is different on every run. The mask that does it is
// the subject here, and it is a mask that can be wrong in the direction
// nothing notices: over-matching costs a file its determinism and reads, on
// the runs where nothing collides, as a file that is fine.
//
// That is #3988. `share/suite/zsh/diagnostics.tests` masked `$$` unanchored,
// this shell writes its diagnostics as `file:builtin:LINENO:`, and a run
// whose pid happened to equal one of those line numbers had the mask eat the
// line number too — so the file answered two different ways, the column came
// back non-deterministic, and it did so on about one run in a hundred.
//
// Two rules are held here, over every file in the tree rather than over the
// three that have the mask today:
//
//  1. a file that names `$$` inside a `sed` does it through the shared
//     `nopid` helper, and
//  2. the helper masks the pid where it is the **operand of a refusal** and
//     leaves alone a number that merely equals it.
//
// The second is checked by running the helper, with a collision constructed
// rather than waited for: every number in the probe line *is* the running
// shell's pid, so the case asks the same question whatever pid it gets.
func TestThePidMaskIsAnchoredOnTheRoleAndNotOnTheDigits(t *testing.T) {
	root := filepath.Join("..", "..", suite.OurRoot)
	var checked int
	for _, path := range testsFiles(t, root) {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		def, has := nopidDefinition(text)
		if !has {
			if line, inline := sedNamingThePid(text); inline != "" {
				t.Errorf("%s masks the pid inline on line %d — %s\n"+
					"\tuse the shared `nopid` helper, which this test can run",
					path, line, inline)
			}
			continue
		}
		checked++
		probeTheMask(t, path, def)
	}
	if checked == 0 {
		t.Fatal("no suite file defines nopid, so this test ran nothing — " +
			"either the helper was renamed or the root is wrong")
	}
}

// probeTheMask runs one file's helper against three lines and requires it to
// tell the pid from two numbers that are not one.
func probeTheMask(t *testing.T, path, def string) {
	t.Helper()
	// Every number below is `$p`, which is the probe shell's own pid: the
	// collision is built rather than waited for, so the answer does not
	// depend on what pid this run happens to get.
	script := def + `
p=$$
one=$(printf '%s\n' "f.tests:kill:$p: kill $p failed: invalid argument" | nopid)
[ "$one" = "f.tests:kill:$p: kill <pid> failed: invalid argument" ] || printf 'operand-failed [%s]\n' "$one"
two=$(printf '%s\n' "f.tests:kill:$p: kill: $p: no such process" | nopid)
[ "$two" = "f.tests:kill:$p: kill: <pid>: no such process" ] || printf 'errno-failed [%s]\n' "$two"
three=$(printf '%s\n' "f.tests: line $p: kill: 99: invalid signal specification" | nopid)
[ "$three" = "f.tests: line $p: kill: 99: invalid signal specification" ] || printf 'signal-eaten [%s]\n' "$three"
printf 'ran\n'
`
	out, err := exec.Command("/bin/sh", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("%s: running the mask: %v\n%s", path, err, out)
	}
	got := string(out)
	if !strings.Contains(got, "ran\n") {
		t.Fatalf("%s: the probe did not finish: %q", path, got)
	}
	// The first two say the mask still does its job — a mask that matched
	// nothing would pass the third on its own, and then the file it lives in
	// would be non-deterministic for the reason the mask exists.
	if strings.Contains(got, "operand-failed") {
		t.Errorf("%s: the mask left the pid where a refusal names it: %s", path, got)
	}
	if strings.Contains(got, "errno-failed") {
		t.Errorf("%s: the mask left the pid in the errno sentence: %s", path, got)
	}
	// And the third is #3988 itself: a number that is not the pid's role
	// survives, even on the run where it has the pid's value.
	if strings.Contains(got, "signal-eaten") {
		t.Errorf("%s: the mask ate a number that merely equals the pid: %s\n"+
			"\tthis is what made the zsh column non-deterministic (#3988)", path, got)
	}
}

// nopidDefinition is the file's one-line `nopid()` helper, as a shell can run
// it.
func nopidDefinition(text string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "nopid() {") {
			return line, true
		}
	}
	return "", false
}

// sedNamingThePid is a `sed` that rewrites `$$` written anywhere but in the
// helper — the shape #3988 was, and the one this test cannot run.
func sedNamingThePid(text string) (int, string) {
	for i, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.Contains(line, "sed ") && strings.Contains(line, "$$") {
			return i + 1, strings.TrimSpace(line)
		}
	}
	return 0, ""
}

// testsFiles is every suite file in the tree, tier by tier.
func testsFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, suite.OurExt) {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	if len(out) == 0 {
		t.Fatalf("%s holds no %s files", root, suite.OurExt)
	}
	return out
}

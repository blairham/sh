// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// This shell's own answer is that a redirection which cannot be made is a
// complaint and nothing more, on a special builtin as on anything else.
//
// The panel records the opposite for the same binary invoked as `sh`, and the
// two are not two builds: `set -o posix` reaches the second answer from the
// first, which is what makes this a mode rather than a property of a name.
func TestPosixModeMakesAFailedRedirectionFatal(t *testing.T) {
	out, st := answersRun(t, "exec 3>/nope/x\necho after\n")
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("out %q status %d, want this shell's own answer, which carries on", out, st)
	}

	out, st = answersRun(t, "set -o posix\nexec 3>/nope/x\necho after\n")
	if strings.Contains(out, "after") {
		t.Errorf("out %q, want posix mode to have ended the script", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want this shell's fatal status", st)
	}

	out, st = answersRun(t, "set -o posix\nset +o posix\nexec 3>/nope/x\necho after\n")
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("out %q status %d, want leaving the mode to restore the answer", out, st)
	}
}

// The request itself is granted rather than refused, which it was not while
// the name stood for something this shell did not do. `set +o posix` is the
// thirteenth line of Homebrew's own script and has always been granted; `set
// -o posix` is the direction that changed.
func TestPosixModeIsGrantedAndListed(t *testing.T) {
	out, st := answersRun(t, "set -o posix\necho \"st=$?\"\n")
	if st != 0 || !strings.Contains(out, "st=0") {
		t.Errorf("out %q status %d, want the request granted", out, st)
	}
	if strings.Contains(out, "not implemented") || strings.Contains(out, "invalid option") {
		t.Errorf("out %q, want no complaint", out)
	}

	// And the listing reports the state it is really in, both ways round.
	out, _ = answersRun(t, "set -o posix\nset -o | grep '^posix'\n")
	if !strings.Contains(out, "posix") || !strings.Contains(out, "on") {
		t.Errorf("out %q, want the listing to show the mode on", out)
	}
	out, _ = answersRun(t, "set -o | grep '^posix'\n")
	if !strings.Contains(out, "off") {
		t.Errorf("out %q, want the listing to show the mode off by default", out)
	}
}

// The mode moves a second axis, and the two have to be remembered apart.
//
// `unset` of a readonly name is a complaint this shell carries on past, and
// POSIX mode ends the script on it — measured on bash 5.3.15 both ways round,
// and on that binary invoked as `sh`, where `set +o posix` reaches this
// shell's own answer. bash 3.2 is fatal in neither mode, so what is recorded
// here is bash 5's.
//
// It is not the redirection axis read twice: `set -o posix` makes no
// difference at all to `unset 1x`, which bash 5.3 accepts in silence whatever
// the mode, and zsh answers the two the opposite way round from ksh93. Two
// saved answers rather than one, and the round trip below is what would catch
// them being collapsed into one.
func TestPosixModeMakesUnsettingAReadonlyNameFatal(t *testing.T) {
	const src = "readonly x=1\nunset x\necho after\n"

	out, st := answersRun(t, src)
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("out %q status %d, want this shell's own answer, which carries on", out, st)
	}

	out, st = answersRun(t, "set -o posix\n"+src)
	if strings.Contains(out, "after") {
		t.Errorf("out %q, want posix mode to have ended the script", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want this shell's fatal status", st)
	}

	out, st = answersRun(t, "set -o posix\nset +o posix\n"+src)
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("out %q status %d, want leaving the mode to restore the answer", out, st)
	}

	// And the two axes are independent on the way back out: leaving the mode
	// has to restore *both* dialect answers, and a single saved value would
	// put one of them back over the other.
	out, st = answersRun(t, "set -o posix\nset +o posix\nexec 3>/nope/x\necho after\n")
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("out %q status %d, want the redirection answer restored as well", out, st)
	}
}

// And the mode moves what a redirection's target is expanded *into*.
//
// This shell's own answer field-splits the target and matches it as a
// pattern, which is the half POSIX forbids; `set -o posix` turns it off and
// `set +o posix` puts it back. Measured 2026-09-16 on bash 5.3.20 and 3.2.57
// alike, in a directory holding exactly `only-one.txt` — and the same answers
// come out of the binary invoked as `sh`, which is the door the driver opens
// to the same mode (#3207).
func TestPosixModeStopsMatchingARedirectionTarget(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "only-one.txt"), []byte("CONTENT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runHere := func(src string) (string, int) {
		t.Helper()
		out, st, err := preset.Combined(t, dialecttest.Base{
			Name: "sh", Dir: dir, Env: []string{"PATH=/usr/bin:/bin"},
		}, src)
		if err != nil {
			return out + "unsupported: " + err.Error(), -1
		}
		return out, st
	}

	if out, st := runHere("cat < only-*.txt\n"); out != "CONTENT\n" || st != 0 {
		t.Errorf("out %q status %d, want this shell's own answer, which matches the pattern", out, st)
	}
	out, st := runHere("set -o posix\ncat < only-*.txt\n")
	if st == 0 || strings.Contains(out, "CONTENT") {
		t.Errorf("out %q status %d, want posix mode to open the literal name and fail", out, st)
	}
	if !strings.Contains(out, "only-*.txt") {
		t.Errorf("out %q, want the name as written in the complaint", out)
	}
	if out, st := runHere("set -o posix\nset +o posix\ncat < only-*.txt\n"); out != "CONTENT\n" || st != 0 {
		t.Errorf("out %q status %d, want leaving the mode to restore the answer", out, st)
	}
	// Splitting moves with it, which is the row that says the mode takes the
	// whole of POSIX's sentence rather than only the pattern half.
	if out, st := runHere("set -o posix\ne=\"a b\"\nprintf 'X\\n' > $e\n"); out != "" || st != 0 {
		t.Errorf("out %q status %d, want a file called `a b` written", out, st)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "a b")); err != nil || string(got) != "X\n" {
		t.Errorf("`a b` = %q (%v), want one file of that name", got, err)
	}
	// And the count is still the other axis's: braces make two words before
	// either question is reached.
	if out, _ := runHere("set -o posix\nprintf 'X\\n' > {c,d}\n"); !strings.Contains(out, "ambiguous redirect") {
		t.Errorf("out %q, want a brace target still ambiguous in the mode", out)
	}
}

// And the mode moves what a **bare `.`** costs, which is a third axis it
// carries and not a second face of the special-builtin one.
//
// Measured 2026-09-19, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> case.sh`
// over a script file with standard input on the null device, a bare `.` on one
// line and `echo after=$?` on the next:
//
//	bash 5.3.20                 the complaint and its usage, 2, `after=2` runs
//	bash 5.3.20 `set -o posix`  the same two lines, and the script ends at 2
//	bash 5.3.20 as `sh`         the same two lines, and the script ends at 2
//	bash 3.2.57                 the same, with 3.2's shorter usage line
//
// The control is `false` on the line before, which runs on under the mode in
// every column — so what ends the script is this builtin's failure and not the
// mode being fatal about everything (#3818).
func TestPosixModeEndsTheScriptOnABareDot(t *testing.T) {
	if out, st := answersRun(t, ".\necho after=$?\n"); !strings.Contains(out, "after=2") || st != 0 {
		t.Errorf("out %q status %d, want this shell's own answer, which runs on", out, st)
	}
	out, st := answersRun(t, "set -o posix\n.\necho after=$?\n")
	if strings.Contains(out, "after=") || st != 2 {
		t.Errorf("out %q status %d, want the mode to end the script at 2", out, st)
	}
	if !strings.Contains(out, "filename argument required") {
		t.Errorf("out %q, want the complaint still written", out)
	}
	// Leaving the mode puts the dialect's own answer back, which is the half
	// a one-way swap gets wrong.
	if out, st := answersRun(t, "set -o posix\nset +o posix\n.\necho after=$?\n"); !strings.Contains(out, "after=2") || st != 0 {
		t.Errorf("out %q status %d, want leaving the mode to restore the answer", out, st)
	}
	// The control: an ordinary failure under the same mode runs on.
	if out, st := answersRun(t, "set -o posix\nfalse\necho after=$?\n"); !strings.Contains(out, "after=1") || st != 0 {
		t.Errorf("out %q status %d, want an ordinary failure to run on in the mode", out, st)
	}
}

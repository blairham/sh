// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
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

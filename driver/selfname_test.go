// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// The name a diagnostic gives the shell, where the dialect answers with one of
// its own rather than taking argv[0].
//
// Three of the four in the panel print argv[0] whole, which is what an empty
// SelfName means and what every other test in this file assumes. The fourth
// prints a fixed name however it was invoked, and that is a different question
// from `$0`, which is still the path — so these check both, together, since a
// change that shortened one along with the other would look correct from
// either side alone.
//
// The axis is named here and never a shell, per the rule for a package outside
// dialect/. What the field is *set* to lives in the dialect that answers it.

const selfName = "named-by-the-dialect"

// withSelfName is the substrate's own shell with that one answer given.
func withSelfName() (sh interp.Diagnostics) {
	sh.SelfName = selfName
	return sh
}

// TestSelfNameNamesTheShellAndNotTheScript. The two routes with no file are
// the shell's own; a script is named by its path, which is unanimous across
// the panel and must not follow the dialect's answer.
func TestSelfNameNamesTheShellAndNotTheScript(t *testing.T) {
	const invokedAs = "/some/where/deep/on/the/machine/shellbinary"

	sh := shell()
	sh.Diagnostics = withSelfName()

	t.Run("a command string", func(t *testing.T) {
		_, errs, _ := runArgs(t, sh, invokedAs, "-c", "nosuchcommand-xyz")
		if !strings.HasPrefix(errs, selfName) {
			t.Errorf("stderr = %q, want it to begin with %q", errs, selfName)
		}
		if strings.Contains(errs, invokedAs) {
			t.Errorf("stderr = %q, want the path it was invoked by left out", errs)
		}
	})

	// The one the issue was filed about, and the one no recorded case can
	// see: the oracle normalizes the shell's own name, so a row agrees
	// whichever of the two is printed. It shows up only when a caller invokes
	// the shell by an absolute path and reads what it wrote — which is what
	// this does.
	t.Run("a parse failure on standard input", func(t *testing.T) {
		path := writeScript(t, "echo one\nif\n")
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		saved := os.Stdin
		os.Stdin = f
		defer func() { os.Stdin = saved }()

		_, errs, _ := runArgs(t, sh, invokedAs)
		if !strings.HasPrefix(errs, selfName) {
			t.Errorf("stderr = %q, want it to begin with %q", errs, selfName)
		}
		if strings.Contains(errs, invokedAs) {
			t.Errorf("stderr = %q, want the path it was invoked by left out", errs)
		}
	})

	t.Run("a script is named by its path", func(t *testing.T) {
		path := writeScript(t, "nosuchcommand-xyz\n")
		_, errs, _ := runArgs(t, sh, invokedAs, path)
		if !strings.Contains(errs, path) {
			t.Errorf("stderr = %q, want the script's own path in it", errs)
		}
		if strings.Contains(errs, selfName) {
			t.Errorf("stderr = %q, want the shell's own name left out — the script is what is named", errs)
		}
	})

	t.Run("a parse failure in a script is named by its path", func(t *testing.T) {
		path := writeScript(t, "echo one\nif\n")
		_, errs, _ := runArgs(t, sh, invokedAs, path)
		if !strings.Contains(errs, path) {
			t.Errorf("stderr = %q, want the script's own path in it", errs)
		}
		if strings.Contains(errs, selfName) {
			t.Errorf("stderr = %q, want the shell's own name left out", errs)
		}
	})
}

// TestSelfNameLeavesDollarZeroAlone, which is the half that makes it a
// separate answer rather than a different value for the name.
//
// Measured: the shell that shortens its diagnostic still reports the whole
// path in `$0` on every route. Writing the short name into the shell's name
// would have satisfied every assertion above and broken this one, silently,
// since nothing else in the suite reads `$0` on a shell with the field set.
func TestSelfNameLeavesDollarZeroAlone(t *testing.T) {
	const invokedAs = "/some/where/deep/on/the/machine/shellbinary"
	sh := shell()
	sh.Diagnostics = withSelfName()

	out, errs, code := runArgs(t, sh, invokedAs, "-c", "echo $0")
	if code != 0 || errs != "" {
		t.Fatalf("status %d stderr %q", code, errs)
	}
	if got := strings.TrimSpace(out); got != invokedAs {
		t.Errorf("$0 = %q, want the path it was invoked by (%q)", got, invokedAs)
	}
}

// TestWithoutSelfNameArgvZeroStillNamesTheShell — the other position of the
// axis, and the majority one. An axis asserted in one position only is a
// default being restated.
func TestWithoutSelfNameArgvZeroStillNamesTheShell(t *testing.T) {
	const invokedAs = "/some/where/deep/on/the/machine/shellbinary"
	_, errs, _ := runArgs(t, shell(), invokedAs, "-c", "nosuchcommand-xyz")
	if !strings.HasPrefix(errs, invokedAs) {
		t.Errorf("stderr = %q, want it to begin with the path it was invoked by", errs)
	}
}

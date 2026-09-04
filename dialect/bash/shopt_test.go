// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/driver"
)

// What `shopt` does here is what bash 5.3 was measured doing: the listing
// format, the statuses of every flag combination, and which options change
// which behavior. The snippets are ours; the outputs are the binary's.

func TestShoptQueriesAndFormats(t *testing.T) {
	for _, tc := range []struct {
		src    string
		want   string
		status int
	}{
		// A bare name prints the two-column line and answers by status.
		{`shopt nullglob`, "nullglob            \toff\n", 1},
		{`shopt -s nullglob; shopt nullglob`, "nullglob            \ton\n", 0},
		// -p prints the command that would restore the state, status alike.
		{`shopt -p nullglob`, "shopt -u nullglob\n", 1},
		{`shopt -s nullglob; shopt -p nullglob`, "shopt -s nullglob\n", 0},
		// -q is the status alone.
		{`shopt -q nullglob`, "", 1},
		{`shopt -s nullglob; shopt -q nullglob`, "", 0},
		{`shopt -q`, "", 0},
		// Every queried name must be on for success.
		{`shopt -s nullglob; shopt -q nullglob dotglob`, "", 1},
		// `--` ends the options.
		{`shopt -s -- nullglob; shopt -q nullglob`, "", 0},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if out != tc.want || st != tc.status {
			t.Errorf("%s = %q status %d, want %q status %d",
				tc.src, out, st, tc.want, tc.status)
		}
	}
}

func TestShoptListings(t *testing.T) {
	// The full listing carries every name, two-column; nothing else on the
	// full list needs pinning here because the states are asserted one by
	// one above and below.
	out, st := runBash(t, t.TempDir(), `shopt`)
	if st != 0 || !strings.Contains(out, "nullglob            \toff\n") ||
		!strings.Contains(out, "sourcepath          \ton\n") {
		t.Errorf("shopt listed %q status %d", out, st)
	}
	// `-s` alone lists what is on, `-u` what is off, `-p` reissuably.
	out, st = runBash(t, t.TempDir(), `shopt -s nullglob; shopt -s`)
	if st != 0 || !strings.Contains(out, "nullglob            \ton\n") ||
		strings.Contains(out, "dotglob") {
		t.Errorf("shopt -s listed %q status %d", out, st)
	}
	out, st = runBash(t, t.TempDir(), `shopt -u`)
	if st != 0 || !strings.Contains(out, "nullglob            \toff\n") ||
		strings.Contains(out, "sourcepath") {
		t.Errorf("shopt -u listed %q status %d", out, st)
	}
	out, st = runBash(t, t.TempDir(), `shopt -p nullglob dotglob`)
	if st != 1 || out != "shopt -u nullglob\nshopt -u dotglob\n" {
		t.Errorf("shopt -p listed %q status %d", out, st)
	}
}

func TestShoptRefusals(t *testing.T) {
	for _, tc := range []struct {
		src    string
		said   string
		status int
	}{
		{`shopt -s nosuchopt`, "shopt: nosuchopt: invalid shell option name", 1},
		{`shopt nosuchopt`, "shopt: nosuchopt: invalid shell option name", 1},
		{`shopt -su nullglob`, "cannot set and unset shell options simultaneously", 1},
		{`shopt -z`, "shopt: usage: shopt [-pqsu] [-o] [optname ...]", 2},
		// A name this shell recognizes and cannot move: refused out loud,
		// never accepted quietly.
		{`shopt -s failglob`, "shopt: failglob: not implemented", 1},
		{`shopt -u sourcepath`, "shopt: sourcepath: not implemented", 1},
	} {
		out, st := runBash(t, t.TempDir(), tc.src)
		if st != tc.status || !strings.Contains(out, tc.said) {
			t.Errorf("%s = %q status %d, want %q status %d",
				tc.src, out, st, tc.said, tc.status)
		}
	}
	// The states already held are granted: off may be turned off, on on.
	for _, src := range []string{`shopt -u failglob`, `shopt -s sourcepath`} {
		if out, st := runBash(t, t.TempDir(), src); st != 0 || out != "" {
			t.Errorf("%s = %q status %d, want a quiet success", src, out, st)
		}
	}
	// The names it knows are switched even when one it does not sits
	// between them, and the status still reports the failure.
	out, st := runBash(t, t.TempDir(),
		`shopt -s nullglob nosuchopt dotglob; shopt -q nullglob && shopt -q dotglob && echo both`)
	if st != 0 || !strings.Contains(out, "both") || !strings.Contains(out, "nosuchopt") {
		t.Errorf("a mixed set gave %q status %d", out, st)
	}
}

func TestShoptWiresTheGlobOptions(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`shopt -s nullglob; echo zz*zz; echo done`, "\ndone"},
		{`shopt -s nullglob; shopt -u nullglob; echo zz*zz`, "zz*zz"},
		{`shopt -s nocasematch; case A in a) echo hit;; esac`, "hit"},
		{`shopt -s nocaseglob; case A in a) echo hit;; *) echo exact;; esac`, "exact"},
	} {
		out, _ := runBash(t, t.TempDir(), tc.src)
		if got := strings.TrimRight(out, "\n"); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// A subshell's shopt is its own: the option is inherited in and the change
// never escapes.
func TestShoptStaysInsideASubshell(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `(shopt -s nullglob); echo zz*zz`)
	if got := strings.TrimSpace(out); got != "zz*zz" {
		t.Errorf("a subshell's shopt escaped: %q", got)
	}
}

func TestShoptDashOReachesTheSetOptions(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `shopt -so pipefail; false | true; echo ps=$?`)
	if st != 0 || !strings.Contains(out, "ps=1") {
		t.Errorf("shopt -so pipefail gave %q status %d, want the pipe failing", out, st)
	}
	out, st = runBash(t, t.TempDir(), `shopt -o`)
	if st != 2 || !strings.Contains(out, "not implemented") {
		t.Errorf("shopt -o alone gave %q status %d, want a refusal", out, st)
	}
}

// bashShell is the same value cmd/bash assembles, minus the prompt wiring the
// tests never reach.
func bashShell(out, errs *bytes.Buffer) driver.Shell {
	return driver.Shell{
		Name:        "bash",
		Dialect:     bash.Dialect(),
		Semantics:   bash.Semantics(),
		Diagnostics: bash.Diagnostics(),
		Prelude:     bash.Prelude(),
		Register:    bash.Apply,
		Stdout:      out,
		Stderr:      errs,
	}
}

// `shopt -s extglob` reaches the parser, one line late by design: the line
// that sets it is already parsed, so the groups are read from the next line
// on — which is measured, and is why both snippets here put the pattern on a
// line of its own.
func TestShoptExtglobTogglesTheGrammarPerLine(t *testing.T) {
	var out, errs bytes.Buffer
	code := driver.MainArgs(bashShell(&out, &errs), []string{
		"bash", "-c",
		"shopt -s extglob\ncase ab in @(ab|cd)) echo hit;; esac\n",
	})
	if code != 0 || strings.TrimSpace(out.String()) != "hit" {
		t.Errorf("extglob on: %q / %q status %d, want hit", out.String(), errs.String(), code)
	}

	out.Reset()
	errs.Reset()
	code = driver.MainArgs(bashShell(&out, &errs), []string{
		"bash", "-c",
		"case ab in @(ab|cd)) echo hit;; esac\n",
	})
	if code == 0 || !strings.Contains(errs.String(), "syntax error") {
		t.Errorf("extglob off: %q / %q status %d, want a syntax error",
			out.String(), errs.String(), code)
	}

	// And off again: the grammar follows the option in both directions.
	out.Reset()
	errs.Reset()
	code = driver.MainArgs(bashShell(&out, &errs), []string{
		"bash", "-c",
		"shopt -s extglob\nshopt -u extglob\necho @(ab)\n",
	})
	if code == 0 || !strings.Contains(errs.String(), "syntax error") {
		t.Errorf("extglob off again: %q / %q status %d, want a syntax error",
			out.String(), errs.String(), code)
	}
}

func TestShoptGlobstarThroughTheBuiltin(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "d", "e"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "d", "e", "f"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runBash(t, dir, `shopt -s globstar; echo **/f`)
	if got := strings.TrimSpace(out); got != "d/e/f" {
		t.Errorf("globstar gave %q, want d/e/f", got)
	}
	out, _ = runBash(t, dir, `echo **/f`)
	if got := strings.TrimSpace(out); got != "**/f" {
		t.Errorf("without globstar gave %q, want the literal pattern", got)
	}
}

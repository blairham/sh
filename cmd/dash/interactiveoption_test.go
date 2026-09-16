// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/testenv"
)

// This suite starts shells, so it runs in a home of its own — a shell reads
// startup files and writes history from the environment it was handed, and a
// suite that hands it the developer's environment is measuring the developer's
// dotfiles. internal/testenv is the guard; its own package comment is the
// argument for its shape.
func TestMain(m *testing.M) {
	if testenv.Assembled() {
		os.Exit(m.Run())
	}
	os.Exit(testenv.Run("cmd/dash", m))
}

// `-o interactive` is this shell's second spelling of `-i`, and it prompts for
// it — #3195.
//
// dash is the second column with the name and the one that shows it is not a
// zsh row: it has no `no` spelling at all, so `+o interactive` is the only way
// back and `-o nointeractive` is `Illegal option` — which is measured and is
// why Semantics.NonInteractiveOptionName stays empty in this preset.
//
// Against the binary's own shell value rather than the library's, because what
// this decides is a *route* and the route is the front end's. Measured
// 2026-09-16 against dash 0.5.12 with the program on a pipe: `dash -o
// interactive` writes its `can't access tty` remark and then a `$ ` prompt
// around the line, exactly as `dash -i` does, and a plain `dash` writes
// neither.
func TestTheInteractiveOptionNameDrawsAPrompt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		argv    []string
		prompts bool
	}{
		{"the option alone", []string{"dash", "-o", "interactive"}, true},
		{"the letter, for comparison", []string{"dash", "-i"}, true},
		{"nothing written", []string{"dash"}, false},
		{"the option under a plus", []string{"dash", "+o", "interactive"}, false},
		{"the letter then the option", []string{"dash", "-i", "+o", "interactive"}, false},
		{"the option then the letter", []string{"dash", "+o", "interactive", "-i"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			out, errs, code := prompt(t, "echo TYPED\n", tc.argv...)
			both := out + errs
			if code != 0 {
				t.Fatalf("status %d, out %q, stderr %q", code, out, errs)
			}
			if !strings.Contains(both, "TYPED") {
				t.Fatalf("said %q, want the typed line to have run under either answer", both)
			}
			if got := strings.Contains(both, "$ "); got != tc.prompts {
				t.Errorf("prompted = %v, want %v — said %q", got, tc.prompts, both)
			}
		})
	}
}

// prompt drives the binary's own shell value with the typed lines on a pipe
// standing in for a person.
func prompt(t *testing.T, typed string, argv ...string) (out, errs string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	sh := shell()
	sh.SystemStartupDirectory = t.TempDir()
	sh.Stdout, sh.Stderr = &o, &e
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString(typed)
		_ = w.Close()
	}()
	t.Cleanup(func() { _ = r.Close() })
	sh.Stdin = r
	code = driver.MainArgs(sh, argv)
	return o.String(), e.String(), code
}

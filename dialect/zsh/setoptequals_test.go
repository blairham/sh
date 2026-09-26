// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// EQUALS: `=cmd` becomes the path the command resolves to, at the head of an
// unquoted word and at the head of an assignment's value and after each of
// its colons. On by default, which is zsh's.
//
// The option was `recorded(…)` here until #4566: accepted, reported back
// correctly by `[[ -o equals ]]` and by `$options[equals]`, and acted on by
// nothing — so `unsetopt equals; print -r -- =ls` wrote the path. That was
// being read as a working control. #4566's own "the option really does switch
// it off" was measured on an *assignment value*, where this shell expanded
// nothing in either state and the two agreed because there was nothing there
// to disagree about.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// `/opt/homebrew/bin/zsh`, `-f -c`, 2026-09-26. `go version -m` on that path
// reports *not a Go executable* and on this shell's own binary reports
// `github.com/blairham/sh/cmd/zsh`, so the two are different programs.
func TestEqualsOptionSwitchesTheExpansionOffAndOnAgain(t *testing.T) {
	for _, tc := range []struct {
		name, src, on, off string
		// found is whether the `on` state is a prefix the run's own path is
		// appended to, rather than the whole answer.
		found bool
	}{
		{"a command word's argument", `print -r -- =p`, "", "=p", true},
		{"an assignment's value", `v==p; print -r -- $v`, "", "=p", true},
		{"after a colon in a value", `v=x:=p; print -r -- $v`, "x:", "x:=p", true},
		{
			"a word MAGIC_EQUAL_SUBST claimed",
			"setopt magicequalsubst\nprint -r -- a==p",
			"a=", "a==p", true,
		},
		// The control that keeps this from reading as "the option removes
		// the characters": a word with no `=cmd` in it is untouched in both
		// states.
		{"no equals expansion in the word", `print -r -- a=p`, "a=p", "a=p", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, st := range []struct{ verb, want string }{
				{"setopt", tc.on}, {"unsetopt", tc.off},
			} {
				got, dir := equalsOptionRun(t, st.verb+" equals\n"+tc.src)
				want := st.want
				if st.verb == "setopt" && tc.found {
					// The expansion answers a real path, so the expectation
					// is the run's own directory with the name on it rather
					// than a string chosen here.
					want += filepath.Join(dir, "p")
				}
				if got != want {
					t.Errorf("%s equals; %s = %q, want %q", st.verb, tc.src, got, want)
				}
			}
		})
	}
}

// The option is remembered and reported whichever way it is spelled, which is
// the half it already had as a recorded name and must keep.
func TestEqualsOptionIsReportedBack(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`[[ -o equals ]] && print -r -- on`, "on"},
		{"unsetopt equals\n[[ -o equals ]] || print -r -- off", "off"},
		{`print -r -- $options[equals]`, "on"},
		{"unsetopt equals\nprint -r -- $options[equals]", "off"},
		{"unsetopt equals\nsetopt equals\nprint -r -- $options[equals]", "on"},
	} {
		if got, _ := equalsOptionRun(t, tc.src); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// equalsOptionRun runs a snippet with a directory of its own on `PATH` and one
// program called `p` in it, and hands back that directory so a row can say
// what the expansion should have found.
func equalsOptionRun(t *testing.T, src string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil { //nolint:gosec // a program has to be executable
		t.Fatalf("write the program: %v", err)
	}
	out, _, err := preset.Combined(t, dialecttest.Base{
		Dir:  dir,
		Vars: map[string]string{"PATH": dir, "HOME": dir},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return strings.TrimRight(out, "\n"), dir
}

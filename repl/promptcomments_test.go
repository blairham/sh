// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// Whether a `#` typed here opens a comment is the *shell's* answer, asked for
// by name and asked again for every line.
//
// Three of the four panel shells open a comment with nothing set, and the
// fourth leaves it to an option that is off until somebody sets it — see
// interp.Semantics.PromptCommentsNeedTheOption for the measurement. A session
// that decided this for itself would be a second shell disagreeing with the
// first about what it was told, which is why the name travels and the state
// does not (#2537).
func TestAHashAtThePromptFollowsTheNamedOption(t *testing.T) {
	for _, tc := range []struct {
		name, option string
		on           bool
		want         string
	}{
		{
			// The majority, and what a caller without a dialect gets.
			name: "no option is named, so a hash opens a comment",
			want: "a\n",
		},
		{
			name:   "the named option is off, so a hash is a word",
			option: "promptcomments",
			want:   "a #b\n",
		},
		{
			name:   "the named option is on, so a hash opens a comment",
			option: "promptcomments",
			on:     true,
			want:   "a\n",
		},
		{
			// A name this shell has never heard of is not a name that is
			// off: DialectOption's second result is the whole difference
			// between a knob turned down and a knob that is not there, and
			// reading a missing name as "off" would turn one dialect's typo
			// into every other dialect's grammar.
			name:   "a name the shell does not have is not a name that is off",
			option: "nobodyhasthis",
			want:   "a\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := commentSession(t, tc.option, tc.on, "echo a #b\n"); got != tc.want {
				t.Errorf("output %q, want %q", got, tc.want)
			}
		})
	}
}

// And it is asked again per line, so a line that moves the option is obeyed
// from the next line on.
//
// Read once at startup it would be a setting a person types, watches do
// nothing, and believes is broken — the same reason the history rules are
// read per accepted line rather than held.
func TestTheCommentOptionIsReReadForEveryLine(t *testing.T) {
	// The stub's own switch is a variable the session sets, so this is the
	// option moving underneath the loop rather than two sessions.
	got := commentSession(t, "promptcomments", false,
		"echo a #b\nPROMPTCOMMENTS=on\necho c #d\nPROMPTCOMMENTS=\necho e #f\n")
	if want := "a #b\nc\ne #f\n"; got != want {
		t.Errorf("output %q, want %q", got, want)
	}
}

// commentSession runs typed through a session whose shell names option and
// holds it at on, and returns what the commands wrote.
func commentSession(t *testing.T, option string, on bool, typed string) string {
	t.Helper()
	var ran, said bytes.Buffer
	vars := map[string]string{"PS1": "", "PS2": "", "PATH": "/bin:/usr/bin"}
	if on {
		vars["PROMPTCOMMENTS"] = "on"
	}
	r := newTestRunner(vars)
	r.Stdout = &ran
	// One name, answered from a variable so a line the session runs can move
	// it. Every other name is unknown, which is what the last row above is
	// about.
	r.SetOptionNamespace(func(r *interp.Runner, name string) (bool, bool) {
		if name != "promptcomments" {
			return false, false
		}
		v, _ := r.GetVar("PROMPTCOMMENTS")
		return v != "", true
	})
	s := Shell{
		Runner:                r,
		In:                    strings.NewReader(typed),
		Out:                   &ran,
		Err:                   &said,
		CommentsNeedTheOption: option,
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatalf("run %q: %v", typed, err)
	}
	return ran.String()
}

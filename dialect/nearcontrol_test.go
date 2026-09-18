// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A control character in the script text a diagnostic quotes back (#3563).
//
// One preset quotes a script's own line at all, and it is the one that has to
// decide what to do with a byte the terminal would obey rather than print: a
// tab moves the caret and an escape character starts a sequence, so writing
// the script's bytes through lets that script repaint the screen it is being
// complained about on.
//
// Measured 2026-09-17 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin LC_ALL=C zsh
// s.sh` with stdin from /dev/null, one byte at a time.
func TestAControlCharacterInTheQuotedScriptText(t *testing.T) {
	for _, tc := range []struct {
		name string
		b    byte
		want string
	}{
		{"a tab has a letter", 0x09, `\t`},
		{"the first control character", 0x01, "^A"},
		{"a bell", 0x07, "^G"},
		{"an escape", 0x1b, "^["},
		{"the last of the C0 range", 0x1f, "^_"},
		{"delete", 0x7f, "^?"},
		// The high half is the same rule behind a meta prefix, and it stops
		// at 0xa0 — which is the row that says the boundary is not "the top
		// bit set".
		{"the meta form of the first", 0x80, `\M-^@`},
		{"the meta form of a tab", 0x89, `\M-\t`},
		// A newline has a letter too, and this is where it can be asked
		// without ending the line the message quotes.
		{"the meta form of a newline", 0x8a, `\M-\n`},
		{"the meta form of the last", 0x9f, `\M-^_`},
		{"the first byte written through", 0xa0, "\xa0"},
		{"and the last one", 0xff, "\xff"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A body that will not parse, so the second message quotes the
			// line the byte is on.
			src := "printf 'start\\n'\nv=$(echo" + string([]byte{tc.b}) + "a; for)\n"
			_, errs, _ := splitRunWithText(t, presets["zsh"], src)
			want := "`v=$(echo" + tc.want + "a; for)'"
			if !strings.Contains(errs, want) {
				t.Errorf("wrote %q, want it to hold %q", errs, want)
			}
		})
	}
}

// The cut comes first and the rendering after it, so the limit counts the
// bytes of the script rather than of the rendering: an escape two characters
// wide does not push a character off the end.
func TestTheCutCountsTheScriptsBytesAndNotTheRenderings(t *testing.T) {
	src := "printf 'start\\n'\nv=$(echo\taaaaaaaaaaaaaaaaaaaaaa; for)\n"
	_, errs, _ := splitRunWithText(t, presets["zsh"], src)
	if !strings.Contains(errs, "`v=$(echo\\taaaaaaaaaaa...'") {
		t.Errorf("wrote %q, want twenty bytes of the script and then the ellipsis", errs)
	}
}

// splitRunWithText is splitRun with the program's text handed to the runner,
// which is what a front end does before each line and what a message quoting a
// script line reads from.
func splitRunWithText(t *testing.T, p dialecttest.Preset, src string) (out, errs string, status int) {
	t.Helper()
	f := p.Parse(t, src)
	var o, e strings.Builder
	r := p.Runner(dialecttest.Base{Stdout: &o, Stderr: &e})
	r.SetProgramText(src)
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return o.String(), e.String(), st
}

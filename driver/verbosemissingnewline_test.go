// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// The echo of a line the input did not end (#3130).
//
// `-v` writes the shell's input back **as it was read**, so the last line of
// a `-c` string — or of a file with no final newline — is a line with no
// newline on it. One column adds one anyway, which puts the echo and the
// first byte the script writes on separate lines; the rest leave them
// together. The axis is moved both ways over both routes, and the control is
// an input that really does end its last line: nothing there can tell the two
// answers apart.
func TestTheEchoOfALineTheInputDidNotEnd(t *testing.T) {
	const src = "echo one\necho two"
	for _, tc := range []struct {
		name  string
		adds  interp.Answer
		file  bool
		ended bool
		want  string
	}{
		{name: "a command string, added", adds: interp.Yes, want: "echo one\none\necho two\ntwo\n"},
		{name: "a command string, as read", adds: interp.No, want: "echo one\none\necho twotwo\n"},
		{name: "a file, added", adds: interp.Yes, file: true, want: "echo one\none\necho two\ntwo\n"},
		{name: "a file, as read", adds: interp.No, file: true, want: "echo one\none\necho twotwo\n"},
		// The control: with the newline in the input there is nothing for
		// either answer to add, so the axis reaches nothing.
		{
			name: "a file that ends its last line, added", adds: interp.Yes,
			file: true, ended: true, want: "echo one\none\necho two\ntwo\n",
		},
		{
			name: "a file that ends its last line, as read", adds: interp.No,
			file: true, ended: true, want: "echo one\none\necho two\ntwo\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// One stream for both, because what is asserted is where the
			// echo falls against the output it runs into — which is the
			// whole of the difference.
			var out strings.Builder
			sh := shell()
			sh.Semantics.VerboseEchoAddsAMissingNewline = tc.adds
			sh.Stdout, sh.Stderr = &out, &out
			text := src
			if tc.ended {
				text += "\n"
			}
			argv := []string{"testsh", "-v", "-c", text}
			if tc.file {
				argv = []string{"testsh", "-v", writeScript(t, text)}
			}
			if code := driver.MainArgs(sh, argv); code != 0 {
				t.Fatalf("status %d (%q)", code, out.String())
			}
			if got := out.String(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This shell's trace prefix is a location, and it is the same location a
// diagnostic from that line gets: the function and the offset within it, a
// sourced file by its own name, and `(eval)` for evaluated text.
//
// Measured on zsh 5.9.2, 2026-09-12, `env -i PATH=/usr/bin:/bin` with a
// scratch HOME and ZDOTDIR, `-f` over a script file. The prefix had been a
// second, thinner copy of the rule, which wrote `+q:0>` for every line of
// every function and never named a sourced file at all (#2134).
//
// The shell's own name is left out of the rows: which file the *script* is is
// the harness's, and the three answers below are the ones the copy got wrong.
func TestTheTracePrefixIsTheLocation(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"x_inc": ":\n",
		"g_inc": "g() {\n  :\n}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name, src string
		want      []string
		notWant   []string
	}{
		{
			name: "a line inside a function",
			src:  ":\n:\nq() {\n  :\n  :\n}\nset -x\nq\nset +x\n",
			// The offset from the line the function was written on, which is
			// not always nought — and was always nought before.
			want:    []string{"+q:1> :\n", "+q:2> :\n"},
			notWant: []string{"+q:0>"},
		},
		{
			name:    "a file sourced from a function",
			src:     "q() {\n  set -x\n  source ./x_inc\n  set +x\n}\nq\n",
			want:    []string{"+q:2> source ./x_inc\n", "+./x_inc:1> :\n", "+q:3> set +x\n"},
			notWant: []string{"+q:1> :\n"},
		},
		{
			name:    "a file sourced at the top level",
			src:     "set -x\nsource ./x_inc\nset +x\n",
			want:    []string{"+./x_inc:1> :\n"},
			notWant: []string{"+zsh:1> :\n"},
		},
		{
			name: "evaluated text",
			src:  "set -x\neval '\n:'\nset +x\n",
			want: []string{"+(eval):2> :\n"},
		},
		{
			name: "a function read from a sourced file",
			src:  "source ./g_inc\nset -x\ng\nset +x\n",
			want: []string{"+g:1> :\n"},
		},
		{
			// The nought is written where a diagnostic leaves it out: a body
			// on the same line as its `f() {` is offset nought, and the trace
			// says `+f:0>` where the diagnostic says `f: ` with no number.
			name: "a body on the definition line",
			src:  "f() { echo in; }\nset -x\nf\n",
			want: []string{"+f:0> echo in\n"},
		},
	} {
		out, _ := runZsh(t, dir, tc.src)
		for _, w := range tc.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: %q\n does not carry %q", tc.name, out, w)
			}
		}
		for _, n := range tc.notWant {
			if strings.Contains(out, n) {
				t.Errorf("%s: %q\n still carries %q", tc.name, out, n)
			}
		}
	}
}

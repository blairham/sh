// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// `zle -T tc f` hands the editor's terminal operations to f. Measured
// 2026-10-02 on zsh 5.9.2 through a pseudo-terminal: f is called with the
// termcap code and, for a counted one, the count, and what it leaves in REPLY
// is written; a name with no function behind it writes nothing; and with no
// transformation the sequences go out as they are.
func TestTheTermcapTransformationCallsTheFunction(t *testing.T) {
	for _, c := range []struct {
		name, setup, code, arg, want string
		installed                    bool
	}{
		{"none installed", ``, "cd", "", "", false},
		{"a code", `f() { REPLY="<$1${2:+:$2}>"; return 3; }; zle -T tc f`, "cd", "", "<cd>", true},
		{"a counted code", `f() { REPLY="<$1${2:+:$2}>"; }; zle -T tc f`, "LE", "18", "<LE:18>", true},
		{"no such function", `zle -T tc nosuchfn`, "le", "", "", true},
		{"taken away", `f() { REPLY=x; }; zle -T tc f; zle -Tr tc`, "cd", "", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			var buf strings.Builder
			r := preset.Runner(dialecttest.Base{Dir: t.TempDir(), Stdout: &buf, Stderr: &buf})
			if _, err := r.Run(context.Background(), preset.Parse(t, "zmodload zsh/zle\n"+c.setup+"\n(exit 7)")); err != nil {
				t.Fatal(err)
			}
			got, installed := zsh.TransformTermcap(r, context.Background(), c.code, c.arg)
			if got != c.want || installed != c.installed {
				t.Errorf("got %q, %v; want %q, %v (output %q)", got, installed, c.want, c.installed, buf.String())
			}
			if st := r.ExitStatus(); st != 7 {
				t.Errorf("$? = %d after the call, want the 7 it was before", st)
			}
		})
	}
}

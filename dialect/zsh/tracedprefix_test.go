// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **A command's prefix, traced a word at a time** (#5546). Measured
// 2026-10-03 on zsh 5.9.2 under `-f -c`, both streams on one pipe. A failed
// or refused prefix leaves its line so far, written after the complaint and
// only if the shell stops. A substitution in a value writes a copy of the
// line so far ahead of its own. See interp/xtraceprefixasitgoes.go.
func TestATracedPrefixIsWrittenAsItGoes(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`readonly r; set -x; a=1 r=2 true; echo st=$?`, "zsh:1: read-only variable: r\n+zsh:1> a=1 r=2 "},
		{`readonly r; set -x; a=1 r=2 /bin/echo hi; echo st=$?`, "zsh:1: read-only variable: r\n+zsh:1> echo 'st=1'\nst=1\n"},
		{`readonly r; set -x; a=1 r=2 command true; echo st=$?`, "zsh:1: read-only variable: r\n+zsh:1> echo 'st=1'\nst=1\n"},
		{`readonly r; set -x; a=1 r=$(echo s >&2) c=3 true`, "+zsh:1> a=1 r=+zsh:1> echo s\ns\nzsh:1: read-only variable: r\n+zsh:1> a=1 r='' "},
		{`readonly r; set -x; a=1 r=2 c=3 true >/dev/null`, "zsh:1: read-only variable: r\n+zsh:1> a=1 r=2 "},
		{`set -x; a=1 b=${x?boom} c=$(echo s >&2) true`, "zsh:1: x: boom\n+zsh:1> a=1 b="},
		{`set -x; a=1 b=${x?boom} /bin/echo hi; echo st=$?`, "zsh:1: x: boom\n+zsh:1> echo 'st=1'\nst=1\n"},
		{`set -x; a=1 b=$((1/0)) :; echo st=$?`, "zsh:1: division by zero\n+zsh:1> a=1 b="},
		{`set -x; a=1 b=$((1/0)) /bin/echo hi; echo st=$?`, "zsh:1: division by zero\n+zsh:1> echo 'st=1'\nst=1\n"},
		{`set -x; a=1 b=$((1/0)) c=3 true 2>/dev/null`, "+zsh:1> a=1 b="},
		{`set -x; a=$(echo s >&2) b=$(echo t >&2) true`, "+zsh:1> a=+zsh:1> echo s\ns\n+zsh:1> a='' b=+zsh:1> echo t\nt\n+zsh:1> a='' b='' +zsh:1> true\n"},
		{`set -x; x=$(echo a) y=$(echo b)`, "+zsh:1> x=+zsh:1> echo a\n+zsh:1> x=a y=+zsh:1> echo b\n+zsh:1> x=a y=b \n"},
		{`b=0; set -x; a=1 b=$((1/0)) c=3; echo "[$a][$b][$c]"`, "+zsh:1> a=1 b=zsh:1: division by zero\n\n"},
		{`set -x; a=1 b=2 true`, "+zsh:1> a=1 b=2 +zsh:1> true\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
		if err != nil {
			t.Fatal(err)
		}
		if out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}

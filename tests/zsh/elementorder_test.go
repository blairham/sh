// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An element write expands its value before its subscript — an element, a
// table's key, an append, a flag group, a span — traced and not, and with
// `globassign` on the traced value is expanded once. Measured 2026-10-15 on
// zsh 5.9.2 from a file (`-f`), this script, byte for byte apart from the
// script's name in the trace and the complaints. Every element row wrote SUB
// before VAL here untraced, the flag group's operand ran twice, and the
// traced `globassign` write ran its value twice (#6604, #6605).
func TestAnElementWriteExpandsItsValueFirst(t *testing.T) {
	t.Parallel()
	src := "s() { echo SUB >&2; echo \"$1\"; }\n" +
		"v() { echo VAL >&2; }\n" +
		"a=(1 2 3); typeset -A m\n" +
		"a[$(s 1)]=$(v)x\n" +
		"m[$(s k)]=$(v)z\n" +
		"a[$(s 2)]+=$(v)w\n" +
		"a[(r)$(s 3)]=$(v)Q\n" +
		"a[$(s 2),$(s 3)]=$(v)R\n" +
		"i=1; a[$((i++))]=$((i*10)); echo \"${a[*]} ${m[k]} $i\"\n" +
		"(a[$(s 1)]=$(v)$((1/0)))\n" +
		"(a[$(s 1/0)]=$(v)y)\n" +
		"setopt globassign\n" +
		"set -x\n" +
		"a[$(s 1)]=$(v)y\n" +
		"m[$(s k)]=$(v)q\n" +
		"set +x\n" +
		"echo \"${a[*]} ${m[k]}\"\n"
	want := "VAL\n" +
		"SUB\n" +
		"VAL\n" +
		"SUB\n" +
		"VAL\n" +
		"SUB\n" +
		"VAL\n" +
		"SUB\n" +
		"VAL\n" +
		"SUB\n" +
		"SUB\n" +
		"10 R z 2\n" +
		"VAL\n" +
		"ERR\n" +
		"VAL\n" +
		"SUB\n" +
		"ERR\n" +
		"+> a[$(s 1)]=+> v\n" +
		"+> echo VAL\n" +
		"VAL\n" +
		"+> a[$(s 1)]=y +> s 1\n" +
		"+> echo SUB\n" +
		"SUB\n" +
		"+> echo 1\n" +
		"+> a[$(s 1)]=y \n" +
		"+> m[$(s k)]=+> v\n" +
		"+> echo VAL\n" +
		"VAL\n" +
		"+> m[$(s k)]=q +> s k\n" +
		"+> echo SUB\n" +
		"SUB\n" +
		"+> echo k\n" +
		"+> m[$(s k)]=q \n" +
		"+> set +x\n" +
		"y R q\n"
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if got := elementOrderNormalized(out); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// elementOrderNormalized takes out what names the script rather than what it
// did: the location in front of a complaint and in a trace prefix.
func elementOrderNormalized(out string) string {
	var b strings.Builder
	for _, line := range strings.SplitAfter(out, "\n") {
		if strings.Contains(line, "division by") || strings.Contains(line, "divide by") {
			line = "ERR\n"
		}
		b.WriteString(elementOrderTracePrefix.ReplaceAllString(line, "+> "))
	}
	return b.String()
}

var elementOrderTracePrefix = regexp.MustCompile(`\+[^ >+]*:\d+> `)

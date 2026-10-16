// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An element write expands its value, then its subscript's words, then
// evaluates the subscript — over an indexed array, a table and an append,
// traced and not. Measured 2026-10-15 on bash 5.3.20 from a file, this
// script, byte for byte apart from the location in front of each complaint.
// Every element row wrote SUB before VAL here untraced, and the table's key
// ran its substitution twice (#6604, #6605).
func TestAnElementWriteExpandsItsValueFirst(t *testing.T) {
	src := "s() { echo SUB >&2; echo \"$1\"; }\n" +
		"v() { echo VAL >&2; }\n" +
		"a=(1 2); declare -A m\n" +
		"a[$(s 1)]=$(v)x\n" +
		"m[$(s k)]=$(v)z\n" +
		"a[$(s 0)]+=$(v)w\n" +
		"i=1; a[$((i++))]=$((i*10)); echo \"${a[*]} ${m[k]} $i\"\n" +
		"a[$(s 1)]=$(v)$((1/0))\n" +
		"a[$(s 1/0)]=$(v)y\n" +
		"set -x\n" +
		"a[$(s 0)]=$(v)y\n" +
		"m[$(s k)]=$(v)q\n" +
		"set +x\n" +
		"echo \"${a[*]} ${m[k]}\"\n"
	want := "VAL\n" +
		"SUB\n" +
		"VAL\n" +
		"SUB\n" +
		"VAL\n" +
		"SUB\n" +
		"1w 10 z 2\n" +
		"VAL\n" +
		"ERR\n" +
		"VAL\n" +
		"SUB\n" +
		"ERR\n" +
		"++ v\n" +
		"++ echo VAL\n" +
		"VAL\n" +
		"+ a[$(s 0)]=y\n" +
		"++ s 0\n" +
		"++ echo SUB\n" +
		"SUB\n" +
		"++ echo 0\n" +
		"++ v\n" +
		"++ echo VAL\n" +
		"VAL\n" +
		"+ m[$(s k)]=q\n" +
		"++ s k\n" +
		"++ echo SUB\n" +
		"SUB\n" +
		"++ echo k\n" +
		"+ set +x\n" +
		"y 10 q\n"
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

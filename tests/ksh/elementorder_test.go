// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// An element write expands its subscript's words, then its value, then
// evaluates the subscript — over an indexed array, a table, an append and a
// chain, traced and not. Measured 2026-10-15 on ksh93u+ 2012-08-01 from a
// file, this script, byte for byte apart from the location in front of each
// complaint. The arithmetic was evaluated before the value here, so
// `a[$((i++))]=$((i*10))` and `d[i++][i++]=$((i*10))` stored the wrong
// numbers and a subscript that would not evaluate never reached the value;
// and traced, the value went first (#6604).
func TestAnElementWriteExpandsItsSubscriptFirst(t *testing.T) {
	src := "s() { echo SUB >&2; echo \"$1\"; }\n" +
		"v() { echo VAL >&2; }\n" +
		"a=(1 2); typeset -A m\n" +
		"a[$(s 1)]=$(v)x\n" +
		"m[$(s k)]=$(v)z\n" +
		"a[$(s 0)]+=$(v)w\n" +
		"i=1; a[$((i++))]=$((i*10)); echo \"${a[*]} ${m[k]} $i\"\n" +
		"c[$(s 1)][$(s 2)]=$(v)n\n" +
		"i=0; d[i++][i++]=$((i*10))\n" +
		"typeset -p c d\n" +
		"(a[$(s 1)]=$(v)$((1/0)))\n" +
		"(a[$(s 1/0)]=$(v)y)\n" +
		"(c[$(s 1/0)][2]=$(v)y)\n" +
		"set -x\n" +
		"a[$(s 0)]=$(v)y\n" +
		"m[$(s k)]=$(v)q\n" +
		"set +x\n" +
		"echo \"${a[*]} ${m[k]}\"\n"
	want := "SUB\n" +
		"VAL\n" +
		"SUB\n" +
		"VAL\n" +
		"SUB\n" +
		"VAL\n" +
		"1w 20 z 2\n" +
		"SUB\n" +
		"SUB\n" +
		"VAL\n" +
		"typeset -a c=([1]=([2]=n) )\n" +
		"typeset -a d=(([1]=0) )\n" +
		"SUB\n" +
		"VAL\n" +
		"ERR\n" +
		"SUB\n" +
		"VAL\n" +
		"ERR\n" +
		"SUB\n" +
		"VAL\n" +
		"ERR\n" +
		"+ s 0\n" +
		"+ echo SUB\n" +
		"+ 1>& 2\n" +
		"SUB\n" +
		"+ echo 0\n" +
		"+ v\n" +
		"+ echo VAL\n" +
		"+ 1>& 2\n" +
		"VAL\n" +
		"+ a[0]=y\n" +
		"+ s k\n" +
		"+ echo SUB\n" +
		"+ 1>& 2\n" +
		"SUB\n" +
		"+ echo k\n" +
		"+ v\n" +
		"+ echo VAL\n" +
		"+ 1>& 2\n" +
		"VAL\n" +
		"+ m[k]=q\n" +
		"y 20 q\n"
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

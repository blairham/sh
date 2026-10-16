// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A frozen name is refused at the element's store: after the value, then
// the subscript's words and its arithmetic, so a subscript that will not
// evaluate is what is reported. A scalar is refused once its value is
// expanded, and a value that will not expand is reported instead. Measured
// 2026-10-15 on bash 5.3.20 from a file, this script, normalized as below,
// traced and not. We refused every row before expanding anything (#6674).
func TestAFrozenElementIsRefusedAtItsStore(t *testing.T) {
	checkFrozen(t, frozenElementScript,
		"VAL RO ERR VAL SUB RO VAL SUB RO VAL SUB RO SUB RO VAL SUB ERR ERR VAL SUB RO done")
}

// A frozen whole literal is refused before any of its words is expanded,
// traced or not. Measured the same way.
func TestAFrozenLiteralIsRefusedBeforeItsWords(t *testing.T) {
	checkFrozen(t, frozenLiteralScript, "RO RO RO RO RO done")
}

// A list through a subscript is refused as a construct before the
// subscript is expanded, so quoting it back runs nothing — and before a
// freeze is consulted. Measured 2026-10-15 on bash 5.3.20: no SUB on either
// row, and the list's sentence on the frozen one too. We ran the subscript's
// substitution to quote it, and refused the frozen name instead (#6674).
func TestAListThroughASubscriptRunsNothingBeforeItIsRefused(t *testing.T) {
	checkFrozen(t, "s() { echo SUB >&2; echo \"$1\"; }\n"+
		"a=(1 2); (a[$(s 1)]=(p q))\n"+
		"readonly b=(1 2); (b[$(s 1)]=(p q))\n"+
		"echo done\n", "LIST LIST done")
}

const frozenElementScript = "s() { echo SUB >&2; echo \"$1\"; }\n" +
	"v() { echo VAL >&2; }\n" +
	"readonly a=(1 2)\n" +
	"typeset -A m; m[k]=1; readonly m\n" +
	"readonly x=1\n" +
	"(x=$(v)y)\n" +
	"(x=$((1/0)))\n" +
	"(a[$(s 1)]=$(v)x)\n" +
	"(a[$(s 1)]+=$(v)x)\n" +
	"(m[$(s k)]=$(v)x)\n" +
	"(a[$(s 1)]=x)\n" +
	"(a[$(s 1/0)]=$(v)y)\n" +
	"(a[$(s 1)]=$((1/0)))\n" +
	"(set -x; a[$(s 1)]=$(v)x)\n" +
	"echo done\n"

const frozenLiteralScript = "s() { echo SUB >&2; echo \"$1\"; }\n" +
	"v() { echo VAL >&2; }\n" +
	"readonly a=(1 2)\n" +
	"typeset -A m; m[k]=1; readonly m\n" +
	"(a=($(v)))\n" +
	"(a+=($(v)))\n" +
	"(a=(p $((1/0))))\n" +
	"(m=([$(s k)]=$(v)))\n" +
	"(set -x; a=($(v)))\n" +
	"echo done\n"

// frozenNormalized is a run's output with each refusal reduced to a word
// and the trace and the locations left out: the order the side effects
// happened in and how each write ended, which is the whole of the question.
func frozenNormalized(out string) string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "readonly variable"), strings.Contains(line, "is read only"),
			strings.Contains(line, "read-only variable"):
			keep = append(keep, "RO")
		case strings.Contains(line, "division by"), strings.Contains(line, "divide by"):
			keep = append(keep, "ERR")
		case strings.Contains(line, "cannot assign list"):
			keep = append(keep, "LIST")
		case strings.Contains(line, "attempt to set slice"):
			keep = append(keep, "SLICE")
		case line == "", strings.HasPrefix(line, "+"):
		default:
			keep = append(keep, line)
		}
	}
	return strings.Join(keep, " ")
}

func checkFrozen(t *testing.T, src, want string) {
	t.Helper()
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if got := frozenNormalized(out); got != want {
		t.Errorf("got  %s\nwant %s\nfrom %q", got, want, out)
	}
}

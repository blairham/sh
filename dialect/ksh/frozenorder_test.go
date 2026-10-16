// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A frozen name is refused at the element's store: after the subscript's
// words, then the value, then the subscript's arithmetic, so a subscript
// that will not evaluate is what is reported. A scalar is refused once its
// value is expanded, and a value that will not expand is reported instead.
// Measured 2026-10-15 on ksh93u+ 2012-08-01 from a file, this script,
// normalized as below, traced and not. We refused every row before
// expanding anything (#6674).
func TestAFrozenElementIsRefusedAtItsStore(t *testing.T) {
	checkFrozen(t, frozenElementScript,
		"VAL RO ERR SUB VAL RO SUB VAL RO SUB VAL RO SUB RO SUB VAL ERR SUB ERR SUB VAL RO done")
}

// A frozen whole literal is refused before any of its words is expanded,
// traced or not — the trace, which prints what the words came to, used to
// expand them first. Measured the same way.
func TestAFrozenLiteralIsRefusedBeforeItsWords(t *testing.T) {
	checkFrozen(t, frozenLiteralScript, "RO RO RO RO RO done")
}

// A list through a subscript reads the whole subscript first — its words
// and its arithmetic — and builds the list after, a table's key included.
// Measured 2026-10-15 on ksh93u+ 2012-08-01 from a file. We built the list
// first (#6675).
func TestAnElementLiteralReadsItsSubscriptFirst(t *testing.T) {
	checkFrozen(t, "s() { echo SUB >&2; echo \"$1\"; }\n"+
		"v() { echo VAL >&2; }\n"+
		"a=(1 2); a[$(s 1)]=(p $(v)); typeset -p a\n"+
		"a=(1 2); (a[$(s 1/0)]=(p $(v)))\n"+
		"a=(1 2); (a[$(s 1)]=(p $((1/0))))\n"+
		"a=(1 2); i=1; a[i++]=(p $((i*10))); typeset -p a\n"+
		"a=(1 2); a[$(s 1)]+=(p $(v)); typeset -p a\n"+
		"typeset -A h; (h[$(s k)]=(p $(v)); typeset -p h)\n"+
		"echo done\n",
		"SUB VAL typeset -a a=(1 (p) ) SUB ERR SUB ERR typeset -a a=(1 (p 20) ) "+
			"SUB VAL typeset -a a=(1 (p) ) SUB VAL typeset -A h=([k]=(p) ) done")
}

// The whole-array spelling over a frozen name is refused for its subscript,
// as it is over any name, before the value is expanded — a table's too.
// Nothing is stored. Measured 2026-10-16 on ksh93u+ 2012-08-01 from a file,
// this script, byte for byte before normalizing. We refused the freeze
// instead (#6761).
func TestAFrozenWholeArraySubscriptIsRefusedForItsSubscript(t *testing.T) {
	checkFrozen(t, frozenWholeArrayScript,
		"INVALID INVALID INVALID INVALID INVALID done")
}

// Each row reads its name back from an exit trap, since a refusal ends the
// subshell and a store taken before it would otherwise go unseen.
const frozenWholeArrayScript = "v() { echo VAL >&2; }\n" +
	"readonly a=(1 2)\n" +
	"typeset -A m; m[k]=1; readonly m\n" +
	"(trap 'typeset -p a' EXIT; a[@]=$(v)x)\n" +
	"(trap 'typeset -p a' EXIT; a[*]+=x)\n" +
	"(trap 'typeset -p m' EXIT; m[@]=$(v)x)\n" +
	"(trap 'typeset -p m' EXIT; m[@]+=$(v)x)\n" +
	"(set -x; a[@]=$(v)x)\n" +
	"echo done\n"

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
		case strings.Contains(line, "bad array subscript"):
			keep = append(keep, "BAD")
		case strings.Contains(line, "invalid subscript"):
			keep = append(keep, "INVALID")
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

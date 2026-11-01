// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A frozen name is refused once the value is expanded, and an element
// write's subscript is never reached: not its words, and not an expression
// that will not evaluate. A value that will not expand is reported instead
// of the refusal. Measured 2026-10-15 on zsh 5.9.2 from a file, this script,
// normalized as below, traced and not. We refused every row before
// expanding anything (#6674).
func TestAFrozenElementIsRefusedBeforeItsSubscript(t *testing.T) {
	t.Parallel()
	checkFrozen(t, frozenElementScript,
		"VAL RO ERR VAL RO VAL RO VAL RO RO VAL RO ERR VAL RO done")
}

// A frozen whole literal expands every word before it is refused, a table's
// keys included, and a word that will not expand is reported instead.
// Measured the same way.
func TestAFrozenLiteralIsExpandedBeforeItIsRefused(t *testing.T) {
	t.Parallel()
	checkFrozen(t, frozenLiteralScript, "VAL RO VAL RO ERR SUB VAL RO VAL RO done")
}

// A list through a subscript is built before the subscript is read, and the
// subscript's arithmetic comes after the list too; a table's slice is
// refused once the list is built, before the subscript. Measured 2026-10-15
// on zsh 5.9.2 from a file. We read the subscript first (#6675).
func TestAnElementLiteralBuildsItsListFirst(t *testing.T) {
	t.Parallel()
	checkFrozen(t, "s() { echo SUB >&2; echo \"$1\"; }\n"+
		"v() { echo VAL >&2; }\n"+
		"a=(1 2); a[$(s 1)]=(p $(v)); typeset -p a\n"+
		"a=(1 2); (a[$(s 1/0)]=(p $(v)))\n"+
		"a=(1 2); (a[$(s 1)]=(p $((1/0))))\n"+
		"a=(1 2); i=1; a[i++]=(p $((i*10))); typeset -p a\n"+
		"a=(1 2); a[$(s 1)]+=(p $(v)); typeset -p a\n"+
		"typeset -A h; (h[$(s k)]=(p $(v)); typeset -p h)\n"+
		"readonly r=(1 2); (r[$(s 1)]=(p $(v)))\n"+
		"echo done\n",
		"VAL SUB typeset -a a=( p 2 ) VAL SUB ERR ERR typeset -a a=( p 10 2 ) "+
			"VAL SUB typeset -a a=( 1 p 2 ) VAL SLICE VAL SUB RO done")
}

// A flag group selecting over a table is refused once its operand has been
// expanded, so a substitution in the operand runs and one that fails is
// reported instead. Measured 2026-10-15 on zsh 5.9.2 from a file. We
// refused on the letter with the operand never expanded (#6674).
func TestAFlaggedTableWriteExpandsItsOperandBeforeTheRefusal(t *testing.T) {
	t.Parallel()
	checkFrozen(t, "s() { echo SUB >&2; echo \"$1\"; }\n"+
		"v() { echo VAL >&2; }\n"+
		"typeset -A t; t=(k 1)\n"+
		"(t[(r)$(s 1)]=$(v)Q)\n"+
		"(t[(r)$((1/0))]=Q)\n"+
		"(t[(e)$(s k)]=$(v)Q; typeset -p t)\n"+
		"echo done\n",
		"VAL SUB SLICE ERR VAL SUB typeset -A t=( [k]=Q ) done")
}

// The whole-array spelling over a frozen name is refused for the freeze
// once the value is expanded, an array's and a table's alike, and nothing is
// stored. Measured 2026-10-16 on zsh 5.9.2 from a file, this script, byte
// for byte before normalizing — already the answer before #6761, which moved
// the other two columns; pinned so that change cannot move this one.
func TestAFrozenWholeArraySubscriptIsRefusedForTheFreeze(t *testing.T) {
	checkFrozen(t, frozenWholeArrayScript,
		"VAL RO RO VAL RO VAL RO VAL RO done")
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

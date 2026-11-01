// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `set -a` reaching a name a loop, `getopts` or a `{fd}` redirection fills,
// both answers of Semantics.AllexportMarksALoopOrGetoptsName. Each row stores
// into a name nothing has exported and reads the child's environment back;
// `x=9` is the control, exported under either answer.
func TestAllexportMarksALoopOrGetoptsName(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, store, as string }{
		{"a for loop", `for w in 4; do :; done`, "w"},
		{"a select loop", `select w in a; do break; done <<< 1 2>/dev/null`, "w"},
		{"the getopts name", `getopts x w -x`, "w"},
		{"a descriptor's name", `exec {w}>/dev/null`, "w"},
	} {
		for _, answer := range []Answer{Yes, No} {
			t.Run(tc.name+"/"+answer.String(), func(t *testing.T) {
				sem := permissive()
				sem.AllexportMarksALoopOrGetoptsName = answer
				out, st := run(t, `set -a; x=9; `+tc.store+`; set +a; env | grep -E '^(x|`+tc.as+`)='`, withSem(sem))
				if st != 0 || !strings.Contains(out, "x=9\n") {
					t.Fatalf("out=%q st=%d: the control did not export", out, st)
				}
				if got := strings.Contains(out, tc.as+"="); got != (answer == Yes) {
					t.Errorf("answer %v: out=%q, exported=%v", answer, out, got)
				}
			})
		}
	}
}

// OPTARG and OPTIND as `getopts` writes them, both answers of
// Semantics.AllexportMarksGetoptsOwnParameters — and the name operand beside
// them answering the other axis, so a mark that reached the two through the
// name's question (or the reverse) fails here.
func TestAllexportMarksGetoptsOwnParameters(t *testing.T) {
	t.Parallel()
	for _, own := range []Answer{Yes, No} {
		for _, name := range []Answer{Yes, No} {
			t.Run("own="+own.String()+"/name="+name.String(), func(t *testing.T) {
				sem := permissive()
				sem.AllexportMarksGetoptsOwnParameters = own
				sem.AllexportMarksALoopOrGetoptsName = name
				out, st := run(t, `set -a; getopts x: o -x6; set +a; env | grep -E '^(o|OPTARG|OPTIND)=' | sort`, withSem(sem))
				want := ""
				if own == Yes {
					want += "OPTARG=6\nOPTIND=2\n"
				}
				if name == Yes {
					want += "o=x\n"
				}
				if st != 0 || out != want {
					t.Errorf("out=%q st=%d, want %q", out, st, want)
				}
			})
		}
	}
}

// A builtin's output operand is marked by every shell, so it asks nothing:
// under CoreSemantics, which answers neither axis above, `read r` exports its
// name and no refusal is written. (`printf -v` and a bare `read` reach axes of
// their own there, so the operand is shown through the one builtin that does
// not.)
func TestAnOperandUnderAllexportAsksNothing(t *testing.T) {
	t.Parallel()
	out, st := run(t, `set -a; read r <<< 5; set +a; env | grep -E '^r='`, withSem(CoreSemantics()))
	if st != 0 || out != "r=5\n" {
		t.Errorf("out=%q st=%d, want r exported and nothing asked", out, st)
	}
}

// And nothing is asked without `set -a`: a loop and `getopts` under
// CoreSemantics run silently and export nothing.
func TestAFilledNameWithoutAllexportAsksNothing(t *testing.T) {
	t.Parallel()
	out, st := run(
		t,
		`for f in 1; do :; done; getopts x: o -x6; echo $f $o $OPTARG; env | grep -E '^(f|o|OPTARG|OPTIND)='`,
		withSem(CoreSemantics()),
	)
	if st != 1 || out != "1 x 6\n" {
		t.Errorf("out=%q st=%d, want the values, nothing exported and no refusal", out, st)
	}
}

// A store a freeze refused is not marked: `printf -v` over a readonly name
// leaves it without the export attribute, as bash 5.3.20 does — measured
// 2026-10-15, `declare -r p="1"` with no `x`. The builtin refuses before it
// reaches the operand store, which is what keeps the mark away.
func TestAFrozenOperandIsNotMarkedByAllexport(t *testing.T) {
	t.Parallel()
	out, _ := run(
		t,
		`readonly p=1; set -a; printf -v p %s 3 2>/dev/null; set +a; env | grep -E '^p='`,
		withSem(permissive()),
	)
	if out != "" {
		t.Errorf("out=%q, want the frozen name left unexported", out)
	}
}

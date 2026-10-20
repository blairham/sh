// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A store under a table's empty key is warned about here, and only while the
// shell is tracing. Measured 2026-10-15 against ksh93u+ 2012-08-01 from a
// script file:
//
//	set -x
//	typeset -A m; m[""]=e
//	e=; m[$e]=f
//
// writes `warning: adding empty subscript` in front of each of the two
// assignment lines, and the same file without `set -x` writes no warning at
// all. See interp.Diagnostics.EmptyTableKeyTraced (#6609).
func TestAnEmptyTableKeyIsWarnedAboutUnderTracing(t *testing.T) {
	const warn = "warning: adding empty subscript"
	for _, c := range []struct {
		name, src string
		// warnings is how many times the warning is written.
		warnings int
		// before is the trace line the last warning must sit directly in
		// front of, where there is one.
		before string
	}{
		{"an element assignment", "typeset -A m\nset -x\nm[\"\"]=e", 1, "+ m['']=e"},
		{"a key that expanded to nothing", "typeset -A m; e=\nset -x\nm[$e]=f", 1, "+ m['']=f"},
		{"the append spelling", "typeset -A m\nset -x\nm[\"\"]+=e", 1, "+ m['']+=e"},
		// Every store, and not only the one that adds the key.
		{"a second store under the same key", "typeset -A m\nset -x\nm[\"\"]=1; m[\"\"]=2", 2, "+ m['']=2"},
		{"a default assigned by an expansion", "typeset -A m\nset -x\n: ${m[\"\"]:=z}", 1, "+ : z"},
		{"an arithmetic store", "typeset -A m\nset -x\nx=$(( m[\"\"]=2 ))", 1, "+ x=2"},
		{"a literal", "typeset -A m\nset -x\nm=([\"\"]=1)", 1, "+ m['']=1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if st != 0 {
				t.Fatalf("status %d, output %q", st, out)
			}
			if n := strings.Count(out, warn); n != c.warnings {
				t.Fatalf("%d warnings, want %d, in %q", n, c.warnings, out)
			}
			lines := strings.Split(out, "\n")
			for i, l := range lines {
				if l == c.before {
					if i == 0 || !strings.HasSuffix(lines[i-1], ": "+warn) {
						t.Errorf("%q is not directly behind the warning in %q", c.before, out)
					}
					return
				}
			}
			t.Errorf("no line %q in %q", c.before, out)
		})
	}
}

// `read` says it as the builtin, located the way that builtin's own complaints
// are: measured, `<script>[N]: read: warning: adding empty subscript`.
func TestReadWarnsAboutAnEmptyTableKeyAsTheBuiltin(t *testing.T) {
	out, _ := runKsh(t, t.TempDir(), "typeset -A m\nset -x\nread 'm[]' </dev/null")
	if !strings.Contains(out, ": read: warning: adding empty subscript\n") {
		t.Errorf("got %q, want the warning spoken by read", out)
	}
}

// The two controls the measurement draws the line with: without tracing
// nothing is said, and an indexed array's empty subscript is element zero and
// not an empty key, so it is not warned about either.
func TestAnEmptyTableKeyIsNotWarnedAboutOtherwise(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"not tracing", "typeset -A m; m[\"\"]=e; m+=([\"\"]=f); : ${m[\"\"]:=z}; typeset -p m", "typeset -A m=(['']=f)\n"},
		{"an indexed array", "x=(); set -x; x[\"\"]=1; set +x; typeset -p x", "+ x[0]=1\ntypeset -a x=(1)\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, c.want)
			}
		})
	}
}

// A keyed literal traces as the element writes it performs, and each key is
// quoted the way the same key is in an element assignment's own line.
// Measured 2026-10-15 on ksh93u+ 2012-08-01: `a=(["a b"]=1 [x]="p q")` traces
// `a['a b']=1` and `a[x]='p q'`, where the keys used to be written raw (#6609).
func TestALiteralsTracedKeysAreQuoted(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "typeset -A a\nset -x\na=([\"a b\"]=1 [x]=\"p q\")")
	if want := "+ a['a b']=1\n+ a[x]='p q'\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}

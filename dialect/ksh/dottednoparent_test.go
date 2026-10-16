// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAStoreUnderAMissingParentIsRefused — a dotted name needs its parent:
// an assignment, a declaration and a prefix that stores are `no parent` and
// end the script, wherever they are written (#6628). ksh93u+ 2012-08-01
// `-c`, measured 2026-10-15.
func TestAStoreUnderAMissingParentIsRefused(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		st              int
	}{
		{"an assignment", `a.b=1; echo after`, "ksh: a.b=1: no parent\n", 1},
		{"the parent is the path above", `a=1; a.b.c=1; echo after`, "ksh: a.b.c=1: no parent\n", 1},
		{"an append", `a.b+=x; echo after`, "ksh: a.b+=x: no parent\n", 1},
		{"an element", `a.b[1]=x; echo after`, "ksh: a.b[1]=x: no parent\n", 1},
		{"an array literal", `a.b=(x y); echo after`, "ksh: a.b: no parent\n", 1},
		{"the value expands first", `a.b=$(echo v); echo after`, "ksh: a.b=v: no parent\n", 1},
		{"in a function, caught", `f() { a.b=1; echo in; }; f || echo caught; echo after`, "ksh: a.b=1: no parent\n", 1},
		{"in a subshell", `(a.b=1); echo "st=$?"`, "ksh: a.b=1: no parent\nst=1\n", 0},
		{"a declaration", `typeset a.b=1; echo after`, "ksh: a.b=1: no parent\n", 1},
		{"a declaration with the export letter", `typeset -x a.b=1; echo after`, "ksh: a.b=1: no parent\n", 1},
		{"a declaration with no value", `typeset a.b; echo after`, "ksh: typeset: a.b: no parent\n", 1},
		{"integer", `integer a.b; echo after`, "ksh: typeset: a.b: no parent\n", 1},
		// The letters that take no dotted name say so first.
		{"the export letter with no value", `typeset -x nosuch.x; echo after`, "ksh: typeset: nosuch.x: is not an identifier\n", 1},
		{"the reference letter", `typeset -n a.b=c; echo after`, "ksh: a.b=c: is not an identifier\n", 1},
		{"readonly", `readonly a.b; echo after`, "ksh: readonly: a.b: no parent\n", 1},
		{"unset goes on", `unset a.b; echo "st=$?"`, "ksh: unset: a.b: no parent\nst=1\n", 0},
		{"a special builtin's prefix", `a.b=1 :; echo after`, "ksh: a.b=1: no parent\n", 1},
		{"a POSIX function's prefix", `f() { echo in; }; a.b=1 f; echo after`, "ksh: a.b=1: no parent\n", 1},
		// The controls: a parent that is there, of whatever kind.
		{"a scalar parent", `a=1; a.b=2; echo "st=$? ${a.b}"`, "st=0 2\n", 0},
		{"a compound above", `typeset -C a; a.b.c=1; echo "st=$? ${a.b.c}"`, "st=0 1\n", 0},
		{"the shell's namespace", `.sh.x=1; echo "st=$?"`, "st=0\n", 0},
		{"a declared namespace", `namespace ns { x=1; }; .ns.y=2; echo "st=$? ${.ns.y}"`, "st=0 2\n", 0},
		{"a reference", `typeset -n e=c; e.x=9; echo "st=$?"`, "st=0\n", 0},
		{"through a reference, missing", `c=1; typeset -n e=c; e.x.y=9; echo after`, "ksh: e.x.y=9: no parent\n", 1},
		{"a compound's prefix", `typeset -C a; a.b=1 :; echo "st=$? ${a.b}"`, "st=0 1\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if out != c.want || st != c.st {
				t.Errorf("got %q at %d, want %q at %d", out, st, c.want, c.st)
			}
		})
	}
}

// TestADottedPrefixTheCommandIsOnlyShownIsNotAName — a regular builtin, a
// program and a `function`-form call take no dotted name in their prefix,
// parent or no parent: `is not an identifier`, the walk ends there, and the
// command kind decides what it costs, as a value that will not expand does
// (#6628). ksh93u+ 2012-08-01 `-c`, measured 2026-10-15.
func TestADottedPrefixTheCommandIsOnlyShownIsNotAName(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		st              int
	}{
		{"a regular builtin", `a.b=1 true || echo "caught $?"; echo after`, "ksh: a.b=1: is not an identifier\ncaught 1\nafter\n", 0},
		{"a program", `a.b=1 /usr/bin/true || echo "caught $?"; echo after`, "ksh: a.b=1: is not an identifier\ncaught 1\nafter\n", 0},
		{"a parent does not help", `typeset -C a; a.b=1 /usr/bin/true; echo "st=$?"`, "ksh: a.b=1: is not an identifier\nst=1\n", 0},
		{"the walk ends there", `a.b=1 x=$(echo SIDE >&2) /usr/bin/true; echo "st=$?"`, "ksh: a.b=1: is not an identifier\nst=1\n", 0},
		{"the entry in front runs", `x=$(echo SIDE >&2) a.b=1 /usr/bin/true; echo "st=$?"`, "SIDE\nksh: a.b=1: is not an identifier\nst=1\n", 0},
		{"a function-form call ends the script", `function g { echo in; }; a.b=1 g; echo after`, "ksh: a.b=1: is not an identifier\n", 1},
		{"the namespace", `.sh=1 true; echo "st=$?"`, "ksh: .sh=1: is not an identifier\nst=1\n", 0},
		{"traced", `set -x; a.b=1 true; echo "st=$?"`, "+ true\nksh: a.b=1: is not an identifier\n+ echo st=1\nst=1\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if out != c.want || st != c.st {
				t.Errorf("got %q at %d, want %q at %d", out, st, c.want, c.st)
			}
		})
	}
}

// TestTheShellsNamespaceIsReadOnly — `.sh` itself takes no store, refused as
// a readonly name is, without being listed as one (#6628, and the `{.sh}` row
// of #6591). ksh93u+ 2012-08-01 `-c`, measured 2026-10-15.
func TestTheShellsNamespaceIsReadOnly(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		st              int
	}{
		{"an assignment", `.sh=1; echo after`, "ksh: .sh: is read only\n", 1},
		{"in a function", `f() { .sh=1; echo in; }; f; echo after`, "ksh: .sh: is read only\n", 1},
		{"an append", `.sh+=x; echo after`, "ksh: .sh: is read only\n", 1},
		{"a declaration", `typeset .sh=1; echo after`, "ksh: .sh: is read only\n", 1},
		{"a {name} redirection", `true {.sh}</dev/null; echo "st=$?"`, "ksh: .sh: is read only\nst=1\n", 0},
		{"unset", `unset .sh; echo "st=$?"`, "ksh: unset: warning: .sh: is read only\nst=1\n", 0},
		{"read", `read .sh <<< x; echo "st=$?"`, "ksh: read: warning: .sh: is read only\nst=1\n", 0},
		{"not listed", `readonly -p | /usr/bin/grep -c sh`, "0\n", 1},
		{"a member is not it", `.sh.x=1; .shx=2; echo "st=$? ${.shx}"`, "st=0 2\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if out != c.want || st != c.st {
				t.Errorf("got %q at %d, want %q at %d", out, st, c.want, c.st)
			}
		})
	}
}

// TestTheDottedRefusalsAreLocatedAsKshLocatesThem — which of the two location
// shapes each refusal takes, on line 2 of a command string where both name the
// line (#6628). The assignment's sentences are the script's own line, the
// regular builtin's prefix and a declaration with no value are the builtin's.
// ksh93u+ 2012-08-01 `-c`, measured 2026-10-15.
func TestTheDottedRefusalsAreLocatedAsKshLocatesThem(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"export with a value", "export a.b=1", "ksh: line 2: a.b=1: is not an identifier\n"},
		{"a regular builtin's prefix", "a.b=1 true", "ksh[2]: a.b=1: is not an identifier\n"},
		{"a program's prefix", "a.b=1 /usr/bin/true", "ksh: line 2: a.b=1: is not an identifier\n"},
		{"a declaration with a value", "typeset a.b=1", "ksh: line 2: a.b=1: no parent\n"},
		{"a declaration with none", "typeset a.b", "ksh[2]: typeset: a.b: no parent\n"},
		{"unset", "unset a.b", "ksh[2]: unset: a.b: no parent\n"},
		{"an assignment", "a.b=1", "ksh: line 2: a.b=1: no parent\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), "true\n"+c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

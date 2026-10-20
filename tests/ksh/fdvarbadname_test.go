// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestABraceWordThatIsNotANameIsRefused — any brace word directly before a
// redirection operator is a descriptor variable here, and one that is not a
// name is refused before anything else about the redirection is done: the
// open, the close, the duplication (#6522). The rows are ksh93u+
// 2012-08-01's output, `-c`, measured 2026-10-15.
func TestABraceWordThatIsNotANameIsRefused(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a hyphen", `echo {a-b}</dev/null; echo "st=$?"`, "ksh: a-b: invalid variable name\nst=1\n"},
		{"a positional", `echo {1}</dev/null; echo "st=$?"`, "ksh: 1: invalid variable name\nst=1\n"},
		{"digits first", `echo {1a}>/dev/null; echo "st=$?"`, "ksh: 1a: invalid variable name\nst=1\n"},
		{"unexpanded", `echo {$x}</dev/null; echo "st=$?"`, "ksh: $x: invalid variable name\nst=1\n"},
		{"two dots", `echo {a..b}</dev/null; echo "st=$?"`, "ksh: a..b: invalid variable name\nst=1\n"},
		{"before the open", `echo {a-b}</nonexist; echo "st=$?"`, "ksh: a-b: invalid variable name\nst=1\n"},
		{"a close", `echo {a-b}>&-; echo "st=$?"`, "ksh: a-b: invalid variable name\nst=1\n"},
		{"in a function, the function goes on", `f() { echo {a-b}</dev/null; echo in; }; f; echo "after $?"`, "ksh: a-b: invalid variable name\nin\nafter 0\n"},
		{"on exec, the shell ends", `exec {1}</dev/null; echo after`, "ksh: 1: invalid variable name\n"},
		{"a comma is brace expansion", `echo {a,b}</dev/null`, "a b\n"},
		{"a name", `echo {_1}</dev/null; echo "st=$?"`, "\nst=0\n"},
		{"a dotted name", `echo {.a}</dev/null; echo "st=$?"`, "\nst=0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// TestABraceNameThatCannotTakeTheNumberIsRefusedFirst — the names that are
// names but cannot be stored into: a dotted name with no parent, and a
// subscript the arithmetic refuses. Both are refused before anything else
// about the redirection, the way a word that is not a name is; and a
// subscript that never closes is no subscript, so the name in front of it
// takes the number (#6591). ksh93u+ 2012-08-01 `-c`, measured 2026-10-15.
func TestABraceNameThatCannotTakeTheNumberIsRefusedFirst(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"no parent", `echo {a.b}</dev/null; echo "st=$?"`, "ksh: a.b: no parent\nst=1\n"},
		{"no parent, before the open", `echo {a.b}</nonexist; echo "st=$?"`, "ksh: a.b: no parent\nst=1\n"},
		{"no parent, on a close", `echo {a.b}>&-; echo "st=$?"`, "ksh: a.b: no parent\nst=1\n"},
		{"no parent, a leading dot", `echo {.a.b}</dev/null; echo "st=$?"`, "ksh: .a.b: no parent\nst=1\n"},
		{"the parent is the name before the last dot", `a=1; echo {a.b.c}</dev/null; echo "st=$?"`, "ksh: a.b.c: no parent\nst=1\n"},
		{"a parent that is there", `a=1; echo {a.b}</dev/null; echo "st=$?"`, "\nst=0\n"},
		{"under a compound, any path", `typeset -C a; echo {a.b.c.d}</dev/null; echo "st=$?"`, "\nst=0\n"},
		{"the shell's own namespace", `echo {.sh.x}</dev/null; echo "st=$?"`, "\nst=0\n"},
		{"below it, a parent is needed", `echo {.sh.q.r}</dev/null; echo "st=$?"`, "ksh: .sh.q.r: no parent\nst=1\n"},
		{"on exec, the shell ends", `exec {a.b}</dev/null; echo after`, "ksh: a.b: no parent\n"},
		{"a subscript the arithmetic refuses", `a=(1 2); i=1; true {a[$i]}</dev/null; echo "st=$? ${a[1]}"`, "ksh: $i: arithmetic syntax error\nst=1 2\n"},
		{"refused before the open", `a=(1 2); true {a[$i]}</nonexist; echo "st=$?"`, "ksh: $i: arithmetic syntax error\nst=1\n"},
		{"refused on a close", `a=(1 2); i=1; true {a[$i]}>&-; echo "st=$?"`, "ksh: $i: arithmetic syntax error\nst=1\n"},
		{"in the arithmetic's own words", `a=(1 2); true {a[x+]}</dev/null; echo "st=$?"`, "ksh: x+: more tokens expected\nst=1\n"},
		{"a subscript that never closes", `echo {a[}</dev/null; echo "st=$? ${a[@]}"`, "\nst=0 10\n"},
		{"is not refused first", `echo {a[}</nonexist; echo "st=$?"`, "ksh: /nonexist: cannot open [No such file or directory]\nst=1\n"},
		{"whatever is inside it", `echo {a[x}</dev/null; echo "st=$? ${a[@]} ${!a[@]}"`, "\nst=0 10 0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// TestABraceNameTakesALetterOfTheLocale — a letter past ASCII is a name
// character where the locale has it, at a `{name}` as in an assignment:
// `{é}` stores under en_US.UTF-8 and is `é: invalid variable name` under C
// (#6591). ksh93u+, measured 2026-10-15 in both.
func TestABraceNameTakesALetterOfTheLocale(t *testing.T) {
	const src = `echo {é}</dev/null; echo "st=$?"`
	if out, _ := runKshInAUTF8Locale(t, t.TempDir(), src); out != "\nst=0\n" {
		t.Errorf("UTF-8: got %q, want the store", out)
	}
	if out, _ := runKsh(t, t.TempDir(), src); out != "ksh: é: invalid variable name\nst=1\n" {
		t.Errorf("C: got %q, want the refusal", out)
	}
}

// TestAnFdVariableNeedsItsOperatorAdjacent — a blank between `{v}` and the
// operator leaves `{v}` an argument here, where zsh reads across it (#6587).
// ksh93u+, measured 2026-10-15.
func TestAnFdVariableNeedsItsOperatorAdjacent(t *testing.T) {
	if out, _ := runKsh(t, t.TempDir(), `echo {v} >/dev/null; echo "[$v]"`); out != "[]\n" {
		t.Errorf("got %q, want the word written to the file and v unset", out)
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestAMoveGivesItsSourceBackUnlessItsDestinationWasClosed — the close half
// of `N<&M-` outlives the command only where N was closed as the move was
// made, in this shell, and N is a number rather than a `{name}`'s (#6626).
// Every row opens 9 first and asks `true <&9` after; bash 5.3.20 `-c`,
// measured 2026-10-15.
func TestAMoveGivesItsSourceBackUnlessItsDestinationWasClosed(t *testing.T) {
	const closed = "bash: line 1: 9: Bad file descriptor\n"
	for _, c := range []struct{ name, src, errs string }{
		{"onto a closed number", "true 4<&9-", closed},
		{"onto a number open before", "exec 4</dev/null; true 4<&9-", ""},
		{"onto standard input", "true 0<&9-", ""},
		{"onto standard output", "true 1>&9-", ""},
		{"onto a closed standard stream", "exec 3>&1 1>&-; true 1>&9-; exec 1>&3", closed},
		{"onto a number this command opened", "true 4</dev/null 4<&9-", ""},
		{"onto a function's standard input", "f() { :; }; f 0<&9-", ""},
		{"onto a {name}", "true {v}<&9-", ""},
		{"onto a {name} on a group", "{ :; } {v}<&9-", ""},
		{"onto a {name} behind a saved number", "true 5>/dev/null {v}<&9-", ""},
		{"in parentheses", "( : ) 4<&9-", ""},
		{"exec onto a {name}", "exec {v}<&9-", closed},
		{"exec onto an open number", "exec 4</dev/null; exec 4<&9-", closed},
		{"a refused {name} on exec", "readonly v=1; exec {v}<&9-", "bash: line 1: v: readonly variable\nbash: line 1: v: cannot assign fd to variable\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, errs := runBashSplit(t, "exec 9</dev/null; "+c.src+"; true <&9\n")
			if errs != c.errs {
				t.Errorf("err %q, want %q", errs, c.errs)
			}
		})
	}
}

// TestAPersistingFdVariableOutlivesASavedTable — a `{name}` descriptor that
// outlives its command does so behind a redirection that saved the table
// first, and its number stays its own when a move beside it is taken back
// (#6626). bash 5.3.20 `-c`, measured 2026-10-15.
func TestAPersistingFdVariableOutlivesASavedTable(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"an open", `true 5>/dev/null {v}>/dev/null; true >&$v; echo "st=$?"`},
		{"a duplication", `true 5>/dev/null {v}>&1; true >&$v; echo "st=$?"`},
		{"a here-string", `true 5>/dev/null {v}<<<x; true <&$v; echo "st=$?"`},
		{"a move", `exec 9</dev/null; true {v}<&9-; true <&$v; echo "st=$?"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs := runBashSplit(t, c.src+"\n")
			if out != "st=0\n" || errs != "" {
				t.Errorf("out %q err %q, want st=0 and nothing else", out, errs)
			}
		})
	}
}

// TestADuplicationThroughAnExpansionNamesItsDestination — with nothing open
// at the source, a target that is not plain digits is named as written only
// where nothing, or the operator's own default, stands in front of the
// operator; a number of its own is named instead, and a move names its
// destination however it was written (#6627). `x=3`, 3 closed; bash 5.3.20
// `-c`, measured 2026-10-15.
func TestADuplicationThroughAnExpansionNamesItsDestination(t *testing.T) {
	const dup = "bash: redirection error: cannot duplicate fd: Bad file descriptor\n"
	for _, c := range []struct{ src, errs string }{
		{"true 4>&$x", "bash: line 1: 4: Bad file descriptor\n"},
		{"true 4<&$x", "bash: line 1: 4: Bad file descriptor\n"},
		{"true 2>&$x", "bash: line 1: 2: Bad file descriptor\n"},
		{"true 0>&$x", "bash: line 1: 0: Bad file descriptor\n"},
		{"true 4>&\"$x\"", "bash: line 1: 4: Bad file descriptor\n"},
		{"true 4>&3$x", "bash: line 1: 4: Bad file descriptor\n"},
		{"{ :; } 4>&$x", "bash: line 1: 4: Bad file descriptor\n"},
		{"exec 4>&$x", "bash: line 1: 4: Bad file descriptor\n"},
		{"true >&$x", "bash: line 1: $x: Bad file descriptor\n"},
		{"true 1>&$x", "bash: line 1: $x: Bad file descriptor\n"},
		{"true 0<&$x", "bash: line 1: $x: Bad file descriptor\n"},
		{"true 4>&3", "bash: line 1: 3: Bad file descriptor\n"},
		{"true 4<&$x-", "bash: line 1: 4: Bad file descriptor\n"},
		{"true >&$x-", dup + "bash: line 1: 1: Bad file descriptor\n"},
		{"true <&$x-", dup + "bash: line 1: 0: Bad file descriptor\n"},
		{"true 4<&3-", "bash: line 1: 3: Bad file descriptor\n"},
		{"true {v}<&$x-", dup + "bash: line 1: 3: Bad file descriptor\n"},
		{"true >&3-", dup + "bash: line 1: 3: Bad file descriptor\n"},
		{"exec 4</dev/null; true 4<&3-", dup + "bash: line 1: 3: Bad file descriptor\n"},
		{"exec 4</dev/null; true 4<&3", "bash: line 1: 3: Bad file descriptor\n"},
	} {
		t.Run(c.src, func(t *testing.T) {
			_, errs := runBashSplit(t, "x=3; "+c.src+"\n")
			if errs != c.errs {
				t.Errorf("err %q, want %q", errs, c.errs)
			}
		})
	}
}

// TestABareFdVariableListIsASubshell — the Yes side of
// interp.Semantics.FdVariableMakesABareListASubshell: a command of
// redirections alone that carries a `{name}` is made in a child, so the
// shell keeps neither the name, nor the descriptor, nor what its targets
// expanded, and a move's source comes back (#6630). bash 5.3.20 `-c`,
// measured 2026-10-15.
func TestABareFdVariableListIsASubshell(t *testing.T) {
	for _, c := range []struct{ name, src, want, errs string }{
		{"the name", `{v}>/dev/null; echo "[$v]"`, "[]\n", ""},
		{"a value it had", `v=5; {v}>/dev/null; echo "[$v]"`, "[5]\n", ""},
		{"the descriptor", `{v}>/dev/null {w}>/dev/null; exec {q}>/dev/null; echo "$q"`, "10\n", ""},
		{"a target behind it", `{v}>/dev/null 2>/dev/nul${x:=l}; echo "[$x]"`, "[]\n", ""},
		{"a target ahead of it", `2>/dev/nul${x:=l} {v}>/dev/null; echo "[$x]"`, "[]\n", ""},
		{"the prefix stays", `x=1 {v}>/dev/null; echo "[$v][$x]"`, "[][1]\n", ""},
		{"a move", `exec 9</dev/null; 4<&9- {v}>/dev/null; true <&9; echo "st=$?"`, "st=0\n", ""},
		{
			"a refused name", `readonly v=1; {v}>/dev/null; echo "st=$? [$v]"`, "st=1 [1]\n",
			"bash: line 1: v: readonly variable\nbash: line 1: v: cannot assign fd to variable\n",
		},
		{"a function's", `f() { {v}>/dev/null; echo "in[$v]"; }; f; echo "[$v]"`, "in[]\n[]\n", ""},
		{"without a {name} the target stays", `2>/dev/nul${x:=l}; echo "[$x]"`, "[l]\n", ""},
		{"without a {name} a move closes", `exec 9</dev/null; 4<&9-; true <&9; echo "st=$?"`, "st=1\n", "bash: line 1: 9: Bad file descriptor\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs := runBashSplit(t, c.src+"\n")
			if out != c.want || errs != c.errs {
				t.Errorf("out %q err %q, want %q and %q", out, errs, c.want, c.errs)
			}
		})
	}
}

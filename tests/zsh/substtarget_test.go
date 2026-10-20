// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A plain redirection's target that a process substitution begins, with
// anything after it, is refused before anything in the list is opened, and
// the shell ends on the same boundary as over a duplication target that is
// not a descriptor: a command the shell runs itself. Measured 2026-10-16 on
// zsh 5.9.2 under `-fc` (#6766), and the boundary for both refusals the same
// day — a function call, a group, a loop and an assignment end the shell,
// the parentheses and an external program carry on. See
// interp.Semantics.SubstitutionTargetWithMoreIsRefused and
// interp.DuplicationTargetErrorEndsTheShellOnABuiltin.
func TestARefusedTargetEndsTheShellOnACommandItRunsItself(t *testing.T) {
	const refused = "invalid syntax for process substitution in redirection"
	for _, c := range []struct {
		src, out string
		status   int
		errs     string
	}{
		{`echo hi > >(cat)x; echo next`, "", 1, refused},
		{`echo hi > >(cat)""; echo next`, "", 1, refused},
		{`read v < <(echo a)x; echo next`, "", 1, refused},
		{`{ :; } > >(cat)x; echo next`, "", 1, refused},
		{`f() { :; }; f > >(cat)x; echo next`, "", 1, refused},
		{`x=1 > >(cat)x; echo next`, "", 1, refused},
		{`( : ) > >(cat)x; echo next $?`, "next 1\n", 0, refused},
		{`/bin/cat < <(echo a)x; echo next $?`, "next 1\n", 0, refused},
		// Nothing in the list is opened, and the substitution never starts.
		{`: </nonexistent > >(echo ran >&2)x; echo next`, "", 1, refused},
		// The same boundary for a word after `<&` that is not a descriptor.
		{`: <&x; echo next`, "", 1, "file number expected"},
		{`{ :; } <&x; echo next`, "", 1, "file number expected"},
		{`f() { :; }; f <&x; echo next`, "", 1, "file number expected"},
		{`while false; do :; done <&x; echo next`, "", 1, "file number expected"},
		{`x=1 <&x; echo next`, "", 1, "file number expected"},
		{`( : ) <&x; echo next`, "next\n", 0, "file number expected"},
		{`/bin/cat <&x; echo next`, "next\n", 0, "file number expected"},
		// And an append, a duplication or `&>` opens the path it comes to.
		{`echo hi >> >(cat)x; echo next $?`, "next 1\n", 0, "no such file or directory"},
	} {
		out, st, errs := runZshSplitOnRoute(t, interp.RouteCommandString, c.src)
		if out != c.out || st != c.status || !strings.Contains(errs, c.errs) ||
			strings.Contains(errs, "ran") || strings.Contains(errs, "nonexistent") {
			t.Errorf("%s: got %q, status %d, stderr %q; want %q, status %d, stderr naming %q",
				c.src, out, st, errs, c.out, c.status, c.errs)
		}
	}
}

// And one running against a plain operator is refused while reading, by the
// whole word: nothing before it in the program runs (#6812). See
// syntax.Dialect.ProcessSubstitutionAgainstAPlainRedirectionIsRefused.
func TestASubstitutionAgainstAPlainRedirectionIsAParseError(t *testing.T) {
	for _, c := range []struct{ src, word string }{
		{`echo before; echo hi > <(cat)`, "<(cat)"},
		{`echo before; cat < >(cat)`, ">(cat)"},
		{`echo hi >| <(cat)x`, "<(cat)x"},
		{`echo hi >! <(cat)`, "<(cat)"},
		{`f() { echo hi 2> <(cat); }`, "<(cat)"},
	} {
		_, err := syntax.Parse(c.src, zsh.Dialect())
		if err == nil || !strings.Contains(err.Error(), "`"+c.word+"'") {
			t.Errorf("%s: got %v, want the word %s refused", c.src, err, c.word)
		}
	}
	for _, src := range []string{`echo hi >> <(cat)`, `cat <> >(cat)`, `echo hi &> <(cat)`, `echo hi > x<(cat)`} {
		if _, err := syntax.Parse(src, zsh.Dialect()); err != nil {
			t.Errorf("%s: %v, want it read", src, err)
		}
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// Some builtins read an argument's first unquoted `=` as an assignment's, and
// so give what follows it, and every colon after that, an assignment value's
// tildes, while the word is otherwise an ordinary argument: split, matched
// against the filesystem, and passed on. Measured 2026-10-03 on zsh 5.9.2
// under -f:
//
//	builtin local x=~root:~root   /var/root:/var/root
//	\local x=~root  l=local; $l x=~root  noglob local x=~root   /var/root
//	builtin local x=$(echo a b)   x=a, and b declared: still split
//	builtin local "x=~root"  x\=~root  x=a~root   as written
//	alias a=~root; hash -d nd=~root   /var/root
//	echo x=~root  print x=~root  set -- x=~root   as written
//
// The declaration written as the reserved word takes its own route and is
// not this one. A dialect names its builtins with
// MarkBuiltinWhoseEqualsOpensATildeContext; the core names none (#5670).

// MarkBuiltinWhoseEqualsOpensATildeContext names a builtin whose arguments'
// first unquoted `=` opens an assignment value's tilde context.
func (r *Runner) MarkBuiltinWhoseEqualsOpensATildeContext(name string) {
	if r.equalsTildeBuiltins == nil {
		r.equalsTildeBuiltins = map[string]bool{}
	}
	r.equalsTildeBuiltins[name] = true
}

// equalsOpensATildeContextFor reports whether the i-th word of a command
// whose argv so far is argv is an argument of such a builtin, reached by its
// name or through `builtin`, and not a declaration's own operand.
func (r *Runner) equalsOpensATildeContextFor(c *syntax.SimpleCmd, argv []string, i int) bool {
	if len(r.equalsTildeBuiltins) == 0 || i == 0 || len(argv) == 0 {
		return false
	}
	k := 0
	if argv[0] == "builtin" && len(argv) > 1 {
		k = 1
	}
	if !r.equalsTildeBuiltins[argv[k]] {
		return false
	}
	// A declaration the reserved word runs reads its operands on a route of
	// its own, which already gives an assignment's value its tildes.
	return k != 0 || !r.declares(argv[0]) || !r.declarationCommand(c, argv)
}

// equalsTildeContextWord arms the flag for one word and hands back the
// restore. It is spent by the word pipeline, so nothing expanded inside the
// word inherits it; see spendTheEqualsTildeContext.
func (r *Runner) equalsTildeContextWord() func() {
	prev := r.equalsTildeContext
	r.equalsTildeContext = true
	return func() { r.equalsTildeContext = prev }
}

// spendTheEqualsTildeContext reads the flag and puts it down for the duration
// of the word.
func (r *Runner) spendTheEqualsTildeContext() (armed bool, restore func()) {
	if !r.equalsTildeContext {
		return false, func() {}
	}
	r.equalsTildeContext = false
	return true, func() { r.equalsTildeContext = true }
}

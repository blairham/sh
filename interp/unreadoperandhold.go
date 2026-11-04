// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A declaration operand nothing can read while it is held.
//
// The column that stores a declaration's operands in turn holds each one's
// value under its name so the operands behind it see it — `typeset x=1 y=$x`
// gives `y` 1 there — and puts it back before the utility stores everything
// for real (see interp/declarationinturn.go). The hold writes the cell
// directly and the release puts the saved state back, so what it changes is
// only what a reader between the two sees. Where nothing between them can read
// the name, the hold and its release are work with no answer to give, and they
// were the costliest part of `typeset a=$1 b=$2 c` in a `function` body: a
// saved copy of each cell and the put-back at the end (#6373).
//
// Nothing can read it when every one of these holds:
//
//   - the operand is a whole name, not appended to and not a reference, so
//     the hold is the plain one;
//   - every word written behind it is literal text that expands to itself and
//     bare positional parameters, which no variable can change — quoted, or in
//     an assignment's value, so that the IFS a hold might be holding splits
//     none of them;
//   - the command has no assignment prefix and no array-literal operand, both
//     of which expand after the operands where the hold would be visible;
//   - nothing is traced, since each later operand's trace line expands `PS4`;
//   - and the utility makes its operands locals of the running function. A
//     later operand can still end the walk — a frozen name, an unset
//     positional under `set -u` — and a stop there puts back everything that
//     was held (operandsInTurn.stop), so a hold that was never made leaves the
//     same state. At the top level a stop keeps what was held, and the hold is
//     made.
//
// The redirections are not on the list: the walk is put back before they are
// applied.

// unreadHoldShortcut switches the route on. A variable so a test can run the
// same script with every operand held.
var unreadHoldShortcut = true

// holdIsUnread reports whether the hold of the whole-name operand name can be
// left out, because nothing behind it can read it before it is put back.
func (r *Runner) holdIsUnread(c *syntax.SimpleCmd, behind []*syntax.Word, name string, argv []string) bool {
	if !unreadHoldShortcut || len(c.Assigns) > 0 || r.tracing() || r.isNameref(name) {
		return false
	}
	for _, w := range behind {
		if !wordReadsNoVariable(w) {
			return false
		}
	}
	return r.operandsAreLocalsOfTheCall(argv)
}

// wordReadsNoVariable reports whether a word's expansion is its own text and
// positional parameters, with nothing a variable could change: every literal
// span is quoted or plain text, and a parameter is bare, positional, and either
// quoted or in an assignment's value, where it is neither split nor matched.
func wordReadsNoVariable(w *syntax.Word) bool {
	_, assigns := writtenAssignmentName(w)
	for i := range w.Spans {
		s := &w.Spans[i]
		switch s.Kind {
		case syntax.Literal:
			if s.Quoting == syntax.Unquoted && !selfExpandingText(s.Value) {
				return false
			}
		case syntax.ParamExp:
			if !bareParamOnly(s.Param) || (!assigns && s.Quoting == syntax.Unquoted) {
				return false
			}
			if _, ok := positionalNumber(s.Param.Name); !ok {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// selfExpandingText reports whether unquoted text expands to itself whatever
// the variables hold: no tilde, which reads `HOME`; no pattern or brace
// character; no `+`, which makes an appending operand that reads the name it
// appends to; no subscript; no escape.
func selfExpandingText(s string) bool {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case isNameByte(c), c == '=', c == '-', c == '.', c == ',', c == '/', c == ':', c == '%', c == '@':
		default:
			return false
		}
	}
	return true
}

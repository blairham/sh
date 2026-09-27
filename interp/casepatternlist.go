// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// Whether a `case` arm's **parenthesized** pattern list is read as one word —
// blanks and newlines inside it being characters of a pattern, and an
// alternative being writable as nothing — is a question about how text is
// *lexed*, so it lives on syntax.Dialect and not on Semantics. One dialect
// spells it as an option a running script switches, which is why a runner has
// to be able to move it.
//
// That is the same combination [Runner.SetDoubledQuoteInSingleQuotes] has, and
// the same consequences follow: the answer that decides is the one in force
// when the text is **lexed**, the dialect is copied and replaced rather than
// written through, and the front end's run loop reads the rest of the program
// with whatever it finds. See interp/doubledquote.go for the long version.
//
// # Three fields and one question
//
// The reading is three fields of syntax.Dialect — a blank inside the list, a
// newline inside it, and an alternative written as nothing — and they move
// together because they were **measured** together. Against
// `/opt/homebrew/bin/zsh`, `zsh 5.9.2 (aarch64-apple-darwin25.4.0)`, from a
// script file under `set -n`, 2026-09-27, with the option moved on the line
// before:
//
//	                                     option off   option on
//	case "a b" in (a b) echo m;; esac       parses      refused
//	case a in (a<newline>|b) …              parses      refused
//	case "" in ( ) echo em;; esac           parses      refused
//
// **The neighboring reading does not move, and that is the control**: `case
// x; in a) :;; esac` parses with the option on and off alike, so this is not
// "the header is read the standard's way" and the flag it would have taken
// with it — syntax.Dialect.CaseHeaderSpansSeparators — is deliberately not
// here. A group of fields moved on one measurement that never varied the thing
// it was keyed on is the shape this package has been caught by before.

// CasePatternListReadAsOneWord reports whether this runner reads a `case`
// arm's parenthesized pattern list as one word.
func (r *Runner) CasePatternListReadAsOneWord() bool {
	// One of the three rather than all three, because nothing writes them
	// apart: a dialect places all three at construction and the setter below
	// is the only other writer. TestTheCasePatternListReadingMovesAsOne is
	// what keeps that true.
	return r.lang().CasePatternListSpansBlanks
}

// SetCasePatternListReadAsOneWord moves the reading, for a dialect whose
// option namespace has a name for it.
//
// The dialect is copied and replaced rather than written through: the pointer
// is shared with every subshell cloned from this runner, and a script must not
// change the grammar of the shell that spawned it.
func (r *Runner) SetCasePatternListReadAsOneWord(on bool) {
	d := r.dialect()
	if d.CasePatternListSpansBlanks == on &&
		d.CasePatternListSpansNewlines == on &&
		d.CasePatternMayBeEmpty == on {
		return
	}
	d.CasePatternListSpansBlanks = on
	d.CasePatternListSpansNewlines = on
	d.CasePatternMayBeEmpty = on
	r.Dialect = &d
}

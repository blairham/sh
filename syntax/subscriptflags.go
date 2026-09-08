// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "strings"

// A subscript may open with a parenthesized flag group of its own —
// `${a[(re)value]}` — which decides how the rest of the subscript selects an
// element. One grammar in the panel has it, so this file is that grammar's
// shape, measured with the binary rather than taken from a manual.
//
// **It is not the `${(flags)name}` group wearing a different position**, and
// the three differences are each measurable:
//
//   - The character set is not the same one. An expansion flag group carries
//     any of the fifty-odd characters in paramFlagChars; a subscript
//     recognizes exactly thirteen, and `${a[(U)1]}` is no flag group at all.
//   - The failure is not the same failure. An unreadable character in
//     `${(…)x}` is `error in flags near position N`; an unreadable one in a
//     subscript means the parentheses were never a group, so the subscript
//     stands as written and is read as arithmetic — `${a[(z)2]}` is
//     `bad math expression`, which is the same *reading* the four grammars
//     without subscript flags give it, each in its own words. An unknown
//     subscript flag is therefore not an error of its own anywhere.
//   - The same letter means something else. `(i)` in an expansion group is a
//     sort order; `(i)` in a subscript is the index of the first match.
//     `(e)`, `(n)`, `(s)`, `(f)`, `(w)` and `(p)` differ the same way.
//
// Sharing the parser would therefore have to be undone at every one of those
// three points, which is the whole of it.

// subscriptFlagChars are the flag letters a subscript's group may carry that
// take no argument, and subscriptFlagArgChars those that must be followed by
// one. Measured by asking the shell for every ASCII letter, twice — bare and
// with a `:1:` argument — and keeping the ones that did not fall back to
// arithmetic. There are no others.
const (
	subscriptFlagChars    = "wfpeiIrRkK"
	subscriptFlagArgChars = "bns"
)

// SubscriptFlags is the flag group a subscript opened with.
//
// Arg is the rest of the subscript — the operand the search flags match
// against, and the ordinary subscript when the group named no search flag at
// all: `${a[()2]}` and `${a[(e)2]}` are both the second element, measured.
// Keeping it here rather than replacing ParamExpr.Index leaves the written
// subscript recoverable, which a diagnostic that names `a[(re)x]` needs.
type SubscriptFlags struct {
	// Src is the group as written, parentheses included: `(rn:2:)`.
	Src string
	// Flags is the letters in written order with their arguments stripped,
	// so `(rn:2:)` carries "rn".
	Flags string
	// Nth and Begin are the `n` and `b` arguments, and Sep the `s` one, as
	// written. They are *not* expanded: measured, `(rb:$one:)` reads `$one`
	// as arithmetic and fails there rather than substituting a value.
	Nth   string
	Begin string
	Sep   string
	// Arg is the subscript after the group.
	Arg *Word
}

// Subscript is the word a subscript's *reading* uses: the operand behind a
// flag group where there is one, and the whole subscript where there is not.
//
// One accessor rather than a test at each of the dozen places that read a
// subscript's text, because every one of them wants the same answer: the
// group is never part of the text being read, whatever it selects.
func (p *ParamExpr) Subscript() *Word {
	if p.IndexFlags != nil {
		return p.IndexFlags.Arg
	}
	return p.Index
}

// scanSubscriptFlags reads the flag group text opens with, returning it and
// the subscript that follows.
//
// ok is false for anything that is not a group, and that is the *whole* of
// the error handling: a character the group cannot carry, an argument-taking
// flag with no argument, an argument with no closing delimiter and a group
// with no closing parenthesis all mean the parentheses were ordinary text.
// Measured — `${a[(z)2]}`, `${a[(n)2]}` and `${a[(n:2)x]}` are all
// `bad math expression` and none of them is a flag error — so there is no
// position to record and nothing to defer to the run, which is what makes
// this additive: a subscript no group could open reads exactly as it did
// before the grammar had groups at all.
func scanSubscriptFlags(text string) (g *SubscriptFlags, rest string, ok bool) {
	if !strings.HasPrefix(text, "(") {
		return nil, text, false
	}
	g = &SubscriptFlags{}
	i := 1
	for i < len(text) {
		c := text[i]
		if c == ')' {
			// An empty group is still a group: `${a[()2]}` is the second
			// element and not an arithmetic failure.
			g.Src = text[:i+1]
			return g, text[i+1:], true
		}
		takesArg := strings.IndexByte(subscriptFlagArgChars, c) >= 0
		if !takesArg && strings.IndexByte(subscriptFlagChars, c) < 0 {
			return nil, text, false
		}
		g.Flags += string(c)
		i++
		if !takesArg {
			continue
		}
		if i >= len(text) {
			return nil, text, false
		}
		open := text[i]
		if open == ')' {
			return nil, text, false
		}
		j := strings.IndexByte(text[i+1:], matchingFlagDelimiter(open))
		if j < 0 {
			return nil, text, false
		}
		arg := text[i+1 : i+1+j]
		switch c {
		case 'n':
			g.Nth = arg
		case 'b':
			g.Begin = arg
		case 's':
			g.Sep = arg
		}
		i += j + 2
	}
	return nil, text, false
}

// LeadingIndex is one subscript of a chain: everything ParamExpr.Index and
// ParamExpr.IndexFlags hold for the last one, for one of the ones before it.
//
// A pair rather than a bare word, because a subscript is the two together —
// `${a[(r)x][1]}` opens with a flag group, and a link that carried only the
// text would search nothing and read `(r)x` as arithmetic.
type LeadingIndex struct {
	// Index is the subscript as written, flag group included.
	Index *Word
	// Flags is the group it opened with, nil when there was none.
	Flags *SubscriptFlags
}

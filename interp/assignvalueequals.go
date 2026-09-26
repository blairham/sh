// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// `=cmd` runs at an assignment value's head and after each of its colons,
// exactly where a tilde does.
//
// Runner.expandEquals is the *word* road and it was the whole of it: a value
// that named a command kept the two characters, because
// Runner.expandAssignValue called expandTildeIn and expandColonTildes and
// never this (#4566). The two positions are the tilde's two positions, which
// is why this file is shaped like interp/assignmentshapedword.go rather than
// like a second call bolted onto the head.
//
// Measured 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) at
// /opt/homebrew/bin/zsh, run `-f`; `go version -m` on it says *not a Go
// executable*. The left column is the reference and the right is what this
// shell answered:
//
//	v==ls;         print -r -- "[$v]"    [/bin/ls]        was [=ls]
//	typeset t==ls; print -r -- "[$t]"    [/bin/ls]        was [=ls]
//	export x==ls;  print -r -- "[$x]"    [/bin/ls]        was [=ls]
//	v=x:=ls;       print -r -- "[$v]"    [x:/bin/ls]      was [x:=ls]
//	v==ls:=ls;     print -r -- "[$v]"    [/bin/ls:/bin/ls] was [=ls:=ls]
//	v=x=ls;        print -r -- "[$v]"    [x=ls]           unchanged
//	arr[1]==ls                           [/bin/ls]        was [=ls]
//	v+==ls                               [/bin/ls]        was [=ls]
//
// and under MAGIC_EQUAL_SUBST, where the *word* road takes the same two
// positions from the first unquoted `=` rather than from the front:
//
//	print -r -- a==ls                    a=/bin/ls        was a==ls
//	print -r -- a=x:=ls                  a=x:/bin/ls      was a=x:=ls
//	print -r -- --opt==ls                --opt=/bin/ls    was --opt==ls
//	print -r -- a=b==ls                  a=b==ls          unchanged
//
// **The two controls, and both already agreed**, which is what says this is
// the value road rather than the expansion being absent: `print -r -- =ls` is
// `/bin/ls` in both, through Runner.expandEquals and Semantics.EqualsExpansion;
// and `unsetopt equals; w==ls` is `=ls` in both, so the option really does
// switch it off. Each keeps the `=ls` visible where it does not expand, so a
// row that produced nothing at all would be read as such rather than as
// agreement.
//
// **A colon bounds the name here and does not in an ordinary word.** Measured
// on the same binary: `v==ls:b` is `/bin/ls:b`, and the word `=ls:b` reports
// `ls:b not found`. That is the tilde's asymmetry as well — a colon closes an
// assignment value's tilde prefix and Semantics.TildeColonEndsAnOrdinaryWordsPrefix
// is an axis for the ordinary word — so the two roads are told apart by which
// bytes close a segment rather than by two readings of the same road.
//
// A name that runs off the end of its span into something that is not plain
// text is left as written, which is the limit expandColonTildes already draws
// for the same reason: `v==$e` with `e=ls` is `/bin/ls` in the reference and
// keeps its characters here. It is a residue of this road rather than of this
// change — the word road has the same limit and had it before — and it is not
// what the issue is about.

// assignValueEquals expands the `=cmd` forms an assignment's value takes: one
// at the head, and one after each unquoted colon.
//
// Nothing is asked where nothing could move, so an ordinary `x=1` reaches no
// axis: a refusal reported for every assignment in a script would be an axis
// refusing the shape rather than the behavior.
func (r *Runner) assignValueEquals(w *syntax.Word) {
	if w == nil || len(w.Spans) == 0 {
		return
	}
	r.expandEqualsSegments(w, 0, 0, true, false)
}

// wordEquals is the same pair of positions counted from the first unquoted
// `=` of a word MAGIC_EQUAL_SUBST has claimed, which is the road
// interp/assignmentshapedword.go opens for the tilde.
//
// The wider axis is asked and the narrower one is not, because the narrow
// assignment-shape rule belongs to the columns with no `=cmd` expansion at
// all: measured, `print -r -- FOO==ls` keeps its characters in zsh with the
// option off and in bash 5.3.20, which is the shell the narrow rule was
// measured on. EqualsExpansion stands in front of both, so a dialect without
// the expansion asks neither.
func (r *Runner) wordEquals(w *syntax.Word) {
	if len(w.Spans) == 0 {
		return
	}
	span, eq, ok := firstUnquotedEquals(w.Spans)
	if !ok || !equalsCouldMove(w.Spans, span, eq) {
		return
	}
	if !r.ask(r.sem().EqualsExpansion, "`=cmd` expanding to a path") {
		return
	}
	if !r.ask(r.sem().TheFirstUnquotedEqualsInAWordOpensATildeContext,
		"every word with an unquoted `=` in it being a tilde context") {
		return
	}
	r.expandEqualsSegments(w, span, eq+1, true, true)
}

// equalsCouldMove reports whether a `=` at span/eq has a `=cmd` behind it
// that the word road could expand: one straight after the `=`, or one after a
// colon in the rest of that span. tildeCouldMove is the same gate for the
// tilde, and the two are separate because a word may carry either without the
// other.
func equalsCouldMove(spans []syntax.Span, span, eq int) bool {
	v := spans[span].Value
	return strings.HasPrefix(v[eq+1:], "=") || strings.Contains(v[eq:], ":=")
}

// expandEqualsSegments rewrites every `=cmd` a value holds: the one at
// headSpan/headOff when ask is for a head at all, and the one after each
// unquoted colon in every plain span of the word.
//
// The axis is asked on the first segment that could move rather than up
// front, for assignValueEquals' reason — and it is asked once, because a
// value with two of them is one reading of one word.
func (r *Runner) expandEqualsSegments(w *syntax.Word, headSpan, headOff int, head, answered bool) {
	asked, on := answered, answered
	for i := range w.Spans {
		s := &w.Spans[i]
		if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
			continue
		}
		last := i == len(w.Spans)-1
		v := s.Value
		var b strings.Builder
		for j := 0; j < len(v); j++ {
			// The two positions, and nowhere else: the value's head where
			// the caller named one, and straight after an unquoted colon.
			atHead := head && i == headSpan && j == headOff
			afterColon := j > 0 && v[j-1] == ':'
			if v[j] != '=' || !atHead && !afterColon {
				b.WriteByte(v[j])
				continue
			}
			// The name runs to the next colon, or to the end of the span
			// when nothing follows it. An empty name is the bare `=` and is
			// not this expansion.
			k := j + 1
			for k < len(v) && v[k] != ':' {
				k++
			}
			if k == j+1 || k == len(v) && !last {
				b.WriteByte(v[j])
				continue
			}
			if !asked {
				on = r.ask(r.sem().EqualsExpansion, "`=cmd` expanding to a path")
				asked = true
			}
			if !on {
				return
			}
			path, ok := r.equalsPath(v[j+1 : k])
			if !ok {
				return
			}
			b.WriteString(path)
			j = k - 1
		}
		s.Value = b.String()
	}
}

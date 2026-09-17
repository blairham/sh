// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "strings"

// dotRangeOf splits a subscript word at the first `..` written in one of its
// literal spans, and is nil where there is none.
//
// A second `..` ends the high end again, and what stood between the two is
// never read: measured on ksh93u+ 2012-08-01 with `a=(a b c d e)`,
// `${a[0..9..3]}` is `a b c d`, `${a[1..3..4..2]}` is `b c`, and
// `${a[1..1+..3]}` is `b c d` although `1+` is no expression. The pairs are
// taken left to right without overlapping, so `${a[1...3]}` is the range 1
// through `.3` — the single element `b` — and `${a[1....3]}` is 1 through 3.
//
// Spans rather than the raw text, because what counts is where the characters
// were written and not how: a quoted `"1..3"` is the range in the shell with
// the construct, while a `..` a substitution brings in is not, and neither is
// one whose first dot was escaped — `1\..3` is lexed as a literal `1`, a
// backslash-quoted `.` and a literal `.3`, so no single literal span holds the
// pair. A span that is a substitution is never looked inside: `$((1))..3`
// splits between the two, and `${x:-1..2}` is a default and not a range.
func dotRangeOf(w *Word) *SubscriptRange {
	rng := splitAtDots(w)
	if rng == nil {
		return nil
	}
	for {
		again := splitAtDots(rng.Hi.Text)
		if again == nil {
			return rng
		}
		rng.Hi = again.Hi
	}
}

// splitAtDots is one split of dotRangeOf's: at the first written `..`.
func splitAtDots(w *Word) *SubscriptRange {
	if w == nil {
		return nil
	}
	for i, s := range w.Spans {
		if s.Kind != Literal || s.Quoting == BackslashQuoted {
			continue
		}
		at := strings.Index(s.Value, "..")
		if at < 0 {
			continue
		}
		lo := &Word{Start: w.Start, Stop: s.Pos}
		lo.Spans = append(lo.Spans, w.Spans[:i]...)
		if head := s.Value[:at]; head != "" {
			left := s
			left.Value = head
			lo.Spans = append(lo.Spans, left)
		}
		hi := &Word{Start: s.Pos, Stop: w.Stop}
		if tail := s.Value[at+2:]; tail != "" {
			right := s
			right.Value = tail
			hi.Spans = append(hi.Spans, right)
		}
		hi.Spans = append(hi.Spans, w.Spans[i+1:]...)
		return &SubscriptRange{Lo: SubscriptEnd{Text: lo}, Hi: SubscriptEnd{Text: hi}}
	}
	return nil
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// One dialect words a refused `test` from the **word counts of the segments
// between the connectives**, rather than from where a left-to-right parse
// stopped.
//
// It is a second reading of the same refusal and not a second evaluator: every
// list below is status 2 in that column and status 2 here, and only the
// sentence and the word it names move. So the expression is read exactly as it
// is read everywhere else, and this is asked only once that reading has
// refused — see [Diagnostics.TestRefusalCountsTheWordsBetweenConnectives],
// which is the flag, and Runner.runTestForm, which is where it is asked.
//
// # The rule
//
// Split the operands at every `-a` and `-o`; the **leftmost segment that does
// not read** is what the shell complains about, and a segment is judged by how
// many words it holds once its leading `!`s are off:
//
//	1 word    a bare word, and nothing to complain about
//	2 words   word 1 must be a unary operator. Spelled like one and not
//	          known: `unknown condition: <word 1>`. Not spelled like one:
//	          the two-word sentence, naming word 1
//	3 words   word 1 a known unary operator: `too many arguments`, word 3
//	          being one word too many. Otherwise word 2 must be a binary
//	          operator — spelled like one and not known names it, and any
//	          other word is the ordinary operator's-place sentence
//	4 or more a primary off the front — a known unary and its operand, or a
//	          binary triple — leaves the rest over: `too many arguments`.
//	          Where there is no primary the complaint names **word 1**
//
// # Measured
//
// 2026-09-27 against zsh 5.9.2 (aarch64-apple-darwin25.4.0) under `-f -c`,
// `env -i PATH=/usr/bin:/bin LC_ALL=C`; `go version -m` on that binary says
// *not a Go executable*. Forty-five operand lists, of which the twenty below
// are the ones that decide the rule:
//
//	[ a b ]                 parse error: condition expected: a
//	[ -q a ]                unknown condition: -q
//	[ a b c ]               condition expected: b
//	[ a -q b ]              unknown condition: -q
//	[ -n foo scrimble ]     too many arguments
//	[ a b c d ]             condition expected: a
//	[ a b c d e f ]         condition expected: a
//	[ -n a -x b ]           too many arguments
//	[ a = b c d ]           too many arguments
//	[ a -o b c ]            parse error: condition expected: b
//	[ a b -a c ]            parse error: condition expected: a
//	[ -n a -a b c ]         parse error: condition expected: b
//	[ a = b -a c d ]        parse error: condition expected: c
//	[ a b c -a ]            condition expected: b
//	[ a b c d -o ]          condition expected: a
//	[ ! a b ]               parse error: condition expected: a
//	[ ! a b c ]             condition expected: b
//	[ ! -n a b ]            too many arguments
//	[ -n a -o b c d ]       condition expected: c
//	[ x -a y -a z w ]       parse error: condition expected: z
//
// **The last two rows of the first block are what a left-to-right parse gets
// wrong, and they are why this is a count.** `[ a b c -a ]` names `b` and
// `[ a b c d -o ]` — the same shape one word longer — names `a`. A reader that
// stopped where the words stopped making sense would name the same word in
// both; a reader that counts the words in front of the connective names word 2
// of a three-word segment and word 1 of a four-word one, which is exactly what
// those two rows say.
//
// The rule was then **predicted onto twenty shapes it had not been fitted to**
// — among them `a b -o c d`, `-n a -o b c d`, `a b c -o`, `a -a b c d`,
// `x -a y -a z w`, `a b -a c d -o e f`, `-n a -a b c -o d e` and
// `a = b -a c d e` — and it is right in every one of them.
//
// # What it does not reach
//
// The `parse error: ` prefix belongs to the two-word sentence and nothing
// else, which is a property of that column's wording rather than of this
// reading: it is on every two-word failure, top level or inside a segment,
// and off the three-word and longer ones. So it is one string in
// [Diagnostics.TestTwoWordOperatorExpected] and no rule here.
//
// A segment this reading finds nothing wrong with leaves the refusal exactly
// as the ordinary reader made it — an operand a numeric comparison could not
// read, an operator with nothing behind it, a group that never closed. Those
// are failures of *evaluation* rather than of arity and the column words them
// elsewhere.

// countedSegmentRefusal answers the complaint one column makes about a refused
// operand list, or nil where it finds nothing to complain about.
func (r *Runner) countedSegmentRefusal(form testForm, args []string) error {
	for _, seg := range splitAtConnectives(form, args) {
		if err := r.countedSegmentComplaint(seg); err != nil {
			return err
		}
	}
	return nil
}

// splitAtConnectives breaks the operands at every connective. There is no
// grouping to be at the top *of*: that column's `[` has no parentheses — `[ ( a
// ) ]` there is `unknown file attribute:` at 1, measured 2026-09-27 — so every
// connective in the list is one of these.
func splitAtConnectives(form testForm, args []string) [][]string {
	var out [][]string
	start := 0
	for i, w := range args {
		if w == form.and || w == form.or {
			out = append(out, args[start:i])
			start = i + 1
		}
	}
	return append(out, args[start:])
}

// countedSegmentComplaint is the per-segment half of the rule above.
func (r *Runner) countedSegmentComplaint(seg []string) error {
	// A leading `!` is off before the words are counted, which is what makes
	// `[ ! a b ]` the two-word shape's sentence and `[ ! a b c ]` the
	// three-word shape's.
	for len(seg) > 0 && seg[0] == "!" {
		seg = seg[1:]
	}
	switch len(seg) {
	case 0, 1:
		// An empty segment is what a trailing connective leaves and is not
		// complained about: `[ a b c -a ]` is the *left* segment's sentence.
		return nil
	case 2:
		if r.isTestUnary(seg[0]) {
			return nil
		}
		if spelledLikeATestOperator(seg[0]) {
			return &testError{kind: errUnaryExpected, operand: seg[0]}
		}
		return &testError{kind: errTwoWordOperatorExpected, operand: seg[0]}
	case 3:
		if r.isTestUnary(seg[0]) {
			// The operator took its operand and the third word is one too
			// many, which is the row where word 1 beats the binary reading.
			return &testError{kind: errTooManyArguments, operand: seg[2]}
		}
		if r.hasTestBinaryOperator(seg[1]) {
			return nil
		}
		if spelledLikeATestOperator(seg[1]) {
			return &testError{kind: errUnaryExpected, operand: seg[1]}
		}
		return &testError{kind: errBinaryExpected, operand: seg[1]}
	default:
		if r.isTestUnary(seg[0]) || r.hasTestBinaryOperator(seg[1]) {
			// A primary came off the front and the rest is left over.
			return &testError{kind: errTooManyArguments, operand: seg[len(seg)-1]}
		}
		if spelledLikeATestOperator(seg[0]) {
			return &testError{kind: errUnaryExpected, operand: seg[0]}
		}
		// No primary at all, so the complaint is about where the segment
		// *starts* rather than about where it stopped making sense.
		return &testError{kind: errBinaryExpected, operand: seg[0]}
	}
}

// spelledLikeATestOperator reports a word the shell would have named as an
// operator it has not got: a `-` with something after it.
//
// The spelling and not membership of the table, which is the same test
// Diagnostics.TestLeftoverOperator is measured to make — `[ -q a ]` and
// `[ -Q a ]` are both `unknown condition` in that column, and neither letter
// is an operator anywhere.
func spelledLikeATestOperator(word string) bool {
	return len(word) > 1 && strings.HasPrefix(word, "-")
}

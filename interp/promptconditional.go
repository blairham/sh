// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strconv"

// The conditional prompt escape: `%(x.true.false)`, a question about the
// shell and two texts to choose between.
//
// It is the one shape in the prompt language that is not a value — every
// other code draws something, and this one *decides*. So it is not a row of
// [PromptStyle.Codes]: the walker reads it, and what a dialect supplies is
// which character opens it, which closes it, and what each test letter asks.
//
// The construct, measured against zsh 5.9.2 one piece at a time:
//
//	%(?.yes.no)      a test letter, then two arms
//	%2(?.yes.no)     a count in front of the escape
//	%(2?.yes.no)     or inside the parentheses, where it wins: measured,
//	                 `%2(3?.T.F)` is true at status 3 and false at status 2
//	%(?:yes:no)      the delimiter is whatever character follows the letter
//	%(?.a%(j.b.c)d.e)  arms nest, and hold further escapes
//
// Three edges that a reading which "looks for the next delimiter" gets wrong,
// and each is measured:
//
//   - The **true** arm ends at the delimiter and the **false** arm ends at
//     the closing parenthesis, not at another delimiter: `%(1?.T.F.G)` draws
//     `F.G`.
//   - A nested construct is skipped whole. `%(?.a%(1?.).Y)b.c)` draws `aYb`,
//     so neither the `.` inside the nested arm nor the `)` inside it ended
//     anything belonging to the outer one — counting parentheses is not
//     enough, and the skip has to be the same parse.
//   - An escape inside an arm hides the character after it: with `.` as the
//     delimiter, `%(1?.a.b%.c)` draws the working directory between `b` and
//     `c`, so `%.` was a code and not the end of the arm.
//
// A letter the dialect has no test for is not a refusal. Measured, zsh draws
// nothing at all for `%(a.T.F)` and swallows both arms — `%(a.T.F)X` is `X` —
// so an unknown letter is the table saying this shell's prompt language has
// no such question, and the paired half is a letter that *is* in the table
// and that this reader cannot answer, which is refused by name.

// PromptCondition is one question a conditional escape asks.
//
// A named question rather than a function for the reason [PromptField] is
// one: a dialect's table stays data, and how a shell finds out its own group
// id is not a thing a dialect package has to know.
//
// **How the count is compared is part of what each of these means**, and it
// is measured per condition rather than shared. Some are an equality —
// `%(?.…)` is true when the last status *is* n, so `%2(?.…)` is false at
// status 5 — and some are a floor: `%2(j.…)` is true with two jobs or more.
// The two are not interchangeable and nothing but measurement separates them.
//
// The sign is nothing to any of them but [ConditionColumn]: measured,
// `%-1(?.T.F)` is true at status 1, `%-5(v.…)` asks for five elements, and
// only the column reads a negative count as a question about the space left.
type PromptCondition int

const (
	// ConditionNone is no question at all, and the zero value so that a
	// letter missing from a table cannot quietly become one.
	ConditionNone PromptCondition = iota
	// ConditionExitStatus is the status of the last command, and true when
	// it *equals* the count: measured, `%(?.T.F)` is `T` after a success and
	// `%2(?.T.F)` is `T` only after a command that exited 2.
	ConditionExitStatus
	// ConditionJobs is how many jobs the shell is looking after, and true at
	// the count or above.
	ConditionJobs
	// ConditionEffectiveUser and ConditionEffectiveGroup are the effective
	// ids, each true when it equals the count. Measured with uid 501 and gid
	// 20: `%501(#.T.F)` and `%20(g.T.F)` are the only counts that answer
	// `T`, and the default count of nought is what makes a bare `%(#.…)`
	// the test for root.
	ConditionEffectiveUser
	ConditionEffectiveGroup
	// ConditionPrivileged is whether the shell is running with privileges,
	// and it is the one test that ignores the count entirely: measured,
	// `%501(!.T.F)` is `F` where `%501(#.T.F)` is `T`.
	ConditionPrivileged
	// ConditionShellLevel is how deep this shell is nested in others, true
	// at the count or above. Measured against SHLVL 5, which answered every
	// count up to and including five.
	ConditionShellLevel
	// ConditionEvalDepth is how many function calls and evals enclose the
	// expansion, and ConditionOpenConstructs how many constructs the parser
	// is still inside. Both are true at the count or above. Measured, the
	// first is nought at a script's top level and inside a sourced file, one
	// inside a function, and two inside a function called from another; the
	// second is nought everywhere a script can reach, which is the same
	// thing FieldOpenState says when a script asks it.
	ConditionEvalDepth
	ConditionOpenConstructs
	// ConditionColumn is how many columns have already been drawn on this
	// line — see promptWalk.column, which is the only condition the walker
	// answers itself, because it is the only one that is a fact about the
	// prompt being drawn rather than about the shell drawing it.
	ConditionColumn
	// ConditionLineWidth is what the line wraps at, and is *not* a test
	// letter: no dialect maps a letter to it and no script can ask it. The
	// walker asks it, because the column above cannot be counted without it,
	// and the two readers answer it differently — a Runner reads `COLUMNS`,
	// which is what powerlevel10k sets to 1024 before it measures a prompt.
	ConditionLineWidth
	// ConditionSeconds is how long the shell has been running, true at the
	// count or above.
	ConditionSeconds
	// ConditionPromptArrayCount is how many elements the prompt array holds
	// and is true at the count or above; ConditionPromptArrayElement is
	// whether the element the count names is set and not empty. Measured
	// with `psvar=(a b '')`: the first answered counts up to three, and the
	// second answered for one and two and not for three, since the third
	// element is empty. A count of nought names the first element, which is
	// what makes a bare `%(V.…)` a question about `$psvar[1]`.
	ConditionPromptArrayCount
	ConditionPromptArrayElement
	// ConditionCwdComponents is how many components the working directory
	// has and ConditionCwdComponentsHome how many it has with the home
	// directory written `~`. Both are true at the count or above.
	ConditionCwdComponents
	ConditionCwdComponentsHome
	// The clock, each true when it *equals* the count. Measured on the tenth
	// of September 2026 at 11:09 on a Thursday: the month answered 8 and not
	// 9 — it is the count of months already gone rather than the month's
	// number — the day answered 10, the hour 11, the minute 9, and the day
	// of the week 4 with Sunday as nought.
	ConditionMonth
	ConditionDayOfMonth
	ConditionHour
	ConditionMinute
	ConditionDayOfWeek
)

// PromptQuantityResolver is what one condition counts, for the reader that
// holds the facts.
//
// A number rather than the answer, so that the *comparison* stays with the
// condition's meaning — an equality for the status and a floor for the jobs —
// and a reader that knows the shell's group id does not also have to know
// which of the two zsh spells `%(g.…)` with.
//
// n is the count the escape carried, and is read only by the conditions whose
// meaning needs it before the comparison: [ConditionPromptArrayElement] names
// an element with it. Everything else may ignore it.
//
// The second result says whether this reader has an answer at all, and false
// is the by-name refusal's condition — the same split [PromptResolver] makes,
// for the same reason. A drawer answers everything; a script's expansion says
// which test it could not answer and stops.
type PromptQuantityResolver func(c PromptCondition, n int) (int, bool)

// conditional reads one `%(…)` construct and draws the arm it chooses.
//
// i is the index of the character that opened it and num the count that stood
// in front of the escape. The result is the index of the last character the
// construct consumed, so the walk resumes after it whether or not anything
// was drawn.
func (w *promptWalk) conditional(runes []rune, i int, num string) int {
	j := i + 1
	if k := skipDigits(runes, j); k > j {
		// Digits inside the parentheses are the count and they beat any in
		// front of the escape: measured, `%2(3?.T.F)` answers `T` at status
		// 3 and `F` at status 2.
		num, j = string(runes[j:k]), k
	}
	if j >= len(runes) {
		return len(runes)
	}
	test := runes[j]
	j++
	if j >= len(runes) {
		return len(runes)
	}
	delim := runes[j]
	yes, next, hasDelim := w.scanArm(runes, j+1, delim)
	var no []rune
	end := next
	if hasDelim {
		// The false arm runs to the closing character rather than to another
		// delimiter: measured, `%(1?.T.F.G)` draws `F.G`.
		no, end, _ = w.scanArm(runes, next, w.st.ConditionalEnd)
	}
	cond, known := w.st.Conditions[test]
	if !known {
		// A letter this dialect has no test for. Measured, zsh draws nothing
		// and swallows the whole construct — `%(a.T.F)X` is `X` — which is
		// the table saying there is no such question rather than this shell
		// saying it has no answer. The paired half is a letter that *is* in
		// the table and that this reader cannot answer, below.
		return end - 1
	}
	on, answered := w.conditionHolds(cond, promptCount(num))
	if !answered {
		w.refused = string(w.st.Conditional) + string(test)
		return end - 1
	}
	if on {
		w.walk(yes)
	} else {
		w.walk(no)
	}
	return end - 1
}

// scanArm reads one arm of a conditional and says where it ended.
//
// stop is the character that ends it — the delimiter for the true arm and the
// closing parenthesis for the false one. The second result is the index just
// past it, and the third says whether it was found at all: an arm that runs
// off the end of the text is the rest of the text, which is measured rather
// than lenient — `%(?.T` draws `T`.
//
// An escape hides whatever it introduces, including a whole nested construct,
// which is what keeps `%(?.a%(1?.).Y)b.c)` one conditional and not two.
func (w *promptWalk) scanArm(runes []rune, i int, stop rune) ([]rune, int, bool) {
	start := i
	for i < len(runes) {
		switch runes[i] {
		case stop:
			return runes[start:i], i + 1, true
		case w.st.Escape:
			i = w.skipEscape(runes, i)
		default:
			i++
		}
	}
	return runes[start:], i, false
}

// skipEscape says where the escape at i ends, without drawing any of it.
//
// The count, the code, and — where the code opens a conditional — the whole
// of that conditional, read by the same rules the outer one is. Anything
// less counts characters that belong to a nested arm as the outer arm's.
func (w *promptWalk) skipEscape(runes []rune, i int) int {
	if i+1 >= len(runes) {
		return len(runes)
	}
	_, j := w.countAt(runes, i+1)
	if j >= len(runes) {
		return len(runes)
	}
	code := runes[j]
	j++
	if code != w.st.Conditional || w.st.Conditional == 0 {
		return j
	}
	j = skipDigits(runes, j)
	if j >= len(runes) {
		return len(runes)
	}
	// The test letter, then the delimiter it chose.
	j++
	if j >= len(runes) {
		return len(runes)
	}
	delim := runes[j]
	_, j, hasDelim := w.scanArm(runes, j+1, delim)
	if hasDelim {
		_, j, _ = w.scanArm(runes, j, w.st.ConditionalEnd)
	}
	return j
}

// countAt reads the count in front of a code and says where the code begins.
//
// One reader rather than two, because the walk and the skip above have to
// agree about where a code starts: an arm scanned by rules the walk does not
// share is an arm that ends in the wrong place, and nothing about the drawn
// text says which of the two was wrong.
//
// **A minus is read only in front of the code that has an answer for one.**
// zsh takes it for every code that counts — measured, `%-2~` is the *leading*
// two components of the working directory where `%2~` is the trailing two —
// and this shell carries it for the conditional alone. Every other code still
// refuses `%-` by name, which is #1699, and a plausible wrong answer in place
// of that refusal would be worse than the gap.
func (w *promptWalk) countAt(runes []rune, i int) (string, int) {
	if !w.st.NumericArgument || i >= len(runes) {
		return "", i
	}
	j := i
	signed := runes[j] == '-'
	if signed {
		j++
	}
	j = skipDigits(runes, j)
	if signed && (w.st.Conditional == 0 || j >= len(runes) || runes[j] != w.st.Conditional) {
		return "", i
	}
	return string(runes[i:j]), j
}

// skipDigits says where the run of digits at i ends.
func skipDigits(runes []rune, i int) int {
	for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
		i++
	}
	return i
}

// conditionHolds is the comparison each condition means, over the number the
// reader answered with.
//
// Which comparison is measured per condition and recorded on [PromptCondition]:
// an equality for the status, the ids and the clock, a floor for the counts,
// and the truth of the number itself where the count has already been spent
// naming what to look at.
func (w *promptWalk) conditionHolds(c PromptCondition, n int) (bool, bool) {
	if c == ConditionColumn {
		return w.column(n), true
	}
	if w.quantity == nil {
		return false, false
	}
	v, answered := w.quantity(c, n)
	if !answered {
		return false, false
	}
	switch c {
	case ConditionPrivileged, ConditionPromptArrayElement:
		return v != 0, true
	}
	if n < 0 {
		// The sign is nothing to any test but the column: measured,
		// `%-1(?.T.F)` answers where `%1(?.T.F)` answers.
		n = -n
	}
	switch c {
	case ConditionExitStatus, ConditionEffectiveUser, ConditionEffectiveGroup,
		ConditionMonth, ConditionDayOfMonth, ConditionHour, ConditionMinute,
		ConditionDayOfWeek:
		return v == n, true
	}
	return v >= n, true
}

// column is `%(l.…)`: whether at least n columns have been drawn on this
// line, and — with a negative count — whether at least that many are left.
//
// Answered here rather than by the reader because it is a fact about the
// prompt being drawn and not about the shell drawing it. Measured with
// `COLUMNS=10` and three columns drawn: every count up to three answers, and
// so does every negative count down to seven, which is what the line has
// left.
//
// A width below nought is not a line at all, and is measured rather than
// guarded against: with `COLUMNS=-1`, every count at or above nought answers
// `F` — including nought itself, which no column can be short of — and every
// count below it answers `T`, whatever has been drawn.
func (w *promptWalk) column(n int) bool {
	width := w.lineWidth()
	if width < 0 {
		return n < 0
	}
	if n < 0 {
		return width-w.col >= -n
	}
	return w.col >= n
}

// lineWidth is what the line wraps at, asked of the reader once.
//
// Lazily, so a prompt with no conditional in it never asks: the question
// costs a variable read, and a prompt is drawn again on every keystroke.
// A reader with no answer is a line that does not wrap, which is the same
// answer a reader that reports nought gives — measured, `COLUMNS=0` and
// `COLUMNS` unset behave alike.
func (w *promptWalk) lineWidth() int {
	if w.width == unaskedWidth {
		w.width = 0
		if w.quantity != nil {
			if v, ok := w.quantity(ConditionLineWidth, 0); ok {
				w.width = v
			}
		}
	}
	return w.width
}

// cell moves the column on by what one character costs, wrapping where the
// line runs out.
//
// The rule is measured across widths one to six with wide characters at every
// position, and it is two wraps rather than one. A character that will not
// *fit* starts a new line before it is drawn, which is why `日日日` at width
// five leaves the column at two rather than at nought; and a line filled
// exactly wraps after, which is why `x日` at width three leaves it at nought.
// The second wrap is skipped for a character wider than the whole line, which
// is the only thing that explains `日` at width one leaving the column at two.
//
// What a character costs is not what the line editor counts, and the
// difference is measured: a control character occupies no cell of an edited
// line and counts as one column here — `\e[31m` written literally is five —
// while a combining mark is nought in both and an East Asian wide character
// is two. A tab reaches the next multiple of eight.
//
// The per-character part of that is displayWidth, shared with the `(m)`
// expansion flag, which measures the same three classes the same way. The tab
// is this reader's own: it is a fact about where the column already stands,
// which a length has no answer for.
func (w *promptWalk) cell(r rune) {
	if r == '\n' {
		w.col = 0
		return
	}
	width := w.lineWidth()
	d := displayWidth(r)
	if r == '\t' {
		d = 8 - w.col%8
	}
	if w.col+d > width {
		w.col = 0
	}
	w.col += d
	if d > 0 && d <= width && w.col >= width {
		w.col = 0
	}
}

// promptCount reads the count an escape carried.
//
// Nothing is nought, which is every test's default and is measured: a bare
// `%(?.…)` asks about status nought and a bare `%(l.…)` about a column that
// nothing can be short of.
//
// A minus with no digits after it is minus one rather than nought: measured,
// `%-(?.T.F)` answers exactly where `%-1(?.T.F)` does, and `%-~` draws what
// `%-1~` draws.
//
// A run of digits too long to be a number is the largest one, where zsh warns
// and truncates the run to nineteen digits. Recorded rather than matched: the
// warning names a line and a function and this walker knows neither, and a
// count that large is false for every test either way.
func promptCount(arg string) int {
	if arg == "" {
		return 0
	}
	neg := arg[0] == '-'
	if neg {
		arg = arg[1:]
	}
	if arg == "" {
		if neg {
			return -1
		}
		return 0
	}
	n, _ := strconv.Atoi(arg)
	if neg {
		return -n
	}
	return n
}

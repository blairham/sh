// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
)

// `zformat` is this shell's string formatter: `%c` substitution with a field
// width and a ternary, and a second job aligning a list into columns.
//
// Measured 2026-09-06 against zsh 5.9.2 with a scratch HOME and no startup
// files, alongside the module's manual page, which documents every form.
//
// Three forms and they share almost nothing:
//
//	zformat -f param format spec …   `%c` substitution
//	zformat -F param format spec …   the same, with a presence test
//	zformat -a array sep spec …      column alignment
//
// The argument model is worth stating because it is the one thing a reading
// of the synopsis gets wrong. **Exactly one option word is read**, and
// everything after it is a positional parameter however it is spelled:
// measured, `zformat -a -f R x` names `-f` as the array to assign — `not an
// identifier: -f` — rather than taking it as a second letter, and `zformat -f
// -F R "%(c.y.n)" c:` complains about `%(c.y.n)` as a *specification*, which
// only happens if `-F` became the parameter name and `R` the format. So the
// letters are not stackable and not repeatable, and a second one is data.

// zformatMode is which of the three jobs was asked for.
type zformatMode uint8

const (
	// zformatNone is no leading option word at all, which is not a usage
	// error in itself: every argument is then read as a specification, and
	// the first one that is not `c:string` names itself. Measured — `zformat
	// R "%c" c:1` is `invalid argument: R`.
	zformatNone zformatMode = iota
	zformatSubst
	zformatPresence
	zformatAlign
)

// zformatOutcome is why a format string stopped being read, which is two
// different things wearing one shape: the *format* was unreadable, or an
// expression inside it would not evaluate. They need different answers,
// because the second has already reported itself and made the shell fatal —
// a `malformed format string` on top of a `division by zero` would blame the
// format for the expression's failure.
type zformatOutcome uint8

const (
	zformatRead zformatOutcome = iota
	zformatUnreadable
	zformatUnevaluatable
)

func registerZformat(r *interp.Runner) {
	r.Register("zformat", zformatBuiltin)
}

// zformatMinimumArguments is how many words this builtin needs before it will
// look at any of them.
//
// Three, measured against every shape: `zformat`, `zformat a`, `zformat -f R`
// and `zformat -a A` are all `not enough arguments`, and `zformat -a A -` —
// three words, no specifications — is 0 and an empty array. So the count is
// checked before the letters, which is why a two-word call never reaches a
// complaint about its letter.
const zformatMinimumArguments = 3

func zformatBuiltin(r *interp.Runner, _ context.Context, args []string) int {
	if len(args) < zformatMinimumArguments {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	if args[0] == "--" {
		args = args[1:]
	}
	mode, rest, code := zformatOption(r, args)
	if code != 0 {
		return code
	}
	switch mode {
	case zformatAlign:
		return zformatAlignInto(r, rest[0], rest[1], rest[2:])
	case zformatSubst, zformatPresence:
		return zformatSubstInto(r, mode, rest[0], rest[1], rest[2:])
	}
	// No mode: nothing to assign, and the arguments are still graded as
	// specifications so that a mistyped letter names itself.
	if _, code := zformatSpecs(r, rest); code != 0 {
		return code
	}
	return 0
}

// zformatOption reads the one leading option word.
func zformatOption(r *interp.Runner, args []string) (zformatMode, []string, int) {
	switch args[0] {
	case "-f":
		return zformatSubst, args[1:], 0
	case "-F":
		return zformatPresence, args[1:], 0
	case "-a":
		return zformatAlign, args[1:], 0
	}
	if len(args[0]) == 2 && args[0][0] == '-' {
		// A single unknown letter is a bad letter; anything longer is data,
		// which is what makes `-fa` `invalid argument: -fa` rather than a
		// complaint about a letter.
		r.Diagnosef("invalid option: %s\n", args[0])
		return zformatNone, nil, 1
	}
	return zformatNone, args, 0
}

// zformatSpecs reads the `char:string` specifications of `-f` and `-F`.
//
// The character is one **byte** and not one rune, measured: `zformat -f R
// "%é" "é:x"` is `invalid argument: é:x`, so a multibyte character is not a
// specifier however it is written.
func zformatSpecs(r *interp.Runner, words []string) (map[byte]string, int) {
	table := map[byte]string{}
	for _, w := range words {
		if len(w) < 2 || w[1] != ':' {
			r.Diagnosef("invalid argument: %s\n", w)
			return nil, 1
		}
		// A repeated specifier is the last one, measured.
		table[w[0]] = w[2:]
	}
	return table, 0
}

// zformatSubstInto is `-f` and `-F`: substitute, then assign.
func zformatSubstInto(r *interp.Runner, mode zformatMode, param, format string, words []string) int {
	table, code := zformatSpecs(r, words)
	if code != 0 {
		return code
	}
	out, why := zformatExpand(r, mode, format, table)
	if why == zformatUnreadable {
		r.Diagnosef("malformed format string\n")
		return 1
	}
	if why != zformatRead {
		// An expression inside the format would not evaluate. The shell has
		// already reported that and stopped, and saying `malformed` on top of
		// `division by zero` would blame the format for the expression's
		// failure.
		//
		// And the status is 0, which is what the shell then exits with:
		// measured on zsh 5.9.2, `false; zformat -f R "%0(c.y.n)" c:1/0`
		// stops the script at 0, where the same division in `$(( ))` stops
		// it at 1. The script ends on the builtin's own answer, and the
		// builtin's answer is that it did not fail. The evaluation has
		// already written the fatal status, so it is written back here.
		r.SetExitStatus(0)
		return 0
	}
	if !isStoreOperandName(param) {
		r.DiagnoseAsTheShellf("not an identifier: %s\n", param)
		return 1
	}
	r.SetOperandVar(param, out)
	return 0
}

// zformatExpand walks a format string.
//
// A `%` sequence this shell does not recognize is left exactly as written —
// measured, `a%xb` comes back `a%xb` and so does `%5x`, the width included.
// That is the honest answer for a formatter whose specifier set is supplied by
// the caller: a `%` that names no specification is not an error and not an
// empty string, it is text.
func zformatExpand(
	r *interp.Runner, mode zformatMode, format string, table map[byte]string,
) (string, zformatOutcome) {
	var out strings.Builder
	for i := 0; i < len(format); {
		c := format[i]
		if c != '%' {
			out.WriteByte(c)
			i++
			continue
		}
		text, next, ok, why := zformatEscape(r, mode, format, i, table)
		if why != zformatRead {
			return "", why
		}
		if !ok {
			// Not a sequence this shell reads, the trailing `%` of a string
			// included. Written back as it stands.
			out.WriteByte(c)
			i++
			continue
		}
		out.WriteString(text)
		i = next
	}
	return out.String(), zformatRead
}

// zformatEscape reads one `%` sequence, starting at the `%`.
func zformatEscape(
	r *interp.Runner, mode zformatMode, format string, at int, table map[byte]string,
) (text string, next int, ok bool, why zformatOutcome) {
	i := at + 1
	number, negative, i, hasNumber := zformatWidthNumber(format, i)
	if i < len(format) && format[i] == '(' {
		text, next, why = zformatTernary(r, mode, format, i+1, number, negative, hasNumber, table)
		return text, next, why == zformatRead, why
	}
	// Not a ternary, so the leading number was a field width after all, and
	// a maximum may follow it.
	minWidth := number
	maxWidth, i, hasMax := zformatMaxWidth(format, i)
	if i >= len(format) {
		return "", 0, false, zformatRead
	}
	specifier := format[i]
	if specifier == '%' {
		return zformatPad("%", minWidth, maxWidth, hasMax), i + 1, true, zformatRead
	}
	value, held := table[specifier]
	if !held {
		return "", 0, false, zformatRead
	}
	return zformatPad(value, minWidth, maxWidth, hasMax), i + 1, true, zformatRead
}

// zformatWidthNumber reads the number in front of a `%` escape — a field
// width, or a ternary's test number when a `(` follows it.
//
// It differs from the signed reader in one case, and that case was three of
// this builtin's five disagreements with zsh 5.9.2: **a lone `-` with no
// digits is consumed and reports no number.** The signed reader rewinds
// instead, which leaves the `-` to be read as the specifier letter, and the
// whole spec is then copied out literally — so `%-s`, `%-.s`, `%-.2s` and
// `%-.0s` were all literal text here and are all valid to the reference.
//
// Reporting **no** number rather than zero is the part that needs measuring
// rather than assuming, because the same read feeds a ternary: `%-(c.y.n)`
// behaves exactly as `%(c.y.n)`, so the sign without digits must not arrive
// as a test number of 0 either. The two are indistinguishable for a width,
// where an absent minimum and a minimum of zero both pad nothing.
//
// One sign, leading, and only `-`: the reference emits `%--2s`, `%- s`,
// `%+2s` and `%5-s` literally.
func zformatWidthNumber(format string, at int) (n int, negative bool, next int, has bool) {
	if n, negative, next, has = zformatNumber(format, at); has {
		return n, negative, next, true
	}
	if at < len(format) && format[at] == '-' {
		return 0, true, at + 1, false
	}
	return 0, false, at, false
}

// zformatNumber reads an optionally negative decimal run.
//
// It reports the sign **separately** from the value, because `-0` and `0` are
// the same integer and this builtin tells them apart: under `-F` the sign
// inverts the comparison, so `%0(a.a.A)` asks whether the value is longer
// than nothing and `%-0(a.a.A)` asks whether it is empty. Deriving the sign
// from `n < 0` instead works for every other number and silently fails for
// this one.
func zformatNumber(format string, at int) (n int, negative bool, next int, has bool) {
	i := at
	if i < len(format) && format[i] == '-' {
		negative = true
		i++
	}
	start := i
	for i < len(format) && format[i] >= '0' && format[i] <= '9' {
		i++
	}
	if i == start {
		return 0, false, at, false
	}
	n, _ = strconv.Atoi(format[start:i])
	if negative {
		n = -n
	}
	return n, negative, i, true
}

// zformatMaxWidth reads the `.max` half of a `%min.maxc` width.
//
// Two shapes measured against zsh 5.9.2 go the opposite way from the obvious
// reading, and a fix for either one does not imply the other:
//
//   - A `.` with no digits is a precision that is **absent, not zero**.
//     `%5.s` behaves exactly as `%5s` and `%.s` as `%s`, while `%.0s` yields
//     the empty string — which is what tells the two readings apart. So the
//     dot is consumed and no maximum is reported, where rewinding past it
//     left the specifier letter unfindable and the whole spec was copied out
//     literally.
//   - A **negative** precision is not a precision. `%5.-2s` is emitted
//     literally by the reference, so the run read here is unsigned. The
//     minimum is not: `%-5s` is valid and pads on the left, so the signed
//     reader stays in use there and must not be shared with this half.
func zformatMaxWidth(format string, at int) (max, next int, has bool) {
	if at >= len(format) || format[at] != '.' {
		return 0, at, false
	}
	i := at + 1
	start := i
	for i < len(format) && format[i] >= '0' && format[i] <= '9' {
		i++
	}
	if i == start {
		return 0, at + 1, false
	}
	max, _ = strconv.Atoi(format[start:i])
	return max, i, true
}

// zformatPad applies a field width. A minimum pads — to the right, or to the
// left when it is negative — and a maximum truncates.
func zformatPad(s string, minWidth, maxWidth int, hasMax bool) string {
	if hasMax && maxWidth >= 0 && len(s) > maxWidth {
		s = s[:maxWidth]
	}
	switch {
	case minWidth > len(s):
		return s + strings.Repeat(" ", minWidth-len(s))
	case -minWidth > len(s):
		return strings.Repeat(" ", -minWidth-len(s)) + s
	}
	return s
}

// zformatTernary reads `%[n](c<d>true<d>false)`, starting after the `(`.
//
// The test number may be written on either side of the parenthesis, and when
// it is written on both the one **before** it wins — measured, `%1(2c.y.n)`
// with `c:2` chooses the false text, which only happens if the test is 1.
func zformatTernary(
	r *interp.Runner, mode zformatMode, format string, at, number int,
	negative, hasNumber bool, table map[byte]string,
) (text string, next int, why zformatOutcome) {
	i := at
	if inner, innerNegative, after, has := zformatNumber(format, i); has {
		if !hasNumber {
			number = inner
			if negative && !innerNegative {
				// The sign was written **outside** the parenthesis and the
				// number inside it, and the sign still applies. Measured,
				// `%-(5j.short.long)` chooses `short` for a value of length
				// five and `long` for one of length six — the `<=` reading —
				// where dropping the sign inverts it. It reaches the
				// arithmetic mode too: `%-(2c.y.n)` with `c:2` is `n`,
				// because the number under test is -2 and not 2.
				number = -inner
			}
			negative = negative || innerNegative
		}
		i = after
	}
	if i+1 >= len(format) {
		return "", 0, zformatUnreadable
	}
	specifier := format[i]
	delimiter := format[i+1]
	trueText, i, ok := zformatUntil(format, i+2, delimiter)
	if !ok {
		return "", 0, zformatUnreadable
	}
	falseText, i, ok := zformatUntil(format, i, ')')
	if !ok {
		return "", 0, zformatUnreadable
	}
	test, evaluated := zformatTest(r, mode, table, specifier, number, negative)
	if !evaluated {
		// The expression would not evaluate, and the shell has already said
		// so and stopped. Nothing is written.
		return "", 0, zformatUnevaluatable
	}
	chosen := falseText
	if test {
		chosen = trueText
	}
	// Either half may hold `%` sequences of its own.
	out, why := zformatExpand(r, mode, chosen, table)
	if why != zformatRead {
		return "", 0, why
	}
	return out, i, zformatRead
}

// zformatUntil reads up to an unescaped terminator, where `%` escapes the
// next character — which is how a `)` is written inside the false text.
func zformatUntil(format string, at int, terminator byte) (text string, next int, ok bool) {
	var out strings.Builder
	for i := at; i < len(format); i++ {
		switch {
		case format[i] == '%' && i+1 < len(format) && format[i+1] == terminator:
			out.WriteByte(terminator)
			i++
		case format[i] == '%' && i+1 < len(format) && format[i+1] == '(':
			group, after, closed := zformatGroup(format, i)
			if !closed {
				return "", 0, false
			}
			out.WriteString(group)
			i = after - 1
		case format[i] == terminator:
			return out.String(), i + 1, true
		default:
			out.WriteByte(format[i])
		}
	}
	return "", 0, false
}

// zformatGroup copies a `%(`…`)` group whole, starting at its `%`, so that a
// nested ternary's delimiters are never read as the enclosing half's.
//
// Only `%(` opens a group. A bare `(` is ordinary text — measured,
// `%(8n.a(b.c).d)` is `a(b.d)`, which is the true half `a(b` followed by the
// three characters left over after the false half — so counting every paren
// would change a case that already agrees.
//
// `%` takes the next byte with it whatever it is, which is what keeps a `%)`
// written inside a group from closing it early. The group's *content* is not
// interpreted here; it is expanded later by whichever half is chosen, and
// only then does an unreadable escape inside it matter.
func zformatGroup(format string, at int) (group string, next int, closed bool) {
	depth := 0
	for i := at; i < len(format); i++ {
		if format[i] == '%' && i+1 < len(format) {
			if format[i+1] == '(' {
				depth++
			}
			i++
			continue
		}
		if format[i] == ')' {
			depth--
			if depth == 0 {
				return format[at : i+1], i + 1, true
			}
		}
	}
	return "", 0, false
}

// zformatTest is which half of a ternary is chosen, and it is the whole
// difference between `-f` and `-F`.
//
// `-f` reads the specifier's value as an **arithmetic expression** and asks
// whether it equals the test number — measured, `%2(c.y.n)` with `c:1+1`
// chooses the true text and with `c:x` chooses the false one, so it is the
// shell's own evaluation and not a number parse.
//
// `-F` asks about the value's **width** instead: true when it is longer than
// the test number, and a negative test number reverses the question into
// "no longer than". Measured at the boundary in both directions — with `c:abc`
// and a test of 3 the answer is false, and with `c:abcd` it is true — because
// a strict comparison and a loose one differ nowhere else.
func zformatTest(
	r *interp.Runner, mode zformatMode, table map[byte]string, specifier byte, number int,
	negative bool,
) (test, evaluated bool) {
	value := table[specifier]
	if mode == zformatPresence {
		if negative {
			return len(value) <= -number, true
		}
		return len(value) > number, true
	}
	n, ok := r.ArithValue(value)
	return n == number, ok
}

// zformatAlignInto is `-a`: lay a list of `left:right` pairs out in columns.
//
// The rule that is easy to miss is which strings *count*. A string with no
// colon is left alone and a string whose right half is empty loses its colon,
// and **neither takes part in deciding the column** — measured, `foo:` beside
// `x:y` puts the separator one character in, at `x`'s width, not four at
// `foo`'s.
func zformatAlignInto(r *interp.Runner, array, separator string, words []string) int {
	type row struct {
		left, right string
		aligned     bool
		text        string
	}
	rows := make([]row, 0, len(words))
	width := 0
	for _, w := range words {
		left, right, split := zformatSplitPair(w)
		switch {
		case !split, right == "":
			// Neither shape takes part in deciding the column, and both are
			// written back as their left half — which is not the same as the
			// string as given: `a\:b` with no unescaped colon in it comes
			// back `a:b`, so the escape is undone even where nothing split.
			rows = append(rows, row{text: left})
		default:
			rows = append(rows, row{left: left, right: right, aligned: true})
			if len(left) > width {
				width = len(left)
			}
		}
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if !row.aligned {
			out = append(out, row.text)
			continue
		}
		out = append(out, row.left+strings.Repeat(" ", width-len(row.left))+separator+row.right)
	}
	if !isIdentifier(array) {
		r.DiagnoseAsTheShellf("not an identifier: %s\n", array)
		return 1
	}
	r.SetArray(array, out)
	return 0
}

// zformatSplitPair reads one `left:right` string.
//
// A backslash escapes **whatever follows it**, not only a colon: measured
// against zsh 5.9.2, `a\\b` is `a\b` and `a\qb` is `aqb`, so the rule is
// that the backslash is dropped and the next byte is taken literally. Reading
// `\:` as the only escape left `a\\b` as `a\\b` and made a doubled
// backslash look deliberate.
//
// A lone backslash at the very end has nothing to escape and stays: `end\` is
// `end\`.
//
// **Only the left half is unescaped.** The right half is returned exactly as
// written, which is measurable and not an oversight: `x:y\\z` comes back
// `x.y\\z` with the separator applied and the doubled backslash intact.
// Unescaping both halves would be the natural symmetry and it is wrong.
func zformatSplitPair(w string) (left, right string, split bool) {
	var out strings.Builder
	for i := 0; i < len(w); i++ {
		switch {
		case w[i] == '\\' && i+1 < len(w):
			out.WriteByte(w[i+1])
			i++
		case w[i] == ':':
			return out.String(), w[i+1:], true
		default:
			out.WriteByte(w[i])
		}
	}
	return out.String(), "", false
}

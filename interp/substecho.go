// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"
	"strings"

	"github.com/blairham/sh/syntax"
)

// runningText is the source of the program this runner is executing, which is
// what a diagnostic about a substitution body has to quote.
//
// # Why the runner holds text at all
//
// A substitution's body is parsed when it is expanded rather than where the
// line was read, and the note at the head of Runner.runCommandSubst costs out
// and declines moving that. So a refusal is written from here, with the body
// and the span in hand and nothing else — and two dialects follow it with a
// second line that quotes text the body does not contain. Measured 2026-09-16
// from a script file, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> word.sh`
// with stdin from /dev/null, line 2 of which is `q=1; v=$(echo hi; for); z=2`:
//
//	bash 5.3.20   word.sh: line 2: `q=1; v=$(echo hi; for); z=2'
//	zsh 5.9.2     word.sh:3: parse error near `v=$(echo hi; for); z...'
//	ksh93u+, dash 0.5.12, BusyBox ash 1.37.0: no second line
//
// bash quotes the whole *script* line and zsh the script from the start of
// the word, so the answer is the text of whatever program is running, which
// is only ever known to whoever read it: the front end for a script or a
// command string, runSourced for a sourced file or `eval`'s text. They hand
// it here, and it is kept as a string that shares the reader's bytes rather
// than a copy.
//
// # Why "borrowed" is a flag and not an empty string
//
// Five places run text that is not the script's: a sourced file, `eval`, a
// function defined in either and called after it was left, a trap body, and
// the body of the older substitution spelling. Each of those states the text
// it runs, so that a failure inside one is never quoted against the script's
// lines. Unset, the script's text is the answer, which is what a function
// written in the script itself needs and costs it no record. A function
// defined through Runner.DefineFunction is left unset too, rather than given
// a record per autoloaded name, and is caught by the check in substTextLines.
type runningText struct {
	text string
	// base is the line offset the text was entered at, so that a line of the
	// *script's* numbering indexes it: non-zero for `eval`'s text in the
	// dialects that continue the caller's lines.
	base     int
	borrowed bool
}

// SetProgramText tells the runner the source of the program the front end is
// running, everything read so far, numbered from its first line.
//
// Called before each line is run, because a program read from standard input
// grows as it runs; the string shares the reader's buffer, so the call costs
// nothing. Leave it unset for a prompt: the line such a diagnostic would
// quote is still on the screen above it, and no dialect writes it again.
func (r *Runner) SetProgramText(text string) {
	r.scriptText = text
}

// textInForce is the source the running program came from and the line
// offset it is numbered at, or the empty string where nobody said.
func (r *Runner) textInForce() (string, int) {
	if r.runText.borrowed {
		return r.runText.text, r.runText.base
	}
	return r.scriptText, 0
}

// substFailureAtItsLine moves the runner's line to the line a substitution
// body's refusal was found on, for the two messages about it, and returns
// what puts it back.
//
// The line the command began on is where every other runtime diagnostic is
// placed, and it is the wrong line here as soon as a body runs past it.
// Measured 2026-09-16 over a script whose lines 3 and 4 are `q=1; v=$(echo hi`
// and ` for); z=2`:
//
//	bash 5.3.20   line 4: syntax error near unexpected token `)'
//	zsh 5.9.2     :4: parse error near `)'
//	dash 0.5.12   4: Syntax error: Bad for loop variable
//	ksh93u+       line 3: syntax error at line 4: `)' unexpected
//
// So the failure's line, except in the one dialect whose sentence already
// says it — and which, having said it, places the message at the command.
// That is Diagnostics.ParseFailureNamesItsOwnLine, read for the same reason
// ParseDiagnostic reads it.
//
// **The backquoted spelling is placed here too, and used not to be.** It
// returned a no-op, so the refusal stood at the line the command began on
// whatever the body did — which is one dialect's answer and no shell's.
// Measured 2026-09-18 over the seven shapes of #3553, `env -i
// PATH=/usr/bin:/bin LC_ALL=C <shell> s.sh`, stdin from /dev/null, each body
// `echo hi` then `for` so the failure is on the body's second line:
//
//	                              file line  zsh  ksh93  dash  bash
//	`v=` + the body on line 2             3    3      3     2     4
//	the same with two lines above         4    4      4     2     5
//	`cat `, `: `, `echo `, `export v=`    3    3      3     2     3
//	a three-line body failing on its 3rd  4    4      4     3     6
//
// So three of the four are the *failure's* line, which is what this does for
// every other spelling; the dialect that numbers the body from its own first
// line already arrives with the base taken off
// (BackquotedSubstitutionRestartsLines) and reads the same rule; and the
// fourth adds the body's newlines where the substitution stands in the
// command's first token — Diagnostics.BackquotedSubstitutionFailureAddsItsBodysNewlines,
// which holds that sweep.
//
// body is the substitution's text, read for that addition alone.
func (r *Runner) substFailureAtItsLine(span syntax.Span, body string, failure error) func() {
	d := r.diag()
	line := d.ParseFailureLine(failure)
	if span.Backquoted && d.BackquotedSubstitutionFailureAddsItsBodysNewlines &&
		r.expandingOuterWord != nil && r.expandingOuterWord == r.commandFirstWord {
		line += strings.Count(body, "\n")
	}
	// The dialect that numbers a backquoted body from its own first line
	// needs no special case: its base is already nought, so the failure's
	// line *is* the body's and the prefix and the sentence agree. Measured
	// 2026-09-18 from a script file under `env -i PATH=/usr/bin:/bin
	// LC_ALL=C`, `echo one` / ``echo `if; then :; fi` `` / `echo two`: dash
	// 0.5.12 writes `<script>: 1: Syntax error: ";" unexpected` where the
	// `$( … )` spelling of the same body writes `2:`, and BusyBox 1.37.0
	// writes `line 1` against `line 2` the same way — the split #2471
	// identified.
	if d.ParseFailureNamesItsOwnLine || line < 1 || r.linePin != 0 {
		return func() {}
	}
	saved := r.line
	r.line = line
	return func() { r.line = saved }
}

// substFailureEcho is the second line a refused substitution body is given,
// without the location in front of it, and the line to place it at — or the
// empty string where the dialect writes none or the text is not known.
//
// failure is the refusal already shifted into the script's lines, and raw the
// same refusal in the body's own.
func (r *Runner) substFailureEcho(span syntax.Span, body string, raw, failure error, failureBase int) (string, int) {
	d := r.diag()
	var se *syntax.Error
	if !errors.As(failure, &se) || se.Kind != syntax.ErrUnexpected {
		return "", 0
	}
	line := d.ParseFailureLine(failure)
	if line < 1 {
		return "", 0
	}
	if span.Backquoted {
		// The older spelling is read with the script in the dialect that
		// quotes, so its answer is the *body's* line: `echo hi; for` for
		// “ v=`echo hi; for` “, and ` for` alone for a body whose second
		// line holds it. Measured on bash 5.3.20, 2026-09-16. The text is
		// the body, which is in hand, so nothing else is needed.
		//
		// Placed where the first message was, which that spelling does not
		// move — see substFailureAtItsLine — so the two cannot disagree.
		return d.offendingLine(d.ParseFailureLine(raw), raw, body), 0
	}
	if !d.EchoesTheOffendingLine && !d.SubstitutionParseFailureQuotesTheWord {
		return "", 0
	}
	text, textBase := r.textInForce()
	start := failureBase + 1 - textBase
	lines := substTextLines(span, body, text, start)
	if lines == nil {
		return "", 0
	}
	own := line - textBase
	if d.EchoesTheOffendingLine {
		return r.substBodyEcho(span, lines, start, own, d.offendingLine(own, failure, text)), line
	}
	return r.substWordEcho(span, lines, r.lineBase+int(span.Pos.Line)-textBase, own, line)
}

// substTextLines is the program's text split into lines, or nil where the text
// is not the text the span was read from.
//
// A span carries a position and not a source, and every route that runs text
// has to have said which text that is. Checked rather than trusted, because
// the cost of a wrong answer is a line quoted out of some other program — a
// function defined in one file and called from another — and writing nothing
// is what this did before there was an answer at all.
//
// The check is that the span's line holds the opener and the body's first
// line, anywhere on it. Not at the span's column, because a span inside an
// arithmetic expression or an expansion's operand is placed from *that*
// text's start: `echo $(( $(for) + 1 ))` is quoted whole by the dialect that
// echoes the line, and it is the right line with the wrong column.
//
// start is the span's line in the text, one-based.
func substTextLines(span syntax.Span, body, text string, start int) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if start < 1 || start > len(lines) {
		return nil
	}
	first, _, _ := strings.Cut(body, "\n")
	if !strings.Contains(lines[start-1], "$("+first) {
		return nil
	}
	return lines
}

// substLevel is what one lexing level still has open when the input runs out.
//
// A *level* is the text of one command substitution body, the script itself
// being the outermost of them. The unit matters, and it is what the reading
// this was built from got wrong: the messages do not come one per open
// context, they come one per level.
//
// Measured 2026-09-19 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin LC_ALL=C zsh
// -f s.sh`, standard input on the null device, one line per script. The first
// message is `s.sh:1: parse error near `)'` in every row and is left out; the
// rest is the whole of what follows it:
//
//	echo $(for)                          parse error near `$(for)'
//	echo "$(for)"                        unmatched "
//	echo ${x:-$(for)}                    closing brace expected
//	echo "${x:-$(for)}"                  unmatched "
//	echo ${x:-"$(for)"}                  unmatched "
//	echo ${x:-${y:-$(for)}}              closing brace expected
//	echo "${x:-"$(for)"}"                unmatched "
//	echo "x ${y:-$(for)} z"              unmatched "
//	echo "x $(echo ${y:-$(for)})"        closing brace expected, unmatched "
//	echo ${x:-$(echo "$(for)")}          unmatched ", closing brace expected
//	echo $(echo "$(for)")                unmatched ", parse error near `$(echo "$(for)")…'
//	echo $(echo $(for))                  parse error near `$(echo $(for))'
//	echo $(echo ${y:-$(for)})            closing brace expected, parse error near `$(echo ${y…'
//	echo ${x:-$(echo ${y:-$(for)})}      closing brace expected, closing brace expected
//	echo "$(echo "$(echo "$(for)")")"    unmatched ", unmatched ", unmatched "
//	echo $(( $(for) + 1 ))               parse error near `$(( $(for) + 1 ))'
//	echo "$(( $(for) + 1 ))"             unmatched "
//
// Three rules come off those rows and each has a row that fails without it:
//
//   - **One line per level, innermost outward.** Row 9 and row 10 are the
//     same three contexts in opposite orders and the lines follow the order;
//     rows 14 and 15 are two and three levels each writing one.
//   - **Two contexts open at one level write one line, not two.** Row 6 is
//     two braces and writes one, where "one line per open context" predicts
//     two. That is the row that falsifies the reading this replaces.
//   - **A quote at a level outranks a brace at that level.** Row 4 has the
//     quote outside the brace and row 5 has it inside, and both write
//     `unmatched "` and never the brace's sentence — so it is not the
//     innermost context of the level that is named, it is the quote.
//
// And the level with nothing open writes nothing at all, which is rows 12 and
// 16: the intermediate `$( )` and the `$(( ))` are levels that hold no quote
// and no brace, and neither contributes a line. Only the *outermost* level
// has a sentence for that case, which is the `parse error near` line #3331
// and #3731 already build.
//
// This shell writes one of those lines rather than all of them, because one
// refusal is written from one place. The one it writes is the innermost
// level's, which is the first zsh writes.
type substLevel struct {
	// quoted is a double quote still open at this level, whether it is
	// around the failing substitution or around an expansion holding it.
	quoted bool
	// braced is a `${` still open at this level: the failing substitution is
	// somewhere in an expansion's operand.
	braced bool
	// arith is an arithmetic expansion's text, which is a level with no
	// sentence of its own. Measured: `echo $(( $(for) + 1 ))` writes the
	// *outermost* level's `parse error near `$(( $(for) + 1 ))'` and no line
	// for the arithmetic, exactly as an intermediate `$( )` writes none.
	//
	// A field rather than a bare reset, because the text is re-lexed and the
	// spans it yields carry a quoting of their own: a reset alone left the
	// span's own `Quoting` to fold in below and produced `unmatched "` for a
	// row with no quote in it.
	arith bool
}

// inBraceOperand records that what is being expanded now is an expansion's
// operand, and returns the undo.
//
// The enclosing expansion's own quoting comes with it, because that is the
// route by which `"${x:-$(for)}"`'s quote reaches the operand: the `"` is on
// the *expansion's* span and the operand's spans are written bare inside it.
func (r *Runner) inBraceOperand(q syntax.Quoting) func() {
	saved := r.substLevel
	r.substLevel.braced = true
	if q != syntax.Unquoted {
		r.substLevel.quoted = true
	}
	return func() { r.substLevel = saved }
}

// atFreshSubstLevel opens a new level for a substitution body, and returns
// the undo.
//
// A body is lexed on its own, so what the text around it left open is not
// open inside it: the level that answers for `echo $(echo "$(for)")` is the
// body's, which holds the quote, and not the script's, which holds nothing.
//
// The level it replaces is kept rather than dropped, because a level with
// nothing open writes no line and the one outside it still writes its own —
// `echo "$(echo $(for))"` is `unmatched "` from the script's level, with the
// body's level contributing nothing between them.
//
// That is also why the substitution's *own* quoting is folded into the level
// being left behind: the `"` of that row is around the outer `$( )`, so it is
// on this span and not in anything the outer level had recorded.
func (r *Runner) atFreshSubstLevel(span syntax.Span) func() {
	saved, savedOuter := r.substLevel, r.substLevelsOut
	savedFragment := r.substFragmentLine
	r.substLevelsOut = append(append([]substLevel{}, r.substLevelsOut...), outerLevel(saved, span))
	r.substLevel = substLevel{}
	// And the body's lines are its own from here: whatever re-lexed text
	// this substitution was written in, the body is parsed separately and
	// placed by Runner.lineBase. See Runner.substFragmentLine.
	r.substFragmentLine = 0
	return func() {
		r.substLevel, r.substLevelsOut = saved, savedOuter
		r.substFragmentLine = savedFragment
	}
}

// outerLevel is the level a nested text is entered from, with the entering
// span's own quoting folded in. See atFreshSubstLevel.
func outerLevel(at substLevel, span syntax.Span) substLevel {
	if !at.arith && span.Quoting != syntax.Unquoted {
		at.quoted = true
	}
	return at
}

// inArithText opens the level of an arithmetic expansion's re-lexed text, and
// returns the undo. See substLevel.arith.
func (r *Runner) inArithText(span syntax.Span) func() {
	saved, savedOuter := r.substLevel, r.substLevelsOut
	savedFragment := r.substFragmentLine
	r.substLevelsOut = append(append([]substLevel{}, r.substLevelsOut...), outerLevel(saved, span))
	r.substLevel = substLevel{arith: true}
	// The text is lexed again here, so what it yields is numbered from the
	// expansion's own line rather than from the file's, and how far in that
	// line is has to travel with it. See Runner.substFragmentLine (#3810).
	r.substFragmentLine += int(span.Pos.Line) - 1
	return func() {
		r.substLevel, r.substLevelsOut = saved, savedOuter
		r.substFragmentLine = savedFragment
	}
}

// inArithCommandText opens the re-lexed text of an arithmetic *command* —
// `(( … ))`, or one part of a `for (( ; ; ))` header — and returns the undo.
//
// Not a level, and that is the difference from inArithText: a command has
// nothing lexically open around it, so nothing here writes a second message.
// What it shares is the numbering — the text is lexed again when it runs, so
// a substitution inside it carries the text's own line and not the file's,
// and `(( $(for) ))` on line 2 was reported at line 1 in every dialect.
// See Runner.substFragmentLine (#3810).
//
// The construct's own position, which is the line its text begins on: a `((`
// and the expression after it are never separated by a newline.
func (r *Runner) inArithCommandText(at syntax.Pos) func() {
	saved := r.substFragmentLine
	r.substFragmentLine += int(at.Line) - 1
	return func() { r.substFragmentLine = saved }
}

// substLevelEcho is the line a level with something open writes, or the empty
// string where the level holds neither a quote nor a brace.
//
// The failing span's own quoting is folded in here rather than at the door,
// because it is the one context that is on the span: `echo "$(for)"` has the
// quote around the substitution itself, where `"${x:-$(for)}"` has it around
// the expansion the substitution is an operand of.
func (r *Runner) substLevelEcho(span syntax.Span) string {
	d := r.diag()
	// The innermost level first, and outward past every level that holds
	// nothing. A level with neither a quote nor a brace writes no line at
	// all — the intermediate `$( )` of `echo "$(echo $(for))"` and the
	// `$(( ))` of `echo "$(( $(for) + 1 ))"` are both such a level — and the
	// level outside it still writes its own, which in both of those is the
	// script's `unmatched "`.
	// The one context that is on the span rather than around it.
	inner := outerLevel(r.substLevel, span)
	for at := len(r.substLevelsOut); ; at-- {
		level := inner
		if at < len(r.substLevelsOut) {
			level = r.substLevelsOut[at]
		}
		switch {
		case level.quoted:
			if d.UnmatchedQuote == "" {
				return ""
			}
			return Wording(d.UnmatchedQuote, `unmatched %[1]s`, `"`, `"`, "", 0, 0) + "\n"
		case level.braced:
			if d.UnmatchedBraceSubst == "" {
				return ""
			}
			return Wording(d.UnmatchedBraceSubst, "closing brace expected", "${", "}", "", 0, 0) + "\n"
		}
		if at == 0 {
			return ""
		}
	}
}

// substWordEcho is the second line of the dialect that quotes the script from
// the start of the word holding the substitution.
//
// Measured on zsh 5.9.2, 2026-09-16, from script files:
//
//	echo x$(echo hi; for) y        `x$(echo hi; for) y'   the word, not the command
//	q=1; v=$(echo hi; for); z=2    `v=$(echo hi; for); z...'   cut at twenty bytes
//	a[1]=$(for), a+=$(for)         `a[1]=$(for)', `a+=$(for)'   an assignment is one word
//	case $(for) in *) ;; esac      `$(for) in *) ;; esac...'
//	v=$(echo a <nl> echo b <nl> for)   `v=$(echo a'   the word's own line, to its end
//
// The cut is the one the same dialect makes for a construct that ran out —
// twenty bytes, marked at twenty and over — so it is Diagnostics.nearText.
//
// **The line is one past the failure's** where a newline ends it: `word.sh:3`
// for a refusal on line 2, `word.sh:1` for the same text with no newline at
// the end of the file, and `zsh:2` under `-c` for a two-line string. That is
// the reader having taken the newline, which is why it is read off the text.
//
// Only for a span written straight into a word, at the column the word places
// it. A redirection target is such a word and reaches this — its expansion
// walks the spans itself and had recorded no word at all, which is a missing
// call rather than a missing rule, so `echo hi >$(for)` wrote nothing where
// that shell quotes `$(for)` (#3355).
//
// Inside double quotes the second message is `unmatched "` and inside an
// expansion's operand `closing brace expected`, and those come first, because
// they are what the *level* has open and this sentence is what a level with
// nothing open writes. See substLevel for the rows and the three rules.
//
// An arithmetic expansion's text is re-lexed, so the span's column is counted
// from that text rather than from the line, and a substitution nested in a
// substitution is placed from the enclosing body; where neither level has a
// quote or a brace open, neither of those is written, rather than a quote
// from the wrong place.
//
// Nor inside a function body, where this dialect locates a message by the
// function and the line within it. It reads a body where the definition is,
// so its own second message there is numbered in the *file* — `s.sh:4` for a
// body refused on line 3 — and a runner that reaches the body only when it is
// called has no location that says that. A trap body is the same case from
// the other side: this dialect reads the action when the trap is *set*, and
// refuses it there with a message of its own.
func (r *Runner) substWordEcho(span syntax.Span, lines []string, start, own, line int) (string, int) {
	w := r.expandingWord
	if r.inTrapBody || r.diag().LocationNamesTheFunction && r.locationIsInsideAFunctionBody() {
		return "", 0
	}
	if own < 1 || own > len(lines) {
		return "", 0
	}
	if own < len(lines) {
		// A newline ends the failure's line and the reader has taken it, so
		// the message stands one line past it. Read off the text here for
		// the reason the word's own sentence reads it off below: it is the
		// same end of input and there is one rule for where it is placed.
		line++
	}
	if echo := r.substLevelEcho(span); echo != "" {
		return echo, line
	}
	if w == nil || w != r.expandingOuterWord || span.Quoting != syntax.Unquoted ||
		w.Start.Line != span.Pos.Line || w.Start.Col > span.Pos.Col {
		return "", 0
	}
	row := lines[start-1]
	from, at := int(w.Start.Col)-1, int(span.Pos.Col)-1
	if at+2 > len(row) || row[at:at+2] != "$(" {
		return "", 0
	}
	return Wording(r.diag().SyntaxUnexpected, `"%[1]s" unexpected`, r.diag().nearText(row[from:])) + "\n", line
}

// substBodyEcho cuts the echoed line back to the text the shell was reading,
// for the dialect that names the construct because it read that text at
// expansion time.
//
// A here-document body is a program that dialect reads on its own, and the
// line it echoes is a line of *that* program: measured 2026-09-17 on bash
// 5.3.20, a body of `before $(echo hi; for) after` on line 4 is echoed
// `echo hi; for) after` — from just past the opener, because that is where
// the text it was reading began. A body whose substitution runs onto a second
// line echoes that line whole, `for) y`, for the same reason: the opener is
// not on it.
//
// The older spelling is read at expansion time too and does **not** take
// this: a `$( … )` refused inside a backquoted body is echoed `echo $(for)`
// — the enclosing text's line, whole — so the cut belongs to the
// here-document body alone and is keyed on the line that body begins at.
//
// The line as written is the answer everywhere else, and it is the answer
// here too when the opener is not where the span says — a `<<-` body has had
// its leading tabs taken off, so the column the body was lexed at is not the
// file's. Checked rather than assumed, because the cost of trusting it is a
// sentence cut in the middle of a word.
func (r *Runner) substBodyEcho(span syntax.Span, lines []string, start, own int, echo string) string {
	if r.expansionBodyLine == 0 || span.Backquoted || own != start || echo == "" {
		return echo
	}
	if own < 1 || own > len(lines) {
		return echo
	}
	row := lines[own-1]
	at := int(span.Pos.Col) - 1
	if at < 0 || at+2 > len(row) || row[at:at+2] != "$(" {
		return echo
	}
	return "`" + row[at+2:] + "'\n"
}

// firstTokenWord is the word of the earliest token of a simple command — its
// first assignment's value, its first word, or its first redirection's target,
// whichever was written first — or nil where that token carries no word, as a
// bare `v=` does.
//
// Read only by Runner.commandFirstWord, and by pointer identity: what the one
// message that asks needs to know is whether the word it is expanding is the
// command's first token, and comparing words is exact where comparing
// positions would have to know that an assignment's value starts after its
// name.
func firstTokenWord(c *syntax.SimpleCmd) *syntax.Word {
	var at syntax.Pos
	var word *syntax.Word
	take := func(pos syntax.Pos, w *syntax.Word) {
		if !pos.IsValid() || (at.IsValid() && !at.After(pos)) {
			return
		}
		at, word = pos, w
	}
	if len(c.Assigns) > 0 {
		take(c.Assigns[0].Pos(), c.Assigns[0].Value)
	}
	if len(c.Args) > 0 {
		take(c.Args[0].Pos(), c.Args[0])
	}
	if len(c.Redirs) > 0 {
		take(c.Redirs[0].Pos(), c.Redirs[0].Word)
	}
	return word
}

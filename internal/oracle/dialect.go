// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import "github.com/blairham/sh/syntax"

// Dialect is the flag set the corpus needs, constructed here so that no
// consumer borrows a shell's preset: the corpus is dialect-neutral,
// and each flag beyond the core is named for the construct some case uses.
// A new case wanting a flag beyond these adds it here by name.
//
// **Every flag below was re-read against the `ash` column when it became the
// panel's seventh (#2331), and none came off.** That is the argument these
// comments make, checked rather than assumed: a flag is on because the
// grammar that has to *read* every case is the one that takes the construct,
// so a column can only widen the set of constructs the corpus records. ash is
// the narrowest member of the panel and refuses every construct these flags
// stand for, which is the strongest form the check could have taken — a
// seventh column that took one of them would have been the interesting
// outcome and there was none. The tallies did move, and they are re-derived
// below from the recorded cells rather than incremented.
//
// It lives beside the corpus rather than in a test file because two gates now
// ask the question — the printer's round trip and the formatter's — and two
// copies of "what grammar reads the corpus" would eventually disagree (#1401).
func Dialect() syntax.Dialect {
	d := syntax.Core()
	// `${!x}` and `${!prefix*}` — the param/ indirection cases.
	d.ParamIndirection = true
	// `$+v`, `$=v`, `$~v` and `$^a` — the flag sigils written without the
	// braces. One of the seven columns reads them as expansions and the other
	// six as text, and the corpus records both answers, so the grammar that
	// has to *read* every case is the one that takes them.
	d.BareParamFlags = true
	// `$[1+2` — the older arithmetic spelling. Four of the seven columns
	// have the construct and refuse it unterminated; the other three — dash,
	// ksh93 and ash — run the line as text, and the corpus records both
	// answers. The sentence here used to say two and two, which did not add
	// up to six either: it counted distinct shells where the rest of this
	// file counts columns. Same reason as
	// HeredocEndsAtClosingParen below: the grammar that has to *read* every
	// case is the one that takes the construct.
	d.DollarBracketArith = true
	// `!` with no pipeline after it, and a second `!` that toggles the first.
	// Four of the seven take a bare negation before a terminator and three
	// take the toggle, in two different groupings — zsh is in the first and
	// not the second — so the widest reading here would be
	// BareNegationAtEitherPlace — and it is deliberately **not** taken. This
	// dialect is the one SyntaxError names, so a case it reads is a case the
	// printer has to write back; the two rows recording a bare `!` at a
	// *closer* and before an and-or are refusals in this column and stay
	// refusals, exactly as the three empty-body rows do above. The
	// terminator reading is bash's, which is the column the flag answers for.
	d.BareNegationReach = syntax.BareNegationBeforeATerminator
	d.RepeatedNegationToggles = true
	// `;;&` — the case-continue cases.
	d.CaseContinue = true
	// `;|` — the same terminator spelled zsh's way, which no shell has
	// alongside `;;&`: each refuses the other. Both are on here for the
	// reason above — the grammar that has to *read* every case is wider
	// than any one shell, and the corpus records both spellings.
	d.CaseContinuePipe = true
	// `case a in (a|` newline `b)` — the arm whose parenthesized pattern
	// list spans a newline. One of the seven takes it and the other six call
	// the line a syntax error, and the corpus records both; the grammar that
	// has to *read* every case is the one that takes the construct, which is
	// the same argument as DollarBracketArith above.
	d.CasePatternListSpansNewlines = true
	// `case 'a b' in (a b)` — the arm whose parenthesized pattern holds a
	// blank. Same shape and same argument as the newline above: one of the
	// seven matches it and the other six call the line a syntax error, and
	// the corpus records both.
	d.CasePatternListSpansBlanks = true
	// `~(i)[[:lower:]]` — the pattern-modifier prefix, whose `(` would
	// otherwise end the word. One of the seven columns takes it and the
	// other six refuse the line at the paren or read the `~` as something
	// else entirely, and the corpus records both answers; the grammar that
	// has to *read* every case is the one that takes the construct, which is
	// the same argument as DollarBracketArith above. The `(` is claimed only
	// directly behind a `~`, so no case without one is read differently.
	d.TildeGroup = true
	// `function f() { …; }`, both markers at once.
	d.FunctionKeywordParens = true
	// `function _p_${w} { … }` — a keyword name that is not a name, carried
	// to the definition instead of refused while parsing. Five of the seven
	// answer that way — the three bash columns call the name invalid where
	// the definition runs, zsh defines it, and ash reads the line without a
	// word and defines nothing — and the corpus records that dash and ksh93
	// refuse it while reading, so the grammar that has to *read* every case
	// is the one that takes the construct, as above. The `name()` spelling is deliberately not widened
	// with it: ksh93 refuses that one while reading, which is the split
	// `cmd/a-function-name-may-hold-an-expansion` records.
	d.FunctionNameCheckedWhenTheDefinitionRuns = true
	// `function '' { … }` — the empty string as a name, which one of the
	// seven defines and four refuse where the definition runs rather than
	// while reading it; ash is the seventh and takes the line in silence.
	// The grammar that has to *read* every case is the one that takes the
	// construct, as above.
	d.FunctionKeywordNameIsAnyWord = true
	// `function a b { … }` — one body under several names, which one of the
	// seven defines and the other six read as a different program: four are
	// a syntax error at the second name, dash has no keyword at all, and
	// ksh93 parses the line and defines only the first. The corpus records
	// all of those, so the grammar that has to *read* every case is the one
	// that takes the construct, as above.
	d.FunctionMultipleNames = true
	// `function a b` with no body, and the `;` that may stand between a name
	// list and the body it does have. One of the seven declares each name
	// with an empty body and the other six refuse the line — dash alone by
	// having no keyword, so it reads `function` as a command it cannot find —
	// and the corpus records all of it, so the grammar that has to *read*
	// every case is the one that takes the construct, as above.
	//
	// EmptyCompoundBody does **not** come with it, which bounds what a case
	// may be written as: a declaration with no body prints back as `{ }` and
	// this grammar cannot read that, so a bodyless declaration belongs inside
	// an `eval` string where the printer never reaches it. The three cases
	// recording `{ }`, `( )` and an empty loop body as refusals are why the
	// flag stays off — one of the seven takes them and the corpus keeps the
	// refusal, which is the one place this dialect is not the widest reading.
	d.FunctionKeywordBodyIsOptional = true
	// `'a b'() { … }` — the POSIX form's name read as any word, which one of
	// the seven defines and four refuse where the definition runs rather than
	// while reading it; ash reads it in silence and defines nothing, the same
	// answer it gives the two rows above. Only dash refuses to parse it, so
	// the grammar that
	// has to *read* every case is the one that takes the construct, as above.
	d.FunctionNameIsAnyWord = true
	// `[[ x == @(a|b) ]]` — extended patterns where a condition reads them.
	//
	// `d.ExtendedPattern` — the same construct in *argument* position — is
	// **on**, and the note that used to stand here explaining why it could
	// not be was wrong about its own evidence (#2474).
	//
	// It said the flag stopped `pat/an-empty-quantified-group-is-not-a-
	// function-definition` parsing, "where the case exists to record that it
	// is a function definition". That case records the opposite: its `Why`
	// says `a+(` opens a group, leaves `a` standing in front of a brace
	// group, and makes the whole line a syntax error that bash and ksh93
	// both refuse. So the flag did not break the case — it made this grammar
	// start agreeing with the two shells the case measures, and what was
	// missing was the one field that says so.
	//
	// A note asserting a blocker is worse than no note, because it is cited
	// rather than rechecked: this one was quoted into #2466's commit message
	// and kept the axis without a corpus row for as long as it stood.
	d.ExtendedPatternInCondition = true
	d.ExtendedPattern = true
	// FuncBodyMustBeCompound is deliberately **off**, where it was on to let
	// `f() echo hi` be recorded as a syntax error. It stopped being a
	// narrowing of one production when the keyword form began reading it too
	// (#1833), and the corpus has a case — `function a; echo B` — whose body
	// is a simple command and which every grammar here has to read. The two
	// cannot both hold, and the rule this file follows everywhere else
	// decides it: the grammar that has to *read* every case is the one that
	// takes the construct. The rows are still graded, by the panel's answers,
	// which is where a refusal is a fact rather than a flag.
	//
	// Four cases stopped being marked SyntaxError with it — `f() echo hi`,
	// `f() x=1`, `f() >out` and `f() echo hi >out`. That mark says only "this
	// grammar rejects it", so what it cost is a skipped parse and what it
	// bought back is a grammar that reads every keyword case.
	//
	// `coproc NAME { …; }` — the cases that pin where a name may be written,
	// which needs both flags: the word, and the name a dialect may put after
	// it. One dialect has the first and not the second, and the corpus
	// records what that costs.
	d.Coproc = true
	d.CoprocName = true
	// `v=$(cat <<EOF` / `a` / `EOF)` — the here-document cases whose delimiter
	// carries the closing parenthesis. Three of the seven columns refuse them
	// — dash, zsh and ash — and the corpus records both answers, so the
	// grammar that has to *read* every case is the one that takes them.
	d.HeredocEndsAtClosingParen = true
	// And the widest reading of a body line that joined across a
	// backslash-newline, for the same reason: four of the seven columns end
	// the document on the joined text and the corpus records what the other
	// three do instead, so the grammar that has to read every case is the
	// one that recognizes the delimiter wherever any column does (#2430).
	d.HeredocDelimiterAcrossAContinuation = syntax.HeredocDelimiterOnTheJoinedLine
	// `${ echo hi;}` — the body that runs in the current shell. Three of the
	// seven columns have it — bash 5.3 in both of its columns, which is why
	// bash 3.2 does not, and ksh93 — and the other four call it a bad
	// substitution; the corpus records both. Same argument as the flags
	// above — the grammar that has to *read* every case is the one that
	// takes the construct — and here it is load-bearing rather than tidy: a
	// `#` is a comment in that body and the strip operator in `${x#a}`, so
	// without the flag a case with a comment in one is lexed as an ordinary
	// expansion and the apostrophe in `# it's fine` runs off the end (#1397).
	d.CurrentShellSubstitution = true
	// `${| echo hi;}` — the same body, valued from `$REPLY`. One of the seven
	// columns has it, bash 5.3 in both of its columns being one binary, and
	// the corpus records what the other five say instead; same argument as
	// the flag above, and load-bearing for the same reason — the body's `#`
	// is a comment here too (#2656).
	d.ReplySubstitution = true
	// `${(echo hi)}` — the parenthesized subshell as a substitution body.
	// One of the seven columns has it, ksh93, and the corpus records what
	// the other six say instead; same argument as the two flags above, the
	// grammar that has to *read* every case being the one that takes the
	// construct. It is narrow by measurement rather than by caution: the `}`
	// has to sit directly behind the matching `)`, so `${(o)$(f)}` and the
	// other zsh flag groups in this file are untouched by it and are still
	// read as the parameter form (#2615).
	d.SubshellSubstitution = true
	// `a |& b` — the pipe-of-both-streams cases. Four of the seven columns
	// take the operator — three as a pipe and ksh93 as a coprocess — and the
	// corpus records what the other three say about it, so the grammar that
	// reads every case has to have it.
	d.PipeBothStreams = true
	// `>!`, `>>|`, `>>!` and the same four behind `&>` — the
	// clobber-override marker in its other spellings. One of the seven
	// columns reads them and the corpus records what the other six do instead,
	// which for the `|` spellings is a syntax error, so the grammar that has
	// to read every case is the one that takes them.
	d.ClobberOverrideMarker = true
	// `b[(r)y]=Q` — the subscript flag group, on the left of an assignment.
	// One of the seven takes it; four read the whole subscript as arithmetic
	// and fail on the parenthesis, and dash and ash refuse the `(` while
	// reading. The corpus records all of that, so the grammar that has to
	// *read* every case is the one that takes it.
	d.ArraySubscriptFlags = true
	// `=(cmd)` — the temp-file spelling of process substitution. One of the
	// seven writes a file and the other six refuse the `(`; the corpus
	// records both, so the grammar that has to *read* every case is the one
	// that takes the construct, as above.
	d.ProcessSubstitutionToFile = true
	// `cmd >; file` — the write that only lands if the command succeeded.
	// One of the seven has it and the corpus records the syntax error the
	// other six report at the `;`, so the grammar that has to *read* every
	// case is the one that takes it, as above.
	d.RenameOnSuccessRedirect = true
	// `<#((expr))` and `>#((expr))`, the file-position redirections — the
	// other half of what this shell alone does to a descriptor. Measured
	// 2026-09-16 on ksh93u+ 2012-08-01 (#3034).
	d.SeekRedirect = true
	// `let a=(5 + 3)` — the arithmetic utility taking a compound assignment
	// as an operand, which one of the seven columns has and the other six
	// refuse while reading the line. The core's list holds the five
	// declarations; this adds the sixth word for the same reason every flag
	// above is set, since the grammar that has to *read* every case is the
	// one that takes the construct. Added rather than replaced, so the five
	// the core names stay.
	d.DeclarationUtilities = withDeclarationUtility(d.DeclarationUtilities, "let")
	return d
}

// withDeclarationUtility is d.DeclarationUtilities plus one word, copied
// rather than written through: the map came out of syntax.Core() and a
// caller that holds one of its own must not find a word this file added.
func withDeclarationUtility(words map[string]bool, name string) map[string]bool {
	out := make(map[string]bool, len(words)+1)
	for w, ok := range words {
		out[w] = ok
	}
	out[name] = true
	return out
}

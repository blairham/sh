// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import "github.com/blairham/sh/syntax"

// Dialect is the flag set the corpus needs, constructed here so that no
// consumer borrows a shell's preset: the corpus is dialect-neutral,
// and each flag beyond the core is named for the construct some case uses.
// A new case wanting a sixth flag adds it here by name.
//
// It lives beside the corpus rather than in a test file because two gates now
// ask the question — the printer's round trip and the formatter's — and two
// copies of "what grammar reads the corpus" would eventually disagree (#1401).
func Dialect() syntax.Dialect {
	d := syntax.Core()
	// `${!x}` and `${!prefix*}` — the param/ indirection cases.
	d.ParamIndirection = true
	// `$+v`, `$=v`, `$~v` and `$^a` — the flag sigils written without the
	// braces. One of the six columns reads them as expansions and the other
	// five as text, and the corpus records both answers, so the grammar that
	// has to *read* every case is the one that takes them.
	d.BareParamFlags = true
	// `$[1+2` — the older arithmetic spelling. Two of the six shells have
	// the construct and refuse it unterminated; the other two run the line
	// as text, and the corpus records both answers. Same reason as
	// HeredocEndsAtClosingParen below: the grammar that has to *read* every
	// case is the one that takes the construct.
	d.DollarBracketArith = true
	// `;;&` — the case-continue cases.
	d.CaseContinue = true
	// `;|` — the same terminator spelled zsh's way, which no shell has
	// alongside `;;&`: each refuses the other. Both are on here for the
	// reason above — the grammar that has to *read* every case is wider
	// than any one shell, and the corpus records both spellings.
	d.CaseContinuePipe = true
	// `case a in (a|` newline `b)` — the arm whose parenthesized pattern
	// list spans a newline. One of the six takes it and the other five call
	// the line a syntax error, and the corpus records both; the grammar that
	// has to *read* every case is the one that takes the construct, which is
	// the same argument as DollarBracketArith above.
	d.CasePatternListSpansNewlines = true
	// `case 'a b' in (a b)` — the arm whose parenthesized pattern holds a
	// blank. Same shape and same argument as the newline above: one of the
	// six matches it and the other five call the line a syntax error, and
	// the corpus records both.
	d.CasePatternListSpansBlanks = true
	// `function f() { …; }`, both markers at once.
	d.FunctionKeywordParens = true
	// `function _p_${w} { … }` — a keyword name that is not a name, carried
	// to the definition instead of refused while parsing. Two of the six
	// answer that way and the corpus records what the other four do, so the
	// grammar that has to *read* every case is the one that takes the
	// construct, as above. The `name()` spelling is deliberately not widened
	// with it: ksh93 refuses that one while reading, which is the split
	// `cmd/a-function-name-may-hold-an-expansion` records.
	d.FunctionNameCheckedWhenTheDefinitionRuns = true
	// `function '' { … }` — the empty string as a name, which one of the six
	// defines and four refuse where the definition runs rather than while
	// reading it. The grammar that has to *read* every case is the one that
	// takes the construct, as above.
	d.FunctionKeywordNameIsAnyWord = true
	// `function a b { … }` — one body under several names, which one of the
	// six defines and the other five read as a different program: three are
	// a syntax error at the second name, dash has no keyword at all, and
	// ksh93 parses the line and defines only the first. The corpus records
	// all of those, so the grammar that has to *read* every case is the one
	// that takes the construct, as above.
	d.FunctionMultipleNames = true
	// `function a b` with no body, and the `;` that may stand between a name
	// list and the body it does have. One of the six declares each name with
	// an empty body and the other five call the line a syntax error, and the
	// corpus records both — so the grammar that has to *read* every case is
	// the one that takes the construct, as above.
	//
	// EmptyCompoundBody does **not** come with it, which bounds what a case
	// may be written as: a declaration with no body prints back as `{ }` and
	// this grammar cannot read that, so a bodyless declaration belongs inside
	// an `eval` string where the printer never reaches it. The three cases
	// recording `{ }`, `( )` and an empty loop body as refusals are why the
	// flag stays off — one of the six takes them and the corpus keeps the
	// refusal, which is the one place this dialect is not the widest reading.
	d.FunctionKeywordBodyIsOptional = true
	// `'a b'() { … }` — the POSIX form's name read as any word, which one of
	// the six defines and four refuse where the definition runs rather than
	// while reading it. Only dash refuses to parse it, so the grammar that
	// has to *read* every case is the one that takes the construct, as above.
	d.FunctionNameIsAnyWord = true
	// `[[ x == @(a|b) ]]` — extended patterns where a condition reads them.
	d.ExtendedPatternInCondition = true
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
	// carries the closing parenthesis. Two of the six shells refuse them and
	// the corpus records both answers, so the grammar that has to *read* every
	// case is the one that takes them.
	d.HeredocEndsAtClosingParen = true
	// `${ echo hi;}` — the body that runs in the current shell. Two of the
	// six have it, one of them only since 5.3, and the other four call it a
	// bad substitution; the corpus records both. Same argument as the flags
	// above — the grammar that has to *read* every case is the one that
	// takes the construct — and here it is load-bearing rather than tidy: a
	// `#` is a comment in that body and the strip operator in `${x#a}`, so
	// without the flag a case with a comment in one is lexed as an ordinary
	// expansion and the apostrophe in `# it's fine` runs off the end (#1397).
	d.CurrentShellSubstitution = true
	// `a |& b` — the pipe-of-both-streams cases. Four of the six columns
	// take the operator and the corpus records what the other two say about
	// it, so the grammar that reads every case has to have it.
	d.PipeBothStreams = true
	// `>!`, `>>|`, `>>!` and the same four behind `&>` — the
	// clobber-override marker in its other spellings. One of the six columns
	// reads them and the corpus records what the other five do instead,
	// which for the `|` spellings is a syntax error, so the grammar that has
	// to read every case is the one that takes them.
	d.ClobberOverrideMarker = true
	// `b[(r)y]=Q` — the subscript flag group, on the left of an assignment.
	// One of the six takes it and the other five read the whole subscript as
	// arithmetic and fail on the parenthesis; the corpus records both, so the
	// grammar that has to *read* every case is the one that takes it.
	d.ArraySubscriptFlags = true
	// `=(cmd)` — the temp-file spelling of process substitution. One of the
	// six writes a file and the other five refuse the `(`; the corpus
	// records both, so the grammar that has to *read* every case is the one
	// that takes the construct, as above.
	d.ProcessSubstitutionToFile = true
	return d
}

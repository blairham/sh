// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/blairham/sh/syntax"
)

// The parenthesized expansion flags: `${(U)x}` and its family. One grammar in
// the panel has the construct, so what it means here is that shell's answer,
// measured and recorded in docs/spec/grammar/parameter-expansion.md; the
// vendor manual's rule list gives the order the steps run in, and the steps
// below carry the rule numbers they implement.

// implementedParamFlags are the flag letters this slice carries. Anything
// else the grammar accepted is refused *by name* when the expansion is
// reached, because the only thing worse than refusing a flag is answering it
// wrong with status 0.
const implementedParamFlags = "ULfsj@kvP%qMuoOniaQcwWA~Z"

// expandFlagged answers an expansion that carries a flag group, as fields.
// It reports false only when the node carries no group, so the ordinary
// paths stay exactly as they were.
// head has the meaning expandSpan gives it: whether this span stands at the
// head of the word being built, which is what the `${~spec}` flag's tilde
// half asks about.
func (r *Runner) expandFlagged(s syntax.Span, sp splitPolicy, head bool) ([]string, bool) {
	e := s.Param
	if e == nil || !e.HasFlags {
		return nil, false
	}
	quoted := s.Quoting != syntax.Unquoted
	// A `(~)` in the group exempts the separator this expansion *inserts*
	// from the escape the rest of the result gets, so whether that escape
	// runs has to be known at the join rather than after it — see
	// interp/tildeflaggroup.go. Asked only where a separator is marked, so
	// no other expansion answers this question twice.
	var escapeSep func(string) string
	if tildeMarksJoinSep(e) {
		if quoted ||
			!r.ask(r.globSubstAnswer(s), "globbing the result of an expansion") {
			escapeSep = globEscape
		}
	}
	words, isList, escaped, ok := r.flaggedWords(e, sp, quoted, escapeSep)
	if !ok {
		return nil, true
	}
	if !isList {
		v := words[0]
		if quoted {
			if !escaped {
				v = globEscape(v)
			}
			return []string{v}, true
		}
		if v == "" {
			// An unquoted expansion of an empty value is no field at all.
			return nil, true
		}
		if !escaped &&
			!r.ask(r.globSubstAnswer(s), "globbing the result of an expansion") {
			v = globEscape(v)
		}
		// `${(U)~g}` is measured: the group is read, the case applied, and
		// the tilde marks what came out. The tilde may only follow the
		// group — `${~(U)g}` is a bad substitution — so this is the one
		// order there is.
		return []string{r.tildeFlagHead(s, head, v)}, true
	}
	// Empty words are removed from a list result — measured on both sides:
	// `${(s.:.)x}` on `a::b` is two words however it is quoted, and only
	// `"${(@s.:.)x}"` keeps the third. `$@` and an `[@]` subscript keep
	// their empties in quotes without needing the flag, exactly as they do
	// without one.
	// A `${(U)=v}` splits on IFS inside the group, and the fields it made
	// are kept whole in quotes: measured, `v=' a '; "${(U)=v}"` is three
	// fields, the outer two empty. That is the same rule `(@)` asks for,
	// reached by a different flag.
	keepEmpty := quoted && (r.flagKeepsFields(e) || splitFlagInGroup(e, sp))
	// And a quoted `(f)` or `(s)` keeps the empty field at each *edge* while
	// still dropping the interior ones, which is the same rule `${=spec}`
	// already follows for an IFS split and was measured separately for these
	// two flags — see splitFlagEdges.
	edges := !keepEmpty && splitFlagEdges(e, quoted)
	out := make([]string, 0, len(words))
	for i, w := range words {
		if w == "" && !keepEmpty && !(edges && (i == 0 || i == len(words)-1)) {
			continue
		}
		if quoted || !r.ask(r.globSubstAnswer(s), "globbing the result of an expansion") {
			w = globEscape(w)
		}
		out = append(out, w)
	}
	return r.tildeFlagElements(s, head, out), true
}

// flaggedWords runs the flag pipeline and returns the resulting words, raw.
// ok is false when the expansion failed and the failure has been reported.
// escapeSep says whether the escape that a marked `(~)` separator has to sit
// outside of would run at all; escaped reports that it has already been done
// here, around that separator, so the caller must not do it again.
func (r *Runner) flaggedWords(e *syntax.ParamExpr, sp splitPolicy, quoted bool,
	escapeSep func(string) string,
) (words []string, isList, escaped, ok bool) {
	if e.FlagsErrPos > 0 {
		// A character the group could not carry, deferred here by the
		// parser: reached in a branch never taken, it is no error at all,
		// which is measured. The position counts from the `$`.
		r.diagf("%s\n", Wording(r.diag().ExpansionFlagsError,
			"error in flags near position %[1]d in '%[2]s'",
			e.FlagsErrPos, "${"+e.Src+"}"))
		r.expandErr = true
		return nil, false, false, false
	}
	// Whether the group asks for minimal quoting is answered by the same pass
	// that refuses what this slice does not carry, rather than by a second
	// walk further down: a `-` is the minimal-quoting modifier or it is the
	// sort flag, and one reading has to decide both questions or they can
	// come apart.
	minimal := false
	for i, c := range e.Flags {
		if c == '-' && minimalQuoteModifier(e.Flags, i) {
			// The `-` a `q` in front of it ate. See interp/minimalquote.go
			// for how it is told apart from the sort flag spelled the same.
			minimal = true
			continue
		}
		if !r.paramFlagCarried(c) {
			r.diagf("${%s}: the (%c) expansion flag is not implemented\n", e.Src, c)
			r.expandErr = true
			return nil, false, false, false
		}
	}
	// `(~)` marks the string argument of a flag written behind it, and the
	// only argument this interpreter can hold marked is the join separator.
	// The compositions it cannot are named rather than carried — see
	// interp/tildeflaggroup.go for the measurement behind each.
	markJoin := tildeMarksJoinSep(e)
	if why, refuse := tildeMarkRefusal(e, markJoin, splitFlagInGroup(e, sp)); refuse {
		r.diagf("${%s}: the (~) expansion flag is not implemented %s\n", e.Src, why)
		r.expandErr = true
		return nil, false, false, false
	}
	if strings.ContainsRune(e.Flags, 'A') && isAssignOp(e.Op) {
		// `(A)` is the one flag whose whole job is a *side effect*: it makes
		// the name an array — `(AA)` an association — where the expansion
		// assigns, and does nothing at all where it does not. Measured on
		// zsh 5.9.2, both halves:
		//
		//	unset u; ${(A)u=x y}   leaves `typeset -a u=( 'x y' )`
		//	unset u; ${(A)u:-x y}  leaves u unset, `:-` being no assignment
		//	v="a b"; ${(A)#v}      3, the string's length, exactly as ${#v}
		//	v="a b"; "${(A)v}"     one field, exactly as "${v}"
		//	v="a|b"; ${(As:|:)v}   `a b`, exactly as ${(s:|:)v}
		//
		// So the flag is carried by *not* acting on the six lines the panel's
		// plugin managers actually write, which are all the second kind, and
		// the assignment it exists for is refused by name here rather than
		// left to look like it worked: an assignment silently making a
		// scalar where the script asked for an array is the shape a later
		// `${u[2]}` reads as empty.
		//
		// Refused for the *operator* rather than for the assignment actually
		// firing, which is deliberate. `${(A)u=x y}` assigns nothing when `u`
		// is already set, so a check on whether it fired would refuse a line
		// on one run and carry it on the next, and the reader would have
		// nothing to go on. The refusal is a statement about the construct.
		r.diagf("${%s}: the (A) expansion flag is not implemented for an assignment\n", e.Src)
		r.expandErr = true
		return nil, false, false, false
	}
	if strings.Count(e.Flags, "q") > 4 {
		r.diagf("${%s}: the (%s) expansion flag is not implemented\n",
			e.Src, strings.Repeat("q", strings.Count(e.Flags, "q")))
		r.expandErr = true
		return nil, false, false, false
	}

	words, set, isList := r.flagBase(e)

	// Rule 4: (P) treats the value so far as a further name, before any
	// operator runs — `${(P)x:-def}` tests the *resolved* value.
	if strings.ContainsRune(e.Flags, 'P') {
		words, set, isList = r.namedBase(strings.Join(words, " "), "")
	}

	// The is-it-set question, asked of whatever the base and `(P)` came to:
	// measured, `v=nosuchvar; ${(P)+v}` is 0 while `${(P)v}` is empty and
	// `${+v}` is 1, so the group's name resolution runs and its value
	// transformations do not — `${(U)+v}` is `1` and not an uppercased
	// anything. In front of the nounset check on purpose: `set -u` is not
	// tripped by asking.
	if setTestAnswers(e) {
		return []string{setTestResult(set)}, false, false, true
	}

	if !set && e.Name != "" {
		r.checkNounset(e)
	}

	// Rule 5: in double quotes the words are joined — with the `j`
	// separator when one was given, else the first character of IFS —
	// unless the fields were asked for.
	//
	// A length is asked of the words *before* this, which is measured and is
	// not what the rule numbers suggest: `"${(U)#a}"` on `(abc de f)` is 3,
	// the element count, and not 8, the length of the joined text. A `j`
	// separator does not reach the count either — `"${(Uj.-.)#a}"` is 3 as
	// well — so the join is skipped rather than undone, and `(c)` reads a
	// separator of its own where it wants one.
	joined := false
	if quoted && isList && !e.Length && !r.flagKeepsFields(e) && !markJoin {
		words = []string{strings.Join(words, r.flagJoinSep(e))}
		isList = false
		joined = true
	}

	// Rule 7: the operator, applied to the value at this level. Measured:
	// the flags apply to what the operator leaves — `${(U)x:-def}` is DEF,
	// `${(U)u:=def}` assigns def and substitutes DEF.
	words, isList, ok = r.applyFlagOp(e, words, set, isList)
	if !ok {
		return nil, false, false, false
	}

	// Rule 9: length — the element count for a list and the value's own
	// length for a scalar, unless `c`, `w` or `W` said to count something
	// else. See lengthflags.go.
	if e.Length {
		words, isList = []string{itoa(r.flaggedLength(e, words, isList))}, false
	}

	// An `=` beside the group is this same step with IFS for a separator.
	ifsSplit := splitFlagInGroup(e, sp)
	hasSplit := strings.ContainsAny(e.Flags, "fs") || ifsSplit
	// Rule 10: forced joining, ahead of a split — `${(s.:.)a}` on an array
	// joins its elements with IFS's first character and splits the result.
	// `Z` is deliberately absent from this condition, and that is measured
	// rather than an oversight: an unquoted `${(Z+n+)a}` over the array
	// `('"x' 'y"')` is two fields there, the elements read as command lines
	// one at a time, where `${(s.:.)a}` over the same array is the single
	// field `"x y` — joined first, exactly as this rule says. So the two
	// splits differ here, and only the *quoted* join at rule 5 reaches `Z`,
	// which is why `"${(Z+n+)a}"` on that array is one field and
	// `"${(@Z+n+)a}"`, whose `@` skips that join, is two again.
	if (strings.ContainsRune(e.Flags, 'j') || hasSplit) && !joined && isList && !markJoin {
		words = []string{strings.Join(words, r.flagJoinSep(e))}
		isList = false
	}

	// Rule 11: splitting. `f` is split-at-newlines; an empty `s` separator
	// splits into characters, which is measured.
	if hasSplit {
		var split []string
		for _, w := range words {
			if ifsSplit {
				// `${=spec}` splitting, which is field splitting on IFS and
				// not a separator the group named. Quoted it keeps the
				// fields at the edges — see interp/splitflag.go.
				ifs, set := r.ifs()
				split = append(split, r.splitFieldsAsking(w, nil, ifs, set, quoted)...)
				continue
			}
			split = append(split, r.splitFlagged(w, e)...)
		}
		words, isList = split, true
	}

	// Rules 12, 13, 14 in the manual's order: case, prompt escapes, quoting.
	for _, c := range e.Flags {
		if c == 'U' || c == 'L' {
			for i, w := range words {
				words[i] = r.convertCase(w, c == 'U')
			}
		}
	}
	if strings.ContainsRune(e.Flags, '%') {
		for i, w := range words {
			v, pok := r.promptEscapes(w, e)
			if !pok {
				return nil, false, false, false
			}
			words[i] = v
		}
	}
	if n := strings.Count(e.Flags, "q"); n > 0 {
		for i, w := range words {
			words[i] = quoteFlagged(w, n, minimal)
		}
	}
	// Rule 14's other half: `Q` takes one level of quoting *off*. The manual
	// lists it beside `q` and measurement says which of the two runs first —
	// `${(Qq)v}` and `${(qQ)v}` on `'a b'` are both `'a b'`, the round trip,
	// where a `Q` that ran first would have left `a\ b`. See quoteflag.go.
	if strings.ContainsRune(e.Flags, 'Q') {
		for i, w := range words {
			words[i] = r.unquoteFlagged(w)
		}
	}
	// A marked separator's join, held back to here from rule 10.
	//
	// The shell being modeled joins at rule 10 and marks the separator so
	// the steps between there and here step over it. Holding the join back
	// instead is the same answer wherever those steps rewrite text one
	// character at a time — `b=('a b' 'c'); ${(~qj.|.)b}` is `a\ b|c` either
	// way, since `q` marks a character at a time and never the bar — and the
	// steps where it is *not* the same answer are refused by name in
	// tildeflaggroup.go rather than held back through.
	//
	// Ordering is the one step that has to see the join, and it is the next
	// one: `c=(x a); ${(~oj.|.)c}` is `x|a`, unsorted, exactly as
	// `${(oj.|.)c}` is, and `e=(p p q); ${(~uj.|.)e}` keeps both `p`s for the
	// same reason — the sort and the dedup have one word by the time they
	// run.
	if markJoin {
		words = []string{joinLiveSep(words, r.flagJoinSep(e), escapeSep)}
		isList = false
	}
	// The shell-split flag, `${(Z:opts:)v}`, which reads the value as a
	// command line. It stands here rather than beside the other splits at
	// rule 11, and the place is measured on zsh 5.9.2 — the one shell that
	// has the flag — from both sides:
	//
	//	v="a|b";  "${(Z+n+q)v}"   a\|b       one field: the `q` ran first
	//	v="'a|b'"; "${(Z+n+Q)v}"  a | b     three: the `Q` ran first
	//	v="b  a"; "${(oZ+n+)v}"   a b       sorted: the ordering ran after
	//
	// The first two are the discriminating ones. A split that ran at rule 11
	// would hand `q` three words and give back three fields, and would hand
	// `Q` one quoted word and give back one — both the opposite of what the
	// shell answers. The third pins the other end: the words this makes are
	// what `(o)` sorts, so it cannot go last either.
	//
	// `(f)` and `(s)` compose with it rather than racing it, measured the
	// same day: `${(fZ+n+)v}` on `a:b\nc` is `a:b` and `c`, each line then
	// read as a command line of its own.
	if opts, ok := shellSplitOpts(e); ok {
		words, isList = r.splitShellWordsAll(words, opts), true
	}

	// The ordering step is last of all, which is *later* than the rule
	// numbers suggest and later than this file used to put it. Three
	// measurements fix it there rather than one:
	//
	//	a=(B a);        ${(@oU)a}   A B          after the case conversion
	//	a=("%x" "*");   ${(@%o)a}   * <path>     after the prompt escapes
	//	a=("a b" "a!"); ${(@qo)a}   a! a\ b      after the quoting
	//	a=("'z'" b);    ${(@Qo)a}   b z          and after the unquoting
	//
	// The last two are the ones that would be got wrong by reading the rule
	// list: `*` sorts ahead of a path only once `%x` has become one, and
	// `a!` ahead of `a\ b` only once the space has become a backslash —
	// both orders reverse if the sort runs first.
	if orderApplies(e) {
		words = orderWords(e, words)
	}
	return words, isList, markJoin && escapeSep != nil, true
}

// assignThroughFlags is the assignment side of `${(U)u:=def}` and of
// `${(U)v::=abc}`: the word is stored as written and the flags apply only to
// what is substituted, which is measured — the first leaves `def` behind and
// expands to `DEF`, the second leaves `abc` and expands to `ABC`.
//
// One function for both operators so the two cannot drift: the only
// difference between them is *whether* this runs, which is the caller's
// question and not this one's.
func (r *Runner) assignThroughFlags(e *syntax.ParamExpr) ([]string, bool, bool) {
	v := r.joinWord(e.Arg)
	switch {
	case e.Index != nil && !r.wholeArrayIndex(e):
		r.assignSubscript(e, v)
	case e.Op == syntax.ParamAssignAlways:
		// The name check belongs to the operator that always assigns, for
		// the reason it does on the path without a flag group: a `${(U)#::=w}`
		// that answered `W` would be an assignment to nothing at status 0.
		if !r.assignableParamName(e.Name) {
			return nil, false, false
		}
		r.setVar(e.Name, v)
	case e.Name != "":
		r.setVar(e.Name, v)
	}
	return []string{v}, false, true
}

// isAssignOp reports whether an operator assigns — the conditional `=` and
// `:=`, which assign when their test fires, and `::=`, which always does.
//
// The `(A)` flag's refusal asks this rather than naming ParamAssign, because
// the flag's job is to say what *kind* of parameter the assignment leaves
// behind and every operator that assigns has that question. Measured
// 2026-09-07 on zsh 5.9.2: `unset u; ${(A)u=x y}` and `unset u; ${(A)u::=x y}`
// both leave `typeset -a u=( 'x y' )`.
func isAssignOp(op syntax.ParamOp) bool {
	return op == syntax.ParamAssign || op == syntax.ParamAssignAlways
}

// flagKeepsFields reports whether a double-quoted result keeps one field per
// word: the `@` flag asks for it, and `$@` and an `[@]` subscript already
// have it — `"${(U)@}"` keeps its fields exactly as `"$@"` does.
func (r *Runner) flagKeepsFields(e *syntax.ParamExpr) bool {
	if strings.ContainsRune(e.Flags, '@') || e.Name == "@" {
		return true
	}
	return e.Index != nil && r.atArrayIndex(e)
}

// flagJoinSep is what joining uses: the `j` argument when one was given, and
// the first character of IFS — a space by default — when not.
func (r *Runner) flagJoinSep(e *syntax.ParamExpr) string {
	if strings.ContainsRune(e.Flags, 'j') {
		return r.flagArgument(e, 'j', e.JoinSep)
	}
	return ifsFirst(r.ifs())
}

// SetFlagArgumentEscapes installs the escape set the `(p)` expansion flag
// reads a following flag's argument with, so `${(pj:\n:)a}` joins on a real
// newline.
//
// A function the dialect supplies rather than a table here, because "the
// escapes" is not one answer even inside one shell: the same shell's `echo`
// keeps the backslash on a letter it does not know and needs `\0` in front of
// an octal number, where its `print` drops the backslash and takes `\101`. A
// flag argument is read with the second of those, measured — and with one
// documented exception, which the decoder handed here has to carry: `\c` ends
// `print`'s output and is an ordinary unknown escape in a flag argument, so
// `${(pj:A\cB:)a}` joins on `AcB`.
//
// Nil is the runner nobody told, and there `(p)` is refused by name. That is
// deliberate: a `(p)` read as a no-op joins on a backslash and an `n` at
// status 0, which is the plausible-answer failure this codebase minds most.
func (r *Runner) SetFlagArgumentEscapes(decode func(string) string) {
	r.flagArgEscapes = decode
}

// paramFlagCarried reports whether this runner carries a flag letter.
//
// All but one are answered by the constant above. `p` is answered by whether
// the dialect supplied the escape set its arguments are read with, because
// that set is a measurement about one shell and this package holds nobody's —
// so a runner nobody told refuses the letter by name instead of reading it as
// a no-op that joins on a backslash and an `n` at status 0.
func (r *Runner) paramFlagCarried(c rune) bool {
	if c == 'p' {
		return r.flagArgEscapes != nil
	}
	return strings.ContainsRune(implementedParamFlags, c)
}

// flagArgument is what an argument-taking flag's argument comes to: the text
// as written, unless a `p` was written *in front of that flag*, which is the
// whole of what the `(p)` flag does.
//
// Two readings, and they are alternatives rather than steps. Measured
// 2026-09-07 on zsh 5.9.2, the only shell in the panel with the flag, with
// `a=(x y)`:
//
//	s=-;     ${(pj:$s:)a}    x-y       an argument that is one `$name`
//	s='\n';  ${(pj:$s:)a}    x\ny      and the value is *not* then escaped
//	         ${(pj:\n:)a}     x<LF>y    an argument with an escape in it
//	         ${(pj:\$s:)a}    x$sy      which is why the name is read raw
//	         ${(pj:$s\t:)a}   x$s<TAB>  and why one `$name` means the whole
//	         ${(j:\n:)a}      x\ny      no `p`: neither reading runs
//	         ${(j:$s:p)a}     x$sy      and `p` behind the flag is neither
//
// The fourth row is what fixes the order: `\$s` decodes to `$s`, so a reading
// that escaped first and looked for a name second would substitute there, and
// the shell does not. The second says the substituted value is handed over
// verbatim. So: one `$name` that is set, else the escapes, never both.
//
// The name may be a variable, an array — whose elements arrive joined — or a
// positional. Anything else stays as written, `$#` and `$@` and a bare `$`
// included, and so does a name that is not set: measured, `${(pj:$nosuch:)a}`
// joins on the five characters `$nosuch`.
func (r *Runner) flagArgument(e *syntax.ParamExpr, flag rune, raw string) string {
	if !precededByPrintFlag(e.Flags, flag) {
		return raw
	}
	if name, ok := soleParameterReference(raw); ok {
		if isPositional(name) {
			// A positional is always substituted, out of range included,
			// where a *name* has to be set. Measured with `set -- P Q`:
			// `${(pj:$2:)a}` joins on `Q`, `${(pj:$9:)a}` joins on nothing
			// at all, and `${(pj:$nosuch:)a}` joins on the seven characters
			// `$nosuch`. `$0` is a positional here and answers with the
			// script's own name.
			v, _ := r.specialParam(&syntax.ParamExpr{Name: name})
			return v
		}
		if v, set := r.getVar(name); set {
			return v
		}
	}
	return r.flagArgEscapes(raw)
}

// precededByPrintFlag reports whether a `p` was written before this flag in
// the group. The order is the rule and not a convenience: measured,
// `${(pj:\n:)a}` joins on a newline and `${(j:\n:p)a}` on a backslash and an
// `n`, so a `p` behind the flag it would modify modifies nothing.
func precededByPrintFlag(flags string, flag rune) bool {
	for _, c := range flags {
		if c == flag {
			return false
		}
		if c == 'p' {
			return true
		}
	}
	return false
}

// soleParameterReference reports the name in an argument that is exactly
// `$name`, and false for anything else.
//
// Exactly: `$s` is the name and `A$s`, `$sA`, `$s $s`, `$s\t`, `${s}`, `$M[k]`,
// `$(echo -)` and a bare `$` are all not — every one of them measured left as
// written. So this is a narrow reading on purpose, and widening it to
// "expand the argument" would substitute in six places the shell does not.
func soleParameterReference(arg string) (string, bool) {
	name, ok := strings.CutPrefix(arg, "$")
	if !ok || name == "" {
		return "", false
	}
	if !isNameLike(name) && !isPositional(name) {
		return "", false
	}
	return name, true
}

// splitFlagged splits one word the way the group asked: `f` at newlines, `s`
// at its separator, and an empty separator into characters.
func (r *Runner) splitFlagged(w string, e *syntax.ParamExpr) []string {
	sep := "\n"
	if strings.ContainsRune(e.Flags, 's') {
		sep = r.flagArgument(e, 's', e.SplitSep)
	}
	if sep == "" {
		if w == "" {
			// One empty field rather than none. `strings.Split` on any
			// non-empty separator already answers this way — `${(f)v}` and
			// `${(s.:.)v}` on an empty value are one empty field — and the
			// character split has to agree, because whether the field
			// survives is then the *edge* question and not a second rule.
			// Measured: `v=""; set -- "${(s::)v}"` is one parameter in the
			// shell that has the flag, and none unquoted.
			return []string{""}
		}
		out := make([]string, 0, len(w))
		for _, c := range w {
			out = append(out, string(c))
		}
		return out
	}
	return strings.Split(w, sep)
}

// splitFlagEdges reports whether this expansion keeps the empty field at each
// edge of what `(f)` or `(s)` split.
//
// Measured against zsh 5.9.2 with `(s.:.)` and a colon separator, quoted and
// without `@`, which is the reading that has an answer of its own — `(@)`
// keeps every empty field and unquoted keeps none:
//
//	""       1  ['']            an empty value is one empty field
//	":"      2  ['' '']         both edges, and no interior field between
//	"::"     2  ['' '']         the interior one is dropped
//	":::"    2  ['' '']         and so is a run of them
//	"a:"     2  ['a' '']        the trailing edge is kept
//	":a"     2  ['' 'a']        so is the leading one
//	":a:"    3  ['' 'a' '']     both, around a field that is not empty
//	"a::b"   2  ['a' 'b']       the interior one is dropped
//	"a:::b"  2  ['a' 'b']
//	"a::"    2  ['a' '']        interior dropped, trailing kept
//	"::a"    2  ['' 'a']        leading kept, interior dropped
//
// So it is the edges that are kept and the interior that goes, which is the
// same shape `${=spec}` already had for an IFS split (splitFieldsEdges) and
// is why this is a rule about *which* empty field rather than about whether
// empty fields survive at all. Dropping every one of them answered `n=0` for
// `":"` where the shell says 2, and 0 for an empty value where it says 1 —
// a plausible count at status 0, which is the failure this codebase minds
// most (#1097).
func splitFlagEdges(e *syntax.ParamExpr, quoted bool) bool {
	return quoted && (strings.ContainsAny(e.Flags, "fs") || shellSplitActive(e))
}

// flagBase is the value the pipeline starts from: the words, whether the
// parameter was set, and whether the value is a list rather than a scalar.
func (r *Runner) flagBase(e *syntax.ParamExpr) (words []string, set, isList bool) {
	if e.Inner != nil {
		// An expansion standing where a name would. The flags then apply to
		// what it came to, which is the same rule they follow for a name —
		// `${(U)${v}}` and `${${(U)v}}` are both `ABC`, measured.
		//
		// And they apply to a *list* the same way, which is the whole of
		// `${(j: :)${(qkv)m[@]}}`: the inner is one field per key and per
		// value, and the join then runs over all of them. Reporting a list
		// as a scalar joined it here, before the group ever saw it, so the
		// separator went in once around a value that already held them all.
		words, set, isList = r.nestedWords(e)
		return words, set, isList
	}
	if e.Index != nil {
		if list, lok := r.arraySubscript(e); lok {
			if r.wholeArrayIndex(e) {
				if _, isAssoc := r.assocFor(e.Name); isAssoc {
					// `${(kv)m[@]}` is `${(kv)m}`: a whole-array subscript
					// on an association selects every pair, and `k` and `v`
					// then say which half of each pair is substituted.
					// namedBase is where that is decided, and reaching it is
					// what keeps the subscripted spelling from having a
					// second answer of its own — this path took the *values*
					// whatever the letters said, so `${(qkv)ICE[@]}` came
					// back one field per value, half the list, at status 0.
					return r.namedBase(e.Name, e.Flags)
				}
				return list, list != nil, true
			}
			if key, kok := r.assocSubscriptKey(e, list); kok {
				// `${(k)m[key]}` substitutes the *key* rather than what it
				// holds — measured, `${(k)m[b]}` is `b` where `${m[b]}` is
				// `2` — and only `k` on its own does: `${(kv)m[b]}` and
				// `${(v)m[b]}` are both the value, so `v` beside `k` puts
				// the pair's other half back. An absent key is nothing at
				// all under either spelling and never reaches here.
				return []string{key}, true, false
			}
			if r.assocSearchSubscript(e) {
				// A search over an association hands the group a *list*, so
				// `${(on)m[(I)pat]}` sorts the matches rather than sorting
				// one word made of all of them. And it is set even with no
				// match — measured, `${m[(I)zz]-none}` is empty where
				// `${m[zz]-none}` is `none`, so a search always answers.
				//
				// The `true` says that outright rather than reading it off
				// the slice. It is the same value `list != nil` has here,
				// because the only thing that reaches this branch is
				// assocSearchWords and its result comes from `make`, which
				// never yields nil — the two cases, no matches and some, are
				// the whole of it. Written this way because the set-ness is a
				// fact about the *construct* and would survive somebody
				// changing what an empty search returns, and the non-nil
				// slice is separately load-bearing next door: paramSource
				// reads `elems != nil` for the same question on the route
				// with no flag group.
				return list, true, true
			}
			return []string{strings.Join(list, " ")}, list != nil, false
		}
	}
	return r.namedBase(e.Name, e.Flags)
}

// namedBase resolves a name to its words. flags matters for an associative
// array, where `k` substitutes the keys and `kv` key and value as two
// consecutive words each; the keys come sorted, the same deterministic order
// `${m[@]}` already yields where the shells promise none at all.
func (r *Runner) namedBase(name, flags string) (words []string, set, isList bool) {
	switch name {
	case "@", "*":
		return append([]string(nil), r.Params...), len(r.Params) > 0, true
	case "":
		return []string{""}, false, false
	}
	if a, aok := r.assocFor(name); aok {
		hasK := strings.ContainsRune(flags, 'k')
		hasV := strings.ContainsRune(flags, 'v')
		switch {
		case hasK && hasV:
			keys := a.keys()
			out := make([]string, 0, 2*len(keys))
			for _, k := range keys {
				out = append(out, k, a[k])
			}
			return out, len(a) > 0, true
		case hasK:
			return a.keys(), len(a) > 0, true
		default:
			return a.values(), len(a) > 0, true
		}
	}
	if elems, pok := r.pipelineStatuses(name); pok {
		return elems, true, true
	}
	if _, aok := r.Arrays[name]; aok {
		elems, _ := r.arrayElems(name)
		return elems, true, true
	}
	if produce, dok := r.DynamicArrays[name]; dok {
		return produce(r), true, true
	}
	if v, sok := r.specialParam(&syntax.ParamExpr{Name: name}); sok {
		return []string{v}, true, false
	}
	v, vok := r.getVar(name)
	return []string{v}, vok, false
}

// assocSubscriptKey is the key a `${(k)m[key]}` substitutes in place of the
// value the same subscript would have read, and whether that is what this
// expansion asked for.
//
// Three conditions, each measured on zsh 5.9.2 with `typeset -A m=(a 1 b 2)`:
//
//	${(k)m[b]}   b   the key, on an association and with `k` alone
//	${(kv)m[b]}  2   `v` beside it puts the value back
//	${(v)m[b]}   2   and `v` on its own changes nothing
//	${(k)m[zz]}      an absent key is nothing, not the key that was asked for
//
// The last is why the elements are passed in rather than looked up again:
// nil is how assocSubscript says the element was not there, and a key
// substituted for a missing element would answer `zz` where the shell
// answers with no field at all.
//
// An ordinary array reads the same letter as its *index* — `${(k)x[2]}` is
// `2` there, and `${(k)x[-1]}` is the subscript counted forward — which is a
// different question with a different source, and this answers false for it
// rather than guessing. See #1515.
func (r *Runner) assocSubscriptKey(e *syntax.ParamExpr, elems []string) (string, bool) {
	if elems == nil || !e.HasFlags {
		return "", false
	}
	if !strings.ContainsRune(e.Flags, 'k') || strings.ContainsRune(e.Flags, 'v') {
		return "", false
	}
	if _, isAssoc := r.assocFor(e.Name); !isAssoc {
		return "", false
	}
	if e.IndexFlags != nil {
		// A flag group selects by matching rather than by naming, and which
		// half of each match it substitutes is assocSearchWords' question —
		// already answered there, for the same two letters.
		return "", false
	}
	return r.assocKey(e.Subscript()), true
}

// applyFlagOp runs the expansion's operator over the flagged value — the
// same operators expandParam applies, elementwise where the value is a list.
// ok is false when the expansion was fatal.
func (r *Runner) applyFlagOp(e *syntax.ParamExpr, words []string, set, isList bool) ([]string, bool, bool) {
	fires := !set
	if e.Colon {
		fires = !set || strings.Join(words, "") == ""
	}
	switch e.Op {
	case syntax.ParamNone:
	case syntax.ParamDefault:
		if fires {
			return []string{r.joinWord(e.Arg)}, false, true
		}
	case syntax.ParamAssign:
		if fires {
			return r.assignThroughFlags(e)
		}
	case syntax.ParamAssignAlways:
		// No test, so the assignment is the only branch there is. The flags
		// still apply to what is substituted and not to what is stored:
		// measured, `${(U)v::=abc}` is `ABC` and leaves `abc` behind.
		return r.assignThroughFlags(e)
	case syntax.ParamAlternate:
		if fires {
			return []string{""}, false, true
		}
		return []string{r.joinWord(e.Arg)}, false, true
	case syntax.ParamError:
		if fires {
			r.fatalParamError("%s\n", Wording(r.diag().ParamErrorMessage, "%[1]s: %[2]s",
				e.Name, r.paramErrorWord(e, set)))
			return nil, false, false
		}
	case syntax.ParamTrimPrefix, syntax.ParamTrimPrefixLong,
		syntax.ParamTrimSuffix, syntax.ParamTrimSuffixLong:
		pattern := r.patternOf(e.Arg)
		// Rule: `M` substitutes what the pattern *took* rather than what it
		// left. The same operator and the same match, read from the other
		// side — measured, `${(M)v#h*l}` on `hello` is `hel` and
		// `${(M)v##h*l}` is `hell`, so the shortest/longest choice is still
		// the operator's.
		take := r.trimWith
		if matchingFlag(e) {
			take = r.matchedWith
		}
		for i, w := range words {
			words[i] = take(w, pattern, e.Op)
		}
	case syntax.ParamReplace:
		pattern := r.patternOf(e.Arg)
		for i, w := range words {
			words[i] = r.replaceWith(w, pattern, e)
		}
	case syntax.ParamSubstring:
		if isList {
			return sliceElems(words, r.numOf(e.Arg, e, e.Arg2), e, r), true, true
		}
		words[0] = r.substringRange(words[0], e)
	case syntax.ParamExclude, syntax.ParamSetDifference, syntax.ParamSetIntersection:
		if isList {
			return r.selectElements(e, words), true, true
		}
		// Not a list: `${(U)v:#p}` asks the same question of one value, and
		// the answer is that value or nothing. It stays a scalar rather than
		// becoming an empty list, so `"${v:#p}"` is one empty field the way
		// `"${v#p}"` is.
		words[0] = r.selectScalar(e, words[0])
	case syntax.ParamUpper, syntax.ParamLower, syntax.ParamToggle,
		syntax.ParamUpperFirst, syntax.ParamLowerFirst, syntax.ParamToggleFirst:
		for i, w := range words {
			words[i] = r.changeCase(w, e)
		}
	}
	return words, isList, true
}

// convertCase is the `U` and `L` flags: every letter, under the same locale
// policy the case-changing operators follow — an explicit C locale narrows
// to ASCII and anything else is Unicode-aware.
func (r *Runner) convertCase(v string, upper bool) string {
	convert := unicode.ToUpper
	if !upper {
		convert = unicode.ToLower
	}
	if r.localeIsC() {
		wide := convert
		convert = func(c rune) rune {
			if c < 0x80 {
				return wide(c)
			}
			return c
		}
	}
	return strings.Map(convert, v)
}

// PromptExpand is the prompt-escape language over one string, for a builtin
// whose whole job is the `${(%)…}` flag under another spelling.
//
// Exported so that `print -P` is the *same* expansion rather than a second
// one. Two tables of prompt escapes is how the two answers drift apart, and
// the escapes this shell carries are few enough that the drift would be
// invisible until a script wrote the one they disagreed about.
//
// The second result is false where an escape was refused; the refusal has
// already been written, and the caller's business is only to stop.
func (r *Runner) PromptExpand(text string) (string, bool) {
	return r.promptEscapes(text, nil)
}

// promptEscapes is the `%` flag over one word.
//
// The dialect's table and the one walker, which is the whole of #1090: this
// used to carry four escapes of its own — `%%`, `%x`, `%N`, `%n` — and refuse
// the rest by name, while the prompt drawer read a second table of about
// forty. So `print -P '%F{196}…'` was refused and a drawn prompt answered it.
// The four are rows of the one table now: `%x` and `%N` became
// FieldSourceFile and FieldUnitName, and `%%` and `%n` were already there as
// FieldEscape and FieldUser.
//
// The refusal stays, and it is still the promise: an escape this shell cannot
// answer is named rather than dropped. What names it is now the resolver
// saying it has no answer — for the session's own facts, which a Runner has
// not got — or the table listing the code as Unsupported, for the shapes
// needing a mechanism rather than a value. See interp/prompt.go.
//
// e is the expansion this is the `%` flag of, and nil where a *builtin*
// asked — see PromptExpand. It decides only how a refusal names the place it
// happened.
func (r *Runner) promptEscapes(v string, e *syntax.ParamExpr) (string, bool) {
	out, code, ok := ExpandPromptStyle(r.promptStyle, v, r.promptField)
	if !ok {
		return r.refusePromptEscape(e, code)
	}
	return out, true
}

// refusePromptEscape says, by name, that an escape is not carried here.
//
// One place rather than two, because the wording is the promise: it names the
// escape the script asked for, so a reader can tell which of several in one
// word was the one this shell could not answer.
//
// The expansion route quotes the whole construct back, because `${(%)…}` can
// hold several words and a reader needs to know which. A builtin has already
// been named by the location the dialect writes, so it says the sentence
// alone — and it does not set expandErr, because nothing is being expanded
// and the builtin's own status is the answer.
//
// Setting it there would in fact change nothing observable: the flag is
// cleared at the start of every command, so a builtin cannot leak it into
// the next one, and the builtin's own operands were expanded before it ran.
// It is left out because it would be false rather than because it would
// break, and a mutation that puts it back survives for that reason.
func (r *Runner) refusePromptEscape(e *syntax.ParamExpr, c rune) (string, bool) {
	if e == nil {
		r.diagf("the %%%c prompt escape is not implemented\n", c)
		return "", false
	}
	r.diagf("${%s}: the %%%c prompt escape is not implemented\n", e.Src, c)
	r.expandErr = true
	return "", false
}

// promptUnitName is `%N`: the name of the function, sourced file or script
// being read — the function's *name* where `%x` stays its defining file.
func (r *Runner) promptUnitName() string {
	if len(r.frames) > 0 {
		f := r.frames[len(r.frames)-1]
		if f.Name != "" && f.Name != sourceFrameName {
			return f.Name
		}
		if f.File != "" {
			return f.File
		}
	}
	if r.scriptFile != "" {
		return r.scriptFile
	}
	return r.name()
}

// quoteFlagged is the `q` family, one style per count — all measured:
// backslashes, then single quotes, double quotes, and `$'…'` — with the
// minimal style of `q-` reached through the same door rather than beside it,
// so that a caller cannot pick the count and forget the modifier. See
// interp/minimalquote.go.
func quoteFlagged(v string, count int, minimal bool) string {
	if minimal {
		return quoteMinimal(v)
	}
	switch count {
	case 1:
		return quoteWithBackslashes(v)
	case 2:
		return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
	case 3:
		var b strings.Builder
		b.WriteByte('"')
		for i := 0; i < len(v); i++ {
			if strings.IndexByte("\\`\"$", v[i]) >= 0 {
				b.WriteByte('\\')
			}
			b.WriteByte(v[i])
		}
		b.WriteByte('"')
		return b.String()
	default:
		var b strings.Builder
		b.WriteString("$'")
		eachQuotableByte(v, func(_ int, c byte) {
			switch {
			case c == '\'':
				b.WriteString(`\'`)
			case c == '\\':
				b.WriteString(`\\`)
			case c == '!':
				b.WriteString(`\!`)
			case c < 0x20 || c >= 0x7f:
				b.WriteString(controlEscape(c))
			default:
				b.WriteByte(c)
			}
		}, func(_ int, raw string) { b.WriteString(raw) })
		b.WriteString("'")
		return b.String()
	}
}

// quoteWithBackslashes is the single-`q` style: the characters the shell
// gives meaning to are escaped, each control or non-UTF-8 byte becomes its
// own `$'…'` segment, and an empty value is `”` — every detail measured.
//
// Which characters those are is the table in interp/minimalquote.go, shared
// with `q-` and reached by the `:q` modifier through this function, because
// the question all three ask is the same one and a second copy of the answer
// is how two of them come to disagree. The table is where the two start-only
// specials live: measured, `${(q)…}` on `a~b` is `a~b` and on `~x` is `\~x`.
func quoteWithBackslashes(v string) string {
	if v == "" {
		return "''"
	}
	var b strings.Builder
	eachQuotableByte(v, func(i int, c byte) {
		switch {
		case c < 0x20 || c >= 0x7f:
			// Control bytes and bytes that are not UTF-8 alike — measured,
			// `$'\177'` and `$'\377'`. Ahead of the table on purpose: a tab
			// and a newline are in it, and here they are `$'\t'` and `$'\n'`
			// rather than a backslash and a raw byte.
			b.WriteString("$'" + controlEscape(c) + "'")
		case quotableByte(i, c):
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}, func(_ int, raw string) { b.WriteString(raw) })
	return b.String()
}

// eachQuotableByte walks a string handing single bytes — ASCII, and any byte
// that is not part of a valid multibyte rune — to one function and whole
// multibyte runes to the other, because quoting escapes bytes while UTF-8
// passes through untouched.
//
// Both are given the offset the piece starts at, because two of the bytes
// that need quoting need it only at offset 0.
func eachQuotableByte(v string, one func(i int, c byte), run func(i int, s string)) {
	for i := 0; i < len(v); {
		if v[i] < utf8.RuneSelf {
			one(i, v[i])
			i++
			continue
		}
		c, size := utf8.DecodeRuneInString(v[i:])
		if c == utf8.RuneError && size == 1 {
			one(i, v[i])
			i++
			continue
		}
		run(i, v[i:i+size])
		i += size
	}
}

// controlEscape writes one control byte the way `$'…'` spells it: the seven
// named escapes by name and everything else as three-digit octal — `$'\033'`
// for escape rather than `\e`, which is measured.
func controlEscape(c byte) string {
	switch c {
	case '\a':
		return `\a`
	case '\b':
		return `\b`
	case '\f':
		return `\f`
	case '\n':
		return `\n`
	case '\r':
		return `\r`
	case '\t':
		return `\t`
	case '\v':
		return `\v`
	}
	return fmt.Sprintf(`\%03o`, c)
}

// matchingFlag reports whether the `M` flag was written, which turns the
// operators that *remove* what a pattern matched into ones that keep it.
//
// It reaches exactly two of them, measured across every operator the flag
// group may stand in front of: the four trims, where it substitutes the
// matched part, and `:#`, where it keeps the matching elements instead of
// dropping them. On `/`, `:|`, `:*`, a substring, the conditionals and an
// expansion with no operator at all it does nothing — which is why there is
// no third call site rather than an oversight.
func matchingFlag(e *syntax.ParamExpr) bool {
	return e != nil && strings.ContainsRune(e.Flags, 'M')
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/blairham/sh/syntax"
)

// expandWord turns one word into zero or more fields.
//
// It follows the ordering in docs/spec/grammar/expansion.md, as far as this
// slice goes: parameter expansion, then field splitting of the *unquoted*
// results only. Pathname expansion and quote removal beyond what the lexer
// already did are not here yet.
//
// A word is a sequence of spans rather than a string precisely so this stage
// can tell which parts were quoted. Splitting applies only to the unquoted
// ones, which is why a"b c"d is one field and $x with a space in it is two.
func (r *Runner) expandWord(w *syntax.Word) []string {
	if w == nil {
		return nil
	}
	// Brace expansion comes first and can turn one word into several, so it
	// wraps the rest rather than being a stage inside it.
	if words := braceExpand(w); len(words) > 1 &&
		r.ask(r.sem().BraceExpansion, "brace expansion") {
		var out []string
		for _, bw := range words {
			out = append(out, r.expandOneWord(bw)...)
		}
		return out
	}
	return r.expandOneWord(w)
}

// expandOneWord is the pipeline for a single word, after braces.
func (r *Runner) expandOneWord(w *syntax.Word) []string {
	if w == nil {
		return nil
	}
	r.expandTilde(w)
	r.expandEquals(w)

	// Fields are built up span by span. A span joins onto the field before it
	// unless splitting started a new one, which is what makes x$(f)y attach
	// its literal text to the first and last resulting fields.
	fields := []string{""}
	any := false

	for _, s := range w.Spans {
		// `$@` is the one expansion that yields more than one field on its
		// own, so it cannot go through expandSpan, which returns a string.
		// The first parameter joins onto whatever precedes it and the last
		// stays open for whatever follows — which is why `x$@y` attaches its
		// literal text to the first and last fields rather than becoming
		// words of its own.
		if parts, ok := r.expandAt(s); ok {
			if len(parts) == 0 {
				continue
			}
			any = true
			fields[len(fields)-1] += parts[0]
			fields = append(fields, parts[1:]...)
			continue
		}
		text, split := r.expandSpan(s)
		if !split {
			fields[len(fields)-1] += text
			any = any || text != "" || s.Quoting != syntax.Unquoted
			continue
		}
		ifs, set := r.ifs()
		parts := splitFields(text, ifs, set)
		if len(parts) == 0 {
			// An unquoted expansion of an empty value produces no field at
			// all, so nothing is appended and nothing is started.
			continue
		}
		any = true
		fields[len(fields)-1] += parts[0]
		fields = append(fields, parts[1:]...)
	}

	if len(fields) == 1 && fields[0] == "" && !any {
		return nil
	}

	// Pathname expansion is the last stage, and it acts on whole fields: a
	// pattern that matches nothing is passed through unchanged.
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if matches := r.glob(f); len(matches) > 0 {
			out = append(out, matches...)
			continue
		}
		out = append(out, globUnescape(f))
	}
	return out
}

// expandWordNoSplit expands a word without field splitting or globbing, for
// the contexts that have neither: inside `[[ ]]`, and a redirection target. It
// is a separate entry point rather than a flag on the runner because the
// caller knows which context it is in and the expander should not have to
// guess.
//
// A tilde still expands. Not splitting is not the same as not expanding, and
// leaving it out made `[[ -f ~/x ]]` false in a home directory that has the
// file — which every shell with `[[ ]]` answers true.
func (r *Runner) expandWordNoSplit(w *syntax.Word) []string {
	if w == nil {
		return nil
	}
	r.expandTilde(w)
	var b strings.Builder
	for _, s := range w.Spans {
		if parts, ok := r.expandAt(s); ok {
			b.WriteString(strings.Join(parts, " "))
			continue
		}
		text, _ := r.expandSpan(s)
		b.WriteString(text)
	}
	return []string{globUnescape(b.String())}
}

// expandRedirectTargetViews expands a redirection's target once and returns
// both readings of it: the fields an ordinary word would have become, and the
// text it comes to when nothing is split or matched.
//
// One pass, because the two readings must not each run the command
// substitutions in `> $(f)`. And a pass of its own rather than two calls,
// because splitting is quoting-aware — `"$e"` with a space in it is one field
// and `$e` is two — so the unsplit text cannot be recovered by joining the
// fields, and the fields cannot be recovered by splitting the text.
func (r *Runner) expandRedirectTargetViews(w *syntax.Word) (fields []string, plain string) {
	if w == nil {
		return nil, ""
	}
	r.expandTilde(w)

	fields = []string{""}
	any := false
	var b strings.Builder

	for _, s := range w.Spans {
		if parts, ok := r.expandAt(s); ok {
			b.WriteString(strings.Join(parts, " "))
			if len(parts) == 0 {
				continue
			}
			any = true
			fields[len(fields)-1] += parts[0]
			fields = append(fields, parts[1:]...)
			continue
		}
		text, split := r.expandSpan(s)
		b.WriteString(text)
		if !split {
			fields[len(fields)-1] += text
			any = any || text != "" || s.Quoting != syntax.Unquoted
			continue
		}
		ifs, set := r.ifs()
		parts := splitFields(text, ifs, set)
		if len(parts) == 0 {
			continue
		}
		any = true
		fields[len(fields)-1] += parts[0]
		fields = append(fields, parts[1:]...)
	}
	plain = globUnescape(b.String())

	if len(fields) == 1 && fields[0] == "" && !any {
		return nil, plain
	}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if matches := r.glob(f); len(matches) > 0 {
			out = append(out, matches...)
			continue
		}
		out = append(out, globUnescape(f))
	}
	return out, plain
}

// substitutedWordFields expands the word a `-` or `+` substituted, keeping the
// fields it produces rather than joining them.
//
// It answers false when this expansion is not one of those, or when the word
// is not what it came to — both of which leave the caller to carry on as
// before. The test for which it came to is testFires, the same one
// expandParam applies, so the two cannot drift apart.
func (r *Runner) substitutedWordFields(s syntax.Span) ([]string, bool) {
	e := s.Param
	if e.Arg == nil || e.Length || e.Indirect {
		return nil, false
	}
	if e.Op != syntax.ParamDefault && e.Op != syntax.ParamAlternate {
		return nil, false
	}
	// Whatever the parameter is — a plain name, one element, or the whole
	// array. The fields come from the *word*, so `${x+${b[@]}}` and
	// `${a[0]+${b[@]}}` are two fields exactly as `${a[@]+${b[@]}}` is.
	// Measured in bash and ksh93; restricting this to `[@]` made the first
	// two come back joined.
	fires := r.testFires(e)
	if (e.Op == syntax.ParamDefault && !fires) ||
		(e.Op == syntax.ParamAlternate && fires) {
		// The parameter is what it came to, not the word.
		return nil, false
	}
	// expandWord either way: it builds fields span by span, so a literal
	// stays one field and a nested `${b[@]}` contributes its own — which is
	// the difference between `"${a[@]+p q}"` being one field and
	// `"${a[@]+${a[@]}}"` being two. Each span carries the quoting it was
	// written with, so the outer quotes need no separate handling.
	fields := r.expandWord(e.Arg)
	if s.Quoting != syntax.Unquoted {
		return escapeAll(fields), true
	}
	return fields, true
}

// paramSource is the value an expansion starts from and whether it was set at
// all, before any operator is applied.
//
// Shared with the two field-level questions below, so that "was it set" is
// asked in one place. Answering it twice is how the joined and the split paths
// would come to disagree about the same expansion.
func (r *Runner) paramSource(e *syntax.ParamExpr) (value string, set, subscript bool) {
	if e.Index != nil {
		if elems, ok := r.arraySubscript(e); ok {
			// nil rather than empty is what says the element was not there:
			// an element holding "" is set, and `${a[0]:-d}` has to tell the
			// two apart.
			return strings.Join(elems, " "), elems != nil, true
		}
	}
	// A special parameter supplies a *value*; it does not skip the operators.
	// Returning here was a bug: `${1##*/}` left its argument untouched,
	// because the positional parameter answered and the trim never ran.
	value, set = r.specialParam(e)
	if !set {
		value, set = r.getVar(e.Name)
	}
	return value, set, false
}

// yieldsTheArray reports whether a `-` or `+` expansion came to the parameter
// rather than to its word.
//
// The mirror of substitutedWordFields, and needed for the same reason:
// `"${a[@]-${a[@]}}"` on a set array is the *array*, and it keeps its fields
// exactly as `"${a[@]}"` does. Without this it fell to the scalar path and
// came back as one joined string.
func (r *Runner) yieldsTheArray(e *syntax.ParamExpr) bool {
	if e.Op != syntax.ParamDefault && e.Op != syntax.ParamAlternate {
		return false
	}
	fires := r.testFires(e)
	return (e.Op == syntax.ParamDefault && !fires) ||
		(e.Op == syntax.ParamAlternate && fires)
}

// testFires reports whether the `-`/`+` test fires: unset, or unset-or-empty
// when a colon was written. The same rule expandParam applies, from the same
// source, so the joined and the split paths cannot disagree.
func (r *Runner) testFires(e *syntax.ParamExpr) bool {
	value, set, _ := r.paramSource(e)
	if e.Colon {
		return !set || value == ""
	}
	return !set
}

// expandAssignValue expands the value of an assignment.
//
// An assignment is a tilde context and is not a splitting or a globbing one:
// `PATH=~/bin` expands, `n=*` stores the character, and `IFS=:; x=$y` with
// `y=a:b` stores `a:b` rather than `a b`. Sending it through the ordinary word
// pipeline did all three wrong — the fields were split and rejoined on a
// space, and a value that looked like a pattern was replaced by the directory
// listing.
//
// Array elements are not this: `a=(*.txt)` does glob, because each element is
// an ordinary word.
func (r *Runner) expandAssignValue(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	r.expandTilde(w)
	return strings.Join(r.expandWordNoSplit(w), "")
}

// expandAt handles `$@`, the only expansion that produces several fields by
// itself. Quoted, it is one field per parameter, each keeping its own spaces;
// with no parameters it is *zero* fields, which is why `set -- "$@"` is safe
// on an empty list and `set -- "$*"` is not.
func (r *Runner) expandAt(s syntax.Span) ([]string, bool) {
	if s.Kind != syntax.ParamExp || s.Param == nil {
		return nil, false
	}
	e := s.Param
	// `${!prefix@}` and `${!prefix*}` yield the *names* that begin with the
	// prefix, and the two spellings differ exactly as `$@` and `$*` do.
	if e.Prefix != 0 {
		names := r.namesWithPrefix(e.Name)
		ifs, set := r.ifs()
		if e.Prefix == '*' {
			joined := strings.Join(names, ifsFirst(ifs, set))
			if s.Quoting != syntax.Unquoted {
				return []string{globEscape(joined)}, true
			}
			return splitFields(joined, ifs, set), true
		}
		if s.Quoting != syntax.Unquoted {
			return escapeAll(names), true
		}
		return names, true
	}
	// `${a[@]+word}` and `${a[@]-word}` substitute the *word*, and it keeps
	// its own fields: `"${a[@]+${a[@]}}"` is two fields for a two-element
	// array in all three shells that have arrays, not one joined string.
	// That is the whole point of the idiom — it is how a script expands a
	// possibly-empty array under `set -u` without collapsing it.
	//
	// Only when the word is what the expansion came to. When the *parameter*
	// is what it came to, the array path below is the one that gives its
	// fields.
	if fields, ok := r.substitutedWordFields(s); ok {
		return fields, true
	}
	// `${a[@]}` is one field per element for the same reason `"$@"` is one
	// per parameter: joining them would lose an element containing a space.
	// ParamSubstring as well as ParamNone: `${a[@]:1}` is a slice of the
	// *list*, not a substring of the elements joined together, and taking it
	// down the scalar path is what made it come back as the whole array.
	//
	// Only for `[@]` and `[*]`, though. `${a[0]:1}` names one element and is
	// a substring of it — slicing there is a one-element list with its first
	// element dropped, which is no field at all. The corpus caught that.
	if e.Index != nil && !e.Length &&
		(e.Op == syntax.ParamNone ||
			((e.Op == syntax.ParamSubstring || r.yieldsTheArray(e)) &&
				wholeArraySubscript(r.subscriptText(e.Index)))) {
		if elems, ok := r.arraySubscript(e); ok {
			if e.Indirect {
				// `${!a[@]}` is the array's *subscripts*, not its elements —
				// and the indirection was being ignored, so it answered with
				// the elements and a script iterating `for i in "${!a[@]}"`
				// silently looped over the wrong thing.
				//
				// The subscripts assigned, which is not `0..n-1`: an array
				// with a gap in it has subscripts the count never reaches.
				elems = r.subscriptsOf(e.Name, len(elems))
			}
			if e.Op == syntax.ParamSubstring {
				elems = sliceElems(elems, r.numOf(e.Arg), e.Arg2, r)
			}
			ifs, set := r.ifs()
			if r.subscriptText(e.Index) == "*" {
				// `[*]` is *one* field with the elements joined, where `[@]`
				// is one field each — the same difference `"$*"` has from
				// `"$@"`, and the reason both spellings exist. Taking the
				// `[@]` path for it produced no field at all inside a larger
				// word, so `echo "[${a[*]}]"` printed `[]`.
				joined := strings.Join(elems, ifsFirst(ifs, set))
				if s.Quoting != syntax.Unquoted {
					return []string{globEscape(joined)}, true
				}
				return splitFields(joined, ifs, set), true
			}
			if s.Quoting != syntax.Unquoted {
				return escapeAll(elems), true
			}
			var out []string
			for _, el := range elems {
				out = append(out, splitFields(el, ifs, set)...)
			}
			return out, true
		}
	}
	if e.Name != "@" || e.Op != syntax.ParamNone || e.Length {
		return nil, false
	}
	if s.Quoting != syntax.Unquoted {
		// Escaped for the same reason every other quoted expansion is: the
		// fields go on to pathname expansion, and a `*` in a *value* is not
		// a pattern. Returning them raw made `set -- "$x"` glob.
		return escapeAll(r.Params), true
	}
	// Unquoted, each parameter is then split like any other expansion.
	ifs, set := r.ifs()
	var out []string
	for _, p := range r.Params {
		out = append(out, splitFields(p, ifs, set)...)
	}
	if !r.ask(r.sem().GlobExpansionResults, "globbing the result of an expansion") {
		out = escapeAll(out)
	}
	return out, true
}

// expandSpan expands one span, reporting whether its result is subject to
// field splitting. Only unquoted expansions are; literal text never is,
// however it was written.
func (r *Runner) expandSpan(s syntax.Span) (text string, split bool) {
	unquoted := s.Quoting == syntax.Unquoted
	switch s.Kind {
	case syntax.Literal:
		if s.Quoting == syntax.DollarSingleQuoted {
			// `$'a\tb'` is a tab, and the lexer kept both bytes on purpose so
			// the source text stays recoverable. Decoding it here is what was
			// missing: the quoting was recorded, nothing read it, and the
			// escape reached the output as the two characters it was written
			// as. The result is quoted text like any other.
			return globEscape(expandDollarSingle(s.Value)), false
		}
		// Literal text is never split, however it was written. Its
		// metacharacters stay live only when it was unquoted; quoting is
		// what decides whether text is a pattern at all.
		if unquoted {
			return s.Value, false
		}
		return globEscape(s.Value), false
	case syntax.ParamExp:
		v := r.expandParam(s.Param)
		return r.expansionResult(v, unquoted, r.sem().SplitParamExpansion, "splitting an unquoted parameter expansion")
	case syntax.CommandSubst:
		v := r.commandSubst(r.ctx, s.Value)
		return r.expansionResult(v, unquoted, r.sem().SplitCommandSubstitution, "splitting an unquoted command substitution")
	case syntax.ProcSubstIn, syntax.ProcSubstOut:
		// A path, and a path is never split or globbed however it was
		// written: what came back is a name this shell just made, not text
		// from somewhere that might contain a separator.
		path, ok := r.procSub(r.ctx, s.Kind, s.Value)
		if !ok {
			return "", false
		}
		return globEscape(path), false
	case syntax.ArithSubst:
		tree, perr := r.arithTree(s.Arith, s.Value)
		if perr != nil {
			// A failure to *read* the expression, which can only happen once
			// it has been expanded — so it is reported here rather than by
			// the parser, exactly as the shells report it.
			r.diagf("%s\n", r.diag().ParseFailure(perr))
			r.expandErr = true
			return "", false
		}
		v, err := r.evalNum(tree)
		if err != nil {
			// The expression as written is what dash and ksh93 quote back,
			// and the span still has it: the parser keeps the raw text
			// beside the tree it built from it.
			ae, _ := err.(arithError)
			token := ae.token
			if token == "" {
				// bash blames the whole expression when the failing part is
				// the whole expression, which is also the honest answer when
				// the tree cannot name a smaller piece.
				token = strings.TrimSpace(s.Value)
			}
			if ae.complete {
				r.diagf("%s\n", err.Error())
			} else {
				r.diagf("%s\n", Wording(r.diag().ArithError, "%[2]s",
					strings.TrimSpace(s.Value), err.Error(), token))
			}
			// The command must not run: `echo $((1/0))` fails in every shell
			// in the panel rather than echoing an empty string.
			r.expandErr = true
			return "", false
		}
		return r.expansionResult(r.formatNum(v), unquoted, r.sem().SplitParamExpansion, "splitting an unquoted arithmetic expansion")
	}
	return "", false
}

// expansionResult applies the two axes that govern what happens to the result
// of an expansion: whether it is field-split, and whether its metacharacters
// stay live for pathname expansion.
//
// Quoted, neither applies — that is universal. Unquoted, both are dialect
// questions, and zsh answers no to both while everything else answers yes.
func (r *Runner) expansionResult(v string, unquoted bool, split Answer, axis string) (string, bool) {
	if !unquoted {
		return globEscape(v), false
	}
	// Both axes are asked only when the value could actually differ: a result
	// with no separator in it is not split either way, and one with no
	// metacharacter is not a pattern either way.
	doSplit := false
	// Asked against the *actual* separators, not a guess at them: with
	// IFS=: a value holding no space still splits, and hardcoding whitespace
	// here silently stopped it.
	if ifs, _ := r.ifs(); containsAnyOf(v, ifs) {
		doSplit = r.ask(split, axis)
	}
	if hasUnescapedMeta(v) &&
		!r.ask(r.sem().GlobExpansionResults, "globbing the result of an expansion") {
		// zsh does not treat the result of an expansion as a pattern. The
		// same rule decides `[[ abc == $p ]]`, which is one behavior
		// observed twice rather than two quirks.
		v = globEscape(v)
	}
	return v, doSplit
}

func containsAnyOf(s, chars string) bool {
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(chars, s[i]) >= 0 {
			return true
		}
	}
	return false
}

// expandParam handles the forms this slice implements.
func (r *Runner) expandParam(e *syntax.ParamExpr) string {
	if e == nil {
		return ""
	}
	// An array subscript supplies a value too, and the operators apply to it
	// exactly as they do to a variable. That is what the comment said before
	// this function returned here instead: every operator was skipped, so
	// `${a[0]#h}` on `hello` came back `hello`, and so did `${a[0]/l/L}`,
	// `${a[0]%%o}` and `${a[0]:1}` — silently, with status 0. The same
	// mistake the positional-parameter path made and was fixed for.
	var (
		value     string
		set       bool
		subscript bool
	)
	if e.Index != nil {
		if elems, ok := r.arraySubscript(e); ok {
			if e.Length { //nolint:nestif // the Length question is answered here on purpose
				// `${#a[@]}` is the number of elements; `${#a[0]}` is the
				// length of one. The subscript decides which question was
				// asked, which is why this is here rather than below.
				idx := r.subscriptText(e.Index)
				if idx == "@" || idx == "*" {
					return itoa(len(elems))
				}
				return itoa(len(strings.Join(elems, "")))
			}
			// nil rather than empty is what says the element was not there:
			// an element holding "" is set, and `${a[0]:-d}` has to tell the
			// two apart.
			_ = elems
		}
	}
	value, set, subscript = r.paramSource(e)

	if !set {
		r.checkNounset(e)
	}

	if e.Indirect {
		// `${!x}` reads x, then reads *that* as a name — in bash. ksh93
		// parses the same text and yields the name itself, so the grammar
		// having accepted it is not enough to know what it means.
		if r.ask(r.sem().IndirectionYieldsName, "${!x} yielding the name") {
			return e.Name
		}
		if !set || value == "" {
			return ""
		}
		value, set = r.getVar(value)
	}

	if e.Length {
		// `${#@}` is the number of parameters, not the length of anything —
		// so the length question is answered once, here, and specialParam
		// supplies only the value.
		if e.Name == "@" || e.Name == "*" {
			return itoa(r.specialLength())
		}
		return itoa(len(value))
	}

	// The colon extends the test from "unset" to "unset or empty". That one
	// rule is the whole difference between the two rows of conditionals.
	fires := !set
	if e.Colon {
		fires = !set || value == ""
	}

	switch e.Op {
	case syntax.ParamNone:
		return value
	case syntax.ParamDefault:
		if fires {
			return r.joinWord(e.Arg)
		}
		return value
	case syntax.ParamAssign:
		if fires {
			v := r.joinWord(e.Arg)
			// The side effect that outlives the expansion — and with a
			// subscript it belongs to the *element*. Assigning to the name
			// would replace the whole array with one string, which is worse
			// than the nothing this used to do.
			if subscript {
				r.assignSubscript(e, v)
				return v
			}
			r.setVar(e.Name, v)
			return v
		}
		return value
	case syntax.ParamAlternate:
		if fires {
			return ""
		}
		return r.joinWord(e.Arg)

	case syntax.ParamTrimPrefix, syntax.ParamTrimPrefixLong,
		syntax.ParamTrimSuffix, syntax.ParamTrimSuffixLong:
		return r.trimWith(value, r.patternOf(e.Arg), e.Op)

	case syntax.ParamReplace:
		return r.replaceWith(value, r.patternOf(e.Arg), r.joinWord(e.Arg2), e)

	case syntax.ParamSubstring:
		return substring(value, r.numOf(e.Arg), e.Arg2, r)

	case syntax.ParamUpper:
		return strings.ToUpper(value)
	case syntax.ParamLower:
		return strings.ToLower(value)
	}
	// Anything else is left empty rather than guessed at.
	return ""
}

// assignSubscript is `${a[i]:=v}`, which assigns to the element rather than to
// the array.
//
// Only a numeric subscript: `${a[@]:=v}` is a question about the whole array
// that the panel does not answer alike, and guessing at it would be worse than
// leaving it alone.
func (r *Runner) assignSubscript(e *syntax.ParamExpr, v string) {
	idx := r.subscriptText(e.Index)
	n, err := r.parseNum(idx)
	if err != nil {
		return
	}
	r.setArrayElem(e.Name, n, v)
}

// wholeArraySubscript reports whether a subscript names the whole array rather
// than one element, which is what decides whether `:` slices a list or takes a
// substring of a single value.
func wholeArraySubscript(idx string) bool { return idx == "@" || idx == "*" }

// arrayIndices is the subscripts of an array of n elements, as words.
//
// From 0, not from this dialect's array base. Both shells that have the form
// count from 0 — bash and ksh93 answer `0 1 2` — and zsh, the one that counts
// subscripts from 1, rejects `${!a[@]}` as a bad substitution before any of
// this runs. So a base here would be a guess about a dialect that never
// reaches it, and mutation duly showed no test could tell it from 0.
//
// Dense, because this shell's arrays are: `a=(x); a[5]=y` leaves six elements
// here where bash leaves two, so these are 0..n-1 rather than the subscripts
// that were actually assigned. That is the array model rather than this
// expansion, and it is the same gap `${#a[@]}` already has.
// subscriptsOf is what `${!a[@]}` yields: the subscripts of a stored array,
// or a plain count for anything else that reads as one.
func (r *Runner) subscriptsOf(name string, n int) []string {
	if a, ok := r.Arrays[name]; ok {
		keys := r.arrayKeys(a)
		base := r.arrayBase()
		out := make([]string, 0, len(keys))
		for _, k := range keys {
			// Positions are stored from zero and subscripts are written from
			// wherever the dialect counts, so the base goes back on here —
			// the same edge it came off at.
			out = append(out, itoa(k+base))
		}
		return out
	}
	// Not a stored array — a produced one, or a scalar read as an array of
	// one. Those have no subscripts of their own, so they are counted.
	return arrayIndices(n)
}

func arrayIndices(n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, itoa(i))
	}
	return out
}

// sliceElems is `${a[@]:off:len}` — the same arithmetic substring does, over a
// list instead of a string.
//
// Measured unanimous in bash, ksh93 and zsh for every shape but one, including
// the offset being counted from 0 in zsh, whose *subscripts* count from 1.
//
// The exception is a negative length, where the three disagree: bash refuses
// it outright for a list (`substring expression < 0`) though it accepts it for
// a string, ksh93 yields nothing, and zsh reads it as an offset from the end.
// This follows the rule the string form here already uses, so the two spellings
// agree with each other, and lands on zsh's answer.
func sliceElems(elems []string, off int, lenWord *syntax.Word, r *Runner) []string {
	if off < 0 {
		off += len(elems)
	}
	if off < 0 {
		off = 0
	}
	if off > len(elems) {
		return nil
	}
	out := elems[off:]
	if lenWord == nil {
		return out
	}
	n := r.numOf(lenWord)
	if n < 0 {
		n = len(elems) + n - off
	}
	if n < 0 {
		n = 0
	}
	if n > len(out) {
		n = len(out)
	}
	return out[:n]
}

// numOf evaluates a word as a number, for a substring's offset and length.
func (r *Runner) numOf(w *syntax.Word) int {
	if w == nil {
		return 0
	}
	n, err := r.parseNum(strings.TrimSpace(r.joinWord(w)))
	if err != nil {
		return 0
	}
	return n
}

// trim removes a matching prefix or suffix.
//
// Doubling the operator is what selects the longer match; there is no
// greediness syntax inside the pattern, so the search order is the whole
// implementation. A pattern that does not match removes nothing.
// trimWith and replaceWith resolve the caret axis for the pattern before
// handing it to the matcher, which has no Runner and should not need one.
func (r *Runner) trimWith(value, pattern string, op syntax.ParamOp) string {
	return trim(value, pattern, op, r.patternOpts(pattern))
}

func (r *Runner) replaceWith(value, pattern, with string, e *syntax.ParamExpr) string {
	return replace(value, pattern, with, e, r.patternOpts(pattern))
}

func trim(value, pattern string, op syntax.ParamOp, o patternOpts) string {
	prefix := op == syntax.ParamTrimPrefix || op == syntax.ParamTrimPrefixLong
	longest := op == syntax.ParamTrimPrefixLong || op == syntax.ParamTrimSuffixLong

	// Candidate split points, ordered so the first match found is the one
	// wanted: shortest first for the single operators, longest first for the
	// doubled ones.
	idx := make([]int, 0, len(value)+1)
	for i := 0; i <= len(value); i++ {
		idx = append(idx, i)
	}
	if (prefix && longest) || (!prefix && !longest) {
		for l, r := 0, len(idx)-1; l < r; l, r = l+1, r-1 {
			idx[l], idx[r] = idx[r], idx[l]
		}
	}
	for _, i := range idx {
		if prefix {
			if matchPattern(pattern, value[:i], o) {
				return value[i:]
			}
			continue
		}
		if matchPattern(pattern, value[i:], o) {
			return value[:i]
		}
	}
	return value
}

// replace substitutes a matching span, once or everywhere.
//
// The anchored forms match only at one end, which is what `/#` and `/%` mean.
func replace(value, pattern, with string, e *syntax.ParamExpr, o patternOpts) string {
	switch e.Anchor {
	case '#':
		for i := len(value); i >= 0; i-- {
			if matchPattern(pattern, value[:i], o) {
				return with + value[i:]
			}
		}
		return value
	case '%':
		for i := 0; i <= len(value); i++ {
			if matchPattern(pattern, value[i:], o) {
				return value[:i] + with
			}
		}
		return value
	}

	var b strings.Builder
	for i := 0; i <= len(value); {
		// The longest match at this position, so `*` behaves as it does
		// everywhere else rather than matching empty and looping.
		end := -1
		for j := len(value); j >= i; j-- {
			if matchPattern(pattern, value[i:j], o) {
				end = j
				break
			}
		}
		if end < 0 || end == i && pattern != "" && !matchPattern(pattern, "", o) {
			if i < len(value) {
				b.WriteByte(value[i])
			}
			i++
			continue
		}
		b.WriteString(with)
		if !e.All {
			b.WriteString(value[end:])
			return b.String()
		}
		if end == i {
			// An empty match must still make progress.
			if i < len(value) {
				b.WriteByte(value[i])
			}
			i++
			continue
		}
		i = end
	}
	return b.String()
}

// substring takes a slice of the value.
func substring(value string, off int, lenWord *syntax.Word, r *Runner) string {
	if off < 0 {
		off += len(value)
	}
	if off < 0 {
		off = 0
	}
	if off > len(value) {
		return ""
	}
	if lenWord == nil {
		return value[off:]
	}
	n := r.numOf(lenWord)
	if n < 0 {
		// A negative length is an offset from the end.
		n = len(value) + n - off
	}
	if n < 0 {
		n = 0
	}
	if off+n > len(value) {
		n = len(value) - off
	}
	return value[off : off+n]
}

func (r *Runner) joinWord(w *syntax.Word) string {
	return strings.Join(r.expandWord(w), " ")
}

// ifs returns the field separators. Unset means the default; set and empty
// disables splitting, which is a different state rather than a degree of it.
func (r *Runner) ifs() (value string, set bool) {
	v, ok := r.getVar("IFS")
	if !ok {
		return " \t\n", false
	}
	return v, true
}

// splitFields implements docs/spec/grammar/word-splitting.md.
//
// The rule that makes this more than a strings.Split: a run of IFS whitespace
// is one delimiter and leading and trailing runs are discarded, while *each*
// non-whitespace separator delimits — so two adjacent ones produce an empty
// field. A trailing separator is absorbed and a leading one is not, which is
// the asymmetry a symmetric implementation gets wrong.
func splitFields(s string, ifs string, ifsSet bool) []string {
	if ifsSet && ifs == "" {
		// Set and empty disables the stage entirely, which is a different
		// state from unset rather than a degree of it.
		if s == "" {
			return nil
		}
		return []string{s}
	}
	if s == "" {
		return nil
	}

	isWS := func(c byte) bool {
		return strings.IndexByte(ifs, c) >= 0 && (c == ' ' || c == '\t' || c == '\n')
	}
	isSep := func(c byte) bool { return strings.IndexByte(ifs, c) >= 0 }

	var out []string
	i := 0
	for i < len(s) && isWS(s[i]) { // leading IFS whitespace is discarded
		i++
	}
	for i < len(s) {
		start := i
		for i < len(s) && !isSep(s[i]) {
			i++
		}
		out = append(out, s[start:i])
		if i >= len(s) {
			break
		}
		// One delimiter is: a run of IFS whitespace, at most one
		// non-whitespace separator, and another run of whitespace. Consuming
		// exactly that and then letting the loop read the next field is what
		// makes two adjacent non-whitespace separators produce one empty
		// field rather than two — the bug a hand-rolled version invites.
		for i < len(s) && isWS(s[i]) {
			i++
		}
		if i < len(s) && isSep(s[i]) {
			i++
			for i < len(s) && isWS(s[i]) {
				i++
			}
		}
		// A trailing delimiter is absorbed and does not produce a final
		// empty field; a leading one is not, which the loop above already
		// handled by reading an empty field before consuming it.
	}
	return out
}

// itoa writes an integer.
//
// It delegates rather than looping by hand, and the hand-written loop is why:
// its condition was `n > 0`, so a negative number produced no digits at all
// and every negative arithmetic result expanded to nothing. `echo $((2-7))`
// printed an empty line in all four dialects, and the corpus had no case with
// a negative result in it to notice.
func itoa(n int) string { return strconv.Itoa(n) }

// specialParam answers the parameters that are not variables.
// specialParam supplies the value of a parameter that is not a variable. The
// caller applies the operators, exactly as it does for a variable — the two
// differ in where the value comes from and in nothing else.
func (r *Runner) specialParam(e *syntax.ParamExpr) (string, bool) {
	switch e.Name {
	case "#":
		return itoa(len(r.Params)), true
	case "?":
		return itoa(r.status), true
	case "$":
		// The shell's own process id, and the same inside a subshell: POSIX
		// says `$$` is the *invoking* shell's, which is what makes it usable
		// as a lock name. Nothing here forks for a subshell, so the process
		// id is already the right one.
		return itoa(os.Getpid()), true
	case "!":
		if r.lastJob == nil {
			return "", true
		}
		return itoa(r.lastJob.PID), true
	case "0":
		// zsh reports the *function's* name inside a function where every
		// other shell reports the shell's.
		if r.inFunc != "" && r.ask(r.sem().DollarZeroInFunctionIsFunctionName, "$0 inside a function") {
			return r.inFunc, true
		}
		return r.Name, true
	case "*":
		// `$*` joins with the *first character* of IFS, not with a space.
		sep := " "
		if v, set := r.ifs(); set {
			if v == "" {
				sep = ""
			} else {
				sep = v[:1]
			}
		}
		return strings.Join(r.Params, sep), true
	case "@":
		// Reached only where expandAt declined — inside another expansion's
		// operand, say — where joining is the sensible answer.
		return strings.Join(r.Params, " "), true
	}
	if n, ok := atoi(e.Name); ok && n >= 1 {
		if n <= len(r.Params) {
			return r.Params[n-1], true
		}
		// Reported as *unset*, not as empty. Saying "set" here made
		// `${1-default}` yield nothing, because the default only fires for a
		// parameter that is not set — and it hid every unset positional from
		// `set -u`, which is what turned this up.
		return "", false
	}
	return "", false
}

// specialLength answers `${#@}` and `${#*}`.
//
// dash gives the length of the joined string where everything else gives the
// count. Both are plausible numbers and neither errors, which is what makes
// it worth a switch rather than a majority verdict.
func (r *Runner) specialLength() int {
	if r.ask(r.sem().LengthOfSpecialIsCount, "${#@} being the count of parameters") {
		return len(r.Params)
	}
	return len(strings.Join(r.Params, " "))
}

// expandRawText expands text the lexer kept raw — a here-document body — by
// lexing it and expanding the spans that come back.
//
// Newlines are preserved, because the text is input to a command rather than
// a word: lexing alone would drop them as token separators.
func (r *Runner) expandRawText(text string) string {
	var b strings.Builder
	// The lexer leaves an expansion's inside raw, so parseSpans fills it in —
	// the same handoff a word goes through.
	for _, s := range r.parseSpans(syntax.HeredocSpans(text, r.dialect())) {
		out, _ := r.expandSpan(s)
		// expandSpan marks a literal's metacharacters for the glob stage,
		// and a here-document has no glob stage — the text is input, not a
		// pattern. Without this a backslash in the body came out doubled.
		b.WriteString(globUnescape(out))
	}
	return b.String()
}

// parseSpans fills in the parsed form of any expansion the lexer left raw,
// which the parser normally does when it builds a word.
func (r *Runner) parseSpans(spans []syntax.Span) []syntax.Span {
	out := make([]syntax.Span, len(spans))
	copy(out, spans)
	p := syntax.NewParser("", r.dialect())
	for i := range out {
		switch {
		case out[i].Kind == syntax.ParamExp && out[i].Param == nil:
			out[i].Param = p.ParseParamExpFor(out[i].Value, out[i].Pos)
		case out[i].Kind == syntax.ArithSubst && out[i].Arith == nil:
			// Left nil on purpose: an expression with an expansion in it is
			// read when it is evaluated, which is where expandSpan does it.
		}
	}
	return out
}

// expandTilde replaces a leading `~` with the home directory.
//
// Only unquoted, only at the start of a word, and only up to the first slash:
// `echo ~` expands, `echo "~"` does not, and `echo a~` does not because the
// tilde is not where a word begins.
//
// An assignment's value is also a tilde context, which is what makes
// `PATH=~/bin` work — and it comes for free here, because the parser gives an
// assignment's value its own word.
// expandEquals replaces `=cmd` with the path of cmd, which zsh alone does.
//
// It sits beside expandTilde because it is the same kind of thing and zsh
// groups them together: both rewrite the head of an unquoted word into a
// filename before anything else looks at it. Quoting removes it, and so does
// anything other than `=` in first position — `a=b` is an assignment and stays
// one.
func (r *Runner) expandEquals(w *syntax.Word) {
	if len(w.Spans) == 0 {
		return
	}
	s := &w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted ||
		!strings.HasPrefix(s.Value, "=") || len(s.Value) == 1 {
		return
	}
	if !r.ask(r.sem().EqualsExpansion, "`=cmd` expanding to a path") {
		return
	}
	name := s.Value[1:]
	// The script's PATH, like every other lookup here.
	path, err := r.lookPath(name)
	if err != nil {
		// zsh reports the name without a colon and abandons the script,
		// which is what any failed expansion does here.
		r.diagf("%s\n", Wording(r.diag().EqualsNotFound, "%s not found", name))
		r.expandErr = true
		return
	}
	s.Value = path
}

func (r *Runner) expandTilde(w *syntax.Word) {
	if len(w.Spans) == 0 {
		return
	}
	s := &w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted ||
		!strings.HasPrefix(s.Value, "~") {
		return
	}
	rest := s.Value[1:]
	name, tail := rest, ""
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		name, tail = rest[:i], rest[i:]
	}
	if name != "" {
		// `~user` needs a user database this package does not carry, so it is
		// left alone rather than guessed at.
		return
	}
	home, ok := r.getVar("HOME")
	if !ok {
		return
	}
	s.Value = home + tail
}

// escapeAll marks every field's metacharacters as literal, for the fields
// that reach pathname expansion without having gone through expandSpan.
func escapeAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = globEscape(s)
	}
	return out
}

// checkNounset reports an unset parameter under `set -u`.
//
// Only where the expansion would actually *use* the value. `${x:-d}` and
// `${x-d}` supply one, `${x+d}` asks whether it is set, and `${x:?m}` reports
// in its own words — none of those is an error, and all four shells agree.
func (r *Runner) checkNounset(e *syntax.ParamExpr) {
	if !r.nounset {
		return
	}
	switch e.Op {
	case syntax.ParamDefault, syntax.ParamAssign, syntax.ParamAlternate, syntax.ParamError:
		return
	}
	switch e.Name {
	case "@", "*":
		// No parameters is not the same as unset: `"$@"` with none is empty
		// and quiet in all four.
		return
	}
	if isPositional(e.Name) && !r.ask(r.sem().UnsetPositionalIsAllowed, "an unset positional parameter under set -u") {
		// ksh93 alone lets `$1` be empty here. bash writes the `$` back for
		// a positional and not for a name, which is why the wording is its
		// own field rather than a decoration applied here.
		format := r.diag().UnboundPositional
		if format == "" {
			format = r.diag().UnboundVariable
		}
		r.fatal("%s\n", Wording(format, "%s: parameter not set", e.Name))
		return
	}
	if !isPositional(e.Name) {
		r.fatal("%s\n", Wording(r.diag().UnboundVariable, "%s: parameter not set", e.Name))
	}
}

// isPositional reports whether a parameter name is a positional one.
func isPositional(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// expandDollarSingle decodes the escapes `$'…'` gives meaning to.
//
// The same set `printf` reads, plus `\e` for escape and the hexadecimal and
// unicode forms, and without `\c`: there is no output to stop here, only a
// word being built.
func expandDollarSingle(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		switch c := s[i+1]; c {
		case 'n':
			b.WriteByte('\n')
			i += 2
		case 't':
			b.WriteByte('\t')
			i += 2
		case 'r':
			b.WriteByte('\r')
			i += 2
		case 'a':
			b.WriteByte('\a')
			i += 2
		case 'b':
			b.WriteByte('\b')
			i += 2
		case 'f':
			b.WriteByte('\f')
			i += 2
		case 'v':
			b.WriteByte('\v')
			i += 2
		case 'e', 'E':
			b.WriteByte(0x1b)
			i += 2
		case '\\', '\'', '"', '?':
			b.WriteByte(c)
			i += 2
		case 'x':
			n, used := scanBase(s[i+2:], 16, 2)
			if used == 0 {
				b.WriteString(`\x`)
				i += 2
				continue
			}
			b.WriteByte(byte(n))
			i += 2 + used
		case 'u', 'U':
			width := 4
			if c == 'U' {
				width = 8
			}
			n, used := scanBase(s[i+2:], 16, width)
			if used == 0 {
				b.WriteByte('\\')
				b.WriteByte(c)
				i += 2
				continue
			}
			b.WriteRune(rune(n))
			i += 2 + used
		case '0', '1', '2', '3', '4', '5', '6', '7':
			n, used := scanBase(s[i+1:], 8, 3)
			b.WriteByte(byte(n))
			i += 1 + used
		default:
			// An escape with no meaning keeps both characters, which is what
			// the panel does rather than dropping the backslash.
			b.WriteByte('\\')
			b.WriteByte(c)
			i += 2
		}
	}
	return b.String()
}

// scanBase reads up to max digits in the given base, reporting how many it
// used so the caller can tell "no digits at all" from a zero.
func scanBase(s string, base, maxDigits int) (int, int) {
	n, used := 0, 0
	for used < maxDigits && used < len(s) {
		d := digitValue(s[used])
		if d < 0 || d >= base {
			break
		}
		n = n*base + d
		used++
	}
	return n, used
}

func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// ifsFirst is the character `*` joins with: the first of IFS, a space when
// IFS is unset, and nothing at all when IFS is set but empty.
func ifsFirst(ifs string, set bool) string {
	if !set {
		return " "
	}
	if ifs == "" {
		return ""
	}
	return ifs[:1]
}

// namesWithPrefix is every variable name beginning with prefix, sorted.
//
// Sorted because the shells that have this return them so, and because a map
// has no order to inherit: without it the same script would print its names
// differently on different runs.
//
// It reads the same places a lookup does — what the shell has set, and what
// it inherited — and skips what `unset` took away, so a name that cannot be
// read is not listed either.
func (r *Runner) namesWithPrefix(prefix string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if seen[name] || r.removed[name] || !strings.HasPrefix(name, prefix) {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for name := range r.Vars {
		add(name)
	}
	for name := range r.Dynamic {
		add(name)
	}
	for _, kv := range r.environ() {
		if k, _, ok := strings.Cut(kv, "="); ok {
			add(k)
		}
	}
	sort.Strings(out)
	return out
}

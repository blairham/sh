// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"

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
	// wraps the rest rather than being a stage inside it. One word back can
	// still be an expansion — `{1..1}` is `1`, and a range whose honored
	// step sign points away from the far endpoint holds one element — so
	// the test is whether the word changed, not whether it multiplied.
	if words := r.braceExpand(w); (len(words) > 1 || len(words) == 1 && words[0] != w) &&
		r.ask(r.sem().BraceExpansion, "brace expansion") {
		var out []string
		for _, bw := range words {
			out = append(out, r.expandOneWord(bw)...)
		}
		return out
	}
	return r.expandOneWord(w)
}

// inWord records the word being expanded and returns the undo, so a
// diagnostic raised inside it can name the text the expansion sits in.
//
// It has to be put back rather than cleared: an expansion's operand is a word
// of its own — `${u:-${y@QQ}}` — and the inner one finishing does not mean the
// outer one has.
func (r *Runner) inWord(w *syntax.Word) func() {
	prevWord, prevSpan := r.expandingWord, r.expandingSpan
	r.expandingWord, r.expandingSpan = w, 0
	return func() { r.expandingWord, r.expandingSpan = prevWord, prevSpan }
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

	// Whether an expansion has already failed on this word. Every shell in
	// the panel abandons the word at the first failure rather than going on
	// to diagnose the rest of it, measured — `printf "[%s]" "${(q)x}"
	// "${(qq)x}"` is one line of diagnosis in all six columns and was four
	// here. The state before the loop is what is compared against, because a
	// caller may have failed already and this word is not responsible for
	// that.
	failed := r.expandErr
	defer r.inWord(w)()

	for i, s := range w.Spans {
		if (r.expandErr && !failed) || r.ctl == controlExit {
			break
		}
		r.expandingSpan = i
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
		text, split := r.expandSpan(s, splitByDialect)
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
	// pattern that matches nothing is passed through unchanged — unless the
	// run-time option deletes it, which is what the second result reports.
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		matches, dropped := r.glob(f)
		if len(matches) > 0 {
			out = append(out, matches...)
			continue
		}
		if dropped {
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
//
// splitNever, because not splitting here is unanimous: asking the axis anyway
// made the bare core refuse `x=$two` and `[[ $two = "a b" ]]` over a question
// every shell in the panel answers the same way in these positions.
func (r *Runner) expandWordNoSplit(w *syntax.Word) []string {
	if w == nil {
		return nil
	}
	r.expandTilde(w)
	failed := r.expandErr
	defer r.inWord(w)()
	var b strings.Builder
	for i, s := range w.Spans {
		if (r.expandErr && !failed) || r.ctl == controlExit {
			// The word is abandoned at its first failed expansion, here as
			// in the splitting path.
			break
		}
		r.expandingSpan = i
		if parts, ok := r.expandAt(s); ok {
			b.WriteString(strings.Join(parts, " "))
			continue
		}
		text, _ := r.expandSpan(s, splitNever)
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
//
// The fields view splits without consulting the splitting axis: it exists to
// show what the ordinary-word reading *would* be, and whether that reading
// applies is the redirection's own axis, asked by the caller exactly where
// the two views differ. Asking here as well made the bare core refuse
// `> $two` for splitting — the wrong axis, and asked even when the target
// was one word under both readings.
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
		text, split := r.expandSpan(s, splitAlways)
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
		matches, dropped := r.glob(f)
		if len(matches) > 0 {
			out = append(out, matches...)
			continue
		}
		if dropped {
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
	r.expandColonTildes(w)
	return strings.Join(r.expandWordNoSplit(w), "")
}

// expandColonTildes expands the tildes only an assignment has: one after each
// unquoted colon, which is what makes `PATH=~/bin:~/sbin` and `M=a:~/b` work.
// Unanimous across the panel.
//
// The same limits as the leading tilde: `~user` needs a user database this
// package does not carry and is left as written, and so is a tilde whose
// segment runs off the span into an expansion — `a:~$x` keeps its tilde in
// three of the four shells, and the fourth's answer needs the expansion's
// value, which does not exist yet.
func (r *Runner) expandColonTildes(w *syntax.Word) {
	home, ok := r.getVar("HOME")
	if !ok {
		return
	}
	for i := range w.Spans {
		s := &w.Spans[i]
		if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
			continue
		}
		v := s.Value
		if !strings.Contains(v, ":~") {
			continue
		}
		var b strings.Builder
		for j := 0; j < len(v); j++ {
			b.WriteByte(v[j])
			if v[j] != ':' || j+1 >= len(v) || v[j+1] != '~' {
				continue
			}
			// The segment runs to the next slash or colon; hitting the end
			// of the span only counts as an end when nothing follows it.
			k := j + 2
			for k < len(v) && v[k] != '/' && v[k] != ':' {
				k++
			}
			terminated := k < len(v) || i == len(w.Spans)-1
			if !terminated {
				continue
			}
			switch v[j+2 : k] {
			case "":
				b.WriteString(home)
				j = k - 1
			case "+", "-":
				// The same pair the leading position takes, in the same
				// dialects: `PATH=~+/bin:~-/bin` names both directories.
				if dir, ok := r.tildeDirVar(v[j+2 : k]); ok {
					b.WriteString(dir)
					j = k - 1
				}
			}
		}
		s.Value = b.String()
	}
}

// expandAt handles `$@`, the only expansion that produces several fields by
// itself. Quoted, it is one field per parameter, each keeping its own spaces;
// with no parameters it is *zero* fields, which is why `set -- "$@"` is safe
// on an empty list and `set -- "$*"` is not.
func (r *Runner) expandAt(s syntax.Span) ([]string, bool) {
	if s.Kind != syntax.ParamExp || s.Param == nil {
		return nil, false
	}
	if s.Param.Bad {
		// An expansion the grammar could not read is not a shape at all, so
		// none of the shapes below applies to it. Left to the scalar path,
		// which reports it.
		//
		// This has to come before the subscript test: a Bad node still
		// carries the `[@]` that was read before the operator failed, the
		// array shape matched on it, and `${a[@]@Q}` under a dialect without
		// the family answered with the plain elements at status 0 where the
		// shell it claims to be calls it a bad substitution.
		return nil, false
	}
	// A flag group changes what the whole expansion yields — how many
	// fields, joined with what — so a node that carries one is answered by
	// its own pipeline, before any of the shapes below are considered.
	if fields, ok := r.expandFlagged(s); ok {
		return fields, true
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
			((e.Op == syntax.ParamSubstring || e.Op == syntax.ParamTransform ||
				selectsElements(e.Op) || elementOp(e.Op) || r.yieldsTheArray(e)) &&
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
				elems = sliceElems(elems, r.numOf(e.Arg, e, e.Arg2), e, r)
			}
			if selectsElements(e.Op) {
				// Which elements there are, rather than what each one
				// becomes — so this is here beside the slice and not with
				// the elementOp mapping further down, whose whole shape is
				// one output per input.
				elems = r.selectElements(e, elems)
			}
			if e.Op == syntax.ParamTransform {
				// `"${a[@]@Q}"` is one transformed word per element — the
				// transformation distributes, measured, and the `[*]` join
				// below then applies to what came out rather than to what
				// went in.
				elems = r.transformElems(e, elems)
			}
			ifs, set := r.ifs()
			if elementOp(e.Op) {
				apply := r.elementOpApplier(e)
				mapped := make([]string, len(elems))
				for i, el := range elems {
					mapped[i] = apply(el)
				}
				if r.subscriptText(e.Index) == "*" {
					// `${a[*]#p}` splits the panel: two shells trim each
					// element and join what is left, the third joins first
					// and trims the joined string once. Asked only when the
					// two readings actually differ — `${a[*]%b}` on `(aa ab)`
					// is `aa a` either way, and needs no answer.
					sep := ifsFirst(ifs, set)
					perElement := strings.Join(mapped, sep)
					joinedFirst := apply(strings.Join(elems, sep))
					if perElement == joinedFirst ||
						r.ask(r.sem().OperatorDistributesOverStarSubscript,
							"an operator on `${a[*]}` applying to each element") {
						elems = []string{perElement}
					} else {
						elems = []string{joinedFirst}
					}
				} else {
					elems = mapped
				}
			}
			if r.subscriptJoinsElements(e) {
				// `[*]` is *one* field with the elements joined, where `[@]`
				// is one field each — the same difference `"$*"` has from
				// `"$@"`, and the reason both spellings exist. Taking the
				// `[@]` path for it produced no field at all inside a larger
				// word, so `echo "[${a[*]}]"` printed `[]`.
				//
				// A range joins on the same side of that line as the *name*
				// it was written on: measured, `"${a[1,2]}"` is one field
				// holding `x-y` under `IFS=-` exactly as `"$a"` is, and
				// `"${*[1,2]}"` is one field too — while `"${@[1,2]}"` is
				// one field per parameter, because `@` keeps its fields
				// however it is subscripted.
				joined := strings.Join(elems, ifsFirst(ifs, set))
				if s.Quoting != syntax.Unquoted {
					return []string{globEscape(joined)}, true
				}
				return splitFields(joined, ifs, set), true
			}
			if s.Quoting != syntax.Unquoted {
				if len(elems) == 0 && e.Op == syntax.ParamNone {
					if !wholeArraySubscript(r.subscriptText(e.Index)) {
						// A subscript naming *one* element is one field
						// whatever the element turned out to be, exactly as
						// `"$unset"` is one empty field. Quoting is the whole
						// guarantee, and it does not depend on the element
						// being there.
						//
						// This asked the empty-array axis instead, so a gap
						// produced no field at all and every argument after
						// it moved up one — `set -- "${a[0]}" "${a[1]}"
						// "${a[5]}"` on a sparse array gave `$#` of 2, and a
						// script reading `$3` afterwards read what it thought
						// was `$4`. Two different questions: how many fields
						// an *empty list* makes, and how many a quoted
						// expansion of *one* element makes. Only the first is
						// a dialect's.
						return []string{""}, true
					}
					if r.ask(r.sem().EmptyArrayAtIsOneEmptyField,
						`a quoted "${a[@]}" of an empty array`) {
						// One dialect hands the quotes a field to keep: an
						// empty array is one empty argument there, which is
						// the reason careful scripts write
						// "${a[@]+"${a[@]}"}".
						return []string{""}, true
					}
				}
				return escapeAll(elems), true
			}
			var out []string
			for _, el := range elems {
				out = append(out, splitFields(el, ifs, set)...)
			}
			return out, true
		}
	}
	// `${@@Q}` and `${*@Q}`: a transformation distributes over the positional
	// parameters exactly as it does over a whole array — one word per
	// parameter for `@`, joined for `*`. Measured with zero parameters too:
	// zero fields, the same answer `"$@"` gives.
	if (e.Name == "@" || e.Name == "*") && e.Op == syntax.ParamTransform &&
		e.Index == nil && !e.Length && !e.Indirect {
		elems := r.transformElems(e, r.Params)
		ifs, set := r.ifs()
		if e.Name == "*" {
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

// splitPolicy says what the context a span is expanded in does with a result
// that could split into fields.
//
// An ordinary word asks the dialect's axis. The contexts that never split —
// an assignment's value, `[[ ]]` operands, a case subject, a here-document
// body — must not ask it, because the exemption there is unanimous across the
// panel and an unanswered axis refuses only where the shells genuinely
// disagree; asking anyway made the bare core refuse `x=$two` on a question no
// shell answers differently. The redirection target's ordinary-word view
// splits without asking, because whether that view applies at all is the
// redirection's own axis, asked where the two readings differ.
type splitPolicy uint8

const (
	splitByDialect splitPolicy = iota // an ordinary word: the axis decides
	splitNever                        // a unanimously exempt context
	splitAlways                       // the ordinary-word view of a redirection target
)

// answer resolves the policy against the dialect's own answer. A context that
// never splits, or always does, needs nothing from anyone — which is what
// keeps the refusal for the axis at genuine disagreements only.
func (sp splitPolicy) answer(a Answer) Answer {
	switch sp {
	case splitNever:
		return No
	case splitAlways:
		return Yes
	}
	return a
}

// expandSpan expands one span, reporting whether its result is subject to
// field splitting. Only unquoted expansions are; literal text never is,
// however it was written.
func (r *Runner) expandSpan(s syntax.Span, sp splitPolicy) (text string, split bool) {
	unquoted := s.Quoting == syntax.Unquoted
	switch s.Kind {
	case syntax.Literal:
		if s.Quoting == syntax.DollarSingleQuoted {
			// `$'a\tb'` is a tab, and the lexer kept both bytes on purpose so
			// the source text stays recoverable. Decoding it here is what was
			// missing: the quoting was recorded, nothing read it, and the
			// escape reached the output as the two characters it was written
			// as. The result is quoted text like any other.
			return globEscape(r.expandDollarSingle(s.Value)), false
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
		return r.expansionResult(v, unquoted, sp.answer(r.sem().SplitParamExpansion), "splitting an unquoted parameter expansion")
	case syntax.CommandSubst:
		v := r.commandSubst(r.ctx, s)
		return r.expansionResult(v, unquoted, sp.answer(r.sem().SplitCommandSubstitution), "splitting an unquoted command substitution")
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
		v, ok := r.arithSpanValue(s)
		if !ok {
			return "", false
		}
		return r.expansionResult(v, unquoted, sp.answer(r.sem().SplitParamExpansion), "splitting an unquoted arithmetic expansion")
	}
	return "", false
}

// arithSpanValue evaluates an arithmetic substitution and returns its text,
// reporting whether it produced one at all. A false means the expansion has
// already failed and said so, and the command must not run.
//
// It is a function of its own because a pattern operand needs the same value —
// `${v#$((1+1))}` strips a `2` in every shell in the panel — and the failures
// below have to be reported identically wherever the expression stands rather
// than once here and approximately somewhere else.
func (r *Runner) arithSpanValue(s syntax.Span) (string, bool) {
	// An empty expression is zero in three of the four and an error in
	// dash, which wants a primary and stops the script. Asked only when
	// the text really is empty.
	if strings.TrimSpace(s.Value) == "" &&
		r.ask(r.sem().EmptyArithExpressionIsAnError, "an empty arithmetic expression being an error") {
		r.diagf("%s\n", Wording(r.diag().ArithEmptyExpression,
			`arithmetic expression: expecting primary: ""`))
		r.expandErr = true
		return "", false
	}
	if r.unspecified {
		r.expandErr = true
		return "", false
	}
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
		r.diagf("%s\n", r.arithFailure(s.Value, err))
		// The command must not run: `echo $((1/0))` fails in every shell
		// in the panel rather than echoing an empty string.
		r.expandErr = true
		return "", false
	}
	return r.formatNum(v), true
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

// reportBadSubstitution diagnoses an operator the grammar did not recognize,
// deferred here by the dialect: said only now that the expansion is reached,
// the way bash, dash and zsh treat a bad substitution.
//
// Two things about it are the dialect's. What the sentence *names* — the
// expansion, the word, or the run of the word that shares its quoting — and,
// for the `@` family in a grammar that has it, whether the letter is checked
// at all when the name has no value.
func (r *Runner) reportBadSubstitution(e *syntax.ParamExpr) {
	if e.BadTransform && !r.transformHasValue(e) &&
		r.ask(r.sem().TransformLetterCheckedOnlyWhenValued,
			"whether a transformation's letter is checked on a name with no value") {
		// Nothing to transform, so nothing to refuse: the expansion is
		// empty, the status is untouched and no diagnostic is written.
		return
	}
	// The fallback wording is the one dialect that names the construct; the
	// others' own wordings carry no verb at all — except the dialect whose
	// BadSubstitution is a parse-time syntax error, which words the deferred
	// report separately.
	w := r.diag().BadSubstitutionAtRun
	if w == "" {
		w = r.diag().BadSubstitution
	}
	r.diagf("%s\n", Wording(w, "${%[1]s}: bad substitution", r.badSubstitutionSubject(e)))
	if e.BadTransform {
		// The family exists here and only the letter was wrong, which makes
		// this a failed *expansion* rather than a word that could not be
		// read — measured to carry the status a failed expansion carries,
		// which in one dialect depends on how the shell was started.
		r.fatalExpansionQuiet()
		return
	}
	r.expandErr = true
}

// badSubstitutionSubject is the text this dialect's bad-substitution sentence
// names.
func (r *Runner) badSubstitutionSubject(e *syntax.ParamExpr) string {
	names := r.diag().BadSubstitutionNames
	if names == NamesTheExpansion {
		return e.Src
	}
	var text string
	if names == NamesTheWholeWord {
		text = syntax.PrintWord(r.expandingWord)
	} else {
		text = syntax.PrintWordQuotingRun(r.expandingWord, r.expandingSpan)
	}
	if text == "" {
		// Reached from something that is not a word — a `case` subject read
		// another way, a caller of its own. The expansion is then all there
		// is to name, spelled as the wording expects to receive it.
		return "${" + e.Src + "}"
	}
	return text
}

// transformHasValue reports whether the name a `@` operator was written on has
// anything to transform.
//
// A *list* has one when it is not empty: `a=()` and no positional parameters
// are both nothing to transform, measured, where an empty *string* is a value
// and is refused. So the two spellings cannot share one test.
func (r *Runner) transformHasValue(e *syntax.ParamExpr) bool {
	if e.Index != nil && wholeArraySubscript(r.subscriptText(e.Index)) {
		elems, ok := r.arraySubscript(e)
		return ok && len(elems) > 0
	}
	if e.Index == nil && wholeArraySubscript(e.Name) {
		return len(r.Params) > 0
	}
	_, set, _ := r.paramSource(e)
	return set
}

// expandParam handles the forms this slice implements.
func (r *Runner) expandParam(e *syntax.ParamExpr) string {
	if e == nil {
		return ""
	}
	if e.Bad {
		r.reportBadSubstitution(e)
		return ""
	}
	if e.HasFlags {
		// Normally intercepted in expandAt; reached directly where a single
		// word must result — a pattern operand, say — which is the manual's
		// final rule: the words are rejoined with the first character of
		// IFS.
		words, _, ok := r.flaggedWords(e, false)
		if !ok {
			return ""
		}
		return strings.Join(words, ifsFirst(r.ifs()))
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
				//
				// A range asks the same question `[@]` does when it names
				// elements and the same one `[0]` does when it names
				// characters: measured, `${#a[1,2]}` on `(aa bb cc)` is 2
				// and `${#s[2,4]}` on `hello` is 3.
				if r.subscriptYieldsAList(e) {
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

	// The name the operators see: the parameter's own, until an indirection
	// replaces it — `${!y@a}` reports the attributes of the *target*,
	// measured, so the transformations that read attributes need the name
	// the value came from rather than the one written.
	name := e.Name
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
		name = value
		value, set = r.getVar(name)
	}

	if e.Length {
		// `${#@}` is the number of parameters, not the length of anything —
		// so the length question is answered once, here, and specialParam
		// supplies only the value.
		if e.Name == "@" || e.Name == "*" {
			return itoa(r.specialLength())
		}
		if n, isArr := r.arrayElementCount(e.Name); isArr && n != 1 &&
			r.ask(r.sem().ArrayLengthWithoutSubscriptIsCount, "`${#a}` of an array counting elements") {
			// One dialect counts the elements where the others measure the
			// scalar the bare name yields. Asked only where the two
			// readings differ — a one-element array is its element either
			// way.
			return itoa(n)
		}
		if r.unspecified {
			return ""
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

	case syntax.ParamError:
		if fires {
			// Fatal in all four, and with the same four statuses an unset
			// parameter under `set -u` gets — so it goes through the same
			// door rather than carrying a status of its own.
			r.fatalExpansion("%s\n", Wording(r.diag().ParamErrorMessage, "%[1]s: %[2]s",
				e.Name, r.paramErrorWord(e, set)))
			return ""
		}
		return value

	case syntax.ParamTrimPrefix, syntax.ParamTrimPrefixLong,
		syntax.ParamTrimSuffix, syntax.ParamTrimSuffixLong:
		return r.trimWith(value, r.patternOf(e.Arg), e.Op)

	case syntax.ParamReplace:
		return r.replaceWith(value, r.patternOf(e.Arg), r.joinWord(e.Arg2), e)

	case syntax.ParamSubstring:
		return r.substringRange(value, e)

	case syntax.ParamExclude, syntax.ParamSetDifference, syntax.ParamSetIntersection:
		return r.selectScalar(e, value)

	case syntax.ParamUpper, syntax.ParamLower, syntax.ParamToggle,
		syntax.ParamUpperFirst, syntax.ParamLowerFirst, syntax.ParamToggleFirst:
		return r.changeCase(value, e)

	case syntax.ParamTransform:
		return r.transformParam(e, name, value, set)
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
	if wholeArraySubscript(idx) {
		// The rule above, made explicit for the associative path too — and
		// re-measured there: one shell stores a literal `@` key and another
		// hangs outright, which is nobody's answer to follow.
		return
	}
	if r.assocDeclared(e.Name) {
		r.setAssocElem(e.Name, idx, v)
		return
	}
	n, ok := r.subscriptIndex(idx)
	if !ok {
		return
	}
	r.setArrayElem(e.Name, n, idx, v)
}

// wholeArraySubscript reports whether a subscript names the whole array rather
// than one element, which is what decides whether `:` slices a list or takes a
// substring of a single value.
func wholeArraySubscript(idx string) bool { return idx == "@" || idx == "*" }

// elementOp reports the operators that apply to each element when the
// subscript names the whole array: the trims, the replacements, and the case
// changes. `${a[@]#p}` trims every element — unanimous in the three shells
// with arrays, and applying it to the first alone was the silent bug this
// names: `${a[@]#a}` on `(aa ab)` came back `a ab` with status 0.
func elementOp(op syntax.ParamOp) bool {
	switch op {
	case syntax.ParamTrimPrefix, syntax.ParamTrimPrefixLong,
		syntax.ParamTrimSuffix, syntax.ParamTrimSuffixLong,
		syntax.ParamReplace,
		syntax.ParamUpper, syntax.ParamLower, syntax.ParamToggle,
		syntax.ParamUpperFirst, syntax.ParamLowerFirst, syntax.ParamToggleFirst:
		return true
	}
	return false
}

// elementOpApplier expands the operator's words once and returns the operator
// as a function over one value.
//
// Once, not once per element: `${a[@]#$(cmd)}` runs the command a single time
// in every shell with arrays — measured — and re-expanding per element would
// also re-fire whatever side effects the word carries.
func (r *Runner) elementOpApplier(e *syntax.ParamExpr) func(string) string {
	switch e.Op {
	case syntax.ParamTrimPrefix, syntax.ParamTrimPrefixLong,
		syntax.ParamTrimSuffix, syntax.ParamTrimSuffixLong:
		pattern := r.patternOf(e.Arg)
		return func(v string) string { return r.trimWith(v, pattern, e.Op) }
	case syntax.ParamReplace:
		pattern, with := r.patternOf(e.Arg), r.joinWord(e.Arg2)
		return func(v string) string { return r.replaceWith(v, pattern, with, e) }
	default:
		pattern := r.patternOf(e.Arg)
		return func(v string) string { return r.changeCaseWith(v, pattern, e) }
	}
}

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
	if a, ok := r.AssocArrays[name]; ok {
		// An associative array's subscripts are its keys — in key order,
		// because the shells promise no order and sorted is the one this
		// implementation keeps everywhere.
		return a.keys()
	}
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
func sliceElems(elems []string, off int, e *syntax.ParamExpr, r *Runner) []string {
	lenWord := e.Arg2
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
	n := r.numOf(lenWord, e, nil)
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
//
// An expression rather than a numeral, for the reason a subscript is one:
// `${x:1+1:2}` is `cd` of `abcdef` in every shell on the panel that has
// substrings, and taking a numeral alone made it `ab` — an offset of 0, which
// is a wrong answer that looks like a right one.
//
// e is the expansion the range belongs to and tail is the rest of the range as
// written, because a failure here is named in three shapes rather than one:
// bash puts the parameter in front of the arithmetic sentence, ksh93 blames the
// offset together with everything after it, and zsh gives the bare sentence.
// Both extras are ignored where the dialect wants neither.
func (r *Runner) numOf(w *syntax.Word, e *syntax.ParamExpr, tail *syntax.Word) int {
	if w == nil {
		return 0
	}
	text := strings.TrimSpace(r.joinWord(w))
	n, err := r.subscriptValue(text)
	if err != nil {
		// What is *blamed* is not always what was evaluated: one dialect
		// names the offset together with everything after it in the range.
		// Extending the text before evaluating it instead was a second,
		// invented failure — `${x:2:1+}` reported that `2:1+` would not parse
		// and then that `1+` would not, where the shell reports the one.
		blame := text
		if tail != nil && r.diag().SubstringErrorNamesTheWholeRange {
			blame += ":" + strings.TrimSpace(r.joinWord(tail))
		}
		r.diagf("%s\n", Wording(r.diag().SubstringRangeError, "%[2]s",
			r.paramSubject(e), r.subscriptFailure(blame, err)))
		r.expandErr = true
		return 0
	}
	return n
}

// paramSubject is the parameter as a diagnostic names it: the name, and the
// subscript when one was written — `${a[@]:1+}` is blamed on `a[@]`.
func (r *Runner) paramSubject(e *syntax.ParamExpr) string {
	if e == nil {
		return ""
	}
	if e.Index == nil {
		return e.Name
	}
	return e.Name + "[" + r.subscriptText(e.Index) + "]"
}

// trim removes a matching prefix or suffix.
//
// Doubling the operator is what selects the longer match; there is no
// greediness syntax inside the pattern, so the search order is the whole
// implementation. A pattern that does not match removes nothing.
// trimWith and replaceWith resolve the caret axis for the pattern before
// handing it to the matcher, which has no Runner and should not need one.
// changeCase is `^`, `,` and `~` and their doubled forms.
//
// The operator carries a *pattern* saying which characters to convert, and it
// is matched against one character at a time: `${x^^[ab]}` on `abc` is `ABc`,
// not `ABC`. Discarding the pattern and converting everything was a silent
// wrong answer — the script asked for a subset and got the lot, with status 0.
//
// An empty pattern means every character, which is what `?` would say. The
// single forms look only at the first character, and leave the string alone
// when the pattern does not match it: `${x^b}` on `abc` is `abc`.
func (r *Runner) changeCase(value string, e *syntax.ParamExpr) string {
	return r.changeCaseWith(value, r.patternOf(e.Arg), e)
}

// changeCaseWith is changeCase with the pattern already expanded, so a caller
// applying one operator to many values expands its word once.
func (r *Runner) changeCaseWith(value, pattern string, e *syntax.ParamExpr) string {
	if value == "" {
		return value
	}
	convert := unicode.ToUpper
	switch e.Op {
	case syntax.ParamLower, syntax.ParamLowerFirst:
		convert = unicode.ToLower
	case syntax.ParamToggle, syntax.ParamToggleFirst:
		convert = toggleCase
	}
	if r.localeIsC() {
		// In the C locale only ASCII letters are letters, so `${x^^}` on
		// café is CAFé — measured, and the policy docs/spec/semantics.md
		// records: an explicit C or POSIX locale narrows case to ASCII, and
		// anything else, unset included, is Unicode-aware, which is also
		// what bash stripped of every locale variable does.
		ascii := convert
		convert = func(c rune) rune {
			if c < 0x80 {
				return ascii(c)
			}
			return c
		}
	}
	first := e.Op == syntax.ParamUpperFirst ||
		e.Op == syntax.ParamLowerFirst ||
		e.Op == syntax.ParamToggleFirst

	o := r.patternOpts(pattern)
	var b strings.Builder
	for i, c := range value {
		if (pattern == "" || matchPattern(pattern, string(c), o)) &&
			(!first || i == 0) {
			b.WriteRune(convert(c))
			continue
		}
		b.WriteRune(c)
	}
	return b.String()
}

// localeIsC reports an explicit C or POSIX locale, read the way POSIX ranks
// the variables: LC_ALL over LC_CTYPE over LANG. Unset is not C here —
// measured, a shell stripped of every locale variable still cases beyond
// ASCII — so only asking for C narrows anything.
func (r *Runner) localeIsC() bool {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v, ok := r.getVar(name); ok && v != "" {
			return v == "C" || v == "POSIX"
		}
	}
	return false
}

// toggleCase swaps a letter's case and leaves anything else alone.
func toggleCase(c rune) rune {
	if unicode.IsUpper(c) {
		return unicode.ToLower(c)
	}
	return unicode.ToUpper(c)
}

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

// substringRange is `${x:…}`, which is a substring in three of the panel and
// is *either* a substring or a modifier list in the fourth.
//
// The reading is decided before anything is evaluated, because in the shell
// that has both the two spellings are identical: `${x:h}` is a modifier there
// and arithmetic on an unset `h` everywhere else. Asked only where a segment
// actually begins with an unquoted letter, so `${x:1:2}` needs no answer from
// anyone.
func (r *Runner) substringRange(value string, e *syntax.ParamExpr) string {
	if rangeSegmentIsAModifier(e.Arg) {
		if !r.ask(r.sem().SubstringRangeReadsModifiers,
			"a substring range beginning with a letter being a modifier list") {
			// Not this dialect's reading, so the letter is a name in an
			// expression like any other.
			return substring(value, r.numOf(e.Arg, e, e.Arg2), e, r)
		}
		out, ok := r.applyModifiers(value, modifierSegments(e.Arg, e.Arg2), e)
		if !ok {
			return ""
		}
		return out
	}
	// The offset is a number and the length may still be a modifier, applied
	// to what the offset left: `${x:2:t}` is the tail of `${x:2}`.
	if e.Arg2 != nil && rangeSegmentIsAModifier(e.Arg2) &&
		r.ask(r.sem().SubstringRangeReadsModifiers,
			"a substring range beginning with a letter being a modifier list") {
		sliced := substring(value, r.numOf(e.Arg, e, nil), &syntax.ParamExpr{
			Name: e.Name, Op: e.Op, Arg: e.Arg,
		}, r)
		out, ok := r.applyModifiers(sliced, modifierSegments(e.Arg2, nil), e)
		if !ok {
			return ""
		}
		return out
	}
	return substring(value, r.numOf(e.Arg, e, e.Arg2), e, r)
}

// substring takes a slice of the value.
func substring(value string, off int, e *syntax.ParamExpr, r *Runner) string {
	lenWord := e.Arg2
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
	n := r.numOf(lenWord, e, nil)
	if n < 0 {
		if r.ask(r.sem().SubstringNegativeLengthIsEmpty, "a negative substring length") {
			// One dialect answers a negative length with nothing at all;
			// the others count it from the end.
			return ""
		}
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
	return splitFieldsLiteral(s, nil, ifs, ifsSet)
}

// splitFieldsLiteral is splitFields with some bytes exempt from separating:
// literal[i] true means s[i] is data whatever IFS says. `read` without -r
// feeds it the positions its backslashes escaped, which is what keeps `a\ b`
// one field — by the time the escapes are removed, an escaped space and a
// separating one are the same byte, so only a mask can still tell them
// apart. A nil mask exempts nothing.
func splitFieldsLiteral(s string, literal []bool, ifs string, ifsSet bool) []string {
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

	isWS := func(i int) bool {
		c := s[i]
		return (literal == nil || !literal[i]) &&
			strings.IndexByte(ifs, c) >= 0 && (c == ' ' || c == '\t' || c == '\n')
	}
	isSep := func(i int) bool {
		return (literal == nil || !literal[i]) && strings.IndexByte(ifs, s[i]) >= 0
	}

	var out []string
	i := 0
	for i < len(s) && isWS(i) { // leading IFS whitespace is discarded
		i++
	}
	for i < len(s) {
		start := i
		for i < len(s) && !isSep(i) {
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
		for i < len(s) && isWS(i) {
			i++
		}
		if i < len(s) && isSep(i) {
			i++
			for i < len(s) && isWS(i) {
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
	// the same handoff a word goes through. splitNever: a here-document's
	// body is one blob of input rather than fields, in every shell in the
	// panel, so the splitting axis has nothing to ask.
	for _, s := range r.parseSpans(syntax.HeredocSpans(text, r.dialect())) {
		out, _ := r.expandSpan(s, splitNever)
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
	if name == "+" || name == "-" {
		// `~+` is $PWD and `~-` is $OLDPWD in three of the four, and only
		// when the variable is set: a fresh shell's `~-` stays literal.
		if v, ok := r.tildeDirVar(name); ok {
			s.Value = v + tail
		}
		return
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

// tildeDirVar resolves `~+` and `~-`, in the dialects that have them.
func (r *Runner) tildeDirVar(name string) (string, bool) {
	if !r.ask(r.sem().TildePlusMinusExpands, "`~+` and `~-` expanding to the directories") {
		return "", false
	}
	which := "PWD"
	if name == "-" {
		which = "OLDPWD"
	}
	v, ok := r.getVar(which)
	if !ok || v == "" {
		return "", false
	}
	return v, true
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

// paramErrorWord is what `${x?}` complains with.
//
// The word given, if there is one — `${x?custom}` says `x: custom` in every
// shell in the panel. Without one there is a default, and the default is
// where they part company. Plain `?` is unanimous: the parameter is not set.
// `:?` is four answers, because it covers two cases at once and each shell
// decides differently how to say so — two of them have a phrase covering
// both, one says only "not set" either way, and one tells them apart.
func (r *Runner) paramErrorWord(e *syntax.ParamExpr, set bool) string {
	if w := r.joinWord(e.Arg); w != "" {
		return w
	}
	const notSet = "parameter not set"
	if !e.Colon {
		return notSet
	}
	d := r.diag()
	if set && d.ParamNull != "" {
		// There and empty, which one dialect distinguishes from absent.
		return d.ParamNull
	}
	if d.ParamNullOrNotSet != "" {
		return d.ParamNullOrNotSet
	}
	return notSet
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
		r.fatalExpansion("%s\n", Wording(format, "%s: parameter not set", e.Name))
		return
	}
	if !isPositional(e.Name) {
		r.fatalExpansion("%s\n", Wording(r.diag().UnboundVariable, "%s: parameter not set", e.Name))
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
// Most of the table is unanimous across every shell that has the form at all,
// and docs/spec/grammar/tokenization.md records it escape by escape. Three
// places are not, and each is an axis rather than a choice made here: what
// `\c` means, what a backslash before an unclaimed character does, and
// whether a decoded NUL ends the text. All three are asked only where the
// input reaches them.
func (r *Runner) expandDollarSingle(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		switch c := s[i+1]; {
		case c == 'x':
			n, used := scanBase(s[i+2:], 16, 2)
			if used == 0 {
				b.WriteString(`\x`)
				i += 2
				continue
			}
			if !r.writeDecodedByte(&b, byte(n)) {
				return b.String()
			}
			i += 2 + used
		case c == 'u' || c == 'U':
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
			if n == 0 {
				if !r.writeDecodedByte(&b, 0) {
					return b.String()
				}
			} else {
				b.WriteRune(rune(n))
			}
			i += 2 + used
		case c >= '0' && c <= '7':
			n, used := scanBase(s[i+1:], 8, 3)
			if !r.writeDecodedByte(&b, byte(n)) {
				return b.String()
			}
			i += 1 + used
		case c == 'c':
			p := r.dollarSingleControl()
			if p == DollarSingleControlAbsent {
				// The shell has no `\c` escape, so the backslash is before a
				// character nothing claims and the unknown rule decides it.
				r.writeUnknownEscape(&b, c)
				i += 2
				continue
			}
			x, next, ok := controlArgument(p, s, i+2)
			if !ok {
				// Nothing left to make a control character out of. ksh93
				// drops the escape and produces nothing; bash keeps the two
				// characters, the way it keeps any escape it cannot read.
				if p != DollarSingleControlToggled {
					r.writeUnknownEscape(&b, c)
				}
				i += 2
				continue
			}
			if !r.writeDecodedByte(&b, controlByte(p, x)) {
				return b.String()
			}
			i = next
		default:
			if v, ok := simpleEscape(c); ok {
				b.WriteByte(v)
				i += 2
				continue
			}
			r.writeUnknownEscape(&b, c)
			i += 2
		}
	}
	return b.String()
}

// simpleEscape is the byte a one-character escape stands for, and whether the
// character names one at all.
//
// Every entry is unanimous across bash, ksh93 and zsh — dash has no `$'…'` to
// disagree with — so nothing here is an axis. It is a function rather than
// part of the loop because `\c` has to read the same table: ksh93 controls
// the character an escape *produced*, so `$'\c\t'` is control-tab there.
func simpleEscape(c byte) (byte, bool) {
	switch c {
	case 'n':
		return '\n', true
	case 't':
		return '\t', true
	case 'r':
		return '\r', true
	case 'a':
		return '\a', true
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	case 'v':
		return '\v', true
	case 'e', 'E':
		return 0x1b, true
	case '\\', '\'', '"', '?':
		return c, true
	}
	return 0, false
}

// controlArgument is the character `\c` applies to, where the escape ends, and
// whether there was anything there at all.
//
// The two decoding policies read the argument differently, and it shows only
// where the argument is itself written as an escape. bash takes the raw byte,
// so `$'\c\t'` is control-backslash followed by a `t` — with the one
// exception that a doubled backslash is read as the single character it
// stands for. ksh93 decodes first, so the same text is control-tab.
func controlArgument(p DollarSingleControlPolicy, s string, i int) (byte, int, bool) {
	switch {
	case i >= len(s):
		return 0, i, false
	case s[i] != '\\':
		return s[i], i + 1, true
	case i+1 >= len(s):
		// A backslash with nothing after it escapes the end of the text, so
		// what `\c` controls is that nothing: `\c\` at the end of a printf
		// format is control-NUL and not control-backslash. Only a format
		// reaches this — a `$'…'` cannot end in a lone backslash, because
		// the one before the closing quote escapes it.
		return 0, i + 1, true
	case p == DollarSingleControlMasked:
		if s[i+1] == '\\' {
			return '\\', i + 2, true
		}
		return '\\', i + 1, true
	}
	switch c := s[i+1]; {
	case c == 'x':
		if n, used := scanBase(s[i+2:], 16, 2); used > 0 {
			return byte(n), i + 2 + used, true
		}
	case c >= '0' && c <= '7':
		n, used := scanBase(s[i+1:], 8, 3)
		return byte(n), i + 1 + used, true
	default:
		if v, ok := simpleEscape(c); ok {
			return v, i + 2, true
		}
	}
	// An escape ksh93 does not know loses its backslash, and what `\c`
	// controls is the character that survives.
	return s[i+1], i + 2, true
}

// writeDecodedByte writes one byte an escape decoded to, reporting whether
// decoding carries on.
//
// A zero byte is the interesting one. Where a shell holds a word as a C
// string there is nothing after it to hold, so `$'a\0b'` is `a` — and only
// the *span* ends: `$'a\0b'ccc` is `accc`, because the rest of the word was
// never inside the quotes. zsh counts its strings and keeps all three bytes.
func (r *Runner) writeDecodedByte(b *strings.Builder, c byte) bool {
	if c == 0 && r.ask(r.sem().DollarSingleNulTruncates, `a NUL inside $'…'`) {
		return false
	}
	b.WriteByte(c)
	return true
}

// writeUnknownEscape writes a backslash before a character no escape claims.
func (r *Runner) writeUnknownEscape(b *strings.Builder, c byte) {
	if r.dollarSingleUnknown() == DollarSingleUnknownKeepsBackslash {
		b.WriteByte('\\')
	}
	b.WriteByte(c)
}

// controlByte is what `\cX` decodes to, which is two different arithmetics.
//
// Both uppercase a letter first — `$'\ca'` is 0x01 and not 0x21 anywhere that
// decodes it at all — and then either keep the low five bits or toggle bit 6.
// Those agree over `@` through `_`, which is every letter and six symbols,
// and disagree over everything else: `$'\c1'` is 0x11 one way and `q` the
// other.
func controlByte(p DollarSingleControlPolicy, x byte) byte {
	if x >= 'a' && x <= 'z' {
		x -= 'a' - 'A'
	}
	switch p {
	case DollarSingleControlMasked:
		if x == '?' {
			return 0x7f
		}
		return x & 0x1f
	case DollarSingleControlToggled:
		return x ^ 0x40
	}
	return 0
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
	for k := range r.inheritedEnv {
		add(k)
	}
	sort.Strings(out)
	return out
}

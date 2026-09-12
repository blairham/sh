// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"slices"
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
	return r.globFields(r.expandWordEscaped(w))
}

// expandWordEscaped is expandWord stopped one stage early: the fields are
// built, and each one still carries the marks saying which of its
// metacharacters were quoted. Pathname expansion has not run.
//
// It exists because a word can be expanded *inside* another word — the
// operand of `${x-word}` is the case that matters — and a nested word that
// globs on its own is wrong twice over. It matches against the filesystem
// with only its own text in hand, so `X${u:-[a-b]}y` matched `[a-b]` where
// every shell in the panel matches `X[a-b]y`; and it comes back unescaped,
// so the quoting inside it is gone by the time the enclosing word globs and
// `${u:-"X[a-b]y"}` was a pattern where all six read it as text (#1500).
//
// Handing the marked fields back instead lets the enclosing word do what it
// does with every other field: one match, at the end, over the whole thing.
func (r *Runner) expandWordEscaped(w *syntax.Word) []string {
	if w == nil {
		return nil
	}
	// Brace expansion comes first and can turn one word into several, so it
	// wraps the rest rather than being a stage inside it. One word back can
	// still be an expansion — `{1..1}` is `1`, and a range whose honored
	// step sign points away from the far endpoint holds one element — so
	// the test is whether the word changed, not whether it multiplied.
	// The run-time switch is read before the axis rather than beside it,
	// because the two answer different questions: this shell has braces
	// (the dialect's) and this script asked for them to stop (`set +B`).
	// Reading it first is also what keeps a turned-off expansion silent —
	// an unanswered range axis inside a brace nobody is going to expand is
	// not a disagreement worth refusing a script over.
	if words := r.braceExpand(w); !r.noBraceExpand &&
		(len(words) > 1 || len(words) == 1 && words[0] != w) &&
		r.ask(r.sem().BraceExpansion, "brace expansion") {
		var out []string
		for _, bw := range words {
			out = append(out, r.expandOneWordFields(bw)...)
		}
		return out
	}
	return r.expandOneWordFields(w)
}

// inWord records the word being expanded and returns the undo, so a
// diagnostic raised inside it can name the text the expansion sits in.
//
// It has to be put back rather than cleared: an expansion's operand is a word
// of its own — `${u:-${y@QQ}}` — and the inner one finishing does not mean the
// outer one has.
// The outermost word of the nesting is recorded alongside, and only the first
// call fills it in: a dialect that names "the word" means the one the command
// line holds, not the operand a failure happened to be reached through.
func (r *Runner) inWord(w *syntax.Word) func() {
	prevWord, prevSpan := r.expandingWord, r.expandingSpan
	prevOuter := r.expandingOuterWord
	r.expandingWord, r.expandingSpan = w, 0
	if r.expandingOuterWord == nil {
		r.expandingOuterWord = w
	}
	return func() {
		r.expandingWord, r.expandingSpan = prevWord, prevSpan
		r.expandingOuterWord = prevOuter
	}
}

// expandOneWord is the pipeline for a single word, after braces: fields, then
// pathname expansion over them.
func (r *Runner) expandOneWord(w *syntax.Word) []string {
	return r.globFields(r.expandOneWordFields(w))
}

// expandOneWordFields is that pipeline stopped before the match, giving the
// fields in their marked form. See expandWordEscaped for why the two stages
// are separable.
func (r *Runner) expandOneWordFields(w *syntax.Word) []string {
	if w == nil {
		return nil
	}
	r.expandTilde(w)
	r.expandEquals(w)

	// Fields are built up span by span. A span joins onto the field before it
	// unless splitting started a new one, which is what makes x$(f)y attach
	// its literal text to the first and last resulting fields.
	b := newWordFields()

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
		// The head of the word, for the `${~spec}` flag: nothing has been
		// accumulated in front of this span. An empty span in front of it
		// leaves the head where it was, which is measured — `${empty}${~t}`
		// expands and `x${~t}` does not.
		head := b.head()
		// `$@` is the one expansion that yields more than one field on its
		// own, so it cannot go through expandSpan, which returns a string.
		// The first parameter joins onto whatever precedes it and the last
		// stays open for whatever follows — which is why `x$@y` attaches its
		// literal text to the first and last fields rather than becoming
		// words of its own.
		if parts, ok := r.expandAt(s, splitByDialect, head); ok {
			b.add(s, parts)
			continue
		}
		text, split := r.expandSpan(s, splitByDialect, head)
		if !split {
			b.text(text)
			b.any = b.any || text != "" || s.Quoting != syntax.Unquoted
			continue
		}
		ifs, set := r.ifs()
		// The `${~spec}` flag's tilde half reaches each field the value
		// split into, not only the head of the value: measured under
		// `SH_WORD_SPLIT`, `v='~/zz ~/qq'; ${~v}` is *both* directories in
		// the shell that has the construct. Splitting runs first and every
		// field it produced is at the head of a word of its own, exactly as
		// the elements of a list are.
		//
		// An unquoted expansion of an empty value produces no field at all,
		// so ordinarily nothing is appended and nothing is started — which
		// add answers, along with the one reading that is not "nothing":
		// a distributive span with no elements takes the word with it.
		b.add(s, r.tildeFlagElements(s, head, r.splitFieldsAsk(text, ifs, set)))
	}

	return b.result()
}

// wordFields is the fields of one word as it is assembled, span by span.
//
// The shape is a run of finished fields followed by a run of *open* ones:
// text from a later span joins every open field, and every open field is
// finished the moment a span starts a new one after it. Ordinarily the open
// run holds exactly one field — the word being built — and it grows only
// where a distributive expansion multiplies it, which is why the state is a
// run and not the single trailing field it used to be.
//
// One copy, because there were two: an ordinary word and a redirection
// target, which reads its own word twice and had the lay-in rule typed out a
// second time. globFields is a function for the same reason, and this is the
// stage in front of it.
type wordFields struct {
	// all is the fields, finished ones first.
	all []string
	// open is where the run of open fields begins. all[:open] is finished.
	open int
	// any is whether anything at all reached the word — a substitution that
	// produced a field, or literal text, or a quoted empty span. Without it
	// a word that expanded to nothing cannot be told from a word that was
	// never there, and the two are different: one field or none.
	any bool
}

func newWordFields() wordFields { return wordFields{all: []string{""}} }

// head reports whether nothing has been accumulated in front of the next
// span, which is what the `${~spec}` flag's tilde half asks about.
func (b *wordFields) head() bool { return len(b.all) == 1 && b.all[0] == "" }

// text joins literal or unsplit text onto every field still open.
func (b *wordFields) text(t string) {
	for i := b.open; i < len(b.all); i++ {
		b.all[i] += t
	}
}

// add puts the fields one span produced into the word, by whichever of the
// two rules the span asks for.
func (b *wordFields) add(s syntax.Span, parts []string) {
	if rcExpandOn(s) {
		b.spread(parts)
		return
	}
	b.lay(parts)
}

// lay is the ordinary rule: the first field joins whatever is open, the last
// stays open for whatever follows, and everything between is a word of its
// own. It is what makes `x$@y` attach its literal text to the first and last
// fields rather than becoming words of its own.
func (b *wordFields) lay(parts []string) {
	if len(parts) == 0 {
		return
	}
	b.any = true
	b.text(parts[0])
	if len(parts) == 1 {
		return
	}
	// The first field closed everything that was open, since a new one
	// started after it.
	b.all = append(b.all, parts[1:]...)
	b.open = len(b.all) - 1
}

// spread is the distributive rule the `${^spec}` flag asks for: every open
// field is produced once per part, and all of them stay open, so a second
// distributive span in the same word is a cross product.
//
// Order is measured: `a=(1 2); ${^a}z${^a}` is `1z1 1z2 2z1 2z2`, so the open
// fields are the outer loop and the later span varies fastest.
//
// No parts is not "nothing happens", which is the one place this parts
// company with lay: the word is produced once per element and there are no
// elements, so it is produced no times. Measured, `a=(); x${^a}y` is no word
// at all where `x${a}y` is the single word `xy` — and a field finished before
// it still stands, `a=(1 2); b=(); x${a}z${^b}q` being the single word `x1`.
func (b *wordFields) spread(parts []string) {
	open := b.all[b.open:]
	all := make([]string, 0, b.open+len(open)*len(parts))
	all = append(all, b.all[:b.open]...)
	for _, f := range open {
		for _, p := range parts {
			all = append(all, f+p)
		}
	}
	b.all = all
	// Guarded rather than unconditional, and the guard is equivalent rather
	// than load-bearing: with no parts the open run is now empty, so either
	// there are finished fields — which any cannot change the reading of —
	// or there are none and result answers nil before any is consulted. A
	// mutant setting it unconditionally survives the suite on purpose. It
	// stays because "something was substituted" is false when nothing was,
	// and the field that says so should not be made to lie by a caller that
	// happens not to look.
	if len(parts) > 0 {
		b.any = true
	}
}

// result is the fields the word came to, with the two shapes that are no
// field at all reported as nil: a word every span left empty, and a word a
// distributive span with no elements took away.
//
// The second guard is nil against an empty non-nil slice, which every caller
// here reads the same way — the same equivalence globFields writes down, and
// a mutant deleting it survives the suite for the same reason. It stays
// because "no fields at all" has one spelling in this pipeline and both ways
// of arriving there should use it.
func (b *wordFields) result() []string {
	if len(b.all) == 0 {
		return nil
	}
	if len(b.all) == 1 && b.all[0] == "" && !b.any {
		return nil
	}
	return b.all
}

// globFields is pathname expansion, the last stage of a word: it acts on
// whole fields, and a pattern that matches nothing is passed through
// unchanged — unless the run-time option deletes it, which is what glob's
// second result reports. The marks come off here, once the match has had its
// look at them.
//
// One copy, because there are two callers and they were separate loops: an
// ordinary word and a redirection target, which reads its own word twice. A
// change made to one of them and not the other is the shape this repository
// keeps finding, so the stage is a function rather than a paragraph typed
// twice.
//
// The nil guard is equivalent rather than load-bearing, and is written down
// as such: without it a nil argument comes back as an empty non-nil slice,
// which every caller here reads the same way — `len(…) == 0`, or a join that
// is the empty string either way. A mutant deleting it survives the suite on
// purpose. It stays because the nil is the "no fields at all" answer the word
// pipeline returns, and round-tripping it through a stage should not quietly
// change which of the two a caller gets back.
func (r *Runner) globFields(fields []string) []string {
	if fields == nil {
		return nil
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
	return []string{r.wordTextNoSplit(w, nil)}
}

// wordTextNoSplit is expandWordNoSplit's one loop, with a seam for the caller
// that needs to know which part of the finished text was quoted.
//
// mark, where it is not nil, is handed each span's text together with the
// quoting that span was written in, and what it answers is what goes into the
// word. That is the only channel quoting has left by this point: a word is a
// sequence of spans precisely so the expander can tell the quoted parts from
// the live ones, and joining them into a string is where that is spent.
//
// The glob marks are removed a span at a time rather than once at the end,
// which is the same string: a mark is always written immediately in front of
// the byte it marks and both come out of one span, so no mark straddles a
// boundary. Doing it here is what lets mark see the text a script would, and
// lets the marks it adds of its own survive to the caller.
func (r *Runner) wordTextNoSplit(w *syntax.Word, mark func(string, syntax.Quoting) string) string {
	return r.wordTextUnsplit(w, mark, false, false)
}

// wordTextGlobMarked is the same one word, the same one pass, with the glob
// marks left **in**: the finished text still says which of its
// metacharacters were written live and which came out of quotes.
//
// The one caller is a `[[ … ]]` operand that ends in a glob qualifier group,
// which is the one word in a condition that goes on to match against the
// filesystem — see interp/condqualifier.go. It needs the marked form for the
// reason expandWordEscaped gives: an unmarked field has lost the difference
// between an asterisk a script wrote and one a parameter held, and no amount
// of escaping afterwards can put it back.
//
// Nested globbing stays suspended exactly as it is for every other unsplit
// word. What is different here is only where the marks come off — at the
// match, by the caller, rather than a span at a time on the way past.
func (r *Runner) wordTextGlobMarked(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	return r.wordTextUnsplit(w, nil, true, false)
}

// wordTextUnsplit is the loop all three share. keepMarks says the glob
// marks survive it; colonTildes says the word is an assignment's value, where
// a colon begins a tilde segment of its own.
func (r *Runner) wordTextUnsplit(w *syntax.Word, mark func(string, syntax.Quoting) string, keepMarks, colonTildes bool) string {
	r.expandTilde(w)
	failed := r.expandErr
	// "Without globbing" has to reach the *nested* expansions too, and it did
	// not: a `:-` word builds its fields through expandWord, which matches,
	// so `p=${u:-*}` assigned the directory listing where every shell in the
	// panel assigns one asterisk. The promise this entry point makes is kept
	// here rather than at each of the places that could break it.
	defer r.withoutGlobbing()()
	defer r.inWord(w)()
	var b strings.Builder
	for i, s := range w.Spans {
		if (r.expandErr && !failed) || r.ctl == controlExit {
			// The word is abandoned at its first failed expansion, here as
			// in the splitting path.
			break
		}
		r.expandingSpan = i
		head := b.Len() == 0
		// An assignment's colon begins a tilde segment, so a substituted
		// tilde is at a head after one as surely as at the front of the
		// value: `q=a:${~t}` with `t='~/zz'` is the home directory, and
		// `q=a:${t}` — the same value without the flag — is not. Measured on
		// zsh 5.9.2, the one grammar in the panel with the flag. The colon
		// counts whatever it came from, a literal or another expansion, and
		// counts through quotes: `q="a:"${~t}` expands too.
		if colonTildes && !head && strings.HasSuffix(b.String(), ":") {
			head = true
		}
		var text string
		if parts, ok := r.expandAt(s, splitNever, head); ok {
			text = r.joinUnsplit(s.Param, parts)
		} else {
			text, _ = r.expandSpan(s, splitNever, head)
		}
		if !keepMarks {
			text = globUnescape(text)
		}
		// The colons *inside* the substituted text begin segments of their
		// own, and those do not depend on the head: `q=a${~p}` with
		// `p='~/x:~/y'` keeps the first tilde, which is not at a head, and
		// expands the second, which follows a colon.
		if colonTildes && tildeFlagOn(s) {
			text = r.substitutedColonTildes(text)
		}
		if mark != nil {
			text = mark(text, s.Quoting)
		}
		b.WriteString(text)
	}
	return b.String()
}

// expandRedirectTargetViews expands a redirection's target once and returns
// the three readings of it: the fields an ordinary word would have become,
// the words it comes to when nothing is *split* but everything else happens,
// and the text it comes to when nothing is split or matched at all.
//
// One pass, because the readings must not each run the command substitutions
// in `> $(f)`. And a pass of its own rather than three calls, because
// splitting is quoting-aware — `"$e"` with a space in it is one field and
// `$e` is two — so the unsplit text cannot be recovered by joining the
// fields, and the fields cannot be recovered by splitting the text.
//
// The fields view splits without consulting the splitting axis: it exists to
// show what the ordinary-word reading *would* be, and whether that reading
// applies is the redirection's own axis, asked by the caller exactly where
// the views differ. Asking here as well made the bare core refuse
// `> $two` for splitting — the wrong axis, and asked even when the target
// was one word under both readings.
//
// The **words** view is the middle one, and it is the reading of the dialect
// that does not split a target: an array is still several words there, a
// pattern is still matched, and a scalar holding a space is still one name.
// `v=(f g); cat <$v` is two words and `e="f g"; cat <$e` is one, which is
// exactly the difference the text view cannot express — it joins both to
// `f g`. See redirectTarget, which is the only caller and which decides what
// several words mean (#1792).
func (r *Runner) expandRedirectTargetViews(w *syntax.Word) (fields, words []string, plain string) {
	if w == nil {
		return nil, nil, ""
	}
	r.expandTilde(w)

	f := newWordFields()
	u := newWordFields()
	var b strings.Builder

	for _, s := range w.Spans {
		head := b.Len() == 0
		if parts, ok := r.expandAt(s, splitAlways, head); ok {
			// The plain view is the one that keeps no fields, so it is the
			// one the separator rule applies to. The fields view below is
			// untouched: whether the target is read as fields at all is the
			// redirection's own axis, and it is asked by the caller.
			b.WriteString(r.joinUnsplit(s.Param, parts))
			// add and not lay, though nothing can tell them apart here
			// today: a target holding a distributive expansion is not one
			// field under either rule, so redirectTarget's two readings
			// already differ and the axis sends the one dialect that has
			// the flag to the plain view. A mutant writing lay here
			// survives. It stays add because the lay-in rule has one home,
			// and the shape this repository keeps finding is the second
			// copy that did not get the change.
			f.add(s, parts)
			// The words view takes the same parts: an array is several words
			// however the splitting axis is answered, which is the half of
			// this reading that is not the text view.
			u.add(s, parts)
			continue
		}
		text, split := r.expandSpan(s, splitAlways, head)
		b.WriteString(text)
		// And never splits, whatever the span asked for. That is the whole
		// of the difference from the fields view below.
		u.text(text)
		u.any = u.any || text != "" || s.Quoting != syntax.Unquoted
		if !split {
			f.text(text)
			f.any = f.any || text != "" || s.Quoting != syntax.Unquoted
			continue
		}
		ifs, set := r.ifs()
		f.add(s, r.splitFieldsAsk(text, ifs, set))
	}
	plain = globUnescape(b.String())

	words = u.result()
	if words != nil {
		words = r.globFields(words)
	}
	fields = f.result()
	if fields == nil {
		return nil, words, plain
	}
	return r.globFields(fields), words, plain
}

// substitutedWordFields expands the word a `-` or `+` substituted, keeping the
// fields it produces rather than joining them.
//
// It answers false when this expansion is not one of those, or when the word
// is not what it came to — both of which leave the caller to carry on as
// before. The test for which it came to is testFires, the same one
// expandParam applies, so the two cannot drift apart.
func (r *Runner) substitutedWordFields(s syntax.Span, head bool) ([]string, bool) {
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
	if s.Quoting != syntax.Unquoted {
		// Quoted, so the word substitutes as *text* and nothing in it is a
		// pattern. Measured on zsh 5.9.2: `"${nosuch:-*}"` is one asterisk
		// and `"${nosuch:-*(.)}"` is four characters, where this listed the
		// directory — the escaping ran after the matching rather than
		// instead of it, which no amount of escaping afterwards can undo.
		//
		// Switched off through the same field `set -f` uses, because it is
		// the same question asked from a different place.
		defer r.withoutGlobbing()()
		return escapeAll(r.expandWord(e.Arg)), true
	}
	// Unquoted, so the word's own metacharacters are live and its quoted
	// ones are not — which is a distinction only the marked form can carry,
	// and expandWord drops it. Both halves of that were wrong against all
	// six shells: `${u:-"X[a-b]y"}` globbed where every one of them prints
	// the seven characters, and `X${u:-[a-b]}y` matched the operand alone
	// where every one of them matches the whole word (#1500).
	//
	// So the fields go back marked and the enclosing word matches them, in
	// the one place it matches every other field.
	return r.tildeFlagFields(s, head, r.expandWordEscaped(e.Arg)), true
}

// withoutGlobbing suspends pathname expansion for one nested expansion, and
// returns the restore. The contexts that want it are the ones where a word
// substitutes as text rather than as a pattern: a quoted `:-` word, an
// operand that is an arithmetic expression, and every word read through
// expandWordNoSplit.
//
// Not `set -f`, which is the same effect and a different fact: that one is an
// option the script chose and `$-` reports it, and borrowing it made `$-`
// answer for a flag this had flipped underneath it.
func (r *Runner) withoutGlobbing() func() {
	saved := r.globSuspended
	r.globSuspended = true
	return func() { r.globSuspended = saved }
}

// paramSource is the value an expansion starts from and whether it was set at
// all, before any operator is applied.
//
// Shared with the two field-level questions below, so that "was it set" is
// asked in one place. Answering it twice is how the joined and the split paths
// would come to disagree about the same expansion.
func (r *Runner) paramSource(e *syntax.ParamExpr) (value string, set, subscript bool) {
	if e.Inner != nil {
		// An expansion standing where a name would. Its fields are joined
		// here because this is the scalar view; the list view is
		// nestedFields, and both go through nestedWords so the two cannot
		// come to different values.
		words, iset, _ := r.nestedWords(e)
		return strings.Join(words, ifsFirst(r.ifs())), iset, false
	}
	if e.Index != nil {
		if elems, ok := r.arraySubscript(e); ok {
			// nil rather than empty is what says the element was not there:
			// an element holding "" is set, and `${a[0]:-d}` has to tell the
			// two apart.
			//
			// The separator is the same question the two word-level callers
			// ask, and this is the third place that was answering it with a
			// hard space. It is what a here-document body reaches: the body
			// is lexed as double-quoted text and expanded span by span, so a
			// whole-array subscript in one arrives here rather than at
			// expandAt, and `IFS=-; a=(x y z); cat <<E` printed `x y z`
			// where zsh prints `x-y-z`. A single element joins with nothing,
			// so every `${a[0]}` is unaffected by construction.
			return r.joinUnsplit(e, elems), elems != nil, true
		}
	}
	// A special parameter supplies a *value*; it does not skip the operators.
	// Returning here was a bug: `${1##*/}` left its argument untouched,
	// because the positional parameter answered and the trim never ran.
	value, set = r.specialParam(e)
	if !set {
		value, set = r.getVar(e.Name)
	}
	if e.Name == "!" && !r.lastJobPIDSet &&
		r.sem().LastBackgroundPidIsUnsetBeforeAnyJob == Yes {
		// `$!` before anything has been started is *unset* in two of the
		// four shells rather than set and empty, and `set -u` is fatal about
		// it there. Asked here rather than in specialParam because that
		// function's bool says "this is a parameter and not a variable" —
		// three other callers read it that way — and answering false would
		// send `$!` off to look for a variable of that name.
		//
		// An indirection cannot reach this and needs no guard of its own:
		// `${!x}` parses with the name `x` and the indirect flag set, so the
		// name here is never `!` for one. A `!e.Indirect` clause was written
		// and was dead — a surviving mutant is what said so.
		set = false
	}
	return value, set, false
}

// yieldsTheArray reports whether one of the four conditional expansions came
// to the parameter rather than to its word.
//
// The mirror of substitutedWordFields, and needed for the same reason:
// `"${a[@]-${a[@]}}"` on a set array is the *array*, and it keeps its fields
// exactly as `"${a[@]}"` does. Without this it fell to the scalar path and
// came back as one joined string.
//
// All four of `-`, `=`, `?` and `+` have the same two outcomes — the word or
// the parameter — so all four belong here. `=` and `?` were missing, and both
// silently joined: `"${a[@]:=d}"` and `"${a[@]:?e}"` on a two-element array
// were one field holding `one two` where every shell with arrays gives two,
// and the bare-name spellings followed them down. Nothing failed, because a
// joined array is a plausible string; the field count is the only tell, which
// is why the tests count fields rather than compare text.
//
// The direction differs by operator and that is the whole content of the
// switch. `-`, `=` and `?` substitute their word exactly when the test fires,
// so the parameter is what is left when it does not. `+` is the other way
// round — it substitutes the word when the test does *not* fire — and a fired
// `+` yields nothing rather than the word, which the array path answers with
// no fields at all.
//
// `=` and `?` are only ever the parameter on the non-firing side, so their
// side effects stay where they are: a fired `=` still assigns down the scalar
// path and a fired `?` is still fatal there. Answering true for a fired `?`
// would take the array path and never raise the error — silently returning
// the array a script asked to be told was missing.
func (r *Runner) yieldsTheArray(e *syntax.ParamExpr) bool {
	switch e.Op {
	case syntax.ParamDefault, syntax.ParamAssign, syntax.ParamError:
		return !r.testFires(e)
	case syntax.ParamAlternate:
		return r.testFires(e)
	}
	return false
}

// testFires reports whether the `-`/`+` test fires: unset, or unset-or-empty
// when a colon was written. The same rule expandParam applies, from the same
// source, so the joined and the split paths cannot disagree.
func (r *Runner) testFires(e *syntax.ParamExpr) bool {
	value, set, _ := r.paramSource(e)
	return r.conditionalFires(e, value, set)
}

// conditionalFires is the test the four conditional operators share: unset,
// or unset-or-empty where a colon was written.
//
// One function because the joined path and the split path both ask it, and
// because the colon-less form asks a Semantics axis that the colon form must
// not — see listOfNoPositionalsIsSet.
func (r *Runner) conditionalFires(e *syntax.ParamExpr, value string, set bool) bool {
	if e.Colon {
		return !set || value == ""
	}
	return !r.listOfNoPositionalsIsSet(e, set)
}

// listOfNoPositionalsIsSet resolves Semantics.PositionalListWithNoneIsSet for
// one colon-less conditional, and leaves every other parameter as it found
// it.
//
// Three guards, and each is a place the panel agrees:
//
//   - the parameter has to be `$@` or `$*` itself. A name with a subscript
//     that happens to be `@` is an array and answers elsewhere.
//   - there have to be **no** positional parameters. With any, all six
//     columns call the list set.
//   - the colon form never gets here, because it fires on the empty value
//     whichever way the set-ness question is answered. `${@:=abc}` is
//     refused in all six (#1541) and needs no axis to be.
func (r *Runner) listOfNoPositionalsIsSet(e *syntax.ParamExpr, set bool) bool {
	if !set || len(r.Params) > 0 || e.Subscript() != nil ||
		(e.Name != "@" && e.Name != "*") {
		return set
	}
	return r.ask(r.sem().PositionalListWithNoneIsSet,
		"whether `$@` with no positional parameters is a set parameter")
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
	return r.wordTextUnsplit(w, nil, false, true)
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
// head has the same meaning it has for expandSpan, and reaches the elements
// through tildeFlagElements: only the first of them can be denied a head, the
// rest are fields of their own.
func (r *Runner) expandAt(s syntax.Span, sp splitPolicy, head bool) ([]string, bool) {
	// The hold belongs to one span and is consumed by the scalar path this
	// function falls through to. Cleared here so that a span nothing fell
	// through for cannot leave one behind for a later expansion of the same
	// node — a `for` loop expands one node many times, and a value from the
	// wrong pass is exactly the silent kind of wrong.
	r.nestedHeld = nestedHold{}
	// The same rewrite the scalar path makes, made first: the shapes below
	// are read off the node, and a node still carrying a subscript nothing
	// is going to read would be answered as `$a[@]` rather than as `$a`
	// followed by three characters. See baresubscript.go.
	s, tail := r.unreadBareSubscript(s)
	if parts, ok := r.expandAtList(s, sp, head); ok {
		parts = r.splitFlagFields(s, sp, parts)
		if tail != nil {
			text, _ := r.bareSubscriptText(tail, sp)
			// Onto the last field, which is the one still open for whatever
			// follows the expansion — and a field of its own where the
			// expansion produced none, since the brackets are text and text
			// makes a word whether or not anything expanded in front of it.
			if len(parts) == 0 {
				parts = []string{text}
			} else {
				parts[len(parts)-1] += text
			}
		}
		return parts, true
	}
	if !splitFlagOn(s, sp) {
		return nil, false
	}
	// A `${=spec}` on one of the scalar shapes. It is answered here rather
	// than by the word loop because the loop splits an unquoted result only,
	// and this flag reaches through the quotes: `"${=v}"` on `a b` is two
	// fields. The value itself is the scalar path's, unchanged — expandSpan
	// is exactly the call the loop would have made — so the flag adds the
	// splitting and nothing else.
	text, _ := r.expandSpan(s, sp, head)
	return r.tildeFlagElements(s, head, r.splitFlagFields(s, sp, []string{text})), true
}

// expandAtList answers the expansions that yield a list of fields on their
// own. See expandAt, which is the entry point and applies `${=spec}` to
// whatever this returns.
func (r *Runner) expandAtList(s syntax.Span, sp splitPolicy, head bool) ([]string, bool) {
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
	if r.refuseAbsentParameter(s.Param) {
		// Before every shape below, because the shape that loses the read is
		// the subscript one: `${jobstates[x]}` is answered here with no
		// fields at all and never reaches the scalar path, so a test placed
		// where a value is fetched never sees it. See absentparam.go.
		return nil, true
	}
	// A flag group changes what the whole expansion yields — how many
	// fields, joined with what — so a node that carries one is answered by
	// its own pipeline, before any of the shapes below are considered.
	if fields, ok := r.expandFlagged(s, sp, head); ok {
		return fields, true
	}
	e := s.Param
	// `${+name}` is a count of set-ness and not a value, so none of the
	// shapes below applies to it — and this stands in front of the bare-array
	// rewrite in particular, which would otherwise turn `${+a}` into
	// `${+a[@]}` and answer with one field per element.
	if setTestAnswers(e) {
		_, set, _ := r.paramSource(e)
		return []string{setTestResult(set)}, true
	}
	// A bare array name is the *array* in one dialect, so the node is given
	// the subscript that says so and the array path below answers it. See
	// bareArrayAsList for why that is a rewrite rather than a path of its own.
	if listed, ok := r.bareArrayAsList(e, s, sp); ok {
		e = listed
	}
	// `$@` and `$*` under an operator are the *parameters*, one at a time,
	// and the same rewrite is what says so. See positionalsAsList.
	if listed, ok := r.positionalsAsList(e); ok {
		e = listed
	}
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
			return r.splitFieldsAskPlain(joined, ifs, set), true
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
	if fields, ok := r.substitutedWordFields(s, head); ok {
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
	// A nested expansion whose inner came to a list arrives here too, and by
	// the same route: what it stands for is a list of values, so everything
	// below — the slice, the element filter, the per-element operators, the
	// join and the splitting — is the same question it is for an array. See
	// listBase, which is the one place the two sources meet.
	if elems, ok := r.listBase(e); ok {
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
			elems = r.positionalSliceElems(e, elems)
			elems = sliceElems(elems, r.numOf(e.Arg, e, e.Arg2), e, r)
		}
		if zipsElements(e.Op) {
			// Its own branch beside the four below rather than one of them:
			// those choose which elements survive and this produces a list
			// longer than either input, so reshapeScalar's string answer has
			// nowhere to put the second field. Quoted, the left operand is
			// one word before the zip sees it — which is why `"${a:^b}"` on
			// `(1 2 3)` and `(x y z)` is the two fields `1 2 3` and `x`, and
			// why that falls out of the rule rather than needing one.
			if s.Quoting != syntax.Unquoted {
				elems = []string{strings.Join(elems, ifsFirst(r.ifs()))}
				elems = r.zipElements(e, elems)
				if len(elems) == 0 {
					// The same guarantee the element filter below
					// records, and the only probe that can see it is a
					// field *count*: `a=(1 2 3); b=(); "${a:^b}"` is one
					// empty field where the unquoted spelling is none.
					// `printf '[%s]'` prints `[]` for no arguments at
					// all, so it answers both the same and reported this
					// as already correct.
					elems = []string{""}
				}
			} else {
				elems = r.zipElements(e, elems)
			}
		}
		if reshapesElements(e.Op) {
			// Which elements there are, rather than what each one
			// becomes — so this is here beside the slice and not with
			// the elementOp mapping further down, whose whole shape is
			// one output per input.
			//
			// Quoting decides what the operator is even looking at, and
			// this distributed regardless. The rule, measured: quotes
			// join first and `[*]` joins last. A quoted `"${a[*]:#p}"`
			// hands the operator the *joined string* and tests that one
			// value, so on `(foo bar baz)` with `ba*` nothing is dropped
			// and the whole array comes back — where filtering leaves
			// `foo`. That was the silent direction: a filter that ran
			// where the shell would have left the array alone, with no
			// diagnostic and status 0.
			//
			// A dropped value is one *empty* field rather than no field,
			// which is what quoting guarantees and what selectScalar
			// returning "" gives: `"${a[*]:*nope}"` is `n=1` in the shell
			// that has the operator, against `n=0` for the unquoted
			// spelling and for `[@]`.
			if s.Quoting != syntax.Unquoted && r.subscriptJoinsElements(e) {
				elems = []string{r.reshapeScalar(e, strings.Join(elems, ifsFirst(r.ifs())))}
			} else {
				elems = r.reshapeElements(e, elems)
			}
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
			if r.subscriptJoinsElements(e) && s.Quoting != syntax.Unquoted {
				// `"${a[*]#p}"` splits the panel: two shells trim each
				// element and join what is left, the third joins first
				// and trims the joined string once. Asked only when the
				// two readings actually differ — `${a[*]%b}` on `(aa ab)`
				// is `aa a` either way, and needs no answer.
				//
				// And asked only when it is *quoted*, which is the other
				// half of the same rule. The axis is a question about the
				// quoted form; the unquoted one has an answer nobody has
				// to be asked for — `a=(oxo yo); printf "[%s]" ${a[*]%o}`
				// is `[ox][y]` in bash, bash 3.2, ksh93 and zsh alike, so
				// the operator distributes and the elements go on to the
				// join below. Asking here gave the unquoted spelling the
				// quoted reading, and in the one dialect that answers no
				// it came back as a single field holding `oxo y`: the trim
				// silently applied to a boundary instead of to an element.
				//
				// *Which* subscripts join in quotes is
				// subscriptJoinsElements' question, and the branch above
				// already asks it that way. This spelled out `[*]` and a
				// search instead, which is that predicate with its range
				// clause struck off, so a quoted range trimmed each element
				// where the shell trims the joined string once: measured on
				// `a=(xa xb xc)`, `"${a[1,3]#x}"` and `"${a[1,3]/x/Y}"` are
				// `a xb xc` and `Ya xb xc` there, exactly what `[*]` gives,
				// against `a b c` and `Ya Yb Yc` from the reading that
				// distributed. The axis is only ever reached in the one
				// grammar that reads a comma as a range, which is the same
				// grammar `[*]` already asks it in.
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
			if s.Quoting != syntax.Unquoted {
				return []string{globEscape(strings.Join(elems, ifsFirst(ifs, set)))}, true
			}
			// Unquoted, the join is a dialect's answer rather than the
			// spelling's, and it is the same answer `[@]` asks — measured,
			// an unquoted `[*]` and an unquoted `[@]` are the same fields
			// in every shell in the panel. So this hands the elements to
			// the list path instead of joining them here: bash joins them
			// there and zsh, ksh93 and dash do not.
			//
			// The join here was unconditional, which is bash's answer
			// given to all four. zsh does not join an unquoted `[*]` at
			// all — `a=("x y" z); printf "[%s]" ${a[*]}` is `[x y][z]`
			// there, which no arrangement of the splitting answer reaches,
			// since the element boundary the join destroys cannot be put
			// back by any later stage.
			//
			// It also picks up the two stages `[@]` already asks about:
			// this path never glob-escaped, so `a=("zz*" other)` matched
			// the directory in the zsh dialect, where the shell leaves the
			// star alone.
			return r.tildeFlagElements(s, head, r.elementFields(elems, sp, r.globSubstAnswer(s))), true
		}
		if s.Quoting != syntax.Unquoted {
			if len(elems) == 0 && e.Op == syntax.ParamNone {
				if !r.wholeArrayIndex(e) {
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
				if r.subscriptNameIsAbsent(e) &&
					r.ask(r.sem().UnsetNameAtIsOneEmptyField,
						`a quoted "${a[@]}" on a name that holds nothing`) {
					// One dialect reads a name that is not a declared
					// array as a scalar, so a quoted whole-array
					// subscript on one nothing ever gave a value to is
					// the empty field `"$a"` would give.
					//
					// Guarded by the name being absent, which is the
					// only half that splits the panel. An array that
					// *exists* and has no elements is no field in every
					// column measured, so it falls through to the empty
					// slice below and asks nobody. The axis was asked
					// without that guard, which gave a declared empty
					// array the unset answer — and, in the dialect that
					// said yes, gave a spurious empty argument to every
					// `f "${a[@]}"` and `set -- "${a[@]}"` written
					// before anything filled the array, at status 0.
					return []string{""}, true
				}
			}
			return escapeAll(elems), true
		}
		return r.tildeFlagElements(s, head, r.elementFields(elems, sp, r.globSubstAnswer(s))), true
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
			return r.splitFieldsAskPlain(joined, ifs, set), true
		}
		if s.Quoting != syntax.Unquoted {
			return escapeAll(elems), true
		}
		return r.tildeFlagElements(s, head, r.elementFields(elems, sp, r.globSubstAnswer(s))), true
	}
	if e.Op != syntax.ParamNone || e.Length {
		return nil, false
	}
	if e.Name == "*" {
		// A bare `$*` unquoted is the same question the subscripted spelling
		// asks, reached by its own branch: it was joining unconditionally too,
		// on the scalar path, so `IFS=:; set -- x y; printf "[%s]" $*` was one
		// field `x:y` in the zsh dialect where the shell gives two.
		//
		// Only unquoted. `"$*"` is one field holding the join in every shell
		// measured, and it stays on the scalar path that produces it — the
		// same division the subscripted spelling keeps above.
		if s.Quoting != syntax.Unquoted {
			return nil, false
		}
		return r.tildeFlagElements(s, head, r.elementFields(r.Params, sp, r.globSubstAnswer(s))), true
	}
	if e.Name != "@" {
		return nil, false
	}
	if s.Quoting != syntax.Unquoted {
		// Escaped for the same reason every other quoted expansion is: the
		// fields go on to pathname expansion, and a `*` in a *value* is not
		// a pattern. Returning them raw made `set -- "$x"` glob.
		return escapeAll(r.Params), true
	}
	// Unquoted, each parameter goes through the same two stages every other
	// expansion does.
	return r.tildeFlagElements(s, head, r.elementFields(r.Params, sp, r.globSubstAnswer(s))), true
}

// elementFields is what an unquoted list expansion yields: the fields its
// elements become, one element at a time.
//
// The two stages that follow an expansion are the dialect's — whether the
// result is split on IFS, and whether what comes out is read as a pattern —
// and this path was performing both without asking either. Every element went
// through splitFields unconditionally and none was ever glob-escaped, so an
// element holding a separator became two arguments and one holding a `*`
// became whatever the directory happened to contain, at status 0 both times.
// `$@`, `${a[@]}` and — since a bare array name is the elements — `$a` all
// arrive here, which is why `for f in $files` was wrong for any filename with
// a space in it.
//
// Asked per element and only where the answer could change the result, which
// is the same discipline expansionResult follows for the scalar path: a
// element with no separator in it is not split either way, and one with no
// metacharacter is not a pattern either way.
func (r *Runner) elementFields(elems []string, sp splitPolicy, glob Answer) []string {
	perElement := r.splitEachElement(elems, sp, glob)
	if !r.listCouldJoinDifferently(elems) {
		return perElement
	}
	// The join is only ever a *different* reading where the split then runs
	// on what it produced. With splitting off there is nothing to undo it and
	// the whole list would come back as one field, which no shell in the
	// panel does — zsh has its splitting off by default and still gives one
	// field per element. So the splitting answer stands in front of this one,
	// and where it says no the question is never reached.
	if !r.ask(sp.answer(r.sem().SplitParamExpansion),
		"splitting an unquoted parameter expansion") {
		return perElement
	}
	ifs, set := r.ifs()
	joined := r.splitEachElement([]string{strings.Join(elems, ifsFirst(ifs, set))}, sp, glob)
	if slices.Equal(perElement, joined) {
		// The two readings coincide, which is the common case: under a
		// whitespace IFS a run of separators is one delimiter and an empty
		// element leaves nothing behind either way. Asking here would make
		// every `for f in $@` demand a dialect for a question that has only
		// one answer — see the note on UnquotedListJoinsOnIFS.
		return perElement
	}
	if r.ask(r.sem().UnquotedListJoinsOnIFS,
		"an unquoted list joining its elements before it is split") {
		return joined
	}
	return perElement
}

// unsplitJoinSeparator is the character a list expansion is joined with when
// it reaches a context that keeps no fields — an assignment's value, a `case`
// subject, a `[[ ]]` operand, a here-document body, a redirection's target.
//
// It exists because those callers were joining on a hard space, chosen before
// there was a rule. Silent in the shape that is hardest to see: the default
// IFS begins with a space, so every script that leaves IFS alone got the right
// answer and the one that sets it got a wrong one at status 0.
//
// Two spellings and two different questions:
//
//   - `*` — `$*`, `${a[*]}`, and a range subscript, which joins on the same
//     side of that line as the name it was written on. The first character of
//     IFS in every graded dialect's shell, so no answer is needed. bash 3.2
//     joins an unquoted `${a[*]}` on a space instead, which is the one
//     deviation in the panel and is recorded rather than modeled: it is a
//     version this repository grades nothing against, and the same build
//     joins `$*` on IFS, so the shell disagrees with itself.
//   - `@` — `$@` and `${a[@]}`, which is UnsplitAtListJoinsOnIFS. bash and
//     ksh93 rejoin on a space where zsh and dash use IFS.
//
// The separator is decided from the *node* rather than from the fields,
// because the two spellings produce identical fields and differ only in what
// the caller is entitled to do with them. That is also why the join lives at
// the callers and not inside expandAt: expandAt returns fields on purpose,
// and it is the caller that has decided not to keep them.
//
// Asked only where the two readings differ, which for this question is not
// the guard listCouldJoinDifferently uses. The join happens either way here
// and only its character is in doubt, so an IFS that is set and *empty* is a
// live answer rather than a reason to stay quiet — joining with nothing is
// what zsh does, and `IFS=""; a=(x y); v=$a` is `xy` there against `x y` in
// bash. What does make the readings coincide is a first character that is
// already a space, and a list too short to use a separator at all.
func (r *Runner) unsplitJoinSeparator(star bool, n int) string {
	sep := ifsFirst(r.ifs())
	if n < 2 || sep == " " {
		// No separator is used, or IFS already begins with the space the
		// other reading would have supplied. Either way there is nothing to
		// ask about, and asking would make every `v=${a[@]}` under the
		// default IFS demand a dialect.
		return " "
	}
	if star {
		return sep
	}
	if r.ask(r.sem().UnsplitAtListJoinsOnIFS,
		"an unquoted `@` list joining on IFS where nothing is split") {
		return sep
	}
	return " "
}

// starSpelled reports whether a node is the `*` half of the pair — the half
// whose separator needs no dialect.
//
// A range subscript joins on the same side of the line as the *name* it was
// written on, which is the rule subscriptJoinsElements already carries for the
// quoted spelling. Asking it here rather than restating it is what keeps the
// two from drifting.
func (r *Runner) starSpelled(e *syntax.ParamExpr) bool {
	return e != nil && (e.Name == "*" || r.subscriptJoinsElements(e))
}

// joinUnsplit is unsplitJoinSeparator applied, for the callers that hold both
// the node and the fields.
func (r *Runner) joinUnsplit(e *syntax.ParamExpr, parts []string) string {
	return strings.Join(parts, r.unsplitJoinSeparator(r.starSpelled(e), len(parts)))
}

// listCouldJoinDifferently is whether joining the elements could reach a
// different set of fields from taking them one at a time.
//
// It cannot when there is nothing to join — one element is its own join — and
// it cannot when there is nothing to join *with*: an IFS that is set and empty
// has no first character, and no shell in the panel joins there, so
// `IFS=""; set -- x y` is two fields for `$@` and for `$*` alike.
//
// Past those, the join changes nothing unless some element is empty or carries
// a separator of its own. Joining n elements that hold no separator puts one
// between each pair and splitting takes them straight back out, so the answer
// is the elements either way — which is what keeps a plain `for f in $@` from
// demanding a dialect.
func (r *Runner) listCouldJoinDifferently(elems []string) bool {
	ifs, set := r.ifs()
	if len(elems) < 2 || ifsFirst(ifs, set) == "" {
		return false
	}
	for _, el := range elems {
		if el == "" || containsAnyOf(el, ifs) {
			return true
		}
	}
	return false
}

// splitEachElement is the reading that takes the elements one at a time.
//
// The other reading joins them first, and elementFields above is what chooses
// between the two.
func (r *Runner) splitEachElement(elems []string, sp splitPolicy, glob Answer) []string {
	ifs, set := r.ifs()
	split := sp.answer(r.sem().SplitParamExpansion)
	var out []string
	for _, el := range elems {
		if el == "" && r.expandingNestedInner {
			// Unless the fields are an inner's, where the element is a value
			// the operator around it is about to read rather than a word the
			// command line is about to lose. See Runner.expandingNestedInner.
			out = append(out, r.escapeResult(el, glob))
			continue
		}
		if el == "" {
			// An unquoted empty element is no field *in this reading*, which
			// under a whitespace IFS is unanimous and has nothing to do with
			// splitting: zsh drops it with its splitting turned off exactly
			// as bash drops it with splitting on.
			//
			// Under a non-whitespace IFS it is not unanimous, and the
			// disagreement is not about the element at all — it is about
			// whether the list was joined before it got here. bash joins, so
			// the empty element is a separator meeting a separator and the
			// field between them survives; zsh, ksh93 and dash do not join,
			// and here it is. UnquotedListJoinsOnIFS is that question, and
			// elementFields asks it.
			continue
		}
		doSplit := false
		if containsAnyOf(el, ifs) {
			doSplit = r.ask(split, "splitting an unquoted parameter expansion")
		}
		// Before the split rather than after, as the scalar path does it:
		// what the escape adds is backslashes, which no IFS puts a field
		// boundary on.
		el = r.escapeResult(el, glob)
		if doSplit {
			out = append(out, r.splitFieldsAsk(el, ifs, set)...)
			continue
		}
		out = append(out, el)
	}
	return out
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
// head says nothing has been accumulated in front of this span in the word
// being built, which is what the `${~spec}` flag's tilde half asks about: a
// substituted tilde expands where a written one would, and a written one
// expands only at the head of a word.
func (r *Runner) expandSpan(s syntax.Span, sp splitPolicy, head bool) (text string, split bool) {
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
		// An unbraced subscript the run does not read as one leaves the
		// parameter behind and hands the brackets back as text. See
		// baresubscript.go.
		s, tail := r.unreadBareSubscript(s)
		v := r.expandParam(s.Param)
		text, split := r.expansionResult(v, unquoted, r.globSubstAnswer(s),
			splitFlagAnswer(s, sp, r.sem().SplitParamExpansion),
			"splitting an unquoted parameter expansion")
		// Before the split, which is measured: `${~v}` on `~/zz ~/qq` is the
		// head expanded and the second tilde left alone, so the value's head
		// is what the flag reaches and not each field's.
		text = r.tildeFlagHead(s, head, text)
		if tail != nil {
			t, tailSplit := r.bareSubscriptText(tail, sp)
			text += t
			split = split || tailSplit
		}
		return text, split
	case syntax.CommandSubst:
		v := r.commandSubst(r.ctx, s)
		return r.expansionResult(v, unquoted, r.sem().GlobExpansionResults,
			sp.answer(r.sem().SplitCommandSubstitution), "splitting an unquoted command substitution")
	case syntax.ProcSubstIn, syntax.ProcSubstOut, syntax.ProcSubstFile:
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
		return r.expansionResult(v, unquoted, r.sem().GlobExpansionResults,
			sp.answer(r.sem().SplitParamExpansion), "splitting an unquoted arithmetic expansion")
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
	tree, text, perr := r.arithTreeOver(s.Arith, s.Value)
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
		r.diagf("%s\n", r.arithFailure(text, err))
		// The command must not run: `echo $((1/0))` fails in every shell
		// in the panel rather than echoing an empty string.
		r.expandErr = true
		return "", false
	}
	// An output format is at the top of the tree rather than anywhere the
	// evaluation could have left it behind, so the result is written from the
	// node: see interp/arithoutput.go.
	format, _ := tree.(*syntax.ArithOutput)
	return r.formatUnder(format, v), true
}

// expansionResult applies the two axes that govern what happens to the result
// of an expansion: whether it is field-split, and whether its metacharacters
// stay live for pathname expansion.
//
// Quoted, neither applies — that is universal. Unquoted, both are dialect
// questions, and zsh answers no to both while everything else answers yes.
func (r *Runner) expansionResult(v string, unquoted bool, glob, split Answer, axis string) (string, bool) {
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
	return r.escapeResult(v, glob), doSplit
}

// escapeResult puts one unquoted expansion result into the escaped form a
// field is carried in.
//
// One function rather than one per caller: the scalar path and the list path
// asked this same question with the same four dialect flags, and the second
// copy is what would have kept #1222 alive for `${a[@]}` after the first was
// fixed.
func (r *Runner) escapeResult(v string, glob Answer) string {
	// The value's own backslashes are marked whatever the answer below is:
	// they are not metacharacters, and no dialect disagrees about them.
	//
	// The question below is then asked of the marked form rather than of the
	// value, which reads consistently and is not a behavior choice — the two
	// are the same predicate. Marking turns each `\c` of the value into
	// `\\` plus `\c`, so the scan consumes exactly the characters it
	// consumed before and every other byte is unchanged and in order.
	// Checked by enumeration over four hundred thousand random strings on
	// the metacharacter alphabet, in all four flag combinations.
	esc := escapeValueBackslashes(v)
	// Asked of the reading that leaves the *most* live, because the mark a
	// value's backslash leaves has not chosen one yet and this question comes
	// first: a field the data reading would glob is one the dialect must be
	// asked about, even where two of the three readings would have left it
	// alone. Answering no here escapes the whole value and the mark never
	// reaches a field, which is what keeps the shell that globs no expansion
	// result from being asked the backslash question at all.
	if r.resultReadsAsPattern(rewriteValueBackslashes(esc, ValueBackslashIsData)) &&
		!r.ask(glob, "globbing the result of an expansion") {
		// zsh does not treat the result of an expansion as a pattern. The
		// same rule decides `[[ abc == $p ]]`, which is one behavior
		// observed twice rather than two quirks.
		return globEscape(v)
	}
	return esc
}

// resultReadsAsPattern reports whether leaving this result live would let
// something downstream read it as a pattern.
//
// It is not the same question as hasUnescapedMeta, and #1386 is the gap
// between them. An unterminated `[` is deliberately *not* a metacharacter —
// `[ a = a ]` runs the test builtin because of that — but the dialect that
// calls one a bad pattern refuses the field in glob before anything asks
// whether it is a pattern at all. So a value holding `a[1m` had no
// metacharacter to protect, went to the filesystem live, and was rejected
// there; and since `ESC [` opens every ANSI escape sequence, that made any
// unquoted expansion of a value carrying color fatal.
//
// Asking it here rather than relaxing the refusal in glob is what keeps the
// two provenances apart. A literal `print -r -- a[1m` is fatal in that
// dialect and is measured correct, and by the time glob has the field the
// escaping is the only thing that still says where the bracket came from —
// so a fix in glob could only have been one that lost the literal case.
func (r *Runner) resultReadsAsPattern(esc string) bool {
	if hasUnescapedMeta(esc, r.dialect().NumericRangePattern, r.dialect().PatternAlternation,
		r.dialect().ExtendedPattern, r.MatchOption(ExtendedPatternOperators)) {
		return true
	}
	// The second gap of the same shape, and #1331 is the one that opened it.
	// A `|` on its own is not a metacharacter — hasUnescapedMeta does not
	// count one and must not, since a field holding nothing else is not a
	// pattern and is not sent to the filesystem. It becomes one only inside
	// a group, and a group is something the *rest of the word* can supply:
	// once an expansion inside `( … )` is read as an expansion rather than
	// as its own source text, the `|` in its value lands between two
	// alternatives that nobody wrote.
	//
	// Measured on zsh 5.9.2, 2026-09-08, in a directory holding `ice.zsh`,
	// `other.zsh` and one file literally named `ice|x.zsh`:
	//
	//	L="ice|other"; print -r -- ($L).zsh   no matches found: (ice|other).zsh
	//	L="ice|x";     print -r -- ($L).zsh   ice|x.zsh
	//
	// The second row is the discriminating one: the `|` is a character the
	// name has to contain, not a choice between two names. Both follow from
	// GlobExpansionResults, which is why this only reports that the axis is
	// worth asking — the dialects that answer yes still read the `|`.
	//
	// Asked only where the dialect has somewhere for a `|` to mean
	// something. Where there are no groups at all it is text however it
	// arrived, and asking would refuse a field with nothing wrong with it.
	//
	// Dropping that guard survives the suite, and it is an equivalent mutant
	// on the panel rather than a gap — recorded so the next reader does not
	// go looking for the row that would kill it. The only dialect that
	// answers No to the axis is also the only one with bare groups, so every
	// dialect the guard excludes answers Yes and hands back the same live
	// text either way. What it changes is which fields *ask*, and that is
	// only observable in a core with the axis unset, where asking refuses a
	// field holding a `|` and nothing else.
	if (r.dialect().PatternAlternation || r.dialect().ExtendedPattern) && hasUnescapedByte(esc, '|') {
		return true
	}
	// Composed with the axis rather than short-circuiting it: a dialect that
	// both globs expansion results and calls an unterminated bracket a bad
	// pattern would be right to refuse this field, since there the result
	// really is a pattern. Only the dialect that answers No to the axis
	// reaches the escape below.
	return r.sem().UnterminatedBracket == BracketBadPattern && hasUnterminatedBracket(esc)
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
		// The outermost word, because that dialect names the word the
		// command line holds however deeply the failure was nested:
		// `"${u:-${x@Z}}"` and `"${v#x${x@Z}y}"` are each blamed entire,
		// measured on ksh93 2026-09-12.
		text = syntax.PrintWord(r.expandingOuterWord)
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
	if e.Index != nil && r.wholeArrayIndex(e) {
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
	if r.refuseAbsentParameter(e) {
		// A parameter a module of this shell's names and this shell has not
		// got. Refused by name rather than expanded to nothing, and refused
		// *here* rather than where a value would be fetched — see
		// absentparam.go for why the fetch is the wrong place.
		return ""
	}
	if e.HasFlags {
		// Normally intercepted in expandAt; reached directly where a single
		// word must result — a pattern operand, say — which is the manual's
		// final rule: the words are rejoined with the first character of
		// IFS.
		words, _, _, ok := r.flaggedWords(e, splitNever, false, nil)
		if !ok {
			return ""
		}
		return strings.Join(words, ifsFirst(r.ifs()))
	}
	// `${+name}` is the is-it-set question, answered before anything reads a
	// value: it takes the same source every conditional expansion takes, and
	// it deliberately does *not* reach the nounset check below — asking
	// whether a name is set without tripping `set -u` is the whole of what
	// the construct is for.
	if setTestAnswers(e) {
		_, set, _ := r.paramSource(e)
		return setTestResult(set)
	}
	// `${#v#a}` is the length of what the operator *leaves* — 2, not 3 —
	// in the one grammar that accepts the pairing at all. The operator was
	// being dropped on the floor: the length block below answered from the
	// untouched value and returned, so a script computing a trimmed length
	// got the untrimmed one and carried on. Nothing said so, and every other
	// shell refuses the expansion outright, so the number was nobody's.
	//
	// Answered by recursion rather than by a second copy of the operator
	// machinery: the same node without its length is exactly the expansion
	// whose result is being measured, so every operator reaches this for the
	// same reason it reaches anything else, and a later fix to one is a fix
	// to both. Measured across every operator this grammar has — the trims,
	// the substring, the replacement, the four conditionals and the element
	// exclusion — and the rule is uniform: apply, then measure.
	//
	// Before any value is read, which is the whole reason it is here and not
	// beside the length block. paramSource expands a nested expansion's
	// words, so intercepting after it would run `${#${v}#a}`'s inner
	// substitution once for the length and once for the operator — and a
	// command substitution in there has side effects that must happen once.
	//
	// The pairing itself is a grammar question and is settled there: the
	// four shells that refuse it never build this node, so reaching here
	// means the dialect accepts it and there is nothing to ask.
	if e.Length && e.Op != syntax.ParamNone {
		inner := *e
		inner.Length = false
		if n, counted := r.operatorResultCount(&inner); counted {
			if r.unspecified {
				return ""
			}
			return itoa(n)
		}
		n := r.stringLength(r.expandParam(&inner))
		if r.unspecified {
			// The same guard the plain length keeps: an unanswered axis
			// underneath has already spoken, and a number on top of it would
			// read as an answer.
			return ""
		}
		return itoa(n)
	}
	// `${#${a[@]}}` is the number of fields the inner came to, not the length
	// of the text they join to — measured, `a=(hello); ${#${a[@]}}` is 1
	// where `s=hello; ${#${s}}` is 5, so one field is not the answer either.
	// The list-ness is the inner's shape, which nestedWords already reports,
	// and the count is taken here rather than after paramSource because that
	// function expands the inner: asking twice would run a command
	// substitution in there twice.
	if e.Length && e.Inner != nil {
		if name, isRef := r.nestedLengthReference(e); isRef {
			// `${#${(P)h}}` is `${#arr}` — the inner is a *name* and not a
			// value, so the length is the one that name answers and not the
			// one its fields join to. Measured on zsh 5.9.2 with `h=arr`:
			//
			//	arr=(a b c);      ${#${(P)h}}   3   the element count
			//	ARR=(x y);        ${#${(UP)h}}  2   the letters rename, so
			//	                                   this measures `ARR`
			//	typeset -A tab=(k1 v1 k2 v2)
			//	nt=tab;           ${#${(P)nt}}  2   an association counts
			//	                                   its pairs
			//	typeset -A one=(a arr); ${#${(P)one[a]}}  3
			//
			// Against 3, 3, 5 and 5 from measuring the joined text, which is
			// a plausible number at status 0 — and the wrong one in the
			// direction a script minds, since `(( ${#${(P)h}} ))` is how a
			// function asks whether the array it was handed the name of has
			// anything in it (#1638).
			//
			// The same split the subscript path already makes: a `(P)` inner
			// is a parameter reference and every other inner is a value. See
			// interp/nestedsub.go.
			// The resolved text may be a *reference* rather than a name —
			// `v='x[@]'` measures the array and not a parameter called
			// `x[@]`, which holds nothing — so the node is built from it.
			// See referenceNode.
			return r.expandParam(r.referenceNode(name, &syntax.ParamExpr{Length: true}, e.Src))
		}
		// The inner is expanded exactly as it would be without the length —
		// the split it is subject to is the one its quoting gives it, and
		// not one a length turns off. See nestedInnerSplit for the
		// measurement that took the claim back out.
		words, _, isList := r.nestedWords(e)
		if isList {
			return itoa(len(words))
		}
		return itoa(r.stringLength(strings.Join(words, "")))
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
	if e.Index != nil && e.Inner == nil {
		// A subscript on a *name*. The same brackets after a nested
		// expansion index what the inner came to and are answered through
		// paramSource below, which is the one route that expands the inner:
		// reaching the name path with no name to read measured `${#${a}[1]}`
		// as 0 rather than as the length of the element.
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
				if r.subscriptYieldsAList(e) && !r.wholeSubscriptMeasuresAScalar(e) {
					return itoa(len(elems))
				}
				return itoa(r.stringLength(strings.Join(elems, "")))
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
		if _, isArr := r.arrayElementCount(e.Name); isArr &&
			r.ask(r.sem().ArrayLengthWithoutSubscriptIsCount, "`${#a}` of an array counting elements") {
			// One dialect counts the elements where the others measure the
			// scalar the bare name yields.
			//
			// Asked for *every* array, one element included, and that is a
			// correction: the reading was skipped at one element on the
			// grounds that such an array is its own element either way,
			// which is true of the value and false of its length. `a=(hello)`
			// is `${#a}` of 1 in the shell that counts and 5 in the shells
			// that measure, and answering 5 for both was a plausible number
			// at status 0 — the failure this codebase minds most. It reached
			// `$functions` with one function defined, where the count is what
			// a script is asking for (#1060).
			n, _ := r.arrayElementCount(e.Name)
			return itoa(n)
		}
		n := r.stringLength(value)
		if r.unspecified {
			return ""
		}
		return itoa(n)
	}

	// The colon extends the test from "unset" to "unset or empty". That one
	// rule is the whole difference between the two rows of conditionals.
	//
	// Read for the four operators that have a test and not before the
	// switch, because a colon-less test on `$@` asks an axis: computing it
	// for every expansion would put that question in front of `${@}` and
	// `${@%x}`, which no shell disagrees about. See conditionalFires.
	fires := false
	switch e.Op {
	case syntax.ParamDefault, syntax.ParamAssign, syntax.ParamAlternate, syntax.ParamError:
		fires = r.conditionalFires(e, value, set)
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
			v := r.substitutedWordText(e.Arg)
			// The side effect that outlives the expansion — and with a
			// subscript it belongs to the *element*. Assigning to the name
			// would replace the whole array with one string, which is worse
			// than the nothing this used to do.
			if subscript {
				r.assignSubscript(e, v)
				return v
			}
			if !r.assignableTarget(e.Op, e.Name) {
				return ""
			}
			r.storeThroughExpansion(e.Name, v)
			return v
		}
		return value
	case syntax.ParamAssignAlways:
		// No test at all, which is the whole of what makes this a different
		// operator: `fires` above is not consulted and there is no
		// non-firing side to return `value` on.
		return r.assignAlways(e, subscript)
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
			r.fatalParamError("%s\n", Wording(r.diag().ParamErrorMessage, "%[1]s: %[2]s",
				e.Name, r.paramErrorWord(e, set)))
			return ""
		}
		return value

	case syntax.ParamTrimPrefix, syntax.ParamTrimPrefixLong,
		syntax.ParamTrimSuffix, syntax.ParamTrimSuffixLong:
		return r.trimWith(value, r.patternOf(e.Arg), e)

	case syntax.ParamReplace:
		return r.replaceWith(value, r.patternOf(e.Arg), e)

	case syntax.ParamSubstring:
		return r.substringRange(value, e)

	case syntax.ParamExclude, syntax.ParamSetDifference, syntax.ParamSetIntersection,
		syntax.ParamElementReplace:
		return r.reshapeScalar(e, value)

	case syntax.ParamUpper, syntax.ParamLower, syntax.ParamToggle,
		syntax.ParamUpperFirst, syntax.ParamLowerFirst, syntax.ParamToggleFirst:
		return r.changeCase(value, e)

	case syntax.ParamTransform:
		return r.transformParam(e, name, value, set)
	}
	// Anything else is left empty rather than guessed at.
	return ""
}

// assignAlways is `${name::=word}`: the word is expanded, stored, and
// substituted, with no test on what the parameter held.
//
// The three steps are in the order the shell that has the construct runs
// them, which is measured rather than assumed. `${#::=$(echo RAN >&2)}` on
// zsh 5.9.2 writes RAN and *then* refuses the name, so the word is expanded
// before the target is checked and a command substitution in it runs even on
// the failing line. Checking first would have been the tidier code and the
// wrong side effect.
func (r *Runner) assignAlways(e *syntax.ParamExpr, subscript bool) string {
	v := r.substitutedWordText(e.Arg)
	if subscript {
		r.assignSubscript(e, v)
		return v
	}
	if !r.assignableParamName(e.Name) {
		return ""
	}
	r.storeThroughExpansion(e.Name, v)
	return v
}

// storeThroughExpansion puts the value an assigning expansion produced where
// its name points.
//
// A run of digits names a *positional parameter*, not a variable spelled with
// digits, and the two are only the same thing until something reads `$#`.
// Storing through setVar left `$#` where it was and put the value in a
// variable that an out-of-range `$1` happened to shadow, so `set --;
// ${1:=new}` read back `new` at `$#` of 0 — and where the positional was
// really there, `set -- p q; ${1::=new}`, the parameter kept `p` while the
// expansion had already substituted `new`. One store, two answers (#1389).
//
// Measured on zsh 5.9.2, the only shell in the panel that assigns a
// positional through an expansion at all — the other five refuse the name,
// which is Semantics.AssignThroughExpansionMayNameAPositional and is asked
// before this is reached:
//
//	set --;      ${1:=new}    $1 is new and $# is 1
//	set --;      ${3:=new}    $3 is new, $1 and $2 are empty, and $# is 3
//	set -- p q;  ${1::=new}   $@ is `new q` and $# is still 2
//	set -- p q;  ${0::=new}   $0 is new and $# is still 2
//
// So a position past the end *widens* the list with empty parameters rather
// than being dropped, which is the half that moves `$#`; and `0` is a
// position like any other to the assignment, though it is the shell's name
// rather than a member of the list and leaves `$#` alone.
func (r *Runner) storeThroughExpansion(name, v string) {
	n, ok := atoi(name)
	if !ok || !isPositional(name) {
		r.setVar(name, v)
		return
	}
	if n == 0 {
		r.Name = v
		return
	}
	for len(r.Params) < n {
		r.Params = append(r.Params, "")
	}
	r.Params[n-1] = v
}

// assignableParamName reports whether an assignment written inside an
// expansion can name this parameter at all, and ends the script when it
// cannot.
//
// A name or a run of digits, and nothing else. Measured 2026-09-07 on zsh
// 5.9.2, which is the only shell in the panel with an operator that assigns
// unconditionally and therefore the only one that reaches this question on
// every parameter:
//
//	${v::=new}   assigns v            a name
//	${1::=new}   assigns $1           a positional
//	${0::=new}   assigns $0           and `0` with it
//	${@::=new}   not an identifier: @
//	${*::=new}   not an identifier: *
//	${#::=new}   not an identifier: #
//	${?::=new}   not an identifier: ?
//	${-::=new}   not an identifier: -
//	${${v}::=x}  not an identifier:    an expansion where the name would be
//
// The last row is why the empty name is refused rather than waved through:
// there is no parameter for the assignment to land on, and the shell says so
// with the name left blank rather than assigning to something invented.
//
// Fatal, and measured fatal: the refusal ends the script at status 1 on all
// three routes — `-c`, a script file and a function body — so it goes through
// fatalExpansion rather than carrying a status of its own.
//
// The wording's fallback is that shell's own, for the reason EqualsNotFound's
// is: a dialect without the grammar never builds a node that reaches here, so
// there is no second answer for the substrate to hold a neutral one against.
// assignableTarget is assignableParamName asked of the names each assigning
// operator actually reaches, and it is one door so the two cannot drift
// apart. They already had: the unconditional `${name::=word}` asked the
// question on both routes through the expander and the conditional
// `${name:=word}` asked it on neither, so `${(U):=abc}` answered `ABC` at
// status 0 where the shell refuses.
//
// **Both operators now ask it of every name**, which is #1541. The
// conditional one used to ask only about the *empty* name (#1529) and called
// setVar on whatever else it was given, so `set --; printf "<%s>" ${@:=abc}`
// substituted `abc` at status 0 in every dialect where all six columns refuse
// fatally — the failure shape this codebase minds most, a plausible value
// where the shell stopped, and a later read of the parameter finds nothing
// behind the word that was substituted.
//
// Two things had to exist before it could be widened, and now do:
//
//   - Four wordings, since `${name:=word}` is in every dialect where
//     `${name::=word}` is in one. See Diagnostics.AssignThroughExpansionBadName.
//   - A positional axis. zsh assigns `${1:=abc}` and the other five refuse it,
//     which is not a wording swap — see
//     Semantics.AssignThroughExpansionMayNameAPositional. The unconditional
//     operator does not ask it: `${1::=new}` assigns in the one shell that can
//     write it, so there is no second answer to hold.
//
// dash's status is not a third thing. It exits 2 where the others exit 1,
// which is Semantics.FatalErrorStatusIsOne — the axis this failure already
// goes through, since it is fatalExpansion's.
//
// It is a run-time check and it fires only when the operator does:
// `set -- p; ${@:=abc}` is `p` at status 0 in all six and never reaches here,
// and `if false; then echo ${@:=abc}; fi` is silent in all six.
func (r *Runner) assignableTarget(op syntax.ParamOp, name string) bool {
	if op != syntax.ParamAssignAlways && isPositional(name) &&
		!r.ask(r.sem().AssignThroughExpansionMayNameAPositional,
			"`${1:=word}` assigning to a positional parameter") {
		if r.unspecified {
			// An axis no dialect answered, already reported where it was
			// asked. A refusal on top of it would be a second complaint
			// about one line, and the dialect's wording is the thing a run
			// with no dialect has not got.
			return false
		}
		// The one name the two operators part company over, and the one the
		// panel parts company over. Refused in the dialect's own words,
		// through the same door as every other unassignable name: what makes
		// a parameter unassignable does not change how the shell says so.
		return r.refuseAssignableName(name)
	}
	return r.assignableParamName(name)
}

func (r *Runner) assignableParamName(name string) bool {
	if isNameLike(name) || isPositional(name) {
		return true
	}
	return r.refuseAssignableName(name)
}

// refuseAssignableName ends the script over a parameter an assignment written
// inside an expansion cannot land on, and reports it the dialect's way.
//
// The second verb is the whole *word* the expansion stands in, which is
// ksh93's subject and nobody else's: measured, `x${@:=abc}y`, `"${@:=abc}"`
// and `a"${@}"b"${@:=abc}"c` are each blamed entire there. The name is the
// fallback for a refusal reached from something that is not a word — a `case`
// subject read another way, a caller of its own — where there is no word to
// print.
//
// Read here rather than through badSubstitutionSubject, which answers the
// same shape of question for a different sentence. The two coincide for
// ksh93 today; tying them together would make a change to either route a
// change to both, which is the reason this wording has a field of its own.
func (r *Runner) refuseAssignableName(name string) bool {
	written := name
	if w := syntax.PrintWord(r.expandingWord); w != "" {
		written = w
	}
	r.diagf("%s\n", Wording(r.diag().AssignThroughExpansionBadName,
		"not an identifier: %[1]s", name, written))
	// A word that could not be read rather than an expansion that failed,
	// which is the same line reportBadSubstitution draws and is measured on
	// the two routes that tell them apart. bash gives up the *line* and
	// carries on — `printf "<%s>" ${@:=abc}` on line 2 of a script still
	// prints `after` from line 3, at status 0 — and a failed expansion under
	// `-c` there exits 127 where this exits 1. Both come out right by
	// leaving the decision to failedExpansion, which reads
	// Semantics.FailedExpansionAbandonsTheLine; fatalExpansion ended the
	// shell in every dialect and took the 127 with it.
	r.expandErr = true
	return false
}

// assignSubscript is `${a[i]:=v}`, which assigns to the element rather than to
// the array.
//
// Only a numeric subscript: `${a[@]:=v}` is a question about the whole array
// that the panel does not answer alike, and guessing at it would be worse than
// leaving it alone.
func (r *Runner) assignSubscript(e *syntax.ParamExpr, v string) {
	if e.IndexFlags != nil {
		// A flag group names the element, exactly as it does on the left of
		// an ordinary assignment: `${b[(r)y]:=V}` writes where the search
		// found, and `${b[(i)nomatch]:=V}` one past the last. The subscript
		// behind the group is a *pattern* rather than an expression, so the
		// arithmetic reading below cannot be reached with it.
		n, ok := r.flaggedAssignIndex(&syntax.Assign{
			Name: e.Name, Index: e.Subscript(), IndexFlags: e.IndexFlags,
		})
		if !ok {
			return
		}
		r.setArrayElem(e.Name, n,
			subscriptSubject(e.IndexText, r.subscriptAsWritten(e.Subscript())), v)
		return
	}
	idx := r.subscriptText(e.Subscript())
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
	r.setArrayElem(e.Name, n, subscriptSubject(e.IndexText, idx), v)
}

// wholeArraySubscript reports whether a subscript names the whole array rather
// than one element, which is what decides whether `:` slices a list or takes a
// substring of a single value.
// listShapedOp reports whether the array path answers this operator with the
// *elements* when the subject is the whole array — one output per element,
// or a selection among them — rather than with one joined value.
//
// Named rather than written inline because two callers must agree on it: the
// array branch, which uses it to decide whether `${a[@]…}` keeps its fields,
// and bareArrayAsList, which uses it to decide whether rewriting a bare name
// to that subscript would reach the branch at all.
//
// The rewrite is local to expandAt — a node the branch declines is answered
// from the untouched original — so a second, drifting copy of this list would
// not produce a wrong *value*. It would produce a wrong *question*: the axis
// would be asked for an expansion whose answer is then thrown away, and an
// unanswered axis is a diagnostic rather than a shrug, so a core that has
// chosen no shell would refuse `a=(x y); echo ${a:=d}` over a reading it
// never used. Mutation says so — dropping the test here refuses all three of
// `${a:=d}`, `${a:?e}` and `${a=d}`.
//
// ParamNone is not here: the branch takes it under *any* subscript, because
// `${a[0]}` is one element and still comes from this path.
func (r *Runner) listShapedOp(e *syntax.ParamExpr) bool {
	return e.Op == syntax.ParamSubstring || e.Op == syntax.ParamTransform ||
		reshapesElements(e.Op) || zipsElements(e.Op) || elementOp(e.Op) ||
		r.yieldsTheArray(e)
}

// listBase is the values an expansion stands for where it stands for several
// of them, and whether it does.
//
// Two sources reach it and the pipeline above cannot tell them apart, which
// is the point. A subscript on a *name* supplies elements; a nested
// expansion whose inner came to a list supplies fields. Everything the
// pipeline then does — the slice, the element filter, the per-element
// operators, the `[*]` join, the glob-escaping and the splitting — is the
// same question for both, and asking it in one place is what keeps the
// nested spelling from acquiring a second, thinner answer of its own. The
// nested half arrived with none at all: it was refused by name, and the one
// shape that slipped past the refusal — an inner that lost its elements
// before it was counted — was joined into a single plausible field at status
// 0 (#1509).
//
// The operator guard is the array's alone. A nested list takes every
// operator elementwise in the shell with the grammar — measured,
// `${${a[@]}#x}`, `${${a[@]}:1}`, `${${a[@]}:#y}` and `${${a[@]}//y/Q}` all
// distribute — where a subscript naming one element must not: `${a[0]:1}` is
// a substring of that element and slicing the list there would drop it.
//
// *Which* subscripts name several is subscriptYieldsAList's question, asked
// here rather than re-derived. This spelled out `[@]` and a search inline,
// which is that predicate with its range clause struck off, so an operator
// over a range ran on one word made of the elements: measured on
// `a=(xa xb xc)`, `set -- ${a[1,3]#x}` leaves three parameters and
// `set -- ${a[1,3]:#xb}` two, where this reading left one apiece. The
// quoted spellings hid it, because a quoted range joins anyway.
func (r *Runner) listBase(e *syntax.ParamExpr) ([]string, bool) {
	if e.Length {
		// `${#…}` is a number, and which number it is — the element count or
		// the length of a string — is answered where the length is taken.
		return nil, false
	}
	if e.Inner != nil {
		words, _, isList := r.nestedWords(e)
		if !isList {
			// Not a list, so the scalar path answers it — and that path
			// expands the inner too. nestedWords has already put the fields
			// back in the hold for it: `${${v}#a}` with a command
			// substitution inside runs it once, and asking twice ran it
			// twice. See holdNested.
			return nil, false
		}
		return words, true
	}
	if e.Index == nil {
		return nil, false
	}
	if r.wholeSubscriptSlicesAScalar(e) {
		// `${s[@]:1}` on a name holding one string is a slice of that
		// *string*, not of a list of one — so the scalar path answers it,
		// and the offsets count characters there exactly as they do for
		// `${s:1}`. Taking it here counted elements instead: an offset
		// inside the value answered with the whole of it and an offset of 1
		// dropped the only element and answered nothing, both at status 0
		// (#1850).
		return nil, false
	}
	switch {
	case e.Op == syntax.ParamNone:
	case r.listShapedOp(e) && r.subscriptYieldsAList(e):
	default:
		return nil, false
	}
	return r.arraySubscript(e)
}

// positionalsAsList gives `$@` and `$*` under an operator the subscript that
// makes the array path answer them, and reports whether it did.
//
// The operators that take a whole array elementwise take the *positional
// parameters* the same way, and that is unanimous rather than a dialect's
// reading: on `set -- ax bx cx`, `${@#a}` is `x bx cx` as three fields in
// bash 5.3, bash 3.2, dash, ksh93 and zsh alike, and so are `${*#a}`,
// `${@//x/Y}` and `${@:-d}`. Without this the node fell through to the
// scalar path, where `$@` is the parameters *joined*, so every one of those
// came back as a single field with the operator applied once to the join —
// `${@#a}` as `x bx cx` in one word, at status 0. Nothing distinguishes the
// two readings by field count when the operator happens to be a no-op on the
// join, which is why `${@%x}` looked right (`ax bx c`, matching dash) while
// `${@#a}` was wrong in the same run.
//
// A rewrite for the same reason bareArrayAsList is one: `${@[@]#a}` already
// answers this correctly, through the branch that distributes an operator
// over the elements, asks the `[*]` join axis where the readings differ, and
// glob-escapes each field. Giving the bare spelling that node is what keeps
// the two from drifting, rather than growing a second, thinner copy of the
// same pipeline beside it.
//
// The subscript is the name's own: `@` takes `[@]` and `*` takes `[*]`, which
// is what preserves the difference between them — measured, `"${*#a}"` is one
// joined field where `"${@#a}"` is three, exactly as `"${a[*]}"` and
// `"${a[@]}"` differ. Quoting is not consulted here at all, because the array
// path below already reads it off the span.
//
// Two operators stay off this path deliberately:
//
//   - ParamNone, which the `$@` and `$*` branches further down already
//     answer, quoting, escaping and all.
//   - ParamTransform, which has its own measured branch below for exactly
//     these two names.
//
// ParamSubstring is on this path too, and getting it here took one extra
// step. The offset does not count the same thing it counts for a named array:
// `${@:1}` is all three parameters on a list of three — measured in bash 5.3,
// ksh93 and zsh 5.9.2 — where `${a[@]:1}` drops the first element. The reason
// is not that the positional slice is 1-based; it is that the positional list
// has one more element at the front. `$0` is element 0, so `${@:0}` is the
// shell's own name followed by every parameter and `${@:1}` is `$1` onward,
// both of which fall straight out of counting from 0 over `$0 $1 … $n`. See
// positionalSliceElems, which is where that element is put on.
//
// So the subscripted spelling agrees rather than differing: `${@[@]:1:2}` is
// `$1 $2` in zsh, not the array reading that drops one, and it is the same
// list being counted (#1589).
func (r *Runner) positionalsAsList(e *syntax.ParamExpr) (*syntax.ParamExpr, bool) {
	if e.Name != "@" && e.Name != "*" {
		return nil, false
	}
	// The same shapes bareArrayAsList declines, and for the same reasons: a
	// subscript is already the question this answers, and the length, the
	// indirection and the `${!prefix@}` prefix are all read before an
	// operator is.
	if e.Index != nil || e.Length || e.Indirect || e.Prefix != 0 {
		return nil, false
	}
	switch e.Op {
	case syntax.ParamNone, syntax.ParamTransform:
		return nil, false
	}
	if !r.listShapedOp(e) {
		// An operator the array branch answers with one joined value is
		// already right on the scalar path, and rewriting the node would ask
		// the array axes for an answer nothing then uses.
		return nil, false
	}
	listed := *e
	listed.Index = &syntax.Word{Spans: []syntax.Span{{Kind: syntax.Literal, Value: e.Name}}}
	return &listed, true
}

// bareArrayAsList gives a bare array name the `[@]` subscript one dialect
// reads it with, and reports whether it did.
//
// `$a` is the elements there — one field each, a slice slicing the *list* and
// `:#` filtering it — exactly as `${a[@]}` is, where bash and ksh93 read the
// bare name as one element and dash has no arrays to ask about. Answering it
// by rewriting the node is what keeps the two spellings from drifting:
// everything `${a[@]}` already gets right, `$a` gets right for the same
// reason and by the same code, and a later fix to one is a fix to both.
//
// Only where the reading can be seen, which is three conditions:
//
//   - Unquoted. Quoted, both readings are one field holding the joined value
//     — that is ArrayScalarIsTheWholeArray's question and it is already
//     answered — so `"$a"` must stay on the scalar path.
//   - In a context that splits. Measured, an unquoted bare name in a context
//     that does not split is the joined value in zsh too: `IFS=-; v=$a` is
//     `x-y-z`, and so are `[[ $a = x-y-z ]]`, `case $a`, and a here-document
//     body. splitNever is exactly those contexts.
//   - More than one element, or an operator whose answer depends on the
//     subject being a list even at one. A one-element array is that element
//     under either reading and every value-to-value operator agrees on it;
//     `${a:1}` and the element-selecting three do not, because a slice takes
//     elements where a substring takes characters and a filter can drop the
//     only element there is. An empty array is no field under either reading,
//     unless the expansion is distributive, where it is the one shape that
//     tells an empty array from a name that is not one — see the count below.
//
// Every one of those is a guard on the *question*, not on the answer: the
// rewrite is local to expandAt, so a node it declines is answered from the
// untouched original either way. What asking too widely costs is the core —
// an unanswered axis is a diagnostic, so a question asked where the readings
// agree turns `a=(x); echo $a` into a refusal.
func (r *Runner) bareArrayAsList(e *syntax.ParamExpr, s syntax.Span, sp splitPolicy) (*syntax.ParamExpr, bool) {
	// A subscript is already the question this answers. `${#a}` is
	// ArrayLengthWithoutSubscriptIsCount's, and asking this one as well
	// refuses it on a core that has chosen no shell — measured by mutation.
	// The indirection and the prefix are here for a reading rather than for a
	// question: `${!a}` reads the value as a *name*, and the same node
	// subscripted is the array's *subscripts*, which the branch below would
	// duly answer with. No grammar reaches that pairing today — the two that
	// spell `${!…}` at all answer this axis no — so no test can tell the
	// guard from its absence, and it stays because the reading it prevents is
	// a different construct rather than a different field count.
	//
	// A flag group needs no clause: expandFlagged answers every node carrying
	// one and returns before this is reached.
	if e.Index != nil || e.Length || e.Indirect || e.Prefix != 0 {
		return nil, false
	}
	// The subscript the name is read with depends on the quoting, because the
	// two spellings of the list do: unquoted it is `[@]`, one field per
	// element, and quoted it is `[*]`, one field holding their join. That is
	// the same division `${a[@]}` and `${a[*]}` already keep, and giving the
	// bare name both halves is what stops the two from drifting.
	sub := "@"
	switch {
	case zipsElements(e.Op):
		// The elements either way, because the zip's own answer is a list
		// whatever the quoting: `"${a:^b}"` is two fields. What quoting
		// decides is that the *left* operand arrives as one word, and the
		// zip does that itself — taking `[*]` here would join the result
		// instead, which is the wrong end (#2112).
		if s.Quoting != syntax.Unquoted && r.sem().ArrayScalarIsTheWholeArray != Yes {
			return nil, false
		}
	case s.Quoting == syntax.Unquoted:
		// In a context that splits, and under an operator the array branch
		// answers with the elements. Both measured; see the note above.
		if sp == splitNever {
			return nil, false
		}
		if e.Op != syntax.ParamNone && !r.listShapedOp(e) {
			return nil, false
		}
	case opReadsTheList(e.Op):
		// Quoted, only the slice needs this. Every other operator is already
		// right through the scalar path: the quoted value *is* the elements
		// joined, and an operator applied to that one string is exactly the
		// "quotes join first" reading the shell follows — measured on the
		// trims, the replacements, `:-` and the three element-selecting
		// operators, all of which already agree. The slice is the one that
		// does not, because its offset counts *elements* under `[*]` where it
		// counts characters in a string: `a=(one two three); "${a:1}"` is
		// `two three` where the name is the list and `ne` where it is the
		// joined value, and on a one-element array it is nothing at all
		// against `olo`. opReadsTheList is that predicate, already named for
		// the unquoted side of the same question.
		sub = "*"
		// Read rather than asked. This is not a new question — it is
		// ArrayScalarIsTheWholeArray, which the scalar path below asks and
		// diagnoses for exactly this node. Asking it here as well would
		// print the refusal twice on a core that has chosen no shell, so a
		// No or an unanswered axis simply declines and lets the one place
		// that already owns the question do the talking.
		if r.sem().ArrayScalarIsTheWholeArray != Yes {
			return nil, false
		}
	default:
		return nil, false
	}
	// arrayElementCount reports zero for a name that is not an array at all,
	// and says which of the two the zero is. Ordinarily the count is the
	// whole test — a scalar and an empty array both stop here, and neither
	// has a reading the two answers differ on — and a distributive expansion
	// is the exception, because it is the one reading under which an empty
	// list is not the same as an empty value. Measured: `a=(); x${^a}y` is
	// no word at all, where `x${^u}y` on a name that was never an array is
	// the single word `xy`. The list path is what can say "no fields"; the
	// scalar path below has only the empty string to say it with.
	n, isArray := r.arrayElementCount(e.Name)
	switch {
	case zipsElements(e.Op):
		// A zip takes whatever the name holds, including nothing and
		// including a scalar: measured, `s=one; b=(x y); ${s:^b}` is
		// `one x`, so a scalar is the one-element list it already is, and
		// an empty array is no elements unquoted and one empty word quoted.
		// Neither reading is available from the scalar path, which has one
		// string to answer with.
		//
		// A name holding *nothing at all* is the exception and is the
		// scalar path's: measured, `a=(); "${a:^b}"` is the two fields ``
		// and `x` where `unset a; "${a:^b}"` is no field. Set-and-empty is
		// one empty word; never-set is not a word.
		if _, held := r.arrayElems(e.Name); !held {
			return nil, false
		}
	case n == 0:
		if !isArray || !rcExpandOn(s) {
			return nil, false
		}
	case n == 1 && !opReadsTheList(e.Op):
		return nil, false
	}
	// Only the unquoted spelling asks this. The quoted one is
	// ArrayScalarIsTheWholeArray's question and the branch above has already
	// read it — asking the field-count axis as well would demand two answers
	// for one reading, and on a core that has chosen no shell it refused
	// `"${a:1}"` over an axis whose answer it never used.
	if sub == "@" && !r.ask(r.sem().ArrayNameWithoutSubscriptIsTheList,
		"a bare array name being its elements") {
		return nil, false
	}
	listed := *e
	listed.Index = &syntax.Word{Spans: []syntax.Span{{Kind: syntax.Literal, Value: sub}}}
	return &listed, true
}

// opReadsTheList reports whether an operator still tells the two readings
// apart when the array holds exactly one element.
//
// The slice alone does: `a=(abcdef); ${a:1}` is nothing at all where the name
// is the list — one element with the first dropped — and `bcdef` where it is
// that element's characters. The offset counts elements in one reading and
// characters in the other, and one element is enough for those to diverge.
//
// The three element-selecting operators look like they belong here and do
// not, which mutation is what settled: `${a:#p}` on a one-element list keeps
// that element or drops it, and on the scalar it is the same element tested
// against the same pattern, so the two readings coincide — and `:|` and `:*`
// coincide for the same reason. Every other operator maps a value to a value
// and gives the same answer whichever way the single element was reached.
func opReadsTheList(op syntax.ParamOp) bool {
	// The zip joins with the slice rather than with the other operators: a
	// quoted `"${a:^b}"` is still two fields, so the scalar path's one string
	// cannot answer it. What quoting decides here is only that the *left*
	// operand arrives as one word, which is why `"${a:^b}"` on `(1 2 3)` and
	// `(x y z)` is `1 2 3` and `x` (#2112).
	return op == syntax.ParamSubstring || zipsElements(op)
}

func wholeArraySubscript(idx string) bool { return idx == "@" || idx == "*" }

// wholeArrayIndex is the same question asked of a node, which is where a flag
// group can be seen.
//
// A group takes the two whole-array spellings away: measured,
// `${a[()@]}` and `${a[(e)*]}` are `bad math expression: operand expected`
// in the grammar that has groups, where `${a[@]}` is the array. So `@` and
// `*` are the whole array only in a subscript nothing opened, and every
// reading that asks has to ask about the node rather than about the text —
// asking about the text is how `${a[(r)@]}` came to be shaped like a list
// while answering with one element.
// A **quoted** `@` takes them away as well, and that is measured rather than
// reasoned: `${n["@"]}` on an associative array looks up a key and finds
// nothing in bash, ksh93 and zsh alike, and on an indexed one it is an
// arithmetic error in all three, because `@` is no number. So the two
// spellings are read off the subscript *as written* — searchOperand's text,
// with substitutions performed and quotes kept — and never off the key quote
// removal produced, which is `@` under one of the two readings the axis
// offers and would make the whole array of a lookup.
func (r *Runner) wholeArrayIndex(e *syntax.ParamExpr) bool {
	return e.IndexFlags == nil && wholeArraySubscript(r.subscriptAsWritten(e.Subscript()))
}

// joinedArrayIndex is `[*]`, the spelling that joins, asked the same way.
func (r *Runner) joinedArrayIndex(e *syntax.ParamExpr) bool {
	return e.IndexFlags == nil && r.subscriptAsWritten(e.Subscript()) == "*"
}

// atArrayIndex is `[@]`, the spelling that keeps its fields, asked the same
// way.
func (r *Runner) atArrayIndex(e *syntax.ParamExpr) bool {
	return e.IndexFlags == nil && r.subscriptAsWritten(e.Subscript()) == "@"
}

// subscriptAsWritten is the subscript's text with its substitutions performed
// and nothing else touched, trimmed of the blanks a whole-array spelling never
// has anyway. It is searchOperand under a name that says what the
// whole-array readings want of it.
func (r *Runner) subscriptAsWritten(w *syntax.Word) string {
	return strings.TrimSpace(r.searchOperand(w))
}

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
		return func(v string) string { return r.trimWith(v, pattern, e) }
	case syntax.ParamReplace:
		pattern := r.patternOf(e.Arg)
		return func(v string) string { return r.replaceWith(v, pattern, e) }
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
	if a, ok := r.assocFor(name); ok {
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
// positionalSliceElems puts `$0` on the front of the positional parameters, so
// that a slice counts over `$0 $1 … $n` and lands where every shell with the
// construct lands.
//
// Only for a slice, and only for the positional list. It is what makes
// `${@:1}` all of the parameters where `${a[@]:1}` drops the first element —
// one list is longer at the front, rather than one offset being 1-based. And
// it is why `${@:0}` names the shell: measured on `set -- ax bx cx`,
// `"${@:0}"` is four fields beginning with the shell's own path in bash 5.3
// and zsh 5.9.2 both, where `"${@:1}"` is three.
//
// No other operator wants it. `${@#a}` distributes over the parameters and
// `$0` is not one of them — measured, it is three fields on a list of three —
// so this stands at the slice and not where the elements are fetched.
//
// A negative offset needs no special case: `${@: -1}` is the last parameter
// either way, because the extra element is at the front and counting back from
// the end never reaches it.
func (r *Runner) positionalSliceElems(e *syntax.ParamExpr, elems []string) []string {
	if e.Name != "@" && e.Name != "*" {
		return elems
	}
	// Through specialParam rather than r.Name, because `$0` is not always the
	// shell: one dialect answers with the function or sourced file it is
	// inside (Semantics.DollarZeroNamesTheInnermostCall), and a slice that
	// reached past its own `$0` for a different answer than `$0` gives would
	// be two readings of one parameter.
	zero, _ := r.specialParam(&syntax.ParamExpr{Name: "0"})
	return append([]string{zero}, elems...)
}

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
	if n < 0 && off < len(elems) {
		// The subject decides, not the sign: the same `-1` is a valid
		// length for a *string* in bash and a refusal for a list, in one
		// shell (#1735). Asked only where there is something to slice —
		// bash's `${a[@]:3:-1}` on three elements is empty at status 0, and
		// so is the same slice of an empty array, so an offset at or past
		// the end is answered before the length is looked at.
		if r.ask(r.sem().ListSliceNegativeLengthIsAnError, "a negative length refusing a list slice") {
			// The length as *written*, which is what the one shell that
			// refuses blames: `${a[@]:1:1-$n}` names `1-$n` and not the -2
			// it came to.
			r.diagf("%s\n", Wording(r.diag().ListSliceNegativeLength,
				"%[1]s: substring expression < 0", syntax.PrintWord(lenWord)))
			r.expandErr = true
			return nil
		}
		if r.unspecified {
			return nil
		}
		// And where it is not refused the two spellings still part: one
		// dialect answers a negative length with nothing at all and the
		// others count it from the end, which is the axis the string form
		// beside this one already asks.
		if r.ask(r.sem().SubstringNegativeLengthIsEmpty, "a negative substring length") {
			return nil
		}
	}
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
	// An arithmetic expression is not a pathname, so the operand is expanded
	// without matching anything: `${x:(i):2}` is an offset of `i`, and its
	// parentheses are the expression's grouping rather than a pattern group
	// that the dialect with glob qualifiers would read as a list.
	restore := r.withoutGlobbing()
	text := strings.TrimSpace(r.joinWord(w))
	restore()
	// The range's own reader, not the subscript's: an offset that expanded
	// to nothing is zero in every column, where the same emptiness in a
	// subscript is refused in one of them.
	n, err := r.expressionValue(text)
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
	if e.Src != "" {
		// A nested expansion has no name to be the subject, and the text it
		// was written as is the only thing a reader can find in the script:
		// a refusal about `${${(P)h}[(x)y]}` names that rather than the
		// `arr[(x)y]` the inner resolved to. Src is set for exactly the
		// nodes with nothing else to name — a nesting, and an expansion the
		// grammar could not read.
		return e.Src
	}
	if e.Index == nil {
		return e.Name
	}
	// The subscript as it was *written*, flag group included: a diagnostic
	// about `${a[(re)x]}` that named `a[x]` would name a subscript the
	// script does not contain. Every subscript of a chain, for the same
	// reason: a refusal about `${m[k][2]}` that named `m[2]` would name a
	// key the table has never held.
	sub := e.Name
	for _, lead := range e.Leading {
		sub += "[" + r.subscriptText(lead.Index) + "]"
	}
	return sub + "[" + r.subscriptText(e.Index) + "]"
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
	// In the C locale only ASCII letters are letters, so `${x^^}` on café is
	// CAFé. What an *unset* locale is splits the panel and is the dialect's
	// answer; caseMapper asks, and is the one place that narrowing lives.
	convert = r.caseMapper(value, convert)
	first := e.Op == syntax.ParamUpperFirst ||
		e.Op == syntax.ParamLowerFirst ||
		e.Op == syntax.ParamToggleFirst

	o := r.patternOpts(pattern, value)
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

// toggleCase swaps a letter's case and leaves anything else alone.
func toggleCase(c rune) rune {
	if unicode.IsUpper(c) {
		return unicode.ToLower(c)
	}
	return unicode.ToUpper(c)
}

func (r *Runner) trimWith(value, pattern string, e *syntax.ParamExpr) string {
	out, m := trim(value, pattern, e.Op, r.patternOpts(pattern, value),
		r.armOrder(), searchingFlag(e))
	r.publishMatch(m)
	return out
}

// armOrder is the axis a longest prefix trim asks when the arms of an
// alternation take different lengths — see interp/trimarm.go — with the
// question left in a closure so that it is asked only at the disagreement.
func (r *Runner) armOrder() armOrder {
	return armOrder{
		answer: r.sem().LongestMatchTakesTheWrittenArm,
		ask: func() bool {
			return r.ask(r.sem().LongestMatchTakesTheWrittenArm,
				"`${x##pat}` and `${x//pat/rep}`, where the arms of an "+
					"alternation take different lengths")
		},
	}
}

// matchedWith is trimWith with the flag that keeps what the pattern took.
func (r *Runner) matchedWith(value, pattern string, e *syntax.ParamExpr) string {
	out, m := matched(value, pattern, e.Op, r.patternOpts(pattern),
		r.armOrder(), searchingFlag(e))
	r.publishMatch(m)
	return out
}

// replaceWith substitutes a matching span, expanding the replacement text
// once per match where — and only where — the pattern reports something.
//
// Both halves are measured, and the difference is visible rather than a
// saving. A pattern that reports fills `$match` or `$MATCH` before each
// replacement, so each one has to be expanded again to read them:
// `x=abcd; ${x//(#b)(b)(c)/[$match[1]]}` is `a[b]d` and
// `${x//(#m)[bc]/<$MATCH:$MBEGIN>}` is `a<b:2><c:3>d`. A pattern that
// reports nothing leaves the replacement the same text every time, and the
// shell expands it **once**: `x=aaa; ${x//a/$RANDOM}` repeats one number
// three times and `i=0; ${x//a/$((++i))}` is `111`, where the same two with
// a `(#b)` in front are three different numbers and `123`.
//
// So this is not an optimization with a behavior consequence, it is the
// behavior. Expanding unconditionally per match made `${x//a/$((++i))}`
// count, and cost four times the run time of a substitution over a long
// value for the trouble.
func (r *Runner) replaceWith(value, pattern string, e *syntax.ParamExpr) string {
	if pattern == "" && e.Anchor == 0 && !r.emptyPatternFires(value) {
		return value
	}
	o := r.replacementPatternOpts(pattern, value)
	// One spelling of "read the replacement", used by both branches. It is
	// `replacementOf` and not `joinWord` because a replacement is **text**
	// and not a pattern (#1337), and having the two branches read it two
	// ways is exactly how that fix would come undone in the branch nobody
	// looks at.
	repl := r.replacementWord(e)
	if !reportsAMatch(o) {
		with := r.replacementFor(repl)
		return replace(value, pattern, e, o, r.armOrder(), r.emptyMatchDeclined,
			func(_ matchReport, matched string) string {
				return with(matched)
			})
	}
	return replace(value, pattern, e, o, r.armOrder(), r.emptyMatchDeclined, func(m matchReport, matched string) string {
		r.publishMatch(m)
		return r.replacementFor(repl)(matched)
	})
}

// emptyPatternFires is whether an unanchored span replacement whose pattern
// is empty gets as far as matching at all.
//
// The three readings are Semantics.EmptyReplacementPattern, and this is the
// only place the axis is asked: a pattern with a byte in it never reaches
// here, so the common path answers no question. Where it says yes, the
// ordinary matcher takes over and the empty pattern behaves as any other
// pattern that matches the empty string — including the end-of-value rule,
// which is why `${v///X}` on `abc` is `XaXbXc` and not `XaXbXcX`.
func (r *Runner) emptyPatternFires(value string) bool {
	switch r.emptyReplacementPattern() {
	case EmptyReplacementPatternMatchesEveryPosition:
		return true
	case EmptyReplacementPatternMatchesAnEmptyValue:
		// The one position such a value has is also its end, and a value
		// with no units has no preceding step to have consumed it.
		return value == ""
	}
	// Nothing, and an unanswered axis, which is reported and then declines.
	return false
}

// replacementPatternOpts is patternOpts for the pattern of a **span
// replacement** — `${v/pat/rep}`, its global and anchored spellings, and the
// deleting form with no replacement.
//
// One field differs, the case fold, and the split is measured rather than
// reasoned: 2026-09-11 on bash 5.3.15 with `v=ABC`, after `shopt -s
// nocasematch`,
//
//	${v//b/X}   AXC      ${v#a}    ABC
//	${v/#a/Y}   YBC      ${v%c}    ABC
//	${v/%c/Z}   ABZ      ${v^^b}   ABC
//
// So it is not "parameter expansion is exempt from the fold". The trims and
// the case-change operator really are exempt and the substitution is not, and
// a probe that used `${x#a}` alone could not tell the two apart — which is
// what the comment on MatchFoldsCase was written from (#1969).
//
// Symmetric, and the whole matcher rather than a prefix test: with the option
// on, `v=abc; ${v//B/X}` is `aXc`, and so are `${v//[B]/X}` and `${v//b?/X}`,
// so what folds is the comparison inside the pattern and not the pattern's
// text. `nocaseglob` reaches none of this — measured, `shopt -s nocaseglob`
// leaves `${v//b/X}` at `ABC` — which is why the fold read here is the
// matching option and not the globbing one.
//
// It is not an axis. Only one shell in the panel has an option that turns
// MatchFoldsCase on at all, so no second dialect can disagree about where its
// answer reaches; a Semantics field here would have exactly one shell able to
// answer it.
func (r *Runner) replacementPatternOpts(pattern string, subjects ...string) patternOpts {
	o := r.patternOpts(pattern, subjects...)
	o.fold = r.MatchOption(MatchFoldsCase)
	return o
}

// reportsAMatch is whether a pattern fills `$MATCH` or `$match` when it
// matches, read off options that were built for it.
//
// Named once because two replacements turn on it — the span one above and the
// whole-element one in elementreplace.go — and it is the whole of what
// decides that a replacement is read again for each match rather than once.
func reportsAMatch(o patternOpts) bool {
	return o.where != nil && o.where.plan.reports()
}

// patternReports is reportsAMatch for a caller holding only the pattern text.
func (r *Runner) patternReports(pattern string) bool {
	return reportsAMatch(r.patternOpts(pattern))
}

func trim(value, pattern string, op syntax.ParamOp, o patternOpts, arm armOrder, search bool) (string, matchReport) {
	lo, hi, m, ok := trimSpan(value, pattern, op, o, arm, search)
	if !ok {
		return value, matchReport{}
	}
	// A span touching either end leaves a slice of the value rather than a
	// new string, and every trim without `(S)` on it touches one — a prefix
	// trim's span begins at 0 and a suffix trim's ends at the length. Named
	// rather than left to the general expression, which is not free:
	// concatenating `"" + value[hi:]` copies what the slice would have
	// shared, 13KB a call on the prompt cache that
	// BenchmarkLongestPrefixTrimOnAPromptCache is cut from.
	switch {
	case lo == 0:
		return value[hi:], m
	case hi == len(value):
		return value[:lo], m
	}
	return value[:lo] + value[hi:], m
}

// matched is trim's other half: the part the pattern took rather than the part
// it left, and nothing at all when it took none.
//
// One flag turns a trim into this — the same operator, the same match, the
// other side of the same split — which is why it shares trimSpan rather than
// scanning again. Measured: `${(M)v#h*l}` on `hello` is `hel` where
// `${v#h*l}` is `lo`, and `${(M)v#zzz}` is empty where `${v#zzz}` is `hello`.
//
// The span is what makes the pair hold under `(S)` for nothing: a searching
// trim takes a piece out of the middle, and the two halves of the split are
// still "the value without it" and "it". Measured, `${(SM)str%%X*}` on
// `aXbXc` is `Xc` beside `${(S)str%%X*}`'s `aXb`.
func matched(value, pattern string, op syntax.ParamOp, o patternOpts, arm armOrder, search bool) (string, matchReport) {
	lo, hi, m, ok := trimSpan(value, pattern, op, o, arm, search)
	if !ok {
		return "", matchReport{}
	}
	return value[lo:hi], m
}

func trimsPrefix(op syntax.ParamOp) bool {
	return op == syntax.ParamTrimPrefix || op == syntax.ParamTrimPrefixLong
}

// trimTakesLongest is the doubled spelling of either trim: the operator that
// asks for as much match as it can have where the single one asks for as
// little.
func trimTakesLongest(op syntax.ParamOp) bool {
	return op == syntax.ParamTrimPrefixLong || op == syntax.ParamTrimSuffixLong
}

// trimSpan is the piece of the value a trim's pattern took: where it begins,
// where it ends, and whether the pattern matched at all.
//
// A span rather than a split point, because `(S)` lets the piece come out of
// the middle — see interp/searchflag.go. The unflagged operators are the same
// span with one end pinned, so both readings leave through here and a fix to
// one cannot miss the other.
//
// Two readings of *which* match, rather than one, where the operator asks for
// the longest and the right-hand end of the match is free to move: the length
// reading below, and the written-arm reading in interp/trimarm.go. Both are
// found and the axis is asked only where they land in different places, so a
// pattern with no alternation — and one whose arms agree — never reaches an
// unanswered dialect's refusal.
func trimSpan(value, pattern string, op syntax.ParamOp, o patternOpts, arm armOrder,
	search bool,
) (int, int, matchReport, bool) {
	lo, hi, m, ok := spanByLength(value, pattern, op, o, search)
	if !ok || !writtenArmReaches(op, search) || !arm.reaches() {
		return lo, hi, m, ok
	}
	arms, prepared := newArmSearch(pattern, o)
	if !prepared {
		return lo, hi, m, ok
	}
	hi, m = armEnd(value, pattern, o, arms, arm, lo, hi, m)
	return lo, hi, m, true
}

// armEnd is where a match beginning at start ends once the written-arm
// reading has had its say: the length reading's end, unless a dialect prefers
// the arm that was written first and the two land in different places.
//
// One function for the trims and for the substitution, because it is one
// rule. It was the trim's alone, and a substitution takes the longest match
// at each position exactly as `${x##pat}` does — so the arm the matcher would
// have preferred was decided before the matcher was consulted there too, and
// `${w//(a|ab)/X}` on `abc` was `Xc` where zsh 5.9.2 says `Xbc` (#2152).
//
// The axis is asked **only where the two readings differ**, which is what
// keeps an unanswered dialect from being refused for having written an
// alternation at all.
func armEnd(value, pattern string, o patternOpts, arms armSearch, arm armOrder,
	start, end int, m matchReport,
) (int, matchReport) {
	j, decided := arms.endAt(value, o, start, end)
	if !decided || j == end {
		return end, m
	}
	if !arm.ask() {
		return end, m
	}
	// The edge is the written arm's and the report is the whole pattern's:
	// matchGroup prefers a written arm on its own, so matching the pattern
	// the script wrote against the piece this reading chose fills `$match`
	// with the same arm the search took.
	if armOK, armReport := matchPatternIn(pattern, value[start:j], value, start, o); armOK {
		return j, armReport
	}
	return end, m
}

// writtenArmReaches is where the two readings can land in different places at
// all: the operator asks for the longest match, and the end of that match is
// free rather than pinned to the end of the value.
//
// A longest *prefix* trim is the unflagged case, and `(S)` adds the searching
// suffix trim to it — under that flag a suffix match need not reach the end,
// so the arms have a length to disagree about. The unflagged suffix trim is
// the boundary and is measured rather than reasoned: `v=abcbc`, and both
// `${v%%(bc|cbc)}` and `${v%%(cbc|bc)}` are `ab` on zsh 5.9.2, the longest
// match in either written order, where `${(S)v%%(b|bc)}` on `abc` is `ac` and
// `${(S)v%%(bc|b)}` is `a`.
func writtenArmReaches(op syntax.ParamOp, search bool) bool {
	return trimTakesLongest(op) && (trimsPrefix(op) || search)
}

// spanByLength is trimSpan's length reading: the piece of the value the
// pattern matches, chosen by where a match may begin and by how much of one
// the operator asked for.
//
// One walk with two orders on it, and the orders are the whole of what the
// four operators and the `(S)` flag disagree about:
//
//	where a match may begin   a prefix trim pins it to 0 and a suffix trim
//	                          lets it move; under (S) both let it move, and
//	                          the search runs from the start for `#` and
//	                          from the end for `%`
//	where it may end          a suffix trim pins it to the end of the value
//	                          and a prefix trim lets it move; under (S) both
//	                          let it move
//
// With one end pinned the other carries the length choice, which is why an
// unflagged suffix trim walks its *starts* shortest-first for `%` where the
// prefix trim walks its ends that way. Under the flag the start order is the
// search direction and the end order is the length choice, both at once.
func spanByLength(value, pattern string, op syntax.ParamOp, o patternOpts,
	search bool,
) (int, int, matchReport, bool) {
	prefix := trimsPrefix(op)
	longest := trimTakesLongest(op)
	stops := unitStops(value, o)
	last := len(stops) - 1

	// What each end of the pattern requires of a piece, and how much subject
	// it could consume at all, so that a candidate the pattern could not
	// match whatever the subject holds is skipped rather than handed to the
	// matcher. See interp/patternspan.go for why both are allowed to ask too
	// little and never too much.
	head, tail, edges := patternEdgeLiterals(pattern, o)
	least, most, bounded := patternSpanBytes(pattern, o)

	first, final, step := 0, last, 1
	switch {
	case !search && prefix:
		// Pinned at the start: there is one place a match may begin.
		final = 0
	case !search && !prefix:
		// Pinned at the end, so the start is the length choice: the longest
		// suffix begins earliest and the shortest begins latest.
		if !longest {
			first, final, step = last, 0, -1
		}
	case search && !prefix:
		// The match that begins closest to the end, which the vendor manual
		// is explicit is not the one that *ends* closest to it.
		first, final, step = last, 0, -1
	}

	for a := first; ; a += step {
		lo := stops[a]
		// The ends this start admits, narrowed to the ones the pattern could
		// fill. A bounded pattern of n bytes leaves exactly one candidate at
		// each start rather than one per remaining unit.
		low, high := a, last
		if !search && !prefix {
			low, high = last, last
		}
		if bounded {
			low = max(low, sort.SearchInts(stops, lo+least))
			high = min(high, sort.SearchInts(stops, lo+most+1)-1)
		}
		// The candidate ends, walked in the order the operator asked for.
		// `at` rather than a range so that one loop serves both directions;
		// on the 13.5KB subject BenchmarkLongestPrefixTrimOnAPromptCache
		// carries, that indexing costs this trim about 15% against the two
		// hard-coded walks it replaces — measured, and kept, because the
		// alternative is a second walk for `(S)` to drift away from and the
		// figure it is 15% of is 57us against the 2139ms the analysis in
		// interp/patternspan.go took off this same expansion.
		at, ahead := low, 1
		if longest {
			at, ahead = high, -1
		}
		for n := high - low; n >= 0; n-- {
			hi := stops[at]
			at += ahead
			piece := value[lo:hi]
			if !edgeLiteralsFit(piece, head, tail, edges) {
				continue
			}
			// The piece is matched where it sits, so a `(#s)` matches only a
			// piece starting at 0 and a `(#e)` only one ending at the last
			// unit. Measured on zsh 5.9.2, `x=abcd; ${x#ab(#e)}` leaves
			// `abcd` alone where `${x#abcd(#e)}` empties it.
			if ok, m := matchPatternIn(pattern, piece, value, lo, o); ok {
				return lo, hi, m, true
			}
		}
		if a == final {
			break
		}
	}
	return 0, 0, matchReport{}, false
}

// replace substitutes a matching span, once or everywhere.
//
// The anchored forms match only at one end, which is what `/#` and `/%` mean.
// with is handed both the match report the pattern filled and the **text**
// the match took, which is what an `&` in the replacement stands for. The
// text is passed rather than read back off the report because the report
// carries a span only where the pattern asked for one — a plain `b` fills
// nothing — and the ampersand is read whatever the pattern was.
//
// `(S)` is the order the spans at a position are walked in and nothing else:
// shortest first where the operator otherwise takes the longest, and the same
// turn for the two anchored forms. Every rule below about empty matches and
// about making progress is written against whichever span came back, so the
// flag inherits all of them — see interp/searchflag.go.
//
// arm is the written-arm reading a substitution shares with the trims, and it
// reaches exactly where the *longest* match is wanted and the end of it is
// free to move: the unanchored forms and `/#`, but not `/%`, which pins the
// end, and not under `(S)`, which asks for the shortest.
//
// **Those last two are skipped rather than guarded against**, and the
// difference matters to a reader: asking there could not change an answer, so
// the condition below buys the preparation rather than a behavior. `/%` never
// consults it because its branch has no free end to move. And under `(S)` the
// arm search is bounded above by the shortest match, which is the *minimum*
// over every variant — so no variant can match shorter, and the first one
// that matches at all matches exactly there. Mutating either condition away
// leaves every test passing, which is the proof rather than a gap in them.
//
// Measured on zsh 5.9.2 with `w=abc`:
//
//	${w//(a|ab)/X}     Xbc    the arm that was written first
//	${w//(ab|a)/X}     Xc
//	${w/(|a)/X}        Xabc   an empty arm is an arm
//	${w/%(c|bc)/X}     aX     the anchor pins the end: no disagreement
//	${w/%(bc|c)/X}     aX
//	${(S)w//(a|ab)/X}  Xbc    shortest first, in either written order
//	${(S)w//(ab|a)/X}  Xbc
//
// declined resolves Semantics.ReplacementEmptyMatchDeclined, and is a
// function rather than a value because an unanswered axis is *reported* when
// it is read: calling it at the top would refuse every substitution instead
// of the two shapes the readings disagree about. Both call sites below are
// reached only by a pattern that matched empty.
//
// See armEnd, which is the whole of the rule, and interp/trimarm.go for the
// search behind it.
func replace(value, pattern string, e *syntax.ParamExpr, o patternOpts, arm armOrder,
	declined func() EmptyMatchDeclinedPolicy, with func(matchReport, string) string,
) string {
	// Every position a match may start or end at, in order, and there is one
	// more of them than there are units. They are unit boundaries rather than
	// byte offsets, so a pattern is never handed half of a character —
	// `${s//?/X}` on a three-character string is `XXX` and not `XXXXXXXXX`.
	stops := unitStops(value, o)

	// How much subject this pattern could possibly consume. Every loop below
	// walks spans and asks the matcher about each one, and a span outside
	// these bounds is one the pattern cannot fill however the subject reads —
	// so the question is skipped rather than asked. See patternspan.go for
	// why the bound is allowed to be too wide and never too narrow.
	lo, hi, bounded := patternSpanBytes(pattern, o)

	shortest := searchingFlag(e)

	// Prepared once for the whole substitution rather than at each position:
	// resolving the arms rewrites the pattern text, and a global substitution
	// asks at every unit of the subject. Not prepared at all for the two
	// spellings whose answer it could not change — see the note above.
	var arms armSearch
	takesTheArm := !shortest && e.Anchor != '%' && arm.reaches()
	if takesTheArm {
		arms, takesTheArm = newArmSearch(pattern, o)
	}

	switch e.Anchor {
	case '#':
		// Anchored at the start, so the end is the length choice: measured,
		// `v=abcabc` gives `${v/#a*b/X}` as `Xc` and `${(S)v/#a*b/X}` as
		// `Xcabc`.
		first, final, step := len(stops)-1, 0, -1
		if shortest {
			first, final, step = 0, len(stops)-1, 1
		}
		for k := first; ; k += step {
			if spanCouldMatch(stops[k], lo, hi, bounded) {
				if ok, m := matchPatternIn(pattern, value[:stops[k]], value, 0, o); ok {
					end := stops[k]
					if takesTheArm {
						end, m = armEnd(value, pattern, o, arms, arm, 0, end, m)
					}
					return with(m, value[:end]) + value[end:]
				}
			}
			if k == final {
				return value
			}
		}
	case '%':
		// And anchored at the end, so the start is: `${v/%b*c/X}` is `aX`
		// and `${(S)v/%b*c/X}` is `abcaX`.
		//
		// Nothing here asks the written-arm reading, and that is the whole of
		// why: the end is pinned, so every match at a given start is the same
		// length and the arms have nothing to disagree about. Measured,
		// `x=abc` gives `aX` for both `${x/%(c|bc)/X}` and `${x/%(bc|c)/X}`.
		first, final, step := 0, len(stops)-1, 1
		if shortest {
			first, final, step = len(stops)-1, 0, -1
		}
		for k := first; ; k += step {
			i := stops[k]
			if spanCouldMatch(len(value)-i, lo, hi, bounded) {
				if ok, m := matchPatternIn(pattern, value[i:], value, i, o); ok {
					return value[:i] + with(m, value[i:])
				}
			}
			if k == final {
				return value
			}
		}
	}

	var b strings.Builder
	// Where the match before this one ended, so that an empty match sitting
	// on it can be recognized. -1 until something has matched.
	lastEnd := -1
	for k := 0; k < len(stops); {
		i := stops[k]
		// The match at this position the operator asked for, so `*` behaves
		// as it does everywhere else rather than matching empty and looping.
		end := -1
		var rep matchReport
		// Bounded to the spans the pattern could *fill* rather than running
		// to the end of the subject. For a pattern of four ordinary
		// characters that is one span instead of one per remaining unit,
		// which is the whole of #1398.
		top := len(stops) - 1
		if bounded {
			top = sort.SearchInts(stops, i+hi+1) - 1
		}
		bottom := k
		if bounded {
			bottom = max(k, sort.SearchInts(stops, i+lo))
		}
		mFirst, mFinal, mStep := top, bottom, -1
		if shortest {
			mFirst, mFinal, mStep = bottom, top, 1
		}
		for m := mFirst; bottom <= top; m += mStep {
			// Every span tried is a piece of value and is matched as one,
			// so a `(#s)` matches only the span starting at 0 and a `(#e)`
			// only the one ending at the last unit. Measured:
			// `x=XbXcX; ${x//(#s)X/-}` is `-bXcX`, not `-b-c-`.
			ok, got := matchPatternIn(pattern, value[i:stops[m]], value, i, o)
			if ok {
				end, rep = stops[m], got
				break
			}
			if m == mFinal {
				break
			}
		}
		if end >= 0 && takesTheArm {
			// The position is settled; the arm may still move where the
			// match ends. Asked here rather than inside the span walk above
			// because the two readings agree about *where* a match begins
			// and differ only about how much of one to take.
			end, rep = armEnd(value, pattern, o, arms, arm, i, end, rep)
		}
		if end == i && lastEnd == i && declined() == EmptyMatchDeclinedAfterAMatch {
			// An empty match where the match before it ended. One reading
			// takes it and the other refuses to replace twice in the same
			// place, and this is one of the two positions they part on:
			// `v=abc` under `@(b|)` is `<>a<><>c` where it is taken and
			// `<>a<>c<>` where it is not.
			end = -1
		}
		if end < 0 || end == i && pattern != "" && !matchPattern(pattern, "", o) {
			if i < len(value) {
				b.WriteString(value[i:stops[k+1]])
			}
			k++
			continue
		}
		b.WriteString(with(rep, value[i:end]))
		lastEnd = end
		if !e.All {
			b.WriteString(value[end:])
			return b.String()
		}
		if end == len(value) {
			// A match that ended at the end of the value leaves nothing to
			// scan, so the empty match waiting at the final stop is not a
			// second match. Without this, a pattern that can match empty
			// replaces once more than it matched: `${v//*/X}` on a non-empty
			// value is one X, because the `*` took the whole of it.
			return b.String()
		}
		if end == i {
			// An empty match must still make progress, and the unit it steps
			// over may be the last one — in which case the scan is over
			// rather than reaching the end of the value as one more position
			// to try.
			//
			// **The same position reached by a failed match is still tried**,
			// which is what makes this a rule about the step and not about
			// the position. Measured on zsh 5.9.2 with extended_glob and
			// `v=abc`, four patterns that differ only in what happens at the
			// `c`:
			//
			//	${v//x#/-}          -a-b-c   empty at 2, so 3 is not a
			//	                             position — not `-a-b-c-`
			//	${v//(#e)/-}        abc-     nothing matched at 2, so 3 is
			//	${v//(x#|(#e))/-}   -a-b-c   empty at 2 again, and the arm
			//	                             that could fire at 3 does not
			//	${v//((#s)|(#e))/-} -abc-    empty at 0, nothing at 1 or 2
			//
			// A value with no units at all has no preceding step and keeps
			// its one match: `${(q):-}`'s empty string under `//x#/-` is `-`.
			if i < len(value) {
				b.WriteString(value[i:stops[k+1]])
			}
			k++
			if k < len(stops) && stops[k] == len(value) && endOfValueDeclined(value, pattern, o, declined) {
				return b.String()
			}
			continue
		}
		for stops[k] < end {
			k++
		}
	}
	return b.String()
}

// endOfValueDeclined is whether the scan stops at the end of the value it has
// just stepped onto after an empty match, which is the second of the two
// positions Semantics.ReplacementEmptyMatchDeclined parts the panel on.
//
// The match is tried before the axis is read, and that order is the point: a
// pattern that cannot match there produces the same result under either
// reading, so asking would refuse a substitution over a question that could
// not have changed its answer.
func endOfValueDeclined(value, pattern string, o patternOpts, declined func() EmptyMatchDeclinedPolicy) bool {
	if ok, _ := matchPatternIn(pattern, "", value, len(value), o); !ok {
		return true
	}
	return declined() != EmptyMatchDeclinedAfterAMatch
}

// unitStops is every position a match may begin or end at: each unit boundary
// of value, and the end of it. One byte apart where a unit is a byte, and one
// character apart where a unit is a character.
func unitStops(value string, o patternOpts) []int {
	stops := make([]int, 0, len(value)+1)
	for i := 0; ; i += o.unitWidth(value[i:]) {
		stops = append(stops, i)
		if i == len(value) {
			return stops
		}
	}
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
		out, ok := r.applyModifiers(value, modifierSegments(
			modifierSource(e.ArgText, e.Arg), modifierSource(e.Arg2Text, e.Arg2), e.Arg2 != nil), e)
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
		out, ok := r.applyModifiers(sliced, modifierSegments(modifierSource(e.Arg2Text, e.Arg2), "", false), e)
		if !ok {
			return ""
		}
		return out
	}
	// Or a length *and then* a modifier list, which is a third shape and not
	// a variation of either: `${x:1:5:t}` is the tail of the five characters
	// from offset one. The parser splits a range once, so `5:t` arrives whole
	// and reached the evaluator as an expression — an arithmetic failure over
	// a range every shell with modifiers reads without complaint.
	if lenWord, mods, ok := splitLengthFromModifiers(e.Arg2); ok &&
		r.ask(r.sem().SubstringRangeReadsModifiers,
			"a substring range beginning with a letter being a modifier list") {
		sliced := substring(value, r.numOf(e.Arg, e, lenWord), &syntax.ParamExpr{
			Name: e.Name, Op: e.Op, Arg: e.Arg, Arg2: lenWord,
		}, r)
		out, ok := r.applyModifiers(sliced, mods, e)
		if !ok {
			return ""
		}
		return out
	}
	return substring(value, r.numOf(e.Arg, e, e.Arg2), e, r)
}

// splitLengthFromModifiers separates a length from the modifier list behind
// it, for the one shape the parser cannot split on its own.
//
// A range is split once, at its first colon, so `${x:1:5:t}` gives an offset
// of `1` and a length word holding `5:t`. Splitting further here rather than
// in the parser keeps a range's shape a question about this dialect and not
// about the grammar, which is the same reason modifierSegments splits a
// chain here.
//
// Only where the length is plain unquoted text, which is what the shape is:
// `${x:1:$n:t}` puts an expansion where the split would be, and guessing at
// its colons would be reading a value rather than a range. Such a word is
// left to the evaluator exactly as it was.
func splitLengthFromModifiers(w *syntax.Word) (*syntax.Word, []string, bool) {
	if w == nil || len(w.Spans) != 1 {
		return nil, nil, false
	}
	s := w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
		return nil, nil, false
	}
	head, tail, found := strings.Cut(s.Value, ":")
	if !found || head == "" || tail == "" {
		return nil, nil, false
	}
	return &syntax.Word{
		Spans: []syntax.Span{{Kind: syntax.Literal, Value: head}},
		Start: w.Start, Stop: w.Stop,
	}, strings.Split(tail, ":"), true
}

// substring takes a slice of the value.
//
// The offset and the length count the same units `${#x}` does — characters
// where the dialect and the locale both say so, bytes otherwise — which is
// why it walks a slice of units rather than indexing the string. Measured
// under a UTF-8 locale, `s=héllo; ${s:1:2}` is `él` in bash, ksh93 and zsh
// and `${s:2}` is `llo`; under `LC_ALL=C` the same shells give `é` and `llo`
// with the `é` cut in half, which is what indexing bytes produces.
func substring(value string, off int, e *syntax.ParamExpr, r *Runner) string {
	lenWord := e.Arg2
	units := r.units(value)
	if off < 0 {
		off += len(units)
	}
	if off < 0 {
		off = 0
	}
	if off > len(units) {
		return ""
	}
	if lenWord == nil {
		return strings.Join(units[off:], "")
	}
	n := r.numOf(lenWord, e, nil)
	if n < 0 {
		if r.ask(r.sem().SubstringNegativeLengthIsEmpty, "a negative substring length") {
			// One dialect answers a negative length with nothing at all;
			// the others count it from the end.
			return ""
		}
		// A negative length is an offset from the end.
		n = len(units) + n - off
	}
	if n < 0 {
		n = 0
	}
	if off+n > len(units) {
		n = len(units) - off
	}
	return strings.Join(units[off:off+n], "")
}

func (r *Runner) joinWord(w *syntax.Word) string {
	return strings.Join(r.expandWord(w), " ")
}

// substitutedWordText is the text an operator's word comes to where the
// operator takes it as a *value* rather than as fields of the command line:
// the assigning forms `:=` and `::=`, and the word `?` complains with.
//
// Not matched against the filesystem. `u=; printf "<%s>" "${u:=X[a-b]y}"`
// stores those seven characters in all six panel shells — the bracket
// expression never reaches a match on its way into the parameter — and what
// becomes of them afterwards is the ordinary rule for an expansion's result,
// which GlobExpansionResults already answers: bash, bash-as-sh, bash 3.2,
// dash and ksh93 read them back as a pattern, zsh does not. Measured
// 2026-09-08 in a directory holding `Xay` and `Xby`: the five print
// `[Xay][Xby]` from the expansion and `<X[a-b]y>` from the variable, zsh
// prints `[X[a-b]y]` and the same variable.
//
// Matching here stored the listing instead — `Xay Xby`, in the parameter,
// where every shell in the panel keeps the text — and took the question away
// from the axis that owns it (#1500). A diagnostic's word is the same shape:
// `${u?X[a-b]y}` names the word, not the files it would have found.
func (r *Runner) substitutedWordText(w *syntax.Word) string {
	defer r.withoutGlobbing()()
	return r.joinWord(w)
}

// replacementOf is the text a `${x/pat/rep}` substitutes.
//
// **A replacement is text, not a pattern**, and this is the only reason it
// cannot go through joinWord: that one expands a word the ordinary way, which
// matches it against the filesystem and splits what comes back. So the `*` in
// `${x//b/*}` listed the directory and the list was joined with spaces and
// pushed into the middle of the value — a plausible string at status 0, with
// nothing said (#1337). `${x//b/[Q]}` was the loud half of the same fault: the
// bracket expression matched no file, and in the dialect where that is fatal
// the whole command stopped.
//
// Measured 2026-09-07 across bash 5.3.15, that build as `sh`, bash 3.2.57,
// ksh93 and zsh 5.9.2, in a directory holding a file the pattern would have
// found: `x=abcd; y=${x//b/*}` puts `a*cd` in the variable in all five. The
// tests here ask it through a *quoted* expansion instead, and that is not a
// weaker assertion but the only one that works — an assignment's value is
// expanded with pathname expansion suspended for the whole word, nested
// operands included, so a row written `y=…; echo "$y"` passes with the fault
// in place. The replacement is *expanded* like
// any word — `${x/b/$r}` with `r="p q"` substitutes both words and one space —
// and it is neither globbed nor split while it is being read: the five agree
// on `ap qcd` in the variable, and bash and ksh93 then split the **whole
// expansion** into `[ap]` and `[qcd]` when it is used unquoted, which is the
// ordinary rule for an expansion's result and not the replacement's own.
//
// What happens to the metacharacters *after* they land is that same ordinary
// rule, and it is already answered elsewhere: unquoted, bash and ksh93 read
// the result as a pattern and `a*cd` finds `axcd`, while zsh does not and
// prints the four characters. Both follow from GlobExpansionResults once the
// replacement stops globbing on its own.
func (r *Runner) replacementOf(w *syntax.Word) string {
	return strings.Join(r.expandWordNoSplit(w), "")
}

// replacementFor reads the replacement word and answers the function from the
// text a match took to the text that replaces it.
//
// Where the ampersand is not read the answer ignores its argument, and that is
// the whole of the difference between the two states of
// [ReplacementAmpersandIsTheMatch]. The word is expanded here, once per call,
// so a caller that must read the replacement again for each match calls this
// again and one that must not does not — see replaceWith for why that
// distinction is behavior rather than an optimization.
func (r *Runner) replacementFor(w *syntax.Word) func(matched string) string {
	if !r.MatchOption(ReplacementAmpersandIsTheMatch) {
		with := r.replacementOf(w)
		return func(string) string { return with }
	}
	tmpl := r.replacementTemplate(w)
	return func(matched string) string {
		return expandAmpersand(tmpl, matched, backslashProtectsItself)
	}
}

// replacementTemplate is replacementOf with the quoting kept: an `&` or a
// backslash that came from a quoted span comes back marked with a backslash,
// which is the spelling expandAmpersand writes back out as itself.
//
// Quoting is what decides whether an `&` is read, exactly as it decides
// whether a `*` is a pattern — see patternOf, which carries the same rule for
// the other operand of the same operator. Measured on bash 5.3.15 with
// `v=abc`: `${v/b/"&"}`, `${v/b/'&'}`, `${v/b/$'&'}` and `${v/b/\&}` are all
// `a&c`, and with `r='&'` the unquoted `${v/b/$r}` is `abc` where the quoted
// `${v/b/"$r"}` is `a&c`. The enclosing quotes are **not** what is asked —
// `"${v/b/[&]}"` still reads the match — because a span carries the quoting it
// was written in and the operand's own reading is replacementWord's question.
func (r *Runner) replacementTemplate(w *syntax.Word) string {
	if w == nil {
		return ""
	}
	return r.wordTextNoSplit(w, func(text string, q syntax.Quoting) string {
		if q == syntax.Unquoted {
			return text
		}
		return escapeAmpersand(text)
	})
}

// escapeAmpersand marks text that must be written out as itself: an `&` and a
// backslash each take a backslash in front, which is what
// backslashProtectsItself undoes.
func escapeAmpersand(text string) string {
	if !strings.ContainsAny(text, `\&`) {
		return text
	}
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '\\' || text[i] == '&' {
			b.WriteByte('\\')
		}
		b.WriteByte(text[i])
	}
	return b.String()
}

// replacementWord is the replacement operand of `${v/pat/repl}`, chosen
// between the two readings the parser kept.
//
// Arg2Enclosed is nil unless the expansion was double-quoted *and* the two
// readings could come to different text, so this asks the axis only at the
// disagreement: an ordinary `"${v/a/b}"` never reaches it, and neither does
// any unquoted one, where the whole panel agrees with the word reading.
//
// See Semantics.ReplacementOperandTakesTheEnclosingQuoting for the panel, and
// syntax.ParamExpr.Arg2Enclosed for why both readings are parsed rather than
// one being derived from the other: the trees are not the same shape, so the
// choice has to be made before either is expanded.
func (r *Runner) replacementWord(e *syntax.ParamExpr) *syntax.Word {
	if e.Arg2Enclosed == nil {
		return e.Arg2
	}
	if r.ask(r.sem().ReplacementOperandTakesTheEnclosingQuoting,
		"a quote in a quoted replacement operand") {
		return e.Arg2Enclosed
	}
	return e.Arg2
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
	return splitFieldsEdges(s, literal, ifs, ifsSet, false, false)
}

// splitFieldsEdges is splitFieldsLiteral with the discarding of the outermost
// delimiters made a parameter.
//
// keepEdges false is the ordinary rule above. keepEdges true says a delimiter
// at either end of the value still separates, so n delimiters always give
// n+1 fields: `' a '` is three fields and `”` is one empty one. That is what
// a quoted `${=spec}` measures — see interp/splitflag.go — and it is a
// parameter here rather than a splitter of its own, because everything else
// about the two is the same rule and a copy of it would drift.
//
// escaped says the string is a field in the escaped form rather than plain
// text, which is what every caller splitting the result of an expansion hands
// it. `read` and `${#(w)v}` hand it plain text and pass false. The two cannot
// be told apart by looking, since a backslash is a legal character of a value
// as well as the form's own mark — so it is the caller that knows.
func splitFieldsEdges(s string, literal []bool, ifs string, ifsSet, keepEdges, escaped bool) []string {
	fields, _ := splitFieldsAt(s, literal, ifs, ifsSet, keepEdges, escaped)
	return fields
}

// splitFieldsAt is splitFieldsEdges with each field's offset in s reported
// beside it: at[i] is where fields[i] began.
//
// `read` is what the offsets are for. The last name on its list takes the
// remainder of the *line* from where its own field started — separators and
// all — and a list of fields cannot say where that was. Rebuilding it by
// joining the fields back together is what silently replaced every separator
// in `IFS=: read -r user rest` with a space (#1208), and the offset is the
// only thing that makes the difference recoverable. It is reported from the
// one splitter rather than recomputed beside it, because a second walk of the
// same rule is a second place for it to drift.
func splitFieldsAt(s string, literal []bool, ifs string, ifsSet, keepEdges, escaped bool) ([]string, []int) {
	if ifsSet && ifs == "" {
		// Set and empty disables the stage entirely, which is a different
		// state from unset rather than a degree of it.
		if s == "" {
			return emptyFields(keepEdges)
		}
		return []string{s}, []int{0}
	}
	if s == "" {
		return emptyFields(keepEdges)
	}

	// The escaped form spells "this byte was quoted" as a backslash in front
	// of it, so a byte of it is either a mark or data and a walk that reads
	// every byte as data cannot tell the two apart. See escapedMarks, and
	// #2212 for what reading them as data did here.
	var marks []bool
	if escaped {
		marks = escapedMarks(s)
	}
	isMark := func(i int) bool { return marks != nil && marks[i] }
	isWS := func(i int) bool {
		c := s[i]
		return !isMark(i) && (literal == nil || !literal[i]) &&
			strings.IndexByte(ifs, c) >= 0 && (c == ' ' || c == '\t' || c == '\n')
	}
	isSep := func(i int) bool {
		return !isMark(i) && (literal == nil || !literal[i]) && strings.IndexByte(ifs, s[i]) >= 0
	}
	// cutAt is where the field in front of the separator at i ends. A marked
	// separator still separates — a value's backslash quotes for the *match*
	// and never for the split, which the whole panel agrees about — but its
	// mark belongs to the separator and goes with it, rather than staying on
	// the end of the field as a backslash nobody wrote.
	cutAt := func(i int) int {
		if i > 0 && isMark(i-1) {
			return i - 1
		}
		return i
	}

	var out []string
	var at []int
	i := 0
	for !keepEdges && i < len(s) && isWS(i) { // leading IFS whitespace is discarded
		i++
	}
	for i < len(s) {
		start := i
		for i < len(s) && !isSep(i) {
			i++
		}
		if i >= len(s) {
			out = append(out, s[start:i])
			at = append(at, start)
			break
		}
		out = append(out, s[start:cutAt(i)])
		at = append(at, start)
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
		// handled by reading an empty field before consuming it. Under
		// keepEdges neither is absorbed, so the field behind the last
		// delimiter is written out here.
		if keepEdges && i >= len(s) {
			out = append(out, "")
			at = append(at, i)
		}
	}
	return out, at
}

// emptyFields is what splitting nothing comes to: no field at all, or the one
// empty field the edge-keeping rule leaves behind, with its offset.
func emptyFields(keepEdges bool) ([]string, []int) {
	if keepEdges {
		return []string{""}, []int{0}
	}
	return nil, nil
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
		// The recorded pid rather than the current job, and the two are not
		// the same once the job has ended: a finished job is forgotten by
		// the notice that reports it, and `$!` still names the process
		// afterwards in every shell in the panel. See Runner.lastJobPID.
		if !r.lastJobPIDSet {
			// One shell answers with a number nothing ever had. Read
			// without asking, so a preset that has not chosen answers with
			// nothing — which is what the other five do.
			if r.sem().LastBackgroundPidIsZeroBeforeAnyJob == Yes {
				return "0", true
			}
			return "", true
		}
		return itoa(r.lastJobPID), true
	case "0":
		// zsh reports whatever the shell is *inside* — the function being
		// run, or the file being sourced — where every other shell reports
		// the shell's own name however deep it is.
		//
		// The innermost call and not the innermost function: a file sourced
		// from a function is what `$0` names while it runs, so asking
		// r.inFunc would answer with the function around it. See
		// Semantics.DollarZeroNamesTheInnermostCall for the measurement.
		if in, ok := r.innermostCall(); ok &&
			r.ask(r.sem().DollarZeroNamesTheInnermostCall, "$0 naming the function or sourced file it is inside") {
			return in, true
		}
		return r.Name, true
	case "*", "@":
		// Reached where expandAt declined and one string is what the context
		// wants: a here-document body, which is lexed as double-quoted text
		// and expanded span by span, and an operator's operand — the pattern
		// in `${v%$@}`, say.
		//
		// Which character joins them is the same question the word-level
		// callers ask, and it is asked in one place rather than reimplemented
		// here. `$*` joins on the first character of IFS in every shell in
		// the panel and needs no answer; `$@` is the axis, and this branch
		// had it hardcoded to a space — so `IFS=-; set -- x y; v="Zx-y";
		// echo ${v%$@}` trimmed nothing where zsh and dash trim to `Z`, and
		// a here-document body printed `x y` where they print `x-y`.
		// The spelling comes from the name rather than through starSpelled,
		// which reads the subscript and so reaches expandWord — a chain this
		// function must stay out of, because the arithmetic evaluator calls
		// it while the builtin table is still being built and Go reports the
		// result as an initialization cycle. The name is the whole answer
		// here regardless: a subscripted `${*[1,2]}` is answered by the array
		// path and only reaches this line when it was not an array at all.
		return strings.Join(r.Params, r.unsplitJoinSeparator(e.Name == "*", len(r.Params))), true
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
	out, _, _ := r.expandRawSpans(text)
	return out
}

// expandRawSpans is that expansion with the two facts a *boundary* needs: how
// far it got, and whether it got there.
//
// Three results rather than one, and each is read by somebody. The text is
// what every caller wanted. ok is false where a span failed, which is what
// lets a caller stop rather than ask the runner. head is the literal text in
// front of the **first** substitution — what a prompt expansion hands back
// when the pass it is running gives up partway; see expandPromptText, which is
// the only reader of it.
//
// The loop abandons the text at its first failure, which is the rule
// expandWord and wordTextNoSplit already follow and which this one did not:
// measured, a here-document body holding `$((nofunc()))` twice is one
// diagnostic in zsh and was two here, because nothing stopped the walk.
func (r *Runner) expandRawSpans(text string) (out, head string, ok bool) {
	var b, h strings.Builder
	spans, ok := r.rawSpans(text)
	if !ok {
		return "", "", false
	}
	// Whether an expansion had already failed before this text, which is not
	// this text's doing — the same comparison expandWord makes, and for the
	// same reason.
	failed := r.expandErr
	stopped := func() bool { return (r.expandErr && !failed) || r.ctl == controlExit }
	// literal is whether everything so far has been literal text, which is
	// what head is accumulating: the first substitution closes it, whether
	// that substitution succeeds or not.
	literal := true
	// The lexer leaves an expansion's inside raw, so parseSpans fills it in —
	// the same handoff a word goes through. splitNever: a here-document's
	// body is one blob of input rather than fields, in every shell in the
	// panel, so the splitting axis has nothing to ask.
	for _, s := range spans {
		if stopped() {
			return b.String(), h.String(), false
		}
		// head is false, and it is the belt to the quoting's braces: a
		// here-document's spans are marked double-quoted — which is what
		// stops the body being split — so `${~t}` in one is suppressed by
		// the quoting before the head is consulted. Measured, a `${~t}` in a
		// body is the value unchanged, and a mutant passing true here is
		// unobservable for that reason.
		part, _ := r.expandSpan(s, splitNever, false)
		// expandSpan marks a literal's metacharacters for the glob stage,
		// and a here-document has no glob stage — the text is input, not a
		// pattern. Without this a backslash in the body came out doubled.
		part = globUnescape(part)
		b.WriteString(part)
		switch {
		case !literal:
		case s.Kind == syntax.Literal:
			h.WriteString(part)
		default:
			literal = false
		}
	}
	return b.String(), h.String(), !stopped()
}

// rawSpans reads raw text back into spans, refusing text that ran out inside
// an expansion instead of handing back a span nobody wrote.
//
// The refusal is the point. A `${` with no closing brace comes back from the
// lexer as a parameter expansion whose name is whatever followed it, so a
// caller that ignored the error answered the *rest of the text* and status 0
// where every shell in the panel abandons the line — `x${` re-read through
// the evaluate flag printed `x`, and a here-document body holding one printed
// its body. Where the dialect happens to refuse an empty name that came out
// looking correct, which is worse: two columns right by accident and the
// third silently wrong (#1653).
//
// A parse failure here is an expansion failure, reported and marked the same
// way the arithmetic reader's is, because it is the same kind of thing: text
// that could only be read once it had been produced.
func (r *Runner) rawSpans(text string) ([]syntax.Span, bool) {
	spans, err := syntax.HeredocSpans(text, r.dialect())
	if err != nil {
		r.diagf("%s\n", r.diag().ParseFailure(err))
		r.expandErr = true
		return nil, false
	}
	return r.parseSpans(spans), true
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
	// tildeValue is the whole of the rule, shared with the substituted tilde
	// `${~name}` produces so the two cannot drift apart.
	s.Value = r.tildeValue(s.Value)
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
	if w := r.substitutedWordText(e.Arg); w != "" {
		return w
	}
	const notSet = "parameter not set"
	if !e.Colon {
		return notSet
	}
	d := r.diag()
	// Whether an empty positional list counts as set is the axis
	// PositionalListWithNoneIsSet, and it reaches the *wording* of a colon
	// form as well as the firing of a colon-less one: `set --; ${@:?}` says
	// `parameter not set` in the column that calls the list unset and names
	// the null in the columns that do not.
	//
	// Read rather than asked, which is the split unsetBlanksInPlace makes
	// for the same reason: the colon form fires either way, so there is no
	// behavior to refuse — only which of a dialect's own two sentences it
	// picks, and a vector with no dialect has neither.
	if e.Name == "@" || e.Name == "*" {
		if e.Subscript() == nil && len(r.Params) == 0 {
			set = r.sem().PositionalListWithNoneIsSet == Yes
		}
	}
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
	case syntax.ParamAssignAlways:
		// The always-assign operator never reads the value, so there is
		// nothing for `set -u` to be about. Measured 2026-09-07 on zsh
		// 5.9.2, the only shell with the construct: `setopt nounset; unset
		// v; ${v::=new}` is `new` at status 0.
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
		r.fatalExpansion("%s\n", r.unboundSigilWording(e.Name))
		return
	}
	if e.Name == "!" {
		// `$!` before any background command, where the dialect calls that
		// unset — see LastBackgroundPidIsUnsetBeforeAnyJob, which is what
		// decides whether this is reached at all. The sigil is written back
		// by the same shell and the same rule as for a positional, so it is
		// the same field: `$!: unbound variable` against
		// `!: parameter not set`.
		//
		// Not through UnsetPositionalIsAllowed. That axis is ksh93 letting an
		// argument it was not given be empty, and ksh93's answer here comes
		// from the other axis instead — it never reaches this line, so
		// asking would be asking the wrong question of the one dialect it
		// would change.
		r.fatalExpansion("%s\n", r.unboundSigilWording(e.Name))
		return
	}
	if !isPositional(e.Name) {
		r.fatalExpansion("%s\n", Wording(r.diag().UnboundVariable, "%s: parameter not set", e.Name))
	}
}

// unboundSigilWording is the `set -u` refusal for a parameter whose name is
// not a variable name — a positional, and `$!`.
//
// One wording for both because it is one measurement: bash writes the `$` back
// for each of them and says `unbound variable`, and the other three write the
// name alone and say `parameter not set`, which is what UnboundVariable
// already holds. Empty means the two are the same line.
func (r *Runner) unboundSigilWording(name string) string {
	format := r.diag().UnboundPositional
	if format == "" {
		format = r.diag().UnboundVariable
	}
	return Wording(format, "%s: parameter not set", name)
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
			n, used := hexEscapeRun(s[i+2:], r.dollarSingleHexEveryDigit(s[i+2:]))
			if used == 0 {
				if !r.digitlessEscape(&b, `\x`) {
					return b.String()
				}
				i += 2
				continue
			}
			if used <= 2 {
				// One or two digits are a byte in every reading, which is
				// the road to a NUL that DollarSingleNulTruncates answers.
				if !r.writeDecodedByte(&b, byte(n)) {
					return b.String()
				}
				i += 2 + used
				continue
			}
			// A longer run is a code point, in the one reading that takes
			// one. The encoder is the shell's own — a value past the last
			// code point is written in the extended form UTF-8 has room
			// for rather than refused, which is measured.
			b.WriteString(EncodeCodePoint(n))
			i += 2 + used
		case c == 'u' || c == 'U':
			width := 4
			if c == 'U' {
				width = 8
			}
			n, used := scanBase(s[i+2:], 16, width)
			if used == 0 {
				if !r.digitlessEscape(&b, `\`+string(c)) {
					return b.String()
				}
				i += 2
				continue
			}
			if n == 0 {
				if !r.writeDecodedByte(&b, 0) {
					return b.String()
				}
			} else {
				// The same reader every other site that has this escape
				// uses, locale and all: the character, the escape written
				// back, or a refusal — see Runner.CodePointEscapeText. It
				// is also the core's one encoder, which WriteRune was not:
				// a surrogate and a value past the last code point are the
				// encoding itself in every shell that has the escape, and
				// a rune conversion makes both a replacement character.
				//
				// This is the one site of the five in the **core**, and it
				// is where the axis's third answer comes from: a shell with
				// `$'…'` and no `\u` anywhere else writes the character
				// whatever the locale says (#2021).
				text, refused := r.CodePointEscapeText(n)
				if refused {
					// A word that was never finished, so the command it
					// belonged to never runs. Measured 2026-09-11 under
					// `LC_ALL=C`: `x=$'a\u00e9Z'; echo AFTER` writes the
					// complaint, nothing else, and leaves status 1 — where
					// the same refusal inside a builtin leaves 0.
					r.RefuseCodePointExpanding()
					return b.String()
				}
				b.WriteString(text)
			}
			i += 2 + used
		case c >= '0' && c <= '7':
			n, used := scanBase(s[i+1:], 8, 3)
			if !r.writeDecodedByte(&b, byte(n)) {
				return b.String()
			}
			i += 1 + used
		case c == 'C' || c == 'M':
			if !r.ask(r.sem().DollarSingleCaretMeta, "the `\\C-` and `\\M-` escapes of a `$'…'`") {
				// A shell without them, or one that has not said. Either
				// way the backslash is before a character nothing here
				// claims, and the unknown rule decides it.
				r.writeUnknownEscape(&b, c)
				i += 2
				continue
			}
			v, next, ok := caretMetaEscape(s, i)
			if !ok {
				// Nothing left to make a byte out of: measured, `$'x\C'`
				// and `$'x\M-'` are both `x`, the escape producing nothing
				// rather than the characters it was written with.
				i = next
				continue
			}
			if !r.writeDecodedByte(&b, v) {
				return b.String()
			}
			i = next
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

// caretMetaEscape reads the `\C-…` or `\M-…` starting at i, where s[i] is
// the backslash, and returns the byte it comes to and the offset past it. ok
// is false when the escape has no argument at all, which produces nothing.
//
// The separating `-` is optional and the argument may be a further escape,
// this one included. Measured on zsh 5.9.2, 2026-09-08, by `od`:
//
//	$'\C-A'  01   $'\CA'      01   the dash is optional
//	$'\C-a'  01   $'\C-1'     11   the argument is masked, not uppercased
//	$'\C-?'  7f   $'\C-\x7f'  1f   and `?` alone is the delete byte
//	$'\C-\M-?' 9f              which a meta bit takes it out of
//	$'\C--'  0d   $'\C- '     00   every other character is masked plainly
//	$'\M-x'  f8   $'\Mx'      f8   meta sets the high bit
//	$'\M-\t' 89   $'\M-\C-?'  ff   over whatever the argument came to
//	$'\C-\M-x' 98              and the mask keeps the high bit it finds
//	$'x\C'   78   $'x\M-'     78   an escape with no argument is nothing
//
// One measured spelling is not reproduced: `$'\C-\C-?'` is 7f on that shell
// where masking twice gives 1f, so its `?` rule survives a control it has
// already applied. A doubled control is written nowhere — the flag that
// writes these never nests one — and reproducing it would mean carrying the
// argument's *spelling* past the point it became a byte.
func caretMetaEscape(s string, i int) (byte, int, bool) {
	meta := s[i+1] == 'M'
	j := i + 2
	if j < len(s) && s[j] == '-' {
		j++
	}
	x, next, ok := caretMetaArgument(s, j)
	if !ok {
		return 0, j, false
	}
	if meta {
		return x | 0x80, next, true
	}
	if x == '?' {
		// The one character the mask is not applied to, and it is the byte
		// exactly rather than its low seven bits: measured, `$'\C-\x3f'` is
		// 7f like `$'\C-?'`, where `$'\C-\M-?'` is 9f and not ff — the meta
		// bit takes the argument out of the rule rather than riding through
		// it.
		return 0x7f, next, true
	}
	// 0x9f and not 0x1f: the high bit is kept, so a meta byte stays one.
	return x & 0x9f, next, true
}

// caretMetaArgument reads the one character, or escape, that a `\C-` or
// `\M-` controls.
//
// It is this file's own reading rather than controlArgument's, because that
// one answers for `\c` and carries three shells' policies for it; the two
// agree on the ordinary escapes and only this one takes a nested `\C-`.
func caretMetaArgument(s string, i int) (byte, int, bool) {
	switch {
	case i >= len(s):
		return 0, i, false
	case s[i] != '\\' || i+1 >= len(s):
		return s[i], i + 1, true
	}
	switch c := s[i+1]; {
	case c == 'C' || c == 'M':
		return caretMetaEscape(s, i)
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
	// An escape nothing claims loses its backslash, and what is controlled
	// is the character that survives.
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

// dollarSingleHexEveryDigit is whether a `\x` inside `$'…'` takes every
// hexadecimal digit that follows rather than stopping at two — see
// Semantics.DollarSingleHexReadsEveryDigit.
//
// Asked only where the two readings can differ, which is a run of three
// digits or more: `$'\x41'` and `$'\x4z'` are the same byte either way, and
// a `$'…'` with no long run in it puts no question to the dialect.
func (r *Runner) dollarSingleHexEveryDigit(digits string) bool {
	if len(digits) < 3 {
		return false
	}
	for i := range 3 {
		if digitValue(digits[i]) < 0 {
			return false
		}
	}
	return r.ask(r.sem().DollarSingleHexReadsEveryDigit,
		"a `\\x` escape reading past two hexadecimal digits")
}

// digitlessEscape writes what `\x`, `\u` or `\U` with no digit after it
// comes to, reporting whether decoding carries on.
//
// Two answers and they are a conflict: one keeps the two characters as they
// were written and the other reads a zero byte and goes on with the text —
// see Semantics.DollarSingleDigitlessEscapeIsAZeroByte. The zero goes through
// writeDecodedByte, so the shell that ends a span at a NUL ends it here too.
func (r *Runner) digitlessEscape(b *strings.Builder, written string) bool {
	if r.ask(r.sem().DollarSingleDigitlessEscapeIsAZeroByte,
		"a `\\x` escape with no hexadecimal digit after it") {
		return r.writeDecodedByte(b, 0)
	}
	b.WriteString(written)
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
// hexEscapeRun reads the digit run of a `\x`, under the two readings the
// panel has: two digits at most, or every digit that follows.
//
// One reader for the two sites that have the escape — a `printf` format and
// `$'…'` — because there is one hexadecimal escape and not two, which is the
// lesson #556 left. A run longer than two is a code point and the value is
// allowed to overflow: ksh93 keeps the low bits of a run past what an integer
// holds, so `$'\x41414141414141414141'` and `$'\x41414141'` are the same six
// bytes there.
func hexEscapeRun(digits string, everyDigit bool) (int, int) {
	if !everyDigit {
		return scanBase(digits, 16, 2)
	}
	return scanBase(digits, 16, len(digits))
}

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

// nestedWords expands the expansion standing where a parameter name would —
// the `${v}` of `${${v}#a}` — and reports whether it came to anything.
//
// It is an ordinary word expansion, deliberately: the inner expansion may be
// a parameter, a command substitution or an arithmetic one, may carry its own
// flags, and may itself be nested, and every one of those is already answered
// by the word pipeline. Reaching for the parameter machinery directly would
// have re-answered a subset of it.
//
// "Set" is whether the inner produced a field at all, which is what the outer
// `:-` and its family test. An inner naming nothing produces none, so
// `${${u}:-d}` substitutes and `${${v}:-d}` on an empty value does too — the
// colon's own rule, unchanged.
func (r *Runner) nestedWords(e *syntax.ParamExpr) (words []string, set, isList bool) {
	// Whatever this call comes to is held for the next reader of the same
	// node in the same span, whether it was expanded here or taken from the
	// hold. There is more than one reader — a conditional asks `testFires`
	// whether its test fires, the list path asks `listBase` whether the
	// inner is a list, and the scalar path asks for the value behind both —
	// and each of them wants what the inner came to rather than a fresh run
	// of it.
	//
	// **Putting it back is the half #1404 was missing**, and it is why this
	// is registered ahead of the read rather than behind it: the hold was
	// filled by the list path alone and emptied by whoever read it first, so
	// the *third* reader found nothing and expanded the inner again.
	// `${${$(cmd):-d}}` ran its command twice for that reason, and a run is
	// not recoverable from the value.
	defer func() { r.holdNested(e, words, set, isList) }()
	if words, set, isList, ok := r.takeNested(e); ok {
		// Already expanded for this span. Every answer is handed over,
		// list-ness included: the question is about the inner's shape, and
		// asking it again would mean expanding the inner again.
		return words, set, isList
	}
	if e.Inner == nil || len(e.Inner.Spans) == 0 {
		return []string{""}, false, false
	}
	if e.Index != nil {
		// `${${v}[2]}` subscripts what the inner came to, which is a second
		// question on top of this one and is answered in interp/nestedsub.go
		// — including which of the inner's shapes a subscript may follow at
		// all. The fields below are the whole of what it reads.
		return r.nestedSubscript(e)
	}
	words = r.nestedInnerFields(e)
	if len(words) == 0 {
		// No field is still a *value*: the empty string, and set. Measured —
		// `a=(); ${${a[@]}-d}` is empty in the shell with the grammar, where
		// `${nosuch-d}` is `d`, so the colon-less test finds something here
		// however little the inner came to.
		//
		// Here rather than in nestedInnerFields, because a subscript counts
		// what the inner came to and an empty list has no element to count:
		// `a=(); ${${a[@]}[(i)x]}` is 1, the position an append would take,
		// where a list holding one empty field would answer 2.
		//
		// Not a list: an empty inner leaves the outer `:-` to fire on the
		// scalar path, which is where `a=(); ${${a[@]}:-d}` already answers
		// `d`.
		return []string{""}, true, false
	}
	// An inner that came to a *list* keeps its fields, and the outer half
	// then applies to each — `${${a[@]}}` is one field per element,
	// `${${a[@]}#x}` trims every one and `${(j: :)${(qkv)m[@]}}` joins them
	// all. The list-ness is a question about the inner's *shape* and not
	// about how many fields it happened to produce: measured,
	// `a=(hello); ${#${a[@]}}` is 1, the element count, where `${#${s}}` on
	// the same five characters is 5. nestedResultIsAList is that question,
	// and it is the one a subscript on the same inner already asks.
	return words, true, r.nestedInnerIsAList(e, words)
}

// nestedInnerIsAList reports whether the fields the inner came to stand for a
// list, asked of the inner as it will have been expanded — its own span,
// carrying any quoting inherited from the expansion around it.
//
// One place, because the answer decides two different things about the same
// expansion — whether a quoted outer joins, and whether `${#…}` counts
// elements or characters — and a second copy is how those two would come to
// disagree.
func (r *Runner) nestedInnerIsAList(e *syntax.ParamExpr, words []string) bool {
	inner, _ := r.nestedInnerSpan(e)
	if inner.Kind != syntax.ParamExp || inner.Param == nil {
		// A command substitution or an arithmetic one in the name position
		// is a *list* there, however few words it came to, and the field
		// count cannot say so — which is the half #1394 turned on. Measured
		// on zsh 5.9.2, 2026-09-12: `${#$(echo abc)}` is 1 and not 3, and
		// `${#$((6*7))}` is 1 and not 2, so one word is a list of one and
		// not a string.
		//
		// Quoted it is a string, because the quotes joined its fields before
		// anything here saw them: `print -r -- "${#$(echo abc)}"` is 3.
		return inner.Quoting == syntax.Unquoted
	}
	return r.nestedResultIsAList(inner.Param, words, inner.Quoting != syntax.Unquoted)
}

// nestedHold is one nested expansion's fields, kept between the two halves of
// a single span's expansion.
//
// The node key and the clearing at the top of expandAt are the invariant
// rather than the cheapest test that passes, and mutation says so: neither is
// observable on its own today, because every route that fills a hold falls
// through to the scalar path in the very next call and empties it. They are
// written down anyway for the reason expandingQuoting keeps its own pair —
// what they prevent is a value from the wrong pass of a loop, which is
// exactly the silent kind of wrong.
//
// expandAtList asks whether a nested expansion came to a list, which it can
// only answer by expanding the inner; where the answer is no, the span falls
// through to the scalar path, which wants the very same fields. The inner
// must run once — `${${(f)$(cmd)}}` runs its command a single time in the
// shell with the grammar, and the fields it produced are not recoverable from
// anything else — so the first half hands them to the second rather than
// asking again.
//
// Keyed on the node, so a hold left by one expansion cannot be read by
// another, and cleared both when it is read and at the top of every expandAt.
type nestedHold struct {
	node   *syntax.ParamExpr
	words  []string
	set    bool
	isList bool
	held   bool
}

// holdNested keeps a nested expansion's fields for the next reader of the
// same node in the same span.
//
// Every expansion of an inner leaves one, which is the half #1404 was
// missing. The hold was filled only by the list path, so the *earlier* reader
// — a conditional asking `testFires` whether its test fires — expanded the
// inner, threw the fields away and left the list path to run it again.
// `${${$(cmd):-d}}` ran its command twice for that reason, and a run is not
// recoverable from the value: a command substitution writes files, moves a
// counter and takes time.
func (r *Runner) holdNested(e *syntax.ParamExpr, words []string, set, isList bool) {
	r.nestedHeld = nestedHold{node: e, words: words, set: set, isList: isList, held: true}
}

// takeNested is the held fields for this node, once. Reading empties the
// hold: the handoff is between readers of one span, and each of them asks
// once, so a hold read twice with nothing put back would be a different
// expansion. Every reader goes through nestedWords, which fills it again.
func (r *Runner) takeNested(e *syntax.ParamExpr) ([]string, bool, bool, bool) {
	if !r.nestedHeld.held || r.nestedHeld.node != e {
		return nil, false, false, false
	}
	h := r.nestedHeld
	r.nestedHeld = nestedHold{}
	return h.words, h.set, h.isList, true
}

// expandingQuoting is how *this* expansion was written, where it stands in a
// word being expanded.
//
// A nested expansion's inner is expanded by the code that owns the inner
// word, which has no way to see the quotes around the whole thing — and
// quoting is exactly what decides whether an inner that came to a list joins.
// The span is already tracked, for the same reason `${(q)…}` needs it: see
// inWord, which saves and restores both halves.
//
// The node has to be the one that span holds, and the identity check is the
// point rather than a guard: a command substitution runs a whole program
// while the word around it is still the one being expanded, so a nested
// expansion reached from inside it — in a here-document body, say — would
// otherwise inherit the quoting of a word it is not in. Unquoted for
// everything that did not come through a word, which is what the routes
// with no word around them had before.
//
// Both halves are belt and braces, and neither is observable on its own
// today: every route into a nested expansion either runs inside the word
// that holds it or arrives with no word at all, and the kind test alone
// turns away the one shape that could carry a stale index — a substitution
// span, which is what a here-document inside a word is reached through. They
// are written as the invariant rather than as the cheapest test that passes,
// because the failure they prevent is silent: a value joined on IFS where
// the script asked for a field.
func (r *Runner) expandingQuoting(e *syntax.ParamExpr) syntax.Quoting {
	if r.expandingWord == nil || r.expandingSpan >= len(r.expandingWord.Spans) {
		return syntax.Unquoted
	}
	s := r.expandingWord.Spans[r.expandingSpan]
	if s.Kind != syntax.ParamExp || s.Param != e {
		return syntax.Unquoted
	}
	return s.Quoting
}

// nestedInnerSpan is the inner substitution as it will be expanded: its own
// span, carrying the quoting of the expansion around it where it was written
// with none, and the splitting policy that quoting implies.
func (r *Runner) nestedInnerSpan(e *syntax.ParamExpr) (syntax.Span, splitPolicy) {
	span := e.Inner.Spans[0]
	if span.Quoting != syntax.Unquoted {
		// Written with quotes of its own, which is a different construct and
		// is why `${(@f)"$(cmd)"}` differs from the same characters without
		// them: quoted, the inner comes to one field and the flags split
		// that. Nothing to inherit.
		return span, splitNever
	}
	if q := r.expandingQuoting(e); q != syntax.Unquoted {
		span.Quoting = q
		return span, splitNever
	}
	return span, splitByDialect
}

// nestedInnerFields expands the inner and returns its fields, which is the
// whole of what both the plain shape and a subscript on it read. One place,
// so the two cannot expand it differently — or twice.
func (r *Runner) nestedInnerFields(e *syntax.ParamExpr) []string {
	var words []string
	// The inner is exactly one substitution span — the grammar admits nothing
	// else in that position — so this is expandOneWord's loop with the loop
	// taken out, and it keeps the fields that expandAt yields rather than
	// joining them the way expandWordNoSplit does. Whether those fields are
	// a list or one string is the caller's question: nestedWords refuses a
	// list and nestedResultIsAList decides it for a subscript.
	//
	// splitByDialect, the ordinary word's policy, because this position keeps
	// fields: a bare array name is the list here exactly as it is on a
	// command line, so `${${a}}` reaches the same answer `${${a[@]}}` does
	// rather than a joined string that looks like one field on purpose.
	//
	// Unless the expansion *around* it was quoted, in which case the inner
	// is expanded quoted as well and the fields it keeps are the ones
	// quoting keeps. Measured on zsh 5.9.2 with `a=(one two)`:
	//
	//	"${${a}}"       one two   the bare name joins, exactly as "$a" does
	//	"${${a}#o}"     ne two    so the operator sees the joined value
	//	"${#${a}}"      7         and a length measures it
	//	IFS=-; "${${a}[2]}"  -    the join is on IFS, not on a hard space
	//	"${${a[@]}[2]}" two       while `[@]` keeps its fields in quotes
	//
	// Which is not a rule of its own: it is what expandAt already does for
	// a quoted span, so the quoting is handed to it rather than reimplemented
	// here. An inner written with quotes of its own keeps them — that is a
	// different construct, and `${(@f)"$(cmd)"}` is the reason it exists.
	span, sp := r.nestedInnerSpan(e)
	defer r.inWord(e.Inner)()
	r.expandingSpan = 0
	// What follows is read by the operator around it and not by the command
	// line, which is the whole of what expandingNestedInner decides — see the
	// field, which carries the measurement.
	prevInner := r.expandingNestedInner
	r.expandingNestedInner = true
	defer func() { r.expandingNestedInner = prevInner }()
	if parts, ok := r.expandAt(span, sp, true); ok {
		words = parts
	} else {
		text, split := r.expandSpan(span, sp, true)
		words = r.nestedInnerSplit(span, text, split)
	}
	// The marks come off once, whichever half produced the fields. The inner
	// is an operand rather than a field of the command line, so a `*` in its
	// value is a character the outer operator matches against and not a
	// pattern the shell is about to escape for someone: leaving them on
	// answered `${${v}}` on `a*b` with a backslash in it.
	return unescapeAll(words)
}

// nestedInnerSplit is the field splitting an inner substitution's *text* is
// subject to, which is the one thing the name position gets from the word
// around it.
//
// The position holds fields — a bare array name in it is the elements, not
// the join — and a substitution written there without quotes is a
// substitution like any other, so what it comes to is split on `$IFS` before
// the outer half ever sees it. Measured 2026-09-10 on zsh 5.9.2, where
// `printf` is the instrument and `echo` cannot see the difference:
//
//	printf "[%s]" ${$(printf "a b")}          [a][b]
//	printf "[%s]" "${$(printf "a b")}"        [a b]
//	f(){ print "b b"; print "a a"; print c; }
//	a=( ${(o)$(f)} ); print $#a               5
//	a=( ${(oj:-:)$(f)} ); print -r -- $a      b-b-a-a-c
//
// so the flags are handed five fields and not one string holding newlines —
// which is what `(o)` sorts and `(j)` joins. A shell that hands them one
// field sorts nothing and joins nothing, and every one of those spellings is
// then the value it started with. That is what left a completion dump's
// `autoload` line with no names on it (#1697): the line is built from
// `$^fpath/(${(o~j.|.)$(typeset +fm '_*')})(N:t)`, whose alternation is the
// join, and one field holding newlines is a pattern that matches no file.
//
// The quoted spelling is a different program and is already right: an inner
// with quotes of its own comes to one field, which is what
// nestedInnerSpan's splitNever says and what `${(@f)"$(cmd)"}` exists for.
// So the policy is the one that reached here rather than a fresh decision,
// and `split` is what expandSpan reports about *this* span — the axis for a
// command substitution and the axis for a parameter, asked where they
// differ rather than assumed to agree.

// A length does not change any of that, and the claim that it did was a
// measurement taken in one context and written down as a rule (#1703). With
// `f(){ printf "b  b\na a\nc\n"; }` — ten characters, five fields, nine
// once joined — measured again on zsh 5.9.2, 2026-09-12:
//
//	print -r -- "${#${(o)$(f)}}"   10   quoted: unsplit, so `(o)` sorts one
//	x=${#${(o)$(f)}}               5    unquoted: five fields, counted
//	printf '[%s]' ${#$(f)}         [5]  and the plain shape agrees
//
// So the split follows the quoting under a length exactly as it does without
// one, and there is nothing here for a length to say.
func (r *Runner) nestedInnerSplit(span syntax.Span, text string, split bool) []string {
	if split {
		ifs, set := r.ifs()
		return r.splitFieldsAsk(text, ifs, set)
	}
	if text == "" && span.Quoting == syntax.Unquoted && span.Kind != syntax.ParamExp {
		// An unquoted substitution that came to nothing is no field, the way
		// one on a command line is — and `split` cannot say so, because a
		// result with no separator in it is reported unsplit whether it is
		// empty or a word. Measured on zsh 5.9.2, 2026-09-12:
		// `printf '[%s]' ${#$(true)[@]}` is `[0]` and `${#$(true)}` is `[0]`,
		// where one empty field would have answered 1 to both.
		//
		// A *parameter* inner is left alone: an empty scalar there is one
		// field, and `s=''; ${#${s}[@]}` is 0 through the shape question
		// rather than through the field count.
		return nil
	}
	return []string{text}
}

// unescapeAll takes the glob marks off every field, for a caller that wants
// the text rather than a pattern.
func unescapeAll(fields []string) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = globUnescape(f)
	}
	return out
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"strconv"
	"strings"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
)

// The context a completion widget's function runs in: the parameters it reads
// the word out of, and the state `compadd` and `compset` move around.
//
// # What this is for
//
// `zle -C name completer function` makes two claims about a key (see
// bindkey.go). Until #2776 this shell kept the first and dropped the second:
// a key bound to a completion widget ran the editor's own completion and the
// widget's function was never called, because the function's whole vocabulary
// — `compadd`, `compset`, `$compstate`, `$words`, `$PREFIX` — did not exist.
// That kept Tab working on a real `~/.zshrc` (#2770) and it left the rc's own
// completions unrun.
//
// This is the second claim, kept. The function runs, what it collects with
// `compadd` is what the key offers, and **a function that offers nothing
// leaves the editor completing exactly what it completed before** — see
// repl.Binding.Candidates, where that ordering is the rule rather than a
// fallback. So the failure #2770 was filed for cannot come back through here:
// the worst a broken completion function can do is cost one call.
//
// # What a function sees, measured
//
// Measured 2026-09-15 through a pseudo-terminal against zsh 5.9.2 with
// `compinit` run, a `zle -C probewid .complete-word _probe` on a key, and
// `_probe` printing its parameters. Typing `git che` and pressing the key:
//
//	words=(git che)  CURRENT=2
//	PREFIX=[che] SUFFIX=[] IPREFIX=[] ISUFFIX=[] QIPREFIX=[] QISUFFIX=[]
//	compstate[context]=command      compstate[nmatches]=0
//	compstate[insert]=automenu-unambiguous
//	compstate[list]=ambiguous       compstate[to_end]=match
//	compstate[all_quotes]=\         compstate[restore]=auto
//	compstate[ignored]=0            compstate[last_prompt]=yes
//	compstate[list_max]=100         compstate[list_lines]=0
//	compstate[unambiguous_cursor]=1 compstate[pattern_insert]=menu
//
// Three of those readings are worth keeping because they are the ones a
// guess gets wrong:
//
//   - `context` is `command` for an argument as well as for the command word.
//     `echo fo` and `git che` both report `command`; it is not the position
//     of the word.
//   - `words` holds the words **as they were typed, quotes included** —
//     `echo "fo` gives `words=(echo "fo)` — while `PREFIX` is the word with
//     the opening quote taken off and `QIPREFIX` is the quote. `compstate`
//     then carries `quote=" quoting=double`.
//   - A trailing space is a word. `git ` with the cursor after it reports
//     `words=(git )` — two words, the second empty — and `CURRENT=2`.
//
// # SUFFIX is empty here, and that is this editor rather than this file
//
// zsh splits the word at the cursor: `PREFIX` is what is before it and
// `SUFFIX` what is after. This editor completes the text before the cursor
// and replaces exactly that — repl's `Completion` says where the word starts
// and where the cursor is, and nothing about where the word ends. So `SUFFIX`
// and `ISUFFIX` are empty here, and `compset -s` and `compset -S` answer 1
// as they do in zsh when there is nothing after the cursor. A function that
// completes in the middle of a word gets the same answer it would get with
// the cursor at the end of it.

// completionState is one completion in flight: everything the parameters read
// and write, and what `compadd` has collected so far.
//
// Held on the context rather than on the Runner, for editoractions.go's
// reason: the dynamic extent of one call is exactly how long it is good for,
// and a `compadd` reached anywhere else — in a script, in a hook, at a prompt
// — finds no state, and that *is* the refusal.
type completionState struct {
	// The word, split the way zsh splits it. prefix and suffix are the two
	// sides of the cursor; iprefix and isuffix are what `compset` has moved
	// out of them, still on the line and not part of what is being matched.
	iprefix, prefix, suffix, isuffix string

	// qiprefix and qisuffix are the quoting the word was typed inside, taken
	// off the front of prefix — an opening `"` lives here and not in PREFIX.
	qiprefix, qisuffix string

	// words is the command line as words, as typed, and current is the
	// one-based index of the word being completed. Both are `compset -n`'s
	// and `compset -q`'s to change.
	words   []string
	current int

	// state is `$compstate`, whole. A map rather than fields because a
	// completion function assigns to keys this shell has no opinion about
	// and expects to read them back.
	state map[string]string

	// matches is what `compadd` has collected: whole replacement words in the
	// line's own quoting, each with the row a listing draws for it and the
	// block it is drawn in, which is what repl's completion seam is answered
	// with.
	matches []repl.Candidate

	// groups is the headings each block has collected, keyed by the block
	// itself with its heading left empty — see compadd.go's group(), which
	// carries the measurement for why a heading cannot identify a block.
	groups map[repl.Group][]string

	// groupOrder is the order `compgroups` declared the blocks in, by name.
	// Empty is a completion that never called it, which is one where the
	// blocks are drawn in the order they were added.
	groupOrder []string

	// computil is what the eight `zsh/computil` builtins keep for the length
	// of this one completion — the parsed `_arguments` specs, the tag loop,
	// and the rest. Made on first use, because most completions never reach
	// any of them. See computil.go.
	computil *computilState

	// c is the question the editor asked, kept for the quoting rule — how a
	// name is escaped depends on the quotation the word is already inside,
	// and that is a fact about this Completion.
	c repl.Completion
}

// The parameter names a completion widget's function reads and writes. Named
// once here because three files spell them and a typo in one of them is a
// parameter that silently reads empty.
const (
	compPrefix   = "PREFIX"
	compSuffix   = "SUFFIX"
	compIPrefix  = "IPREFIX"
	compISuffix  = "ISUFFIX"
	compQIPrefix = "QIPREFIX"
	compQISuffix = "QISUFFIX"
	compWords    = "words"
	compCurrent  = "CURRENT"
	compState    = "compstate"
)

// completionKey is the context key the state rides on, unexported and of an
// unexported type so nothing outside this package can put one there.
type completionKey struct{}

func withCompletion(ctx context.Context, cs *completionState) context.Context {
	return context.WithValue(ctx, completionKey{}, cs)
}

// completionFrom is the state of the completion being performed now, and
// false where nothing is being completed.
//
// The `false` is what `compadd` and `compset` refuse on, and it is the same
// refusal `zle` outside a widget gives: measured on zsh 5.9.2, `compadd x`
// on a `-c` line is `compadd: can only be called from completion function`
// at status 1, and `compset -p 1` says the same of itself.
func completionFrom(ctx context.Context) (*completionState, bool) {
	cs, ok := ctx.Value(completionKey{}).(*completionState)
	return cs, ok && cs != nil
}

// RunCompletion asks this shell's completion system what the word under the
// cursor could become — the driver seam a key's binding names, and the whole
// of #2776's user-visible half.
//
// name is the widget the key was bound to, carried through
// repl.Binding.Candidates by bindkey.go. A name that is not a `zle -C` widget,
// or whose function is not defined, answers nothing — which the editor reads
// as "no opinion about this word" and completes its own way.
func RunCompletion(
	r *interp.Runner, ctx context.Context, name string, c repl.Completion,
) []repl.Candidate {
	def, defined := widgetDefinitionOf(r, name)
	if !defined || def.completer == "" || !r.HasFunction(def.function) {
		return nil
	}
	cs := newCompletionState(c)
	openCompletionParameters(r, cs)
	defer closeCompletionParameters(r)
	// The status goes in and does not come out, which is runWidgetFunction's
	// discipline and is right for the same reason: a Tab must not be what the
	// next `&&` reads.
	status := r.ExitStatus()
	// **With no arguments.** Measured on zsh 5.9.2, 2026-09-15, through a
	// pseudo-terminal: a `zle -C wtest .complete-word _f` whose `_f` prints
	// `$#` reports 0. This used to pass the widget's name, which no caller
	// could see until a *shipped* completion function ran — `_main_complete`
	// takes the completers to try as its arguments, so a widget name arrived
	// as one and came back as `command not found: expand-or-complete`. The
	// name is still reachable, as `$WIDGET`.
	_, err := r.CallFunction(withCompletion(ctx, cs), def.function)
	r.SetExitStatus(status)
	if err != nil {
		return nil
	}
	return cs.groupedMatches()
}

// newCompletionState splits the word the editor asked about the way zsh
// splits it, and seeds `$compstate` with the values a fresh completion has.
func newCompletionState(c repl.Completion) *completionState {
	cs := &completionState{c: c, state: freshCompstate(c), groups: map[repl.Group][]string{}}
	// The opening quote, if the word is inside one, is QIPREFIX and not part
	// of PREFIX — measured, `echo "fo` reports `PREFIX=fo QIPREFIX="`.
	word := c.Word
	if len(word) > 0 && (word[0] == '"' || word[0] == '\'') {
		cs.qiprefix, word = word[:1], word[1:]
	}
	cs.prefix = word
	cs.words, cs.current = completionWords(c)
	return cs
}

// freshCompstate is `$compstate` as a completion widget finds it, from the
// measurement in the file comment.
//
// The keys whose value is empty in a fresh completion — `quote`, `quoting`,
// `parameter`, `redirect`, `exact`, `pattern_match`, `old_list`,
// `unambiguous` — are absent rather than empty, which is what the measured
// `${(k)compstate}` listing shows, and reading one is the empty string
// either way.
func freshCompstate(c repl.Completion) map[string]string {
	st := map[string]string{
		// Measured: `command` for an argument as well as for the command
		// word, so this is not c.Command restated.
		"context":            "command",
		"nmatches":           "0",
		"insert":             "automenu-unambiguous",
		"list":               "ambiguous",
		"to_end":             "match",
		"all_quotes":         `\`,
		"restore":            "auto",
		"ignored":            "0",
		"last_prompt":        "yes",
		"list_max":           "100",
		"list_lines":         "0",
		"unambiguous_cursor": "1",
		"pattern_insert":     "menu",
		"insert_positions":   "",
		"vared":              "",
	}
	if quote := wordOpeningQuote(c.Word); quote != "" {
		st["quote"] = quote
		st["quoting"] = map[string]string{`"`: "double", "'": "single"}[quote]
	}
	return st
}

// wordOpeningQuote is the quotation the word being completed is inside, or
// empty where it is inside none. The opening quote is part of the word and
// stays where it was typed — see docs/spec/completion.md.
func wordOpeningQuote(word string) string {
	if len(word) > 0 && (word[0] == '"' || word[0] == '\'') {
		return word[:1]
	}
	return ""
}

// completionWords is `$words` and `$CURRENT`: the line as words, as typed,
// and which of them the cursor is in.
//
// Split at unquoted blanks and nowhere else, which is the word boundary
// docs/spec/completion.md measured for this editor — not bash's readline set,
// which breaks `--opt=value` in the middle. The current word is whatever the
// editor said it was, so the two can never disagree about where it starts.
//
// A trailing blank makes an empty last word rather than no word, which is
// measured: `git ` reports `words=(git )` and `CURRENT=2`.
func completionWords(c repl.Completion) ([]string, int) {
	before := splitCompletionWords(c.Line[:c.Start])
	words := append(before, c.Word)
	// Whatever is past the cursor is on the line too, and a completion
	// function that looks at `$words[-1]` is looking at it.
	words = append(words, splitCompletionWords(c.Line[min(c.Point, len(c.Line)):])...)
	return words, len(before) + 1
}

// splitCompletionWords breaks text at blanks a backslash or a quotation does
// not protect, and hands back the words with their quoting still on them.
func splitCompletionWords(text string) []string {
	var out []string
	var cur strings.Builder
	var quote byte
	started := false
	for i := 0; i < len(text); i++ {
		ch := text[i]
		switch {
		case quote == 0 && ch == '\\' && i+1 < len(text):
			cur.WriteByte(ch)
			i++
			cur.WriteByte(text[i])
			started = true
		case quote == 0 && (ch == '"' || ch == '\''):
			quote = ch
			cur.WriteByte(ch)
			started = true
		case quote != 0 && ch == quote:
			quote = 0
			cur.WriteByte(ch)
		case quote == 0 && (ch == ' ' || ch == '\t'):
			if started {
				out = append(out, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteByte(ch)
			started = true
		}
	}
	if started {
		out = append(out, cur.String())
	}
	return out
}

// openCompletionParameters publishes the nine parameters a completion widget's
// function reads, produced rather than stored so that what `compset` moves is
// live in the next read — the same rule openWidgetParameters follows, and for
// the same measured reason.
//
// Every one of them is writable, because a completion function assigns to
// them: `_arguments` rewrites `$words` and `$CURRENT`, `_normal` writes
// `$PREFIX`, and `$compstate` is how a widget asks for a listing rather than
// an insertion.
func openCompletionParameters(r *interp.Runner, cs *completionState) {
	scalars := []struct {
		name string
		at   *string
	}{
		{compPrefix, &cs.prefix},
		{compSuffix, &cs.suffix},
		{compIPrefix, &cs.iprefix},
		{compISuffix, &cs.isuffix},
		{compQIPrefix, &cs.qiprefix},
		{compQISuffix, &cs.qisuffix},
	}
	for _, s := range scalars {
		at := s.at
		r.SetDynamic(s.name, func(*interp.Runner) string { return *at })
		r.SetDynamicWriter(s.name, func(_ *interp.Runner, value string) { *at = value })
	}
	r.SetDynamic(compCurrent, func(*interp.Runner) string { return strconv.Itoa(cs.current) })
	r.SetDynamicWriter(compCurrent, func(_ *interp.Runner, value string) {
		if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			cs.current = n
		}
	})
	r.SetDynamicArray(compWords, func(*interp.Runner) []string { return cs.words })
	r.SetDynamicArrayWriter(compWords, func(_ *interp.Runner, values []string) {
		cs.words = values
	})
	r.SetDynamicAssoc(compState, func(*interp.Runner) interp.AssocArray {
		out := make(interp.AssocArray, len(cs.state))
		for k, v := range cs.state {
			out[k] = interp.Scalar(v)
		}
		return out
	})
	r.SetDynamicAssocWriter(compState, func(_ *interp.Runner, key, value string, set bool) {
		if !set {
			delete(cs.state, key)
			return
		}
		cs.state[key] = value
	})
}

// closeCompletionParameters takes all nine away again.
//
// Deferred by the caller and not called straight-line, for the reason
// runWidgetFunction's close is deferred: a panic in the function is caught
// outside this call, so a parameter left behind would be found by a script at
// the next prompt — and `$PREFIX` standing at a prompt is the kind of wrong
// nobody would connect to a crash minutes earlier.
func closeCompletionParameters(r *interp.Runner) {
	for _, name := range []string{
		compPrefix, compSuffix, compIPrefix, compISuffix,
		compQIPrefix, compQISuffix, compCurrent, compWords, compState,
	} {
		r.UnsetDynamic(name)
	}
}

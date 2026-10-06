// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"

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
// This is the second claim, kept. The function runs, and what it collects
// with `compadd` is what the key offers. **A function that offers nothing
// offers nothing**: zsh rings and leaves the line, and so does this
// (repl.CompletionOfferedNothing, #6214). Until the shipped completion
// system ran here, such a function handed the key to the editor's own
// completion instead — the ordering repl.Binding.Candidates still states
// for an action with no opinion — and that turned `cd ` and Tab in a
// directory with no subdirectory into `cd a`. The failure #2770 was filed
// for still cannot come back through here: a function that stops on an
// error is repl.CompletionStopped, and costs one call.
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

	// keepingAll is the blocks a `compadd -2` named, by name: the ones an
	// `-E` filler joins whichever of `-J` and `-V` it was itself given. See
	// compadd.go's fillerGroup.
	keepingAll map[string]repl.Group

	// fillers is how many `-E` cells have been added, which
	// `$compstate[nmatches]` counts as matches although nothing inserts
	// them. See compadd.go's addFillers.
	fillers int

	// packedGroups is the blocks a `compadd` reached while
	// `$compstate[list]` held `packed`. See compadd.go's packGroup.
	packedGroups map[repl.Group]bool

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
// repl.Binding.Candidates by bindkey.go. A name that is not a `zle -C` widget
// answers nothing — which the editor reads as "no opinion about this word"
// and completes its own way. One whose function is not defined, or whose
// function offers no match, answers repl.CompletionOfferedNothing, which
// zsh's answer is too.
func RunCompletion(
	r *interp.Runner, ctx context.Context, name string, c repl.Completion,
) []repl.Candidate {
	def, defined := widgetDefinitionOf(r, name)
	if !defined || def.completer == "" {
		return nil
	}
	if !r.HasFunction(def.function) {
		// A completion widget whose function is not there completes
		// nothing, and says so with the bell — measured, the same as one
		// that adds nothing. See repl.CompletionOfferedNothing.
		return repl.CompletionOfferedNothing()
	}
	cs := newCompletionState(c)
	entry := insertOnEntry(r, c.Menu)
	cs.state["insert"] = entry
	expandCommandAlias(r, cs)
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
	//
	// Through runWidgetFunction, so the function has the widget parameters a
	// key's widget has, read-only as a completion widget's are. Measured
	// 2026-10-04 through a pseudo-terminal against zsh 5.9.2, with `cf()
	// { print "W=$WIDGET t=${(t)BUFFER}"; BUFFER=zz; ran=widget }` behind
	// `zle -C cw complete-word cf` on a key: `W=cw
	// t=scalar-local-readonly-special`, then `read-only variable: BUFFER`,
	// the function stops there, and the line is kept. This called the
	// function bare, so `$WIDGET` was empty, `BUFFER=zz` was an ordinary
	// assignment, and the function ran on (#5999).
	in := repl.Line{Buffer: c.Line, Cursor: utf8.RuneCountInString(c.Line[:min(max(c.Point, 0), len(c.Line))])}
	_, ran, stopped := runWidgetCall(r, withCompletion(ctx, cs), name, in)
	r.SetExitStatus(status)
	if stopped {
		// Stopped on an error: no matches, and not "nothing to say" either,
		// so the editor completes nothing of its own. See
		// repl.Shell.RunCompletion (#6068).
		return repl.CompletionStopped()
	}
	if !ran {
		return repl.CompletionOfferedNothing()
	}
	if strings.Contains(cs.state["insert"], "tab") {
		// The function asked for the key to be typed instead: `tab`
		// anywhere in `compstate[insert]`. Measured 2026-10-05 against zsh
		// 5.9.2 with a completion widget setting it: `tab`, `tabx`, `xtab`
		// and `automenu tab` all type the key, `ta`, `menu` and `automenu`
		// do not, and matches it added make no difference. It is how the
		// completion system's `insert-tab` style types a Tab on an empty
		// line, which here went on to complete every command there is and
		// ask whether to list them all (#6119). See
		// repl.CompletionInsertsTheKey.
		return repl.CompletionInsertsTheKey()
	}
	matches := cs.groupedMatches()
	if len(matches) == 0 {
		// The function ran and offered nothing, which is its answer: zsh
		// leaves the line and rings, where handing the key to this
		// editor's own completion put a prefix of whatever files there
		// were on the line — `cd ` and Tab became `cd a` in a directory
		// with no subdirectory (#6214). The fallback dates from before the
		// completion system could run here (#2770); a function that cannot
		// run is CompletionStopped above, and still costs no more than one
		// call.
		return repl.CompletionOfferedNothing()
	}
	if insert := cs.state["insert"]; insert != entry {
		// Left as it was found, the value says what the editor was going
		// to do anyway, so only a function that changed it has anything
		// to tell it.
		return insertAsAsked(insert, matches)
	}
	return matches
}

// insertOnEntry is `compstate[insert]` as a completion function finds it: how
// the editor means to put an ambiguous answer in the line, in zshcompwid's
// words for it.
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, a `zle -C`
// function printing the value on entry, over `always` and `auto`:
//
//	first Tab                                   automenu-unambiguous
//	the Tab after it listed                     automenu
//	first Tab, unsetopt automenu                unambiguous, and again after
//	first Tab, setopt menucomplete              menu
//	a menu-complete or reverse-menu-complete    menu
//	a menu-complete after a Tab listed          automenu
//
// and a function is not called at all for the Tabs that walk a menu once it
// has started.
func insertOnEntry(r *interp.Runner, menu repl.MenuReason) string {
	switch menu {
	case repl.MenuOnARepeat:
		return "automenu"
	case repl.MenuAsked:
		return "menu"
	}
	if on, _ := conditionOption(r, "automenu"); on {
		return "automenu-unambiguous"
	}
	return "unambiguous"
}

// insertAsAsked is the matches with the function's last word on how they go
// in: `compstate[insert]` as it stands when the function returns.
//
// `menu` and `automenu` start a menu completion, at the match a `:N` after
// either names; `unambiguous` and `automenu-unambiguous` put in what the
// matches agree on and start none, whatever the key would have done — which
// is how the completion system's `menu` style reaches the line. Measured
// 2026-10-06 against zsh 5.9.2 with `compinit` and `x a` over `always` and
// `auto`, the line after one, two and three Tabs:
//
//	zstyle ':completion:*' menu …   1 Tab      2 Tabs     3 Tabs
//	(not set)                       x a        x always   x auto
//	yes, true, yes select           x always   x auto     x always
//	select, select=2                x a        x always   x auto
//	no                              x a        x a        x a
//
// `select` asks for the interactive selection of zsh/complist, which this
// shell does not have (#5761); the walk is what it comes to here, and the
// line reads as it does in zsh after each of those Tabs.
//
// Anything else — a number, `all`, an empty value — is not drawn here, and
// the editor answers as it would have.
func insertAsAsked(insert string, matches []repl.Candidate) []repl.Candidate {
	if len(matches) == 0 {
		return matches
	}
	mode, number, _ := strings.Cut(insert, ":")
	switch mode {
	case "menu", "automenu":
		at, _ := strconv.Atoi(number)
		return repl.MenuCompletion(matches, at)
	case "unambiguous", "automenu-unambiguous":
		return repl.PrefixCompletion(matches)
	}
	return matches
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
	switch cs.qiprefix {
	case "":
		word = prefixAsTyped(word)
	case `"`:
		word = prefixInDoubleQuotes(word)
	}
	cs.prefix = word
	cs.words, cs.current = completionWords(c)
	return cs
}

// prefixAsTyped is `$PREFIX` for a word typed outside any quotes: the word
// with every backslash that quotes nothing taken off, and the ones that quote
// something kept (#6224).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, a `zle -C`
// widget writing `$PREFIX` to a file, typing `x \Ca` for every printable
// character C that is not a letter or digit, and some that are:
//
//	kept      \^ \* \~ \' \" \= \# \[ \] \? \{ \} \( \) \< \> \| \& \; \$ \` \<sp> \\
//	dropped   \a \b \z \A \0 \9 \- \_ \. \, \/ \: \@ \% \+ \!
//
// and a backslash with nothing after it is dropped: `a\` is `a`, `\` alone
// is empty, and `\\\a` is `\\a`. So what is kept is a backslash before a
// character the shell would otherwise read as something, which is why `$PREFIX`
// still has to be unquoted before it is matched — see typedForMatching.
func prefixAsTyped(word string) string {
	if !strings.Contains(word, "\\") {
		return word
	}
	var b strings.Builder
	r := []rune(word)
	for i := 0; i < len(r); i++ {
		if r[i] != '\\' {
			b.WriteRune(r[i])
			continue
		}
		if i+1 == len(r) {
			break
		}
		i++
		if r[i] == '\\' || strings.ContainsRune(prefixQuotedSpecials, r[i]) {
			b.WriteByte('\\')
		}
		b.WriteRune(r[i])
	}
	return b.String()
}

// prefixInDoubleQuotes is `$PREFIX` for a word typed inside double quotes:
// a backslash that quotes something there — before `$`, a backquote, `"` or
// `\` — is kept as it is, and one that quotes nothing is a backslash in the
// name, which `$PREFIX` spells quoted as `\\` (#6224). Measured 2026-10-06
// against zsh 5.9.2 as prefixAsTyped is:
//
//	typed     "\a     "\*a     "\ a     "a\      "\      "\$a    "\"a    "\\a    "\\\a
//	$PREFIX   \\a     \\*a     \\ a     a\\      \\      \$a     \"a     \\a     \\\\a
//
// Inside single quotes nothing quotes anything and `$PREFIX` is the word as
// typed: `'\a` is `\a`.
func prefixInDoubleQuotes(word string) string {
	if !strings.Contains(word, `\`) {
		return word
	}
	var b strings.Builder
	r := []rune(word)
	for i := 0; i < len(r); i++ {
		if r[i] != '\\' {
			b.WriteRune(r[i])
			continue
		}
		if i+1 < len(r) && strings.ContainsRune("$`\"\\", r[i+1]) {
			b.WriteRune('\\')
			b.WriteRune(r[i+1])
			i++
			continue
		}
		b.WriteString(`\\`)
	}
	return b.String()
}

// prefixQuotedSpecials are the characters a backslash before them is kept for
// in `$PREFIX`. See prefixAsTyped for the measurement.
const prefixQuotedSpecials = "^*~'\"=#[]?{}()<>|&;$` "

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

// completionWords is `$words` and `$CURRENT`: the words of the command the
// cursor is in, as typed, and which of them the cursor is in.
//
// **The command the cursor is in, not the line** (#6174). Measured
// 2026-10-05 against zsh 5.9.2 through a pseudo-terminal, a `zle -C` widget
// writing `$words` and `$CURRENT` to a file:
//
//	echo a | grep -          (grep -) 2
//	true && grep -           (grep -) 2
//	(grep -    echo $(grep -    echo `grep -     (grep -) 2
//	for i in a b; do grep -  (grep -) 2
//	if grep -    { grep -    ! grep -           (grep -) 2
//	x=1 grep -               (grep -) 2
//	grep a > out -           (grep a -) 3
//	grep a 2>/dev/null -     (grep a -) 3
//	grep -a<cursor>; echo b  (grep -a) 2
//
// So a separator — `;` `&` `|` `(` `)` a newline, `$(` and a backquote —
// starts the words afresh; at the start of a command a reserved word and an
// assignment are not among them; a redirection is not, nor the word it
// takes when it is written apart; and the words after the cursor stop at the
// next separator. This took every word on the line, so `ps aux | grep -<TAB>`
// was a completion for `ps` (`$words[1]` is how the completion system picks
// the command's completion).
//
// Split at unquoted blanks within a command, which is the word boundary
// docs/spec/completion.md measured for this editor — not bash's readline set,
// which breaks `--opt=value` in the middle. The current word is whatever the
// editor said it was, so the two can never disagree about where it starts.
//
// A trailing blank makes an empty last word rather than no word, which is
// measured: `git ` reports `words=(git )` and `CURRENT=2`.
func completionWords(c repl.Completion) ([]string, int) {
	before := commandWordsBefore(c.Line[:c.Start])
	words := append(before, c.Word)
	// Whatever is past the cursor is on the line too, and a completion
	// function that looks at `$words[-1]` is looking at it — up to the end
	// of this command.
	words = append(words, commandWordsAfter(c.Line[min(c.Point, len(c.Line)):])...)
	return words, len(before) + 1
}

// expandCommandAlias puts an alias in command position into `$words` as what
// it stands for, unless COMPLETE_ALIASES is set or the cursor is in that
// word. Measured 2026-10-05 against zsh 5.9.2 (#6174, #6154): with
// `alias ll='gls -h --x'`, `ll -<TAB>` is `words=(gls -h --x -)` and
// `CURRENT=4`, and under `setopt completealiases` it stays `(ll -)`. It is how
// zsh completes `ls -` as GNU `gls` where `ls` is an alias for it.
func expandCommandAlias(r *interp.Runner, cs *completionState) {
	if recordedDeviates(r, "completealiases") || cs.current < 2 || len(cs.words) == 0 {
		return
	}
	seen := map[string]bool{}
	for !seen[cs.words[0]] {
		value, ok := r.LookupAlias(cs.words[0])
		if !ok {
			return
		}
		seen[cs.words[0]] = true
		var expanded []string
		for _, tok := range splitCompletionTokens(value) {
			if !tok.sep {
				expanded = append(expanded, tok.text)
			}
		}
		if len(expanded) == 0 {
			return
		}
		cs.words = append(expanded, cs.words[1:]...)
		cs.current += len(expanded) - 1
	}
}

// completionToken is one word, or a separator between commands.
type completionToken struct {
	text string
	sep  bool
}

// splitCompletionTokens breaks text at blanks a backslash or a quotation does
// not protect, and at the characters that end a command, handing back the
// words with their quoting still on them.
func splitCompletionTokens(text string) []completionToken {
	var out []completionToken
	var cur strings.Builder
	var quote byte
	started := false
	flush := func() {
		if started {
			out = append(out, completionToken{text: cur.String()})
			cur.Reset()
			started = false
		}
	}
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
		case quote != 0:
			cur.WriteByte(ch)
		case ch == ' ' || ch == '\t':
			flush()
		case ch == '&' && !started && i+1 < len(text) && text[i+1] == '>':
			// `&>`, which is a redirection and not a separator.
			cur.WriteByte(ch)
			started = true
		case ch == '$' && i+1 < len(text) && text[i+1] == '(':
			flush()
			out = append(out, completionToken{sep: true})
			i++
		case strings.IndexByte(";&|()\n`", ch) >= 0:
			if (ch == '&' || ch == '|') && redirectionOperator(cur.String()+string(ch)) {
				// `>&` and `>|` are one operator, not a separator after `>`.
				cur.WriteByte(ch)
				continue
			}
			flush()
			out = append(out, completionToken{sep: true})
		default:
			cur.WriteByte(ch)
			started = true
		}
	}
	flush()
	return out
}

// commandWordsBefore is the words of the last command in text: everything
// since the last separator, less the reserved words and assignments at its
// start and the redirections anywhere in it.
func commandWordsBefore(text string) []string {
	var words []string
	atStart, skipNext := true, false
	for _, tok := range splitCompletionTokens(text) {
		if tok.sep {
			words, atStart, skipNext = nil, true, false
			continue
		}
		if skipNext {
			skipNext = false
			continue
		}
		if _, rest, ok := redirection(tok.text); ok {
			skipNext = rest == ""
			continue
		}
		if atStart && (completionReservedWords[tok.text] || isAssignmentWord(tok.text)) {
			continue
		}
		atStart = false
		words = append(words, tok.text)
	}
	return words
}

// commandWordsAfter is the words after the cursor up to the end of its
// command.
func commandWordsAfter(text string) []string {
	var words []string
	for _, tok := range splitCompletionTokens(text) {
		if tok.sep {
			break
		}
		words = append(words, tok.text)
	}
	return words
}

// completionReservedWords are the words that open a command rather than
// being one, at the start of a command.
var completionReservedWords = map[string]bool{
	"if": true, "then": true, "else": true, "elif": true, "while": true, "until": true,
	"do": true, "!": true, "{": true, "}": true, "time": true, "nocorrect": true,
}

// isAssignmentWord is `name=…` or `name+=…`, which a command starts with
// without it being one of the command's words.
func isAssignmentWord(w string) bool {
	eq := strings.IndexByte(w, '=')
	if eq <= 0 {
		return false
	}
	name := strings.TrimSuffix(w[:eq], "+")
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// redirection splits a word that is a redirection into its operator and the
// target written against it — `2>/dev/null` is `2>` and `/dev/null`, a bare
// `>` is `>` and nothing, so its target is the next word.
func redirection(w string) (op, rest string, ok bool) {
	i := 0
	for i < len(w) && w[i] >= '0' && w[i] <= '9' {
		i++
	}
	for _, o := range []string{"<<<", ">>|", "&>>", ">>", "<<", "<>", ">&", "<&", ">|", "&>", ">", "<"} {
		if strings.HasPrefix(w[i:], o) {
			return w[:i+len(o)], w[i+len(o):], true
		}
	}
	return "", "", false
}

// redirectionOperator reports whether w is, so far, a redirection operator
// and nothing else — which is when a following `&` or `|` belongs to it.
func redirectionOperator(w string) bool {
	op, rest, ok := redirection(w)
	return ok && rest == "" && op == w
}

// splitCompletionWords breaks text at blanks a backslash or a quotation does
// not protect, and nowhere else — a pattern's `|` and `(` are part of it, and hands back the words with their quoting still on them.
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

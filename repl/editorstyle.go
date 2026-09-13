// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// EditorStyle is what a dialect draws while a line is being typed, as opposed
// to what it draws in the prompt.
//
// Separate from PromptStyle because it answers a different question. A prompt
// is text the shell was given and transforms; this is text the shell adds of
// its own accord, and the dialects disagree about whether to add it at all.
type EditorStyle struct {
	// Interrupt is what marks a line abandoned with ^C, drawn where the
	// cursor was before the line ends.
	//
	// Two of the four draw `^C` and two draw nothing. Measured with the
	// output drained after every keystroke — without that the terminal
	// flushes what it has queued when the interrupt arrives, and the
	// measurement is of a screen that never existed:
	//
	//	bash    P> echo hi^C
	//	dash    P> echo hi^C
	//	zsh     P> echo hi
	//	ksh93   P> echo hi
	//
	// In dash it is not the shell's doing at all — dash has no line editor,
	// so the terminal echoes the interrupt character itself. A shell that
	// takes the terminal raw has to draw it deliberately or not, which is
	// what makes this a choice rather than something inherited.
	//
	// Empty draws nothing, which is two of the four dialects' answer and
	// also what a front end told nothing does.
	Interrupt string

	// ListQuery is what is asked before printing a large number of matches,
	// instead of printing them. Two verbs: how many matches there are, and
	// how many rows they would take.
	//
	//	bash   Display all 120 possibilities? (y or n)
	//	zsh    zsh: do you wish to see all 120 possibilities (10 lines)?
	//
	// One of them counts the rows and the other does not, which is why both
	// numbers are passed and a wording may ignore the second.
	//
	// Empty asks nothing and prints, which is what the two dialects with no
	// line editor of their own have no answer about, and what a front end
	// told nothing does.
	ListQuery string

	// ListQueryEchoesTheKey writes the key that answered the question back
	// to the screen. zsh does; bash does not.
	ListQueryEchoesTheKey bool

	// What to do about a line of output that never ended.
	//
	// A command — or a plugin loading — can leave the cursor part-way along a
	// row, and the next prompt has to go somewhere. The two shells with a line
	// editor disagree completely, measured 2026-09-12 through a
	// pseudo-terminal with an rc file whose last act is `printf 'LEFTOVER'`:
	//
	//	bash   LEFTOVERP>                        the prompt runs straight on
	//	zsh    LEFTOVER%                         a marker, and the prompt below
	//	       P>
	//
	// zsh writes the marker in inverse at the cursor, pads with spaces to the
	// end of the row so the terminal wraps, and returns — which leaves the
	// unfinished output on the screen with a mark saying it was unfinished,
	// and puts the prompt on a row of its own. This is what keeps the
	// gitstatusd progress line powerlevel10k prints from being drawn over by
	// the prompt (#2477).
	//
	// Both are named for the *options* that control them rather than given as
	// values, because both are options a person turns off — see
	// HistoryStyle.IgnoreSpaceOption, which is the same shape and the reason
	// it is that shape. An empty name is a dialect with no such option, and
	// the behavior is off.
	//
	// **The two are not independent, and the dependence is measured rather
	// than assumed.** With `nopromptcr` the marker is not written *either*,
	// though `promptsp` is still on — so the return is what the marking is
	// built on, and a dialect that named only the marker would get neither.
	// zsh's own documentation says as much; this is the check.
	MarkUnfinishedOutputOption string

	// ReturnBeforeThePromptOption names the option that puts the cursor at the
	// start of the row before the prompt is drawn. See above for why it is
	// also what makes the marking possible.
	ReturnBeforeThePromptOption string

	// UnfinishedOutputMark is what the marker looks like, written where the
	// output stopped. zsh draws a bold, inverse `%`; a dialect with no such
	// option leaves this empty and nothing is drawn.
	//
	// The text is the dialect's whole answer, escape sequences included, for
	// the reason Interrupt above is: what a shell puts on the screen is not
	// something the substrate should be choosing.
	UnfinishedOutputMark string

	// ClearsBelowThePrompt erases from the cursor to the end of the screen
	// before the prompt is drawn.
	//
	// A third answer and not part of either option, which is measured: with
	// **both** turned off zsh still writes `\e[J` and bash still writes
	// nothing. So it is what this editor does about the rows below a prompt
	// rather than something a person asked for, and it moves on its own.
	ClearsBelowThePrompt bool

	// ListQueryAcceptsOnlyYesOrNo keeps asking until one of them arrives,
	// ringing the bell at anything else. bash does. zsh takes the first key
	// whatever it is and treats everything but `y` as no.
	ListQueryAcceptsOnlyYesOrNo bool

	// WordCharacters is what counts as part of a word besides letters and
	// digits, for `M-b`, `M-f` and the word kills.
	//
	// Measured under a pty on `echo /usr/local/bin` with `M-b`:
	//
	//	bash    echo /usr/local/|bin
	//	zsh     echo |/usr/local/bin
	//
	// bash counts letters and digits and nothing else, so an empty string is
	// its answer and also the answer for a front end that has not said. zsh
	// counts the contents of its `WORDCHARS`, which puts `/`, `.`, `-`, `_`
	// and `=` inside a word — checked one character at a time, because the
	// variable is a claim and the keystroke is the fact: `:`, `,` and `@` are
	// not in it and do break a word in both.
	WordCharacters string

	// KillToStartOfLineTakesTheWholeLine is what `^U` does with the text
	// *after* the cursor.
	//
	// Measured with the cursor at the start of `echo one two`: bash leaves the
	// line untouched, having nothing before the cursor to kill, and zsh empties
	// it. This is the disagreement with the most on it — a finger that means
	// one and gets the other loses a command it had finished typing.
	KillToStartOfLineTakesTheWholeLine bool

	// KillWordBeforeCursorUsesWordCharacters is what `^W` takes off the line.
	//
	// Measured on `echo a+b`: bash leaves `echo `, zsh leaves `echo a+`.
	// bash's `^W` knows only whitespace, which is what makes it the key that
	// takes a whole path off the line however much punctuation is in it; zsh's
	// is the same word its motion keys use, so it stops inside one.
	//
	// A separate answer from WordCharacters because bash gives two different
	// ones to the two keys: `M-Delete` on the same line leaves `echo a+`,
	// agreeing with zsh's `^W` while bash's own `^W` does not.
	KillWordBeforeCursorUsesWordCharacters bool

	// ForwardWordStopsBeforeTheNextWord is where `M-f` leaves the cursor.
	//
	// Measured on `echo one two` from the start of the line:
	//
	//	bash    echo| one two
	//	zsh     echo |one two
	//
	// One character apart on every press, which is enough to make typing feel
	// like somebody else's shell. `M-b` agrees in both, and so does what
	// `M-d` kills, which is why this is about the motion alone.
	ForwardWordStopsBeforeTheNextWord bool

	// TransposeAtTheStartSwapsTheFirstTwo is what `^T` does when there is
	// nothing before the cursor to swap.
	//
	// Measured on `echo abc` with the cursor at the start: bash leaves the
	// line alone, zsh swaps `e` and `c` and leaves the cursor past them.
	// Everywhere else in the line the two agree exactly.
	TransposeAtTheStartSwapsTheFirstTwo bool

	// UndoTakesBackOneKeystrokeAtATime is how much of the line one `^_`
	// returns.
	//
	// Measured with `echo abcdef` typed a character at a time and one `^_`
	// after it: bash leaves an empty line and zsh leaves `echo abcde`. bash
	// takes back the whole *run* of typing as one change and zsh takes back
	// one keystroke, which is the difference between undoing a mistyped
	// word and undoing the line it was in.
	//
	// A run, not the line: measured, `echo abc`, `^B`, `d`, `^_` leaves
	// `echo abc` in bash, so a keystroke that is not typing ends the run.
	// The same answer decides an `M-.` walk — three presses and one `^_`
	// leaves bash in front of the first press and zsh at the second — which
	// is why this is one question rather than two.
	UndoTakesBackOneKeystrokeAtATime bool

	// UndoRestoresTheCursorToWhereItWas puts the cursor back where it stood
	// before the change, rather than after the text the undo put back.
	//
	// Measured on `echo one two` with `^A`, `^K`, `^_`: zsh leaves the cursor
	// at the start of the line, where it was when the kill happened, and bash
	// leaves it at the end of what came back. Same line either way, and the
	// next keystroke lands in a different place.
	//
	// The two agree on the case undo exists for — `^W` then `^_` puts the
	// cursor at the end of the restored word in both, because that is also
	// where it was — and part company on a kill that went forwards.
	UndoRestoresTheCursorToWhereItWas bool

	// LastArgumentStaysOnTheOldestLine is what `M-.` does once it has been
	// pressed more times than there are lines behind the prompt.
	//
	// Measured with three lines in the history and four presses: zsh keeps
	// the oldest line's last word and every further press leaves it there,
	// and bash takes the word it had inserted back off the line and puts
	// nothing in its place.
	LastArgumentStaysOnTheOldestLine bool

	// ViInsertAtStartOfLineSkipsLeadingBlanks is where `I` puts the cursor in
	// vi command mode.
	//
	// Measured under a pty on `   ab` — three leading spaces — with Escape,
	// `$`, `I` and a marker character:
	//
	//	bash    X   ab
	//	zsh        Xab
	//
	// bash inserts at column 0 and zsh inserts at the first character that is
	// not a blank, which is what `^` does in both. The two agree about `^`
	// itself and part company only here.
	//
	// The rest of the command mode agrees across bash 5.3, bash 3.2 and zsh,
	// which is why this is the only field it adds. See docs/spec/editing.md
	// for the table and for the four places the shells disagree about a key
	// this editor does not offer yet.
	ViInsertAtStartOfLineSkipsLeadingBlanks bool

	// CompletionMatchesHiddenFiles offers names beginning with a dot to a
	// word that does not begin with one.
	//
	// The dialects disagree, measured with a `.hidden` beside nine ordinary
	// names and the lot listed on a double Tab:
	//
	//	bash   .hidden  a$b.txt  bracket[1].txt  dir/ …
	//	zsh    a$b.txt  bracket[1].txt  dir/ …
	//
	// bash's readline calls it `match-hidden-files` and has it on. It is
	// off here by default, which is zsh's answer and the one that keeps Tab
	// usable in a home directory — a bare Tab there is otherwise a list of
	// configuration files nobody was reaching for.
	//
	// A word that *does* begin with a dot matches them in either case, so
	// this decides one thing only: what an empty name matches.
	CompletionMatchesHiddenFiles bool
}

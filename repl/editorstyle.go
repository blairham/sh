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
	// SearchOnControlX puts the two plain incremental searches on `^X r` and
	// `^X s` as well as on `C-r` and `C-s`. zsh's emacs keymap does, measured
	// 2026-10-04 with `bindkey -M emacs` on zsh 5.9.2; bash binds `^X s` to
	// spell correction and leaves `^X r` unbound, so the zero value is its.
	SearchOnControlX bool

	// SendBreakOnControlG puts send-break on `^G` in the emacs keymap: the
	// line is given up with a bell and `$?` 1 (see ErrBroken). zsh's emacs
	// keymap does, measured 2026-10-04 with `bindkey '^G'` on zsh 5.9.2; its
	// vi keymaps do not, and bash's `^G` is readline's abort, which keeps the
	// line — so the zero value ignores the key, as this editor always did.
	SendBreakOnControlG bool

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

	// ListQueryThresholdParameter is the shell parameter that says how many
	// matches it takes before the question above is asked, or the empty
	// string where the threshold is not a parameter a script can set.
	//
	// zsh's is `$LISTMAX` and bash's is a readline variable rather than a
	// shell parameter, so only one of the two names anything here — and a
	// front end that names nothing keeps the built-in count, which is a
	// hundred in both shells that ask.
	//
	// Read **live**, on the keystroke that found the matches, because it is
	// a parameter a person sets at the prompt. See editor.listThreshold.
	ListQueryThresholdParameter string

	// KeySequenceWaitParameter is the shell parameter holding how long the
	// line editor waits for the rest of a multi-character key sequence before
	// deciding the one it has is complete, **in hundredths of a second**, or
	// the empty string where the wait is not a parameter a script can set.
	//
	// zsh's is `$KEYTIMEOUT`, default 40. bash's is `keyseq-timeout`, a
	// readline variable in milliseconds rather than a shell parameter, so
	// only one of the two names anything here — and **a front end that names
	// nothing waits no time at all**, which is what every dialect did before
	// this and is why the field is a name and not a duration.
	//
	// Measured 2026-09-28 through a pseudo-terminal against zsh 5.9.2, in vi
	// mode with `echo hello` typed, an Escape, a gap, and then `[D` — the
	// tail of a left arrow. Waited through, the three bytes are the arrow and
	// the line is unchanged; given up on, the Escape was the mode switch and
	// `D` kills to the end of the line:
	//
	//	KEYTIMEOUT  gap     line runs as
	//	40          0.05s   echo hello
	//	40          1.00s   echo hell
	//	200         1.00s   echo hello
	//	1           0.05s   echo hell
	//	200         3.00s   echo hell
	//	1000        4.00s   echo hello
	//
	// **Rows two and three hold the gap still and move only the parameter,
	// and so do rows one and four.** That is the pair that says the number is
	// read, rather than that some fixed timer exists — a grid that varied
	// only the gap would agree with a hard-coded four tenths on every row.
	//
	// **Zero does not mean "no wait".** It and every negative wait
	// *indefinitely*, measured: `KEYTIMEOUT=0` with a four-second gap still
	// reads the arrow, and so does `KEYTIMEOUT=-5`. A value that is not a
	// number is zero for the same reason — the reference declares the name
	// `integer`, so the assignment stores 0 — and waits indefinitely too. The
	// obvious reading of zero is the opposite of the measured one, which is
	// why the rows are here.
	//
	// Read **live**, on the keystroke, because it is a parameter a person
	// sets at the prompt. See Shell.keySequenceWait.
	KeySequenceWaitParameter string

	// BracketedPaste asks the terminal to wrap pasted text in markers, for
	// the length of each line read.
	//
	// A terminal marks a paste only for an application that asked, so this
	// decides whether a paste reaches the shell as text to look at or as a
	// burst of typing — and a burst of typing carries the newlines that run
	// it. Measured 2026-09-14 through a pseudo-terminal:
	//
	//	bash 5.3.3   \e[?2004h before the prompt, \e[?2004l\r after the line
	//	zsh 5.9.2    the same two, the first written after the prompt
	//	ksh93        neither; the markers are typed into the line as `^[[200~`
	//
	// The two that ask agree on the bytes, so the field is whether rather than
	// what. ksh93 is the disagreement that makes it a dialect's answer at all,
	// and a front end that has not said gets no bracketing — which is also
	// what a dialect with no line editor of its own wants.
	//
	// The markers are never typed into the line whatever this says: a shell
	// that did not ask for them can still be sent them by a terminal another
	// program left in the mode, and putting `^[[200~` in a command is the
	// worst of the three things to do with it.
	BracketedPaste bool

	// BracketedPasteParameter names the array the two sequences are read
	// from, at the start of every line, where the dialect keeps them in one.
	// Empty is the two fixed sequences in paste.go.
	//
	// Measured 2026-10-02 on zsh 5.9.2 through a pseudo-terminal, assigning
	// the parameter at one prompt and reading the bytes around the next line:
	// two elements are the two sequences, the first written after the prompt
	// and the second at the end of the line, either of them allowed to be
	// empty; unset, a scalar, one element or three, and neither is written.
	BracketedPasteParameter string

	// PrefixArgument says ESC and a digit, or ESC and a minus, set a count
	// for the next keystroke. See prefixarg.go for what zsh does with one.
	PrefixArgument bool

	// PastedTextStyle is written before a run of text that arrived as a
	// paste, and PastedTextStyleEnd after it. Empty draws the text like any
	// other, which is what a dialect that does not mark a paste does.
	//
	// Measured 2026-09-14 on a paste of `echo PASTED`: bash 5.3.3 and zsh
	// 5.9.2 both draw it as `\e[7mecho PASTED\e[27m` — reverse video, ended
	// by turning reverse video off rather than by resetting everything — and
	// both draw the line again without it on the next keystroke, whatever
	// that keystroke is.
	//
	// Two fields rather than one and a reset, because the ending is the half
	// that was measured: a run that reset everything would also drop whatever
	// the prompt or a highlighter had left in force around it. Text rather
	// than a flag, for the reason Interrupt above is.
	PastedTextStyle    string
	PastedTextStyleEnd string

	// ControlCharacterStyle is written before the caret a control character
	// in the line is drawn as — `^A`, `^?`, `^[` — and ControlCharacterStyleEnd
	// after it. Empty draws the caret like any other text, which is bash.
	//
	// Measured 2026-10-05 through a pseudo-terminal on a paste of
	// `a\tb\x01c\x1b[31md\x7fe`: zsh 5.9.2 draws each caret as `\e[7m^A\e[27m`,
	// standout and its end, which is its `zle_highlight` default for the
	// `special` context, and bash 5.3 draws the same carets plain. Both draw
	// the tab as spaces to its stop, so that half is not a dialect's. See
	// controlglyph.go.
	ControlCharacterStyle    string
	ControlCharacterStyleEnd string

	// TabOnABlankLineTypesItself has a Tab that asks this editor's own
	// completion for a word, with nothing but blanks before the cursor, put a
	// tab in the line instead. zsh's, measured 2026-10-05 against 5.9.2 with
	// no completion system loaded; bash 5.3 completes every command there is
	// on the same keystroke. See editor.tabOnABlankLine (#6119).
	TabOnABlankLineTypesItself bool

	// ListQueryEchoesTheKey writes the key that answered the question back
	// to the screen. zsh does; bash does not.
	ListQueryEchoesTheKey bool

	// SelfInsertWidget is what this shell calls putting the typed character
	// into the line, and empty for a dialect that does not name it.
	//
	// Named, because the name is the whole mechanism: on a printable key the
	// editor asks the shell whether anything has been put in front of typing,
	// and it can only ask by name. A syntax highlighter is what does that — it
	// wraps every widget in the shell's table and recolors the line after
	// each, so the widget it most needs is the one that runs when a person
	// types (#2485).
	//
	// Empty costs nothing: a printable key never leaves this package, which is
	// every dialect that has not named it and every session in a shell that
	// has.
	SelfInsertWidget string

	// SpecialWidgets says this shell has the widgets the line editor calls
	// itself — `zle-line-init`, `zle-line-pre-redraw` and `zle-line-finish`
	// — so the editor asks for them by name. False for a dialect with none,
	// which then never asks. See specialwidgets.go.
	SpecialWidgets bool

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

	// FlowControlOption names the option that, while it is *unset*, takes
	// the terminal's flow control away for the length of an edit — zsh's
	// FLOW_CONTROL. An empty name is a dialect with no such option, and the
	// editor leaves XON/XOFF exactly as it found the terminal, which is what
	// bash does (#5943).
	//
	// The default either way is the terminal's: with `ixon` on, which is how
	// a terminal starts, `C-s` stops the output and `C-q` starts it again and
	// neither reaches the editor; a person who wants the keys runs `stty
	// -ixon`, and the editor starts from what it found. The option is read
	// each time the editor takes the terminal, so `unsetopt flowcontrol`
	// typed at a prompt is live at the next one — and not for the rest of a
	// line a widget changed it on, which is zsh's answer too. See
	// internal/tty's Raw for the measurement.
	FlowControlOption string

	// RunsUnderTheOptions names the options this dialect's line editor runs
	// under: it runs while any of them is on. With it off the session still reads lines and still runs them —
	// it is the *editor* that goes away, so the terminal keeps its own line
	// discipline, echoes the keystrokes itself, and nothing the editor would
	// have drawn is written.
	//
	// Empty is a dialect whose editor is not something a person can turn off,
	// which is three of the five. zsh's option is `zle`, and `-o interactive
	// +o zle` is how a test suite drives the shell through a pseudo-terminal
	// without one — see dialect/zsh's EditorStyle. bash's are `emacs` and
	// `vi`, the editing modes: `set +o emacs +o vi` turns readline off, and
	// measured on bash 5.3.20 the prompt then writes no bracketed-paste
	// request and the terminal gathers the line (#5922).
	//
	// A *name* rather than a bool, for the reason
	// [Shell.CommentsNeedTheOption] is one: the state moves while the session
	// runs. Measured 2026-09-25, zsh 5.9.2 on a pseudo-terminal with `-fiV`,
	// `TERM=dumb` and an empty `PS1`, a line at a time:
	//
	//	unsetopt zle   that line is drawn by the editor, and every line
	//	               after it is plain — no `\e[?2004h`, no redraw
	//	setopt zle     in a `+Z` shell, the reverse: that line is plain and
	//	               the next one is drawn
	//
	// So it is asked per read and not once at startup. An option this shell
	// has never heard of is **not** an option that is off — see
	// [Shell.editorIsOff], which is the same rule and the same reason as
	// [Shell.commentsAreOff].
	RunsUnderTheOptions []string

	// InterruptWithoutTheEditorTakesTheNextLine makes a ^C at a prompt read
	// with the editor off give up the rest of that read, through the next
	// newline, where otherwise the read ends at the ^C.
	//
	// Measured 2026-10-04 through a pseudo-terminal, `abc` and ^C:
	//
	//	zsh 5.9.2, unsetopt zle       `^C`, nothing more until a newline is
	//	                              typed; that line is given up and never
	//	                              run; then a fresh prompt
	//	bash 5.3.20, set +o emacs +o vi  `^C`, a newline and a fresh prompt
	//	                              at once; the next line runs
	//
	// `$?` is 130 after either. See Shell.readCookedLineAtThePrompt.
	InterruptWithoutTheEditorTakesTheNextLine bool

	// UnfinishedOutputMark is what the marker looks like, written where the
	// output stopped. zsh draws a bold, inverse `%`; a dialect with no such
	// option leaves this empty and nothing is drawn.
	//
	// The text is the dialect's whole answer, escape sequences included, for
	// the reason Interrupt above is: what a shell puts on the screen is not
	// something the substrate should be choosing.
	UnfinishedOutputMark string

	// ClearBeforeThePrompt is written last of all, on the row the prompt is
	// about to be drawn on.
	//
	// A third answer and not part of either option, which is measured: with
	// **both** turned off the shell that marks still writes this and the
	// other still writes nothing. So it is what the editor does about the
	// ground under a prompt rather than something a person asked for, and it
	// moves on its own.
	//
	// Measured 2026-09-12, the whole of it is
	// `\e[0m\e[27m\e[24m\e[J` — the three attributes a run of output is
	// most likely to have left on, and then an erase to the end of the
	// screen. The erase is what keeps a shorter prompt from leaving the tail
	// of a longer one behind it; the resets are what keep output that ended
	// mid-escape from drawing the prompt bold.
	//
	// Text rather than a flag, for the reason Interrupt and
	// UnfinishedOutputMark are: what a shell puts on the screen is not
	// something the substrate should be choosing. Empty writes nothing.
	ClearBeforeThePrompt string

	// ListQueryAcceptsOnlyYesOrNo keeps asking until one of them arrives,
	// ringing the bell at anything else. bash does. zsh takes the first key
	// whatever it is and treats everything but `y` as no.
	ListQueryAcceptsOnlyYesOrNo bool

	// ListQueryAnswerTakesTheQuestionsRow erases the question once it is
	// answered: a listing starts on the row the question was on, and a
	// declined one puts the cursor back on the line, which is drawn again
	// where it was. Measured 2026-10-05 through a pseudo-terminal: zsh 5.9.2
	// writes `\r\e[J` before the listing and `\r\e[J\e[A` before redrawing a
	// line it declined to list for; bash 5.3 leaves the question and starts
	// the listing, or a fresh prompt, on the row below (#6119).
	ListQueryAnswerTakesTheQuestionsRow bool

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

	// InsertLastWordTakesArguments is zsh's `insert-last-word`: a count picks
	// the word — a positive one from the end of the line, nought or a negative
	// one from the start — and a widget calling it may name the history offset,
	// the word, and whether the offset counts from the current line. See
	// insertLastWordWith in lastarg.go (#5987). bash's `yank-last-arg` takes
	// neither here, and is what the field's zero value keeps.
	InsertLastWordTakesArguments bool

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

	// ListMatchesWithoutASecondKeyOption names the option that draws the
	// matches on the very keystroke that found them ambiguous, rather than
	// leaving them for a second one.
	//
	// The two shells with a line editor disagree, measured 2026-09-19 through
	// a pseudo-terminal on a word whose matches agree on nothing past what is
	// typed — `: big/aa0` against ten `aa0N/` directories:
	//
	//	bash 5.3.20   \a                     then, on a second key, the listing
	//	zsh 5.9.2     \a and the listing      and a third answer on the second key
	//
	// Named for the option rather than given as a value for the reason
	// MarkUnfinishedOutputOption is: `unsetopt autolist` at the prompt is a
	// person asking for the other answer, and measured, it gets it — the same
	// keystroke then writes the bell alone. An empty name is a dialect with
	// no such option, and the listing waits for a second key.
	ListMatchesWithoutASecondKeyOption string

	// BellRingsOnAnAmbiguousCompletionThatInserts sounds the bell for a
	// completion with more than one match even where it put a prefix on the
	// line.
	//
	// Measured 2026-09-19 through a pseudo-terminal, one Tab at a time,
	// against twelve directories agreeing on `aa`, one unique file and one
	// unique directory:
	//
	//	typed        bash 5.3.20    bash 3.2.57    zsh 5.9.2
	//	: big/a      \a and `a`     \a and `a`     `a`
	//	: big/u      the name       the name       the name
	//	: big/aa0    \a             \a             \a and the listing
	//
	// The two shells are asking different questions. bash asks whether the
	// **word is settled** and rings while it is not, so a keystroke that put
	// half a word in still rings; zsh asks whether the **keystroke did
	// anything** and is silent when it did. A unique match is silent in both,
	// so this is not one shell ringing more often — it is the middle state,
	// and it is bash's alone.
	//
	// False is zsh's answer and the core's, and also what a front end that
	// has not said gets. Both bash 5.3.20 and bash 3.2.57 give the same
	// answer, so it is that shell's rather than a version's.
	BellRingsOnAnAmbiguousCompletionThatInserts bool

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

	// SymlinkedDirectoryMarkedWhenNamedWhole withholds the slash from a
	// symlink to a directory until the word already names it whole, and puts
	// nothing after it in the meantime — readline's `mark-symlinked-directories`
	// off, which is its default.
	//
	// Measured 2026-10-04 through a pseudo-terminal, no startup files, with
	// `realdir/` and `linkdir -> realdir`:
	//
	//	              link⇥        link⇥⇥        real⇥
	//	bash 5.3.20   linkdir      linkdir/      realdir/
	//	zsh 5.9.2     linkdir/                   realdir/
	//
	// readline's two variables are not settable here (see dialect/bash's
	// bind.go on `-v`), so this is the default and only the default: with
	// `mark-symlinked-directories` on bash marks the link at once, and with
	// `mark-directories` off it marks neither.
	SymlinkedDirectoryMarkedWhenNamedWhole bool
}

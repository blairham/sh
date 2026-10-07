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

	// WideEmacsKeymap puts the keys zsh's emacs keymap has beyond the
	// editor's shared table on the emacs keymap: the case keys `M-u`, `M-l`
	// and `M-c`, transpose-words on `M-t`, push-line on `M-q`, quote-line on
	// `M-'`, accept-and-hold on `M-a`, undo on `^X u` and quoted-insert on `^V`, measured 2026-10-06 with
	// `bindkey -M emacs` on zsh 5.9.2. Not in vi editing, whose insert
	// keymap has none of them. The zero value leaves them doing nothing, as
	// this editor always did. bash has the first five of these and none of
	// the rest — see WordKeys (#6241, #6250).
	WideEmacsKeymap bool

	// WordKeys puts the part of that set both shells' emacs keymaps have on
	// the emacs keymap, and nothing else: the case keys `M-u`, `M-l` and
	// `M-c`, transpose-words on `M-t`, the upper-case spellings of all four,
	// and quoted-insert on `^V`. bash 5.3's `bind -p` lists every one of
	// them and none of the rest (#6250). WideEmacsKeymap implies it.
	//
	// What the keys do differs in two places, which are fields of their own:
	// CapitalizeTakesTheFirstCharacter and TransposeWordsReachesTheLineEnd.
	WordKeys bool

	// QuotedInsertInViInsert puts quoted-insert on `^V` in vi insert mode
	// as well. bash's `bind -m vi-insert -p` lists it there, measured
	// 2026-10-06 on bash 5.3.20 (#6250). zsh's viins has vi-quoted-insert
	// there, which draws while it waits — see ViQuotedInsert.
	QuotedInsertInViInsert bool

	// QuotedInsertInViCommand puts quoted-insert on `^V` in vi command mode:
	// the next key goes in the line as it is, before the character under the
	// cursor, as many times as the command's count says, and the cursor stays
	// on that character, in command mode. bash's `bind -m vi-command -p`
	// lists it; zsh's vicmd has nothing on `^V` (`bindkey -M vicmd '^V'` is
	// undefined-key on zsh 5.9.2). Measured 2026-10-06 through a
	// pseudo-terminal against bash 5.3.20, `set -o vi`, `od -c <<< ab`,
	// Escape (the cursor on the `b`), then the keys and `i@`:
	//
	//	keys           the word
	//	^V ^A          a ^A @ b
	//	^V ^A x        a ^A          ← the cursor stayed on the b
	//	3 ^V ^A        a ^A ^A ^A @ b
	//	^V z           a z @ b
	//	^V ESC         a ^[ @ b      ← and still command mode
	//	^V ^A ^V ^B    a ^A ^B @ b
	//
	// (#6259).
	QuotedInsertInViCommand bool

	// CapitalizeTakesTheFirstCharacter makes `M-c` raise the first character
	// of each word whatever it is, and lower the rest, where the zero value
	// raises the first *letter* and passes over a digit before it.
	//
	// Measured 2026-10-06 through a pseudo-terminal against bash 5.3.20 with
	// `INPUTRC=/dev/null`, the cursor at the start of the line:
	//
	//	line     bash 5.3.20   zsh 5.9.2
	//	3AB x    3ab x         3Ab x
	//	a3B x    A3b x         A3b x
	//
	// Elsewhere the two agree: from the middle of a word the character under
	// the cursor is the one raised, and from a blank the next word's first.
	CapitalizeTakesTheFirstCharacter bool

	// TransposeWordsReachesTheLineEnd is what `M-t` swaps when nothing after
	// the cursor is a word: the last word and everything after it to the end
	// of the line, where the zero value takes the last word alone. And a
	// cursor in or before the first word, with no word before it to swap
	// with, is an edit with nothing to act on and rings under
	// BellRingsWhenAnEditHasNothingToActOn.
	//
	// Measured 2026-10-06 through a pseudo-terminal against bash 5.3.20 with
	// `INPUTRC=/dev/null`; `␠` is a blank:
	//
	//	line       cursor   bash 5.3.20      cursor   zsh 5.9.2   cursor
	//	aa bb␠␠    7        bb␠␠␠aa          7        bb aa␠␠     5
	//	aa bb␠␠    5        bb␠␠␠aa          7        bb aa␠␠     5
	//	aa bb;;    7        bb;; aa          7
	//	aa bb cc   1        unchanged, \a    1        unchanged   1
	//	ab         2        unchanged, \a    2
	//
	// Everywhere a word follows the cursor the two agree: `aa bb cc` swaps
	// `aa` and `bb` from 2, 3 and 4 and `bb` and `cc` from 5 to the end, and
	// `aa, bb` from 4 is `bb, aa` with the separator left where it was.
	TransposeWordsReachesTheLineEnd bool

	// ViQuotedInsert puts vi-quoted-insert on `^V` in vi insert mode: the
	// next key goes in the line as it is, as quoted-insert's does in emacs
	// editing, with a `^` drawn at the cursor while it waits. zsh's viins
	// keymap has it, measured 2026-10-06 with `bindkey -M viins '^V'` on zsh
	// 5.9.2. The zero value leaves the key doing nothing, as this editor
	// always did; bash's is QuotedInsertInViInsert, which draws nothing
	// (#6251).
	ViQuotedInsert bool

	// ZshViInsertKeymap gives vi insert mode zsh's viins keymap, which is not
	// its emacs one: most control keys type themselves, `^D` lists, and
	// Backspace, `^U` and `^W` stop where the stretch of insert mode began.
	// The zero value is the shared table, which bash's vi insert mode has
	// always used. See repl/viinsertkeys.go for the measurement (#6272).
	ZshViInsertKeymap bool

	// QuotedInsertAbandonsOnControlC makes `^C` after quoted-insert's `^V`
	// give the line up with a bell, where the zero value puts a `^C` in the
	// line as quoted-insert does any other key. Measured 2026-10-06 through a
	// pseudo-terminal, `abc` with the cursor on the `c`, in emacs and in vi
	// insert editing alike: zsh 5.9.2 writes `\a` and abandons the line, where
	// `^C` alone abandons it silently; bash 5.3.20 with `INPUTRC=/dev/null`
	// draws `ab^Cc` and goes on reading (#6251).
	QuotedInsertAbandonsOnControlC bool

	// WhichCommandWord and RunHelpWord are the commands which-command and
	// run-help put in the line in place of the one they ask about, followed
	// by its command word: zsh's are `which-command` and `run-help`, which
	// are aliases a person can change — measured 2026-10-06 against zsh
	// 5.9.2, unaliasing `which-command` makes `M-?` say `command not found`.
	// Empty rings and does nothing (#6241).
	WhichCommandWord string
	RunHelpWord      string

	// ListOnControlD makes `^D` on a line with something typed the action
	// that deletes the character under the cursor or, with none under it,
	// lists the matches for the word before it — zsh's delete-char-or-list.
	// The zero value deletes and nothing more, which is bash's delete-char.
	// The empty line is the key's own either way: it ends the session.
	//
	// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2 and
	// bash 5.3, a two-row prompt, `^D` after `ls x` in a directory holding
	// xa, xb and xc: zsh lists `xa  xb  xc` under the line and returns to
	// it, with or without compinit, and bash rings the bell and lists
	// nothing. Mid-line both delete and draw the deletion (#6233).
	ListOnControlD bool

	// ControlDAtAContinuationLists makes `^D` on an empty line at a
	// continuation prompt the delete-char-or-list it is everywhere else in
	// the line — a listing for the empty word, and a bell where nothing
	// matches — rather than end of input. The zero value ends input there as
	// at the first prompt, which is bash's: it reports the unfinished command
	// as a syntax error and the session ends (or refuses, under IGNOREEOF).
	//
	// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, a
	// two-row prompt, in an empty directory: `for x in 1`, Return, then `^D`
	// at `for> ` writes `\a` and nothing else, and the loop can still be
	// finished; at `dquote> ` the same; after `if true`, `then`, `^D` at
	// `then> ` asks whether to list every command. `setopt ignoreeof` changes
	// none of it — no refusal is written at a continuation prompt (#6242).
	// Needs ListOnControlD, which is the action it runs.
	ControlDAtAContinuationLists bool

	// CompletionReadsTheContinuation makes a word at the start of a
	// continuation line complete in the context the lines already entered
	// give it — an argument after `for x in 1`, nothing inside a quote an
	// earlier line opened — where the zero value reads the line alone, as
	// bash's does. See repl/continuationcontext.go for the measurement
	// (#6242).
	CompletionReadsTheContinuation bool

	// IgnoreEndOfInputOption names the option that makes `^D` on an empty
	// line at a prompt refuse to end the session, saying
	// Shell.EndOfInputRefused instead; IgnoreEndOfInputParameter names the
	// parameter that does the same and says how many times. A dialect names
	// one or neither, and neither refuses nothing, which is three of the
	// five.
	//
	// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2 and
	// bash 5.3.20, a two-row prompt, `^D` pressed repeatedly at an empty
	// prompt:
	//
	//	zsh   setopt ignoreeof     refuses 9, the 10th says so and exits
	//	bash  set -o ignoreeof     refuses 10 (the option writes IGNOREEOF=10)
	//	bash  IGNOREEOF=3          refuses 3, the 4th exits
	//	bash  IGNOREEOF=03         refuses 3
	//	bash  IGNOREEOF=0          exits on the first
	//	bash  IGNOREEOF= / abc     refuses 10
	//	bash  IGNOREEOF=-1 / +2    refuses 10
	//	bash  IGNOREEOF=' 2' / 3x  refuses 10
	//	bash  IGNOREEOF=2 inherited from the environment, option off:
	//	                           refuses 2
	//
	// So in bash it is the parameter and not the option that decides — the
	// inherited row leaves `set -o` saying `off` and still refuses — and a
	// value counts only when it is all digits. Both shells count consecutive
	// refusals: a command run in between starts the count again, and an empty
	// line or a character typed and erased does not.
	IgnoreEndOfInputOption    string
	IgnoreEndOfInputParameter string

	// EndOfInputRefusals is how many `^D` an ignore-EOF setting refuses where
	// it gives no count of its own: the option, and a parameter whose value
	// is not a count. See IgnoreEndOfInputOption for the measurements.
	EndOfInputRefusals int

	// EndOfInputRefusalStaysOnTheLine draws a refusal zsh's way: a bell and
	// the line under the prompt's last row, with the cursor back on the
	// prompt and the read going on, and the same line drawn on the `^D` that
	// finally ends the session. Measured 2026-10-06, zsh 5.9.2 writes
	// `\r\r\n\a` and the line, then `\r\e[A` and the line redrawn on its own
	// row. The zero value is bash's: the line is ended, the refusal written
	// on a row of its own and a fresh prompt drawn under it, `\r\r\nUse
	// "exit" to leave the shell.\r\n` and the prompt again.
	EndOfInputRefusalStaysOnTheLine bool

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

	// BracketedPasteSetting names a variable that stops the bracketing from
	// the next prompt on while it holds `off`, and leaves BracketedPaste's
	// answer alone otherwise — unset included. bash's is readline's
	// `enable-bracketed-paste`, which `bind 'set enable-bracketed-paste
	// off'` sets: measured 2026-10-06 through a pseudo-terminal against bash
	// 5.3.20, the next prompt is written without `\e[?2004h`, and `on` asks
	// again (#6264).
	BracketedPasteSetting string

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

	// CountPrompt is drawn in place of the prompt's last row while a count
	// is being typed, with the count, sign and all, as its one operand.
	// Empty draws nothing, which is zsh's answer. bash's is `(arg: %d) `,
	// measured 2026-10-06 through a pseudo-terminal against bash 5.3.20 with
	// a two-row prompt: `ESC 9` rewrites `P> echo ab` as `(arg: 9) echo ab`
	// with the cursor where it was in the line, `ESC -` as `(arg: -1)`, and
	// the key that spends the count puts `P> ` back (#6248).
	CountPrompt string

	// CountReadAsReadline is how the count is typed, where it differs from
	// the zero value's — zsh's, see prefixarg.go. Measured 2026-10-06
	// through a pseudo-terminal against bash 5.3.20, the count read back
	// from how far `^F` or `^B` moved, or how many `z` were typed:
	//
	//	keys                bash 5.3.20   zsh 5.9.2
	//	ESC 1 2 z           12 z          2z — a digit is typing
	//	ESC 1 ESC 2 z       12 z          12 z
	//	ESC - 2 ^F          -2            -1, then 2 typed
	//	ESC - ESC 2 ^F      -12           -2
	//	ESC - 2 ESC 3 ^F    -23
	//	ESC 3 ESC - z       ---z          a minus spends nothing
	//	ESC 1 - ^B          -, then ^B
	//	ESC - ESC - z       z             (+1)
	//	ESC - - z           nothing       (-1)
	//
	// So once a count has begun a plain digit is more of it; a minus after
	// a digit is a character typed that many times, ESC or no ESC; a minus
	// after a minus is absorbed, or with ESC turns the sign back; and an ESC
	// digit after a lone minus is appended to that minus's one.
	CountReadAsReadline bool

	// NegativeCountTypesNothing makes a character typed with a negative
	// count type nothing, where the zero value types it once and leaves the
	// cursor in front of it. `^V` with a negative count reads that many keys
	// and puts each in the line once. Measured 2026-10-06 against bash
	// 5.3.20, `ab` with the cursor after the `a`: `ESC - z` and `ESC - 3 z`
	// leave `ab`; `ESC - ^V ^A` gives `a^Ab` with the cursor after the `^A`,
	// and `ESC - 3 ^V` takes the three keys after it as they are.
	NegativeCountTypesNothing bool

	// CountStopsWhereItCannotAct plays a counted key one press at a time and
	// stops at the first press with nothing to act on, which rings — under
	// BellRingsWhenAnEditHasNothingToActOn — only if no press acted at all,
	// or if the key is one that moves or deletes backward a character at a
	// time. And a counted `^T` stops at the end of the line, where the zero
	// value goes on swapping the last two.
	//
	// Measured 2026-10-06 through a pseudo-terminal against bash 5.3.20 with
	// `INPUTRC=/dev/null`, `\a` looked for in what the keys wrote:
	//
	//	line       cursor   keys           after                cursor
	//	echo ab    7        ESC 9 ^B       unchanged, \a        0
	//	echo ab    1        ESC 3 Left     unchanged, \a        0
	//	echo ab    2        ESC 3 BS       ho ab, \a            0
	//	echo ab    1        ESC - 3 ^F     unchanged, \a        0   ← ^B now
	//	echo ab    0        ESC 9 ^F       unchanged            7
	//	echo ab    5        ESC 3 Right    unchanged            7
	//	echo ab    5        ESC 9 ^D       echo                 5
	//	echo ab    7        ESC 2 ^F       unchanged, \a        7
	//	echo ab    0        ESC 2 ^W       unchanged, \a        0
	//	aa bb cc dd 11      ESC 9 ^W       empty                0
	//	abcd       1        ESC 9 ^T       bcda                 4
	//	abcd       4        ESC 2 ^T       abdc                 4
	//
	// So `^B`, Left, Backspace and `^H` ring when they run out part-way and
	// the rest do not.
	CountStopsWhereItCannotAct bool

	// NegativeCaseCountGoesBackward makes the case keys with a negative
	// count change the words *before* the cursor, from the start of the
	// count's word back to the cursor, and leave the cursor where it was;
	// the zero value changes as many words forward. Measured 2026-10-06
	// against bash 5.3.20 on `aa bb cc`:
	//
	//	cursor   keys           after        cursor
	//	8        ESC - M-u      aa bb CC     8
	//	7        ESC - M-u      aa bb Cc     7   ← the word's start to here
	//	6        ESC - M-u      aa BB cc     6
	//	8        ESC - M-c      aa bb Cc     8
	//	14       ESC - 3 M-u    on `aa bb cc dd ee`: aa bb CC DD EE
	NegativeCaseCountGoesBackward bool

	// TransposeWordsCountAsReadline is what a count does to `M-t`, where the
	// zero value's is zsh's (see repl/casewords.go). The pair a count of one
	// swaps is found first; then each further unit of the count moves the
	// later word on a word, and where there is no word after it moves the
	// earlier word back one instead. Nought does nothing, a negative count
	// does nothing and rings, and the cursor goes to the end of the later
	// word. Measured 2026-10-06 through a pseudo-terminal against bash 5.3.20
	// on `aa bb cc dd ee` (#6265):
	//
	//	cursor   ESC 2 M-t          ESC 3 M-t
	//	0, 1     bb aa cc dd ee     cc bb aa dd ee
	//	2 … 4    cc bb aa dd ee     dd bb cc aa ee
	//	5, 6     aa dd cc bb ee     aa ee cc dd bb
	//	9 … 14   aa bb ee dd cc     aa ee cc dd bb
	//
	// and `ESC 9 M-t` at the end of `aa bb cc dd` swaps `aa` and `dd`.
	TransposeWordsCountAsReadline bool

	// YankLastArgCountAsReadline is what a count does to `M-.`, which in the
	// zero value's dialect without InsertLastWordTakesArguments it does not
	// read at all. The first press's count picks the word, from the start
	// of the line — 0 the command word — or, negative, back from the last:
	// -1 is the word before the last. The pick holds for the presses that
	// walk back from it, a later press's count only saying which way to
	// walk, and a line without that word puts nothing in and rings. Measured
	// 2026-10-06 against bash 5.3.20, `: w1 w2 w3 w4` the line before and
	// `: x1 x2 x3` the one before that (#6265):
	//
	//	ESC 0 M-.  :          ESC 4 M-.  w4         ESC 5 M-.  nothing, \a
	//	ESC - M-.  w3         ESC -4 M-. :          ESC -5 M-. nothing, \a
	//	ESC 1 M-. M-.  x1     M-. ESC 2 M-.  x3     M-. M-. ESC - M-.  w4
	//	ESC 4 M-. M-.  nothing, \a
	YankLastArgCountAsReadline bool

	// KillLineReadsOnlyTheSign makes `^K` read only its count's sign: with
	// a negative count it kills from the start of the line to the cursor, as
	// `^U` does, and with any other — nought included — it kills to the end
	// once. The zero value plays `^K` as many times as the count says. Measured 2026-10-06 against
	// bash 5.3.20, `aa bb cc dd` with the cursor at 5: `ESC - ^K` and
	// `ESC -3 ^K` leave ` cc dd`, `ESC 3 ^K` and `ESC 0 ^K` leave `aa bb`,
	// and `ESC - ^K` at the start rings (#6265).
	KillLineReadsOnlyTheSign bool

	// TransposeCharsTakesNoNegativeCount makes `^T` with a count of nought
	// or less do nothing, except at the end of the line, where it swaps the
	// last two as it does with no count. Measured 2026-10-06 against bash
	// 5.3.20 on `abcd`: `ESC - ^T` with the cursor at 1, 2 or 3 and `ESC 0
	// ^T` at 2 leave it as it was, cursor and all, with no bell, and `ESC -
	// ^T` at the end gives `abdc` (#6265).
	TransposeCharsTakesNoNegativeCount bool

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

	// BeepOption names the option that, while it is *unset*, silences every
	// bell the editor rings — zsh's BEEP. An empty name is a dialect with no
	// such option, and the editor always rings. Measured 2026-10-05 against
	// zsh 5.9.2 through a pty: after `unsetopt beep` there is no bell from a
	// completion that matches nothing, from `^G`, from a failing incremental
	// search or from a widget that returns non-zero (#6108).
	BeepOption string

	// RingsWhenAWidgetFails sounds the bell after a shell widget that a key
	// ran returns non-zero (#6108). Measured 2026-10-05 against zsh 5.9.2
	// through a pty, with `w() { BODY }; zle -N w; bindkey '^T' w`:
	//
	//	return 1, return 2, false       \a
	//	return 0                        nothing
	//	zle w2; return 0  (w2 fails)    nothing: only the widget the key ran
	//	zle w2            (w2 fails)    \a, since w returns w2's status
	//	zle accept-line; return 1       \a, then the line runs
	//	zle send-break                  nothing
	//
	// The bell comes before the line is drawn again. bash 5.3 rings nothing
	// for a `bind -x` command that fails.
	RingsWhenAWidgetFails bool

	// BellRingsWhenAnEditHasNothingToActOn sounds the bell for an emacs key
	// whose edit found nothing to act on (#6240). Measured 2026-10-06 through
	// a pseudo-terminal with a two-row prompt, each key alone on `echo ab`
	// after a fresh start, and `\a` looked for in what the key alone wrote:
	//
	//	key                          where           bash 5.3.20   zsh 5.9.2
	//	^D, Delete                   at the end      \a            see below
	//	^F, Right                    at the end      \a            nothing
	//	^B, Left                     at the start    \a            nothing
	//	Backspace, ^H                at the start    \a            nothing
	//	^W, ^U                       at the start    \a            nothing
	//	^T                           at the start    \a            nothing
	//	^T                           on `a`          \a            \a
	//	^Y                           nothing killed  \a            \a
	//	Down, ^N                     on the newest   \a            nothing
	//	                             or no history
	//	Up, ^P                       on the oldest   \a            nothing
	//	Up, ^P                       no history      nothing       nothing
	//	M-f, ^E, ^K, M-d             at the end      nothing       nothing
	//	M-b, ^A, M-Delete            at the start    nothing       nothing
	//
	// So it is the key and not the edit that decides — `^W` and `M-Delete`
	// both kill nothing at the start and only the first rings — which is why
	// each key that rings says so where the editor reads it. vi insert mode
	// rings for none of these in bash. bash 3.2.57 answers the same except
	// for Up with no history at all, which it rings for.
	//
	// zsh's \a for `^T` on one character and `^Y` with nothing killed, and
	// its `^D` at the end listing and Delete being unbound, are its own and
	// not this; false is its answer and the core's. See
	// BellRingsWhenAnEditFails.
	BellRingsWhenAnEditHasNothingToActOn bool

	// BellRingsWhenAnEditFails sounds the bell for the few emacs edits that
	// fail outright, where the zero value lets them do nothing in silence
	// (#6247). zsh rings for far fewer keys than bash — see the table above —
	// and these are the ones: measured 2026-10-06 through a pseudo-terminal
	// against zsh 5.9.2, a two-row prompt, `bindkey -e`, each key alone in a
	// fresh shell and `\a` looked for in what that key wrote:
	//
	//	key                     where                       zsh 5.9.2
	//	^T                      on a line of one character  \a
	//	^T                      at the start of `echo ab`   swaps `ec`
	//	^Y                      nothing killed yet          \a
	//	Delete                  at the end                  \a
	//	Delete                  on a character              deletes it
	//
	// Delete is undefined-key under `zsh -f`, so the bell there is an unbound
	// key's; the system startup file macOS ships binds it to delete-char,
	// which rings at the end of the line the same. Either way the end of the
	// line rings and the middle deletes, which is what this editor's Delete
	// does with this set. And `^D` at the end, listing nothing, rings — that
	// is the listing's, and rings in every dialect, as Tab matching nothing
	// does.
	BellRingsWhenAnEditFails bool

	// What the session keeps of the terminal settings a command leaves
	// behind (#6105). Every shell in the panel keeps a change a command made
	// and exited from: `stty -ixon` typed at the prompt lasts in zsh 5.9.2,
	// bash 5.3, ksh93 and dash. These three fields are where they part.
	// Measured 2026-10-05 through a pty:
	//
	//	                                   zsh   bash  ksh93
	//	sh -c 'stty -icanon'  next sees    icanon icanon -icanon
	//	sh -c 'stty -echo'    next sees    echo  -echo  -echo
	//	sh -c 'stty -ixon; kill -TERM $$'  -ixon ixon  ixon
	//
	// A command a stop suspended keeps nothing in any of the three. zsh's
	// `ttyctl -f` keeps nothing at all until `ttyctl -u`. See
	// interp.Runner.TerminalFrozen.
	//
	// KeptCanonical turns line buffering back on in what is kept.
	KeptCanonical bool
	// KeptEcho turns echo back on in what is kept.
	KeptEcho bool
	// KeepsWhatASignalLeft keeps the settings of a command a signal ended,
	// where the other shells put back what was there before it.
	KeepsWhatASignalLeft bool

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

	// ListMatchesOnASecondKeyOption names the option under which a second
	// completion key in a row lists an ambiguous word's matches. Empty is a
	// dialect whose second key always lists them, which is bash's answer.
	// Named, it is zsh's BASH_AUTO_LIST: on, the second key lists and the
	// first does not whatever ListMatchesWithoutASecondKeyOption says; off,
	// the second key lists nothing either. Measured 2026-10-06 against zsh
	// 5.9.2 — see editor.listsOn for the table.
	ListMatchesOnASecondKeyOption string

	// FillStandsAsideOption names the option under which a completion key
	// that fills in what an ambiguous word's matches agree on does nothing
	// else: no bell, no listing, and the key after it counts as the first
	// in a row rather than the second. zsh's LIST_AMBIGUOUS, which only does
	// this while AUTO_LIST or BASH_AUTO_LIST is on; with the option off, or
	// neither of those on, a fill is a first key like any other ambiguous
	// completion — the bell, and the listing where the first key lists.
	// Measured 2026-10-06 against zsh 5.9.2 — see fillRule for the
	// table. Empty is a dialect with no such option, whose fill rings as
	// BellRingsOnAnAmbiguousCompletionThatInserts says, never lists, and
	// is the first key of a row.
	FillStandsAsideOption string

	// MenuOnARepeatedCompletionOption names the option under which a
	// completion key pressed again on a word the last one left ambiguous
	// starts a menu completion — the first match in the line, and each
	// press after it the next — rather than listing the matches again:
	// zsh's AUTO_MENU, on by default. MenuOnTheFirstCompletionOption names
	// the one that starts the menu on the first press: zsh's MENU_COMPLETE.
	// Empty is a dialect with no such option, whose Tab never starts one.
	// Measured 2026-10-06 against zsh 5.9.2 (#6197) — see menuReason in
	// completemenu.go for the table and for why the repeat waits for a
	// listing.
	MenuOnARepeatedCompletionOption string
	MenuOnTheFirstCompletionOption  string

	// BellRingsWhenAMenuStarts sounds the bell on the keystroke that starts
	// a menu completion, by whatever route, and not on those that walk it.
	// Measured 2026-10-06: zsh 5.9.2 writes `\a` before the first match on
	// every route, and bash 5.3's `menu-complete` writes none.
	BellRingsWhenAMenuStarts bool

	// ListPackedOption names the option that lets each column of a listing
	// be as wide as its own longest match, where that takes fewer rows, and
	// ListRowsFirstOption the one that fills a listing across its rows
	// rather than down its columns. Measured on zsh 5.9.2, 2026-10-05: see
	// arrange in completelist.go for the rows each one draws. Named rather
	// than given as values for the reason ListMatchesWithoutASecondKeyOption
	// is: a person sets them at the prompt. Empty is a dialect with neither,
	// and the listing is laid down its columns with one width for all.
	ListPackedOption    string
	ListRowsFirstOption string

	// ListTypesOption names the option under which a listing of files draws
	// each one with the mark `ls -F` gives it — `/` a directory, `@` a link,
	// `*` an executable, `|` a FIFO, `=` a socket, `%` and `#` a device —
	// and under whose absence it draws none, the directory's slash
	// included, though the column a mark would take is kept. Measured
	// 2026-10-05 against zsh 5.9.2 with no completion system loaded, `ls
	// <TAB>` over a link to a directory, an executable, a FIFO, a link to a
	// file, a file and a directory:
	//
	//	LIST_TYPES set    dlink@  exe*    fifo|   link@   plain   sub/
	//	LIST_TYPES unset  dlink   exe     fifo    link    plain   sub
	//
	// and the link to a directory is inserted as a directory either way,
	// `ls dl<TAB>` giving `ls dlink/`, so the mark is the listing's and not
	// the word's. Empty is a dialect with no such option, whose listing
	// draws a directory's slash and nothing else. See FileTypeMark.
	ListTypesOption string

	// ListReturnsToTheLineOption names the option under which a listing is
	// followed by the cursor going back up to the line, the listing staying
	// on the screen below it until the line ends — zsh's ALWAYS_LAST_PROMPT,
	// on by default (#6129). Empty is a dialect without it, whose listing is
	// followed by the line drawn again under it. Named rather than a value
	// because a person sets it at the prompt. See editor.returnToTheLine.
	ListReturnsToTheLineOption string

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

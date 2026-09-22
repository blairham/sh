// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// HistoryStyle is what a dialect does with the session's history: how it draws
// an incremental search, and what it declines to remember.
//
// Separate from EditorStyle, which is about the line being typed. This is
// about the list behind it, and the two halves are here together because they
// are one surface: the knobs decide what is in the list, and the search is how
// a person reaches it.
//
// Every field's zero value is the substrate's own answer — a search drawn
// inline in wording that names no shell, and nothing filtered — which is what
// a front end that has said nothing gets.
type HistoryStyle struct {
	// SearchPrompt is what replaces the prompt, or is drawn under the line,
	// while a reverse incremental search is running. One verb: the query
	// typed so far.
	//
	// Measured under a pty on 2026-09-05, bash 5.3.15 and zsh 5.9.2, each
	// with four lines of history and `C-r e c h o` typed at the prompt:
	//
	//	bash   (reverse-i-search)`echo': echo two
	//	zsh    P> echo two
	//	       bck-i-search: echo_
	//
	// which is two differences and not one. The wording differs, and so does
	// where it goes: bash puts it where the prompt was and zsh keeps the
	// prompt, drawing the search on a row of its own below the line. See
	// SearchBelowTheLine.
	SearchPrompt string

	// SearchFailedPrompt is the same thing once nothing older matches.
	//
	//	bash   (failed reverse-i-search)`echo': echo one
	//	zsh    failing bck-i-search: echo_
	//
	// Both keep the last line that did match on the screen, and both ring the
	// bell. Neither undoes the query, so the next backspace goes back to a
	// search that works.
	SearchFailedPrompt string

	// SearchBelowTheLine draws the search on its own row under the command
	// line, leaving the prompt and the line where they were. zsh does; bash
	// replaces the prompt instead.
	//
	// It is a field rather than a consequence of the wording because it is a
	// separate decision: a shell could word it either way in either place,
	// and reading it off the presence of a `%s` would be inferring a layout
	// from a sentence.
	SearchBelowTheLine bool

	// How the history *file* encodes an entry, which is two facts and not
	// one — they were measured separately and one shell has both while the
	// default has neither.
	//
	// Measured 2026-09-12 by driving zsh 5.9.2 through a pseudo-terminal,
	// typing a `for` loop and a one-liner, once with `setopt
	// EXTENDED_HISTORY` and once without, and reading the file it left.
	//
	// EntriesContinueOnABackslash says a physical line ending in a backslash
	// is joined to the one after it, so an entry may span several lines. This
	// is what a multi-line command is stored as, and it appears in **both**
	// files — with the option and without — which is why it is its own field
	// rather than part of the timestamp answer:
	//
	//	for i in 1 2\
	//	do\
	//	echo $i\
	//	done
	//
	// A reader that handled only the header would still hand back `done` as an
	// entry of its own, which is what this shell did.
	EntriesContinueOnABackslash bool

	// EntriesMayCarryATimestampHeader says an entry may begin with two
	// numbers and a `;` before the command — when the shell was told to record
	// when each line ran:
	//
	//	: 1789247571:0;go run ./cmd/zsh
	//
	// *May*, not does: a file usually holds both kinds, because the option can
	// be turned on part-way through a file's life and a session that writes
	// without it appends bare lines to a file full of headers. So this is
	// permission to read one, and a line that does not match is the command
	// itself.
	//
	// Only the first `;` after the numbers ends the header, measured: `:
	// 1789253048:0;print 'a;b'` is one entry whose command contains a `;`.
	//
	// The other shell with a history file writes its times differently — a
	// `#<epoch>` line *before* the command — so this is not a constant the
	// substrate could hold. It is here for the reason SearchPrompt is.
	EntriesMayCarryATimestampHeader bool

	// HashTimestampLines is that other spelling: a whole physical line of
	// `#` and digits *before* the command, which is what bash writes when it
	// was told to record when each line ran, and what it leaves out of the
	// list when it reads one back.
	//
	// Measured 2026-09-21, bash 5.3.20, `env -i` with a scratch HOME, one
	// file shape at a time read by `history -r` and listed with `history`.
	// The fact worth stating is that the rule is **decided per read, by the
	// first line of what is being read**, and is not a line-by-line test:
	//
	//	#1699999999 / echo a / #1700000000 / echo b     echo a, echo b
	//	echo a / #1700000000 / echo b                   all three, the
	//	                                                `#` line an entry
	//	#x / echo a / #1700000000 / echo b              all four
	//
	// and it does not carry between reads — reading the first file and then
	// the second in one shell keeps the second file's `#` line. So a file
	// that opens with one of these is a file of them, and a file that does
	// not is a file where `#` is the first character of a command somebody
	// typed. Reading it line by line would eat that command.
	//
	// What counts as one is narrow, and every part of it was measured: the
	// `#` is the first character of the line — `  #1700000000` is a command —
	// and the character after it is a digit. `#1abc` and `#1700000000 extra`
	// are headers; `#`, `#-5`, `# 1700000000` and `#comment here` are not.
	//
	// A dangling one at the end of a header file, with no command after it,
	// is dropped with the rest: `#1 / echo a / #2` is one entry.
	//
	// A policy rather than a flag, because the shell that has these lines
	// reads them **three** ways and which one is in force is not a property
	// of the dialect — it is whether that shell was told to record when each
	// line ran. See [HashTimestampLines] for the three and for where the
	// third was measured.
	HashTimestampLines HashTimestampLines

	// HashTimestampsVariable names the shell variable whose being set moves
	// [HashTimestampLines] up to [HashTimestampLinesAndEntriesSpanThem] for
	// as long as it is set.
	//
	// A variable name rather than a rule, because which reading is in force
	// is not a property of the dialect: the shell that has these lines reads
	// a file one way when it was told to record when each line ran and
	// another way when it was not, and a script turns that on and off at
	// will. The substrate cannot know the name — that is the dialect's — and
	// the dialect cannot reach the session's reader, so the name is what
	// crosses.
	//
	// Empty is a style whose answer does not move, which is every dialect
	// but one.
	HashTimestampsVariable string

	// EmptyLinesAreEntries says a physical line with nothing on it is a
	// command of its own rather than a gap in the file, so reading the file
	// back puts an empty entry in the list.
	//
	// The panel parts over it, which is the only reason this is a field.
	// Measured 2026-09-21, `env -i` with a scratch HOME, the same files read
	// by each shell:
	//
	//	bash 5.3.20   drops the blank, wherever it falls — first, last,
	//	              between two commands, two in a row, and inside a file
	//	              of `#` time lines. All three read routes agree: `-r`,
	//	              `-n`, and the read at the first `set -o history`.
	//	zsh 5.9.2     keeps it, through `fc -R` and through the file
	//	              `$HISTFILE` names, with and without EXTENDED_HISTORY
	//	              headers on the lines around it.
	//	ksh93u+       no text encoding to ask: `$HISTFILE` is a binary
	//	              format, and pointing it at a file of lines overwrote it
	//	              with a two-byte magic mid-probe.
	//
	// So the zero value — an empty line is not an entry — is bash's answer
	// and also the substrate's own, which has dropped blanks since the first
	// read: a blank is what a history file is most often cluttered with and
	// is never worth walking back through. zsh is the one that has to say
	// something, and it says it rather than inheriting bash's (#4024).
	//
	// **Empty, not blank**, wherever this is false. bash lists a line of
	// spaces and a line of one tab as entries of their own — measured — and
	// it is the same line bash draws for the lines a script *runs*, where a
	// line of blanks joins the list and a truly empty one does not. Testing
	// a trimmed line here, which this shell used to, is a rule wider than
	// any shell's and drops what bash keeps.
	//
	// The drop happens **after** the continuation lines have been joined,
	// never before: an empty line at the end of a multi-line command is part
	// of that command, and removing it first would leave the backslash in
	// front of it dangling and swallow the entry after. That ordering costs
	// bash nothing — it has no continuation encoding — but there is one
	// decoder, and the shell it would break is the shell that has one.
	EmptyLinesAreEntries bool

	// Control names the variable holding a colon-separated list of what not
	// to record — `ignorespace`, `ignoredups`, `ignoreboth`. bash calls it
	// HISTCONTROL. zsh spells the same two rules as options rather than as a
	// variable, which is IgnoreSpaceOption and IgnoreDupsOption below.
	//
	// Empty means this dialect has no such variable, and setting one by that
	// name changes nothing. Measured: ksh93 with HISTIGNORE set records the
	// lines anyway, and a variable a shell does not have is not a variable
	// that half works.
	Control string

	// IgnoreSpaceOption and IgnoreDupsOption name the *options* a dialect
	// spells these same two rules as, for a shell that keeps them in its
	// option namespace instead of in a variable. zsh's are HIST_IGNORE_SPACE
	// and HIST_IGNORE_DUPS.
	//
	// Two fields beside Control rather than one field with a mode, because
	// they are two namespaces and not two spellings: a variable is read with
	// GetVar and an option with DialectOption, the second folds case and
	// underscores where the first does not, and a shell can have one, the
	// other or neither. bash has the variable and no such options; zsh has
	// the options and no such variable. Neither is a translation of the
	// other, so neither is written as one.
	//
	// The name is passed to the dialect's namespace exactly as spelled here,
	// so the table may hold the spelling the shell's own documentation uses.
	// Empty is a dialect with no such option, and the rule is then off unless
	// Control turns it on.
	IgnoreSpaceOption string
	IgnoreDupsOption  string

	// Ignore names the variable holding the patterns a recorded line must not
	// match — HISTIGNORE in bash, HISTORY_IGNORE in zsh. Empty is a dialect
	// with none.
	Ignore string

	// IgnoreIsOnePattern reads that variable as a single pattern rather than
	// as a colon-separated list of them. zsh's HISTORY_IGNORE is one;
	// bash's HISTIGNORE is a list.
	IgnoreIsOnePattern bool

	// PatternIgnoredStaysInSession keeps a line the *pattern* knob rejected
	// in the list the up arrow walks, and leaves it out of the file only.
	//
	// This is the panel's one conflict on identical intent, and re-measuring
	// it narrowed it to this knob alone. On 2026-09-06, zsh 5.9.2, one shell,
	// three probes, `fc -l` read before exiting and the file read after:
	//
	//	HISTORY_IGNORE='echo hidden'    fc -l LISTS `echo hidden`; the file
	//	                                does not have it
	//	setopt HIST_IGNORE_SPACE        fc -l does NOT list ` echo hidden`
	//	setopt HIST_IGNORE_DUPS         fc -l does NOT list the repeat
	//
	// So zsh gives *both* answers, and which one it gives is decided by the
	// knob rather than by the shell. bash 5.3.15, bash 3.2.57 and bash-as-sh
	// drop the line from `history` under every one of their three rules, so
	// the panel agrees about a blank and a duplicate and disagrees only about
	// a pattern — which is why the space and dups rules take no axis at all
	// and this one field carries the whole of the difference.
	//
	// The earlier reading of this had it as a property of the dialect and
	// attributed the "kept" observation to HIST_IGNORE_SPACE. It is written
	// down that way in this repository's own history: docs/spec/history.md
	// said so, and so did the issue it came from. Two knobs were measured in
	// one session and the answer of one was recorded against the other.
	PatternIgnoredStaysInSession bool
}

// InForce is this style as it stands for a shell whose variables are these.
//
// Only [HashTimestampsVariable] moves anything today, and the method exists
// rather than a naked lookup at each reader so that a second variable-borne
// answer has one place to go. A style naming no variable is returned as it
// stands.
func (h HistoryStyle) InForce(get func(string) (string, bool)) HistoryStyle {
	if h.HashTimestampsVariable == "" || get == nil {
		return h
	}
	if _, ok := get(h.HashTimestampsVariable); ok {
		h.HashTimestampLines = HashTimestampLinesAndEntriesSpanThem
	}
	return h
}

// The substrate's own search wording, for a front end that has not said. It
// names no shell on purpose: the core does not know its successors, and a
// default borrowed from one of them would make the others look like
// deviations from it.
const (
	defaultSearchPrompt = "(reverse-search)`%s': "
	defaultSearchFailed = "(failed reverse-search)`%s': "
)

// HashTimestampLines is how a history file's `#<digits>` lines are read, and
// there are three answers rather than two.
//
// The third is why this is a policy and not a flag. bash's reading of such a
// file depends on whether that shell was told to record when each line ran —
// HISTTIMEFORMAT — and that is a variable a script sets and unsets, not a
// property of the dialect. So the dialect's reader asks the question of the
// runner each time it opens a file and states the answer here.
//
// Measured 2026-09-22, bash 5.3.20, `env -i` with a scratch HOME, one file
// shape at a time read by `history -r` and listed with `history` after
// HISTTIMEFORMAT was put back:
//
//	file                      HISTTIMEFORMAT unset   HISTTIMEFORMAT set
//	#1 a #2 b                 a · b                  a · b
//	a #1 b                    a · #1 · b             a · b
//	#1 a extra #2 c           a · extra · c          "a\nextra" · c
//	lead #1 a extra #2 c      lead · #1 · a ·        lead · a · extra · c
//	                          extra · c
//
// Row three is the one that pays for the third answer: the lines after a
// command belong to it, so a command that spans lines comes back as the one
// entry somebody typed rather than as one entry per line. Row four says the
// spanning still turns on the file's own first line — a file that does not
// open with a header is read a line at a time even with the variable set —
// while the headers themselves come out of it either way.
type HashTimestampLines int

const (
	// NoHashTimestampLines is the substrate's own answer and every dialect's
	// but one: a `#` is the first character of a command somebody typed.
	NoHashTimestampLines HashTimestampLines = iota

	// HashTimestampLinesWhenTheFileOpensWithOne drops the header lines of a
	// file that begins with one, and leaves a file that does not alone. Each
	// remaining physical line is an entry.
	HashTimestampLinesWhenTheFileOpensWithOne

	// HashTimestampLinesAndEntriesSpanThem drops a header wherever it falls,
	// and — where the file opens with one — runs every line up to the next
	// header into the entry that header opened.
	//
	// The empty lines of such an entry are the entry's, except at the front:
	// measured, `#1`, two empty lines, `echo a`, two empty lines, `#2`,
	// `echo c` reads as `echo a` with two empty lines after it and then
	// `echo c`, and a stretch that is nothing but empty lines is no entry at
	// all. *Empty* rather than blank, as everywhere else here: a line of
	// spaces is text the entry keeps.
	HashTimestampLinesAndEntriesSpanThem
)

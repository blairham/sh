// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/blairham/sh/internal/histjoin"
	"github.com/blairham/sh/syntax"
)

// `fc`: the history list listed, and a command out of it run again.
//
// # Why this is in the core and not in a dialect
//
// The list is a dialect's — bash keeps it in a shell array under a name no
// script can reach, and zsh keeps its own beside it — but the *questions*
// `fc` asks of it are not. Which entry does `-2` name, where does a default
// range start and end, what does an operand out of range come to, does `-r`
// reverse before or after the range is built: none of those is answered by
// naming a shell, and all of them are answered once here. That is the same
// division [Runner.SetHistoryStore] already draws for history expansion,
// which indexes the dialect's list through the core without knowing whose it
// is, and `fc` reads it through the very same seam.
//
// Two things really are the dialect's and only two, and both are layout:
// bash writes `1\t echo one` and zsh writes `    1  echo one`, and under `-n`
// bash keeps the tab where zsh drops it. Those are a
// [Runner.SetHistoryListingLayout] away, beside the runner the way a script
// listing's arrangement is, and they are all `dialect/zsh` now says about
// listing — it used to carry a second lister of its own, which knew nothing
// of ranges, of `-n` or of `-r`, and answered `fc -l 1 2` with silence.
//
// # What it used to be
//
// A stub, whose comment said it was "honest about the history this shell does
// not keep". That premise went stale the day the `history` builtin landed: the
// list has been there ever since, `history` reads it back, and `fc` was the
// one thing that never looked. #4009.
//
// # Measured
//
// 2026-09-21 against bash 5.3.20, script files under `env -i` with a scratch
// `HOME` and `HISTFILE=/dev/null`, `set -o history` written and the entries
// planted with `history -s` or run outright. The numbers below are history
// numbers, `cur` is the number of the `fc` command itself, and `HISTIGNORE`
// is what decides whether the list holds that line at all — which is why both
// shapes were measured rather than one.
//
//	fc -l                the last 16 up to cur-1, `%d\t %s`
//	fc -nl               the same without the number, the tab kept
//	fc -lr               the same, newest first
//	fc -l 1 2            the range, ascending
//	fc -l 3 1            the range, descending — first > last reverses it
//	fc -l -2             cur-2 .. cur-1
//	fc -l -0             cur, which is the `fc` line where the list holds it
//	fc -l 99             out of range: the oldest entry .. cur-1
//	fc -l be             the newest entry beginning `be` .. cur-1
//	fc -l zzz            `fc: no command found` at 1
//	fc -s                cur-1, echoed to standard error and run
//	fc -s a=x            the same with every `a` in it replaced by `x`
//	fc -s -0             `fc: no command found` at 1 — it would be itself
//	fc -0                `fc: history specification out of range` at 1
//
// Two of those are worth naming because no reading of the manual gives them.
// An **absolute** number is in range only up to `cur-2`: `fc -l 25` on a list
// whose newest addressable entry is 25 lists the whole list rather than that
// one entry, measured on three different list lengths. And an absolute out of
// range does not clamp the way a relative one does — as `first` it comes to
// the oldest entry and as `last` it comes to `cur-1`, which is why the two
// roles pass different fallbacks below.

// # The other reading
//
// Every line above is bash's, and two of the questions have a second answer.
// [Semantics.FcEventOutOfRangeIsAnError] refuses an operand the list cannot
// reach where bash clamps it, and
// [Semantics.FcRelativeEventNeedsTheShellsOwnEventNumber] counts `-k` back
// from the shell's own event number — which a script has none of — rather
// than from the end of the list. Both were measured against zsh 5.9.2 on
// 2026-09-21 and both are carried on the vector rather than here, because
// neither is decided by naming a shell: a dialect whose `fc` had a current
// event to count from would want the first answer with the second one's
// refusals. #4018, and the measurements are on the two fields.

// fcListingLayout is how one entry of `fc -l` is written: the format with the
// number, and the format `-n` uses with the command alone.
//
// Measured 2026-09-21. bash 5.3.20 writes `1\t echo one` and `\t echo one`;
// zsh 5.9.2 writes `    1  echo one` and `echo one`, dropping the whitespace
// with the number rather than keeping it.
type fcListingLayout struct {
	numbered string
	bare     string
}

// fcBaseLayout is the core's, which is bash's and POSIX's shape: the history
// number, a tab, and the command.
var fcBaseLayout = fcListingLayout{numbered: "%d\t %s\n", bare: "\t %s\n"}

// SetHistoryListingLayout says how this shell's `fc -l` writes one entry —
// the format taking the history number and the command, and the format `-n`
// uses with the command alone.
//
// Beside the runner rather than on the semantics vector, for the reason
// [Runner.SetScriptListingLayout] is: it is an arrangement rather than a
// conflict about behavior, and every vector field is a value the presets are
// compared over. Empty formats leave the core's, which is the shape POSIX
// describes and bash writes.
func (r *Runner) SetHistoryListingLayout(numbered, bare string) {
	r.fcLayout = fcListingLayout{numbered: numbered, bare: bare}
}

func (r *Runner) fcListing() fcListingLayout {
	l := r.fcLayout
	if l.numbered == "" {
		l.numbered = fcBaseLayout.numbered
	}
	if l.bare == "" {
		l.bare = fcBaseLayout.bare
	}
	return l
}

// fcHistory is the list as one `fc` call sees it: the entries, the history
// number of the oldest, and the number of the `fc` command itself.
type fcHistory struct {
	entries []string
	first   int
	// cur is the `fc` call's own history number. It is the newest entry
	// where the reader recorded that line and one past the newest where it
	// did not, and the difference is visible: with `HISTIGNORE='fc*'` set,
	// `fc -l` on a four-entry list writes all four, and without it the same
	// call writes three and keeps its own line out.
	cur int
	// refuse is Semantics.FcEventOutOfRangeIsAnError, read once: an operand
	// the list cannot reach is refused rather than brought to the nearest
	// end.
	refuse bool
	// ownEvent is Semantics.FcRelativeEventNeedsTheShellsOwnEventNumber,
	// read once: `-k` counts back from the shell's own event number, which a
	// script has none of, rather than from the end of the list.
	ownEvent bool
	// newestIsCurrent is Semantics.FcNewestEntryIsTheCurrentLine, read once:
	// the newest entry is the line this shell is running, so the roads that
	// run one cannot reach it.
	newestIsCurrent bool
	// loose is Semantics.FcNumericOperandSkipsBlanksAndASign, read once: an
	// operand's digits may have whitespace and a `+` in front of them.
	loose bool
}

func (r *Runner) fcHistory(entries []string, rest []string) fcHistory {
	h := fcHistory{entries: entries, first: r.HistoryFirst()}
	h.cur = h.first + len(entries)
	if r.HistoryHasOwnLine() {
		h.cur--
	}
	// The three below are asked once per call rather than at each operand,
	// and all three are asked on every road: the reading of an operand is
	// the same reading whether `-l`, `-s` or an editor is what the entry is
	// wanted for, and each of them decides what a default comes to as well
	// as what a written operand does.
	h.refuse = r.ask(r.sem().FcEventOutOfRangeIsAnError,
		"`fc` given an event the history list does not hold")
	h.ownEvent = r.ask(r.sem().FcRelativeEventNeedsTheShellsOwnEventNumber,
		"`fc` given an event counted back from the current one")
	h.newestIsCurrent = r.ask(r.sem().FcNewestEntryIsTheCurrentLine,
		"`fc` asked to run the newest entry of the list")
	// The fourth is asked only where an operand puts the question, which is
	// the difference between it and the three above: those decide what every
	// operand and every default come to, and this one decides nothing at all
	// unless a word was written with whitespace or a `+` in front of its
	// digits. See fcOperandWantsTheLooseReading.
	for _, spec := range rest {
		if !fcOperandWantsTheLooseReading(spec) {
			continue
		}
		h.loose = r.ask(r.sem().FcNumericOperandSkipsBlanksAndASign,
			"`fc` given an operand with whitespace or a `+` in front of its digits")
		break
	}
	return h
}

// fcOperandWantsTheLooseReading reports a word the two readings of an
// operand's number disagree about: one whose digits stand behind whitespace
// or a `+`, so that one reading has a number there and the other has a word
// to search for.
//
// A word with neither is read the same way by both — `2x` is the event 2 and
// `abc` is a search in each — and a word with one of them but no digits
// behind it is a search in both.
func fcOperandWantsTheLooseReading(spec string) bool {
	s := strings.TrimLeft(spec, " \t\n\v\f\r")
	if s == spec && !strings.HasPrefix(s, "+") {
		return false
	}
	s = strings.TrimPrefix(s, "+")
	s = strings.TrimPrefix(s, "-")
	return s != "" && isDigit(s[0])
}

// last is the history number of the newest entry the list holds.
func (h fcHistory) last() int { return h.first + len(h.entries) - 1 }

// currentLine is the history number of the line this shell is running now,
// and it is the first number no re-run and no edit may reach.
//
// `cur` where the list numbers the `fc` call itself past its newest entry,
// and one below that where the newest entry *is* that line. See
// [Semantics.FcNewestEntryIsTheCurrentLine].
func (h fcHistory) currentLine() int {
	if h.newestIsCurrent {
		return h.cur - 1
	}
	return h.cur
}

// reachable is the newest event a re-run, an edit or a word operand's search
// may come to.
func (h fcHistory) reachable() int { return h.currentLine() - 1 }

// at is the entry that number names.
func (h fcHistory) at(n int) string { return h.entries[n-h.first] }

// clamp brings a number inside the list.
func (h fcHistory) clamp(n int) int { return min(max(n, h.first), h.last()) }

// fcEvent is what one `first` or `last` operand comes to: the history number
// it names before any clamping, and whether it named anything at all. A
// string operand that no entry begins with is the one that does not.
type fcEvent struct {
	num   int
	found bool
}

// resolve reads one operand, with def for an absent one and for an absolute
// number the list does not hold.
//
// The three shapes, in the order bash reads them: a minus sign and digits is
// an event counted back from `cur`, digits alone are a history number, and
// anything else is the newest command that *begins* with the word —
// measured, `fc -l echo\ a` on `echo ax`, `echo bx`, `echo ay` starts at
// `echo ay`, so it is the newest match and a prefix rather than a substring.
func (h fcHistory) resolve(spec string, def int) fcEvent {
	return h.resolveWith(spec, def, def)
}

// resolveWith is the same reading with the two fallbacks told apart: what an
// *absent* operand comes to, and what an absolute number the list does not
// hold comes to.
//
// One value served both while `-l` was the only caller, because there the two
// answers coincide — an absent `last` and an out-of-range `last` are both
// `cur-1`. The editor road is where they part company, measured 2026-09-21 on
// bash 5.3.20 over a three-entry list:
//
//	fc -e ed        the previous command alone — an absent `first` is cur-1
//	fc -e ed 99     the **oldest** entry — an out-of-range `first` is not
//	fc -e ed 1      `echo one` alone — an absent `last` is `first`
//	fc -e ed 1 99   the whole range to cur-1 — an out-of-range `last` is not
//
// So `first` absent is cur-1 and `first` out of range is the oldest, and
// `last` absent is whatever `first` came to and `last` out of range is cur-1.
// Four answers out of two operands, and a single `def` can carry two of them.
func (h fcHistory) resolveWith(spec string, absent, outOfRange int) fcEvent {
	if spec == "" {
		return fcEvent{num: absent, found: true}
	}
	if e, ok := h.countedBack(spec); ok {
		return e
	}
	if num := h.number(spec); num.ok && !num.dashed {
		n := num.n
		// The upper bound is cur-2 and not the newest entry, measured on
		// three list lengths: the command before this one cannot be named
		// by its number, only counted back to. Both bounds belong to the
		// clamping answer — where an event out of range is refused instead,
		// the number is kept and the *range* is what the roads below decide
		// about, because `fc -l 6 3` on five entries writes `5 4 3` where
		// `fc -l 6 6` refuses.
		if !h.refuse && (n < h.first || n > h.cur-2) {
			return fcEvent{num: outOfRange, found: true}
		}
		return fcEvent{num: n, found: true}
	}
	return h.search(spec)
}

// countedBack reads the operands that count back from a current event rather
// than naming one — the `-k` spelling, and the `0` that is a count of none
// rather than a history number — and says whether this was one of them.
//
// [Semantics.FcRelativeEventNeedsTheShellsOwnEventNumber] is the whole of the
// difference, and it is three differences at the surface. Measured 2026-09-21
// against zsh 5.9.2, `zsh -f` on a script file, the list planted with
// `print -s`, beside bash 5.3.20 on the same shapes:
//
//   - `-k` comes to the same place whatever k is, floored where the count
//     runs out: `fc -l -1`, `fc -l -2` and `fc -l -20` each write the whole
//     list, on five entries and on thirty, where bash's `-1` is the newest
//     entry and its `-2` the newest two. `fc -l -2 -2` refuses naming the
//     event `0` rather than `-2`, which is what the floor is visible as.
//   - `0` is the number zero, which is where that count already ended: `fc
//     -l 0` and `fc -l -1` write the same list, and `fc -l 0 0` and `fc -l
//     -1 -1` refuse alike. bash reads it as the command before this one.
//   - `-0` is not a count at all. It is a word, and no entry begins with it:
//     `fc -l -0` is `event not found: -0` where bash reads the same operand
//     as the `fc` line itself.
func (h fcHistory) countedBack(spec string) (fcEvent, bool) {
	num := h.number(spec)
	if h.ownEvent {
		if !num.ok || !num.dashed {
			return fcEvent{}, false
		}
		if num.n != 0 {
			// The floor rather than cur-k: there is no current event to
			// count back from, so every relative operand lands here, below
			// the oldest event this shell numbers.
			return fcEvent{num: fcBeforeTheOldest, found: true}, true
		}
		return h.search(spec), true
	}
	if num.dashed && num.ok {
		return fcEvent{num: h.cur - num.n, found: true}, true
	}
	if num.ok && num.n == 0 {
		// Zero is not a history number and is not read as one: it comes to
		// the command before this one, which is what an operand that was
		// never written comes to under `-s`. Measured, and it is the one
		// place `0` and `-0` part company — `fc -l 0` writes the previous
		// command and `fc -l -0` writes this one.
		return fcEvent{num: h.cur - 1, found: true}, true
	}
	return fcEvent{}, false
}

// fcOperandNumber is one operand read as a number: the digits, whether a
// minus sign stood in front of them, and whether there was a number there at
// all.
type fcOperandNumber struct {
	n      int
	dashed bool
	ok     bool
}

// number reads the number at the front of an operand.
//
// It is a **prefix** and not the whole word, in both readings and unasked:
// measured 2026-09-21, `fc -l 2x` starts at entry 2 in bash 5.3.20 and in
// zsh 5.9.2 alike, `fc -l 3abc` starts at 3 in both, and `fc -l 0x2` is the
// number zero in both — base ten, so the `x2` is what the digits stopped
// before rather than a radix. The core read the whole word with a strict
// parse until #4058 and so agreed with neither.
//
// What may stand in *front* of the digits is the conflict, and it is
// [Semantics.FcNumericOperandSkipsBlanksAndASign]: one reading takes the
// sign or the first digit at the front of the word, the other skips
// whitespace and takes a `+` as well.
//
// A number too large for an int is no number, which leaves the operand to
// the search the way a word with no digits at all does.
func (h fcHistory) number(spec string) fcOperandNumber {
	s := spec
	if h.loose {
		s = strings.TrimLeft(s, " \t\n\v\f\r")
	}
	var dashed bool
	switch {
	case strings.HasPrefix(s, "-"):
		dashed, s = true, s[1:]
	case h.loose && strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	digits := 0
	for digits < len(s) && isDigit(s[digits]) {
		digits++
	}
	if digits == 0 {
		return fcOperandNumber{}
	}
	n, err := strconv.Atoi(s[:digits])
	if err != nil {
		return fcOperandNumber{}
	}
	return fcOperandNumber{n: n, dashed: dashed, ok: true}
}

// refuseTheCurrentLine is [Semantics.FcNewestEntryIsTheCurrentLine] applied
// to an event a road is about to run or edit: the line this shell is standing
// on cannot be re-run, and neither can anything past it.
//
// Asked before the range refusal and not after, which is measured rather than
// tidy: `fc -s 99` on a five-entry list says this and not `no such event: 99`
// in the shell that says both.
func (h fcHistory) refuseTheCurrentLine(r *Runner, n int) (code int, ok bool) {
	if !h.newestIsCurrent || n < h.currentLine() {
		return 0, true
	}
	r.diagf("%s\n", Wording(r.diag().FcCurrentLineRecurses,
		"fc: the current history line would run itself again"))
	return 1, false
}

// fcBeforeTheOldest is the number a relative operand comes to where there is
// no current event to count back from. Below every history number a shell
// gives out, so a range that reaches the list clamps to its oldest entry and
// a range that does not is refused naming this.
const fcBeforeTheOldest = 0

// search is the word operand: the newest entry that begins with it, and no
// event at all when none does.
//
// It stops below the current line rather than at the newest entry, which is
// the same threshold the roads that run one use and is visible on `-l` too:
// measured 2026-09-21 on `ax bx ay by az`, zsh 5.9.2 answers `fc -l a` with
// `ay` onwards and not with `az` alone. See
// [Semantics.FcNewestEntryIsTheCurrentLine], and reachable, which is the
// newest entry in the other reading.
func (h fcHistory) search(spec string) fcEvent {
	for n := min(h.reachable(), h.last()); n >= h.first; n-- {
		if strings.HasPrefix(h.at(n), spec) {
			return fcEvent{num: n, found: true}
		}
	}
	return fcEvent{}
}

// defaultFirst is where a listing with no `first` operand starts.
//
// The two readings part here for the reason they part over `-k`, which is
// why one axis answers both: a shell with a current event takes the sixteen
// events below it, and a shell with none counts on the list instead and
// takes the newest seventeen entries. Measured against zsh 5.9.2: `fc -l` on
// thirty entries writes 14 through 30, on eighteen writes 2 through 18, and
// on seventeen or fewer writes all of them.
func (h fcHistory) defaultFirst() int {
	if h.ownEvent {
		return max(h.first, h.last()-16)
	}
	return h.cur - 16
}

// oneDefault is the entry a road wanting a single event takes when no operand
// named one.
//
// The command before this one, where there is a current event to count back
// from. Where there is not, the count back lands below the list and the
// default — unlike a written operand — is brought into it rather than
// refused: measured, `fc -s` on a five-entry list re-runs the *oldest* entry
// where `fc -s 0`, the same number written down, is refused.
func (h fcHistory) oneDefault() int {
	if h.ownEvent {
		return h.clamp(fcBeforeTheOldest)
	}
	return h.cur - 1
}

// refuseOutsideTheList is Semantics.FcEventOutOfRangeIsAnError applied to a
// resolved range: nothing is said while the range still meets an entry,
// however far past the list either end is.
//
// The two wordings are one measurement apart. Where the ends resolved to the
// same number there is an event to name and it is named; where they differ
// there is not, and the refusal says only that the range is empty.
func (h fcHistory) refuseOutsideTheList(r *Runner, from, to int) (code int, ok bool) {
	if max(from, to) >= h.first && min(from, to) <= h.last() {
		return 0, true
	}
	if from == to {
		r.diagf("%s\n", Wording(r.diag().FcNoSuchEvent, "fc: no such event: %[1]d", from))
		return 1, false
	}
	r.diagf("%s\n", Wording(r.diag().FcNoEventsInRange, "fc: no events in that range"))
	return 1, false
}

// span is the history numbers between two ends, in the order they are written
// out: ascending, or descending where the range was written backwards, and
// then turned around again by `-r`.
//
// Shared by `-l` and the editor road because both were measured to arrange
// themselves the same way — `fc -l 3 1` and `fc -e ed 3 1` both run 3, 2, 1,
// and `-r` reverses whichever order that produced rather than swapping the
// two ends.
func (h fcHistory) span(from, to int, reverse bool) []int {
	a, b := h.clamp(from), h.clamp(to)
	nums := make([]int, 0, max(a, b)-min(a, b)+1)
	if a <= b {
		for n := a; n <= b; n++ {
			nums = append(nums, n)
		}
	} else {
		for n := a; n >= b; n-- {
			nums = append(nums, n)
		}
	}
	if reverse {
		for i, j := 0, len(nums)-1; i < j; i, j = i+1, j-1 {
			nums[i], nums[j] = nums[j], nums[i]
		}
	}
	return nums
}

// fcOperands splits the words after the options into `first` and `last`.
func fcOperands(rest []string) (first, last string) {
	if len(rest) > 0 {
		first = rest[0]
	}
	if len(rest) > 1 {
		last = rest[1]
	}
	return first, last
}

// fcSubstitution is one `pat=rep` operand of `fc -s`.
type fcSubstitution struct{ pat, rep string }

// fcRerunOperands reads `fc -s`'s operands: the leading `pat=rep` words, and
// then the command to find.
//
// Every occurrence is replaced and not only the first — measured, `echo aaa`
// re-run as `fc -s a=x` writes `xxx` — and several substitutions apply in the
// order they were written, each to what the one before it left.
func fcRerunOperands(rest []string) (subs []fcSubstitution, spec string) {
	for _, word := range rest {
		if i := strings.IndexByte(word, '='); i > 0 && spec == "" {
			subs = append(subs, fcSubstitution{pat: word[:i], rep: word[i+1:]})
			continue
		}
		if spec == "" {
			spec = word
		}
	}
	return subs, spec
}

// noCommand is the operand that named nothing: a word no entry begins with.
// The word is passed because the dialect that refuses names it, where bash's
// wording names no operand at all and ignores what it is given.
func (h fcHistory) noCommand(r *Runner, spec string) int {
	r.diagf("%s\n", Wording(r.diag().FcNoCommandFound, "fc: no command found", spec))
	return 1
}

func biFc(r *Runner, ctx context.Context, args []string) int {
	rest, opts, optArg, code := r.builtinOptionsCountingBack("fc", args, "e:lnrs")
	if code != 0 {
		return code
	}
	entries := r.HistoryEntries()
	// Nothing to list, edit or re-run, which two of the columns answer with
	// silence and one with the event it could not find. Asked before the
	// reading below is, so that a shell answering this one with silence —
	// and a shell with no list at all, which is every call it ever gets — is
	// never asked how it would have read an operand.
	if len(entries) == 0 && !r.ask(r.sem().FcEmptyHistoryIsAnError, "`fc` with no history to answer from") {
		if r.unspecified {
			return 2
		}
		return 0
	}
	h := r.fcHistory(entries, rest)
	if len(entries) == 0 && !h.refuse {
		// Where an operand out of range is refused, an empty list is not a
		// case of its own: every range misses a list with nothing in it, and
		// the roads below already word that refusal — from the operands, so
		// the event named is the one that was asked for. Measured, `fc -l`
		// on an empty list is `no such event: 1`, `fc -l -1` on the same
		// list is `no such event: 0`, `fc -l 2 5` is `no events in that
		// range` and `fc -l zzz` is `event not found: zzz`.
		r.diagf("%s\n", Wording(r.diag().FcNoSuchEvent, "fc: no such event: %[1]d", h.first))
		return 1
	}
	switch {
	case strings.ContainsRune(opts, 'l'):
		return h.list(r, rest, strings.ContainsRune(opts, 'n'), strings.ContainsRune(opts, 'r'))
	case strings.ContainsRune(opts, 's'), optArg['e'] == "-":
		// `-e -` is `-s` written the POSIX way: re-run without an editor.
		return h.rerun(r, ctx, rest)
	default:
		return h.edit(r, ctx, rest, optArg['e'], strings.ContainsRune(opts, 'r'))
	}
}

// list is `-l`.
//
// The default range is the sixteen entries ending at the command before this
// one, and an operand replaces either end of it. A range written backwards
// prints backwards, and `-r` turns whichever order came out of that around —
// which is why the reversal is applied to the built sequence rather than to
// the two ends.
func (h fcHistory) list(r *Runner, rest []string, bare, reverse bool) int {
	firstOp, lastOp := fcOperands(rest)
	from := h.resolve(firstOp, h.defaultFirst())
	if !from.found {
		return h.noCommand(r, firstOp)
	}
	// An absent `last` never falls before `first`: measured, `fc -l -0` on a
	// list whose newest entry is this very command writes that one entry and
	// not a two-line range running backwards into it, where the `1` of
	// `fc -l 3 1` — written down — does run the range backwards. The same
	// rule is what makes `fc -l 99` name 99 in the other reading rather than
	// running a range backwards from it.
	to := h.resolve(lastOp, max(h.cur-1, from.num))
	if !to.found {
		return h.noCommand(r, lastOp)
	}
	if h.refuse {
		// Asked of the range and applied before the clamp, which is the one
		// ordering that answers both shells: bash's clamp makes every range
		// reach the list, so asking first would refuse nothing there, and
		// asking after it would refuse nothing in the other reading either.
		if code, ok := h.refuseOutsideTheList(r, from.num, to.num); !ok {
			return code
		}
	}
	nums := h.span(from.num, to.num, reverse)
	layout := r.fcListing()
	for _, n := range nums {
		if bare {
			_, _ = fmt.Fprintf(r.Out(), layout.bare, h.at(n))
			continue
		}
		_, _ = fmt.Fprintf(r.Out(), layout.numbered, n, h.at(n))
	}
	return 0
}

// rerun is `-s`: one entry, substituted, echoed and run.
//
// The echo goes to standard **error** — measured, `fc -s >/dev/null` still
// writes the command and `fc -s 2>/dev/null` does not — and the line takes
// the `fc` call's own place in the list rather than joining it after.
func (h fcHistory) rerun(r *Runner, ctx context.Context, rest []string) int {
	subs, spec := fcRerunOperands(rest)
	e := h.resolve(spec, h.oneDefault())
	if !e.found {
		return h.noCommand(r, spec)
	}
	// Ahead of everything below, because the shell that has this threshold
	// answers with it whatever else the operand was: the event written down,
	// the event past the end of the list, and the default on a list with
	// nothing below the line all come to the one sentence.
	if code, ok := h.refuseTheCurrentLine(r, e.num); !ok {
		return code
	}
	if h.refuse {
		if code, ok := h.refuseOutsideTheList(r, e.num, e.num); !ok {
			return code
		}
	} else if e.num >= h.cur || h.cur-1 < h.first {
		// Naming this very command is refused rather than run, which is
		// what `fc -s -0` is: `-0` counts back none of the way.
		return h.noCommand(r, spec)
	}
	line := h.at(h.clamp(e.num))
	for _, s := range subs {
		line = strings.ReplaceAll(line, s.pat, s.rep)
	}
	r.errf("%s\n", line)
	r.DropHistoryOwnLine()
	r.RecordHistoryEntry(line)
	return r.runSourced(ctx, line, sourced{
		eval:         true,
		label:        "fc",
		syntaxStatus: r.diag().SyntaxStatus(),
	})
}

// edit is `fc` with no `-l` and no `-s`: the entries go to an editor and what
// comes back is run.
//
// # Measured
//
// 2026-09-21 against bash 5.3.20 and zsh 5.9.2 under `env -i` with a scratch
// `HOME`, a scratch `TMPDIR` and the editor a recording stand-in on `PATH`.
// bash's list was seeded by running the commands with `set -o history`, zsh's
// with `print -s`, since zsh fills no list of its own in a script.
//
//	fc -e cat        the previous command through `cat`, then run
//	fc -e ed 1       `echo one` alone — an absent `last` is `first`, where
//	                 under `-l` an absent `last` is cur-1
//	fc -e ed 3 1     3, 2, 1 — a range written backwards is written backwards
//	fc -r -e ed 1 3  the same three, reversed again by `-r`
//	fc -e ed 1 -0    `fc: history specification out of range`, 1
//	fc -e ed zzz     `fc: no command found`, 1
//	fc -n -e ed 1 2  `-n` changes nothing: the file never carries numbers
//
// The entries go into the file **unnumbered**, one per line, and the file's
// name is the editor's last argument — so an editor sees `ed /tmp/…` and can
// read the name. bash names its file `bash-fc.XXXXXX` under `$TMPDIR` and zsh
// names its `zshXXXXXX` under `/tmp`; the two disagree, neither documents it,
// and nothing a script can do makes the spelling a promise, so ours is the
// one the rest of this interpreter's scratch already uses.
//
// # The editor, and its order
//
//	fc -e W with FCEDIT and EDITOR both set   W        both columns
//	FCEDIT=X EDITOR=Y fc                      X        both columns
//	EDITOR=Y fc                               Y        both columns
//	fc with neither set                       vi       both columns
//	set -o posix; fc with neither set         ed       bash; zsh has no
//	                                                   such option at all
//
// So the order is the `-e` operand, then `FCEDIT`, then `EDITOR`, then a
// fallback — and the fallback is `vi` rather than the `ed` the manuals and
// bash's own usage line suggest. It was measured rather than read for exactly
// that reason.
//
// The word is **split** before it runs, and the file is appended after the
// split: `fc -e 'cat -n'` runs `cat -n <file>` in both columns, so this is a
// command line and not a program name.
//
// One divergence is measured and deliberately not modeled: an *empty*
// `FCEDIT` falls through to `EDITOR` in bash — the `${FCEDIT:-…}` reading —
// where zsh takes the empty word and ends up trying to run the temporary file
// itself, `permission denied` at 1. bash's reading is the one POSIX describes
// and the one a script can rely on, and an axis for an empty variable nobody
// sets would cost a vector field to record a typo.
//
// # The three ways out
//
//	the editor exits non-zero      1, and nothing is echoed or run — both
//	                               columns, measured with an editor exiting 3
//	the editor cannot be run       the shell's own `command not found`, and
//	                               `fc` answers 1 rather than 127
//	the file comes back empty      bash: 0, silently, with nothing run. zsh:
//	                               `read error on <file>`, and it ends the
//	                               shell at 1 rather than leaving a status
//
// The empty file is the one conflict of the three, so it is an axis —
// [Semantics.FcEmptyEditIsAnError] — and not bash's answer with a zsh
// exception written into it. The other two agree across both columns.
//
// # What runs, and what is recorded
//
// The text is echoed to standard **error** and then run, which is the same
// pair `-s` does and was measured the same way: `fc -e cat >/dev/null` still
// writes the command and `2>/dev/null` does not. The line takes the `fc`
// call's own place in the history list rather than joining it after.
//
// # A line at a time, where the dialect reads a line at a time
//
// bash pushes the edited file onto its input stream, so it echoes the text a
// **line at a time as it reads it** and records each command it finds as an
// entry of its own. zsh reads the whole of it first: measured 2026-09-21,
// zsh 5.9.2 writes `echo A`, `echo B`, `A`, `B` where bash 5.3.20 writes
// `echo A`, `A`, `echo B`, `B` for the same two-command edit.
//
// That is the same split [Semantics.EvalRunsWhatItParsed] already records —
// bash runs the commands it has read before a later line fails to parse and
// zsh does not — so the granularity follows the axis rather than being a new
// one, and runSourced's own reader decides it. See sourced.read and #4030.
//
// The unit is the **line** and not the statement, which the compound case is
// what says: an editor leaving `for i in a b` / `do` / `echo $i` / `done`
// makes bash echo all four lines before running any of it, because all four
// had to be read to find the end of one command. `echo A; echo B` on one
// line is the same fact from the other side — one echo and one entry for two
// statements. A fix built on a statement's reconstructed text passes the
// two-command case and fails both of those.
//
// What the list holds is the lines joined the way the list joins them
// anywhere else — `for i in a b; do echo $i; done` — which is
// internal/histjoin's rule, shared with the history gate `driver` reads a
// script through. bash reaches both through one input stream, so there is
// one rule and not two.
func (h fcHistory) edit(r *Runner, ctx context.Context, rest []string, editor string, reverse bool) int {
	firstOp, lastOp := fcOperands(rest)
	// An absent `first` is the previous command and an out-of-range absolute
	// one is the oldest entry; an absent `last` is whatever `first` came to
	// and an out-of-range absolute one is the previous command. See
	// resolveWith for the four measurements.
	from := h.resolveWith(firstOp, h.oneDefault(), h.first)
	if !from.found {
		return h.noCommand(r, firstOp)
	}
	to := h.resolveWith(lastOp, from.num, h.cur-1)
	if !to.found {
		return h.noCommand(r, lastOp)
	}
	// `first` at or past the current line is refused; `last` past it is
	// brought under it and nothing is said. Measured 2026-09-21 on zsh
	// 5.9.2 over five entries: `fc 5` and `fc 5 5` are the refusal, `fc 1 5`
	// and `fc 1 99` and `fc 4 99` each edit up to entry 4. See
	// Semantics.FcNewestEntryIsTheCurrentLine.
	if code, ok := h.refuseTheCurrentLine(r, from.num); !ok {
		return code
	}
	if h.newestIsCurrent {
		to.num = min(to.num, h.reachable())
	}
	if h.refuse {
		// The other reading refuses the range rather than the line it would
		// have edited, and words it from the operands: measured, `fc -1` and
		// `fc 0 0` are both `fc: no such event: 0` at 1 where the wording
		// below is the clamping shell's.
		if code, ok := h.refuseOutsideTheList(r, from.num, to.num); !ok {
			return code
		}
	} else if from.num >= h.cur || to.num >= h.cur || h.cur-1 < h.first {
		// This very command, named at either end, is refused rather than run
		// — `fc -0` and `fc -e ed 1 -0` alike. The wording differs from the
		// one `-s` gives the same operand, which is why neither stands in
		// for the other: `fc -0` is `fc: history specification out of range`
		// where `fc -s -0` is `fc: no command found`.
		r.diagf("%s\n", Wording(r.diag().FcOutOfRange, "fc: history specification out of range"))
		return 1
	}
	var written strings.Builder
	for _, n := range h.span(from.num, to.num, reverse) {
		written.WriteString(h.at(n))
		written.WriteByte('\n')
	}
	path, err := r.fcSpool(ctx, written.String())
	if err != nil {
		r.diagf("fc: %s\n", err)
		return 1
	}
	// On every road out, including the one where the editor failed and the
	// one where the text would not parse.
	defer func() { _ = os.Remove(path) }()
	if st := r.fcRunEditor(ctx, editor, path); st != 0 {
		return st
	}
	edited, err := os.ReadFile(path)
	if err != nil {
		r.diagf("fc: %s\n", err)
		return 1
	}
	text := string(edited)
	if strings.TrimSpace(text) == "" {
		// An editor quit without saving, or one that emptied the file. bash
		// runs nothing and says nothing at 0; zsh complains, naming the
		// temporary file. See Semantics.FcEmptyEditIsAnError.
		if r.ask(r.sem().FcEmptyEditIsAnError, "an editor that left `fc`'s file empty") {
			// Raised without the builtin's name in front of it, which is
			// measured and is not how the same shell words its other `fc`
			// refusals: `fc -l zzz` is `z.sh:fc:3: no such event: 1` and this
			// one is `z.sh:3: read error on /tmp/zsh…`. The sentence is the
			// shell's complaint about a file rather than the builtin's about
			// an operand, and it reads as one. Cleared the way runSourced
			// clears it for the same reason.
			outer := r.inBuiltin
			r.inBuiltin = ""
			// And it ends the shell rather than leaving a status behind,
			// measured 2026-09-21 in zsh 5.9.2 three ways: `fc -e trunc ||
			// print caught` prints nothing and exits 1 — so `||` does not
			// catch it — a function around it does not either, and inside
			// `( )` it ends the subshell alone and the parent carries on at
			// 1. That is the shape Runner.fatal already has, and the same one
			// `NULLCMD=; >f` has in this dialect.
			r.fatal("%s\n", Wording(r.diag().FcEmptyEdit, "fc: %[1]s: nothing to run", path))
			r.inBuiltin = outer
			return r.status
		}
		if r.unspecified {
			return 2
		}
		return 0
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	r.DropHistoryOwnLine()
	return r.runSourced(ctx, text, sourced{
		eval:         true,
		label:        "fc",
		syntaxStatus: r.diag().SyntaxStatus(),
		read:         r.fcRead,
	})
}

// fcRead echoes and records one group of lines the reader has consumed.
//
// The echo goes to standard **error**, which is the same stream `-s` writes
// its one line to and was measured the same way: `fc -e cat >/dev/null` still
// writes the command and `2>/dev/null` does not.
func (r *Runner) fcRead(b borrowedLines) {
	r.errf("%s", b.text)
	if b.whole {
		// Read through before any of it ran, so there were no boundaries to
		// record it at: the text is one entry, which is what this road did
		// before it read anything a line at a time and is what every
		// single-command edit comes to either way. Nothing measures the
		// multi-command shape here — see borrowedLines.whole.
		r.RecordHistoryEntry(strings.TrimSuffix(b.text, "\n"))
		return
	}
	lines := strings.Split(strings.TrimSuffix(b.text, "\n"), "\n")
	var entry histjoin.Entry
	for i, line := range lines {
		if b.body < 0 || i < b.body {
			// A line the reader stepped over on its way to a command is an
			// entry of its own — measured, an editor leaving `# c` / `echo
			// A` makes bash record two. Where it holds nothing at all it is
			// no entry, which is RecordHistoryEntry's one guard and is
			// emptiness rather than blankness: a line of three spaces is an
			// entry there too.
			r.RecordHistoryEntry(line)
			continue
		}
		open := ""
		if i > b.body {
			open = openAfter(lines[b.body:i], b.dialect)
		}
		entry.Add(line, open)
	}
	if entry.Len() > 0 {
		r.RecordHistoryEntry(entry.Take())
	}
}

// openAfter is what the lexer is still inside having read these lines, which
// is what the line after them begins inside.
//
// Asked of the parser rather than worked out here, which is the rule
// driver's history gate already states: `$'`, a backquote, a `'` inside a
// double-quoted string and a here-document body are four separate answers and
// the lexer holds all of them. The gate feeds one parser a line at a time and
// can simply ask it between two lines; `fc` reads the editor's text through
// one parser over the whole of it, so the question goes to a second parser
// over the lines before the boundary. The text is one person's edit buffer,
// which is what makes reading it again affordable.
func openAfter(lines []string, d syntax.Dialect) string {
	p := syntax.NewParser(strings.Join(lines, "\n")+"\n", d)
	p.Parse()
	return p.OpenQuote()
}

// fcSpoolSeq numbers the files this builtin writes, so that two `fc` calls in
// one process — or in two Runners sharing one — never name the same one.
var fcSpoolSeq atomic.Uint64

// fcSpool writes the chosen entries where an editor can open them, and hands
// back the name it chose.
//
// # Where, and under what name
//
// `r.tempHome()`, which is this Runner's `TMPDIR` and not the process's, for
// the reason the whole of interp reads variables that way: two Runners in one
// program must be separable, and a script that moved `TMPDIR` moved its own
// scratch and nobody else's. `os.CreateTemp` is forbidden in this tree and
// this is one of the cases it is forbidden for — an empty directory argument
// there resolves through `os.TempDir`, which is the *process* environment.
// `heredocReader` names its spool the same way.
//
// `O_EXCL` and `0600`, which is what bash and zsh both leave behind — a
// history entry is what somebody typed, and the editor is the only other
// program meant to see it.
//
// # Why the gate is asked nothing
//
// This is the shell's own scaffolding and sits on the same side of the line
// [ActionOpen] already draws for a process substitution's pipe and for the
// file `=(cmd)` writes: the path is chosen by the interpreter and never by
// the script, so a policy refusing it would refuse `fc` itself while
// believing it had refused an access — and the diagnostic would name a path
// the operator has never seen. The open is **recorded** all the same, as an
// EventAccess, so an audit says the shell wrote one and when.
//
// What is worth refusing is refused, and it is not this file: the editor is
// an [ActionExec] the gate is asked about in the ordinary way, before it
// runs, by Runner.exec — see the `builtin/fc-editor` row in
// internal/sandboxcheck, which grades exactly that.
func (r *Runner) fcSpool(ctx context.Context, text string) (string, error) {
	path := filepath.Join(r.tempHome(),
		".sh-fc-"+strconv.Itoa(os.Getpid())+"-"+
			strconv.FormatUint(fcSpoolSeq.Add(1), 10))
	r.emit(ctx, Event{Kind: EventAccess, Action: r.act(Action{
		Kind: ActionOpen, Path: path, Write: true,
	})})
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// fcEditorWords is the command line `fc` runs over the file, split into
// words.
//
// The order is the `-e` operand, `FCEDIT`, `EDITOR`, and then a fallback —
// each taken only when it is **non-empty**, which is the `${FCEDIT:-…}`
// reading bash has. See the note on [fcHistory.edit] for zsh's answer to an
// empty one and for why it is not an axis.
//
// The fallback is `vi`, measured in both columns that have the construct, and
// `ed` while `set -o posix` is on — which is bash's alone, since zsh has no
// such option to turn on.
//
// Split on whitespace rather than on `IFS`. Nothing in the panel exposes the
// difference from a builtin's own reading of a variable, and a shell whose
// `IFS` no longer holds a space has bigger problems than its editor.
func (r *Runner) fcEditorWords(given string) []string {
	fcedit, _ := r.getVar("FCEDIT")
	editor, _ := r.getVar("EDITOR")
	for _, word := range []string{given, fcedit, editor} {
		if fields := strings.Fields(word); len(fields) > 0 {
			return fields
		}
	}
	if r.posixMode {
		return []string{"ed"}
	}
	return []string{"vi"}
}

// fcRunEditor runs the editor over the spooled file, and reports what `fc`
// should answer when it did not work.
//
// Zero means carry on. One is what both columns answer for an editor that
// exited non-zero and for an editor that could not be run at all — the latter
// measured as the shell's ordinary `command not found`, on the script's own
// line, with `fc` answering 1 rather than the 127 the search produced.
func (r *Runner) fcRunEditor(ctx context.Context, editor, path string) int {
	argv := append(r.fcEditorWords(editor), path)
	if err := r.exec(ctx, argv, r.environ()); err != nil {
		r.diagf("fc: %v\n", err)
		return 1
	}
	if r.status != 0 {
		return 1
	}
	return 0
}

func init() { builtins["fc"] = biFc }

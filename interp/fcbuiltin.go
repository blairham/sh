// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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
}

func (r *Runner) fcHistory(entries []string) fcHistory {
	h := fcHistory{entries: entries, first: r.HistoryFirst()}
	h.cur = h.first + len(entries)
	if r.HistoryHasOwnLine() {
		h.cur--
	}
	return h
}

// last is the history number of the newest entry the list holds.
func (h fcHistory) last() int { return h.first + len(h.entries) - 1 }

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
	if spec == "" {
		return fcEvent{num: def, found: true}
	}
	if isDashNumber(spec) {
		k, _ := strconv.Atoi(spec[1:])
		return fcEvent{num: h.cur - k, found: true}
	}
	if n, err := strconv.Atoi(spec); err == nil && n >= 0 {
		if n == 0 {
			// Zero is not a history number and is not read as one: it comes
			// to the command before this one, which is what an operand that
			// was never written comes to under `-s`. Measured, and it is
			// the one place `0` and `-0` part company — `fc -l 0` writes the
			// previous command and `fc -l -0` writes this one.
			return fcEvent{num: h.cur - 1, found: true}
		}
		// The upper bound is cur-2 and not the newest entry, measured on
		// three list lengths: the command before this one cannot be named
		// by its number, only counted back to.
		if n < h.first || n > h.cur-2 {
			return fcEvent{num: def, found: true}
		}
		return fcEvent{num: n, found: true}
	}
	for n := min(h.cur-1, h.last()); n >= h.first; n-- {
		if strings.HasPrefix(h.at(n), spec) {
			return fcEvent{num: n, found: true}
		}
	}
	return fcEvent{}
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

func (h fcHistory) noCommand(r *Runner) int {
	r.diagf("%s\n", Wording(r.diag().FcNoCommandFound, "fc: no command found"))
	return 1
}

func biFc(r *Runner, ctx context.Context, args []string) int {
	rest, opts, optArg, code := r.builtinOptionsCountingBack("fc", args, "e:lnrs")
	if code != 0 {
		return code
	}
	entries := r.HistoryEntries()
	if len(entries) == 0 {
		// Nothing to list, edit or re-run, which two of the columns answer
		// with silence and one with the event it could not find.
		if r.ask(r.sem().FcEmptyHistoryIsAnError, "`fc` with no history to answer from") {
			r.diagf("%s\n", Wording(r.diag().FcNoSuchEvent, "fc: no such event: 1"))
			return 1
		}
		if r.unspecified {
			return 2
		}
		return 0
	}
	h := r.fcHistory(entries)
	switch {
	case strings.ContainsRune(opts, 'l'):
		return h.list(r, rest, strings.ContainsRune(opts, 'n'), strings.ContainsRune(opts, 'r'))
	case strings.ContainsRune(opts, 's'), optArg['e'] == "-":
		// `-e -` is `-s` written the POSIX way: re-run without an editor.
		return h.rerun(r, ctx, rest)
	default:
		return h.edit(r, rest)
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
	from := h.resolve(firstOp, h.cur-16)
	if !from.found {
		return h.noCommand(r)
	}
	// An absent `last` never falls before `first`: measured, `fc -l -0` on a
	// list whose newest entry is this very command writes that one entry and
	// not a two-line range running backwards into it, where the `1` of
	// `fc -l 3 1` — written down — does run the range backwards.
	to := h.resolve(lastOp, max(h.cur-1, from.num))
	if !to.found {
		return h.noCommand(r)
	}
	a, b := h.clamp(from.num), h.clamp(to.num)
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
	e := h.resolve(spec, h.cur-1)
	if !e.found || e.num >= h.cur || h.cur-1 < h.first {
		// Naming this very command is refused rather than run, which is
		// what `fc -s -0` is: `-0` counts back none of the way.
		return h.noCommand(r)
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

// edit is `fc` with no `-l` and no `-s`: the entry goes to an editor and what
// comes back is run.
//
// The editor is not here. What is here is the refusal that comes before it,
// because that is the half a script can see without one: `fc -0` names this
// command and is `fc: history specification out of range` at 1, where the
// same operand under `-s` is `fc: no command found`. Two shapes of the same
// fact and the panel words them differently, so neither stands in for the
// other. The rest — writing the entry to a file, running `${FCEDIT:-…}` over
// it and running the result — is filed rather than guessed at.
func (h fcHistory) edit(r *Runner, rest []string) int {
	firstOp, _ := fcOperands(rest)
	e := h.resolve(firstOp, h.cur-1)
	if !e.found {
		return h.noCommand(r)
	}
	if e.num >= h.cur || h.cur-1 < h.first {
		r.diagf("%s\n", Wording(r.diag().FcOutOfRange, "fc: history specification out of range"))
		return 1
	}
	return 0
}

func init() { builtins["fc"] = biFc }

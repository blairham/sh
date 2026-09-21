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
			return fcEvent{num: outOfRange, found: true}
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
// Known and not modeled, with the measurement so it can be finished later:
// bash echoes the edited text a **line at a time as it reads it**, so two
// separate commands come out as `echo A`, `A`, `echo B`, `B` interleaved,
// and each is recorded as a history entry of its own. This writes the whole
// text once and records it as one entry. The two are identical for a
// single-command edit — which is every case above, and the only shape
// `share/suite`'s `history.tests` uses — and they differ only when the editor
// leaves two or more top-level commands behind. See #4030.
func (h fcHistory) edit(r *Runner, ctx context.Context, rest []string, editor string, reverse bool) int {
	firstOp, lastOp := fcOperands(rest)
	// An absent `first` is the previous command and an out-of-range absolute
	// one is the oldest entry; an absent `last` is whatever `first` came to
	// and an out-of-range absolute one is the previous command. See
	// resolveWith for the four measurements.
	from := h.resolveWith(firstOp, h.cur-1, h.first)
	if !from.found {
		return h.noCommand(r)
	}
	to := h.resolveWith(lastOp, from.num, h.cur-1)
	if !to.found {
		return h.noCommand(r)
	}
	// This very command, named at either end, is refused rather than run —
	// `fc -0` and `fc -e ed 1 -0` alike. The wording differs from the one
	// `-s` gives the same operand, which is why neither stands in for the
	// other: `fc -0` is `fc: history specification out of range` where
	// `fc -s -0` is `fc: no command found`.
	if from.num >= h.cur || to.num >= h.cur || h.cur-1 < h.first {
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
	r.errf("%s", text)
	r.DropHistoryOwnLine()
	r.RecordHistoryEntry(strings.TrimSuffix(text, "\n"))
	return r.runSourced(ctx, text, sourced{
		eval:         true,
		label:        "fc",
		syntaxStatus: r.diag().SyntaxStatus(),
	})
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

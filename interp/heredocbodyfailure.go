// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A here-document body that will not expand, on a command **this shell runs
// itself**.
//
// heredocprocess.go is the other half: there the command is a process of its
// own, the body is expanded in that process, and the failure is the child's,
// so every column reports it, leaves the command unrun and carries the script
// on. Here there is no child, and the panel splits over whose failure it is.
//
// It went unasked. `heredocBody` handed the body straight back when the shell
// ran the command itself, so `: <<END` with `$(( 1/0 ))` in it wrote the
// arithmetic complaint and then left **0** behind and carried on: `||` never
// fired, `set -e` never tripped, and three of the five columns had ended the
// shell (#4684).
//
// # The rule, with one noun in it
//
// **Whose failure it is.** Measured 2026-09-26 with `-c`, the failing
// redirection on its own line, against bash 5.3.20 (/opt/homebrew/bin/bash),
// zsh 5.9.2 under `-f` (/opt/homebrew/bin/zsh), ksh93u+ 2012-08-01
// (/bin/ksh), dash 0.5.12 (/bin/dash) and BusyBox v1.37.0 in the
// digest-pinned alpine image the oracle reaches that column through. `go
// version -m` says *not a Go executable* for each of the four on this machine
// and `github.com/blairham/sh/cmd/<shell>` for ours. A body of `$(( 1/0 ))`,
// and `echo "after st=$?"` on the line after the delimiter:
//
//	                          bash    zsh    ksh93  dash   ash
//	: <<END       special     st=1    stops  stops  stops  stops
//	read x <<END  regular     st=1    stops  st=1   st=2   st=1
//	f <<END       function    st=1    stops  st=1   st=2   st=1
//	{ :; } <<END  group       st=1    stops  st=1   st=2   st=1
//	cat <<END     external    st=1    st=1   st=1   st=2   st=1
//
// The external row is the control and is nobody's question: it is the other
// file's, unchanged, and the only row the five agree on.
//
// Two readings fit the other four rows, and they are told apart by holding
// one noun fixed at a time:
//
//   - **The redirection's.** A file that will not open draws exactly that
//     grid in ksh93, dash and ash: `: < /nonexistent/f` stops all three and
//     `read x < /nonexistent/f` carries on in all three, at 1, 2 and 1. It is
//     [Semantics.RedirectErrorOnSpecialBuiltinFatal] and nothing else, and
//     the status is [Diagnostics.RedirectFailureStatus]'s — which is the
//     column ash is worth having, since its failed redirections report 1
//     where its fatal errors exit 2.
//   - **The shell's own failed expansion.** In bash and zsh the same file
//     draws a *different* grid — both carry on for `:` and for `read`, and
//     `: < /nonexistent/f || echo CAUGHT` prints `CAUGHT` in both — so a
//     failed body is not a failed redirection there. It is what
//     `echo $(( 1/0 ))` is: `echo $(( 1/0 )); echo SAME` writes no `SAME` in
//     bash and neither does `: <<END; echo SAME` with the failing body, where
//     the unopenable file writes it. zsh ends the shell for all three, which
//     is that column's answer to every failed expansion.
//
// So the split is three to two and it is not the split
// [Semantics.HeredocExpandsInTheCommandsProcess] makes, which is why this is
// an axis of its own rather than a second reading of that one.
//
// # The row the noun needs
//
// "A here-document body that failed" and "a redirection that could not be set
// up" agree on every row above unless a redirection that is **not** a
// here-document can fail the same way, and one can: a *target* whose
// expansion fails. `read x < $(( 1/0 ))` ends the shell in dash and ash at 2,
// where the same failure in a body carries on at 2 and at 1. That is
// [Semantics.RedirectTargetExpandsInTheCommandsProcess] being No in those two
// columns — the target is expanded by the shell, so its failure is the
// shell's — and it is what says the noun here is the *body* and not
// redirections at large.
//
// # And the failure kind is not the noun either
//
// `${q?word}` holds the command fixed and moves what failed. In an ordinary
// word it ends the shell in all five. In a body it follows this axis and not
// the kind: `read x <<END`, a function and a group carry on at 1, 2 and 1 in
// ksh93, dash and ash, and end the shell in bash and zsh — where bash's own
// reading of `${q?word}` is fatal and its reading of a division is the line,
// which is exactly what "the shell's own failed expansion" means. A body of
// `$(( 1/0 ))` under a **quoted** delimiter is the control the pair needs:
// `: <<'END'` runs its command at 0 in all five, because nothing in it is
// expanded. A `<<-` body behaves as a `<<` body does, in all five.

// bodyExpansionFailed reports whether expanding a here-document body left an
// error behind rather than text.
//
// Two shapes, which are two of the three [Runner.targetExpansionFailed] reads:
// a fatal one has already unwound and is waiting to be caught, and one that
// only reported itself is still in ordinary flow. The third — a give-up of the
// *line* — is the target's alone, because it is reached by an unmatched
// pattern in a word aimed at a file and a here-document body has no pattern
// in it to fail on. It was written here first and no probe could make it fire.
//
// **A body that will not parse is not a body that will not expand**, and it is
// the exclusion this door turns on. `: <<END` with `$(echo hi; for)` in it is
// a substitution whose body is not a program; nothing was expanded, and what
// becomes of the shell is
// [Semantics.SubstitutionParseFailureInAHeredocBodyEndsTheShell]'s, measured
// one construct over. Letting it in here regraded it as a redirection failure
// and cost ksh93 the status it ends at — 3, the substitution's own, where a
// failed redirection on a special builtin is 1. Measured 2026-09-26; the rows
// that are still wrong on that construct are #4687 and not this one.
//
// A plain request to stop is deliberately not among them either. `: <<END`
// with `$(exit 3)` in its body is a substitution's own exit, it is not an
// error this shell reported, and a boundary that caught it would be catching a
// request to stop — which is [Runner.giveUpTheCommand]'s rule at the other
// half of the same construct.
func (r *Runner) bodyExpansionFailed() bool {
	if r.abandon == abandonSubstParse && r.ctl == controlExit {
		return false
	}
	return r.pendingFileError() || (r.expandErr && r.ctl == controlNone)
}

// expandBodyInThisShell expands a here-document body for a command this shell
// runs itself and settles what a failure in it costs.
//
// The wrapper rather than a flag inside the expander, for the reason
// [Runner.confineToTheProcess] is one: the body expands exactly as it always
// did, and the whole of this is what becomes of the failure it left behind.
//
// What it reads afterwards is the whole of what it reads: there is no marker
// taken in front of the expansion, because nothing can arrive here with a
// failure already on the record. A command whose *words* would not expand
// returns before its redirections are opened, a compound command's heading
// clears the flags on the way in, and applyRedirs stops at the first
// redirection that failed — so `: <<A <<B` with a failing `A` never reaches
// `B` at all. A marker was written here first and no probe could make it
// fire; it is left out rather than kept as a guard nothing can exercise.
func (r *Runner) expandBodyInThisShell(expand func() string) string {
	body := expand()
	if !r.bodyExpansionFailed() {
		return body
	}
	if r.ask(r.sem().HeredocBodyFailureIsTheRedirections,
		"whose failure a here-document body that will not expand is") {
		// The redirection's. Through the boundary the external half already
		// uses rather than a second copy of it — a second abandonment
		// boundary is a second place for it to be subtly different — and
		// that boundary is where the status a failed redirection carries is
		// taken, which is the whole of what separates this from a fatal
		// error in the one column that numbers the two differently.
		r.giveUpTheCommand(heredocBodyBoundary)
	} else if !r.unspecified && r.ctl == controlNone {
		// This shell's own failed expansion, through the door a failed
		// *word* goes through, axis and all. Reached only where the failure
		// reported itself without unwinding; one that has already unwound is
		// carrying its status and its reach and is left exactly alone, which
		// is what makes `${q?word}` in a body end the shell in these two
		// columns as it does in an ordinary word.
		r.failedExpansion()
	}
	// Whoever the failure belongs to, the command does not run: its
	// redirections did not come out. That is true of the unanswered reading
	// too — acting on either answer after saying the shells disagree would
	// answer the question anyway.
	r.redirErr = true
	return body
}

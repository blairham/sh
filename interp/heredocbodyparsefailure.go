// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A here-document body holding a substitution that **will not parse**, on a
// command this shell runs itself.
//
// heredocbodyfailure.go is the neighbor: there the body *expanded* and the
// expansion failed. Here nothing was expanded at all, because the
// substitution's body is not a program — `$(echo hi; for)` — and the shell
// refused it while reading the body rather than while evaluating it.
//
// It went unasked. `Runner.confineToTheProcess` reaches
// [Runner.giveUpTheCommand] only for a command the shell runs as a process of
// its own, so `cat <<END` with such a body was graded and `: <<END`,
// `read x <<END`, a function and a group were not: the refusal's abandonment
// stood untouched and ended the shell in ten of the twenty rows (#4687).
//
// # The rule, with one noun in it
//
// **Whose failure it is** — the same noun heredocbodyfailure.go names, with
// the same five answers, which is why this reads
// [Semantics.HeredocBodyFailureIsTheRedirections] rather than adding an axis.
// What is new is the **status**: a refusal carries a number of its own and
// keeps it, where a failed expansion takes the redirection's.
//
// Measured 2026-09-26 with `-c`, `; echo SAMELINE` on the redirection's **own
// line** and `echo "AFTER st=$?"` on the line after the delimiter — the
// pairing this question has to be asked on, since `; echo SAME` written after
// the *delimiter* is a separate line and both readings print it. Against bash
// 5.3.20 (/opt/homebrew/bin/bash), zsh 5.9.2 under `-f`
// (/opt/homebrew/bin/zsh), ksh93u+ 2012-08-01 (/bin/ksh), dash 0.5.12
// (/bin/dash) and BusyBox v1.37.0 in the digest-pinned alpine image
// (alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b),
// under `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin from /dev/null. `go
// version -m` says *not a Go executable* for each of the four on this machine
// and `github.com/blairham/sh/cmd/<shell>` for ours. A body of
// `$(echo hi; for)`:
//
//	                          bash        zsh      ksh93       dash    ash
//	: <<END       special     line, st=1  ends, 1  ends, 3     ends 2  ends 2
//	read x <<END  regular     line, st=1  ends, 1  command     ends 2  ends 2
//	echo <<END    regular     line, st=1  ends, 1  command     ends 2  ends 2
//	f <<END       function    line, st=1  ends, 1  command     ends 2  ends 2
//	{ :; } <<END  group       line, st=1  ends, 1  command     ends 2  ends 2
//	cat <<END     external    command     ends, 1  command     ends 2  ends 2
//
// "line" is the give-up that took `; echo SAMELINE` with it and still ran the
// next line; "command" is the one that left `SAMELINE` running. The external
// row is the control and is the other file's — it is what
// [Semantics.SubstitutionParseFailureInAHeredocBodyEndsTheShell] was measured
// on, and nothing here moves it.
//
// zsh, dash and BusyBox ash end the shell on every row, which is that axis
// answering Yes, so the reading below is never reached in those three. bash
// and ksh93 are where it is decided, and they part in both directions:
//
//   - **The redirection's**, ksh93. `read x <<END` carries on and runs the
//     rest of its line, and only a **special** builtin stops the shell —
//     which is [Semantics.RedirectErrorOnSpecialBuiltinFatal] and nothing
//     else. `command : <<END` is the pair that says so: the same body, the
//     same failure and the same builtin, and it carries on, because `command`
//     is what takes a special builtin's specialness away. `export v <<END`,
//     `eval : <<END`, `. /dev/null <<END` and `exec 3<<END` all end it at 3,
//     and a pipeline element, a subshell and `&` do not, because the command
//     is not this shell's to run.
//   - **This shell's own failed expansion**, bash. `: <<END; echo SAMELINE`
//     writes no `SAMELINE` and the next line reads `st=1`, and so does
//     `command : <<END` — the special-builtin pair that moves ksh93 does not
//     move bash at all, which is what says bash is not under the first
//     reading. `cat <<END; echo SAMELINE` *does* write it, which is the
//     external control and the row this must not reach.
//
// Each of the two readings is refuted by a measured cell under the other, so
// the axis is load-bearing here in both directions: under the first, bash
// would run `SAMELINE`, and under the second ksh93 — which ends the shell for
// any other failed expansion — would stop on `read x <<END`, where it carries
// on.
//
// # And the status is the refusal's, which is the whole reason this is not
// the neighbor's door
//
// Routing this shape through heredocbodyfailure.go was tried in #4684 and
// taken back, because it regraded a refusal as a failed redirection and cost
// ksh93 the number it ends at. One cell says it, and it is the sharpest thing
// on this page — the same command, the same delimiter, the same column, one
// body apart:
//
//	                                       ksh93     BusyBox ash
//	: <<END with `$(( 1/0 ))`   expansion  ends, 1   ends, 1
//	: <<END with `$(echo hi; for)`  parse  ends, 3   ends, 2
//
// 1 is ksh93's fatal status and what its own `: < /nonexistent` reports; 3 is
// [Diagnostics.SyntaxErrorStatus], the refusal's own, surviving a stop that
// was taken away and then put back by a different rule. In ash the pair is 1
// against 2 for the same reason, one axis along. So the expansion's number is
// the redirection's and the refusal's is its own, and
// [Runner.redirFailureStatusOfItsOwn] is how the second survives the first's
// door.

// bodyParseFailed reports whether a here-document body held a substitution
// whose own body is not a program.
//
// The same conjunction [Runner.giveUpTheCommand] reads of the same state one
// file over, deliberately spelled the same way: this door sends a failure to
// that boundary, so a predicate wider here than there would send it a shape
// it then declines to grade — and the command would run with a body nobody
// produced.
//
// Defensive rather than load-bearing in its second half, and that is
// measured: a mutant dropping `r.ctl == controlExit` survives the whole of
// interp/ and dialect/, because all three writers of abandonSubstParse set
// controlExit in the same statement and every route that clears controlExit
// puts abandonRequested back. Kept because being wider than the boundary is
// the failure mode, not being narrower.
func (r *Runner) bodyParseFailed() bool {
	return r.abandon == abandonSubstParse && r.ctl == controlExit
}

// giveUpABodyThatWillNotParse settles what such a body costs a command this
// shell runs itself.
//
// Through [Runner.giveUpTheCommand] under either reading rather than beside
// it, because that is where
// [Semantics.SubstitutionParseFailureInAHeredocBodyEndsTheShell] is asked and
// that question is the same one at both — the three columns answering Yes end
// the shell whoever the failure belongs to, and a second copy of the boundary
// is a second place for it to be subtly different.
func (r *Runner) giveUpABodyThatWillNotParse() {
	isRedirs := r.ask(r.sem().HeredocBodyFailureIsTheRedirections,
		"whose failure a here-document body that will not parse is")
	r.giveUpTheCommand(heredocBodyBoundary)
	if isRedirs || r.unspecified || r.ctl != controlNone {
		// The redirection's, or nobody answered, or the dialect ended the
		// shell over the refusal and there is nothing left to reach.
		return
	}
	// This shell's own, through the door a failed *word* goes through: the
	// give-up costs the line where the dialect gives up lines and the shell
	// where it does not, exactly as any other failed expansion does there.
	//
	// The status it writes is the fatal one where the boundary above left
	// the refusal's, and the one column that reaches this line numbers the
	// two alike — bash answers Yes to
	// [Semantics.SubstitutionParseFailureCarriesTheFatalStatus], so both are
	// 1. Nothing chooses between them, so nothing here pretends to: a
	// restore was written and no probe could make it fire.
	r.failedExpansion()
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"
)

// How deep a function may call.
//
// Every shell in the panel bounds it — an unbounded one is a segmentation
// fault away from a recursive function with no base case — and two of them let
// a script *move* the bound by writing to a parameter. The bound is the
// shell's and the parameter's name is the dialect's, which is the same seam
// [Runner.SetRegexMatch] uses: the core enforces and a dialect names.
//
// A dialect says three things about it at once and [FunctionNesting] is that
// statement — the name, how the value reads, and what the refusal costs.

// FunctionNesting is everything a dialect has to say about the bound on
// function nesting: the parameter a script moves it with, how that
// parameter's value reads, and what the refusal costs.
//
// One statement rather than three setters, because the three facts are not
// independent and a dialect that answered one of them and forgot another
// would be the shape this tree keeps finding: a second helper that omits the
// fix the first one carries. The zero value is a shell whose bound is its own
// and cannot be moved, which is what three of the panel are.
type FunctionNesting struct {
	// Parameter is the name a script writes to move the bound. Empty is a
	// shell that has no such name — measured 2026-09-23, ksh93u+
	// 2012-08-01 answers a runaway with `f: recursion too deep` and reads
	// no parameter at all, where bash 5.3.20 and zsh 5.9.2 both read one.
	//
	// The *wording* of the refusal is separate and is
	// Diagnostics.FunctionNestingLimit, because a shell may have the bound
	// without having the parameter.
	Parameter string
	// ZeroIsABound makes a stored zero a bound of zero — no call at all —
	// rather than "the script has said nothing".
	//
	// The two shells that read a parameter disagree about this and the
	// disagreement is measured, 2026-09-27, script files under
	// `env -i PATH=/usr/bin:/bin`, with a recursion that counts:
	//
	//	FUNCNEST=   bash 5.3.20            zsh 5.9.2
	//	5           refuses the 6th call   refuses the 6th call
	//	0           no bound               refuses the 1st call
	//	(empty)     no bound               refuses the 1st call
	//	abc         no bound               refuses the 1st call
	//	-1          no bound               no bound
	//
	// The middle three rows are one row in zsh because the parameter is an
	// **integer** there, so an empty value and a word are both stored as 0
	// before this is ever asked; in bash the name is an ordinary string and
	// the three are separately unreadable. So what actually parts the two
	// columns is the single question this field asks, and the negative row
	// is the control that says it is not simply "zsh bounds harder": both
	// shells read a negative as no bound.
	//
	// zsh's negative row is where this shell deliberately stops copying.
	// `FUNCNEST=-1` with a runaway recursion **segmentation faults** there,
	// measured in the same run at status 139 — an unbounded recursion is
	// exactly what the substrate's own ceiling exists for, so a negative
	// falls through to it and the shell reports rather than dies.
	ZeroIsABound bool
	// RefusalEndsTheScript makes the refusal a fatal error rather than a
	// give-up of the input line.
	//
	// Measured 2026-09-27 in the same run, with `echo AFTER` on the line
	// after the call:
	//
	//	                       bash 5.3.20   zsh 5.9.2
	//	f || echo or           no `or`       no `or`
	//	the next line          runs          never runs
	//	exit status            1             1
	//	( f ); echo after      contained     contained
	//	sourced file           n/a           gives up the file, caller runs
	//
	// The last row is why this is fatalQuiet's door rather than an exit:
	// a boundary reading a file of its own gives that file up and carries
	// on, which is the answer measured for `. inner.zsh` — `OUTER-AFTER`
	// printed and the shell left at 0.
	RefusalEndsTheScript bool
	// RefusalNamesTheCalleeInTheLocation puts the function that could not be
	// entered where the location's own name would have gone, instead of in
	// the sentence.
	//
	// The two columns say the same thing in two places, which is why this
	// rides with the wording rather than beside it. Measured 2026-09-27,
	// script files, the callee named `f` and the chain `g` calls `h` calls
	// `g`:
	//
	//	where the call is       bash 5.3.20            zsh 5.9.2
	//	top level, line 4       …:line 4: f: max…(0)   f:4: max…
	//	inside the callee       …: f: max…(5)          f: max…
	//	inside another function …: g: max…(2)          g: max…
	//
	// So bash's location is the script's throughout and the callee's name is
	// in its sentence, where zsh's sentence has no name in it at all and the
	// callee stands in the location. The third row is the one that says the
	// name is the **callee's** and not the frame's: the shell is inside `h`
	// and both columns name `g`.
	//
	// Only the name moves. The line stays whatever the location would have
	// written — the caller's number at the top level and nothing at all
	// inside a body — which is measured in the first two rows and is why
	// this is a name override rather than a frame.
	RefusalNamesTheCalleeInTheLocation bool
}

// SetFunctionNesting is the dialect's statement about the bound, called from
// Apply.
//
// The parameter is read at each call rather than cached, because a script may
// set it from inside the recursion it is bounding — which is exactly what
// makes it worth having, and is measured: bash honors a value assigned part of
// the way down.
func (r *Runner) SetFunctionNesting(n FunctionNesting) { r.funcNest = n }

// functionNestingLimit is how many calls may be active at once, and whether
// the script has said.
//
// A **negative** is no bound in both columns and an unreadable value is no
// bound either, with nothing said about it in either case. Measured
// 2026-09-23 on bash 5.3.20 with a function that recurses past six, `env -i
// PATH=/usr/bin:/bin LC_ALL=C`: `FUNCNEST=0`, `FUNCNEST=`, `FUNCNEST=abc`
// and `FUNCNEST=-2` all run to the function's own base case there.
//
// Zero is the one the two columns part over and is
// [FunctionNesting.ZeroIsABound]'s question, measured beside those rows —
// see the table there, and note that it is asked of the *stored* value, so
// zsh's integer attribute has already turned an empty value and a word into
// it before this is reached.
func (r *Runner) functionNestingLimit() (int, bool) {
	if r.funcNest.Parameter == "" {
		return 0, false
	}
	text, ok := r.getVar(r.funcNest.Parameter)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n < 0 {
		return 0, false
	}
	if n == 0 {
		return 0, r.funcNest.ZeroIsABound
	}
	return n, true
}

// refuseFunctionNesting reports and gives up where a call would pass the
// bound, and says whether it did.
//
// **The whole input line goes with it**, which is measured rather than
// assumed: `f || echo or` prints no `or`, `for i in 1 2; do f; echo body;
// done` prints no `body`, and the line after the call runs normally at status
// 1. So it is abandonTheCommand's give-up and not the script's end — the same
// shape a refused readonly assignment takes. Measured 2026-09-23 on bash
// 5.3.20.
//
// The bound is read against the calls already active, so a bound of N lets N
// of them stand and refuses the N+1st: with `FUNCNEST=5` a counter in the body
// reaches 5. The name in the sentence is the **callee's**, and the location is
// the call site's — bash's two-function chain names the function that could not
// be entered and the line inside its caller.
func (r *Runner) refuseFunctionNesting(name string) bool {
	n, ok := r.functionNestingLimit()
	if !ok || r.depth < n {
		return false
	}
	d := r.diag()
	if r.funcNest.RefusalNamesTheCalleeInTheLocation {
		// Around the write alone, and restored after it: the field is read
		// by the trace prefix as well as by the diagnostic, and a name left
		// set would rename every line the shell traced after a refusal it
		// had already survived.
		was := r.locationNamesInstead
		r.locationNamesInstead = name
		defer func() { r.locationNamesInstead = was }()
	}
	r.diagf("%s\n", Wording(d.FunctionNestingLimit,
		"%[1]s: maximum function nesting level exceeded (%[2]d)", name, n))
	r.status = 1
	if r.funcNest.RefusalEndsTheScript {
		// The status is the one measured rather than the fatal axis's,
		// which happen to be the same number in the one column that
		// answers this way — see endTheScriptAt for why the two are asked
		// separately at all.
		r.endTheScriptAt(1)
		return true
	}
	r.abandonTheCommand()
	return true
}

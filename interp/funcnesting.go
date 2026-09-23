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

// SetFunctionNestingParameter names the parameter a script writes to move the
// bound on function nesting. Empty — the zero value — is a shell whose bound
// is its own and cannot be moved, which is what three of the panel are.
//
// A dialect calls it from Apply. The *wording* of the refusal is separate and
// is Diagnostics.FunctionNestingLimit, because a shell may have the bound
// without having the parameter: measured 2026-09-23, ksh93u+ 2012-08-01
// answers a runaway with `f: recursion too deep` and has no FUNCNEST at all,
// where bash 5.3.20 and zsh 5.9.2 both read one.
//
// The parameter is read at each call rather than cached, because a script may
// set it from inside the recursion it is bounding — which is exactly what
// makes it worth having, and is measured: bash honors a value assigned part of
// the way down.
func (r *Runner) SetFunctionNestingParameter(name string) { r.funcNestParam = name }

// functionNestingLimit is how many calls may be active at once, and whether
// the script has said.
//
// Only a **positive integer** counts. Measured 2026-09-23 on bash 5.3.20 with
// a function that recurses past six, `env -i PATH=/usr/bin:/bin LC_ALL=C`:
// `FUNCNEST=0`, `FUNCNEST=`, `FUNCNEST=abc` and `FUNCNEST=-2` all run to the
// function's own base case, so none of the four is a bound — an unreadable
// value is not a refusal either, and nothing is said about it.
func (r *Runner) functionNestingLimit() (int, bool) {
	if r.funcNestParam == "" {
		return 0, false
	}
	text, ok := r.getVar(r.funcNestParam)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n <= 0 {
		return 0, false
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
	r.diagf("%s\n", Wording(d.FunctionNestingLimit,
		"%[1]s: maximum function nesting level exceeded (%[2]d)", name, n))
	r.status = 1
	r.abandonTheCommand()
	return true
}

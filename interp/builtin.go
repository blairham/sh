// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// builtins are commands the shell runs itself.
//
// They exist because they must: `set` and `shift` change the shell's own
// state, and a child process cannot. That is also why the gate does not see
// them — nothing leaves this process — while it does see every external
// command and every file opened for a redirection.
var builtins = map[string]Builtin{
	":":        biTrue,
	"true":     biTrue,
	"false":    biFalse,
	"echo":     biEcho,
	"pwd":      biPwd,
	"trap":     biTrap,
	"break":    biBreak,
	"continue": biContinue,
	// `eval` and `.` are added in source.go's init rather than here — they
	// run arbitrary shell, so they reach the dispatcher that reads this map,
	// and Go calls a literal that closes that loop an initialization cycle.
	// `unset`, `exit` and `return` joined them, for the reason the init
	// below gives.
}

// `unset a[i+1]` evaluates its subscript, and evaluating an expression
// substitutes into it first — so a command substitution written in one reaches
// the dispatcher that reads the map above, exactly as `eval` does. That is a
// real capability rather than an accident of layering: the subscript is an
// arithmetic expression, and every way of writing one is open to it.
//
// `exit` and `return` are here for exactly that reason and no other: their
// status operand is an arithmetic expression in zsh, so `return $(( ))` — or
// `return "$(f)"`, or any subscript inside the expression — reaches the same
// dispatcher. They were in the literal above while the operand was read with
// atoi and could reach nothing.
//
// `read` joined them when its operand became a subscripted one. A name like
// `buf[$#buf+1]` is stored through the same element route an assignment takes,
// and that route evaluates the subscript — so the builtin now reaches the
// dispatcher for exactly the reason `unset` does, through the arithmetic and
// not through anything of `read`'s own.
//
// `cd` is here for the plainest version of the same reason: it *calls a shell
// function* — the directory-change hook — and a function body is arbitrary
// shell. It was in the literal above while it could only move the runner and
// print.
//
// `wait` joined them for exactly `read`'s reason, one letter later: `wait -p
// A[$key]` stores the finished job's process id through the same element
// route an assignment takes, and that route evaluates the subscript — so a
// command substitution written inside one reaches the dispatcher. Before the
// letter there was nothing of `wait`'s that could reach anything.
//
// `set` is the newest and reaches the dispatcher two constructs further out:
// `set -A a …` stores an array, a name carrying the integer attribute folds
// every element through the arithmetic, and a subscript with a flag group in
// it now searches — which expands the group's operand, and an operand may
// hold a command substitution. The chain was already there and closed the
// moment the arithmetic learned to read a group (#1986).
func init() {
	builtins["set"] = biSet
	builtins["unset"] = biUnset
	builtins["exit"] = biExit
	builtins["return"] = biReturn
	builtins["read"] = biRead
	builtins["wait"] = biWait
	builtins["cd"] = biCd
}

// biBreak and biContinue transfer control out of a loop. They are recorded on
// the runner rather than returned as errors, because leaving a loop is
// ordinary control flow and modeling it as a failure would make every caller
// check for something that is not one.
func biBreak(r *Runner, _ context.Context, args []string) int {
	rest, want, st, done := r.loopControlCount("break", args)
	if done {
		return st
	}
	reach, st, done := r.loopControlReach("break", want)
	if done {
		return st
	}
	// After the place, which is where bash asks it: `break 1 2` outside a
	// loop is `only meaningful in a for, while, or until loop` there and is
	// `too many arguments` in zsh, and that ordering is
	// Semantics.LoopControlPlaceIsJudgedBeforeTheCount read at a second site
	// rather than a reading of its own. Inside a loop both columns say too
	// many, and neither leaves the loop.
	if st, done := r.extraNumericOperands("break", rest); done {
		return st
	}
	r.ctl, r.ctlDepth = controlBreak, reach
	return 0
}

func biContinue(r *Runner, _ context.Context, args []string) int {
	rest, want, st, done := r.loopControlCount("continue", args)
	if done {
		return st
	}
	reach, st, done := r.loopControlReach("continue", want)
	if done {
		return st
	}
	if st, done := r.extraNumericOperands("continue", rest); done {
		return st
	}
	r.ctl, r.ctlDepth = controlContinue, reach
	return 0
}

// extraNumericOperands is what a word written **behind** the count of
// `break`, `continue`, `return`, `exit` or `shift` comes to: the status the
// builtin ends at, and whether it ends there.
//
// One helper for the five because it is one rule in every shell that has it —
// bash reaches it through `no_args`, once, from the reader all five share —
// and a second copy beside the first is how this repository's recurring
// defect is spelled: a fix applied to one spelling and not its twin.
//
// The count is already read when this is asked, which is measured rather than
// convenient: `shift abc def` is `abc: numeric argument required` and
// `exit abc def` is `exit: abc: numeric argument required`, so a count that
// will not read is what a script hears about first and this never speaks. It
// does win over everything *after* the read — `shift 5 2` on three positional
// parameters is `too many arguments` and not the out-of-range complaint, and
// `shift -2 3` is the same.
//
// See Semantics.ExtraNumericOperand for the three readings and the panel.
func (r *Runner) extraNumericOperands(name string, args []string) (int, bool) {
	if len(args) < 2 {
		return 0, false
	}
	p := r.extraNumericOperand()
	if r.unspecified {
		return r.status, true
	}
	if p == ExtraNumericOperandIgnored {
		return 0, false
	}
	r.diagf("%s\n", Wording(r.diag().NumericOperandTooMany, "%[1]s: too many arguments", name))
	if p == ExtraNumericOperandGivesUpTheStatement {
		// The rest of the *statement* goes with it and the input does not:
		// measured, `shift 1 2; echo SAME` prints no `SAME` and the next
		// line runs, and a loop around the refusal stops where it stands.
		// That is controlAbandon, which the refused readonly assignment
		// beside it already raises — see interp/compound.go.
		r.status = 2
		r.abandonTheCommand()
		return 2, true
	}
	// Refused and nothing given up: the loop around it runs on and complains
	// again on the next pass.
	r.status = 1
	return 1, true
}

// numericOperandMarker takes a leading `--` off the operands of a builtin
// whose only operand is a number, reporting whether the reading may go on.
//
// One helper for the four builtins that ask it — `break`, `continue`,
// `return` and `exit` — because a second reader beside the first is what
// drifts the next time the marker is measured, and it already had: `shift`
// took the marker from its first day and these four read it as the operand
// and refused it, so `break -- 1` ended the script where five of the seven
// columns end the loop. See Semantics.NumericOperandDoubleDashEndsOptions.
//
// Only the first word is looked at, which is what makes `break -- --` a
// count of `--`, and only where it actually is the marker — a dialect is
// never asked about `break 2`.
func (r *Runner) numericOperandMarker(args []string) ([]string, bool) {
	if len(args) == 0 || args[0] != "--" {
		return args, true
	}
	switch {
	case r.ask(r.sem().NumericOperandDoubleDashEndsOptions, "`--` read as the end of a numeric operand's options"):
		return args[1:], true
	case r.unspecified:
		return args, false
	}
	// The two columns with no marker here read the word as the operand, and
	// it is not one — which is the complaint the caller was going to make
	// about it anyway.
	return args, true
}

// loopControlCount reads the count `break` and `continue` share, and reports
// the one every shell in the panel refuses and ours took in silence.
//
// **Which of the two complaints comes out is an order and not a wording.**
// The count and the place are both available for `break abc` with no loop to
// leave, every column writes exactly one of them, and which one divides the
// panel — the bash family looks at the place and never reads the word, the
// other four read the word and never look at the place. That is
// Semantics.LoopControlPlaceIsJudgedBeforeTheCount, asked below and only
// where the two answers differ; where the place is judged first the count is
// never read here at all and loopControlReach has the only complaint.
//
// The comment this replaced asserted the opposite order for every dialect and
// cited bash's place complaint as the evidence for it, so the code refused
// `break abc` where bash names the loops and carries on (#2299).
//
// The operands *past* the end-of-options marker come back with the count,
// because the caller has one more question to ask of them and asking it of
// the words as written would count the marker: `break -- 1` is one operand
// and not two. See Runner.extraNumericOperands.
//
// `return` is the counter-case and is not this question: `return abc` outside
// a function writes the operand's complaint *and* the place's, in that order
// (#2762), so the two builtins do not share a rule here.
//
// See Diagnostics.LoopControlCount for the panel's sentences and for why the
// script's ending is not an axis: all seven end there.
func (r *Runner) loopControlCount(name string, args []string) ([]string, int, int, bool) {
	args, marked := r.numericOperandMarker(args)
	if !marked {
		// The axis went unanswered; ask told the script so and the builtin
		// stops rather than guessing which of `--` and the word behind it
		// is the count.
		return nil, 0, r.status, true
	}
	if len(args) == 0 {
		return args, 1, 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(args[0]))
	if err == nil && n > 0 {
		return args, n, 0, false
	}
	d := r.diag()
	// Both complaints are available from here, and the dialect decides which
	// one it makes. Asked only at the disagreement: there has to be no loop
	// the word could reach — loopControlFloor is what makes a boundary one of
	// the ways there is none — and a count this reader has already refused,
	// so `break 2` inside a loop and a bare `break` anywhere ask nothing.
	if r.loopDepth-r.loopControlFloor(1) <= 0 {
		switch {
		case r.unspecified:
			// A boundary axis went unanswered inside the floor, and it is
			// the first question asked here.
			return args, 0, r.status, true
		case r.ask(r.sem().LoopControlPlaceIsJudgedBeforeTheCount,
			"a misplaced `break` being judged before its count is read"):
			// The place is judged first, so the word is never read and never
			// quoted: a count of one carries through to loopControlReach,
			// which finds no loop and writes the place's complaint.
			return args, 1, 0, false
		case r.unspecified:
			return args, 0, r.status, true
		}
	}
	// The number this reading produced, for the one dialect whose sentence
	// quotes that rather than the word: a word that is no number at all
	// reads as nought there, which is what `break abc` says.
	operand := args[0]
	if d.LoopControlCountNamesTheNumber {
		operand = strconv.Itoa(n)
	}
	if err == nil && d.LoopControlCountOutOfRange != "" {
		// A number, and not positive, in the one column that parts the two:
		// it complains and the script carries on — but not from the loop.
		// **Every loop the word can reach ends, at status 1, and that is so
		// for `continue` as much as for `break`.** Measured 2026-09-16 in
		// bash 5.3.20 and 3.2.57 alike: `for i in 1 2; do for j in a b; do
		// continue -1; echo tail; done; echo mid; done; echo "st=$?"` prints
		// one complaint, then `st=1` — no `tail`, no `mid`, no second pass
		// of either loop — and `break 0 && echo and` inside a loop prints
		// nothing after the complaint. What "reach" means is the floor's,
		// so a function call is a wall in 5.3 and is not one in 3.2, exactly
		// as it is for a count that is in range. Ours took the count as 1,
		// so `continue 0` complained once per pass and a loop went on
		// running at status 0.
		r.diagf("%s\n", Wording(d.LoopControlCountOutOfRange, "", name, operand))
		floor := r.loopControlFloor(r.loopDepth)
		if r.unspecified {
			return args, 0, r.status, true
		}
		if reach := r.loopDepth - floor; reach > 0 {
			r.ctl, r.ctlDepth = controlBreak, reach
		}
		return args, 0, 1, true
	}
	// The fallback chain is a chain of *formats*, not of rendered text: the
	// operand can hold a `%` and rendering twice would read it as a verb.
	format := d.LoopControlCount
	if format == "" {
		format = d.NumericArgument
	}
	r.diagf("%s\n", Wording(format, "%[1]s: invalid number: %[2]s", name, operand))
	// The script ends, in every column of the panel — see the field. Not
	// through BadOptionToSpecialBuiltinFatal: plain bash answers No there
	// and still ends the script here.
	// The status is the builtin's own rather than the one
	// Semantics.FatalErrorStatusIsOne gives every other fatal error: bash
	// says Yes to that axis and reports **2** here, where ksh93 and zsh
	// report 1, so the two are different facts. See fatalAtStatus.
	r.fatalAtStatus(orDefault(d.LoopControlCountStatus, 2))
	return args, 0, r.status, true
}

// loopControlReach is how many loops a `break` or `continue` can see, plus
// what the builtin reports when the answer is none.
//
// The count is the dynamic one — how many loops execution is inside right now
// — and not a lexical question about where the word was written. That is
// measured rather than convenient: a function whose body is a bare `break`,
// called from a loop, leaves the loop in bash 3.2 and in zsh.
//
// What it cannot always see is the loops on the far side of a *boundary*, and
// the panel splits three ways over where the boundaries are — see
// Runner.loopControlFloor. The floor is subtracted rather than checked,
// because `break 2` from inside a boundary with one loop in it stops at that
// loop in every column that has a boundary there at all: measured, `f(){ for
// j in 1; do break 2; done; echo infunc; }` called from a loop prints
// `infunc` in bash 5.3, ksh93 and dash and does not in bash 3.2 or zsh, which
// is the same split as the bare `break`.
//
// Ours used to set the control value whatever the count was, and nothing
// consumed it, so it unwound past the top and the rest of the *script*
// vanished — silently, at status 0. Two things were wrong and they are
// separable: nothing was reported, and a line that five of the six panel
// columns finish was given up (#1236).
func (r *Runner) loopControlReach(name string, want int) (int, int, bool) {
	if reach := min(want, r.loopDepth-r.loopControlFloor(want)); reach > 0 {
		return reach, 0, false
	}
	if msg := Wording(r.diag().LoopControlOutsideALoop, "", name); msg != "" {
		r.diagf("%s\n", msg)
	}
	if !r.ask(r.sem().LoopControlOutsideALoopIsFatal, "a `break` or `continue` with no loop around it") {
		// Reported, or not, and then ignored: the next command on the line
		// runs and the status is the builtin's own success. That is dash,
		// ksh93, bash and bash called as `sh` alike — they differ over the
		// message and agree about everything else.
		return 0, 0, true
	}
	r.fatalQuiet()
	return 0, r.status, true
}

// loopControlFloor is the loop depth a `break` cannot reach past: the
// innermost boundary between the word and the loops it is counting.
//
// Two boundaries, and they are two axes because the panel does not group
// them. Measured 2026-09-11 on `f(){ break; }; for i in 1 2; do f; echo body;
// done` and on `for i in 1 2; do ( break; echo insub ); echo body; done`:
//
//	bash 5.3   	both are boundaries — and it says so, twice
//	dash, ksh93	the call is, the subshell is not
//	bash 3.2   	neither is
//	zsh        	neither is
//
// So a single field would have had to give bash 5.3's subshell answer to
// dash, or dash's to bash 5.3. The complaint is not a third thing: a boundary
// leaves the `break` with no loop at all, which is #1236's question, already
// answered by Diagnostics.LoopControlOutsideALoop — bash writes a sentence
// there and dash and ksh93 write nothing, which is exactly what the two rows
// show.
//
// Each axis is asked only where its answer decides something: a boundary with
// enough loops inside it to satisfy the count changes nothing, because the
// word never has to look past it. That is what keeps `for i in 1 2; do ( for j
// in 1; do break; done ); done` from asking either — and it is the count and
// not the mere presence of a loop, since `break 2` from inside one loop in a
// function does have to look past the call.
//
// The command substitution and the pipeline element are deliberately not
// here. They are subshells too, and bash 5.3 — the one column that makes `(
// )` a boundary — does not make either of them one: `for i in 1 2; do x=$(
// break ); done` and `do break | cat; done` draw no complaint from it, where
// the parenthesized form draws one per pass.
func (r *Runner) loopControlFloor(want int) int {
	floor := 0
	if r.callLoopFloor > 0 && r.loopDepth-r.callLoopFloor < want &&
		r.ask(r.sem().FunctionCallIsALoopControlBoundary,
			"whether a `break` inside a function reaches a loop outside it") {
		floor = r.callLoopFloor
	}
	if r.subshellLoopFloor > 0 && r.loopDepth-r.subshellLoopFloor < want &&
		r.subshellLoopFloor > floor &&
		r.ask(r.sem().SubshellIsALoopControlBoundary,
			"whether a `break` inside a subshell reaches the loop outside it") {
		floor = r.subshellLoopFloor
	}
	return floor
}

// biReturn reads the operand before it judges the place, and reports the
// operand's complaint first.
//
// The two complaints are not alternatives, which is what the old order
// assumed. Measured 2026-09-14, `( return abc ); echo "ret=$?"` in a script
// file under `env -i PATH=/usr/bin:/bin`:
//
//	bash 5.3.15   return: abc: numeric argument required
//	              return: can only `return' from a function or sourced script
//	              ret=2, and the script carries on
//	bash 3.2      the same two lines
//	bash-as-sh    return: abc: numeric argument required — and nothing more
//	dash          return: Illegal number: abc
//	BusyBox ash   return: Illegal number: abc
//	ksh93, zsh    nothing at all, ret=0
//
// So the shell with both writes the operand's first, and the three that stop
// on the operand never reach the place — bash called as `sh` is the clearest
// of them, because it is the same binary as the first row with one complaint
// missing rather than a different wording. Refusing the place first left the
// operand unread in every column, and the status agreed in all seven, which
// is why nothing in the tree noticed (#2762).
func biReturn(r *Runner, _ context.Context, args []string) int {
	// What `$?` was as `return` began, which is what the RETURN trap's
	// action sees — the operand below is for the caller, not the trap. Read
	// here rather than where it is stored, because the operand is an
	// arithmetic expression in one dialect and reading one is a step of its
	// own.
	seen := r.status
	args, marked := r.numericOperandMarker(args)
	if !marked {
		// Unanswered, and reported by ask: the same 2 the unreadable
		// operand below leaves, and for the same reason.
		return 2
	}
	operand, haveOperand := 0, false
	if len(args) > 0 {
		switch n, ok := r.statusOperand("return", args[0]); {
		case ok:
			if st, done := r.extraNumericOperands("return", args); done {
				return st
			}
			operand, haveOperand = n, true
		case r.unspecified:
			// statusArgument has reported the unanswered axis already. It is
			// the first question this builtin asks, so it is the one the
			// strict core reports.
			return 2
		default:
			status := r.refusedReturnOperand(args[0])
			if r.ctl == controlExit {
				// A special builtin's usage error is fatal here, so the
				// script is already leaving and there is no place left to
				// judge. That is the bash-as-`sh`, dash and ash row above:
				// one complaint and gone.
				return status
			}
			operand, haveOperand = status, true
		}
	}
	if !r.hasSomethingToReturnFrom() {
		// Nothing to return from. Three of the panel end the script here
		// with the status given; bash refuses and carries on, which is a
		// difference in *where the script stops* rather than in wording.
		if r.ask(r.sem().ReturnOutsideAFunctionIsRefused, "a `return` with nothing to return from") {
			r.diagf("%s\n", Wording(r.diag().ReturnOutsideAFunction,
				"return: can only `return' from a function or sourced script"))
			// Reported and not obeyed: no control flow is set, so the next
			// statement runs.
			return 2
		}
		if r.unspecified {
			return 2
		}
	}
	r.returnSeenStatus = seen
	r.ctl = controlReturn
	if haveOperand {
		return operand
	}
	// The same reading `exit` takes one builtin over: a bare `return` hands
	// back what the last command did, and one column reads that as this
	// execution unit's last command rather than the shell's. One helper for
	// both, because the panel reads the two spellings identically — the
	// function row and the `${ …;}` row are the same shell answering 0 to
	// both.
	return r.operandLessStatus()
}

// refusedReturnOperand reports a status operand `return` will not take.
//
// The function returns anyway — bash skips the rest of the body and leaves 2
// behind for the caller — so this is not badStatusArg with a different word
// in it: that one is for a shell on its way out and sets controlExit, which
// here would end the script in the shell that carries on.
//
// Where a special builtin's failure is fatal the script does end, and that is
// BadOptionToSpecialBuiltinFatal rather than a second axis about operands:
// dash ends the script at status 2, and so does bash called as `sh`, which is
// bash's own posix mode reaching the POSIX rule. Plain bash reports it and
// runs the next command, and `set -o posix` moves it — measured 2026-09-13,
// and see Semantics.BadOptionToSpecialBuiltinFatalInPosixMode. ksh93 and zsh
// never arrive here at all, because neither refuses any word — so the fatality
// is left to the axis the two shells that *can* answer it agree with.
func (r *Runner) refusedReturnOperand(arg string) int {
	r.diagf("%s\n", Wording(r.diag().NumericArgument, "%[1]s: invalid number: %[2]s", "return", arg))
	if r.ask(r.sem().BadOptionToSpecialBuiltinFatal, "a special builtin's usage error ending the script") {
		// fatalQuiet sets controlExit, which the caller reads: the script
		// ends rather than the function, and nothing after this point runs —
		// including the complaint about having nowhere to return to, which
		// the shells that stop here do not write.
		// No `r.status = 2` to go with the 2 below: the return value is what
		// the dispatcher writes, and it is the status the panel ends at. See
		// setFatalStatus.
		r.fatalUsageQuiet()
	}
	return 2
}

// statusOperand reads the status operand `exit` and `return` share.
//
// One reading for two builtins, because the panel reads the two identically —
// every row of Semantics.StatusArgument was measured on both and they never
// parted, down to zsh answering 3 for `exit r` and for `return r` with r=3.
// Two readings is the shape that has cost this tree seven bugs: the second
// copy omits what the first one learned.
//
// ok is false when the dialect refuses the word, and what a refusal *costs*
// stays with the caller — that is the one place the two builtins do differ,
// since `exit` is leaving and `return` is handing a status back to a caller
// that carries on.
//
// The eight-bit mask rides on the policy rather than being an axis of its
// own: each of the four either masks or does not, and the four answers line
// up one-to-one with the four readings. What comes back is the *shell's*
// reading — `return 300` is 300 under dash and zsh and 44 under bash and
// ksh93 — and `exit` truncates it again on its way out, because a process
// carries eight bits whatever the shell decided. So `exit 300` is 44 in all
// six and only `return 300` can tell the two groups apart.
func (r *Runner) statusOperand(builtin, arg string) (int, bool) {
	arg = strings.TrimSpace(arg)
	// A plain number that already fits is unanimous and never reaches the
	// axis, which is what lets the strict core run `return 3` at all. The
	// bound is 255 rather than "any run of digits" because 256 is where the
	// panel parts: two of them mask there and two hand the number back whole.
	// Leading zeros belong on this side too — `return 010` is 10 in all four,
	// including the two that read `$((010))` as 8.
	if n, err := strconv.Atoi(arg); err == nil && n >= 0 && n <= 255 {
		// Recorded as an arithmetic evaluation where the dialect reads this
		// operand as one, even though the shortcut above is what answered
		// it. `return 1` in zsh really is an expression, and a math function
		// whose implementation ends in one hands that value back — see
		// mathfunc.go, and the plugin manager whose scheduler ends in
		// `return 1` on one branch and `return idx` on the other. Only the
		// second of those goes the long way round, so a record made only
		// there would give the two branches different meanings.
		//
		// Read from the vector rather than through statusArgument, which
		// reports an unanswered axis: the shortcut is what makes `return 3`
		// work in the strict core, and asking here would refuse it.
		if r.sem().StatusArgument == StatusArgArithmetic {
			r.lastArith = intNum(n)
		}
		return n, true
	}
	switch r.statusArgument(builtin) {
	case StatusArgStrict:
		// Digits and nothing else, and no mask: dash keeps `return 300` at
		// 300 and refuses `return -1` outright.
		if n, err := strconv.Atoi(arg); err == nil && n >= 0 {
			return n, true
		}
		return 0, false
	case StatusArgNumeric:
		n, err := strconv.Atoi(arg)
		if err != nil {
			return 0, false
		}
		return mask8(n), true
	case StatusArgLeadingDigits:
		return mask8(leadingDecimal(arg)), true
	case StatusArgArithmetic:
		return r.arithmeticStatusOperand(arg)
	}
	// No dialect answered. statusArgument has already reported it and set
	// r.unspecified, which is what the callers read to tell this apart from
	// a refusal.
	return 0, false
}

// arithmeticStatusOperand evaluates the operand as an arithmetic expression,
// which is what zsh does with it — `return r` is r's value and `return r+1`
// is one more.
//
// It goes through arithTree and evalArith rather than a reader of its own, so
// that the operand gets the same arithmetic `let` and `$(( … ))` get. That is
// what makes `return 0x10` 16 and `return "(r+1)*2"` six without any of it
// being written twice.
// An empty operand needs no special case: `return ""` is 0 in zsh, and an
// empty expression already evaluates to 0 through this same reader, which is
// what `$(( ))` is. One was written here anyway, and a mutation run showed it
// was dead — disabling it changed nothing.
func (r *Runner) arithmeticStatusOperand(expr string) (int, bool) {
	tree, perr := r.arithTree(nil, expr)
	if perr != nil {
		r.diagf("%s\n", r.diag().ParseFailure(perr))
		// Reported as a math error rather than as a refused operand, and 0
		// is what the shell exits with after one: `exit 3abc` complains and
		// leaves 0. Returning ok here keeps the numeric-argument wording —
		// which this dialect never uses — off the back of a math complaint.
		return 0, true
	}
	v, err := r.evalArith(tree)
	if r.unspecified {
		// An axis inside the *expression* went unanswered — `ask` has
		// reported it already. Handing the value back as if it were fine
		// would be a status invented out of a question the shell refused to
		// answer, so this leaves through the same door an unanswered
		// StatusArgument does. `let` guards its own evaluation the same way.
		return 0, false
	}
	if err != nil {
		r.diagf("%v\n", err)
		return 0, true
	}
	return v, true
}

// mask8 is the eight bits a status can carry.
func mask8(n int) int { return ((n % 256) + 256) % 256 }

// leadingDecimal reads the decimal number an operand starts with and ignores
// whatever follows it, which is how ksh93 reads a status: `3abc` is 3, `abc`
// is 0, and ` -5x` is -5 before the mask makes it 251.
//
// Not atoi, which takes digits only and refuses the rest of a word outright,
// and not atoiSigned, which takes a sign but still refuses trailing text. The
// difference is the whole of the ksh93 column, so it is a third reader rather
// than a flag on either of those two.
func leadingDecimal(s string) int {
	i, neg := 0, false
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		neg = s[i] == '-'
		i++
	}
	n := 0
	for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		return -n
	}
	return n
}

// specialBuiltins are the ones POSIX marks special. Two consequences follow
// from the same list — an assignment prefixed to one persists, and a failure
// in one is fatal to a non-interactive shell — so it is one concept rather
// than two lists that could drift.
// `source` is here and is not POSIX's, because POSIX has no `source`: it is
// the other spelling of `.` in the two dialects that have it, and both of
// them mark it special everywhere the rule applies. Measured 2026-09-13 —
// under `bash -o posix` and `zsh -o posixbuiltins` alike, an assignment
// prefixed to `source` persists exactly as one prefixed to `.` does, and
// under bash a usage error in either ends the script. A list that held one
// spelling and not the other would answer two different ways about one
// builtin depending on which name the script used.
var specialBuiltins = map[string]bool{
	"break": true, ":": true, "continue": true, ".": true, "eval": true,
	"exec": true, "exit": true, "export": true, "readonly": true,
	"return": true, "set": true, "shift": true, "source": true,
	"times": true, "trap": true, "unset": true,
}

func biTrue(*Runner, context.Context, []string) int  { return 0 }
func biFalse(*Runner, context.Context, []string) int { return 1 }

// biSet implements the part of `set` this slice needs: replacing the
// positional parameters.
//
// `set --` with nothing after it clears them, which is different from `set`
// with no arguments at all — that lists variables, in the dialect's shape.
// See setlisting.go.
func biSet(r *Runner, ctx context.Context, args []string) int {
	status := r.setOptionsAndOperands(ctx, args)
	if r.setRefusalOwed {
		// The dialect that reports every bad option word has now read them
		// all, and what it still owes is the usage block and the fatality.
		//
		// Here rather than at the end of the loop, because the loop is not
		// the only way out: `set -A` with no name reports and returns from
		// where it stands, and paying the debt only on the loop's own exit
		// left that refusal with no usage block and no fatality at all. One
		// door out of the builtin is one place to settle, and a later early
		// return cannot silently swallow either.
		return r.finishSetRefusals()
	}
	return status
}

// setOptionsAndOperands reads `set`'s option words and then its operands.
//
// The listing is written from here rather than from inside the loop in the
// one dialect that defers it. See Semantics.SetListsOptionsOnceAtTheEnd: the
// options are listed once, after the whole parse, in the form the last `-o`
// or `+o` decided — so `set -o -e -o` lists once with `errexit on` already in
// it, and `set -o -Z` lists not at all because the parse ended early.
//
// Only on a parse that came out at 0, which is that last row: every early
// return inside the loop is a refusal, and a refusal is exactly what ksh93
// writes no listing after.
func (r *Runner) setOptionsAndOperands(ctx context.Context, args []string) int {
	r.pendingOptionListing = listingNotPending
	st := r.setOptionWordsAndOperands(ctx, args)
	pending := r.pendingOptionListing
	r.pendingOptionListing = listingNotPending
	if pending != listingNotPending && st == 0 {
		return r.listOptions(pending == listingAsInput)
	}
	return st
}

// pendingListingForm is which listing a deferred `set -o` still owes.
type pendingListingForm uint8

const (
	listingNotPending pendingListingForm = iota
	// listingAsTable is what a `-o` that took no word asks for.
	listingAsTable
	// listingAsInput is what a `+o` asks for — and what a `-o` that
	// *declined* its word asks for, which is the rule that separates
	// `set -o -e` from `set -e -o`.
	listingAsInput
)

func (r *Runner) setOptionWordsAndOperands(_ context.Context, args []string) int {
	if len(args) == 0 {
		return r.setListing()
	}
	if st, done := r.refuseBeforeApplyingSetOptions(args); done {
		return st
	}
	// The sort letter is a request about this one call's operands, so it
	// cannot outlive the call that made it.
	r.setSortsOperands = false
	defer func() { r.setSortsOperands = false }()
	// Options come before `--`, and each is a letter that may be turned on
	// with `-` or off with `+`. Only the ones with implemented behavior are
	// accepted; the rest are refused rather than silently ignored, which
	// would let a script believe it had asked for something.
	i := 0
	// Set by the array letter, which turns the operands from positional
	// parameters into an array's elements. See setarray.go.
	arrayName := ""
	arrayFront, haveArray := false, false
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if strings.HasPrefix(a, "--") {
			// A word beginning with `--`, after the bare `--` above, which is
			// the terminator and is a whole word rather than a name of
			// nothing. What the builtin makes of one is the dialect's, and
			// the three readings are measured — see
			// Semantics.SetLongOptionWord.
			switch r.sem().SetLongOptionWord {
			case LongOptionWordIsAnOptionName:
				// Two of these words are not option names at all, and they
				// are asked first because one of them — `--s` — is a prefix
				// no option name would match and the other is a word the
				// namespace has never heard of. See
				// Semantics.SetHasTheStateAndDefaultWords.
				if r.applySetControlWord(a[2:]) {
					continue
				}
				// The same second spelling for the option namespace the
				// invocation has, read by the same function, so the `no`
				// fallback and the `=value` cannot drift between the two
				// routes.
				if !r.applyLongSetOption(a[2:]) {
					return r.setOptionFailure()
				}
				continue
			case LongOptionWordIsDiscarded:
				// Swallowed whole: no option moves, nothing is said, and the
				// word does not fall through to become a positional
				// parameter either. Measured on zsh 5.9.2, `set --zzz q` is
				// status 0 with an empty standard error and `q` as `$1`.
				continue
			}
			// And otherwise it is letters, which is what the loop below
			// reads — the reading four of the six columns have.
		}
		if !setOptionWord(a) {
			break
		}
		on := a[0] == '-'
		if len(a) == 1 {
			// A bare `-` or `+`. Consumed by every column; whether it also
			// turns `-x` and `-v` off is the dialect's, and one column
			// answers no to both signs. See Semantics.BareOptionWord.
			if r.sem().BareOptionWord.clearsTraceAndVerbose(on) {
				r.setLetters("xv", false)
			}
			// And it ends the option parse, exactly as `--` does: measured
			// unanimous 2026-09-13, `set -e - -Z` is errexit on with `-Z`
			// as the one positional parameter in all seven columns, and
			// `set -u - -o zzznosuch` is nounset on with two. Nobody reads
			// the words behind it as options, so there is no axis here.
			//
			// A `continue` here read them, which is the same word this
			// consumes and a different place to stop. The two are
			// indistinguishable on the row that motivated #2699 — `set - a
			// b`, where the next word is no option word and the loop breaks
			// on it anyway — so the reading was never chosen, only
			// inherited. The reading pass in refuseBeforeApplyingSetOptions
			// has always stopped here, since it returns on any word shorter
			// than two characters.
			i++
			break
		}
		// The long spelling, whose name is the next word. Every option this
		// shell has can be written either way, and every shell in the panel
		// uses the same names for them — which is what makes the long form
		// the one that needs no dialect.
		//
		// `o` is read where it sits rather than only as a word of its own,
		// because it is nearly always the last letter of a bundle:
		// `set -euo pipefail` is the line at the top of a great many scripts,
		// and it was refused outright. The letters before it are ordinary
		// letters and still apply.
		//
		// And the *first* `o` rather than a trailing one, because characters
		// behind it are an operand welded to the letter — `set -ozzznosuch`,
		// which is also the spelling `sh -oerrexit` reaches the front end
		// with. Cutting only a trailing `o` sent every such word to the
		// letter table, where `o` is nobody's option letter, so the whole of
		// it came back as `set: -o: invalid option` — an answer no panel
		// column gives (#2640). Which reading applies is
		// Semantics.SetOLetterAttachesItsName.
		if before, after, ok := strings.Cut(a[1:], "o"); ok {
			if !r.setLetters(before, on) {
				return r.setOptionFailure()
			}
			if after != "" {
				if r.ask(r.sem().SetOLetterAttachesItsName,
					"`set -oNAME` reading the rest of the word as the long name") {
					// The namespace the *script* means by a `set -o` name,
					// exactly as the spaced spelling reads it.
					if !r.setNamedOption(after, on) {
						return r.setOptionFailure()
					}
					continue
				}
				if r.unspecified {
					return r.status
				}
			}
			// The word behind a bare `-o` is not always its name: in bash
			// and ksh93 a word this loop would itself read as options is
			// not taken at all, so the `-o` stays a bare one and the word
			// is read as option letters after the listing.
			// Semantics.SetODeclinesADashWord, asked only where there is
			// such a word to decline.
			declined := false
			if i+1 < len(args) && setODeclinedWord(args[i+1]) {
				declined = r.ask(r.sem().SetODeclinesADashWord,
					"`set -o` declining a next word that begins with a dash")
				if r.unspecified {
					return r.status
				}
			}
			if declined || i+1 >= len(args) {
				// With no name to set, `-o` lists the options and `+o`
				// writes them back as input — four shapes, each the
				// dialect's own.
				//
				// Or it does not list here at all. One dialect writes one
				// listing after the whole parse, in the form the last of
				// these decided, and a `-o` that declined its word asks for
				// the `+o` form there. See
				// Semantics.SetListsOptionsOnceAtTheEnd, asked wherever the
				// listing would have happened so that a shell which never
				// reaches one is never asked.
				//
				// Asked only where the two readings can disagree, which is
				// where **something still happens after this listing site**:
				// a word the `-o` declined, which is the only way a second
				// listing site is ever reached and the only way an option
				// changes after one, or welded letters read after the
				// listing. A `set -o` that is the last thing the parse does
				// writes the same listing either way, and asking there would
				// refuse the commonest line in the builtin from any shell
				// whose vector does not answer.
				//
				// And once a listing is pending, this `set` is already in
				// the deferred reading: a second site that listed here
				// would make two listings out of the one this dialect
				// writes. That is the `set +o -o` row.
				deferred := false
				if declined || after != "" || r.pendingOptionListing != listingNotPending {
					deferred = r.ask(r.sem().SetListsOptionsOnceAtTheEnd,
						"the `set -o` listing deferred to the end of the option parse")
					if r.unspecified {
						return r.status
					}
				}
				st := 0
				switch {
				case deferred && (declined || !on):
					r.pendingOptionListing = listingAsInput
				case deferred:
					r.pendingOptionListing = listingAsTable
				default:
					st = r.listOptions(!on)
				}
				// And in the shells that do not weld, what follows the `o`
				// is more option letters, read after the listing rather than
				// instead of it.
				if after != "" && !r.setWeldedLetters(after, on) {
					return r.setOptionFailure()
				}
				if declined {
					// The word the `-o` would not take is left to the loop,
					// which reads it as what it looks like.
					continue
				}
				if after == "" {
					return st
				}
				continue
			}
			i++
			// The namespace the *script* means by a `set -o` name, which is
			// the dialect's own where it has one (#1080).
			if !r.setNamedOption(args[i], on) {
				return r.setOptionFailure()
			}
			// The welded characters are letters here too, and they are read
			// *after* the name the next word gave: measured, `set -oe x` in
			// bash, dash and ash refuses `x` as the name and never turns
			// errexit on.
			if after != "" && !r.setWeldedLetters(after, on) {
				return r.setOptionFailure()
			}
			continue
		}
		// The array letter, which welds its name out of the rest of its word
		// and falls back to the *next word* — the same shape `-o` has, and
		// the only other one in this builtin. The letters in front of it
		// still apply: `set -eA nn 1 2` is errexit as well as an assignment,
		// and `set -xAv p q` turns tracing on and fills `v`, measured in both
		// shells that have the letter.
		//
		// The weld is core rather than an axis because the two columns that
		// have `-A` at all agree on every spelling of it: measured
		// 2026-09-18 from a script file under `env -i PATH=/usr/bin:/bin`,
		// ksh93u+ 2012-08-01 and zsh 5.9.2 both read `set -As a z y` as the
		// array `s` holding `a z y`, `set -Aarr p q` as `arr` holding `p q`,
		// `set -Ae c q` as `e` and not errexit, and `set +As a z y` as the
		// plus form of the same. Reading only the *final* `A` of a bundle
		// sent every welded spelling to the letter table, where it was
		// refused as an unknown option and took the line with it (#3412).
		if idx := strings.IndexByte(a[1:], 'A'); idx >= 0 && r.setArrayLetter() {
			letters, welded := a[1:1+idx], a[2+idx:]
			if !r.setLetters(letters, on) {
				return r.setOptionFailure()
			}
			if welded == "" {
				if i+1 >= len(args) {
					return r.setArrayWithoutAName(on)
				}
				i++
				welded = args[i]
			}
			arrayName, arrayFront, haveArray = welded, !on, true
			cont := r.ask(r.sem().SetArrayOptionsContinuePastTheName,
				"the words after `set -A name` read as options rather than as values")
			if r.unspecified {
				return r.status
			}
			if cont {
				// Options carry on past the name, so the values are whatever
				// the option parse does not claim — which is exactly the
				// words that would have become the positional parameters,
				// and a `--` among them still ends the options.
				continue
			}
			// Or the name ends the options and every word behind it is a
			// value, `--` and dash words included.
			i++
			break
		}
		if !r.setLetters(a[1:], on) {
			// 2 unless the refusal recorded a status of its own, which a
			// denied `set -m` does in one dialect.
			return r.setOptionFailure()
		}
	}
	if r.setRefusalOwed {
		// Before the array assignment and before the positional parameters,
		// because neither happens: `set -q -z` sets nothing in the shell
		// that reports both, exactly as in the four that stop at the first
		// word. What is owed is paid by the caller.
		return r.status
	}
	if haveArray {
		// The positional parameters are left alone: measured, `set -- one two
		// three; set -A a x y` keeps all three of them in both shells. So
		// this returns rather than falling into the replacement below.
		values := args[i:]
		if r.setSortsOperands {
			// The values are sorted before they are placed, so the plus
			// form overlays the front with the sorted list: measured,
			// `a=(z 2); set -s +A a b a` leaves `a b`. The positional
			// parameters are not touched, sort letter or no.
			values = slices.Clone(values)
			slices.SortFunc(values, shellOrder)
		}
		return r.setArrayOperands(arrayName, arrayFront, values)
	}
	// `set -C` alone sets an option and leaves the parameters alone; only an
	// explicit `--`, or operands after the options, replaces them.
	if i == 0 || (i <= len(args) && args[min(i-1, len(args)-1)] == "--") || i < len(args) {
		r.Params = append([]string(nil), args[i:]...)
		// And the replacement is remembered, because one dialect lets a
		// sourced file's own `set` stand where the caller's parameters
		// would otherwise come back — see Semantics.DotSetCancelsTheRestore.
		// Here rather than anywhere else that writes r.Params: measured on
		// both bash builds, `set -x`, a bare `set` and `shift` all leave
		// that restore alone, and only a replacement cancels it.
		r.paramsReplacedBySet = true
	}
	if r.setSortsOperands {
		// With operands they are sorted as they arrive, and with none the
		// parameters already there are: `set -- c b a; set -s` is `a b c`.
		// The order is byte order, which is ksh93's under LC_ALL=C — see
		// shellOrder for why a locale's is not attempted.
		r.Params = slices.Clone(r.Params)
		slices.SortFunc(r.Params, shellOrder)
	}
	return 0
}

// setOptionWord reports whether a word is one `set`'s loop reads as options
// rather than as the first of the operands.
//
// One definition rather than two, because a bare `-o` asks the same question
// about the word behind it: what a dialect declines there is exactly what the
// loop would have gone on to read as letters, and a second predicate would be
// free to drift from the first. `--` satisfies it and is handled a line
// earlier, where it ends the options rather than beginning a word of them.
//
// A one-character `-` or `+` **is** one of these, since #2699. It used not to
// be, which left `set - a b` with three positional parameters and `-` as the
// first where every column of the panel leaves two — the word is consumed
// everywhere, unanimously. What the word then *means* is
// Semantics.BareOptionWord, which is a three-way and not part of this
// predicate.
//
// `setODeclinedWord` shares this, so `set -o -` now declines the `-` and
// lists, which is what bash and ksh93 do and is the row #2698 was holding.
func setOptionWord(a string) bool {
	return len(a) >= 1 && (a[0] == '-' || a[0] == '+')
}

// setODeclinedWord reports whether a bare `-o` will not take this word as its
// long option name — in the dialects that decline one at all, which is
// Semantics.SetODeclinesADashWord.
//
// The empty word is here beside the option words and not behind an axis of
// its own, because the panel splits over it in exactly the same place:
// `set -o ""` lists in the three bash columns and in ksh93 and is refused as
// a name — `Illegal option -o `, `no such option: ` — in dash, BusyBox ash
// and zsh. Two fields could only ever have agreed.
func setODeclinedWord(a string) bool {
	return a == "" || setOptionWord(a)
}

// refuseBeforeApplyingSetOptions is the pass over every option word that runs
// before one of them is applied, and is the whole of where that reading is
// decided.
//
// A dialect that applies as it goes needs nothing here: the loop below is
// already that reading. What this adds is the other one, where a single bad
// word anywhere in the list leaves the shell exactly as it was — so
// `set -e -Q` in bash is errexit **off**, where `set -Q -e` is off in every
// column and tells the two readings apart from nothing.
//
// Two dialects want it and they want different reaches, which is the whole of
// what `names` carries. bash's pass knows the letter table and not the name
// table, so `command set -e -o zzznosuch` is errexit **on** there — the axis
// is Semantics.SetValidatesOptionLettersFirst, and it is asked. ksh93's
// reaches both, because a shell that reports every bad option word has by
// then read every option word: `command set -u -o zzznosuch` is nounset
// **off** there. That second reach is not a value of the letters axis but the
// other half of Semantics.SetReportsEveryBadOption, which no other dialect
// answers yes to — so it is read where that one is read rather than asked
// again (#2670).
//
// See Semantics.SetValidatesOptionLettersFirst for why the invocation and the
// environment are not this question.
func (r *Runner) refuseBeforeApplyingSetOptions(args []string) (int, bool) {
	if r.atInvocation || r.fromEnvironment {
		// The front end's own parse is position-sensitive where this one is
		// not, measured; one pass cannot be both, and guessing that they are
		// one would put the builtin's answer on a route that contradicts it.
		return 0, false
	}
	names := r.reportsEveryBadSetOption()
	preceded, found := r.unknownSetOption(args, names, false)
	if !found {
		// Every option word is one this dialect has, so there is nothing to
		// read ahead for and the loop below is the only pass.
		return 0, false
	}
	if !names {
		if !preceded {
			// The bad word is the first thing in the list, where applying as
			// you go has applied nothing either — the two readings cannot be
			// told apart and neither is worth asking about.
			return 0, false
		}
		if !r.ask(r.sem().SetValidatesOptionLettersFirst,
			"`set` reading every option word's letters before it applies any of them") {
			if r.unspecified {
				return r.status, true
			}
			return 0, false
		}
	}
	// No `preceded` gate on the wider reach, because that dialect leaves
	// nothing applied wherever the bad word stands: `command set -Z -e` is
	// errexit off in ksh93, so the options *behind* a bad one are as unapplied
	// as the ones in front of it. Under the letters-only reading that row is
	// off in every column and says nothing, which is why the gate belongs to
	// the axis that is asked and not to this one.
	//
	// Said the same way the applying loop says it, and by the same calls, so
	// that the dialect which reports every bad word still reports every bad
	// word — with nothing applied, nothing listed and no parameters replaced.
	r.unknownSetOption(args, names, true)
	if r.setRefusalOwed {
		// Paid by biSet, which is the one door out of the builtin.
		return r.status, true
	}
	return r.setOptionFailure(), true
}

// unknownSetOption walks `set`'s option words without applying anything, and
// finds the first one this dialect has not got.
//
// The word shapes are the applying loop's on purpose: a pass that cut the
// words differently would validate options the other one never reads.
// `set -A name` is where it stops rather than guesses: what follows that name
// is another dialect's answer, and this pass may not ask one.
//
// `names` is how far the reading reaches. With it false only the option
// *letters* are read, and the characters behind an `o` are what the `-o`
// takes under either reading rather than letters — which is the whole of why
// a refusal can survive into the applying pass at all. With it true the long
// names are read too, in both spellings the applying loop takes them in: the
// next word after a bare `-o`, and the rest of the word where the dialect
// welds. The two reaches are one walk because the second is the first plus
// the names, and a second walk would be free to cut the words differently
// from this one and from the loop below.
//
// With report false nothing is written and nothing is said. The second result
// is whether anything stood in front of the bad option, which is the only
// place the two letters-readings disagree. With report true each bad option is
// refused by the same call the applying loop refuses one with, so the wording,
// the status, the usage debt and the fatality are all decided in one place
// rather than two.
func (r *Runner) unknownSetOption(args []string, names, report bool) (preceded, found bool) {
	seen := false
	mark := func() {
		if !found {
			preceded, found = seen, true
		}
	}
	refuseLetter := func(opt rune, sign bool) bool {
		mark()
		if !report {
			return false
		}
		return r.badSetOptionLetter(opt, sign)
	}
	refuseName := func(name string) bool {
		mark()
		if !report {
			return false
		}
		return r.badSetOptionName(name)
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" || len(a) < 2 || (a[0] != '-' && a[0] != '+') {
			return
		}
		if w := r.sem().SetLongOptionWord; w != LongOptionWordIsOptionLetters &&
			strings.HasPrefix(a, "--") {
			// Cut the way the applying loop cuts it, which is this pass's
			// whole rule. Read as letters instead, `--noglob` would be an
			// `-n` this dialect has and a `-`, `-o`, `-g` and `-b` it reads
			// as something else entirely.
			//
			// A dialect that discards the word has nothing to refuse, so the
			// reading pass has nothing to say about it either.
			if _, control := setControlWord(a[2:]); control &&
				w == LongOptionWordIsAnOptionName &&
				r.sem().SetHasTheStateAndDefaultWords == Yes {
				// Not an option name and not a bad one: a word this shell's
				// `set` takes on its own account. The applying loop answers
				// it, and a validating pass that refused it first would
				// refuse what that loop grants.
				continue
			}
			if nm, _ := r.longSetOptionName(a[2:]); w == LongOptionWordIsAnOptionName &&
				names && !r.hasSetOptionName(nm) {
				// The spelling travels with the refusal, because this pass
				// is where the report is made: the applying loop never
				// reaches a word this one has already refused.
				r.longSetOptionSpelling = true
				keep := refuseName(nm)
				r.longSetOptionSpelling = false
				if !keep {
					return
				}
			}
			seen = true
			continue
		}
		sign := a[0] == '-'
		letters, read, stop := a[1:], false, false
		if before, after, ok := strings.Cut(letters, "o"); ok {
			letters, read = before, true
			switch {
			case after != "" && r.sem().SetOLetterAttachesItsName == Yes:
				// `set -oNAME`, where the dialect welds. Strictly Yes and not
				// "not No", for the reason hasSetLetter gives about being
				// generous: an unanswered axis leaves the applying pass to
				// report the missing dialect, and a name refused here would
				// be a refusal that pass might never have made.
				if names && !r.hasSetOptionName(after) && !refuseName(after) {
					return
				}
			case after == "" && i+1 < len(args) &&
				(!setODeclinedWord(args[i+1]) || r.sem().SetODeclinesADashWord != Yes):
				// A word the `-o` will not take is not the `-o`'s operand, so
				// this pass reads it as the letters it is — which is the whole
				// of why bash's `set -o -Z` refuses `Z` with no listing
				// written. Read rather than asked, for the reason hasSetLetter
				// is: a pass that applies nothing must not be the one that
				// refuses a dialect for an unanswered axis.
				i++
				if names && !r.hasSetOptionName(args[i]) && !refuseName(args[i]) {
					return
				}
			}
		} else if idx := strings.IndexByte(letters, 'A'); idx >= 0 && r.setArrayLetter() {
			// Cut the way the applying loop cuts it, which is this pass's
			// whole rule: the letter welds its name out of the rest of its
			// word, so nothing behind the `A` is a letter to refuse. Reading
			// only a trailing `A` made every welded spelling a bundle of
			// unknown letters here, and `set -As a z y` was refused before
			// the applying loop ever saw it (#3412).
			letters, stop = letters[:idx], true
		}
		for _, opt := range letters {
			if !r.hasSetLetter(opt) {
				if !refuseLetter(opt, sign) {
					return
				}
				continue
			}
			seen = true
		}
		// The `-o` counts as read only once the letters in front of it are
		// past: `set -Zo pipefail` refuses `Z` with nothing yet applied,
		// which is the same answer under both readings and must not be
		// mistaken for the disagreement.
		if read {
			seen = true
		}
		if stop {
			return
		}
	}
	return
}

// hasSetOptionName reports whether a `set -o` name is one this dialect has at
// all, and is to the names what hasSetLetter is to the letters — including in
// being allowed to be too generous and never too strict. A name it calls this
// dialect's that the applying pass then refuses — one the shell has and will
// not move — costs only that the reading pass misses it and the applying pass
// answers as it did before.
//
// A dialect with a `set -o` namespace of its own is taken at its word without
// being asked, because asking is how that namespace is *moved*: optionMover
// answers "known" and "moved" in one call, and a pass that applies nothing
// cannot make it. No dialect installs one and reports every bad option word,
// so nothing reaches this today; being generous is what keeps that from
// mattering if one ever does.
func (r *Runner) hasSetOptionName(name string) bool {
	if r.optionMover != nil {
		return true
	}
	if name == "pipefail" {
		// The one name with an axis of its own, and setOption asks it there.
		return r.sem().PipefailOption != No
	}
	if _, ok := r.lookupSetOption(name); ok {
		return ok
	}
	// And the spellings this shell's namespace reads that its roster does not
	// publish — a name written with separators it has not got, and a `no` in
	// front of one. Asked here as well as at the applying seam because this
	// pass reports first: a word it called unknown never reaches the loop
	// that would have taken it. See Runner.setOptionNamespaceName.
	resolved, _ := r.setOptionNamespaceName(name, true)
	if resolved == name {
		return false
	}
	_, ok := r.lookupSetOption(resolved)
	return ok
}

// hasSetLetter reports whether an option letter is one this dialect has at
// all, which is the question the pass above asks and the one the applying
// switch below answers by falling through to badSetOptionLetter.
//
// One home for it rather than two: a second table would be a copy of a
// decision, and a copy is what makes removing either one change nothing.
// It reads the axes rather than asking them, because a pass that applies
// nothing must not be the one that reports a missing dialect — the applying
// switch does that, from the same fields, a moment later.
//
// It is allowed to be too generous and never too strict. A letter it calls
// this dialect's that the switch then refuses — `set -p` in a shell with the
// letter and no privileged mode to give it, or an immovable `-t` asked to
// move — costs only that the validating pass misses it and the applying pass
// answers as it did before. A letter it called foreign that the switch would
// have taken would be a refusal no shell makes.
func (r *Runner) hasSetLetter(opt rune) bool {
	if _, ok := r.optionLetterNames[opt]; ok {
		// A letter the dialect spells its own way, asked first for the same
		// reason setLetters asks it first: the letters that need a table are
		// exactly the letters two shells disagree about, and the switch
		// below would refuse one of them on a shared axis rather than answer
		// it. A validating pass that missed such a letter refused `set -G`
		// in the one shell whose `-G` is `globstar` — with `G` in that
		// shell's own usage line, one line down (#3152).
		return true
	}
	if _, ok := setLetterNames[opt]; ok {
		return true
	}
	switch opt {
	case 'h':
		return r.sem().SetHasTheHLetter != No
	case 'E':
		return r.sem().SetHasTheErrtraceLetter != No
	case 'T':
		return r.sem().SetHasTheFunctraceLetter != No
	case 't':
		return r.sem().SetHasTheTLetter != No ||
			strings.ContainsRune(r.diag().ImmovableOptionLetters["set"], opt)
	case 'p':
		return r.sem().SetHasThePrivilegedLetter != No
	case 'f':
		// Accepted by every dialect: either it turns globbing off, or it
		// writes the name the dialect gives it, or it is inert.
		return true
	case 'B':
		return r.sem().SetBTurnsOffBraceExpansion != No
	case 'H':
		return r.sem().HistoryExpansion != No
	case 'k':
		return r.sem().KeywordAssignments != No
	case 's':
		return r.sem().SetSLetterSortsTheOperands != No
	}
	return false
}

// setWeldedLetters reads the characters behind an `-o` as more option
// letters, which is what the five columns that do not weld do with them.
//
// They are the letters Semantics.SetValidatesOptionLettersFirst cannot see,
// so a refusal here is the *applying* pass speaking in a dialect that has
// two, and it answers differently for it. The flag rather than an argument
// because the refusal happens two calls down, past a bool that says only
// whether it worked.
func (r *Runner) setWeldedLetters(letters string, on bool) bool {
	r.setLettersWelded = true
	defer func() { r.setLettersWelded = false }()
	return r.setLetters(letters, on)
}

// setLetterNames maps the letters every shell in the panel spells the same
// way to the long names they abbreviate, so both spellings of an option run
// through the one table and cannot drift apart. The letters missing from it
// are the ones some shell reads differently — f, h, E and T — which stay in
// the switch below, each behind the axis that says whose reading applies.
var setLetterNames = map[rune]string{
	'C': "noclobber",
	// The letter for allexport, which POSIX gives and all four have.
	'a': "allexport",
	'e': "errexit",
	'm': "monitor",
	// One-way: all four shells ignore `set +n` once it is on — and with it
	// on, the `set +n` never runs anyway. The table entry says so.
	'n': "noexec",
	'u': "nounset",
	// POSIX, all four: write input back as it is read.
	'v': "verbose",
	'x': "xtrace",
}

// setLetters applies the short spelling, reporting whether every letter was
// one this shell has.
//
// Split out of the loop above so a bundle ending in `o` can apply the letters
// before it and then read the name that follows — `set -euo pipefail` is
// errexit and nounset as well as pipefail, and dropping either half of that
// would be worse than refusing the whole line.
func (r *Runner) setLetters(letters string, on bool) bool {
	for _, opt := range letters {
		if name, ok := r.optionLetterNames[opt]; ok {
			// A letter this shell spells its own way, which is asked first
			// because the letters that need a dialect table are exactly the
			// letters two shells disagree about: `T` carries traps into
			// functions in one shell and is `cdablevars` in another, and the
			// shared reading below would refuse it on the axis rather than
			// answer it. See Runner.SetOptionLetterNames.
			if name == "" {
				// Taken, and it moves nothing. The measurement rather than a
				// gap — see the installing dialect, where the one letter in
				// this state is named with what was measured about it.
				continue
			}
			if r.immovableName(name) && !r.atInvocation {
				// A letter abbreviating a name this shell will not move
				// from a script is refused **as a letter**, which is not
				// the same sentence the name gets. Measured 2026-09-18 on
				// ksh93u+ 2012-08-01, where `-E` is the invocation's
				// spelling of `rc`: `set -E` is `set: -E: unknown option`
				// and `set -o rc` is `set: rc: bad option(s)`, and the two
				// wordings are the shell telling a letter it has never
				// heard of from a name it lists and will not take.
				return r.badSetOptionLetter(opt, on)
			}
			sign := "-"
			if !on {
				sign = "+"
			}
			if !r.setNamedOptionSpelled(name, sign+string(opt), on) {
				return false
			}
			continue
		}
		if name, ok := setLetterNames[opt]; ok {
			o := commonSetOptions[name]
			if o.try != nil {
				sign := "-"
				if !on {
					sign = "+"
				}
				if !o.try(r, on, sign+string(opt)) {
					return false
				}
				continue
			}
			o.apply(r, on)
			continue
		}
		switch opt {
		case 'h':
			// A letter three shells have and no two mean identically: bash
			// and ksh93 abbreviate command tracking with it (hashall,
			// trackall), zsh a history option (histignoredups), and dash
			// refuses it outright.
			if !r.ask(r.sem().SetHasTheHLetter, "`set -h` being an option letter at all") {
				if r.unspecified {
					return false
				}
				if !r.badSetOptionLetter(opt, on) {
					return false
				}
				continue
			}
			if r.ask(r.sem().SetHLetterTracksCommands, "which option `set -h` abbreviates") {
				// The same state the hashall and trackall table entries
				// write, so the letter and the names cannot disagree.
				r.setCommandTracking(on)
			} else if r.unspecified {
				return false
			} else {
				r.histIgnoreDups = on
			}
		case 'H':
			// History expansion, which bash and ksh93 both abbreviate with
			// this letter and mean the same thing by. zsh has the letter too
			// and means `rmstarsilent` — a different option entirely — so it
			// never reaches here: its own table above answers first, which is
			// exactly what that table is for.
			//
			// Asked rather than assumed because dash has neither the letter
			// nor the feature, and a dash that quietly took `-H` would be
			// promising an expander it has not got.
			if !r.ask(r.sem().HistoryExpansion, "`set -H` rewriting `!!` into the previous command") {
				if r.unspecified {
					return false
				}
				if !r.badSetOptionLetter(opt, on) {
					return false
				}
				continue
			}
			// The same state `set -o histexpand` writes, so the letter and
			// the name cannot answer differently — the rule `-h` and hashall
			// already follow.
			r.SetHistoryExpansion(on)
		case 'k':
			// POSIX's keyword option: every `name=value` word of a simple
			// command becomes a prefix assignment. bash and ksh93 have the
			// letter and mean this by it; zsh has the letter and means
			// `interactivecomments`, so it never reaches here — its own
			// table above answers first, which is exactly what that table is
			// for — and dash and BusyBox ash have neither the letter nor the
			// option, so it is asked rather than assumed.
			if !r.ask(r.sem().KeywordAssignments, "`set -k` making every `name=value` word an assignment") {
				if r.unspecified {
					return false
				}
				if !r.badSetOptionLetter(opt, on) {
					return false
				}
				continue
			}
			// The same state `set -o keyword` writes, so the letter and the
			// name cannot answer differently.
			r.keywordAssignments = on
		case 'E', 'T':
			// The trap-carriage letters. zsh spells different options with
			// the same letters and dash and ksh93 have neither, so a wrong
			// guess here would quietly mean something else — and the two are
			// asked apart because one column has `E` and refuses `T`.
			has, why := r.sem().SetHasTheErrtraceLetter, "`set -E` carrying the ERR trap into functions"
			if opt == 'T' {
				has, why = r.sem().SetHasTheFunctraceLetter, "`set -T` carrying the DEBUG and RETURN traps into functions"
			}
			if !r.ask(has, why) {
				if r.unspecified {
					return false
				}
				if !r.badSetOptionLetter(opt, on) {
					return false
				}
				continue
			}
			if opt == 'E' {
				r.errtrace = on
			} else {
				r.functrace = on
			}
		case 't':
			// `set -t`: read and run one more line, then stop. Two of the
			// panel have the letter and mean this by it, one has it and
			// refuses to move it, and one has never heard of it — so it is
			// asked rather than assumed, and a dialect that answers no
			// reaches its own refusal below.
			//
			// The letter and not the name, because the two shells that have
			// the letter do not both have a long spelling for it: bash lists
			// `onecmd` and ksh93 lists nothing, so a letter routed through
			// the name table would be looking up a name one of them has not
			// got. Both write the same state, which is what keeps `set -t`
			// and `set -o onecmd` one question with one answer.
			if !r.ask(r.sem().SetHasTheTLetter, "`set -t` stopping the shell after one command") {
				if r.unspecified {
					return false
				}
				// A shell that *has* the letter and will not move it still
				// grants a request for the state it is already in, which is
				// the rule every long name follows. Measured 2026-09-10 in
				// the one dialect that refuses it: `set +t` there is silent
				// at 0 with the option off, and `set -t` is silent at 0 in a
				// shell started with `-t` — while the move in either
				// direction is `can't change option`. A shell that has not
				// got the letter at all refuses both, which is why the grant
				// hangs on the dialect saying it has the letter rather than
				// on the state alone.
				has := strings.ContainsRune(r.diag().ImmovableOptionLetters["set"], opt)
				if has && r.atInvocation && r.sem().ImmovableOptionsSetAtInvocation == Yes {
					// And the same shell takes it on the command line that
					// started it, which is a route split inside one shell:
					// measured, `zsh -t script` runs one line where `set -t`
					// in that script stops it dead. The letter's state is
					// the same `onecmd` the name writes, so `$-` reports `t`
					// either way round.
					r.onecmd = on
					continue
				}
				if on == r.onecmd && has {
					continue
				}
				if !r.badSetOptionLetter(opt, on) {
					return false
				}
				continue
			}
			r.onecmd = on
		case 'p':
			// The short spelling of `privileged`. bash, ksh93 and zsh have
			// the letter and all three mean privileged mode by it; dash and
			// ash refuse it, so it is asked rather than assumed.
			//
			// Written through the long name rather than into a field of its
			// own, which is what keeps `set +p` and `set +o privileged` one
			// request: all three shells that have the letter also list the
			// name, measured. This shell has no privileged mode, so the name
			// is one of the entries whose whole answer is "already off" —
			// turning it off is granted and turning it on is refused, which
			// is setoptions.go's bargain and not a special case here.
			if !r.ask(r.sem().SetHasThePrivilegedLetter, "`set -p` being an option letter at all") {
				if r.unspecified {
					return false
				}
				if !r.badSetOptionLetter(opt, on) {
					return false
				}
				continue
			}
			if !r.setNamedOption("privileged", on) {
				return false
			}
		case 'f':
			// Not universal: one shell spells this option the long way only
			// and uses `-f` for something else, which does not touch
			// globbing. Asked rather than assumed, and only here — `set -o
			// noglob` needs no dialect.
			if r.ask(r.sem().SetFTurnsOffGlobbing, "`set -f` turning off pathname expansion") {
				r.noglob = on
				continue
			}
			// The something else, where the dialect names it. Written
			// through the long name rather than into a field of its own so
			// that the letter and the name are one state: measured, `set -f`
			// and `set -o norcs` leave zsh in the same place and both put
			// `f` in `$-` (#1542). A dialect that names nothing leaves the
			// letter accepted and inert, which is what it was before.
			if name := r.sem().SetFLetterOption; name != "" {
				if !r.setNamedOption(name, on) {
					return false
				}
			}
		case 'B':
			// The short spelling of `braceexpand`, in the two shells that
			// mean brace expansion by the letter. The third has it and means
			// the terminal bell, so a dialect that answers no falls through
			// to its own refusal rather than to a silent no-op — which is
			// why this reaches badSetOptionLetter and `-f` above does not.
			// Measured 2026-09-11: `set -B` in that shell writes nothing and
			// leaves `{a,b}` expanding, and ours has never implemented its
			// bell.
			if r.ask(r.sem().SetBTurnsOffBraceExpansion, "`set -B` turning brace expansion off") {
				r.noBraceExpand = !on
				continue
			}
			if r.unspecified {
				return false
			}
			if !r.badSetOptionLetter(opt, on) {
				return false
			}
		case 's':
			// ksh93's sort letter: the operands `set` is given — or, with
			// none, the positional parameters already there — come back in
			// order. It is not an option: nothing is turned on, `$-` does not
			// change, and `+s` sorts exactly as `-s` does. zsh has the letter
			// and means an invocation option by it, so its own table above
			// answers first; bash refuses it and dash takes it as the
			// invocation's standard-input letter. See
			// Semantics.SetSLetterSortsTheOperands.
			if r.ask(r.sem().SetSLetterSortsTheOperands, "`set -s` sorting the operands") {
				r.setSortsOperands = true
				continue
			}
			if r.unspecified {
				return false
			}
			if !r.badSetOptionLetter(opt, on) {
				return false
			}
		default:
			// A letter this shell has not got. What comes back is whether to
			// carry on rather than whether it worked: one dialect reports it
			// and reads the letters behind it, and the debt it leaves — the
			// usage block and the fatality — is on the runner for
			// finishSetRefusals to pay. The other four stop here, which is
			// what a false says.
			if !r.badSetOptionLetter(opt, on) {
				return false
			}
		}
	}
	return true
}

// setOption applies `set -o name`, reporting whether the name is one we have.
//
// The names are the same in every shell in the panel, which is what makes the
// long form the one that needs no dialect: `set -o noglob` means the same
// thing in all four where `set -f` does not.
// badSetOptionName reports a long option name this shell does not have.
//
// The same shape as a bad option letter, and for the same reason: one of the
// panel follows it with a usage line, one puts the builtin's name in the
// location rather than in the sentence, and one answers 1 where the rest
// answer 2.
// badSetOptionLetter reports an option letter this shell does not have.
//
// The letter is its own question, with its own status and its own fatality:
// Diagnostics.SetInvalidOptionLetterStatus and
// Semantics.BadSetOptionLetterFatal, beside the name's two. It read the
// name's pair until #2629 on the strength of a #483 measurement of `-q`,
// `-j`, `-z` and `-A` — four letters, so nothing in it compared a letter
// against a name — and two of the seven columns had been disagreeing all
// along: BusyBox ash ends the script for the letter and not the name, and
// bash 3.2 reports 1 for the name and 2 for the letter. The letter asked
// neither question before that: it reported 2 for everybody and never ended
// a script, so `zsh -q` exited 2 where zsh exits 1 and `set -q` in a dash
// script carried on where dash stops (#483).
//
// The words are the dialect's, in Diagnostics.SetInvalidOptionLetter, because
// the panel spells this four ways — `-q: invalid option`, `Illegal option -q`,
// `-q: unknown option`, `bad option: -q` (#598). The sign rides in the wording
// rather than being one: bash and ksh93 echo back the `+` of `set +q` and dash
// and zsh write `-q` whichever was asked, so the format takes the spelling as
// written and the bare letter and each dialect uses the one it says.
//
// A letter the dialect really has and this shell has not implemented is said
// to be missing instead, the way every other builtin's is — see
// Diagnostics.UnimplementedOptionLetters. Telling a script that `set -b` is
// invalid in a bash that has it would be a worse answer than telling it the
// truth.
func (r *Runner) badSetOptionLetter(opt rune, on bool) bool {
	d := r.diag()
	sign := "+"
	if on {
		sign = "-"
	}
	spelled, bare := sign+string(opt), string(opt)
	if has := d.ImmovableOptionLetters["set"]; strings.ContainsRune(has, opt) {
		// A letter this shell really has and really will not move, which is
		// neither of the other two answers. Worded, statused and made fatal
		// exactly as the same shell's refusal of the *name* is — measured,
		// they are one sentence with one status — and given the letter's own
		// spelling, because that is what the shell echoes back.
		r.saySetRefusal(Wording(d.SetImmovableOptionName, "set: %[1]s: not implemented", spelled),
			d.SetInvalidOptionNameUsage, true)
		return r.setRefusalStatus(refusedOptionLetter,
			"a `set` option letter this shell will not move ending the script")
	}
	msg := Wording(d.SetInvalidOptionLetter, "set: %[1]s: invalid option", spelled, bare)
	usage := true
	if has := d.UnimplementedOptionLetters["set"]; strings.ContainsRune(has, opt) {
		msg, usage = "set: "+spelled+" is not implemented yet", false
	}
	r.saySetRefusal(msg, usage, false)
	return r.setRefusalStatus(refusedOptionLetter, "a refused `set` option letter ending the script")
}

func (r *Runner) badSetOptionName(name string) bool {
	d := r.diag()
	msg := Wording(d.SetInvalidOptionName, "set: %[1]s: invalid option name", name)
	r.saySetRefusal(msg, d.SetInvalidOptionNameUsage, true)
	return r.setRefusalStatus(refusedOptionName, "an unknown `set -o` name ending the script")
}

// saySetRefusal writes one refused `set` option, and takes the whole of the
// difference between the two routes it can arrive by.
//
// Inside a script it is the builtin speaking: the dialect's location, `set`
// named where the dialect names it, and the builtin's own usage line under it
// where the dialect prints one.
//
// At an invocation nothing has been read, and the panel says so — measured
// 2026-09-05 on `-q` and on `-o zzznosuch`. Three of the four drop the
// builtin's name and its location alike and print their own name and the
// sentence, which is `invocationPrefix` and the same wording with the leading
// `set: ` taken off. The usage block is the *shell's* there rather than
// `set`'s, so it comes from a field of its own. bash is the exception on the
// long spelling only, and reports it exactly as the builtin would with its own
// name standing where `set` would — location and all, `bash: line 0: bash:
// zzznosuch: invalid option name`.
func (r *Runner) saySetRefusal(msg string, usage, isName bool) {
	d := r.diag()
	if r.fromEnvironment {
		// A third shape, and the plainest of them: an option name that came
		// out of the environment rather than out of an argument vector or a
		// script. The location is the shell's own, at line 0 like the
		// invocation's, and nothing stands where `set` would — measured,
		// `SHELLOPTS=nosuchoption` draws `<shell>: line 0: nosuchoption:
		// invalid option name` from the one shell that reads the variable,
		// against `<shell>: line 0: <shell>: …` for the same bad name written
		// as `-o`. No usage block either: nobody was given a usage to get
		// wrong.
		r.diagf("%s\n", strings.TrimPrefix(msg, "set: "))
		return
	}
	if !r.atInvocation {
		if r.setRefusalOwed && !r.reportsEveryBadSetOption() {
			// A word behind one this loop has already refused, in the
			// dialect that carries on applying rather than reporting: it
			// reports the *first* bad word and no other. Measured, `set -Z
			// -Y -x -o` writes one sentence there and two in the dialect
			// that reports every one.
			return
		}
		r.diagf("%s\n", msg)
		if usage {
			if r.reportsEveryBadSetOption() {
				// One block after all of them rather than one per word,
				// which is measured: `set -q -z` in the dialect that reports
				// both prints two sentences and a single usage line. Owed
				// here — where it is known that *this* refusal wanted one —
				// and paid in finishSetRefusals.
				r.setUsageOwed = true
				// Which block, decided here where the spelling of *this*
				// word is still known. The payment happens after the whole
				// option loop, by which time nothing remembers whether the
				// word that owed it was a letter or a `--name`.
				r.setUsageOwedBlock = r.setUsageBlock()
				return
			}
			r.sayBuiltinUsage(r.setUsageBlock())
		}
		return
	}
	if rest, ok := strings.CutPrefix(msg, "set: "); ok && isName && d.InvocationNameRefusalNamesTheShell {
		r.diagf("%s: %s\n", r.name(), rest)
		return
	}
	name := r.name()
	switch {
	case d.InvocationOptionRefusalNamesTheBase:
		name = filepath.Base(name)
	case d.InvocationOptionRefusalNamesTheInvocation:
		// The whole word the shell was started by, where this shell's own
		// short name would otherwise stand. See the field: it is the one
		// diagnostic that route writes with a longer name than the rest.
		name = r.invokedAs()
	}
	r.errf("%s%s\n", d.invocationPrefix(name), strings.TrimPrefix(msg, "set: "))
	// A `--name` gets the block that names long options, where the dialect
	// writes a second one — see Diagnostics.InvocationLongOptionUsage for the
	// two measured rows. Drawn through the vector's own helper rather than
	// rendered here, because the front end writes the same block under a word
	// it stopped before the option table ever saw it, and a shell that wrote
	// that block two ways would be two shells.
	if u := d.invocationUsageBlock(r.name(), r.longSetOptionSpelling); u != "" {
		r.errf("%s\n", u)
	}
}

// setUsageBlock is the usage the `set` builtin writes under a refusal, which
// is the dialect's own except where the word was spelled `--name` and the
// dialect writes a second block for that spelling.
func (r *Runner) setUsageBlock() string {
	if r.longSetOptionSpelling {
		if u := r.diag().SetLongOptionUsage; u != "" {
			return u
		}
	}
	return r.diag().BuiltinUsage["set"]
}

// sayBuiltinUsage writes a usage line the way the dialect writes one: after
// its own location, or on a line of its own where the dialect prints no
// prefix in front of it.
func (r *Runner) sayBuiltinUsage(usage string) {
	if usage == "" {
		return
	}
	if r.diag().BuiltinUsageUnprefixed {
		r.errf("%s\n", usage)
	} else {
		r.diagf("%s\n", usage)
	}
}

// setRefusalSpelling is which of `set`'s two refusals is speaking: a long
// `-o` name, or an option letter.
//
// One place for both used to be enough, because the panel answered them
// identically — measured in #483, across the six columns there were then.
// BusyBox ash arrived later and does not: an unknown name reports 1 and the
// script carries on, and an unknown letter ends it at 2. bash 3.2 splits the
// status the same way without splitting the fatality. So the spelling is
// carried to the two places that read a dialect's answer rather than being
// dropped at the door, and every caller has to say which one it is.
//
// See Semantics.BadSetOptionNameFatal for the measurement and for the probe
// that shows the seam is the spelling and not the `-o` route.
type setRefusalSpelling int

const (
	refusedOptionName setRefusalSpelling = iota
	refusedOptionLetter
)

// status is what a refusal of this spelling reports in this dialect.
func (sp setRefusalSpelling) status(d Diagnostics) int {
	if sp == refusedOptionLetter {
		return orDefault(d.SetInvalidOptionLetterStatus, 2)
	}
	return orDefault(d.SetInvalidOptionNameStatus, 2)
}

// fatal is the axis saying whether a refusal of this spelling ends the script.
func (sp setRefusalSpelling) fatal(s *Semantics) Answer {
	if sp == refusedOptionLetter {
		return s.BadSetOptionLetterFatal
	}
	return s.BadSetOptionNameFatal
}

// setRefusalStatus records what a refused `set` option reports and ends the
// script where the dialect says such a refusal is fatal.
//
// It returns whether the option loop should carry on. Only one dialect says
// yes, and there the fatality is owed rather than applied: it has to end the
// script *after* every bad word has been reported rather than instead of the
// ones behind the first. See Semantics.SetReportsEveryBadOption, and
// finishSetRefusals, which pays both debts.
func (r *Runner) setRefusalStatus(sp setRefusalSpelling, why string) bool {
	if sp == refusedOptionLetter && r.setLettersWelded && !r.atInvocation {
		// A letter the validating pass never read, because it was welded
		// behind an `-o` and is the `-o`'s operand to that pass. What refuses
		// it is the applying pass, and there it is an ordinary failure rather
		// than the usage error the first pass reports: 1, with the usage
		// block the wording already wrote, and it does not end the script.
		// Measured in all three bash columns — `set -ozzznosuch` lists the
		// options, says `-z: invalid option` and carries on at 1, where the
		// same shell's `set -Z` is 2 and ends an `sh` script (#2660).
		if r.ask(r.sem().SetValidatesOptionLettersFirst,
			"a `set` option letter welded behind an `-o` being refused by the applying pass") {
			r.setOptionStatus = 1
			return false
		}
		if r.unspecified {
			r.setOptionStatus = r.status
			return false
		}
	}
	status := sp.status(r.diag())
	r.setOptionStatus = status
	if r.reportsEveryBadSetOption() || r.setAppliesPastARefusal() {
		if !r.setRefusalOwed {
			// The first refusal's spelling, because that is the word every
			// other dialect stops at: what this one owes is the fatality it
			// deferred, not a fresh one for the last word it read. ksh93 is
			// the only dialect that gets here by the first route and it
			// answers the two spellings alike, so the choice is unmeasurable
			// today — which is why it is written down.
			r.setRefusalSpelling = sp
		}
		r.setRefusalOwed = true
		return true
	}
	r.endOnSetRefusal(status, sp, why)
	return false
}

// setAppliesPastARefusal reports whether the option loop goes on *applying*
// the words behind one it refused — see
// Semantics.SetAppliesTheWordsAfterARefusedOption, which is a different
// question from how many of them are reported.
//
// Scoped exactly as reportsEveryBadSetOption is, and for the same reason: the
// invocation and environment routes have parse quirks of their own that are
// not this.
func (r *Runner) setAppliesPastARefusal() bool {
	return !r.atInvocation && !r.fromEnvironment &&
		r.sem().SetAppliesTheWordsAfterARefusedOption == Yes
}

// endOnSetRefusal ends the script where a refused `set` option ends one, and
// is the whole of where that is decided.
//
// Nothing to end where the request did not come from `set`. Measured in
// zsh 5.9.2, the dialect that ends a script over this at all: on a pipe,
// `set -m; print st=$?; print DONE` writes `zsh:set:1: can't change option:
// -m` and neither `print` runs, while `setopt monitor; print st=$?; print
// DONE` writes `zsh:setopt:1: can't change option: monitor` and then `st=1`
// and `DONE`, leaving at 0. Same option, same sentence, same status on the
// builtin — so the difference is which builtin asked, and `set` is one of the
// special ones the standard makes fatal on error while a dialect's own option
// builtin is an ordinary one. The environment's list is the third route and
// is not `set` either; it says so already, in ApplyInheritedShellOptions.
func (r *Runner) endOnSetRefusal(status int, sp setRefusalSpelling, why string) {
	if r.outsideSetBuiltin {
		return
	}
	if r.ask(sp.fatal(r.sem()), why) {
		r.status = status
		r.fatalUsageQuiet()
	}
}

// reportsEveryBadSetOption is the axis, read only where `set` itself is
// speaking.
//
// Read rather than asked, and not because the answer is cheap: an unanswered
// axis here would put a complaint about a missing dialect in front of a
// refusal that is already correct for whoever stops at the first word, which
// is what every preset but one does and what the standard describes. The
// invocation and environment routes are excluded for the reason the axis
// gives — the front end's parse has quirks of its own that are not this.
func (r *Runner) reportsEveryBadSetOption() bool {
	return !r.atInvocation && !r.fromEnvironment &&
		r.sem().SetReportsEveryBadOption == Yes
}

// finishSetRefusals pays what the reports above left owing: one usage block
// after all of them, and then the fatality.
//
// Called once, at the end of the option loop, and only in the dialect that
// carried on past a bad word — everywhere else setRefusalStatus has already
// done both and nothing is owed.
func (r *Runner) finishSetRefusals() int {
	usage, block := r.setUsageOwed, r.setUsageOwedBlock
	r.setRefusalOwed, r.setUsageOwed, r.setUsageOwedBlock = false, false, ""
	if usage {
		r.sayBuiltinUsage(block)
	}
	sp := r.setRefusalSpelling
	status := sp.status(r.diag())
	if r.ask(sp.fatal(r.sem()),
		"a refused `set` option ending the script after every bad word is reported") {
		r.status = status
		r.fatalUsageQuiet()
	}
	r.setOptionStatus = 0
	return status
}

// applyLongSetOption applies one `set --name` word, dashes already off, with
// the spelling recorded for the length of the call so that a refusal picks the
// usage block this dialect writes for a long option rather than the one it
// writes for a letter.
func (r *Runner) applyLongSetOption(word string) bool {
	name, on := r.longSetOptionName(word)
	r.longSetOptionSpelling = true
	defer func() { r.longSetOptionSpelling = false }()
	// The spelling a refusal echoes is the word as it was written, for the
	// reason Runner.SetLongOption gives on the other route to the same
	// reading.
	return r.setNamedOptionSpelled(name, word, on)
}

func (r *Runner) setOption(name string, on bool) bool {
	return r.setOptionSpelled(name, name, on)
}

// setOptionSpelled is setOption for a caller whose script wrote the name some
// other way — a spelling this shell's namespace reads and its roster does not
// publish, resolved by Runner.setOptionNamespaceName on the way in.
//
// The refusals echo the spelling and the judgements read the name, and the
// split is measured rather than tidy: `set -o login-shell` in the shell that
// folds separators is `set: login-shell: bad option(s)` — the word as it was
// written — and it is refused at all because `login_shell`, the name it
// resolved to, is one a script may not move.
func (r *Runner) setOptionSpelled(name, spelled string, on bool) bool {
	moved := r.applySetOption(name, spelled, on)
	if moved {
		// And the parameter this name is a second spelling of, where it is
		// one. After the state has moved and only where the request was
		// granted, for the reason an assignment action runs after the store:
		// a refused option moved nothing for a parameter to follow. See
		// interp/tiedoption.go.
		r.optionTieMoved(name, on)
	}
	return moved
}

// applySetOption is setOptionSpelled's own work, with the tie above kept out
// of it so that every road out of the reading below is one return.
func (r *Runner) applySetOption(name, spelled string, on bool) bool {
	if name == "pipefail" {
		// The one name with an axis of its own, because whether the shell
		// has it was settled before this table existed and the answer is
		// the same question in a different shape.
		if r.ask(r.sem().PipefailOption, "`set -o pipefail`") {
			r.pipefail = on
			return true
		}
		if r.unspecified {
			// The refusal is already reported. A second complaint about the
			// same word would only obscure it.
			return true
		}
		return r.badSetOptionName(spelled)
	}
	o, ok := r.lookupSetOption(name)
	if !ok {
		return r.badSetOptionName(spelled)
	}
	if r.immovableName(name) {
		// A name this shell lists and will not take, in either direction —
		// see Runner.AddImmovableSetOptions. Refused exactly as a name it
		// does not have, which is what makes this one line rather than a
		// wording of its own: measured, the shell this is for says
		// `bad option(s)` and prints its usage for `interactive` word for
		// word as it does for a name it has never heard of, while the row
		// stays in its listing and still reports which state it is in.
		//
		// Unless the words the shell was *started* with are asking, in the
		// dialect that lets them — the same route split the `t` letter
		// takes above, and the reason Semantics.ImmovableOptionsSetAtInvocation
		// is asked where the route is known. Measured 2026-09-16 on ksh93u+
		// 2012-08-01 with the program on a pipe: `ksh -o interactive` draws
		// a prompt, runs the line and draws another, while `set -o
		// interactive` in that same shell is `bad option(s)` in both
		// directions. So the three names it will not move are three a
		// *script* may not move (#3221).
		//
		// And only where there is something to move: the axis governs the
		// refusal and not the applying, so a name the table records without
		// acting on — `rc`, `login_shell` — is refused at the invocation
		// exactly as it is refused in a script, in the wording it already
		// had. Granting it here would trade `bad option(s)` for a
		// `not implemented` those shells never say.
		if !r.atInvocation || r.sem().ImmovableOptionsSetAtInvocation != Yes || o.apply == nil {
			return r.badSetOptionName(spelled)
		}
	}
	if r.inertOptions[name] {
		// A name this shell lists and takes in both directions with nothing
		// behind it moving — see Runner.AddInertSetOptions. Granted before
		// the table is consulted, because what the substrate could do with
		// the state is exactly what this says does not happen.
		return true
	}
	if o.try != nil {
		// A request the dialect may refuse, handed the spelling it was asked
		// with — the refusal echoes it back.
		return o.try(r, on, name)
	}
	if o.apply != nil {
		o.apply(r, on)
		return true
	}
	if on == o.on {
		// Already where it is being asked to be, so the request has been
		// granted. This is what makes `set +o posix` work in a shell with no
		// posix mode, rather than stopping a script over a state it already
		// had.
		return true
	}
	// A name this shell has and does not do. Refused out loud rather than
	// accepted quietly, because accepting would be promising to behave
	// differently afterwards.
	r.diagf("set: %s: not implemented\n", spelled)
	return false
}

// unsetFunction removes one function, and reports what the dialect reports.
//
// Two questions, and the panel answers them independently — which is what
// makes them two fields. ksh93 judges the *name*: `1x` could never be a
// function name, and it says so whether or not a function exists. zsh
// reports the *table*: it complains about any name it does not hold,
// including a perfectly well formed one. bash and dash say nothing about
// either, and unsetting a function that is there is quiet in all four.
func (r *Runner) unsetFunction(name string) int {
	d := r.diag()
	if !isPlainName(name) && !r.dottedFunctionNameIsWellFormed(name) &&
		r.ask(r.sem().UnsetFunctionChecksTheName, "`unset -f` judging the name it was given") {
		r.diagf("%s\n", Wording(d.UnsetBadFunctionName, "unset: %[1]s: invalid function name", name))
		return 1
	}
	fn := r.funcs[name]
	if r.speaksForTheShell(fn) {
		// The shell's own, so the script has no function of that name to
		// remove and this is the "not defined" case rather than a removal
		// (#1082). Every shell in the panel has `dirs`, `popd` and `pushd`
		// as builtins, and in every one of them the name still works after
		// `unset -f`: measured, bash is silent at 0 and zsh writes the same
		// `no such hash table element` it writes for any name it does not
		// hold, at 1. That is the field below and not a new one — a name
		// this shell provides is exactly a name the script never defined.
		fn = nil
	}
	if fn == nil {
		if r.ask(r.sem().UnsetFunctionReportsMissing, "`unset -f` naming a function that is not defined") {
			r.diagf("%s\n", Wording(d.UnsetFunctionNotFound, "unset: %[1]s: not found", name))
			return 1
		}
		return 0
	}
	// A frozen function is not removed either — the refusal `readonly -f`
	// exists to raise, in the words this shell uses for the other table. The
	// name is still there afterwards and still holds its body, which is what
	// separates this from the quiet 0 above (#3192).
	if code, refused := r.readonlyFunctionUnset(name); refused {
		return code
	}
	r.removeFunction(name)
	return 0
}

// parameterNamespaceHolds reports whether a plain `unset` has a *parameter*
// of this name to remove, which is what decides whether the function table is
// reached behind it at all.
//
// It is deliberately not nameIsSet. That asks whether the name has a value,
// and a declaration with none is still a parameter: measured 2026-09-16 on
// bash 5.3.20, every one of `declare a`, `declare -i b`, `declare -a c`,
// `declare -A d`, `declare -l`, `declare -x`, `export e` and a bare `local`
// inside a call leaves a function of the same name standing after one `unset`
// and takes a second to reach it. Reading only the value made all eight of
// them remove the function on the first.
//
// A **reference is resolved and does not itself count**, which is the same
// rule nameIsSet already keeps and is measured too: `v=1; declare -n n=v;
// unset n` takes `v` away and leaves the function `n` callable, while
// `declare -n n=nowhere; unset n` finds nothing at the far end and removes
// the function.
//
// The letters come from captureAttributes rather than from a list written
// again here, so a letter added to that struct is counted by this without a
// second edit — the failure this tree keeps having is a second helper that
// omits what the first one learned. The export attribute is not in that
// struct and is read beside it: `export e` with no value is a parameter, and
// dropping this clause removes the function on the first `unset` for both
// `export f` and `declare -x f`.
//
// The readonly attribute is deliberately *not* read here, and it was until a
// mutation said so. A frozen parameter refuses the `unset` in biUnset before
// this is ever called, so a clause for it killed nothing — the one shape that
// could reach it is a dialect's absent parameter, which unsetReadonly exempts
// on purpose, and no preset that answers this axis `Yes` has any.
func (r *Runner) parameterNamespaceHolds(name string) bool {
	if r.nameIsSet(name) {
		return true
	}
	name = r.throughNameref(name)
	if r.isExported(name) {
		return true
	}
	a := r.captureAttributes(name)
	a.nameref, a.isNameref = "", false
	// The record an `unset` of this scope's own local leaves behind is the
	// one piece of captureAttributes that does **not** count as a parameter
	// here, and it is measured rather than reasoned: with `wrap() { f() {
	// …; }; local f; unset f; unset f; }`, the *first* `unset` takes the
	// local and the *second* reaches the function in bash 5.3.20. So the
	// placeholder is a row in a listing and is not a parameter this table
	// holds — where a bare `local f` that nothing has unset is one, which
	// is the row above it in the same measurement.
	//
	// See interp/unsetenclosinglocal.go, and nameAttributes, where the two
	// records are two fields precisely so that this line can tell them
	// apart.
	a.unsetLeftItDeclared = false
	return a != nameAttributes{}
}

// unsetTheFunctionInstead is the plain `unset NAME` that reaches the
// *function* table, and reports whether it was this shell's answer and what
// status it carries.
//
// The whole of the dialect's question is Semantics.UnsetReachesTheFunctionTable
// — bash alone — and this is where it is asked. Deliberately narrow: it says
// no before consulting the vector unless the name has a function and holds no
// variable, because that is the only shape the panel disagrees about. A plain
// `unset x` over an ordinary parameter is a parameter's removal in all seven
// columns, and asking there would refuse it in a preset that has no answer.
//
// "Holds no variable" is nameIsSet and so is the declaration rather than the
// value: measured 2026-09-16 on bash 5.3.20, `f() { :; }; declare f; unset f`
// takes the valueless declaration and leaves `f` callable, and it takes a
// second `unset f` to reach the body. `f=` and `f=(a b)` behave the same, and
// so does a `local f` inside a call — one table per call, the variable's
// first.
//
// A name this shell provides is not the script's to remove, which is the same
// rule unsetFunction already keeps: `unset pushd` reaches a prelude function
// standing in for a builtin, and a builtin is not what a plain `unset`
// removes anywhere.
//
// `unset -v` is not the plain spelling and takes none of this, which is the
// letterV argument and is measured: `f() { :; }; unset -v f` leaves `f`
// callable in bash 5.3.20, where `unset f` does not. The letter names the
// parameter namespace, so a name with nothing in it is a quiet 0 there
// rather than a function's removal — and reading `-v` as "the plain form
// with a name check" is what let it through here first time.
func (r *Runner) unsetTheFunctionInstead(name string, letterV bool) (bool, int) {
	fn := r.funcs[name]
	if letterV || fn == nil || r.speaksForTheShell(fn) || r.parameterNamespaceHolds(name) {
		return false, 0
	}
	if !r.ask(r.sem().UnsetReachesTheFunctionTable,
		"a plain `unset` over a name that is a function and not a variable") {
		return false, 0
	}
	// Through the freeze rather than around it. `readonly -f b` refuses a
	// plain `unset b` in the same sentence it refuses `unset -f b` — status
	// 1, the body still there, and the script carries on — and that refusal
	// had no reachable caller until this route existed (#3192, #3206).
	if code, refused := r.readonlyFunctionUnset(name); refused {
		return true, code
	}
	r.removeFunction(name)
	return true, 0
}

// removeFunction takes a function out, and gives the name back to the
// prelude's where there was one.
//
// One mechanism for what is two spellings: `unset -f pushd` and — in the
// dialect that has it — `unset "functions[pushd]"` are the same operation
// asked for two ways, and a shell where they left the name in two different
// states would be two shells. A redefinition took the voice away and removing
// it gives the name back: the shells with the builtin uncover it when the
// function shadowing it goes, so `pushd() { echo mine; }; unset -f pushd;
// pushd /tmp` pushes in both of them. Reconstructing the namespace boundary
// by hand, because a dialect written as shell has one table where they have
// two.
func (r *Runner) removeFunction(name string) {
	// A `TRAP…` function *is* a trap in the dialect that reads names that
	// way, so removing it untraps the condition — measured, `TRAPZERR(){ … };
	// unset -f TRAPZERR; false` fires nothing. Here rather than at `unset`'s
	// own site so that every spelling of a removal reaches it, which is the
	// reason this function exists at all.
	r.unbindTrapFunction(name)
	r.removeFunctionQuietly(name)
}

// removeFunctionQuietly is removeFunction without the trap it may have
// stood for, for the callers that are already changing that condition and
// would otherwise undo their own work on the way past.
func (r *Runner) removeFunctionQuietly(name string) {
	delete(r.funcs, name)
	delete(r.funcOrigins, name)
	delete(r.exportedFuncs, name)
	if prelude := r.preludeFuncs[name]; prelude != nil {
		r.funcs[name] = prelude
	}
}

// badSubscriptToUnset reports an `unset` operand whose subscript would not
// evaluate, and answers with the status the builtin carries.
//
// Two answers, and they are not a wording difference. bash gives up the
// command it is running — the rest of that line with it, and the enclosing
// function, list or subshell whole — and carries on at the next top-level
// command; ksh93 and zsh leave a failed builtin behind and go on to the very
// next command, which is the shape a script can test. ksh93 also names the
// builtin in front of the sentence, where it words the identical failure in
// an expansion without one.
//
// **Nothing here ends the script**, which is what this used to claim about
// bash: the axis was a bool spelled "fatal" and bash answered Yes, so an
// `unset` with an unevaluable subscript stopped a script that bash runs to
// the end. See Semantics.BadSubscriptToUnset for the rows and for why an
// obvious probe cannot tell the two apart (#3485).
func (r *Runner) badSubscriptToUnset(sub string, err error) int {
	sentence := r.subscriptFailure(sub, err)
	// The complaint is the shell's rather than the builtin's — it is the same
	// sentence the same shell writes about the same text inside `$(( ))` — so
	// the location must not name `unset`. The one dialect that does name it
	// puts it in the message, where it puts every other builtin's name.
	//
	// **The location is a separate claim** and one column keeps the builtin's
	// there while the sentence stays the language's: see
	// Diagnostics.BadSubscriptKeepsTheBuiltinsLocation, where the control
	// line that tells the two apart is. Clearing the speaker outright made
	// both claims at once (#3496).
	outer := r.inBuiltin
	r.inBuiltin = r.keptBuiltinLocation(outer)
	defer func() { r.inBuiltin = outer }()
	return r.badSubscriptGivesUp(r.sem().BadSubscriptToUnset,
		"how much an `unset` operand's unevaluable subscript gives up",
		Wording(r.diag().UnsetBadSubscript, "%[1]s", sentence))
}

// unsetReadonly refuses to remove a readonly name, reporting 0 when there was
// nothing to refuse.
//
// Every shell in the panel refuses, says so, and leaves the value standing —
// `readonly x=1; unset x; echo "${x-gone}"` prints 1 in all six — so the
// refusal is the core answer and only what follows it splits. dash and zsh end
// the script; bash and ksh93 report and carry on, and go on to the *rest* of
// the operands: `readonly x=1; y=2; unset x y` leaves x standing and removes
// y, which is why this reports per name rather than giving up the builtin.
//
// The status is 1 wherever it can be seen, which is the three that carry on;
// the shell that stops carries FatalErrorStatusIsOne's, as every fatal error
// does, so there is no status of this error's own.
func (r *Runner) unsetReadonly(name string) int {
	refused := r.unsetRefused[name] && !r.readonly[name]
	if !r.readonly[name] && !refused {
		return 0
	}
	if r.AbsentParameter(name) {
		// A parameter the dialect names and this shell has not got is frozen
		// against a *write* and not against `unset`, which is measured
		// rather than a tidiness: the shell being modeled refuses
		// `jobstates=(a b c)` as a read-only variable and answers `unset
		// jobstates` with a silent 0 on the very next line. The two are one
		// attribute here, so the exemption is written down where the
		// difference is — see the dialect's registerAbsentParameters.
		return 0
	}
	// The builtin has been named in the sentence by three of the four
	// dialects, so it must not be named in the *location* by the one that
	// puts every other builtin's name there: zsh writes `zsh:1: read-only
	// variable: x` here and `zsh:unset:1: 1x: invalid parameter name` for a
	// bad name, from the same builtin. It is the same care setVarAs takes for
	// the assignment this refusal is the twin of.
	//
	// **And it is that dialect's answer rather than a rule about the
	// refusal**, which is what the second dialect to name a builtin in a
	// location showed: BusyBox ash writes `./z.sh: unset: line 2: r: is read
	// only`, keeping the name zsh drops. See
	// Diagnostics.UnsetReadonlyIsTheShellsOwn for both rows (#2761).
	//
	// Only there, and that is the whole of the condition. Forgetting the
	// builtin outright also forgets that a builtin is *speaking*, which is a
	// second question and a different dialect's: ksh93 locates a builtin's
	// complaint `<shell>[3]: ` where it locates the shell's own `<shell>:
	// line 3: `, and this refusal is the builtin's there — `/bin/ksh[3]:
	// unset: warning: r: is read only` against `/bin/ksh: line 3: r: is read
	// only` for the assignment refused for the same reason. Measured
	// 2026-09-12, and both forms were already in this tree with nothing
	// choosing between them (#2417).
	if r.diag().NamesBuiltinInLocation && r.diag().UnsetReadonlyIsTheShellsOwn {
		outer := r.inBuiltin
		r.inBuiltin = ""
		defer func() { r.inBuiltin = outer }()
	}
	msg := Wording(r.diag().UnsetReadonly, "unset: %s: cannot unset: readonly variable", name)
	if refused {
		// See Runner.RefuseUnset: the same refusal with no reason given.
		msg = Wording(r.diag().UnsetRefused, "unset: %s: cannot unset", name)
	}
	if r.ask(r.sem().UnsetReadonlyFatal, "unsetting a readonly name ending the script") {
		// fatal carries FatalErrorStatusIsOne's number, which is the whole of
		// the status question here — dash exits 2 and zsh 1, and neither is
		// this error's own.
		r.fatal("%s\n", msg)
		return r.status
	}
	if r.unspecified {
		return 2
	}
	r.diagf("%s\n", msg)
	return 1
}

// unsetWithoutOperands is `unset` with nothing to unset — no operand at all,
// no name after `-v` or `-f`, and no pattern after `-m`.
//
// One place because the panel answers it once, not once per spelling:
// measured 2026-09-08, zsh 5.9.2 writes `not enough arguments` at 1 for every
// one of those four, and bash 5.3, bash 3.2 and dash are silent at 0 for
// every one they have. So the wording is the whole answer and an empty one
// means the dialect says nothing and reports success.
//
// ksh93 is the panel member this does not yet cover: it writes its usage line
// and, `unset` being one of its special builtins, ends the script. That is
// the usage-line shape rather than this sentence, and it is left recorded in
// the corpus rather than guessed at here.
// unsetOnlyRefusingReadonly is `unset -n` in the shell that reads a name which
// is not a reference as naming nothing: no name is removed, and the one thing
// the letter does not excuse is still refused.
//
// Measured 2026-09-12 on bash 5.3.15: `readonly r=1; unset -n r` answers
// `unset: r: cannot unset: readonly variable` at 1, which is word for word
// what the same shell says without the letter — so the refusal is not skipped
// along with the removal, and a script cannot use `-n` to find out quietly
// whether a name is readonly.
func (r *Runner) unsetOnlyRefusingReadonly(names []string) int {
	status := 0
	for _, name := range names {
		base, _, subscripted := r.subscriptOperand(name)
		if !subscripted {
			base = name
		}
		if code := r.unsetReadonly(base); code != 0 {
			status = code
			if r.ctl == controlExit {
				return status
			}
		}
	}
	return status
}

func (r *Runner) unsetWithoutOperands(name string) int {
	wording := r.diag().UnsetNoOperands
	if wording == "" {
		return 0
	}
	r.diagf("%s\n", Wording(wording, "%[1]s: not enough arguments", name))
	return 1
}

// unsetFunctions is the `-f` half of `unset`, which is the whole of
// `unfunction`.
//
// Its own function because the second name must be the *same* removal and not
// a copy of it: the name check, the prelude's functions being untouchable and
// the "no such hash table element" complaint are all one implementation, so a
// fix to any of them reaches both words. See functionsbuiltin.go.
func (r *Runner) unsetFunctions(names []string, matching bool) int {
	if matching {
		return r.unsetMatchingFunctions(names)
	}
	status := 0
	for _, name := range names {
		if code := r.unsetFunction(name); code != 0 {
			status = code
		}
	}
	return status
}

// unsetMatchingFunctions is `unset -f -m`, and so `unfunction -m`: each
// operand is a pattern and every function whose *name* it matches goes.
//
// The letter used to be read before `-f` was, so `unset -f -m 'zi-*'` walked
// the *parameter* table and removed variables while every function it named
// survived — a wrong thing done in silence at status 0, which is the failure
// class this tree keeps finding.
//
// Nothing matching is 1 rather than 0, measured on all three spellings: a
// removal that removed nothing has not done what it was asked, where the
// listing under the same letter has. One pattern matching is enough —
// `unfunction -m 'f*' 'zz*'` with an `fa` defined is 0.
func (r *Runner) unsetMatchingFunctions(patterns []string) int {
	matched := false
	for _, pattern := range patterns {
		o := r.patternOpts(pattern)
		// Collected before anything is removed, because the table being
		// walked is the one being changed.
		for _, name := range r.scriptFuncNames() {
			if !matchPattern(pattern, name, o) {
				continue
			}
			matched = true
			r.removeFunction(name)
		}
	}
	if !matched {
		return 1
	}
	return 0
}

// unsetMatching is `unset -m`: each operand is a pattern, and every parameter
// whose name it matches goes.
//
// The names are collected before anything is removed, because the tables are
// what is being walked; and they are sorted so that a diagnostic from one
// removal — a readonly name — arrives in the same order every run.
//
// Nothing matching is 1, the same answer the function table's `-m` gives and
// measured the same way: `unset -m 'zz*'` in zsh 5.9.2 with no such parameter
// is a silent 1, and this reported success.
func (r *Runner) unsetMatching(patterns []string) int {
	status := 0
	matched := false
	for _, pattern := range patterns {
		// Collected before anything is removed, because what is being
		// walked is the tables themselves; and sorted, so that a refusal
		// from one removal arrives in the same order every run.
		o := r.patternOpts(pattern)
		for _, name := range r.parameterNames() {
			if !matchPattern(pattern, name, o) {
				continue
			}
			matched = true
			if code := r.unsetReadonly(name); code != 0 {
				status = code
				if r.ctl == controlExit {
					return status
				}
				continue
			}
			if code := r.unsetName(name); code != 0 {
				status = code
			}
		}
	}
	if !matched && status == 0 {
		return 1
	}
	return status
}

// parameterNames is every parameter this shell can see, once each and in
// order. The same three sources [Runner.namesWithPrefix] reads.
//
// The array tables are deliberately not among them, and that is a fact about
// this shell rather than an omission: an array's name is in Vars as well —
// `a=(p q)` writes both — so walking Arrays and AssocArrays here added
// nothing at all. It was written, and a mutant that deleted it survived every
// test including the one about arrays, which is how the duplication was
// found. Should the two tables ever come apart, this is one of the places
// that has to be told.
func (r *Runner) parameterNames() []string {
	seen := map[string]bool{}
	var out []string
	// The `removed` half of the guard is the one no test can see, and it is
	// here because a name `unset` has already taken away is not a parameter
	// — not because anything would go wrong without it. Removing a name
	// twice is removing it once, so a mutant that drops this clause passes
	// everything, the same standing this codebase gives an escaped ordinary
	// character. It is the *rule* that is being written down.
	add := func(name string) {
		if seen[name] || r.removed[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}
	for name := range r.Vars {
		add(name)
	}
	for name := range r.Dynamic {
		add(name)
	}
	for name := range r.inheritedEnv {
		add(name)
	}
	sort.Strings(out)
	return out
}

func biUnset(r *Runner, _ context.Context, args []string) int {
	letters := r.sem().UnsetOptions
	if letters == "" {
		letters = "vf"
	}
	args, opts, code := r.builtinOptions("unset", args, letters)
	if code != 0 {
		return code
	}
	if w := r.diag().UnsetFunctionAndVariable; w != "" &&
		strings.ContainsRune(opts, 'f') && strings.ContainsRune(opts, 'v') {
		// Both tables named at once, which the dialect with a sentence for
		// it refuses ahead of everything else on the line — with no operand
		// at all as well. See Diagnostics.UnsetFunctionAndVariable.
		r.diagf("%s\n", w)
		return 1
	}
	if len(args) == 0 {
		// Nothing to unset, however it was spelled. Ahead of every branch
		// below because the one shell that complains gives the same sentence
		// for a bare `unset`, for `-v` and `-f` with no name, and for `-m`
		// with no pattern — see unsetWithoutOperands.
		return r.unsetWithoutOperands("unset")
	}
	if strings.ContainsRune(opts, 'f') {
		// `unset -f` is about functions and not about variables, unanimously
		// — and the option was read and then ignored, so a function survived
		// being unset and went on answering to its name. The exported set
		// goes with it: what is not a function cannot be carried as one.
		//
		// Ahead of `-m` rather than behind it, which is the fix and not the
		// arrangement: `-m` used to be read first and take the operands as
		// patterns over the *parameter* table, so `unset -f -m 'f*'` removed
		// variables and left every function it was asked about standing.
		// The two letters together are one question — which namespace the
		// patterns are matched in — and `-f` is what answers it.
		return r.unsetFunctions(args, strings.ContainsRune(opts, 'm'))
	}
	if strings.ContainsRune(opts, 'm') {
		// `unset -m` reads its operands as patterns and unsets every
		// parameter whose *name* matches one. Ahead of the name check,
		// because a pattern is not a name and would not survive it.
		return r.unsetMatching(args)
	}
	if strings.ContainsRune(opts, 'n') {
		// The letter names the **reference** and not what it points at, which
		// is the whole of the difference between `unset -n r` and `unset r`:
		// measured on bash 5.3.15 and ksh93u+ alike, `v=1; typeset -n r=v;
		// unset -n r` leaves `v` holding 1 and `r` gone, where the plain
		// `unset r` takes `v` away and leaves nothing behind either.
		//
		// Ahead of the axis below, which is the question about a name that is
		// *not* a reference — a real disagreement, and one this branch does
		// not reach. See interp/nameref.go.
		rest := args[:0:0]
		status := 0
		for _, name := range args {
			if !r.isNameref(name) {
				rest = append(rest, name)
				continue
			}
			if r.readonly[name] {
				// A **frozen reference**, which `declare -rn r=v` makes and
				// which this branch used to take away in silence. The freeze
				// is the reference's own — measured 2026-09-16 on bash
				// 5.3.20, `v=1; declare -rn r=v; unset -n r` is `unset: r:
				// cannot unset: readonly variable` at 1 and leaves `r` still
				// aimed at `v`, while `unset -n` over an unfrozen reference
				// is the silent 0 above. Same sentence and same route as the
				// plain `unset` refusal, so a script cannot use `-n` to take
				// apart what it may not unset.
				if code := r.unsetReadonly(name); code != 0 {
					status = code
				}
				if r.unspecified || r.ctl == controlExit {
					return r.status
				}
				continue
			}
			r.unsetNameref(name)
		}
		if len(rest) == 0 {
			return status
		}
		args = rest
	}
	if strings.ContainsRune(opts, 'n') &&
		!r.ask(r.sem().UnsetReferenceLetterRemovesANonReference,
			"`unset -n` on a name that is not a name reference") {
		if r.unspecified {
			return r.status
		}
		// The letter names the reference rather than what it points at, and
		// this shell has no name references — so under this answer there is
		// nothing here for `unset` to remove. Ahead of the name check because
		// the shell that answers this way skips that too: `unset -n 1x` is
		// silent at 0 where plain `unset 1x` refuses the identifier.
		return r.unsetOnlyRefusingReadonly(args)
	}
	// After `-f`, so that a function name keeps its own laxer rule: bash
	// takes `unset -f 1x` without a word where it refuses `unset 1x`.
	letterV := strings.ContainsRune(opts, 'v')
	args, status, ended := r.builtinNames("unset", args, letterV)
	if r.unspecified {
		return status
	}
	for _, name := range args {
		// A reference aimed at an **element** is unset through: `declare -n
		// r=B[1]; unset -v r` takes `B[1]` away and leaves the rest of `B`,
		// measured 2026-09-17 on bash 5.3.20, where this shell left the
		// element standing. Only the element shape is redirected here; a
		// reference aimed at a plain name is already followed below, and the
		// two answers must not be written twice.
		//
		// **Only the delete is followed here, never the name.** Rewriting the
		// name for a plain-name aim as well reads plausibly and is wrong: the
		// unset falls through to the *function* table when the parameter it
		// names is not there, and that fallthrough is under the name the
		// script wrote — `n(){ :; }; declare -n n=nowhere; unset n` removes
		// the function in bash and left it standing with the name rewritten.
		// What the reference does decide is the **freeze**, which is
		// frozenNameOfAnUnset a few lines below.
		written := name
		if aimed, is := r.namerefTarget(name); is && !isNameLike(aimed) {
			name = aimed
		}
		// The brackets this command wrote unquoted are the parser's, and the
		// subscript between them is a word it already expanded: an
		// apostrophe in it came out of a value and is a byte of the key.
		// Asked of the operand as *written*, ahead of the reference rewrite
		// above, because the set records the text the command line carried.
		// See Runner.subscriptOperandRead for the six spellings.
		lexed := r.operandBracketsWereLexed(written)
		base, sub, subscripted := r.subscriptOperandRead(name, lexed)
		if !subscripted {
			base = name
		}
		// A reference aimed at a **name**, with the brackets on the operand:
		// `a=(x y); declare -n r=a; unset "r[0]"`. The rewrite above is the
		// other way round — the reference itself carries the subscript — and
		// cannot see this one, because `r[0]` is not a name the reference
		// table holds. So the base is followed here, after the operand has
		// been taken apart and before anything reads either half.
		//
		// Measured 2026-09-21, bash 5.3.20, where every one of these is the
		// answer `a` would have given to the operand written with its own
		// name, and none of them was the answer here:
		//
		//	a=(x y); unset "r[0]"              the element goes, `a` stays
		//	a=(x y); readonly a; unset "r[0]"  `unset: a: …: readonly`, at 1
		//	a=(x y z); unset "r[@]"            every element goes
		//	declare -A m=([k]=v [j]=w); unset "r[k]"   the key goes
		//
		// The first is the one that mattered: an index of 0 over a name
		// holding no array is a whole parameter's removal, so the removal
		// landed on the *reference*, followed it, and took the entire array
		// away where bash took one element (#4071).
		//
		// **Only a target that is a plain name.** A reference aimed at an
		// element and then subscripted again — `declare -n r=a[1]; unset
		// "r[0]"` — is silent at 0 in bash with the array untouched, which
		// is what leaving the base as written already does here.
		//
		// The base alone, never the operand's text: what a diagnostic quotes
		// is the name the *removal* lands on, which is why the freeze below
		// reads `base` and says `a`.
		if subscripted {
			if aimed, is := r.namerefTarget(base); is && isNameLike(aimed) {
				base = aimed
			}
		}
		// What the subscript *is* decides which element the arithmetic, the
		// flag group and the emptiness refusal below are about: a text a
		// second round turns into `x y` names that key from here on. The
		// round itself is further down, behind the name — see the note
		// there.
		//
		// The operand's own text is deliberately *not* rewritten with the
		// key the round found. What a diagnostic quotes is what the script
		// wrote — `unset 'a[(r)$k]'` names the flag group the script typed —
		// and the element the removal lands on is `sub`, which is the half
		// that moved.
		//
		// Before the subscript is read, and named by the *base*: `unset a[0]`
		// against a readonly `a` is refused by the variable the subscript
		// indexes, and the two shells with arrays that refuse it say `a`
		// rather than `a[0]`. The element is never reached, so the subscript
		// is not evaluated either — which is why this stands ahead of the
		// whole subscripted branch rather than inside it.
		//
		// And asked of the name the unset **lands on**, which through a
		// reference is the target. See frozenNameOfAnUnset.
		//
		// **Unless the brackets name no element at all**, where the freeze on
		// the array is not the question that gets asked: measured 2026-09-17,
		// `a=(1 2 3); readonly a; unset 'a[]'` is silent at 0 in bash 5.3.20
		// and `invalid subscript` at 1 in zsh 5.9.2 — neither mentions the
		// freeze — where ksh93, which reads the empty brackets as element
		// zero, does refuse it. See operandEmptySubscript.
		frozen := r.frozenNameOfAnUnset(base, subscripted)
		if subscripted && r.operandEmptySubscript(sub) {
			frozen = name
		}
		if code := r.unsetReadonly(frozen); code != 0 {
			status = code
			if r.ctl == controlExit {
				return status
			}
			continue
		}
		if subscripted {
			// A parameter this shell has not got refuses by name, ahead of
			// every reading of the brackets — the subscript of a name that
			// holds nothing is read as arithmetic, and the complaint that
			// came back was about the *key's* value rather than about the
			// table. See refuseAbsentParameterUnset.
			if reason, absent := r.refuseAbsentParameterUnset(base); absent {
				r.diagf("%s: %s\n", base, reason)
				status = 1
				continue
			}
			// A name the shell has never heard of takes its brackets with
			// it: they are not read, so the arithmetic in them neither
			// fails nor increments anything. Read rather than asked, for
			// ArithSubscriptSkippedWhenNameUnset's reason — the readings
			// agree on every subscript that has no error and no side
			// effect, so asking here would refuse `unset nope[1]` over a
			// difference it cannot make.
			//
			// It carries no status either, which is the row that separates
			// this from "the operand succeeded": under
			// UnsetStatusIsTheLastSubscripts an earlier failure still
			// stands behind it.
			if r.sem().UnsetSubscriptSkippedWhenNameUnset == Yes && !r.nameIsSet(base) {
				continue
			}
			// **And the round is here, behind the name**, because it is an
			// expansion and an expansion in a subscript can run a command.
			// A script passing text it did not write into `unset` — `v='a[$(
			// … )]'; unset "$v"` — had that command run here where bash
			// never reaches it, which is the defect and not an edge (#4037).
			//
			// Measured 2026-09-21, bash 5.3.20 and 3.2.57 alike, with
			// `$(echo INJECTION! >&2 ; echo 0)` as the subscript's text:
			//
			//	unset "$v", no variable of that name   nothing runs, status 0
			//	unset "$v", `a` readonly               refused, nothing runs
			//	unset "$v", `a=(x y z)`                it runs, element gone
			//	read "$v" / printf -v "$v" / declare "$v=hi" / test -v "$v"
			//	                                       it runs, in both shells
			//
			// So it is the **route** that decides and not the text, and this
			// route's answer is "only once the name is there". The two
			// refusals above — an absent parameter and a frozen one — are
			// measured ahead of it in the same sweep and were already
			// written ahead of it here; what stood in the wrong place was
			// this round alone, which ran before either of them could
			// answer. Semantics.UnsetExpandsAFlatSubscript is whether this
			// shell rounds and Runner.ExpandsAnOperandsSubscriptAgain
			// whether the session still permits it — one of the four
			// surfaces `shopt -s assoc_expand_once` names (#3298).
			//
			// The whole-array reading is about the **token the operand
			// carried**, so it is read here, in front of the round. `@` and
			// `*` name every element when they are what the subscript says;
			// an `@` a round *produces* is an operand the arithmetic reader
			// has no value for, and the array is left standing. Measured
			// 2026-09-21 from a directory holding three files, bash 5.3.20
			// and 3.2.57 alike, over `a=(x y)`:
			//
			//	unset "a[@]"                       the array is emptied
			//	w='a[$(echo @)]'; unset "$w"       `@: operand expected`, 1
			//	k=@; w='a[$k]'; unset "$w"         the same sentence
			//	unset "a[*]" / the arrived `*`     the same pair of answers
			//
			// Both columns agree, so this is not a version's quirk; and the
			// two spellings of the arrival agree with each other, so it is
			// the *provenance* and not the substitution. Read off the text
			// rather than off a flag because that is what the round moves:
			// `sub` after it is the key, and this is the question about what
			// was written (#4070).
			everyElement := sub == "@" || sub == "*"
			if !lexed {
				// A subscript the parser read is not read again: it reached
				// `unset` as a word and was expanded once, which is the
				// measurement in subscriptOperandRead — `unset m[$b]` with
				// `b='x$y'` and a table holding both `x$y` and `xZZZ` takes
				// the literal key away in bash 5.3.20, where `unset "m[$b]"`
				// takes the expanded one. The axis is about the *text* shape
				// and the switch beside it is a script turning that round
				// off; a lexed subscript never reaches either.
				sub = r.operandSubscriptText(base, sub, r.sem().UnsetExpandsAFlatSubscript,
					"`unset` expanding a subscript that reached it as text")
				if r.unspecified {
					return r.status
				}
			}
			// `unset a[1]` is about one element and not about the array.
			// The subscript was read as part of the name, so the whole thing
			// was deleted from a table it was never in and nothing happened
			// at all.
			if r.assocDeclared(base) {
				// `[@]` is a key like any other where the attribute is on:
				// no shell measured clears a keyed array through it, so the
				// whole-array reading below is the indexed array's alone.
				r.unsetAssocElem(base, sub)
				continue
			}
			if everyElement {
				// Every element rather than one, in two of the three shells
				// with arrays. The third reads the brackets as an expression
				// here as everywhere else, and falls through to it.
				if handled, code := r.unsetWholeArray(base); handled {
					status = r.carryUnsetStatus(status, code)
					continue
				}
			}
			if handled, code := r.unsetSubscriptRange(base, sub); handled {
				// A subscript written as a pair, where the dialect reads the
				// comma as a range rather than as the arithmetic operator
				// whose value is its right operand. Ahead of the single
				// reading rather than inside it, because a range names a
				// span and a span is not a subscript.
				status = r.carryUnsetStatus(status, code)
				if r.ctl == controlExit {
					return status
				}
				continue
			}
			if r.unspecified {
				return r.status
			}
			if handled, code := r.unsetFlaggedSubscript(base, name, sub); handled {
				// A subscript flag group — `unset 'a[(r)y]'` — which names
				// its element by searching rather than by counting. Ahead of
				// the arithmetic below, which is what the whole operand went
				// to before and what made it `bad math expression`.
				status = r.carryUnsetStatus(status, code)
				if r.ctl == controlExit {
					return status
				}
				continue
			}
			if handled, code := r.unsetEmptySubscript(base, sub); handled {
				// A subscript written with nothing in it, which is what
				// `unset "a[$i]"` is once a blank `$i` has gone in. Ahead of
				// the arithmetic below, which read it as the expression that
				// is zero in every dialect and so removed the array's
				// *first* element at status 0. See unsetEmptySubscript.
				status = r.carryUnsetStatus(status, code)
				if r.ctl == controlExit {
					return status
				}
				continue
			}
			idx, err := r.subscriptValueOfReference(sub)
			if err != nil {
				// Reported by every shell in the panel, and silent here: the
				// error came back and nothing read it, so `unset a[b c]` was
				// a no-op at status 0. What follows differs — bash gives up
				// on the script and the other two leave a failed builtin
				// behind — but nobody says nothing.
				status = r.badSubscriptToUnset(sub, err)
				if r.operandGaveUpTheBuiltin() {
					return status
				}
				continue
			}
			status = r.carryUnsetStatus(status, r.unsetArrayElem(base, idx, sub))
			continue
		}
		if handled, code := r.unsetTheFunctionInstead(name, letterV); handled {
			// The name holds no variable and does hold a function, and this
			// shell reads that as a function's removal rather than as
			// nothing at all. Behind the readonly *variable* check above,
			// which is measured: a frozen `f3=rv` beside a function `f3`
			// refuses at 1 and leaves the function standing, so the freeze
			// on the parameter is answered before the function table is
			// reached.
			status = r.carryUnsetStatus(status, code)
			if r.ctl == controlExit {
				return status
			}
			continue
		}
		if r.unspecified {
			return r.status
		}
		if code := r.unsetName(name); code != 0 {
			status = r.carryUnsetStatus(status, code)
		}
	}
	if ended {
		// The names that were names are removed first and the script stops
		// after them: measured, a fatal `unset ":" ok1 ok2` leaves neither
		// ok1 nor ok2 standing in the shell that ends the script over it.
		return r.endAfterABadName(status)
	}
	return status
}

// unsetName removes one whole parameter, whatever kind it is, and reports the
// status the operand leaves behind.
//
// Extracted so that the pattern form and the name form remove alike: the two
// entered the builtin by different doors and would otherwise have been two
// copies of this, which is how one of them ends up forgetting a table.
//
// The status is 0 for every removal and 1 for the one refusal this makes —
// an unaimed reference, below. A `return` rather than a field for the reason
// the loops above keep their own: the builtin's status is the operands' and
// not the runner's, and a removal that happened must not overwrite a refusal
// that came before it.
func (r *Runner) unsetName(name string) int {
	// An `unset` **through** a reference with nothing to point at, which is
	// the one route of the three that does not end the script: the sentence
	// is written, the status is 1, and the next line runs. Measured — and
	// the plain `unset u` says it too, where the plain `u=5` and `${u}` part
	// company over whether the reference is being aimed. Ahead of the walk,
	// which answers nothing for an unaimed reference.
	// See Diagnostics.NamerefUnaimedUse.
	if aimless, unaimed := r.unaimedReferenceBase(name); unaimed {
		r.refuseUnaimedReference(aimless, r.builtinComplaintName("unset"))
		return 1
	}
	if r.selfNameref(name) {
		// A plain `unset` of a reference aimed at its own name says the same
		// sentence the read says, **twice**, and then removes the binding it
		// was standing in front of rather than the outer cell it reads.
		// Measured 2026-09-15 on bash 5.3.20 and stable across every shape
		// it was asked in: with an outer value and without one, under `-g`
		// and without it, and on a second `unset` of the same name — the
		// plain spelling leaves the reference aimed, so it warns again.
		// `unset -n` is the other door and is silent there.
		r.warnAboutACycle(name)
		r.warnAboutACycle(name)
	}
	name = r.throughNameref(name)
	// And a member path whose base is a reference, which that walk cannot see
	// — it is keyed on whole names and `c.a` is not one of them. Measured:
	// `typeset zz=(a=1 b=2); typeset -n c=zz; unset c.a` leaves `${zz.a}`
	// empty and `${zz.b}` as 2 in ksh93u+, where this removed a name called
	// `c.a` that nothing held. See Runner.compoundMemberThroughAReference.
	name = r.compoundMemberThroughAReference(name)
	// The `.unset` discipline runs *before* the name goes, which is measured
	// rather than convenient: `u=here; function u.unset { echo "$u"; };
	// unset u` prints `here` there, so the hook reads the value it is about
	// to lose. A name nothing has set fires nothing at all — the event is
	// about a variable going away, and one that was never there does not go.
	// See interp/discipline.go.
	//
	// Behind the map check, because the existence test is a *read*: it runs
	// a produced parameter's producer, and `unset RANDOM` asking the
	// generator for a number it then throws away is a side effect nobody
	// asked for. Nothing here has a discipline until something defines one.
	if r.disciplined != nil {
		// The tables as well as the scalar, which is what declaredNameHolds
		// adds over the store read this used to make: measured 2026-09-18,
		// `typeset -A m=([k]=1); function m.unset { … }; unset m` runs the
		// hook in ksh93u+ and ran nothing here, because a table keeps
		// nothing in Vars for a scalar read to find.
		if r.declaredNameHolds(name) {
			r.disciplineUnsetName(name)
			// And the hooks go with the variable, which is what the same
			// guard says twice: a binding made to a variable is broken when
			// the variable is removed, and a name nothing has set has no
			// variable for either half of this to be about. See
			// Runner.unsetDiscardsTheDisciplines (#3162).
			r.unsetDiscardsTheDisciplines(name)
		}
	}
	if name == "PATH" {
		// The same rule as an assignment to it, and for the same reason: a
		// search nobody can repeat is not an answer worth keeping. Measured,
		// `ls >/dev/null; unset PATH; hash` reports an empty table in bash.
		r.forgetEveryHashedCommand()
	}
	// A name a *calling* function made local is where the panel splits, and
	// in one column the name does not go away at all: the binding does, and
	// what it displaced answers from here on. See
	// interp/unsetenclosinglocal.go.
	//
	// After the hashed-command line above, because the search path really
	// does move when a local `PATH` is taken away, and ahead of everything
	// below, because none of it is what happens here: the name is not
	// removed from a table, its attributes are not cleared, and the
	// hidden-name parameter is *restored* rather than unset — the undo does
	// that itself, through the same restore a call's exit runs.
	if r.unsetTakesAnEnclosingLocal(name) {
		return 0
	}
	// And the parameter whose patterns take names out of a pathname
	// expansion stops where it is unset — taking the hidden-name switch back
	// off with it, whoever turned that on. See interp/ignorednames.go.
	r.ignoredNamesUnset(name)
	if t, tied := r.tieOf(name); tied {
		// Half a tie is not a state this shell has: `unset SCA` leaves `sca`
		// with no elements *and* unset, and `unset sca` leaves `$SCA` unset.
		//
		// **Whether the tie survives is the difference between the two kinds
		// of tie**, and it was one answer for both until #1631. A tie a
		// script made with the letter is forgotten — measured, `typeset -T
		// SCA sca; SCA=a:b; unset SCA; SCA=c:d` leaves `sca` with no
		// elements — where a pair the *shell* installed is its own and comes
		// back: `unset PATH; PATH=/y` splits into `path` again, and `unset
		// path; path=(/q)` writes `PATH` again from the other side. Both
		// names still go away in both cases, which is what `${+path}` being
		// 0 after `unset PATH` says; it is only the pairing that is kept.
		//
		// The two names are removed here rather than by recursing, because
		// the recursion was standing on the untie: with the tie left in
		// place the second call would find it and come straight back.
		if !t.special {
			r.untie(name)
		}
		other := t.scalar
		if name == t.scalar {
			other = t.array
		}
		r.unsetOneName(other)
	}
	// Read before the removal, which is what clears the record: it says
	// whether an earlier `unset` in this call had already left the name
	// declared, and the placeholder's one letter turns on it.
	again := r.unsetLeftItDeclared[name]
	r.unsetOneName(name)
	// And a local of the scope that is *running* is left declared where the
	// column says so — the value and the letters go, the shadow stays, and
	// the name is still a row in a listing. After the removal, because the
	// removal is what clears the record. See
	// interp/unsetenclosinglocal.go.
	r.unsetLeavesARunningScopesLocalDeclared(name, again)
	// And the message that the name has gone, for a dialect keeping state
	// beside it that the name's removal is about. After the removal, so that
	// an action reading the name back sees it gone, and for a name nothing
	// had set, because the state this is about is not the variable. See
	// SetUnsetAction.
	if act, ok := r.unsetActions[name]; ok {
		act(r)
	}
	return 0
}

// unsetOneName is unsetName for a name whose tie, if it had one, has already
// been dealt with by the caller.
//
// Split out rather than reached by recursion so that a tie which *survives*
// the unset cannot send the removal round again — see unsetName.
func (r *Runner) unsetOneName(name string) {
	delete(r.Vars, name)
	// Recorded off rather than deleted, which the tri-state is there for:
	// deleting the record puts the question back to the environment, and the
	// environment still names an inherited name — so `unset IMPORTED;
	// IMPORTED=second` handed the child a name the script had unset. The
	// removal record used to stand in for this, and stopped once an
	// assignment began lifting it. See isExported and nameIsBack.
	if r.exported == nil {
		r.exported = map[string]bool{}
	}
	r.exported[name] = false
	if write, produced := r.dynamicArrayWriters[name]; produced {
		// An `unset` of a produced array is a write of no elements, which is
		// the same message the shell being modeled sends: measured on
		// zsh 5.9.2, `set -- a b c; unset argv` leaves `$#` at 0, exactly as
		// `argv=()` does. Delivered rather than recorded, because the
		// removal below only hides a stored array and the producer would go
		// on answering every read with what it was never told to drop.
		write(r, nil)
	}
	delete(r.Arrays, name)
	delete(r.AssocArrays, name)
	// A compound an element of either table held goes with the table, for the
	// reason the parent's own members go with it below: they are names of
	// their own and nothing else would take them. Measured,
	// `a[1]=(p=1); unset a; ${a[1].p}` is empty in the shell. After the
	// delete, so the sweep finds no element claiming any of them. See
	// interp/subcompound.go.
	r.sweepElementCompounds(name)
	// The table is gone, so the note about how it came to be is meaningless
	// and a later declaration of the name starts the record over. Measured:
	// `declare -A m=([a]=b); unset m; declare -A m` lists `declare -A m`, the
	// declared-only shape, and not the emptied one. See
	// compounddeclaredonly.go.
	delete(r.declaredOnlyCompound, name)
	delete(r.compoundHeldAnElement, name)
	// A compound's members are names of their own, so taking the parent away
	// has to take them with it: `c=(a=1); unset c; ${c.a}` is empty in the
	// shell, and leaving the member behind read back the value the shell had
	// just been told to forget. Before the mark is dropped, because the
	// members are found through it.
	if r.compoundVariable[name] {
		delete(r.compoundVariable, name)
		r.unsetCompoundMembers(name)
	}
	// A namespace is the same shape one level up: the members are names
	// under it, so `unset .ns` has to take them too. The namespace itself
	// stops existing, which is what makes a later `${.ns.gv}` an ordinary
	// unset dotted name rather than a read-through to the global. See
	// interp/namespace.go.
	if ns, ok := r.namespaceUnsetTarget(name); ok {
		delete(r.namespaces, ns)
		for _, m := range r.compoundDescendants(name) {
			r.unsetOneName(m)
		}
	}
	r.clearAttributes(name)
	// Recorded as well as deleted: a name that came from the environment is
	// not in Vars to begin with, and deleting nothing left it visible to
	// every lookup — `unset PATH` did not clear PATH.
	if r.removed == nil {
		r.removed = map[string]bool{}
	}
	r.removed[name] = true
}

// clearAttributes takes a name's attributes away, which is the other half of
// what `unset` does and the half that was missing.
//
// Unanimous across every shell in the panel that spells the letters at all,
// which is what makes it a rule and not an axis: `typeset -i n=5; unset n;
// n=3+4` reads back the four characters in bash 5.3.15, ksh93u+ and zsh
// 5.9.2, and so does every other letter — `-u`, `-l`, `-U` and `-A` all stop
// applying, and `-x` stops reaching a child. The name went, so what it was
// declared to be went with it, and a name assigned afterwards is a plain new
// name.
//
// One place rather than one per letter. The maps are keyed by name and
// nothing on the unset path deleted from them, so every attribute survived
// its own name: `unset PATH; PATH=a:a:b` under a `-U` name silently lost the
// duplicate, and `unset n; n=3+4` under an `-i` name stored 7.
//
// Not reached for a readonly name, which refuses the `unset` outright before
// this — measured, `typeset -r r=1; unset r` is status 1 in bash, ksh93 and
// zsh alike — and not reached for an element or a span, which are not the
// name. Both of those are `unsetName`'s callers' doing rather than this
// function's; see biUnset, where the refusal stands ahead of the subscript
// and the subscripted spellings never come here.
//
// The export attribute, both arrays and any tie are cleared by unsetName
// itself: the tie is untied first because half a tie is not a state this
// shell has, and the associative attribute *is* the table, so deleting the
// table is what takes the attribute off.
func (r *Runner) clearAttributes(name string) {
	if r.isWindowSizeParameter(name) {
		// Except for a name the *shell* is, whose attributes are not a
		// script's to lose. Measured on zsh 5.9.2: `unset COLUMNS` leaves
		// `${(t)COLUMNS}` empty, as it does for any name that is gone, and a
		// later `COLUMNS="3+4"` is **7** at `integer-special` — so the
		// integer letter and its base came back with the parameter rather
		// than having been removed with it. Dropping them made the same line
		// store the three characters `3+4` as a scalar.
		return
	}
	// The list itself is shared with the shadow, which takes the same
	// attributes off for a reason of its own — see localattributes.go.
	r.dropNameAttributes(name)
	// **The hide-in-scope letter is not in that list**, and it used to be
	// taken off here. Measured 2026-09-12 on zsh 5.9.2, `-f`, `env -i` with
	// `PATH=/bin`, where a `local` of a hidden name is the only thing that
	// observes the letter at all:
	//
	//	f(){ local PATH=/c; print -r ${(j:,:)path}; }
	//
	//	f                                      /c    the control
	//	typeset -h PATH; f                     /bin  detached, so the local
	//	                                             does not drive `path`
	//	unset PATH; PATH=/y; f                 /y    still detached
	//	typeset +h PATH; f                     /w    and the plus form is
	//	                                             what takes it off
	//
	// The third row is this one, and it was previously recorded the other
	// way round. That reading could not have been taken from the shell: the
	// letter shows only over one of the shell's own ties, and this engine's
	// `unset` forgot the tie too — so the name that came back was untied
	// whatever the letter said, and both answers looked alike. Fixing the
	// tie (#1631) is what made the row measurable, and it disagreed.
	// Runner.declaredEmpty is deliberately *not* cleared here. It looked like
	// one of these and is not: nothing can observe it for a removed name.
	// hiddenExports is its only reader and it walks the exported set, which
	// `unset` has just written off; an assignment clears the flag before it
	// stores; and a second `typeset -x` on the name re-declares it empty
	// anyway. Measured for both routes — `typeset -x d; unset d; export d`
	// and `typeset -x g; unset g; typeset -x g` — and no shell in the panel
	// tells a child anything either way. A line here would be one no mutant
	// could kill.
}

// biExport marks a name for the environment, and assigns when given a value.
func biExport(r *Runner, _ context.Context, args []string) int {
	// `-f` and `-n` are offered only where the dialect has them. Where it
	// does not, each goes through the ordinary unknown-option path and gets
	// that shell's own refusal, which in two of them ends the script.
	letters := "p"
	// Asked only where there is an `-n` to decide about, for the reason `-f`
	// is below: `export A=1` is the same in all four, and refusing it over a
	// question nothing turned on would be refusing to export anything.
	if hasOption(args, 'n') {
		takes := r.ask(r.sem().ExportTakesTheAttributeOff, "`export -n`")
		if r.unspecified {
			return 2
		}
		if takes {
			letters += "n"
		}
	}
	if hasOption(args, 'f') {
		carries := r.ask(r.sem().ExportCarriesFunctions, "`export -f`")
		if r.unspecified {
			return 2
		}
		switch {
		case carries:
			letters += "f"
		case r.diag().ExportFunctionOptionRefused != "":
			// A dialect that knows the letter and will not do it, which is
			// not the same as one that has never heard of it — and says so
			// in different words.
			r.diagf("%s\n", r.diag().ExportFunctionOptionRefused)
			return 1
		}
	}
	if code, answered := r.namesUnderAPlus(args,
		func(d declaration) bool { return d.exported }); answered {
		return code
	}
	if code, took := r.exportAsADeclaration(args, letters); took {
		return code
	}
	if r.unspecified {
		return r.status
	}
	args, opts, code := r.builtinOptions("export", args, letters)
	if code != 0 {
		return code
	}
	if r.exported == nil {
		r.exported = map[string]bool{}
	}
	if strings.ContainsRune(opts, 'f') {
		if len(args) == 0 {
			// No name is the listing rather than nothing to do: measured
			// 2026-09-16 on bash 5.3.20, `export -f` and `export -pf` each
			// write the exported functions out, bodies and attribute line,
			// byte for byte what `declare -fx` writes (#3192).
			return r.attributedFunctionListing(functionAttributeExported)
		}
		return r.exportFuncs(args)
	}
	if strings.ContainsRune(opts, 'p') || (opts == "" && len(args) == 0) {
		// The listing: exported names alone, in the dialect's shape. An
		// operand narrows it to that name's declaration.
		//
		// `export` with nothing at all is the same listing. POSIX says so and
		// every shell in the panel does it — measured with one name exported
		// under a scrubbed environment, dash, bash 5.3, bash 3.2, ksh93 and
		// zsh all write the exported names out. This printed nothing, which
		// is why its failed write had nothing to fail: `export >&-` answered
		// 0 in silence where dash and bash report the write.
		//
		// The *shape* of the bare listing is not always `-p`'s: dash and both
		// bash builds write the same thing either way, and ksh93 and zsh drop
		// the leading `export` word for the bare form alone. That split is
		// recorded in the corpus and left for a dialect to answer; the
		// listing every shell has is worth more than the silence it replaces.
		//
		// And an operand beside the letter is its own question, which this
		// spelling used to answer by listing whatever was written — so
		// `export -p w=8` listed a name called `w=8` and stored nothing,
		// where bash and ksh93 export `w` without a word. See
		// interp/exportprintoperand.go; a false second result is the reading
		// that the letter is inert, and the declaration loop below runs.
		names, lists := r.exportPrintWithOperands(args)
		if r.unspecified {
			return r.status
		}
		if lists {
			form, dashP := r.bareOrDashP(opts, r.sem().ExportListing)
			return r.declarePrintForm(names, form, dashP,
				func(d declaration) bool { return d.exported })
		}
	}
	args, status, ended := r.builtinNames("export", args, false)
	if r.unspecified {
		return status
	}
	for _, a := range args {
		name, value, hasValue, appends := declarationOperand(a)
		if r.exportRefusesACompound(name, declareFlags{
			export: true,
			remove: strings.ContainsRune(opts, 'n'),
		}) {
			// The same refusal `typeset -x` meets, at the other spelling.
			// This loop is where `export` declares when no letter sent it
			// through declareNames, so the check has to stand in both — and
			// it is one function so the two cannot come to disagree about
			// which values are exportable.
			return r.status
		}
		if base, subs, subscripted := r.operandSubscripts("export", name); subscripted {
			if hasValue {
				// `export a[1]=v` in the two dialects that take the operand:
				// measured, ksh93u+ and zsh 5.9.2 both write the element, and
				// neither puts the array in the environment. See
				// declareelement.go.
				r.declareElement(base, subs[:len(subs)-1], subs[len(subs)-1], value, appends, declareFlags{}, false)
				if r.unspecified || r.operandGaveUpTheBuiltin() {
					return r.status
				}
				r.exportTheArrayOfAnElement(base, !strings.ContainsRune(opts, 'n'))
				if r.unspecified {
					return r.status
				}
				continue
			}
			// And with no value the brackets are still the dialect's to
			// read, which is the whole of #3501 at this spelling — and the
			// name to export is the *base*, which this spelling did not do
			// either: `export 'a[1]'` exported a variable literally named
			// `a[1]`, which no environment can carry, where ksh93 exports
			// `a` (#1380 at the fourth spelling).
			// No export letter in the element declaration's own flags: the
			// letter lands on the *array* or nowhere, and which of those it
			// is comes from the one call below, so a dialect cannot record
			// it by two routes and answer them differently (#3510).
			n, _, done := r.valuelessSubscriptedOperand(base, subs, declareFlags{}, false)
			if done {
				if r.unspecified || r.operandGaveUpTheBuiltin() {
					return r.status
				}
				r.exportTheArrayOfAnElement(base, !strings.ContainsRune(opts, 'n'))
				if r.unspecified {
					return r.status
				}
				continue
			}
			if r.unspecified {
				return r.status
			}
			name = n
		}
		// The export attribute goes to what a reference points at, and this
		// loop takes no scope, so there is never a fresh binding for it to be
		// about instead. See interp/namerefattribute.go.
		//
		// A reference aimed at an **element** parts the two: the value is the
		// one cell's and the attribute is the array's or nowhere at all, which
		// is the axis nameOperandThroughAReference asks. `valueTarget` is the
		// reference itself where it is not "", because setVarAs already
		// resolves one to the element and storing the target's text would make
		// a parameter literally called `a[1]`.
		valueTarget, letters := r.nameOperandThroughAReference("export", name, declareFlags{}, !strings.ContainsRune(opts, 'n'))
		if r.unspecified {
			return r.status
		}
		if target, follows := r.attributeFollowsTheReference(name, declareFlags{}); follows {
			name = target
		}
		if hasValue {
			// `export` is a declaration, so its plain word meets the same
			// refusal `typeset`'s does where the name is really holding an
			// array. No scope is ever taken here, so the cell is never a
			// fresh one. See Runner.inconsistentTypeRefused.
			if r.inconsistentTypeRefused(name, false, declareFlags{}) {
				return r.status
			}
			if r.unspecified {
				return r.status
			}
			// `export a=1; export a+=2` is `a=12`. See declarationAppend.
			// No shadow is taken here either, so the value it joins is the
			// one the name already reads even inside a function.
			if appends {
				if !r.declarationAppend(r.orName(valueTarget, name), value, false, false) {
					return r.status
				}
			} else {
				r.setVarAs(r.orName(valueTarget, name), value, assignedByDeclaration)
			}
			if r.ctl == controlExit {
				// The assignment ended the script, so the builtin has
				// nothing left to report — and returning its own status
				// would put back the one the failure set.
				return r.status
			}
		}
		if !letters {
			// The column that refuses the attribute has already said so, and
			// the value above is the whole of what it does: measured, `export
			// b=Z` over a reference to `a[1]` leaves `p Z r` with no export
			// letter on `a` and nothing declared under either name, at 0. So
			// none of the declaration below runs.
			continue
		}
		if hasValue {
			// `export` is an attribute word, so naming a name a previous
			// declaration left with no value of its own gives it the empty in
			// its own right — see declarationOwnsTheStandingEmpty.
			r.declarationOwnsTheStandingEmpty(name)
		} else {
			// And with no value it is a declaration like any other, so what
			// a bare `export v` leaves behind is
			// DeclaredNameWithoutValueIsEmpty's — the same question `typeset
			// v` asks under a word that also carries an attribute. This
			// builtin never takes a scope, so the cell is never a fresh one
			// and the inherit answer never arrives; declareEmpty does the
			// standing-empty half itself, which is why it replaces the call
			// above rather than following it (#3345).
			r.declareEmpty(name, false, true, true, false, true)
			if r.unspecified || r.ctl == controlExit {
				return r.status
			}
		}
		// Recorded either way rather than deleted for `-n`: a name that came
		// in through the environment is exported by having done so, and only
		// an explicit "no" can take that off. Deleting the record put the
		// question back to the environment, which answers yes.
		r.declarationExports(name, !strings.ContainsRune(opts, 'n'))
	}
	if ended {
		// See biDeclare: the names are exported and then the script stops.
		return r.endAfterABadName(status)
	}
	if r.assignFailed && status == 0 {
		// A name it refused to assign is the builtin's failure, not just a
		// remark: the dialect that reports a readonly reassignment and
		// carries on leaves 1 in `$?` for `export x=2` as much as for `x=2`.
		return 1
	}
	return status
}

// Registered here rather than in the table above: `shift` reads its count as
// an expression in two dialects, which reaches the evaluator, which reaches
// the runner, which reaches the table — a cycle Go refuses to order. Two
// other builtins are registered this way for the same kind of reason.
//
// The four declaration builtins joined them when a subscripted operand
// started naming an element: a subscript is an expression, so reading one
// reaches the evaluator by exactly the route `shift` does.
func init() {
	builtins["shift"] = biShift
	builtins["export"] = biExport
	builtins["local"] = biLocal
	builtins["typeset"] = biDeclare
	builtins["readonly"] = biReadonly
}

// firstOptionLetter is the letter a bundle of single-letter options is
// refused by: the dashes are stripped and the first rune after them taken, so
// `--help` is `h`.
func firstOptionLetter(operand string) string {
	letters := strings.TrimLeft(operand, "-")
	if letters == "" {
		return operand
	}
	return string([]rune(letters)[0])
}

// biShift drops the first n positional parameters.
//
// Shifting past the end is where the panel splits: fatal in dash and ksh93,
// survivable in bash and zsh. The dialect answers it rather than this taking
// a side.
// subscriptOperand splits `a[1]` into the name and the subscript.
//
// Only for a name that really ends in one: `unset a` is the whole array and
// `unset a[1]` is an element of it, and the two arrive as the same kind of
// word. The subscript comes back as text because its reading is the base
// name's to decide: an expression for an indexed array, any string at all for
// a declared associative one — so `unset m[k]` is an element of `m` when `m`
// carries the attribute and an element numbered by whatever `k` evaluates to
// when it does not.
//
// The split is where it stops. It used to reject anything that was not a
// numeral, which made `unset a[i+1]` a bad *name* rather than a bad
// subscript — a whole different complaint about a subscript that is fine.
// Whether the text evaluates is the caller's question, asked where the
// element is reached and not here, which is also what keeps a builtin out of
// the arithmetic evaluator's reach until it means to use it.
// The brackets have to *balance*, though, and that is here rather than at a
// caller because it decides whether there is a subscript at all. Measured
// 2026-09-12 from a script file, `typeset 'm[a]b]'=v` is refused by the name
// check in all three shells that reach it — `not a valid identifier` in bash
// 5.3, `invalid variable name` in ksh93u+, `not an identifier` in zsh 5.9.2 —
// and so is `typeset 'n[x[y]'=v`. Without the check the text between the
// first `[` and the last `]` became the subscript, so `m[a]b]=v` quietly
// placed a key literally spelled `a]b`.
//
// `a[1][2]=v` is the same shape and the same refusal in bash 5.3 and zsh, and
// **ksh93 is a fourth answer this does not implement**: it builds a compound
// inside the element, `typeset -a a=([1]=([2]=v) )`, which needs compound
// variables this engine does not have (#2491). bash 3.2 is a fifth — an empty
// array under the base name at status 0. So the refusal here is right in two
// of the four columns that reach it and is a loud complaint rather than a
// silent wrong answer in the third; what it replaced was `1][2: arithmetic
// syntax error`, which is nobody's (#1380).
func (r *Runner) subscriptOperand(operand string) (string, string, bool) {
	return r.subscriptOperandRead(operand, false)
}

// subscriptOperandRead is subscriptOperand with the one thing a caller may
// answer differently: whether the *parser* read these brackets, which decides
// what an unterminated quote between them is. See operandBracketsBalance.
//
// One caller passes true — `unset`, and only for an operand this command
// wrote with its brackets outside every quoting construct. Measured
// 2026-09-19 from a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`,
// bash 5.3.20, a table holding one element under the key `x'y`:
//
//	unset m[$b]      b="x'y"   the element is gone
//	unset m["x'y"]             gone
//	unset m[x\'y]              gone
//	unset "m[$b]"              still there, silent at 0
//	o="m[$b]"; unset $o        still there
//	o="m[$b]"; unset "$o"      still there
//
// The line runs through the brackets and not through the key: the three that
// remove it wrote `[` and `]` unquoted, and the three that leave it handed
// the builtin a string. Every other operand builtin refuses the key on
// **both** routes — `printf -v c[$b] P`, `read c[$b]`, `typeset c[$b]=W` and
// their quoted spellings are all “not a valid identifier“ there — so this
// is `unset`'s row and not a rule about operands. The control is a key with
// nothing to quote, `unset m[plain]`, which removes the element on every one
// of the six spellings.
func (r *Runner) subscriptOperandRead(operand string, lexed bool) (string, string, bool) {
	open := strings.IndexByte(operand, '[')
	if open <= 0 || !r.operandBracketsBalance(operand[open:], lexed) {
		return "", "", false
	}
	base := operand[:open]
	sub := subscriptOperandText(r.operandSubscriptUnquoted(operand[open+1 : len(operand)-1]))
	return base, r.operandSubscriptTilde(base, sub), true
}

// subscriptOperandText is the space-trimming an operand's subscript gets, and
// it stops short of turning a **blank** subscript into an **empty** one.
//
// Trimming outright collapsed `a[ ]` into `a[]`, which the panel distinguishes
// at every operand route: measured 2026-09-17, a script file, `a=(1 2 3)`,
//
//	                  a[]                        a[ ]
//	bash 5.3.20       unset removes nothing      unset removes element 0
//	                  typeset writes nothing     typeset writes element 0
//	zsh 5.9.2         invalid subscript          operand expected at end
//	ksh93u+           element 0 either way       element 0 either way
//
// — which is Semantics.EmptyArithSubscript on the left and
// Semantics.BlankArithSubscriptIsTheEmptyExpression on the right, two axes
// whose whole reason for being apart is that one shell answers them
// differently (see their doc comments). With the text collapsed, only the
// first was ever reachable through an operand, and it was reached for both.
func subscriptOperandText(sub string) string {
	if trimmed := strings.TrimSpace(sub); trimmed != "" || sub == "" {
		return trimmed
	}
	// All whitespace and not nothing: left as one space, which is what the
	// blank axis is about and is the shortest text that cannot be mistaken
	// for the empty one.
	return " "
}

// operandSubscriptTilde is Semantics.SubscriptKeyExpandsALeadingTilde reached
// from the other side: the subscript a *builtin's operand* carries as text.
//
// Here rather than at each keyed branch, because there are six of them —
// `unset`, `read` and the other stores, `test -v`, `printf -v`, `${!ref}` and
// a descriptor assignment — and a shell that expanded the tilde for some of
// them would answer a script differently depending on which route reached the
// element. Measured 2026-09-17 on bash 5.3.20, with the element stored under
// `$HOME/k` or read back afterwards, every route taking the tilde:
//
//	unset 'm[~/k]'         removes it
//	test -v 'm[~/k]'       true
//	printf -v 'm[~/z]' …   stores under $HOME/z
//	read 'm[~/k]'          stores under $HOME/k
//	r='m[~/k]'; ${!r}      reads it
//
// **Only for a table already keyed**, which is the same guard the word route
// has and is what keeps the arithmetic subscript out of it: `declare -a q;
// unset 'q[~/k]'` in bash complains about `~/k` and not about the path, so
// the indexed operand is left exactly as written.
//
// The text arrives with its quotes still in it, so the tilde is taken only
// where the operand opens with one — `unset "m[\"~/k\"]"` is a key that
// starts with a quote and no tilde expansion reaches it, in bash as here.
func (r *Runner) operandSubscriptTilde(base, sub string) string {
	if !strings.HasPrefix(sub, "~") || !isPlainName(base) {
		return sub
	}
	if !r.assocDeclared(r.throughNameref(base)) {
		return sub
	}
	if !r.ask(r.sem().SubscriptKeyExpandsALeadingTilde,
		"a subscript's leading tilde expanding to the home directory") {
		return sub
	}
	return r.tildeValue(sub)
}

// operandSubscripts is subscriptOperand for the callers that can take a
// *chain* — `a[1][2]=v` as a declaration's operand, where the second
// subscript reaches into what the first named.
//
// One subscript is the common answer and comes back as a one-element slice,
// so a caller reads the last one the same way whether or not there was a
// chain. A dialect without the grammar never sees more than one: the reader
// below stops at the first `]` there, which is what leaves `a[1][2]=v` the
// unbalanced operand every other column refuses (#2491).
func (r *Runner) operandSubscripts(builtin, operand string) (string, []string, bool) {
	if !r.dialect().ChainedAssignSubscript || !chainedOperandBuiltin(builtin) {
		// `unset` reads a lexed operand's brackets here exactly as its body
		// does a few lines on. Two readings of one operand is what made the
		// name check call `unset m[80's]` a plain name and the body call it
		// an element — see subscriptOperandRead, and note that this is the
		// shape CLEANROOM's "a second helper spreads the bug" note is about.
		lexed := builtin == "unset" && r.operandBracketsWereLexed(operand)
		base, sub, ok := r.subscriptOperandRead(operand, lexed)
		if !ok {
			return "", nil, false
		}
		return base, []string{sub}, true
	}
	open := strings.IndexByte(operand, '[')
	if open <= 0 {
		return "", nil, false
	}
	base, rest := operand[:open], operand[open:]
	var subs []string
	for rest != "" {
		if rest[0] != '[' {
			return "", nil, false
		}
		end := subscriptCloses(rest)
		if end < 0 {
			return "", nil, false
		}
		subs = append(subs, strings.TrimSpace(rest[1:end]))
		rest = rest[end+1:]
	}
	return base, subs, true
}

// chainedOperandBuiltin reports whether a chain of subscripts is a shape this
// builtin's operand may take.
//
// The declaration utilities, and not `unset`. Measured 2026-09-14 on ksh93u+:
// `export a[1][2]=v` and `readonly a[1][2]=v` each write the nested element
// and list it back, where `unset a[1][2]` is `unset: a[1][2]: cannot be an
// array` at 1 — so the chain is a thing a *write* may name and not a thing an
// operand may be. Reading one for `unset` would take the last subscript for
// the whole name and remove an element nobody named, at status 0.
func chainedOperandBuiltin(builtin string) bool {
	switch builtin {
	case "typeset", "declare", "integer", "local", "export", "readonly", "float", "nameref":
		return true
	}
	return false
}

// subscriptCloses is where the bracket run starting at text[0] closes,
// counting nested brackets on the way, or -1 where it never does.
//
// The same count subscriptBracketsBalance makes, and for the same reason: it
// is what tells `a[i[0]]` — a subscript reading another element, which is a
// legal expression — from `a[1][2]`, where the first bracket has closed and a
// second one follows.
func subscriptCloses(text string) int {
	depth := 0
	for i := range len(text) {
		switch text[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// subscriptBracketsBalance reports whether text is one `[…]` and nothing
// after it, counting nested brackets on the way.
//
// The count is what tells `a[i[0]]` — a subscript reading another element,
// which is a legal expression — from `a[1][2]`, where the first bracket has
// closed and a second one follows. Both hold a `[` inside and both end in
// `]`, so neither a search for the last `]` nor one for a second `[` can
// separate them.
func subscriptBracketsBalance(text string) bool {
	depth := 0
	for i := range len(text) {
		switch text[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i == len(text)-1
			}
			if depth < 0 {
				return false
			}
		}
	}
	return false
}

// hasOption reports whether the letter appears in the option words before the
// first operand.
func hasOption(args []string, letter byte) bool {
	for _, a := range args {
		if a == "--" || !strings.HasPrefix(a, "-") || a == "-" {
			return false
		}
		if strings.IndexByte(a[1:], letter) >= 0 {
			return true
		}
	}
	return false
}

func biShift(r *Runner, _ context.Context, args []string) int {
	n := 1
	// What the complaint names differs from what the shift does: one dialect
	// reports the operand *as written*, and an operand that was never given
	// is reported as `(null)` rather than as the default it stood in for.
	operand := shiftAbsentOperand
	// Whether the marker was taken, which decides more than where the count
	// starts: past it a dash word is an operand rather than an option, so
	// ksh93 reads `shift -- -1` as a count below zero where it refuses a
	// bare `-1` as an option it does not have.
	marked := false
	if len(args) > 0 && args[0] == "--" {
		// The end-of-options marker, and only the first one: what follows is
		// the count however it is spelled, so `shift -- -1` is a negative
		// count in the three that take the marker and `shift -- --`
		// complains about the second `--` as a count. Asked here rather than
		// alongside the option words because bash answers the two
		// differently — no dash word is an option there and the marker still
		// works.
		switch {
		case r.ask(r.sem().ShiftDoubleDashEndsOptions, "`shift --` read as the end of options"):
			args = args[1:]
			marked = true
		case r.unspecified:
			return r.status
		}
	}
	// zsh's operands are the names of arrays to shift, with the count still
	// optional in front of them. A word that *names an array* is a name; any
	// other word is a count. See Semantics.ShiftNamesAreArrays.
	var names []string
	if len(args) > 0 {
		// The two readings part company in only two places: a first word
		// that names an array, which is a name to zsh and a count to
		// everyone else, and words behind the count, which zsh takes as
		// names and the others ignore. Everywhere else the answer cannot
		// change what happens, so asking there would refuse a program the
		// core can already run.
		named := false
		if r.shiftNamed(args[0]) {
			switch {
			case r.ask(r.sem().ShiftNamesAreArrays, "`shift` taking array names as operands"):
				named = true
			case r.unspecified:
				return r.status
			}
		}
		if named {
			// The first word is already a name, so there is no count word to
			// read and the default of one stands.
			names = args
		} else {
			operand = args[0]
			if st, done := r.shiftCount(args[0], marked, &n); done {
				return st
			}
			// Words behind a count that reads: names to zsh, ignored
			// everywhere else, and the difference shows even when none of
			// them is an array — the positional parameters are left alone
			// whenever any name is given. Asked only once the count itself
			// stands, because a count that does not read ends the builtin
			// before the rest of the line can matter.
			if len(args) > 1 {
				switch {
				case r.ask(r.sem().ShiftNamesAreArrays, "`shift` taking array names as operands"):
					names = args[1:]
				case r.unspecified:
					return r.status
				}
			}
			if len(names) == 0 {
				// Not names, so it is one operand too many — and the
				// column that refuses does so ahead of both ends of the
				// range below.
				if st, done := r.extraNumericOperands("shift", args); done {
					return st
				}
			}
		}
	}
	if n < 0 {
		// The other end of the range. Same fatality rule and the same
		// untouched `$#`; only the wording differs, and dash never arrives
		// here at all — a negative count is a word that is not a number
		// there, which shiftCount has already said.
		return r.shiftOutOfRange(r.diag().ShiftNegativeCount, n, operand)
	}
	if len(names) > 0 {
		// Named arrays replace the positional parameters outright rather
		// than shifting them too: measured, `set -- P Q; a=(1 2 3); shift a`
		// leaves `$@` as `P Q`.
		return r.shiftArrays(names, n, operand)
	}
	if n > len(r.Params) {
		return r.shiftOutOfRange(r.shiftTooManyWording(operand), n, operand)
	}
	r.Params = r.Params[n:]
	return 0
}

// shiftNamed reports whether a word names an array, which is what makes it an
// operand of `shift` rather than a count.
//
// Strictly an array: arrayElems answers a scalar as a one-element array and
// would make `shift v` on `v=hello` a name, where zsh reads it as a count of
// `hello` — which is zero, and leaves both `v` and `$@` alone.
func (r *Runner) shiftNamed(word string) bool {
	if r.assocDeclared(word) {
		// An association has no order to shift off the front of, and zsh
		// leaves one alone at status 0. Reading it as a count reaches the
		// same place — arithmetic on the name is zero — by the path the
		// other non-arrays take.
		return false
	}
	_, ok := r.arrayElemsOfTheName(word)
	return ok
}

// shiftArrays is the named form: each name's array loses its first n
// elements.
//
// A name that is not an array is left alone and not complained about, and a
// count past the end of one array does not stop the others — so the status is
// carried rather than returned. That is only sound where past-the-end is
// survivable, which ShiftNamesAreArrays says it may only be answered in.
func (r *Runner) shiftArrays(names []string, n int, operand string) int {
	status := 0
	for _, name := range names {
		elems, ok := r.arrayElemsOfTheName(name)
		if !ok || r.assocDeclared(name) {
			continue
		}
		if n > len(elems) {
			status = r.shiftOutOfRange(r.shiftTooManyWording(operand), n, operand)
			continue
		}
		r.setArray(name, elems[n:])
	}
	return status
}

// shiftAbsentOperand stands where a count word was never written, which is
// what one dialect's complaint names and what another's leaves out.
const shiftAbsentOperand = "(null)"

// shiftTooManyWording is the complaint for a count above `$#`, or the empty
// string where this shell has nothing to say about that end.
//
// Two things are decided here and they are not the same kind. Whether the
// dialect *has* a sentence is Diagnostics.ShiftTooMany, which is empty in the
// one column that never complains about this end at all. Whether a dialect
// that has one *writes* it is Runner.ReportsShiftPastTheEnd, which is the
// capability behind bash's `shift_verbose` — held by the shell rather than by
// the dialect, because a script moves it while it runs and a listing reads it
// back.
//
// The third thing is which of two sentences, and it is the dialect's again: a
// format naming the operand has no operand to name where none was written, so
// the shell that drops the slot says so with a second wording rather than
// with a placeholder. See Diagnostics.ShiftTooManyWithNoCount.
func (r *Runner) shiftTooManyWording(operand string) string {
	if !r.ReportsShiftPastTheEnd() {
		return ""
	}
	if operand == shiftAbsentOperand {
		if w := r.diag().ShiftTooManyWithNoCount; w != "" {
			return w
		}
	}
	return r.diag().ShiftTooMany
}

// shiftOutOfRange reports a count outside `0..$#`, in whichever direction.
//
// One rule for both ends: fatal in dash and ksh93, survivable in bash and
// zsh, `$#` untouched either way, and status 1 where it is survived. The
// wording is the caller's because the two directions are worded separately —
// bash is silent above `$#` and complains below it, and zsh has a sentence
// for each.
func (r *Runner) shiftOutOfRange(wording string, n int, operand string) int {
	if r.ask(r.sem().ShiftPastEndFatal, "shift past the end being fatal") {
		// controlReturn only unwound a function, so at the top level the
		// script carried on past an error the shell calls fatal.
		r.fatalUsage("%s\n", Wording(wording, "shift: can't shift that many", n, operand))
		return r.status
	}
	// Survivable, and still worth saying where the dialect says it: zsh
	// prints its complaint and carries on, and bash prints nothing at all
	// for a count above `$#`. No fallback here for that reason — an empty
	// wording is a dialect that has nothing to say rather than one that has
	// not been asked.
	if wording != "" {
		r.diagf("%s\n", Wording(wording, wording, n, operand))
	}
	return 1
}

// shiftCount reads the operand, reporting whether the builtin is finished.
//
// A leading `-` splits the panel three ways, which is ShiftOptionWords: ksh93
// reads every dash word as an *option* and refuses it as one, zsh reads only
// the ones that are not all digits that way, and bash and dash read them all
// as the count and complain about the number. Same input, two different kinds
// of complaint, and which input gets which differs again.
//
// Both refusals end the script in the two dialects where a special builtin's
// failure is fatal, and `shift` is a special builtin, so that rule is the one
// already in place rather than a new one.
//
// A lone `-` is not a dash word in any of the three readings and reaches the
// count, which is bash's and dash's answer for it. The marker `--` never gets
// here: biShift has taken it off already, or the dialect does not have one
// and the word is the count. marked says which — past a marker that was taken
// there are no options left to read, so `shift -- -1` is a count in all three
// dialects that have one.
func (r *Runner) shiftCount(operand string, marked bool, n *int) (int, bool) {
	if !marked && len(operand) > 1 && operand[0] == '-' {
		p := r.shiftOptionWords()
		if r.unspecified {
			return r.status, true
		}
		if p == ShiftOptionWordsAny || (p == ShiftOptionWordsNonNumeric && !allDigits(operand[1:])) {
			// The *first letter*, not the whole word: a leading `-` word is
			// a bundle of single-letter options, so `shift --help` is
			// refused as `-h` — the dashes are stripped and the first
			// letter after them is the one named. printf's options already
			// follow the same rule, measured the same way.
			return r.badBuiltinOption("shift", "-"+firstOptionLetter(operand)), true
		}
	}
	if v, ok := atoiSigned(operand); ok {
		// A plain number, which every reading agrees on. Nothing is asked
		// for a count of zero or more: `shift 2` is two everywhere, and so
		// is `shift +2` — the sign is unanimous.
		return r.shiftTakeCount(v, operand, n)
	}
	if r.ask(r.sem().ShiftCountIsArithmetic, "`shift n` reading its count as an expression") {
		// An expression rather than a number: `shift 1+1` moves two and
		// `shift n` moves whatever n holds. An unset name is zero there, so
		// `shift abc` shifts nothing and succeeds, where the dialects that
		// want a number call it one they cannot read.
		tree, perr := r.arithTree(nil, operand)
		if perr != nil {
			r.diagf("shift: %v\n", perr)
			return 2, true
		}
		v, err := r.evalArith(tree)
		if err != nil {
			return r.status, true
		}
		return r.shiftTakeCount(v, operand, n)
	}
	if r.unspecified {
		return r.status, true
	}
	return r.shiftBadNumber(operand)
}

// shiftTakeCount accepts a count that was read, asking about a negative one.
//
// Below zero is where the reading of the word itself is still in question:
// bash, ksh93 and zsh have a count that is out of range, and dash has a word
// that is not a number — the same complaint it makes about `-x`. Above or at
// zero nothing is asked, which is why the ordinary `shift 2` needs no
// dialect.
func (r *Runner) shiftTakeCount(v int, operand string, n *int) (int, bool) {
	if v < 0 && !r.ask(r.sem().ShiftNegativeIsOutOfRange, "a negative `shift` count read as a count out of range") {
		if r.unspecified {
			return r.status, true
		}
		return r.shiftBadNumber(operand)
	}
	*n = v
	return 0, false
}

// shiftBadNumber is the complaint about an operand that is not a count, and
// the end of the script where a special builtin's bad operand ends one.
func (r *Runner) shiftBadNumber(operand string) (int, bool) {
	d := r.diag()
	r.diagf("%s\n", Wording(d.ShiftBadNumber, "shift: %[1]s: numeric argument required", operand))
	status := orDefault(d.BuiltinBadOptionStatus, 2)
	if r.ask(r.sem().BadOptionToSpecialBuiltinFatal, "a special builtin's bad operand ending the script") {
		r.fatalUsageQuiet()
		// The builtin's own status and not `r.status`, which fatalQuiet has
		// just written the dialect's generic fatal answer into — 1 in bash,
		// and the panel ends this at 2. See setFatalStatus (#2583).
		return status, true
	}
	return status, true
}

// biEcho writes its arguments separated by spaces.
//
// Whether backslash escapes expand without -e is the EchoInterpretsEscapes
// axis — dash and zsh expand them, bash and ksh93 do not — asked below, and
// only when a backslash appears, so `echo hi` needs no dialect. Which option
// letters exist, which of `-e -E` wins, and the \x and \e set extensions are
// each their own axis, read the same way.
//
// Runner.EchoExpandsEscapes is a script's way onto the other side of that
// axis in the one shell with a name for it, and it is read where the axis is
// read: behind `-e`, which already expands, and behind `-E`, which still
// suppresses for the one call. Measured — see the switch.
func biEcho(r *Runner, _ context.Context, args []string) int {
	newline := true
	letters := r.sem().EchoOptions
	if letters == "" {
		letters = "n"
	}
	// -1 is -E, +1 is -e, 0 is the dialect's default. A word carrying any
	// letter outside the dialect's set is not an option at all — the whole
	// word becomes an operand, which is unanimous: `echo -nq hi` prints
	// `-nq hi` in all four shells.
	forced := 0
	for len(args) > 0 {
		a := args[0]
		if len(a) < 2 || a[0] != '-' || strings.ContainsFunc(a[1:], func(c rune) bool {
			return !strings.ContainsRune(letters, c)
		}) {
			break
		}
		for i := 1; i < len(a); i++ {
			switch a[i] {
			case 'n':
				newline = false
			case 'e':
				forced = 1
			case 'E':
				// `-E -e` expands everywhere; only `-e -E` splits the
				// panel, so the question waits for that order.
				if forced != 1 ||
					r.ask(r.sem().EchoLastEscapeFlagWins, "which of `echo -e -E` decides") {
					forced = -1
				}
			}
		}
		args = args[1:]
	}
	out := strings.Join(args, " ")
	// dash and zsh expand backslash escapes without -e; bash and ksh93 do
	// not. A grouping no other axis produces.
	// Asked only when the text could differ either way, so `echo hi` needs no
	// dialect and `echo 'a\tb'` does.
	expand := forced == 1
	if forced == 0 {
		// The session switch first, and it is an override rather than a
		// second answer: one shell in the panel lets a script ask to stand on
		// the other side of the axis while it runs, and a shell that has
		// asked is not also asking the dialect. Reading it ahead of the axis
		// is what keeps a dialect with no answer from being complained about
		// in a shell that has already said which side it is on. See
		// Runner.EchoExpandsEscapes.
		expand = strings.ContainsRune(out, '\\') &&
			(r.EchoExpandsEscapes() ||
				r.ask(r.sem().EchoInterpretsEscapes, "echo interpreting backslash escapes"))
	}
	if expand && strings.ContainsRune(out, '\\') {
		// The two set extensions are asked only when their escapes appear.
		hex := strings.Contains(out, `\x`) &&
			r.ask(r.sem().EchoExpandsHexEscapes, "echo expanding \\xHH")
		// The two spellings of the escape character are two questions, and
		// each is asked only where its own letter appears: ksh93 has `\E`
		// and not `\e`, zsh has `\e` and not `\E`, so one answer for both
		// was wrong for half the panel (#908).
		esc := strings.Contains(out, `\e`) &&
			r.ask(r.sem().EchoExpandsEscEscape, "echo expanding \\e")
		capEsc := strings.Contains(out, `\E`) &&
			r.ask(r.sem().EchoExpandsCapitalEscEscape, "echo expanding \\E")
		// One axis for the two Unicode letters, because no shell measured
		// has one without the other, and one more for what a hexadecimal
		// escape with no digit after it means — which is the same question
		// for `\x`, `\u` and `\U`, and is asked only where such an escape
		// actually runs out of digits.
		unicode := (strings.Contains(out, `\u`) || strings.Contains(out, `\U`)) &&
			r.ask(r.sem().EchoExpandsUnicodeEscapes, "echo expanding \\uHHHH and \\UHHHHHHHH")
		how := echoEscapes{
			hex: hex, esc: esc, capEsc: capEsc, unicode: unicode,
			// Handed in even where the dialect has no `\u`: the reader is
			// only reached from the branch `unicode` guards, so a dialect
			// without the escape never calls it, and a nil here would be a
			// panic waiting for the first dialect that grows one.
			codePoint: r.CodePointEscapeText,
		}
		if emptyHexRun(out, hex, unicode) {
			how.emptyRunIsNul = r.ask(r.sem().EchoEmptyHexDigitRunIsNul,
				"echo reading a hexadecimal escape with no digits")
		}
		var stopped, refused bool
		out, stopped, refused = expandEchoEscapes(out, how)
		if stopped {
			// `\c` ends the output, newline included.
			newline = false
		}
		if refused {
			// Reported once, here, however many escapes the argument held —
			// and *before* the output, which is the order the two streams
			// actually come out in: the shell that refuses reads the word
			// before `echo` writes anything.
			//
			// The text before the escape is still written, with the closing
			// newline `echo` would have added. Measured: `echo "a<esc>Z"`
			// leaves `a` and a newline, and `echo -n` the same without one.
			r.RefuseCodePoint()
			if newline {
				out += "\n"
			}
			if out != "" {
				if _, err := r.stdout().Write([]byte(out)); err != nil {
					r.writeFailed = err
				}
			}
			return r.status
		}
	}
	if newline {
		out += "\n"
	}
	if out == "" {
		// `echo -n` has nothing to write, and nothing cannot fail to be
		// written: it succeeds even on a closed descriptor, everywhere
		// measured.
		return 0
	}
	// Recorded, not returned: whether a write that went nowhere fails the
	// command is the dispatcher's question, answered once for every builtin.
	if _, err := r.stdout().Write([]byte(out)); err != nil {
		r.writeFailed = err
	}
	return 0
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func hexValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	default:
		return int(c-'A') + 10
	}
}

// xsiEscape is the escape table POSIX's XSI `echo` defines with one letter,
// and it is the one part of the subject nothing in the panel argues about: an
// `echo` argument, a `printf` format and a `%b` argument all read these eight
// the same way, in all six shells measured.
//
// It is written once because the three sites that read escapes disagree about
// everything *else* — `\c`, `\e`, `\x` and the octal forms each split the
// panel differently, and each split falls in a different place per site — so
// the eight that never move are the thing to hold still, rather than the
// thing to copy three times.
//
// ok is false for a character no entry claims, which leaves what to do with
// the backslash to the caller: the sites do not agree about that either.
func xsiEscape(c byte) (byte, bool) {
	switch c {
	case 'a':
		return '\a', true
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case 't':
		return '\t', true
	case 'v':
		return '\v', true
	case '\\':
		return '\\', true
	}
	return 0, false
}

// echoEscapes is which of the extensions this dialect admits, for the one
// argument being read. A struct rather than a row of booleans: the set grew to
// five and a call site passing them positionally is a place two of them can be
// swapped without anything failing to compile.
type echoEscapes struct {
	hex           bool
	esc           bool
	capEsc        bool
	unicode       bool
	emptyRunIsNul bool
	// codePoint turns one escape's value into text, with the locale
	// consulted — see Runner.CodePointEscapeText. A closure rather than a
	// flag because the answer is a dialect's *and* the runner's variables,
	// and expandEchoEscapes is a pure function of its two arguments.
	//
	// nil where no `\u` escape can be reached, which is every dialect
	// without the escape and every argument without one in it.
	codePoint func(int) (string, bool)
}

// emptyHexRun reports whether the text carries a hexadecimal escape that runs
// out of digits, which is the only place EchoEmptyHexDigitRunIsNul is asked.
//
// Asked of the escapes this dialect actually reads: a `\x` in a shell without
// `\x` is two characters whatever the digits after it are, so the axis has no
// bearing there and asking it would refuse a word the shell has an answer for.
func emptyHexRun(s string, hex, unicode bool) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] != '\\' {
			continue
		}
		i++
		switch {
		case s[i] == 'x' && hex, (s[i] == 'u' || s[i] == 'U') && unicode:
			if i+1 >= len(s) || !isHexDigit(s[i+1]) {
				return true
			}
		}
	}
	return false
}

// expandEchoEscapes interprets the escapes `echo` expands where the dialect
// says it does: the XSI set, with `\xHH`, `\uHHHH`, `\UHHHHHHHH`, `\e` and
// `\E` admitted per dialect — `\e` and `\E` separately, because ksh93 and zsh
// have one each and not the other, and the two Unicode letters together,
// because no shell measured has one without the other.
// stopped reports a `\c`, which discards the rest of the output and the
// closing newline with it. refused reports an escape the locale has no room
// for in a dialect that refuses one, which ends the text the same way and is
// the caller's to report and abandon on — see Runner.RefuseCodePoint.
func expandEchoEscapes(s string, how echoEscapes) (expanded string, stopped, refused bool) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		if e, ok := xsiEscape(s[i]); ok {
			b.WriteByte(e)
			continue
		}
		switch s[i] {
		case 'c':
			return b.String(), true, false
		case 'e', 'E':
			admitted := how.esc
			if s[i] == 'E' {
				admitted = how.capEsc
			}
			if !admitted {
				b.WriteByte('\\')
				b.WriteByte(s[i])
				break
			}
			b.WriteByte(0x1b)
		case '0':
			// `\0` and up to three octal digits after it.
			n, j := 0, i+1
			for j < len(s) && j <= i+3 && s[j] >= '0' && s[j] <= '7' {
				n = n*8 + int(s[j]-'0')
				j++
			}
			b.WriteByte(byte(n))
			i = j - 1
		case 'x':
			if !how.hex {
				b.WriteByte('\\')
				b.WriteByte(s[i])
				break
			}
			n, j := hexRun(s, i+1, 2)
			if j == i+1 && !how.emptyRunIsNul {
				// `\x` with no digits stays as written.
				b.WriteString(`\x`)
				break
			}
			b.WriteByte(byte(n))
			i = j - 1
		case 'u', 'U':
			if !how.unicode {
				b.WriteByte('\\')
				b.WriteByte(s[i])
				break
			}
			// Four digits after `\u` and eight after `\U`, fewer accepted:
			// `\u41` is `A` and `\u00410` is `A` followed by a zero.
			width := 4
			if s[i] == 'U' {
				width = 8
			}
			n, j := hexRun(s, i+1, width)
			if j == i+1 && !how.emptyRunIsNul {
				b.WriteByte('\\')
				b.WriteByte(s[i])
				break
			}
			text, no := how.codePoint(n)
			if no {
				// A code point the locale cannot hold, in the dialect that
				// refuses one. What came before the escape is still written
				// — measured — and nothing after it is.
				return b.String(), false, true
			}
			b.WriteString(text)
			i = j - 1
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String(), false, false
}

// hexRun reads up to max hexadecimal digits from i, and reports the value and
// where the run ended. An empty run is a zero at i, which is the caller's to
// tell apart from a zero digit — see Semantics.EchoEmptyHexDigitRunIsNul.
func hexRun(s string, i, maxDigits int) (n, end int) {
	end = i
	for end < len(s) && end < i+maxDigits && isHexDigit(s[end]) {
		n = n*16 + hexValue(s[end])
		end++
	}
	return n, end
}

// EncodeCodePoint writes one code point the way the escape sites do, which is
// the *original* UTF-8 rather than the range it was later narrowed to.
//
// Exported because a dialect's own escape reader needs the same encoder and
// must not grow a second one: `print` has `\u` and `\U` already, and wrote a
// replacement character for everything Go's rune type refuses (#1840) while
// the shell it copies writes the encoding.
//
// Measured 2026-09-10 against zsh 5.9.2 and bash 5.3.15, which agree on all of
// it: a surrogate is encoded rather than replaced (`\ud800` is `ed a0 80`), so
// is a value past the last code point (`\U110000` is `f4 90 80 80`), and the
// five- and six-byte forms are reachable — `\U200000` is `f8 88 80 80 80` and
// `\U4000000` is `fc 84 80 80 80 80`. A value past what six bytes can hold
// overflows into the lead byte rather than being refused, which is the one
// place the two shells part company and is left where the arithmetic puts it.
func EncodeCodePoint(n int) string {
	u := uint32(n)
	switch {
	case u < 0x80:
		return string([]byte{byte(u)})
	case u < 0x800:
		return string([]byte{byte(0xC0 | u>>6), cont(u, 0)})
	case u < 0x10000:
		return string([]byte{byte(0xE0 | u>>12), cont(u, 6), cont(u, 0)})
	case u < 0x200000:
		return string([]byte{byte(0xF0 | u>>18), cont(u, 12), cont(u, 6), cont(u, 0)})
	case u < 0x4000000:
		return string([]byte{
			byte(0xF8 | u>>24), cont(u, 18), cont(u, 12), cont(u, 6), cont(u, 0),
		})
	}
	return string([]byte{
		byte(0xFC | u>>30), cont(u, 24), cont(u, 18), cont(u, 12), cont(u, 6), cont(u, 0),
	})
}

// cont is one continuation byte: the six bits at that shift, under the 10 the
// encoding marks them with.
func cont(u uint32, shift int) byte { return byte(0x80 | (u>>shift)&0x3F) }

// biCd changes the shell's working directory.
//
// This is the definition of a core primitive: it changes the runner's own
// state, every dialect needs it, and no shell function can say it. It lived
// in cmd/bash while that binary was demonstrating Register, which was the
// right place for a demonstration and the wrong one to leave it.
// cdOptions reads `cd`'s leading options.
//
// `-L` and `-P` are the unanimous two: the default and `-L` keep the name the
// directory was reached by, and `-P` resolves it. Measured through a symlink,
// where all four print the link's path for the first two and the real one for
// the third.
//
// `-q` is a third and `-s` a fourth, and both belong to one shell — see
// CdHasQuietOption and CdHasSymlinkFreeOption.
//
// A lone `-` is not an option — it is the previous directory — which the
// length test leaves alone. `--` ends the options in all six, which is what
// lets a directory whose name begins with a dash be reached at all: measured,
// `cd -- -dashdir` moves in every one of them and `cd -dashdir` moves only in
// zsh, where the word is an operand rather than a bundle of letters.
func (r *Runner) cdOptions(args []string) (rest []string, opts cdFlags, code int) {
	sawLogical, sawPhysical := false, false
	done := func(rest []string, code int) ([]string, cdFlags, int) {
		// Which of the two decides is a question only when both were given,
		// and it is asked only then: with one of them the two rules agree,
		// and a shell that refused an unambiguous `cd -P` would be refusing
		// over a disagreement that is not in front of it.
		if sawLogical && sawPhysical && !r.ask(r.sem().CdLastPathOptionWins, "which of `cd -L` and `cd -P` decides") {
			opts.physical = sawPhysical
		}
		return rest, opts, code
	}
	for len(args) > 0 {
		a := args[0]
		if len(a) < 2 || a[0] != '-' {
			break
		}
		if a == "--" {
			return done(args[1:], 0)
		}
		for i := 1; i < len(a); i++ {
			switch a[i] {
			case 'L':
				opts.physical, sawLogical = false, true
			case 'P':
				opts.physical, sawPhysical = true, true
			case 'q':
				// zsh's quiet `cd`, and the letter that stops a plugin
				// manager dead: the loader wraps every move in an anonymous
				// function precisely so the directory hooks stay quiet, and
				// without the letter here the whole word became the operand
				// and the move never happened. #1558.
				//
				// Honored rather than accepted: what `-q` asks is that
				// the directory-change hook not run, so the letter is
				// carried to the site that fires it — see biCd's last act
				// and Semantics.DirectoryChangeHook. It was free while this
				// shell had no such site, and dialect/zsh's tripwire test is
				// what stopped it staying free once the site arrived.
				//
				// `continue` rather than `break`, and the two are the same
				// thing here: this switch is inside the letter loop, so
				// breaking the switch also goes on to the next letter. A
				// mutation run cannot tell them apart and no test can, which
				// is why the word is chosen for the reader — the letter is
				// done and the next one is next, said once.
				//
				// Unanswered stops here rather than falling through, and the
				// difference is what a shell with no dialect *says*. The
				// question below is reached only by this one having defaulted
				// to no, so falling through would name two unanswered axes
				// where one was asked — and the reader would have to work out
				// which of them decided. `cd -Z` names one axis; `cd -q` now
				// names one too.
				if a := r.sem().CdHasQuietOption; a != No {
					if r.ask(a, "`cd -q`") {
						opts.quiet = true
						continue
					}
					return nil, opts, r.status
				}
				// A shell without the letter answers the word the way it
				// answers any other letter it does not have, which is the
				// next question rather than a second rule.
				fallthrough
			case 's':
				// zsh's other letter, and the same shape as `-q` down to the
				// unanswered branch: carried to where there is something to
				// do with it rather than swallowed here, because what it
				// asks about is the *operand* and the operand has not been
				// read yet. See Semantics.CdHasSymlinkFreeOption.
				//
				// The `q` case falls into this one, so the guard has to be
				// on the letter as well as on the axis: a `q` in a shell
				// without `-q` must reach the unknown-letter question below
				// and not be read as an `s`.
				if ans := r.sem().CdHasSymlinkFreeOption; a[i] == 's' && ans != No {
					if r.ask(ans, "`cd -s`") {
						opts.symlinkFree = true
						continue
					}
					return nil, opts, r.status
				}
				fallthrough
			default:
				// The one place the panel splits: three of them refuse a
				// letter `cd` does not have, and zsh reads the word as
				// somewhere to go instead — `cd -Q` looks for a directory
				// called `-Q` there.
				if !r.ask(r.sem().CdRefusesUnknownOption, "an option `cd` does not have") {
					return done(args, 0)
				}
				// The dialect that names every letter of a bundle it cannot
				// use names them here too: `cd -dash` is four complaints and
				// one usage block. The letters `cd` has are the cases above,
				// and the two conditional ones are asked of the dialect
				// rather than assumed, so a shell with `-q` is not told it
				// lacks one. See Semantics.BuiltinReportsEveryBadOption.
				known := "LP"
				if r.sem().CdHasQuietOption != No {
					known += "q"
				}
				if r.sem().CdHasSymlinkFreeOption != No {
					known += "s"
				}
				if bad := r.everyBadOption(a, known); len(bad) > 1 &&
					r.ask(r.sem().BuiltinReportsEveryBadOption, "`cd` naming every bad letter of a bundle") {
					return nil, opts, r.badBuiltinOption("cd", bad...)
				}
				return nil, opts, r.badBuiltinOption("cd", "-"+string(a[i]))
			}
		}
		args = args[1:]
	}
	return done(args, 0)
}

func biCd(r *Runner, ctx context.Context, args []string) int {
	args, opts, code := r.cdOptions(args)
	if code != 0 {
		return code
	}
	physical := opts.physical
	old := r.workDir()
	dir, dash, code, stop := r.cdDestination(args, old)
	if stop {
		return code
	}
	// A rewrite of the current directory announces where it went in one of
	// the two shells that have the form, the way `cd -` does in three of the
	// four. Recorded here and printed after the move, because a rewrite that
	// arrives nowhere prints nothing in either.
	substituted := len(args) > 1 && !stop

	// What the failure below names, captured before CDPATH and before the
	// operand is joined against the working directory: every shell in the
	// panel reports the place it was asked for rather than the place it
	// worked out. For `cd alpah` that is `alpah` as typed, and for a `cd`
	// with no operand it is the *value of HOME* — measured 2026-09-08 with
	// HOME set to a directory that is not there, where bash, dash, ksh93 and
	// zsh all name `/nonexistent-dir` and none of them names nothing.
	//
	// Read from args[0] before this, which is where the crash was: with no
	// operand there is no args[0], so `cd` with an unreachable HOME indexed
	// an empty slice and took the shell down with it — `sudo -i`, a
	// container, a home that has been removed. One name for both cases
	// rather than a second guard beside the first, because a second guard is
	// what the spelling correction below would have had to add.
	named := dir
	// `cd -s` refuses an operand that crosses a symbolic link, and refuses it
	// *here* — before CDPATH, before the join, and before the operand's
	// existence is asked about. That order is measured: `cd -s /tmp/no/such`
	// on a machine where `/tmp` is a link says `not a directory` while
	// `cd -s real/nosuch` says `no such file or directory`, so the walk stops
	// at the first link it meets and leaves a missing component to the
	// ordinary failure below.
	//
	// ENOTDIR rather than a sentence of its own, because that is what the
	// shell says and because the dialect already words it: the refusal is
	// indistinguishable from the kernel's own, and a reader who does not know
	// the letter reads it as the path not being a directory — which, for a
	// `cd` that will not follow links, it is not. See
	// Semantics.CdHasSymlinkFreeOption.
	if opts.symlinkFree && r.operandCrossesASymlink(old, dir) {
		notDir := &fs.PathError{Op: "chdir", Path: dir, Err: syscall.ENOTDIR}
		r.NoteErrno(notDir)
		r.diagf("%s\n", Wording(r.diag().CdCannotChange, "cd: %[1]s: %[2]s",
			named, r.diag().reasonText(reason(notDir))))
		return orDefault(r.diag().CdStatus, 1)
	}
	announced := false
	if !filepath.IsAbs(dir) && !dash {
		// CDPATH, searched for an operand that is not absolute and does not
		// lead with a dot — `cd ./x` names a place, not a search. The entry
		// that wins decides the announcement: a plain `.` moves quietly, and
		// any other winner is printed — in three of the four; zsh moves in
		// silence either way.
		//
		// One column searches a dotted operand too and refuses a miss
		// outright, which is the same axis read in two places: see
		// Semantics.CdpathReplacesTheRelativeLookup.
		found, viaPath, searched := r.searchCdpath(dir)
		if r.unspecified {
			return 2
		}
		switch {
		case viaPath != "":
			if viaPath != "." &&
				r.ask(r.sem().CdpathAnnouncesTheDirectory, "`cd` printing where CDPATH sent it") {
				announced = true
			}
			if r.unspecified {
				return 2
			}
			dir = found
		case searched && r.cdpathReplacesTheLookup():
			// The search ran and missed, in the shell where there is no
			// ordinary relative lookup left to fall back to. What it says is
			// the ordinary missing-directory sentence, which is what the
			// reference writes, so the failure below does the talking.
			return r.cdNotThere(named)
		}
		if r.unspecified {
			return 2
		}
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(old, dir)
	}
	if physical {
		// `-P` is where the directory *is*, rather than the name it was
		// reached by. Every shell in the panel resolves the whole path and
		// reports the resolved one from `pwd` afterwards, so this replaces
		// the name rather than only checking it.
		//
		// A path that cannot be resolved is left as written: what to say
		// about a directory that is not there is the question below, and it
		// answers with what the operating system said rather than with
		// anything this step could add. A symlink cycle arrives here that
		// way and reads as ELOOP from the stat, which is what the panel
		// says a cycle is — under `-P` and under `-L` alike, since the
		// chdir hits it either way.
		//
		// Through the gate, one component at a time: resolving is a walk
		// over the filesystem and a script chose the path, so a policy has
		// to see each step of it. See physicalpath.go.
		if resolved, err := r.physicalPath(dir); err == nil {
			dir = resolved
		}
	}
	// Through the gate. A denied stat surfaces as the missing-directory
	// error below, so `cd` into a path the policy hides fails the way `cd`
	// into a path that is not there does — same sentence, same status.
	//
	// The *entering* question rather than "is this a directory": a directory
	// with no execute bit stats perfectly well and cannot be entered, and
	// reading the stat moved us into one where the whole panel refuses. An
	// operand that is not a directory comes back as ENOTDIR from the kernel
	// now, which is the errno the synthesized one here spelled by hand. See
	// enterable (#1492).
	err := r.enterable(dir)
	if err != nil {
		// A misspelling is the one failure this can still recover from, and
		// only where the shell asked for that — see cdCorrected, which
		// answers false in every shell that did not. The corrected operand is
		// printed rather than merely used: bash writes `alpha/beta` for
		// `alpah/beta` on **stdout** before moving, which is a different
		// stream from the diagnostic below and the reason a person can tell
		// a correction from a failure.
		if fixed, shown, corrected := r.cdCorrected(named, old, physical); corrected {
			dir, err = fixed, nil
			r.printf("%s\n", shown)
		}
	}
	if err != nil {
		// The number the kernel gave, for the parameter and the builtin that
		// present it — `cd` is the plainest system call a script makes, and
		// the one it asks about afterwards. See interp/errno.go.
		r.NoteErrno(err)
		// The reason the operating system gave, rather than one made up
		// here: three of the four report it, and two of those distinguish a
		// path that is not there from one that is not a directory. Saying
		// "no such directory" for both was a sentence no shell prints and an
		// answer one of them can tell is wrong.
		//
		// The operand as it was *written*, even when a correction was tried
		// and failed: bash says `cd: alpha/bteta/gamma: No such file or
		// directory` for the path it could not finish correcting, naming what
		// the person typed rather than how far it got.
		r.diagf("%s\n", Wording(r.diag().CdCannotChange, "cd: %[1]s: %[2]s",
			named, r.diag().reasonText(reason(err))))
		return orDefault(r.diag().CdStatus, 1)
	}
	// Only the runner's own directory moves. Calling os.Chdir would move the
	// whole process, which is wrong for an embedded interpreter and would be
	// shared by every Runner in it.
	r.Dir = dir
	r.setVar("OLDPWD", old)
	r.setVar("PWD", dir)
	if announced {
		r.printf("%s\n", dir)
	}
	if dash && r.ask(r.sem().CdDashPrintsTheDirectory, "`cd -` printing where it went") {
		// Asked only for `cd -`, which is the only form any of them prints.
		r.printf("%s\n", dir)
	}
	if substituted &&
		r.ask(r.sem().CdSubstitutionPrintsTheDirectory, "`cd old new` printing where it went") {
		r.printf("%s\n", dir)
	}
	// Last, after the move and after anything `cd` itself printed, which is
	// measured: at a prompt `cd -` wrote the directory and *then* the hook's
	// marker, and a CDPATH move did the same. Only on a `cd` that got
	// somewhere — every return above this line is a `cd` that did not move,
	// and none of them fires it.
	//
	// `-q` is the one move that stays quiet, which is the whole of what that
	// letter means; the flag is read here rather than at the option loop
	// because here is where there is something to suppress.
	if !opts.quiet {
		r.FireHook(ctx, nil, r.sem().DirectoryChangeHook)
	}
	return 0
}

// cdFlags is what `cd`'s option letters left behind, and it is a struct for
// one reason: a second `bool` in a return list is a second thing to thread
// through the same three places, and the two before it were already returned
// positionally where a reader has to count. See cdOptions.
type cdFlags struct {
	// physical resolves the path rather than keeping the name it was
	// reached by — `-P` against the default and `-L`.
	physical bool

	// quiet suppresses the directory-change hook — zsh's `-q`, and nothing
	// besides. See Semantics.CdHasQuietOption.
	quiet bool

	// symlinkFree refuses an operand that crosses a symbolic link — zsh's
	// `-s`. See Semantics.CdHasSymlinkFreeOption.
	symlinkFree bool
}

// searchCdpath walks CDPATH for a relative operand that does not lead with
// a dot, returning the joined path of the first entry holding a directory of
// that name and the entry that held it. All four shells search; who prints
// afterwards is the axis at the call.
func (r *Runner) searchCdpath(operand string) (found, via string, searched bool) {
	if strings.HasPrefix(operand, "./") || strings.HasPrefix(operand, "../") {
		// A dot component names a place rather than starting a search, in
		// every column. See Semantics.CdpathReplacesTheRelativeLookup for
		// the one row this leaves disagreeing and why it is not modeled.
		return "", "", false
	}
	cdpath, ok := r.getVar("CDPATH")
	if !ok || cdpath == "" {
		// An empty CDPATH is no search at all, which is why the shell that
		// has no fallback still reaches an ordinary relative `cd` with one.
		return "", "", false
	}
	searched = true
	for _, entry := range strings.Split(cdpath, string(filepath.ListSeparator)) {
		if entry == "" {
			entry = "."
		}
		candidate := entry + string(filepath.Separator) + operand
		abs := candidate
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(r.workDir(), abs)
		}
		// Through the gate; an entry the policy hides is walked past the
		// way an entry with no such directory is.
		if st, err := r.stat(abs); err == nil && st.IsDir() {
			return abs, entry, true
		}
	}
	return "", "", searched
}

// cdNotThere is the ordinary missing-directory refusal, for the one place a
// `cd` fails without ever asking the filesystem: a CDPATH search that missed
// in the shell that has no relative lookup left to fall back to.
//
// The same wording and the same status the walk below produces, from the same
// errno, so a script trapping on `cd` cannot tell the two apart — which is
// what the reference does, since there the search *is* the lookup.
func (r *Runner) cdNotThere(named string) int {
	err := &fs.PathError{Op: "chdir", Path: named, Err: syscall.ENOENT}
	r.NoteErrno(err)
	r.diagf("%s\n", Wording(r.diag().CdCannotChange, "cd: %[1]s: %[2]s",
		named, r.diag().reasonText(reason(err))))
	return orDefault(r.diag().CdStatus, 1)
}

// cdpathReplacesTheLookup resolves the axis, and only where a CDPATH is set
// and non-empty — a shell with none has no search for the answer to be about.
// See Semantics.CdpathReplacesTheRelativeLookup.
func (r *Runner) cdpathReplacesTheLookup() bool {
	if cdpath, ok := r.getVar("CDPATH"); !ok || cdpath == "" {
		return false
	}
	return r.ask(r.sem().CdpathReplacesTheRelativeLookup,
		"`cd` resolving a relative operand through CDPATH alone")
}

// cdDestination works out where a `cd` was asked to go, reporting whether it
// is finished already — refused, or asked to go nowhere.
//
// The operands are three questions the panel answers differently, and it
// answered all three with one reading before: the first operand was taken and
// the rest ignored, which is bash 3.2's and dash's answer given to all four
// dialects (#1491).
//
//	cd ""          an empty *operand*, which is not the same as no operand
//	cd a b         two operands, which two of the panel read as a rewrite of
//	               the current directory rather than as a mistake
//	HOME= cd       an empty HOME, which is not the same as an absent one
func (r *Runner) cdDestination(args []string, old string) (dir string, dash bool, code int, stop bool) {
	if len(args) > 1 {
		if to, code, ok := r.cdSubstituted(args, old); ok {
			return to, false, 0, false
		} else if code != 0 {
			return "", false, code, true
		}
	}
	if len(args) == 0 {
		dir, code, stop = r.cdHome()
		return dir, false, code, stop
	}
	switch dir = args[0]; dir {
	case "":
		// An empty operand is somewhere rather than nowhere: three of the
		// six take it as the directory they are already in — OLDPWD moves to
		// where the shell was and the change hook fires, so it is a real
		// move to the same place rather than a no-op — and joining an empty
		// operand against the working directory below is exactly that. Two
		// refuse it outright, which is the axis (#1491).
		if !r.ask(r.sem().CdEmptyOperandIsAnError, "an empty operand to `cd`") {
			return "", false, 0, r.unspecified
		}
		r.diagf("%s\n", Wording(r.diag().CdEmptyOperand, "cd: null directory"))
		return "", false, orDefault(r.diag().CdStatus, 1), true
	case "-":
		// The previous directory, which is why cd records one.
		dash = true
		dir, _ = r.getVar("OLDPWD")
		if dir == "" {
			if code, stop := r.cdNowhere(r.diag().CdOldpwdNotSet, "cd: OLDPWD not set"); stop {
				return "", true, code, true
			}
			// Not an error here, and not nothing either: the dialects that
			// survive this go to where they already are, which still prints
			// for the ones that print.
			dir = old
		}
	}
	return dir, dash, 0, false
}

// cdHome is `cd` with no operand at all.
//
// Absent and empty are two answers and were one: `biCd` read HOME and tested
// the value against "", so a HOME set to nothing reached the branch that says
// HOME is not set. Five of the six shells separate them — an empty HOME is an
// empty *destination*, which is somewhere, and only an absent one is "HOME not
// set" — and ours said `HOME not set` where bash says nothing at all.
func (r *Runner) cdHome() (dir string, code int, stop bool) {
	home, set := r.getVar("HOME")
	if !set {
		code, _ := r.cdNowhere(r.diag().CdHomeNotSet, "cd: HOME not set")
		// With no HOME there is nowhere to go even for the dialects that
		// do not call it an error, so this stops either way.
		return "", code, true
	}
	if home != "" {
		return home, 0, false
	}
	// Set to nothing. One shell refuses it, in the same words it refuses an
	// empty operand with; the rest go where they already are, quietly, and
	// the empty destination below joins to exactly that.
	if !r.ask(r.sem().CdEmptyHomeIsAnError, "a HOME set to the empty string") {
		return "", 0, r.unspecified
	}
	r.diagf("%s\n", Wording(r.diag().CdEmptyOperand, "cd: null directory"))
	return "", orDefault(r.diag().CdStatus, 1), true
}

// cdSubstituted is `cd old new`, which rewrites the current directory's path
// rather than naming a directory — the form two of the panel have.
//
// It reports the rewritten path, or a status and false where the form is
// refused. A third return of (0, false) means this dialect has no second
// operand at all and the caller should take the first.
//
// The first occurrence in the *string*, not the first path component: measured
// in both shells that have it, `cd a Z` from `…/a/q/a/w` lands in `…/Z/q/a/w`.
// What comes back is absolute, so it skips CDPATH the way any absolute operand
// does, and a rewritten path that is not there is reported as that path rather
// than as either operand.
func (r *Runner) cdSubstituted(args []string, old string) (to string, code int, ok bool) {
	if !r.ask(r.sem().CdSubstitutesTheOperands, "`cd old new` rewriting the current directory") {
		if r.unspecified {
			return "", r.status, false
		}
		// Not the form, so the operands are simply too many — or not, in the
		// two columns that ignore everything after the first.
		if !r.ask(r.sem().CdRefusesExtraOperands, "`cd` given more operands than it takes") {
			if r.unspecified {
				return "", r.status, false
			}
			return "", 0, false
		}
		return "", r.cdTooManyOperands(), false
	}
	if len(args) > 2 {
		// The form takes exactly two, and both shells that have it refuse a
		// third — in different words and with different statuses, which is
		// what cdTooManyOperands carries.
		return "", r.cdTooManyOperands(), false
	}
	i := strings.Index(old, args[0])
	if i < 0 {
		r.diagf("%s\n", Wording(r.diag().CdBadSubstitution, "cd: bad substitution", args[0]))
		return "", orDefault(r.diag().CdStatus, 1), false
	}
	return old[:i] + args[1] + old[i+len(args[0]):], 0, true
}

// cdTooManyOperands refuses more operands than this `cd` takes.
//
// Two fields rather than one because the panel writes two different things:
// bash and zsh write a sentence and no usage block, ksh93 writes its usage
// block and no sentence. Neither implies the other, and a dialect could write
// both.
func (r *Runner) cdTooManyOperands() int {
	d := r.diag()
	if msg := Wording(d.CdTooManyOperands, ""); msg != "" {
		r.diagf("%s\n", msg)
	}
	if d.CdTooManyOperandsShowsUsage {
		r.builtinUsageLine("cd")
	}
	return orDefault(d.CdTooManyOperandsStatus, orDefault(d.CdStatus, 1))
}

// cdNowhere is `cd` with nothing to go to: no HOME, or no OLDPWD.
//
// Two dialects call that an error and two stay where they are and report
// success — quietly, which is the surprising half: a script that relies on
// `cd` moving has already carried on by the time it notices.
func (r *Runner) cdNowhere(wording, fallback string) (int, bool) {
	if !r.ask(r.sem().CdWithoutHomeIsAnError, "`cd` with nowhere to go being an error") {
		return 0, false
	}
	r.diagf("%s\n", Wording(wording, fallback))
	return orDefault(r.diag().CdStatus, 1), true
}

func biPwd(r *Runner, _ context.Context, args []string) int {
	// `-L` and `-P` with the last one deciding, which is unanimous —
	// `pwd -L -P` is physical in all four shells. Operands are ignored by
	// three of the four, which is what leaving them unread does.
	_, opts, code := r.builtinOptions("pwd", args, "LP")
	if code != 0 {
		return code
	}
	physical := false
	for _, o := range opts {
		physical = o == 'P'
	}
	dir := r.workDir()
	if physical {
		// Where the directory *is*, not the name it was reached by. A path
		// that cannot be resolved is printed as held, the same answer `cd
		// -P` gives for one — and through the gate a component at a time,
		// because `pwd -P` is a question about the filesystem and a policy
		// that hides part of it must be able to refuse the answer.
		if resolved, err := r.physicalPath(dir); err == nil {
			dir = resolved
		}
	}
	// Through printf, which records a failed write for the dispatcher to fold
	// into the status. Writing the stream directly discarded the error, so
	// `pwd >&-` answered 0 in silence where dash says `pwd: pwd: I/O error`
	// and bash `pwd: write error: Bad file descriptor`, both with status 1.
	r.printf("%s\n", dir)
	return 0
}

// biRead reads a line into variables.
//
// Also a primitive by the same test: it has to set a variable in the *calling*
// shell, which a child process cannot reach.
//
// Without -r a backslash escapes the character after it, including a newline,
// which is why -r is what scripts should use and rarely do.
func biRead(r *Runner, ctx context.Context, args []string) int {
	// Every leading `-` word was skipped here, whatever it was — so
	// `read -s v` echoed what was meant to be hidden and `read -n 1 v` read
	// a whole line, both without a word said. All four refuse an option they
	// do not have, and the ones they *do* have and this shell does not are
	// named as missing rather than as unknown.
	//
	// Through the shared reader, because options bundle: `read -ra arr` is
	// `-r -a arr` in every shell, and reading whole words here refused the
	// bundle wholesale — `-ra is not implemented yet` — with the letter this
	// builtin does implement inside it (#347). The letters are the
	// dialect's: `n` takes a count in the two shells that count and is a
	// flag in the one where it decorates completion, so even the shape of
	// the optstring is an answer rather than a constant.
	letters := r.sem().ReadOptions
	if letters == "" {
		letters = "r"
	}
	args, opts, optArg, _, code := r.builtinOptionsArg("read", args, letters)
	if code != 0 {
		return code
	}
	raw := strings.Contains(opts, "r")

	// Whether the name being filled is the shell's own REPLY rather than one
	// the script wrote, and whether a freeze refused a write the builtin went
	// on past. Both are read by readFill and by the status below.
	defaulted := false
	refused := false

	// The prompt operand: `read "v?Name: "` reads into v and writes `Name: `
	// where there is a terminal to write it to. Two of the six spell it, it
	// is the *first* operand alone — `read v "w?p"` is a bad name `w?p` in
	// both of them — and it has to be settled here, in front of everything
	// that looks at a name, because the word a shell judges is the part before
	// the `?`. Before the array letter takes its own operand too: `read -A
	// "arr?p"` prompts and fills arr in both shells that have the form.
	//
	// Asked only when there is a `?` to split at. The axis decides nothing
	// for `read v`, and an axis reported where it decides nothing is a
	// refusal a script cannot act on.
	operandPrompt, prompted := "", false
	if len(args) > 0 && strings.Contains(args[0], "?") {
		style := r.sem().ReadPromptOperand
		switch style {
		case ReadOperandIsAllName:
		case ReadPromptNeedsANameBeforeIt, ReadPromptAloneNamesTheDefault:
			name, text, _ := strings.Cut(args[0], "?")
			operandPrompt, prompted = text, true
			if name == "" && style == ReadPromptAloneNamesTheDefault {
				// Nothing in front of the `?` names the default, which is
				// the shape the idiom is usually written in. Dropping the
				// operand is what reaches it: the bare-`read` rule below is
				// the one that knows what the default is called.
				args = args[1:]
				break
			}
			// A fresh slice: builtinOptionsArg hands back a view of the
			// caller's words, and writing the split name into element zero
			// of that would rewrite the command line the trace prints.
			args = append([]string{name}, args[1:]...)
		default:
			r.diagf("%s\n", r.unanswered("what a `?` in `read`'s first operand means"))
			r.status = 2
			r.unspecified = true
			return 2
		}
	}

	// -s is parsed and deliberately does nothing more: silence is about a
	// terminal's echo, and this runner never echoes what it reads. Parsing
	// it is the point — measured, `printf x | read -s v` reads x and prints
	// nothing in every shell that has the letter, terminal or none, and
	// skipping the word instead once let a password echo (#321).

	// -i is parsed and does nothing, which is not the same as ignoring it.
	// It is the text a *line editor* opens with, so it has an effect only
	// where there is a terminal and an editor on it — and this runner's
	// `read` never opens one, because the letter that would (`-e`) is
	// refused by name as unimplemented. bash answers the same way wherever
	// its own input is not a terminal: `printf x | read -i pre -r l` sets l
	// to `x`, seed and all. The reading that would be wrong is treating the
	// seed as a *default* for an empty line — `printf '\n' | read -i pre l`
	// leaves l empty in bash, not `pre`, and in no shell in the panel does
	// it do otherwise (#761). The letter still consumes its argument, which
	// is the half that has to be right either way: without it `read -i pre
	// -r l` would read `pre` as the variable name.

	// -p rides the optstring's shape the way -n does: `p:` takes a prompt,
	// handled once the stream is known, and a bare `p` names the coprocess
	// as the source. With one running the letter reads from its near end;
	// with none it is the measured refusal — the dialect's words, status 1,
	// and the variables untouched, which is the part that separates this
	// from a read that reached its input and failed.
	coprocSource := -1
	if _, ok := optArg['p']; !ok && strings.Contains(opts, "p") {
		fd, running := r.CoprocRead()
		if !running {
			r.diagf("%s\n", Wording(r.diag().ReadNoCoprocess, "read: -p: no coprocess"))
			return 1
		}
		coprocSource = fd
	}

	// The stream: standard input, or the descriptor -u names — resolved
	// against the shell's own table, where `exec 5<file` put it.
	in := r.In()
	if coprocSource >= 0 {
		if rd, open := r.readerForFd(coprocSource); open {
			// Wrapped so the end of the coprocess's output is noticed where
			// it happens. One dialect keeps the read end past the reaping and
			// lets go of it exactly here — see
			// Semantics.ReapedCoprocessEnds — and end-of-file is not a state
			// the descriptor is in, only something a read came back with.
			in = &coprocReader{Reader: rd, r: r}
		}
	}
	if word, ok := optArg['u']; ok {
		fd, numeric := atoi(word)
		if !numeric || fd < 0 {
			return r.readBadNumberFor(r.diag().ReadBadDescriptorSpec, word)
		}
		rd, open := r.readerForFd(fd)
		if !open {
			// Three answers with one status: bash and ksh93 complain in
			// their own words and zsh says nothing, so an empty wording is
			// the dialect's answer rather than a gap to paper over.
			if w := r.diag().ReadBadFileDescriptor; w != "" {
				// Through Wording for the format handling alone — the
				// fallback is never reached, because an empty wording is an
				// answer here: one shell reports this failure in silence.
				r.diagf("%s\n", Wording(w, w, word))
			}
			return 1
		}
		in = rd
	}

	// The first operand, judged before the stream is *read* in the dialects
	// that judge it there. Observable only through the input — a refusal that
	// comes first leaves the line for the next reader — which is why it is an
	// axis rather than a placement this file could pick.
	//
	// Asked in this order, and only here: an operand that *is* a name is
	// filled the same way whichever answer the dialect gives, so the two
	// orders differ over a bad name and nowhere else.
	//
	// Behind the option handling rather than in front of it, because an
	// option's complaint comes first: ksh93 spells `-p` as the coprocess
	// flag and answers `read -p 'PROMPT ' v` with `read: no query
	// process`, not with a complaint about the operand `PROMPT ` its own
	// option left standing. In front of the prompt, so that nothing is
	// written to a terminal for a read that is not going to happen.
	if len(args) > 0 && !r.isReadName(args[0]) {
		if r.unspecified {
			return 2
		}
		if r.ask(r.sem().ReadRefusesABadNameBeforeReading,
			"`read` judging its first operand before it reads") {
			return r.badReadName(args[0])
		}
		if r.unspecified {
			return 2
		}
	}

	// The prompt: written to standard error, no newline, and only when the
	// stream being read is a terminal — both measured in both shells whose
	// -p takes an argument, and the second half from both sides: a pipe gets
	// no prompt, and neither does a -u descriptor on a file while the
	// terminal sits untouched on standard input. Raw rather than through the
	// diagnostic path, because a prompt carries no location in any shell.
	if prompt, ok := optArg['p']; ok && inputIsTerminal(in) {
		r.errf("%s", prompt)
	}
	// The operand's prompt is written on the same terms as the option's, and
	// measured the same way: `printf 'x\n' | zsh -c 'read "v?p"'` writes
	// nothing and still reads into v, so the split is unconditional and only
	// the writing is for a terminal.
	if prompted && inputIsTerminal(in) {
		r.errf("%s", operandPrompt)
	}

	// The delimiter: a newline unless -d renamed it. The argument's first
	// character speaks — `read -d xy` stops at the x in every shell with
	// the letter — and an empty argument means NUL in the two measured
	// saying so. (ksh93 reads through a NUL instead; seeing that difference
	// takes a NUL in the input, which is also what it takes to care.)
	delim := byte('\n')
	if word, ok := optArg['d']; ok {
		delim = 0
		if word != "" {
			delim = word[0]
		}
	}

	// The counts: -n reads at most N characters, still stopping at the
	// delimiter; -N reads exactly N, the delimiter ordinary, backslash
	// ordinary, and the text handed over whole rather than split.
	count, exact := -1, false
	if word, ok := optArg['n']; ok {
		n, numeric := atoi(word)
		if !numeric || n < 0 {
			return r.readBadNumberFor(r.diag().ReadBadCount, word)
		}
		count = n
	}
	if word, ok := optArg['N']; ok {
		n, numeric := atoi(word)
		if !numeric || n < 0 {
			return r.readBadNumberFor(r.diag().ReadBadCount, word)
		}
		count, exact = n, true
	}

	// -k is the third count and is not one of these two: it reads characters
	// from the *terminal*, nothing is a terminator, and one name is filled
	// with the lot. readkeys.go holds the measurements and the reading; what
	// is here is where it joins the rest of the builtin.
	keys, readsKeys := readKeyCount(opts, optArg)
	if readsKeys && keys < 0 {
		// Its own wording, measured: `read -k2v x` is `number expected after
		// -k: 2v` where a bad `-t` is the dialect's ReadBadNumber. The
		// attached form is the only way to reach it — a *word* that is not a
		// number was never the argument, it is the name to read into.
		r.diagf("%s\n", Wording(r.diag().ReadBadOptionNumber,
			"read: number expected after -%[1]s: %[2]s", "k", optArg['k']))
		return 1
	}

	// The array: bash's -a names it in the option's argument and ignores
	// any operands after it; ksh93 and zsh spell it -A and take the name as
	// the first operand, clearing the names that follow. The letters
	// differ, so the behaviors can ride them without an axis.
	//
	// named says an array was asked for at all, which the name cannot say:
	// `read -a ""` names an empty one, and bash 5.3.20 and 3.2.57 refuse it
	// as `` `': not a valid identifier `` at 1 where reading it as no array
	// filled REPLY at 0.
	array, named, letterA := "", false, false
	if name, ok := optArg['a']; ok {
		array, named, letterA = name, true, true
	}
	if strings.Contains(opts, "A") {
		array, named = "REPLY", true
		if len(args) > 0 {
			array, args = args[0], args[1:]
		}
	}

	// The timeout, if the dialect has the letter: seconds, fractions
	// allowed. Zero and expiry land in the same place — nothing read in the
	// time given — and what that reports is the dialect's number.
	timeout, timed := time.Duration(-1), false
	if word, ok := optArg['t']; ok {
		secs, err := strconv.ParseFloat(word, 64)
		if err != nil {
			// Not a written number. One column reads the argument as an
			// *expression* instead — the same shape UlimitOperandIsArithmetic
			// records at the other builtin — so `read -t abc` there is an
			// unset name, which is nought, and times out at once in silence.
			// The axis is asked here and not in front of the plain reading,
			// so `read -t 3` never puts the question.
			n, ok := r.readTimeoutArithmetic(word)
			if r.unspecified {
				return r.status
			}
			if !ok {
				return r.readBadNumberFor(r.diag().ReadBadTimeout, word)
			}
			secs = n
		}
		if secs < 0 {
			return r.readBadNumberFor(r.diag().ReadBadTimeout, word)
		}
		timeout, timed = time.Duration(secs*float64(time.Second)), true
	}

	// The terminal, for -k, and before the byte source is built so that -t
	// still bounds it — measured, `read -k -t 1` inside a widget waits a
	// second for a keystroke and reports 1 when none comes. A source -u or -p
	// already named is left alone: it may not be a terminal, and `read -k 2
	// -u 3 v` on a file reads two bytes of the file.
	keyTerminal := (*os.File)(nil)
	if readsKeys {
		explicit := io.Reader(nil)
		if _, named := optArg['u']; named || coprocSource >= 0 {
			explicit = in
		}
		src, restore, held := r.readKeySource(explicit)
		defer restore()
		if !held {
			return r.readNoTerminal()
		}
		in = src
		if explicit == nil {
			// The terminal, as a file. Kept so the timed read below can wait
			// for readability instead of parking a read on it — see
			// pollingKeySource, and why that distinction is load-bearing here
			// and nowhere else.
			keyTerminal, _ = src.(*os.File)
		}
	}

	next := directByteSource(in)
	switch {
	case timed && timeout == 0:
		// A timeout of zero is a question about the stream rather than a
		// deadline that has already passed, and the panel answers it three
		// ways — see ReadZeroTimeoutStyle.
		if src, reads := r.zeroTimeoutSource(in); reads {
			next = src
			break
		}
		if r.sem().ReadZeroTimeout != ReadZeroTimeoutPolls {
			r.diagf("%s\n", r.unanswered("what `read -t 0` asks of the stream"))
			r.status = 2
			r.unspecified = true
			return 2
		}
		// The status is the whole answer: nothing is read and no name is
		// touched, in either direction.
		if inputWaiting(in) {
			return 0
		}
		return 1
	case timed:
		// A deadline bounds the wait but does not remove it, and a shell
		// that started this job is blocked for the whole of it.
		r.settleBackgroundJobBeforeABlockingRead(in)
		var stop func()
		whole := !r.ask(r.sem().ReadTimeoutBoundsReadability,
			"`read -t` bounding the wait for the first byte rather than the whole read")
		if r.unspecified {
			return 2
		}
		// `read -k` on the terminal waits for the bytes rather than parking a
		// read on them, because the stream it would park on is the line
		// editor's own input and an abandoned read there swallows the next
		// key somebody presses. Every other read keeps the shared machinery,
		// whose cost is confined to the pipe it was used on.
		if polled, can := keyTimedSource(keyTerminal, timeout, whole); can {
			next = polled
			break
		}
		next, stop = r.timedByteSource(ctx, in, timeout, whole)
		defer stop()
	default:
		// The read that never returns, which is the one a coprocess makes
		// by construction.
		r.settleBackgroundJobBeforeABlockingRead(in)
	}
	if readsKeys {
		return r.readKeysInto(next, keys, args)
	}
	text, lits, end := readSegment(next, raw, delim, count, exact)

	// A `read` that fails still assigns. All four shells clear the variables
	// at end of input rather than leaving what was there, and the reason is
	// the loop everyone writes: `while read -r l` leaves `l` behind, and a
	// stale value after the loop reads as the last line rather than as
	// nothing. Assigning happens before the status is decided, not instead
	// of it.
	status := 0
	switch end {
	case endTimeout:
		status = orDefault(r.diag().ReadTimeoutStatus, 1)
		// A deadline is not an end of input, and two of the three shells with
		// the letter treat it as no read at all: no name is touched, not even
		// cleared, so whatever the variable held survives. The third assigns
		// the short read — which looks like clearing only because the usual
		// way to reach a timeout is with nothing having arrived.
		if !r.ask(r.sem().ReadTimeoutKeepsWhatArrived,
			"an expired `read -t` assigning what did arrive") {
			if r.unspecified {
				return 2
			}
			return status
		}
	case endEOF:
		status = 1
		switch {
		case exact:
			// Both shells with the letter report the short read; whether
			// the partial text survives into the variable splits them.
			if text != "" && !r.ask(r.sem().ReadExactCountKeepsPartial,
				"a short `read -N` keeping what did arrive") {
				text = ""
			}
		case count >= 0 && text != "":
			// A full count and a wholly empty input answer the same way
			// everywhere; only the partial fill asks.
			if r.ask(r.sem().ReadPartialCountSucceeds,
				"a short `read -n` counting as a success") {
				status = 0
			}
		}
	}

	if len(args) == 0 && array == "" {
		// Three shells set REPLY; dash wants a variable name and says so.
		if r.ask(r.sem().ReadRequiresAVariableName, "a bare `read` with no variable") {
			r.diagf("%s\n", Wording(r.diag().ReadArgCount, "read: arg count"))
			return 2
		}
		if r.unspecified {
			return 2
		}
		args = []string{"REPLY"}
		// Not a name the script wrote, which one status rule turns on: see
		// Runner.readFrozenStatus.
		defaulted = true
	}
	// The names, judged in the order they are filled. The array goes first
	// because it is filled first: `read -a 1bad` in bash refuses and leaves
	// the array untouched, while ksh93's `read -A a 1bad` fills a and *then*
	// refuses the operand after it.
	// The `-a` array's name is a **plain** name, where an ordinary operand may
	// carry a subscript — `read B[1]` writes one element and is right. Judged
	// on the letter rather than through an axis, for the reason above: the
	// two spellings are two shells' and they answer this differently.
	//
	// Measured 2026-09-17 from script files under `env -i`:
	//
	//	read -a 'A[0]' <<< "x y"     bash 5.3.20  `A[0]': not a valid
	//	                             identifier, 1, no A afterwards
	//	read -A 'A[0]' <<< "x y"     ksh93u+      0, and A is (x y) — the
	//	                             subscript dropped
	//
	// Here the bash spelling answered 0 and made a parameter *called* `A[0]`
	// holding the words, so `declare -p A` said the array the script meant
	// was not found. `-A` is untouched and keeps ksh93's answer.
	//
	// Through a reference first, if the name is one, so the judgement is on
	// what the fill would reach: `declare -n e=XXX[0]; read -a e` is the same
	// refusal in bash, and here it made `XXX[0]` the same way. That is the
	// route mapfile already takes — see mapfileTarget and #3478, where this
	// shape was closed for `mapfile`, `readarray` and `unset` and `read` was
	// the writer left out. A reference aimed at a plain name fills the target
	// and always did.
	if letterA {
		if aimed, is := r.namerefTarget(array); is {
			array = aimed
		}
		if !isPlainName(array) {
			if r.unspecified {
				return 2
			}
			return r.badReadName(array)
		}
	}
	if named && !r.isReadName(array) {
		if r.unspecified {
			return 2
		}
		return r.badReadName(array)
	}
	// A bad operand stops the filling at itself and is reported after it,
	// which is what leaves the names in front of it set and the ones behind
	// it as they were. The list keeps its full length here on purpose: the
	// last name takes the remainder of the line only when the line held more
	// fields than there are *names*, and that count is the one the script
	// wrote — `printf 'X Y Z\n' | { read a 1bad c; }` gives a=X in all six
	// shells, not the whole line.
	// Only the -A spelling touches the operands after the array: it took its
	// name from among them and clears the rest, where bash's -a leaves the
	// names after its argument exactly as they were — both measured with
	// `x=keep`.
	clearRest := strings.Contains(opts, "A")
	// Which operands are judged at all, which rides the array letter rather
	// than an axis because the letters differ and no shell has both.
	//
	// bash's `-a` takes the array's name in the option's argument and then
	// **ignores the operands after it entirely**: measured 2026-09-07,
	// `read -a arr good 1bad` answers 0 with the array filled and no word
	// said about `1bad`, and `b=keep; read -a arr b` leaves b as keep. Only
	// the first operand is still judged, by the check in front of the read —
	// `read -a arr 1bad` is `not a valid identifier` there. ksh93's `-A`
	// takes its name from among the operands and judges the rest: the same
	// line spelled `-A` refuses `1bad` with the array filled behind it.
	//
	// So a walk over the operands here would refuse a line bash accepts.
	judgesTheOperands := array == "" || clearRest
	fill, badName, bad := len(args), "", false
	for i, name := range args {
		if !judgesTheOperands || r.isReadName(name) {
			continue
		}
		if r.unspecified {
			return 2
		}
		// A count changes who is judged past the first name: bash carries
		// on and ksh93 stops. Asked here rather than in front of the walk,
		// so that it is asked only when there is a bad name past the first
		// for it to decide about — the two dialects that have no count that
		// reaches this leave it unanswered.
		// `i > 0 || array != ""` rather than `i > 0`: with an array the
		// first name filled is the array's, so every operand beside it is
		// already past the first. Measured — `read -A -N 3 arr 1bad b` is
		// quiet in the shell a count releases, where `read -A arr 1bad b`
		// is not.
		if (i > 0 || array != "") && count >= 0 && !r.ask(r.sem().ReadCountJudgesTheNamesAfterTheFirst,
			"`read` with a count judging the names after the first") {
			if r.unspecified {
				return 2
			}
			// The filling still stops here, and only the complaint is
			// withheld: measured, `b=keep; read -n 3 a 1bad b` leaves b as
			// keep in the shell that says nothing, so the bad name ends the
			// list there as surely as it does where it is reported.
			fill = i
			break
		}
		fill, badName, bad = i, name, true
		break
	}
	if exact {
		// -N hands the text over whole: `read -N 5 x y` on `a b c` puts all
		// five characters in x and nothing in y, measured in both shells
		// with the letter.
		if array != "" {
			if !r.readMayWrite(array) {
				if r.ctl == controlExit {
					return r.status
				}
				if r.readRefusalEndsTheBuiltin() {
					return r.readFrozenStatus(len(args[:fill]), !defaulted)
				}
				refused = true
			} else {
				r.setArray(array, exactElems(text))
			}
			if clearRest {
				for i, name := range args[:fill] {
					if st, stop := r.readFill(name, "", fill-i-1, defaulted, &refused); stop {
						return st
					}
				}
			}
			return r.readRefusedOrStatus(refused, status, badName, bad)
		}
		for i, name := range args[:fill] {
			v := ""
			if i == 0 {
				v = text
			}
			if st, stop := r.readFill(name, v, fill-i-1, defaulted, &refused); stop {
				return st
			}
		}
		return r.readRefusedOrStatus(refused, status, badName, bad)
	}
	// Splitting sees the escapes: an escaped separator is data and does not
	// split, which is why the mask rides along rather than the processing
	// being a pre-pass over the string.
	ifs, set := r.ifs()
	fields, at, _ := splitFieldsAt(text, lits, ifs, set, false, false)
	if array != "" {
		// An array target takes the fields *as* fields, so the tail of the
		// splitting rule is live here: `IFS=:; read -A a` on `a:b:` fills
		// three elements in the shell where a trailing separator delimits and
		// two in the rest of the panel. The mask goes with it, because an
		// escaped separator is data — `a\:` is one element `a:` in both
		// readings.
		fields = r.readFieldsTail(fields, text, lits, ifs, set)
		if len(fields) == 0 && r.ask(r.sem().ReadNoFieldsIsOneEmptyElement,
			"`read` into an array leaving one empty element where the line had no fields") {
			// A line that splits into nothing at all: two of the three shells
			// with the letter leave one empty element and bash leaves none.
			// After the tail above rather than before it, because the shell
			// that opens a field on a closing whitespace run has a field by
			// then and is not asking this — measured, `read -A r` on a line
			// of spaces is one element there and not two.
			fields = []string{""}
		}
		if r.unspecified {
			return r.status
		}
		if !r.readMayWrite(array) {
			if r.ctl == controlExit {
				return r.status
			}
			if r.readRefusalEndsTheBuiltin() {
				return r.readFrozenStatus(len(args[:fill]), !defaulted)
			}
			refused = true
		} else {
			r.setArray(array, fields)
		}
		if clearRest {
			for i, name := range args[:fill] {
				if st, stop := r.readFill(name, "", fill-i-1, defaulted, &refused); stop {
					return st
				}
			}
		}
		return r.readRefusedOrStatus(refused, status, badName, bad)
	}
	// The last name takes the remainder of the *line* from where its own
	// field began — the text as it was read, separators and all. Rebuilding
	// it from the fields and joining them on a hard space is the wrong
	// answer all six shells disagree with, and a silent one: `IFS=: read -r
	// user rest` on a passwd line gave the right number of words with every
	// colon replaced by a space, at status 0 (#1208). Only the closing run of
	// IFS *whitespace* comes off it, which is why the offset rather than the
	// field is what this needs.
	//
	// It is a remainder only where the line held more fields than there were
	// names. One field per name means the last name takes its own field, so
	// `IFS=: read x y` on `a:b:` gives `b` and not `b:` — and that is where
	// the tail of the splitting rule becomes visible through `read`'s names:
	// the shell that opens a field on a trailing separator has one field
	// more than there are names, reaches the remainder, and keeps the colon
	// the other five absorb.
	//
	// Asked at that count and nowhere else on this path, which is the whole
	// of where the two readings differ. Past the names the extra empty field
	// changes no value — the remainder is the same text either way — and
	// short of them the name it would fill is the empty string it was going
	// to be given anyway.
	if len(fields) == len(args) {
		// trailingSeparatorField and not readFieldsTail: `read`'s own
		// question about a closing *whitespace* run is not asked here,
		// because nothing on this path can answer differently for it.
		// readRemainder takes the closing run of IFS whitespace off the
		// remainder anyway, so `read x` on `a  ` is `a` and `read x y` on
		// `a b  ` is `a` and `b` under either answer — and an axis reported
		// where it decides nothing is a refusal a script cannot act on.
		fields = r.trailingSeparatorField(fields, text, lits, ifs, set, false)
	}
	// `at` is the splitter's own list and is one short of `fields` exactly
	// when the answer above added one. The short entry is never the one read:
	// an offset is read only for the name at len(args)-1, and the field that
	// was added sits at len(args). Reaching the remainder at all means either
	// the splitter found more fields than there are names — so `at` is longer
	// than `args` — or the tail answer carried a count that was equal, which
	// leaves `at` exactly as long.
	for i, name := range args[:fill] {
		var v string
		switch {
		case i >= len(fields):
			v = ""
		case i == len(args)-1 && len(fields) > len(args):
			v = r.readRemainderValue(text, at, fields, i, ifs)
		case i == len(args)-1:
			v = r.readLastFieldValue(fields[i], ifs)
		default:
			v = fields[i]
		}
		if st, stop := r.readFill(name, v, fill-i-1, defaulted, &refused); stop {
			return st
		}
	}
	return r.readRefusedOrStatus(refused, status, badName, bad)
}

// readRefusedOrStatus folds a refusal the builtin went on past into the
// status it reports.
//
// The one column that carries on reports 1 for it, whatever the read itself
// did, and reports it once however many names were frozen — measured
// 2026-09-16 on ksh93u+ with two frozen names, which writes two sentences and
// one status. A bad name is reported ahead of it, being the older complaint
// and the one that already decides its own status.
func (r *Runner) readRefusedOrStatus(refused bool, status int, badName string, bad bool) int {
	if st := r.readAfterABadName(status, badName, bad); bad || !refused {
		return st
	}
	return 1
}

// readFill writes one of `read`'s names, reporting the status to return where
// a freeze stopped the builtin there.
//
// left is how many names are still to come, which decides the status in the
// one column that parts "I stopped early" from "a write failed"; written says
// the name is one the script wrote rather than the shell's own REPLY.
func (r *Runner) readFill(name, value string, left int, defaulted bool, refused *bool) (int, bool) {
	if r.readMayWrite(name) {
		if st, refused := r.storeThroughOperand(name, value); refused {
			// The subscript would not evaluate, so nothing was written and
			// the builtin gives up at that name: the ones behind it keep
			// what they held. Measured 2026-09-17, `r=(1 2 3); b=preset;
			// read 'r[1/0]' b <<< "x y"` is status 1 with `b` still `preset`
			// in bash 5.3.20 and ksh93u+ alike.
			return st, true
		}
		return 0, false
	}
	if r.ctl == controlExit {
		// The dialect whose refusal ends the script has already said so.
		return r.status, true
	}
	if !r.readRefusalEndsTheBuiltin() {
		// ksh93: every frozen name is reported and every other name is
		// filled, and the builtin reports 1 at the end of it. Recorded
		// rather than returned, because the loop goes on.
		*refused = true
		return 0, false
	}
	return r.readFrozenStatus(left, !defaulted), true
}

// readRemainder is the value the last name on a `read` takes when the line
// held more fields than there were names: the text from that field's start to
// the end of the line.
//
// What comes off the end is the closing run of IFS whitespace and nothing
// else. A closing non-whitespace separator stays — `IFS=: read x y` on
// `a:b:c::` gives `b:c::` in all six shells — and a whitespace character that
// is not in IFS stays too, which is why the mask and IFS both have to be
// consulted rather than unicode.IsSpace. The escape mask is honored on the
// same reasoning the splitter honors it: a backslashed space is data.
func readRemainder(text string, start int, literal []bool, ifs string) string {
	end := len(text)
	for end > start {
		i := end - 1
		if literal != nil && literal[i] {
			break
		}
		c := text[i]
		if c != ' ' && c != '\t' && c != '\n' {
			break
		}
		if strings.IndexByte(ifs, c) < 0 {
			break
		}
		end--
	}
	return text[start:end]
}

// readRemainderValue is the remainder with Semantics.ReadTrailingEscapedSeparator
// applied, for the last name on a `read` that took the rest of the line.
//
// Two readings, and both are written out here because the axis is read only
// where they differ:
//
//   - the trim ignores the escape mask, so the closing run of IFS whitespace
//     comes off whether it was escaped or not. bash, ksh93 and zsh.
//   - the remainder ends where the **last field with content of its own**
//     ends, so a field's escaped trailing space is part of the field and
//     survives, while a field made of nothing but escaped separators does
//     not. dash.
//
// The second is measured rather than reasoned, and two rows settle its shape.
// `a b c\ ` leaves `b c ` there — the escaped space belongs to the field `c`
// closes — while `a b \ ` leaves `b`, where the escaped space is a field of
// its own and goes. A rule written about the character before it instead
// would answer `x b ` for `a x b\ \ `, which dash answers `x b  `.
//
// This implementation had neither: it honored the mask character by
// character, which is dash's answer on the first row and nobody's on the
// second (#1360).
func (r *Runner) readRemainderValue(text string, at []int, fields []string, i int, ifs string) string {
	start := at[i]
	bare := readRemainder(text, start, nil, ifs)
	// The end can only ever move *forward* from the plain trim, because the
	// only thing either reading declines to take off is whitespace the other
	// one took: a non-whitespace separator is not trimmed by anybody, and a
	// field's own content is past the plain trim's stop by definition.
	end := start + len(bare)
	for j := i; j < len(fields) && j < len(at); j++ {
		if allSeparatorWhitespace(fields[j], ifs) {
			continue
		}
		if e := at[j] + len(fields[j]); e > end {
			end = e
		}
	}
	kept := text[start:end]
	if kept == bare {
		return bare
	}
	// Both trimming answers take the plain end here and part on the
	// single-field value below. An unanswered axis is reported and then
	// leaves the value where it found it.
	if r.readTrailingEscapedSeparator() == ReadTrailingEscapedSeparatorTrimmedFromARemainder ||
		r.sem().ReadTrailingEscapedSeparator == ReadTrailingEscapedSeparatorTrimmed {
		return bare
	}
	return kept
}

// allSeparatorWhitespace reports whether every byte of a field is an IFS
// whitespace character — which, a field's content being what the splitter did
// *not* treat as a separator, means every byte of it was escaped.
//
// An empty field counts, and that is the answer the remainder wants: the
// empty fields two adjacent non-whitespace separators leave are not content
// and must not hold the end open. `IFS=: read x y` on `a:b:c::` is `b:c::` in
// all six shells, and it stays that way because the plain trim never takes a
// colon — not because an empty field extended it.
func allSeparatorWhitespace(field, ifs string) bool {
	for i := range len(field) {
		c := field[i]
		if c != ' ' && c != '\t' && c != '\n' {
			return false
		}
		if strings.IndexByte(ifs, c) < 0 {
			return false
		}
	}
	return true
}

// readLastFieldValue is the last name's value where the line held exactly one
// field per name, so there was no remainder to take.
//
// A field can only end in IFS whitespace that was escaped — an unescaped one
// is what closed the field and the splitter kept it out — so the trim below
// differing from the field at all *is* the disagreement, and is where the
// axis is read. One column trims here and the other two leave the field
// alone, which is the row that made the axis three-valued.
func (r *Runner) readLastFieldValue(field, ifs string) string {
	trimmed := readRemainder(field, 0, nil, ifs)
	if trimmed == field {
		return field
	}
	if r.readTrailingEscapedSeparator() == ReadTrailingEscapedSeparatorTrimmed {
		return trimmed
	}
	return field
}

// readTimeoutArithmetic is `read -t` given a word that is not a written
// number, in the one dialect that reads the argument as an expression.
//
// An unset name is nought there, which is the whole of what the issue's case
// comes to: `read -t abc` is a timeout of nought and times out at once,
// silently. A word the expression grammar cannot read at all — `3abc` — is
// refused, and the caller words it.
//
// Not ulimitArithmetic's reading, and the difference is measured: that one
// refuses an expression naming a parameter that is not set, and this one
// evaluates it to nought.
func (r *Runner) readTimeoutArithmetic(word string) (float64, bool) {
	if !r.ask(r.sem().ReadTimeoutOperandIsArithmetic, "`read -t` reading its argument as an expression") {
		return 0, false
	}
	tree, err := r.arithTree(nil, word)
	if err != nil {
		return 0, false
	}
	n, err := r.evalArith(tree)
	if err != nil {
		return 0, false
	}
	return float64(n), true
}

// readBadNumberFor is a count, timeout or descriptor argument that is not a
// number, worded after what the number was *for* — see
// Diagnostics.ReadBadTimeout and its two siblings. An empty wording is the
// shared sentence, which is what a dialect that words all three alike leaves
// behind.
//
// Every letter that reaches it passes one now: `-u`, `-n`, `-N` and `-t` each
// have a field, since the one column that words all three separately needed
// the third (#3367). The wrapper that passed no wording went with it — four
// call sites and none of them wanted the fallback.
func (r *Runner) readBadNumberFor(wording, word string) int {
	if wording == "" {
		wording = r.diag().ReadBadNumber
	}
	r.diagf("%s\n", Wording(wording, "read: %[1]s: invalid number", word))
	return orDefault(r.diag().ReadBadNumberStatus, 1)
}

// readerForFd is the stream `read -u` names: standard input by its number,
// anything past the named three from the shell's own table — which holds
// what a redirection resolved to, exactly the "copied as it is now" a real
// descriptor duplication means. A number nothing is open at, or one held by
// something that cannot be read, is not a stream.
func (r *Runner) readerForFd(fd int) (io.Reader, bool) {
	if fd == 0 {
		return r.stdin(), true
	}
	if v, held := r.fds[fd]; held {
		rd, ok := v.(io.Reader)
		return rd, ok
	}
	return nil, false
}

// exactElems is what `read -N` gives an array: the raw text as one element,
// or none at all when nothing survived.
func exactElems(text string) []string {
	if text == "" {
		return nil
	}
	return []string{text}
}

// How a readSegment ended: at the delimiter, at a satisfied count, at the
// end of the input, or out of time. Only the last two mark a failure, and
// they mark different ones.
const (
	endDelim = iota
	endCount
	endEOF
	endTimeout
)

// A byte and how it arrived. evEOF and evTimeout carry no byte.
const (
	evByte = iota
	evEOF
	evTimeout
)

// directByteSource reads the stream a byte at a time, in the calling
// goroutine — the path every read without a deadline takes.
func directByteSource(in io.Reader) func() (byte, int) {
	var ch [1]byte
	return func() (byte, int) {
		n, err := in.Read(ch[:])
		if n == 0 || err != nil {
			return 0, evEOF
		}
		return ch[0], evByte
	}
}

// timedByteSource reads the stream a byte at a time until the deadline. The
// reads happen on their own goroutine — an io.Reader cannot be told to stop
// waiting — and each is made only when asked for, so a satisfied read never
// reads ahead into input a later `read` should get. A read still in flight
// when the deadline passes is abandoned; if its byte ever arrives it is
// lost, which is the cost of a timeout over a plain pipe and is confined to
// the stream the timeout was used on. stop releases the goroutine and must
// be called once the segment is read.
//
// bounded says the deadline covers the *whole* read. Where it does not, the
// timer is dropped as soon as the first byte arrives and everything after it
// is read without one — see Semantics.ReadTimeoutBoundsReadability, which is
// what decides which of the two this is.
func (r *Runner) timedByteSource(ctx context.Context, in io.Reader, timeout time.Duration, whole bool) (next func() (byte, int), stop func()) {
	if timeout <= 0 {
		// No time at all, which is out of time before the first byte. A
		// timeout that is *written* as zero never reaches here — it is a
		// question about the stream rather than a deadline, and is answered
		// by ReadZeroTimeout — so this is the guard and not the behavior.
		return func() (byte, int) { return 0, evTimeout }, func() {}
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	type event struct {
		b   byte
		eof bool
	}
	req := make(chan struct{})
	// Buffered by one, so a byte or an end-of-file arriving after the
	// deadline parks in the channel instead of parking the goroutine: the
	// sender loops back to a closed req and exits.
	resp := make(chan event, 1)
	go func() {
		var ch [1]byte
		for range req {
			n, err := in.Read(ch[:])
			if n == 0 || err != nil {
				resp <- event{eof: true}
				return
			}
			resp <- event{b: ch[0]}
		}
	}()
	done, arrived := false, false
	next = func() (byte, int) {
		if done {
			return 0, evEOF
		}
		req <- struct{}{}
		if arrived && !whole {
			// The deadline bounded the wait for the stream to become
			// readable and nothing after it, so this read waits as long as
			// it has to. A byte dripping every 0.1s under `-t 0.25` gives
			// the whole line and status 0 in the shell that reads this way,
			// where a whole-read deadline gives 1 and a partial line (#644).
			ev := <-resp
			if ev.eof {
				done = true
				return 0, evEOF
			}
			return ev.b, evByte
		}
		select {
		case ev := <-resp:
			if ev.eof {
				done = true
				return 0, evEOF
			}
			arrived = true
			return ev.b, evByte
		case <-tctx.Done():
			done = true
			return 0, evTimeout
		}
	}
	stop = func() {
		cancel()
		close(req)
	}
	return next, stop
}

// readSegment reads until the delimiter, the count, or the end of the input,
// honoring the backslash unless raw: it removes the special meaning of the
// character after it and is itself removed — so `a\tb` (a literal backslash,
// then a t) delivers `atb` — and an escaped newline vanishes whole, the line
// continuation, whatever the delimiter is. An escaped delimiter is data with
// the backslash dropped, and a backslash the input ends on escapes nothing
// and is dropped too; both measured, unanimous (docs/spec/semantics.md,
// "read's options are the dialect's letters"). With exact set the count is
// the only stop: the delimiter and the backslash are ordinary bytes, which
// is what makes `read -N` the way to take input exactly as it came.
//
// literal marks the delivered bytes a backslash escaped, aligned with text,
// nil when there were none. The caller's field splitter needs it because an
// escaped separator does not split — by the time the escapes are gone, an
// escaped space and a separating one are the same byte, so a mask has to
// carry what the string no longer can.
//
// count limits the characters as delivered, after an escape or a
// continuation has folded its backslash away; negative means unlimited.
func readSegment(next func() (byte, int), raw bool, delim byte, count int, exact bool) (text string, literal []bool, end int) {
	var b strings.Builder
	var escapedAt []int // offsets in b whose byte arrived behind a backslash
	pending := false    // a backslash read, its character not yet
	for {
		if count >= 0 && b.Len() >= count {
			return b.String(), literalMask(escapedAt, b.Len()), endCount
		}
		c, ev := next()
		switch ev {
		case evEOF:
			// A pending backslash had nothing to escape and is dropped.
			return b.String(), literalMask(escapedAt, b.Len()), endEOF
		case evTimeout:
			return b.String(), literalMask(escapedAt, b.Len()), endTimeout
		}
		if exact {
			b.WriteByte(c)
			continue
		}
		if pending {
			pending = false
			if c == '\n' {
				// The continuation: backslash and newline vanish whole,
				// under -d too, where the newline is no longer the
				// delimiter. An escaped *other* delimiter is the branch
				// below — data, not a join.
				continue
			}
			// Escaped, so literal: the delimiter does not end the read
			// here, and a separator marked this way must not split.
			escapedAt = append(escapedAt, b.Len())
			b.WriteByte(c)
			continue
		}
		if !raw && c == '\\' {
			pending = true
			continue
		}
		if c == delim {
			return b.String(), literalMask(escapedAt, b.Len()), endDelim
		}
		b.WriteByte(c)
	}
}

// literalMask spreads escaped offsets into a mask aligned with the text, nil
// when nothing was escaped — the common case, kept allocation-free.
func literalMask(at []int, n int) []bool {
	if len(at) == 0 {
		return nil
	}
	m := make([]bool, n)
	for _, i := range at {
		m[i] = true
	}
	return m
}

// readLine reads one line, honoring the backslash unless raw, and says
// whether the input ended.
//
// The two are separate answers because a final line with no newline is both:
// there is a line, and there will not be another. All four shells assign it
// and report failure, which is what stops `while read -r l` from running a
// last unterminated line twice — once as the line, once as the empty read
// after it.
func (r *Runner) readLine(raw bool) (line string, atEOF bool) {
	in := r.In()
	// `select` reads here, and a `select` in the body of a background job
	// waits on its stream exactly as `read` does — see
	// settleBackgroundJobBeforeABlockingRead.
	r.settleBackgroundJobBeforeABlockingRead(in)
	text, _, end := readSegment(directByteSource(in), raw, '\n', -1, false)
	return text, end == endEOF
}

// biLocal makes variables local to the running function.
//
// Shell scoping is *dynamic*, not lexical: a local is visible to everything
// the function calls, and stops existing when the function returns. That is
// why this saves the outer value on a stack rather than creating a new
// environment — there is only ever one set of variables, and `local` says
// which of them to put back.
func biLocal(r *Runner, _ context.Context, args []string) int {
	// Asked here rather than at the prefix, so that a shell which never
	// finds this builtin through `command` is never asked — see biCommand.
	nowhere := r.localUnderCommandPrefix &&
		r.ask(r.sem().LocalThroughCommandDeclaresNothing, "`command local a=1` declaring nothing")
	if r.unspecified {
		return r.status
	}
	if len(r.scopes) == 0 && !nowhere {
		// The refusal is the declaration's, so a declaration that goes
		// nowhere has nothing to refuse: `command local a=1` outside a
		// function is silent at 0 in the two columns that drop it, where a
		// bare `local` there is `not in a function` at 2. Measured with the
		// rest of Semantics.LocalThroughCommandDeclaresNothing.
		if st, stop := r.localOutsideAFunction(); stop {
			return st
		}
	}
	// The letters are the dialect's own, and may be none at all: dash gives
	// `local` no options, so `local -r x` declares a variable named `-r`
	// there — and then refuses it as the bad name it is. The parse is
	// `typeset`'s, because in both shells that read letters here they are
	// the same letters meaning the same attributes.
	var f declareFlags
	if known := r.sem().LocalOptions; known != "" {
		rest, flags, code := r.parseDeclareFlags("local", args, known)
		if code != 0 {
			return code
		}
		args, f = rest, flags
	}
	if len(r.scopes) > 0 && (len(args) == 0 || f.print) {
		// Bare `local` is a listing, and the shells do not agree what of —
		// see BareLocalListingForm. `local -p` is the same listing spelled
		// as a letter, except when operands narrow it to named declarations.
		//
		// Narrowed to the running call's own names in the dialect that reads
		// the word that way — the *listing* is `declare -p`'s, and which
		// names are in it is this word's. See Runner.localListingSkipsAName.
		if len(args) > 0 {
			defer r.localListingIsTheRunningCallsOwn()()
			return r.declarePrint(args)
		}
		return r.bareLocalListing()
	}
	if kept, dash := localDashOperands(args); dash {
		// `local -` is the operand that is not a name — see localdash.go.
		// Taken out before the names are checked, because that check is what
		// used to refuse it: the shell answered ``local: `-': not a valid
		// identifier`` and then let every option the body set outlive the
		// call (#3000).
		if !r.ask(r.sem().LocalDashSavesTheShellOptions, "`local -` saving the shell's options") {
			if r.unspecified {
				return r.status
			}
			// The dialect that reads `-` as a parameter name rather than as
			// this request. It falls through as the operand it wrote.
		} else {
			args = kept
			r.saveOptionsForThisCall()
			if len(args) == 0 {
				return 0
			}
		}
	}
	if f.global && !nowhere {
		// `local -g` is not a local declaration at all: the letter says the
		// name belongs to the shell, so every step below — the shadow, the
		// scope's saved export, the freeze that comes off at the return — is
		// about a binding this line does not make. So it is the *declaration*
		// from here on, which is what the shell that spells the letter on
		// both words does.
		//
		// Measured 2026-09-19 on bash 5.3.20, `env -i PATH=/usr/bin:/bin
		// LC_ALL=C` with a scratch HOME, eleven shapes with `local -g`
		// against the same line written `declare -g`: the two are the same
		// bytes and the same status in every one — past a local, valueless,
		// `+=`, an array literal, `-x`, `-r`, `-i`, `-A`, under a call's
		// assignment prefix, from a nested call, and through `command`.
		//
		// Delegating rather than reading the letter a second time here is
		// the point. `-g` is asked in five separate places on the
		// declaration route — the operand's assignment (interp/globaloperand.go),
		// the call's prefix and its own (interp/globalunderacallprefix.go,
		// interp/globalunderitsownprefix.go), the export letter that asks for
		// it, and the store — and a copy of that reading under this word
		// would be the second helper that carries four of the five.
		//
		// Below the refusal, the listing and `local -`, which are this
		// word's own and are not the declaration's: `local -g x=1` outside a
		// function is still `can only be used in a function`, and a bare
		// `local -gp q` still lists what `local` lists. Above the name
		// check, because declareNames does that itself and a bad name
		// refused twice is reported twice.
		return r.declareNames("local", args, f)
	}
	args, status, ended := r.builtinNames("local", args, false)
	if r.unspecified {
		return status
	}
	if nowhere {
		// The names have been read and a bad one has already been refused;
		// what does not happen is the declaration. See
		// Semantics.LocalThroughCommandDeclaresNothing.
		if ended {
			return r.endAfterABadName(status)
		}
		return status
	}
	for _, a := range args {
		name, value, hasValue, appends := declarationOperand(a)
		if r.typeLetterOverAnArrayLiteralRefused(name, f) {
			// `local -i z=(1 2)` is refused in the same words `typeset -i
			// z=(1 2)` is, with `local` in the location — see
			// typeLetterOverAnArrayLiteralRefused, which is one gate for all
			// four declaration utilities.
			return r.status
		}
		if r.unspecified {
			return r.status
		}
		if base, subs, subscripted := r.operandSubscripts("local", name); subscripted {
			if hasValue {
				// `local a[1]=v` is `typeset a[1]=v` under the other word, and
				// the scope is the whole of what it adds — see declareelement.go.
				r.declareElement(base, subs[:len(subs)-1], subs[len(subs)-1], value, appends, f, true)
				if r.unspecified || r.operandGaveUpTheBuiltin() {
					return r.status
				}
				continue
			}
			// And with no value the brackets are still read where the
			// dialect reads them, and the name declared is the *base* with
			// the array letter on it — measured, bash's `local 'a[1]'`
			// leaves `declare -a a`, a fresh local array, where this
			// declared a local literally named `a[1]` (#3501, #1380).
			n, letters, done := r.valuelessSubscriptedOperand(base, subs, f, true)
			if done {
				if r.unspecified || r.operandGaveUpTheBuiltin() {
					return r.status
				}
				continue
			}
			if r.unspecified {
				return r.status
			}
			name, f = n, letters
		}
		// Before the attributes, for the reason biTypeset gives: `-x` here
		// must not answer for the name this declaration shadows.
		wasExported := r.isExported(name)
		if r.declarationShadowRefused(name) {
			// See biDeclare: the operand is refused and the rest are still
			// declared, which is what the shell that refuses does — and the
			// fatal answer keeps its own status, which the `return 1` at the
			// end of this would otherwise overwrite.
			if r.unspecified || r.ctl == controlExit {
				return r.status
			}
			r.assignFailed = true
			continue
		}
		// shadow does nothing when there is no scope to save into, which is
		// the dialect that took this as a global: there is nothing to put
		// back, and it becomes a plain assignment.
		fresh := r.shadow(name)
		// And a name already holding a **reference** puts everything below on
		// what it points at. After the shadow, which is what leaves a fresh
		// binding out of it: the copy drops the reference along with every
		// other attribute. See interp/namerefattribute.go.
		if target, follows := r.attributeFollowsTheReference(name, f); follows {
			name = target
			wasExported = r.isExported(name)
		}
		// After the shadow, for the reason biDeclare gives: the cell this
		// declaration writes is a fresh binding, and an attribute applied
		// ahead of the shadow was saved as the *outer* name's and came back
		// on return as its own (#1673).
		// What the name carries before the letters land, so a refused
		// reference below can put it back — the same hold biDeclare takes,
		// and one function rather than two. See declarationtakenback.go.
		held := r.holdTheDeclaration(name)
		r.applyAttributes(name, f)
		r.localExportAttribute(name, f.export)
		if r.unspecified {
			// The declaration is not made at all: reporting the unanswered
			// axis and then assigning anyway is the silent wrong answer.
			return r.status
		}
		r.shadowedExport(name, wasExported)
		// After the shadow, for the reason biDeclare gives: the scope has
		// just saved the outer name's attribute and this is what may change
		// it. See hideinscope.go.
		r.setHideInScope(name, f)
		// After the shadow, the same order `typeset -A` keeps: the caller's
		// absence comes back when the function returns. Through the shared
		// mark rather than an `if` of its own, because the copy that stood
		// here had the table's half and not the array's — see
		// markDeclaredCompound and #1535.
		if !r.markDeclaredCompound(name, fresh, f, hasValue) {
			if r.unspecified || r.ctl == controlExit {
				return r.status
			}
			r.assignFailed = true
			continue
		}
		if f.readonly && f.readonlyOff {
			// `local +r y` after this same call's `local -r y=1`, which is
			// the one shape that reaches this with a freeze still standing:
			// the shadow above clears an attribute it *displaced*, and a
			// second declaration of a name this scope already shadowed
			// finds the copy made and nothing left to displace. Measured
			// 2026-09-07 — zsh answers `in=[2]`, bash refuses the
			// declaration before it gets here, and the two shells without
			// `local` never arrive.
			if code := r.removeReadonly(name, hasValue); code != 0 {
				return code
			}
			if r.unspecified || r.ctl == controlExit {
				return r.status
			}
		}
		if f.nameref {
			// `local -n out=$1` — the common spelling of a name reference,
			// and the reason the letter is on this word at all. Behind the
			// shadow, which is the whole of what `local` adds to it: the
			// reference belongs to this call and the caller's own name of
			// the same spelling gets itself back on return. See
			// interp/nameref.go.
			if code := r.declareNameref("local", name, value, f, hasValue,
				r.readonly[name] && !f.readonlyOff, !fresh, held, fresh); code != 0 {
				status = code
				if r.ctl == controlExit {
					return r.status
				}
				r.assignFailed = true
				continue
			}
			// The readonly letter, which is marked at the bottom of this
			// loop and which this branch's `continue` walked past — the same
			// line biDeclare's reference branch was missing. `local -rn`
			// freezes the call's own reference and the caller gets its own
			// back on return, which is what the shadow is for.
			if f.readonly && !f.readonlyOff {
				r.markReadonly(name)
			}
			continue
		}
		switch {
		case hasValue && r.arrayLiteralHiddenByQuoting(name, value, appends):
			// `local -a q="(1 2)"` is the same operand `typeset -a q="(1 2)"`
			// carries, and the word does not change what the text means —
			// measured, bash leaves two elements under either spelling. Ahead
			// of both stores below for the reason it is ahead of biDeclare's:
			// they would put the characters in element 0. This builtin has a
			// loop of its own, so the rule has to be asked in both places or
			// the shell answers one spelling and not the other.
			//
			// And the give-up with it: `local -a x="(a;b)"` is refused
			// inside the re-read and costs the rest of the line, exactly as
			// the other word's operand does.
			if r.unspecified || r.operandGaveUpTheBuiltin() {
				return r.status
			}
		case hasValue && appends:
			// `local a+=2` joins what the *local* is holding, which the
			// shadow above has already made: with no outer value carried
			// in, `f(){ local a+=2; }` leaves `2`. See declarationAppend.
			if !r.declarationAppend(name, value, false, fresh) {
				return r.status
			}
			if r.ctl == controlExit {
				return r.status
			}
		case hasValue:
			r.setVar(name, value)
			if r.ctl == controlExit {
				return r.status
			}
		default:
			if r.valuelessDeclarationLists(name, f, fresh) {
				// The same listing under the other word, on a name this
				// scope has already made local: `f(){ local s=1; local s; }`
				// writes `s=1` in the shell that lists. See
				// valuelessDeclarationLists.
				r.listStandingDeclaration(name)
			}
			if r.unspecified {
				return r.status
			}
			// No guard on r.unspecified here, deliberately. The one axis
			// declareEmpty asks that this builtin could not already reach —
			// InheritedValueSurvivesADeclaredType — needs a cell the
			// declaration did *not* just make, and `local` inside a function
			// always makes one. Adding a guard would change what the axes
			// this builtin has always reached do, which is not this change's
			// business, and nothing could exercise it either way.
			if !r.declarationCarriesAnArrayLiteral(name) {
				r.declareEmpty(name, fresh, f.export || f.readonly,
					withoutMatching(f) != (declareFlags{}),
					f.inherit || r.LocalInheritsTheOuterValue(), false)
			}
		}
		if f.readonly && !f.readonlyOff {
			r.markReadonly(name)
		}
	}
	if ended {
		// See biDeclare: `local` is the same declaration under another word
		// and gives up the same amount of the line.
		return r.endAfterABadName(status)
	}
	if r.assignFailed && status == 0 {
		// See biExport.
		return 1
	}
	return status
}

// bareOrDashP picks the listing shape for `export` and `readonly`: the one
// `-p` writes when `-p` was written, and BareDeclarationListing when nothing
// was. Two shells answer the two differently — see that field.
// namesUnderAPlus answers `export +` and `readonly +`: a lone plus sign as
// the whole of the line, which is the builtin's own listing with the values
// left off.
//
// The sign is `typeset`'s reading arriving under a second word — see
// Semantics.SignAloneIsAnOptionWordToExport, and the field for why those two
// builtins ask a question of their own rather than `typeset`'s. The filter is
// the caller's, so `export +` writes the exported names and `readonly +` the
// frozen ones.
//
// Only the sign on its own, with nothing else on the line. `export + q` is a
// silent 0 in the one shell that takes the sign at all — it neither lists the
// name nor takes the attribute off it — and that is a corner deliberately not
// followed: an operand here keeps the refusal every other column gives it.
// Measured 2026-09-12 on zsh 5.9.2.
//
// The bool is whether this was that shape, so a caller can go on with the
// line it really has.
func (r *Runner) namesUnderAPlus(args []string, keep func(declaration) bool) (int, bool) {
	if len(args) != 1 || args[0] != "+" {
		return 0, false
	}
	if !r.ask(r.sem().SignAloneIsAnOptionWordToExport, "a bare `+` given to `export` or `readonly`") {
		// A name, and one no script may declare — which is the refusal the
		// operand path already gives it.
		return r.status, r.unspecified
	}
	// `export +` and `readonly +`, so no produced name: neither attribute is
	// carried by one in any column of the panel, which is the same reason
	// their `-p` leaves them out — see declarePrintForm.
	return r.declarationFilteredNameListing(r.declarableNames(), nil, ProducedListingUnspecified, keep), true
}

// The second result is whether `-p` was the word written, which is a
// different question from which *shape* came back: two dialects give the two
// forms one shape and a produced parameter can still be in one listing and
// not the other. See ProducedDeclaration.Silent.
func (r *Runner) bareOrDashP(opts string, dashP DeclarationListingForm) (DeclarationListingForm, bool) {
	if strings.ContainsRune(opts, 'p') {
		return dashP, true
	}
	return r.sem().BareDeclarationListing, false
}

// readonlyRecordsTheCompound reports whether `readonly -a` and `readonly -A`
// declare the kind as well as freezing the name — see
// Semantics.ReadonlyRecordsTheCompoundAttribute.
//
// Asked only where one of the two letters was written, which is the whole of
// the disagreement: a `readonly` with no kind letter freezes a name in every
// column and raises no question.
func (r *Runner) readonlyRecordsTheCompound() bool {
	return r.ask(r.sem().ReadonlyRecordsTheCompoundAttribute,
		"the array letter on `readonly` declaring an array")
}

// biReadonly marks variables immutable.
func biReadonly(r *Runner, _ context.Context, args []string) int {
	if code, answered := r.namesUnderAPlus(args,
		func(d declaration) bool { return d.readonly }); answered {
		return code
	}
	// The letters are the dialect's, the way `read`'s and `unset`'s are: they
	// were fixed here as `paAf` until #2277, so three columns took kind
	// letters their shell has never had and then walked into an axis they
	// cannot be asked. Empty is POSIX's set.
	letters := r.sem().ReadonlyOptions
	if letters == "" {
		letters = "p"
	}
	args, opts, code := r.builtinOptions("readonly", args, letters)
	if code != 0 {
		return code
	}
	// The kind the letters named, carried the way every other declaration
	// loop carries it. `readonly` was a fourth loop that read `aA` and
	// applied neither mark, so `readonly -a a` froze a name and recorded
	// nothing about what it was (#1554).
	// The `n` letter, where the dialect has one: it suppresses the freeze
	// this call would otherwise make and changes nothing else. Read once,
	// here, so that every branch below — the listing, the kind letters, the
	// append and the plain store — is the branch it always was and only the
	// freeze is missing. See Semantics.ReadonlyReferenceLetter.
	freezes := true
	if strings.ContainsRune(opts, 'n') {
		var answered bool
		freezes, answered = r.readonlyReferenceLetterFreezes()
		if !answered {
			return r.status
		}
	}
	f := declareFlags{
		readonly: freezes,
		array:    strings.ContainsRune(opts, 'a'),
		assoc:    strings.ContainsRune(opts, 'A'),
	}
	if strings.ContainsRune(opts, 'f') && r.sem().FunctionAttributeLetters != "" {
		// The `f` letter names the *function* table, which is a different
		// freeze under the same word — `readonly f` makes a variable
		// immutable and `readonly -f f` makes a function undefinable. The
		// letter was in ReadonlyOptions and reached nothing, so the option
		// was accepted, no record was kept, and the function could be
		// redefined and unset afterwards at status 0 (#3192).
		//
		// With no name it is the listing `declare -fr` writes, `-p` or not:
		// measured on bash 5.3.20, `readonly -f` and `readonly -pf` are the
		// same bytes.
		if len(args) == 0 {
			return r.attributedFunctionListing(functionAttributeReadonly)
		}
		names, status, ended := r.builtinNames("readonly", args, false)
		if r.unspecified || ended {
			return status
		}
		return r.freezeFunctions(names)
	}
	if strings.ContainsRune(opts, 'p') || ((opts == "" || opts == "n") && len(args) == 0) {
		// The listing: readonly names alone, in the dialect's shape. `-p` and
		// nothing at all list alike, which is the same rule `export` follows
		// and is measured the same way. The `n` letter with no operand lists
		// too rather than declaring nothing: measured 2026-09-18, a bare
		// `readonly -n` writes the same frozen names a bare `readonly` does.
		//
		// With operands beside the letter it is the axis `export` asks on the
		// same line of reasoning — see interp/exportprintoperand.go. This
		// builtin answered it by never listing, which is right in four
		// columns and silently wrong in the two that do, and it is the half
		// of #3904 that made the two words disagree here for a difference the
		// panel does not have.
		names, lists := r.exportPrintWithOperands(args)
		if r.unspecified {
			return r.status
		}
		if lists {
			form, dashP := r.bareOrDashP(opts, r.sem().ReadonlyListing)
			return r.declarePrintForm(names, form, dashP,
				func(d declaration) bool { return d.readonly })
		}
	}
	args, status, ended := r.builtinNames("readonly", args, false)
	if r.unspecified {
		return status
	}
	for _, a := range args {
		name, value, hasValue, appends := declarationOperand(a)
		// A **member path** whose base is a reference is a member of the name
		// the reference points at, and the whole operand is about that cell.
		// Here rather than beside attributeFollowsTheReference below, which
		// runs *after* the scope is taken: that rule is keyed on whole names
		// and never saw a dotted one, and redirecting after the shadow would
		// leave the copy standing over `c.b` — a binding nothing reads, with
		// this line's letter recorded on it and given back at the return. See
		// Runner.compoundMemberThroughAReference.
		name = r.compoundMemberThroughAReference(name)
		if base, subs, subscripted := r.operandSubscripts("readonly", name); subscripted {
			sub := subs[len(subs)-1]
			// The readonly attribute on an element is the axis with three
			// answers — see Semantics.ReadonlyElement. Only the two dialects
			// that take a subscripted operand at all arrive here.
			//
			// Asked without a value as well, because the refusal is about
			// the attribute rather than about the assignment: measured,
			// `readonly "a[1]"` is refused in the same words as
			// `readonly a[1]=v`. Nothing else changes for the valueless
			// form, which falls through to the path it always took.
			if hasValue {
				r.declareElement(base, subs[:len(subs)-1], sub, value, appends, declareFlags{readonly: true}, false)
				if r.unspecified || r.operandGaveUpTheBuiltin() {
					return r.status
				}
				continue
			}
			n, _, done := r.valuelessSubscriptedOperand(base, subs, declareFlags{readonly: true}, false)
			if done {
				if r.unspecified || r.operandGaveUpTheBuiltin() {
					return r.status
				}
				continue
			}
			if r.unspecified {
				return r.status
			}
			if r.elementDeclarationRefused(base, sub, declareFlags{readonly: true}, false) {
				return r.status
			}
			// And the name this freezes is the *base*: `readonly 'a[1]'`
			// froze a variable literally named `a[1]` and left `a` writable,
			// where ksh93 leaves `typeset -r -a a` (#3501, #1380).
			name = n
		}
		// The scope, where this dialect reads `readonly` as its own
		// `typeset -r` — see Semantics.ReadonlyDeclaresALocal. Ahead of
		// everything the operand does, the order biLocal and biDeclare keep:
		// the copy has to be taken before an attribute or a value lands, or
		// the caller's name gets this call's back on return (#1673).
		fresh := r.readonlyDeclaresALocal(name)
		if r.unspecified {
			return r.status
		}
		// The freeze belongs to what a reference points at, which is the row
		// of #3136 that costs a script something: a helper handed the name of
		// a caller's variable and told to freeze it froze the reference, and
		// the caller's variable stayed writable at status 0. See
		// interp/namerefattribute.go.
		//
		// And a reference aimed at one **element** parts the value from the
		// freeze — see biExport, where the same two lines stand for the same
		// reason. This is the row of #3881 that costs a script something: the
		// freeze landed on the whole array in a dialect that freezes nothing
		// here, so the next ordinary write to any element was refused.
		valueTarget, letters := r.nameOperandThroughAReference("readonly", name, f, freezes)
		if r.unspecified {
			return r.status
		}
		if target, follows := r.attributeFollowsTheReference(name, f); follows {
			name = target
		}
		if (f.array || f.assoc) && r.readonlyRecordsTheCompound() {
			// Ahead of the assignment, the order every other declaration
			// loop keeps: the letters say what the name is and the value
			// then lands in it.
			if !r.markDeclaredCompound(name, fresh, f, hasValue) {
				if r.unspecified || r.ctl == controlExit {
					return r.status
				}
				r.assignFailed = true
				continue
			}
		}
		if r.unspecified {
			return r.status
		}
		if hasValue {
			// See biExport: a declaration's plain word over a name really
			// holding an array is the same refusal under this word.
			if r.inconsistentTypeRefused(name, false, declareFlags{}) {
				return r.status
			}
			if r.unspecified {
				return r.status
			}
			// `readonly a+=2` joins what the name holds and then freezes it
			// — over the standing value where this builtin took no scope,
			// and over the fresh local's nothing where it did.
			if appends {
				if !r.declarationAppend(r.orName(valueTarget, name), value, false, fresh) {
					return r.status
				}
				if r.ctl == controlExit {
					return r.status
				}
				if !letters {
					// The refused attribute: the join has happened and this
					// operand is finished. See the tail of this loop.
					continue
				}
				r.declarationAssignmentExport(name, false)
				if r.unspecified {
					return r.status
				}
				if freezes {
					r.markReadonly(name)
				}
				continue
			}
			r.setVarAs(r.orName(valueTarget, name), value, assignedByDeclaration)
			if r.ctl == controlExit {
				// See biExport.
				return r.status
			}
			if !letters {
				continue
			}
			// `readonly` is one shell's `typeset -r` and behaves like it
			// here: an assignment through it resets the export attribute
			// where that shell's `typeset` does. See
			// declarationAssignmentExport.
			r.declarationAssignmentExport(name, false)
			if r.unspecified {
				return r.status
			}
		}
		if !letters {
			// The column that refuses the attribute has already said so, and
			// the value above is the whole of what it does: measured,
			// `readonly b=Y` over a reference to `a[1]` leaves `p Y r` with
			// `a` **unfrozen**, at 0. Nothing is declared and nothing is
			// frozen, so the rest of the loop does not run.
			continue
		}
		// `readonly` is an attribute word too — see biExport and
		// declarationOwnsTheStandingEmpty.
		if hasValue {
			r.declarationOwnsTheStandingEmpty(name)
		} else {
			// A valueless declaration: the name shows whatever
			// DeclaredNameWithoutValueIsEmpty says a declared name holds,
			// which is the same question every other declaration word asks.
			// Without it a `readonly R` inside a function left the local
			// unset where the shell that scopes it leaves the empty string,
			// and a `readonly R` at the *top level* did the same in that
			// shell — the freeze landed on a name that did not exist, so
			// `${R-unset}` took its default there and nowhere else (#2887).
			//
			// The scope gate this used to carry was doing the standing-empty
			// call's job as well: declareEmpty owns that half, so `fresh` is
			// passed rather than tested. `readonly` has no letter for
			// inheriting and the shell-wide name is about a *local*
			// declaration: measured 2026-09-17, `readonly R` inside a
			// function with the option on takes the enclosing value the same
			// way `local R` does.
			r.declareEmpty(name, fresh, true, true, r.LocalInheritsTheOuterValue(), true)
			if r.unspecified || r.ctl == controlExit {
				return r.status
			}
		}
		if freezes {
			// The `n` letter's whole effect: the name is declared, holds what
			// it holds and is not frozen. Nothing is taken *off* either — a
			// name the shell already froze stays frozen, which is why this is
			// a skip of the mark rather than an unmark.
			r.markReadonly(name)
		}
	}
	if ended {
		// See biDeclare.
		return r.endAfterABadName(status)
	}
	if r.assignFailed && status == 0 {
		// See biExport.
		return 1
	}
	return status
}

// readonlyReferenceLetterFreezes asks what the `n` letter does here, and
// answers whether this call still freezes what it declares.
//
// Asked only where the letter was written, which is the whole of the
// disagreement: a `readonly` without it freezes in every column and raises no
// question. The second result is whether the dialect answered at all — see
// Semantics.ReadonlyReferenceLetter, where Unspecified is what a dialect
// whose `readonly` has no `n` holds and is unreachable from here.
func (r *Runner) readonlyReferenceLetterFreezes() (freezes, answered bool) {
	switch r.sem().ReadonlyReferenceLetter {
	case ReadonlyReferenceLetterDeclaresAnUnfrozenName:
		return false, true
	case ReadonlyReferenceLetterIsInert:
		return true, true
	}
	r.errf("%s\n", r.diag().Report(r.name(), r.line,
		r.unanswered("the `n` letter of `readonly`")))
	r.status, r.unspecified = 2, true
	return true, false
}

// readonlyDeclaresALocal saves the name in the innermost scope where the
// dialect reads `readonly` as a declaration word with a scope of its own,
// and reports whether the copy was taken here — the same `fresh` shadow
// hands back, and the same thing it means.
//
// Asked only inside a function, which is the only place the two answers
// differ: at the top level there is no scope to take and every shell in the
// panel freezes the name it names. See Semantics.ReadonlyDeclaresALocal.
//
// It is r.shadow rather than r.shadowTypeset deliberately. The keyword-word
// gate belongs to `typeset` and was measured not to reach this one: ksh93
// scopes `typeset` in a `function`-defined function and does not scope
// `readonly` in the same function, so routing this through that gate would
// have given ksh93 an answer it does not hold.
func (r *Runner) readonlyDeclaresALocal(name string) bool {
	if len(r.scopes) == 0 {
		return false
	}
	if !r.ask(r.sem().ReadonlyDeclaresALocal, "`readonly` inside a function declaring a local") {
		return false
	}
	return r.shadow(name)
}

// readonlyScopesItsOperands reports whether a `readonly` written here will
// declare locals, which is what decides the order its array-literal operands
// are assigned in — see the `locks` comment in Runner.runBuiltinCommand.
//
// It reads the axis where readonlyDeclaresALocal asks it, and the difference
// is deliberate: this is consulted once for the whole command, before the
// builtin has looked at an operand or even at its letters, and an unanswered
// axis reported from there would fire for a `readonly -p` that names no name
// at all. The builtin asks, per operand, where the answer actually changes
// what the declaration does.
func (r *Runner) readonlyScopesItsOperands() bool {
	return len(r.scopes) > 0 && r.sem().ReadonlyDeclaresALocal == Yes
}

// biExit ends the shell.
//
// A bare `exit` reports what the last command did, and a status is taken
// modulo 256 because that is all a process can carry — `exit 300` is 44 in
// every shell measured.
func biExit(r *Runner, ctx context.Context, args []string) int {
	// Asked before the operand is read, because the shell is not going
	// anywhere: `exit 3` with a job the shell is checking for stays, and the
	// 3 is never used.
	// The status is the dialect's, and the two that stay disagree about it —
	// bash reports a builtin that failed, zsh reports nothing of the kind.
	if r.HoldsExitForJobs() {
		return r.diag().StoppedJobsAtExitStatus
	}
	args, marked := r.numericOperandMarker(args)
	if !marked {
		// Unanswered: the shell stops rather than leaving with a status it
		// has just refused to read, which is what the unreadable operand
		// below does too.
		r.stopTheShellForExit()
		return r.status
	}
	if len(args) > 0 {
		switch n, ok := r.statusOperand("exit", args[0]); {
		case ok:
			if st, done := r.extraNumericOperands("exit", args); done {
				return st
			}
			// Masked here and not in the reading, because the eight bits are
			// the *process's* limit rather than a decision any shell made:
			// `exit 300` is 44 in all six, including the two that leave a
			// `return 300` at 300. That difference is only ever visible
			// through `return`, since a shell that has exited has no `$?`
			// left to read.
			r.status = mask8(n)
		case r.unspecified:
			// No dialect answered; statusArgument has already said so, and
			// the script stops rather than exiting with a status it just
			// refused to choose.
			r.stopTheShellForExit()
			return r.status
		default:
			return r.badStatusArg("exit", args[0])
		}
	}
	if len(args) == 0 {
		// A bare `exit` reports what the last command did, and one column
		// reads "the last command" as this execution unit's rather than the
		// shell's. Ahead of the trap question below, which is a narrower
		// reading of the same word and wins where it applies.
		r.status = r.operandLessStatus()
	}
	if len(args) == 0 && r.inExitTrap &&
		r.ask(r.sem().ExitInTrapReportsEarlierStatus, "a bare `exit` in an EXIT trap") {
		// The status the trap was entered with, not the one its own commands
		// left behind: `trap "false; exit" 0; true` is 0 in three of the
		// four.
		r.status = r.exitTrapEntryStatus
	}
	// Not `set -e` firing, and said so rather than left: this is the
	// producer a try-always block has to tell that one apart from, and the
	// two reach the same field. `exit` runs the cleanup halves it unwinds
	// through and `set -e` does not (#1238).
	r.stopTheShellForExit()
	// Before the unwinding, in the dialects whose EXIT trap is fired from
	// where the word was written rather than from the end of the shell. The
	// stop above is raised first so that the note only this producer can
	// take — whether a file was being read — is the `exit` word's own and not
	// something the trap body's own `.` overwrote.
	r.runExitTrapInsideTheExitingCall(ctx)
	return r.status
}

// biTrap sets what runs when a condition arises: EXIT, a signal, or one of
// the pseudo-conditions a dialect has (ERR, DEBUG, RETURN — pseudotrap.go).
//
// Every condition the shell knows is taken, including the two signals nobody
// can catch: `trap … KILL` is accepted, listed and reset here exactly as any
// other signal is, and the handler it records can never run. That is not a
// silent wrong answer — it is the unanimous one, measured on all five
// references, and trappableSignals carries the reasoning at length (#2919).
func biTrap(r *Runner, _ context.Context, args []string) int {
	args, done := r.trapOptions(args)
	if done != trapKeepGoing {
		return int(done)
	}
	if len(args) == 0 {
		return r.printTraps(nil, false)
	}
	body, conds := args[0], args[1:]
	if len(conds) == 0 {
		return r.trapSingleArgument(body)
	}
	// Before the conditions, because the dialect that reads the action now
	// says nothing about a bad condition when the action will not parse.
	if st, refused := r.trapActionRefused(body); refused {
		return st
	}
	// Every condition is checked before any is acted on, so a bad one does
	// not leave half the request applied.
	type target struct {
		name   string
		pseudo string
		// spelled is the word the script wrote, kept for the one condition
		// that answers to two names and lists back the one it was given.
		spelled string
		sig     syscall.Signal
		exit    bool
	}
	targets := make([]target, 0, len(conds))
	for _, c := range conds {
		if strings.EqualFold(c, "EXIT") || c == "0" {
			targets = append(targets, target{exit: true})
			continue
		}
		// The pseudo-conditions come before the signal table because they
		// are not in it: a dialect that has ERR takes the word here, and one
		// that does not falls through and refuses it as the unknown name it
		// is there.
		if name, ok := r.pseudoCondition(c); ok {
			targets = append(targets, target{pseudo: name, spelled: strings.ToUpper(c)})
			continue
		}
		if r.unspecified {
			return r.status
		}
		name, sig, kind := r.canonicalSignal(c)
		switch kind {
		case signalUnknown:
			msg := Wording(r.diag().TrapBadSignal, "trap: %[1]s: bad trap", c)
			if r.diag().TrapBadSignalUnprefixed {
				r.errf("%s\n", msg)
			} else {
				r.diagf("%s\n", msg)
			}
			// 1 in all four, and the one thing they agree on here.
			return 1
		}
		targets = append(targets, target{name: name, sig: sig})
	}
	// Setting, resetting or ignoring anything makes the trap state this
	// runner's own: a listing it inherited from the shell around it is
	// dropped rather than shown alongside — see trapsModified.
	r.trapsModified()
	for _, tg := range targets {
		// What this condition holds now, kept where the dialect scopes a
		// function's traps to the call — taken here, at the modification,
		// rather than at the call, which is the measured difference between
		// this and the option table the same shell scopes. See localtraps.go.
		//
		// EXIT is named rather than skipped here, because whether it is one
		// of these is that file's rule and not this loop's: a second copy of
		// the answer beside the first is how the two drift apart.
		switch {
		case tg.pseudo != "":
			r.localizeTrap(tg.pseudo, 0)
		case tg.exit:
			r.localizeTrap("EXIT", 0)
		default:
			r.localizeTrap(tg.name, tg.sig)
		}
		// A `trap` command naming a condition is the whole of that
		// condition's handler afterwards, so the function form of it — where
		// the dialect has one — goes with the action it replaces. Measured:
		// after `TRAPZERR(){ … }; trap 'echo T' ZERR`, `functions TRAPZERR`
		// finds nothing. See trapfunction.go.
		switch {
		case tg.pseudo != "":
			r.releaseTrapFunction(tg.pseudo)
		case tg.exit:
			r.releaseTrapFunction("EXIT")
		default:
			r.releaseTrapFunction(tg.name)
		}
		switch {
		case tg.pseudo != "":
			// The word this condition was named by, which the dialect with
			// two names for it echoes back in its listing.
			if tg.pseudo == "ERR" {
				r.errTrapSpelling = tg.spelled
			}
			r.setPseudoTrap(tg.pseudo, body)
		case tg.exit:
			if body == "-" {
				r.exitTrap = nil
				continue
			}
			// A second trap replaces the first rather than adding to it.
			b := body
			r.exitTrap = &b
			r.trapDepth = r.depth
		case body == "-":
			r.trapSignal(tg.name, tg.sig, nil)
		default:
			b := body
			r.trapSignal(tg.name, tg.sig, &b)
		}
	}
	return 0
}

// sortedKeys lists a map's keys in a stable order, so `trap` prints the same
// thing twice running.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// badStatusArg reports a status operand `exit` will not take.
//
// The status is 2 in both shells that refuse, and it is not the fatal-error
// status: bash exits 1 for a fatal error and 2 for this. A usage error is its
// own thing, which is why it is written here rather than routed through fatal.
//
// **A refused operand is not a reason to leave.** `exit` is the one builtin
// whose whole job is to end the script, which is why this door used to end it
// unconditionally — and that is the wrong reading, measured 2026-09-13 with
// `exit status; echo "after=$?"` at the top of a script. bash 5.3.15 reports
// the complaint, leaves 2 behind and runs the next command; so do ksh93 and
// zsh, which never refuse a word at all. dash ends the script at 2, and so
// does that same bash called as `sh`, which is its posix mode reaching the
// POSIX rule that a special builtin's usage error is fatal. Whether the shell
// goes is therefore BadOptionToSpecialBuiltinFatal and not this builtin's
// name — the same door `return`, `shift` and `unalias` already came through,
// and `exit` was the one caller of the four that never asked.
//
// `return` refuses the same words in the same shells and words the complaint
// from the same template; what differs is only what it sets on the way out,
// so see refusedReturnOperand rather than folding the two.
func (r *Runner) badStatusArg(builtin, arg string) int {
	r.diagf("%s\n", Wording(r.diag().NumericArgument, "%[1]s: invalid number: %[2]s", builtin, arg))
	if r.ask(r.sem().BadOptionToSpecialBuiltinFatal, "a special builtin's bad operand ending the script") {
		r.fatalUsageQuiet()
		// The 2 and not `r.status`, for the reason shiftBadNumber returns the
		// builtin's status: see setFatalStatus (#2583).
		return 2
	}
	// Reported and not obeyed: no control flow is set, so the next statement
	// runs and the 2 below is what it finds in `$?`.
	return 2
}

// nameIsSet reports whether the shell has heard of a parameter at all, which
// is what UnsetSubscriptSkippedWhenNameUnset asks about the base of a
// subscripted operand.
//
// Set-ness and not emptiness, measured a name at a time on zsh 5.9.2: `v=`,
// a `typeset -a` array with no elements and a bare `typeset s` all count as
// heard of, and their subscripts *are* evaluated. Only a name that was never
// assigned, or was assigned and then unset, is skipped. The two declared
// forms are why getVar alone is not the answer — a table with no entries
// holds no value for it to return.
// The tables are consulted ahead of the read, and the order is the point
// rather than an economy: a bare read of an array name asks
// ArrayScalarIsTheWholeArray, and this question does not need that answer.
// Asking it would refuse `unset "a[1]"` outright in a core with no dialect,
// over a value nothing on this path would have looked at.
func (r *Runner) nameIsSet(name string) bool {
	name = r.throughNameref(name)
	if !r.removed[name] && (r.arrayDeclared(name) || r.assocDeclared(name)) {
		return true
	}
	_, ok := r.getVar(name)
	return ok
}

// carryUnsetStatus is how one operand's status joins the builtin's, and it is
// the whole of UnsetStatusIsTheLastSubscripts.
//
// Under the last-subscript reading the operand overwrites what came before it,
// so a success after a failure is a success; otherwise a failure sticks and
// only another failure replaces it. Only the branches that actually read a
// subscript call this — a plain name, an absent name and an association's key
// leave the status alone in both readings, which is measured and is the
// correction the issue's two-row table would have missed.
func (r *Runner) carryUnsetStatus(status, code int) int {
	if r.sem().UnsetStatusIsTheLastSubscripts == Yes {
		return code
	}
	if code != 0 {
		return code
	}
	return status
}

// exportAsADeclaration is `export` reading the declaration letters, in the
// dialect whose `export` is `typeset -gx` under another word — see
// Semantics.ExportOptions. The second result is whether it took the line.
//
// It takes only a line that *wrote* one of those letters. A plain `export
// A=1`, a bare `export` and `export -p` are the same three commands they were
// before this existed and go on down the loop below, which is deliberate: the
// listing has a shape of its own that no declaration writes, and the common
// path must not be re-routed to reach a letter it never carried.
//
// Where it does take the line the whole declaration is Runner.declareNames,
// which is the same choice `integer` made and for the same reason: the
// attribute, the arithmetic a later assignment means, the readonly refusal
// and every letter's meaning come from the one place. A second implementation
// of `-i` under `export` is the thing that would drift the first time either
// was measured again.
//
// The two attributes the *word* decides are set here rather than read off the
// letters, because the letters that spell them are the two this dialect
// refuses: `export -x` and `export -g` are bad options, since the word
// already says both.
func (r *Runner) exportAsADeclaration(args []string, letters string) (int, bool) {
	extra := r.sem().ExportOptions
	if extra == "" || !hasAnyOption(args, extra) {
		return 0, false
	}
	name := r.inBuiltin
	if name == "" {
		name = "export"
	}
	rest, f, code := r.parseDeclareFlags(name, args, letters+extra)
	if code != 0 {
		return code, true
	}
	if f.print || len(rest) == 0 {
		// A listing after all — `export -p` and `export -i` with no names —
		// and the listing is the loop's below, not a declaration's.
		return 0, false
	}
	f.export = true
	// And the *word* asked for it, so a plus word carrying some other letter
	// may not take it back off: `export +i q=4` is still an export. See
	// declareFlags.exportForced.
	f.exportForced = true
	// No scope is ever taken by this word, which is what the loop below says
	// too: `export` inside a function attributes the global.
	f.global = true
	return r.declareNames(name, rest, f), true
}

// hasAnyOption reports whether any leading option word of args carries one of
// these letters, under either sign.
//
// The leading words only, and it stops at the first operand: a `-i` inside a
// *value* — `export t=-i` — is not an option and must not put the line on a
// route it never asked for. `--` ends them, as it does for every other reader
// of an option word here.
func hasAnyOption(args []string, letters string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if len(a) < 2 || (a[0] != '-' && a[0] != '+') {
			return false
		}
		if strings.ContainsAny(a[1:], letters) {
			return true
		}
	}
	return false
}

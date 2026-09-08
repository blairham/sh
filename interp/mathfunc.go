// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"

	"github.com/blairham/sh/syntax"
)

// Math functions: a shell function registered under a name that arithmetic can
// call, which is `functions -M` in the one shell that has the facility.
//
// The registration's *name* is a dialect's — see dialect/zsh, which is where
// `functions -M` is spelled — and the capability is here, because what it
// needs is the arithmetic evaluator calling back into the interpreter. That is
// a seam this engine did not have and is the substance of #1493: an expression
// stops being a pure reading of values and becomes a place a shell function
// runs, with everything a call brings — positional parameters, a scope, a
// status, a recursion bound, and the shell's own state changed by the time the
// expression resumes.
//
// # The value is not REPLY
//
// The documented convention is that the function sets `REPLY` and the shell
// reads it. Measured 2026-09-08 on zsh 5.9.2 (Homebrew, aarch64) and zsh 5.9
// (`/bin/zsh`, macOS 26), both agreeing, that is not what happens: `REPLY` is
// never read, and what comes back is **the value of the last arithmetic
// evaluation performed anywhere during the call**.
//
//	body of the implementation      $(( mf(5) ))
//	REPLY=7                         5    — the argument, the last thing evaluated
//	REPLY=$((7))                    7
//	: $((123)); REPLY=7             123
//	REPLY=7; : $((123))             123
//	(( x = 77 ))                    77
//	return 42                       42
//	local i=9; return i             9
//	float REPLY; REPLY=2.5          2.5
//	h(){ : $((55)); }; h            55
//	: (nothing at all)              5    — the argument again
//	: called as mf(3,4)             4    — the *last* argument
//	: $((123)) before registering    123  — evaluated before the call even existed
//
// So `REPLY=$(( … ))`, the idiom every documented caller writes, gives the
// right answer by accident: the arithmetic on its right-hand side is the last
// evaluation and its value is what returns. An implementation that read
// `REPLY` would agree with every idiomatic caller and disagree with the one
// that matters — the plugin manager on the maintainer's rc file registers
// `zi_scheduler_add`, whose implementation sets no `REPLY` at all and ends in
// `return idx`, and writes the registration with `2>/dev/null` so a silent
// failure is invisible. A `REPLY` reading returns the last *argument* there
// and the scheduler picks the wrong task.
//
// [Runner.lastArith] is the record that makes it possible, and it is a
// shell-lifetime value rather than a per-call one, because the last row of the
// table above says so: an evaluation from before the registration is still
// what an empty implementation hands back.
//
// # What else was measured
//
//   - **Arity is `name [min [max [impl]]]`.** Nothing after the name is
//     `min 0, max unbounded`; a `min` alone makes `max` equal to it, so
//     `functions -M mf 1` takes exactly one argument. A `max` of -1 is
//     unbounded. `min` below zero is refused, and a `max` below `min` is
//     refused unless it is the -1 that means no bound. A fifth operand is
//     `-M: too many arguments`.
//   - **Registration never checks the implementation.** `functions -M mf 1 1
//     nodef` is status 0 and the failure arrives at the *call*, as `no such
//     function: nodef`. So does an implementation removed between the two.
//   - **A name that is not an identifier is refused**, as
//     `-M mf*: bad math function name`, and that is also why `functions -M`
//     with operands never lists: every operand form is a registration.
//   - **`functions -M` with no operands lists**, most recently registered
//     first, each in the form that would make it.
//   - **`functions +M name…` removes**, silently for a name never registered.
//     `unfunction -M` is not the spelling: that is `bad option: -M`.
//   - **`$0` inside the implementation is the math function's name**, not the
//     implementation's, and `$#` and `$*` are the arguments — already
//     evaluated, so `mf(1+1,2*3)` passes `2` and `6`, and passed as the shell
//     writes a number, so `mf(1.5,2.0)` passes `1.5` and `2.`.
//   - **Math functions are not in the `functions` listing** and not in the
//     `$functions` associative array: `${+functions[mf]}` is 0 with `mf`
//     registered.

// runMathFuncBody is how an expression reaches the interpreter: it runs the
// registered implementation with the arguments as its positional parameters
// and the *registered* name as the one `$0` answers with.
//
// A variable assigned in init rather than a direct call, for the reason
// source.go's init gives about `eval` and `.`: this is the point where
// evaluating an expression runs arbitrary shell, so it reaches the dispatcher
// that reads the builtins map — and every builtin whose operand is an
// expression reads that map on the way here, which Go calls an initialization
// cycle. Moving the builtins out of the literal one at a time would not end,
// because `let`, `shift`, `printf`, `test` and the declarations all evaluate
// arithmetic. Breaking it once, at the seam that created it, does.
var runMathFuncBody func(r *Runner, fn *syntax.FuncDecl, name string, args []string) error

func init() {
	runMathFuncBody = func(r *Runner, fn *syntax.FuncDecl, name string, args []string) error {
		return r.callFuncAs(r.ctx, fn, name, args)
	}
}

// mathFunc is one registration: the name arithmetic calls, the arity it
// accepts, and the shell function that runs.
type mathFunc struct {
	// min is the fewest arguments a call may pass, and max the most. A max
	// of mathFuncUnbounded is no upper limit at all, which is what a
	// registration with no arity gets.
	min, max int
	// impl is the shell function to run, which defaults to the registered
	// name and is looked up at the call rather than here.
	impl string
}

// mathFuncUnbounded is the max that means "as many as are written". Spelled
// -1 because that is the value the shell takes and prints back.
const mathFuncUnbounded = -1

// registerMathFunc records a registration, replacing any under the same name.
//
// A re-registration moves the name to the *front* of the listing rather than
// keeping the place the first one gave it. Measured: registering `a`, then
// `b`, then `a` again lists `a` first, where a table that kept the original
// position would list `b` first — which is what this did until a mutation run
// found nothing checking it.
func (r *Runner) registerMathFunc(name string, fn mathFunc) {
	if r.mathFuncs == nil {
		r.mathFuncs = map[string]mathFunc{}
	}
	r.forgetMathOrder(name)
	r.mathOrder = append(r.mathOrder, name)
	r.mathFuncs[name] = fn
}

// forgetMathOrder drops a name from the arrival order, for a removal and for
// the re-registration that is about to put it back at the front.
func (r *Runner) forgetMathOrder(name string) {
	for i, n := range r.mathOrder {
		if n == name {
			r.mathOrder = append(r.mathOrder[:i:i], r.mathOrder[i+1:]...)
			return
		}
	}
}

// removeMathFunc is `functions +M name`. A name that was never registered is
// not an error — measured, status 0 with nothing said.
func (r *Runner) removeMathFunc(name string) {
	if _, had := r.mathFuncs[name]; !had {
		return
	}
	delete(r.mathFuncs, name)
	r.forgetMathOrder(name)
}

// mathFuncListing is every registration, most recently registered first, in
// the form that would make it. Measured: registering `aa bb cc dd ee` in that
// order lists `ee` first, and registering them backwards lists `aa` first, so
// the order is the table's and not the alphabet's.
func (r *Runner) mathFuncListing() []string {
	// mathOrder holds exactly the registered names and nothing else, which
	// is why there is no second check here for a name the table no longer
	// has: registerMathFunc and removeMathFunc are the only writers and both
	// keep the two in step. A guard here as well would be a second mechanism
	// for one invariant, and the one that is never wrong is the one that
	// stops being maintained.
	out := make([]string, 0, len(r.mathOrder))
	for i := len(r.mathOrder) - 1; i >= 0; i-- {
		name := r.mathOrder[i]
		out = append(out, "functions -M "+mathFuncSpec(name, r.mathFuncs[name]))
	}
	return out
}

// mathFuncSpec writes a registration back the way the shell says it, which
// elides what a re-registration would default to and is measured row by row:
//
//	registered as        said back as
//	mf                   mf
//	mf 0                 mf 0 0
//	mf 1                 mf 1
//	mf 2 2               mf 2
//	mf 0 -1              mf
//	mf 1 -1              mf 1 -1
//	mf 0 1               mf 0 1
//	mf 1 1 g             mf 1 1 g
//	mf 0 -1 g            mf 0 -1 g
//
// So: nothing at all when the whole registration is the default; otherwise the
// minimum, then the maximum unless it equals a *non-zero* minimum, then the
// implementation when it is not the name. An implementation forces the maximum
// out, because the operand it stands in is positional.
func mathFuncSpec(name string, fn mathFunc) string {
	if fn.min == 0 && fn.max == mathFuncUnbounded && fn.impl == name {
		return name
	}
	spec := name + " " + strconv.Itoa(fn.min)
	if fn.max != fn.min || fn.min == 0 || fn.impl != name {
		spec += " " + strconv.Itoa(fn.max)
	}
	if fn.impl != name {
		spec += " " + fn.impl
	}
	return spec
}

// evalMathFunc runs a math function call written inside an expression.
//
// The three failures are separate sentences in the shell that has them, and
// they are separate because they are separate questions: a name nobody
// registered, a registration called with the wrong count, and a registration
// whose implementation is not there. The middle one quotes the call as it was
// written, which is why [syntax.ArithCall] keeps the source text.
func (r *Runner) evalMathFunc(x *syntax.ArithCall) (arithNum, error) {
	fn, ok := r.mathFuncs[x.Name]
	if !ok {
		return intNum(0), arithError{
			msg:      Wording(r.diag().MathFunctionUnknown, "unknown function: %s", x.Name),
			complete: true,
		}
	}
	if len(x.Args) < fn.min || (fn.max != mathFuncUnbounded && len(x.Args) > fn.max) {
		return intNum(0), arithError{
			msg:      Wording(r.diag().MathFunctionArgumentCount, "wrong number of arguments: %s", x.Text),
			complete: true,
		}
	}
	// The arguments are evaluated before the call and passed as *strings*,
	// which is what the implementation sees in `$1`, `$2` and `$*`. Written
	// the way this dialect writes a number, so a float arrives spelled the
	// way the shell spells one.
	args := make([]string, len(x.Args))
	for i, arg := range x.Args {
		v, err := r.evalNum(arg)
		if err != nil {
			return intNum(0), err
		}
		args[i] = r.formatNum(v)
	}
	body, ok := r.funcs[fn.impl]
	if !ok {
		return intNum(0), arithError{
			msg:      Wording(r.diag().MathFunctionMissingImpl, "no such function: %s", fn.impl),
			complete: true,
		}
	}
	// And the call runs, under the shell's own context: an expression is
	// evaluated in the middle of a command and there is no other one to
	// hand it. The name the call is *known by* rather than the
	// implementation's goes onto the frame, because that is what `$0`
	// answers with inside it.
	if err := runMathFuncBody(r, body, x.Name, args); err != nil {
		return intNum(0), err
	}
	// Whatever arithmetic evaluated last during the call, which is the
	// contract measured at the top of this file. The arguments above are
	// part of "during the call", which is why an implementation that
	// evaluates nothing hands back its last argument.
	return r.lastArith, nil
}

// mathFuncOperands reads the operands of a `functions -M` registration:
// `name [min [max [impl]]]`. The bool is false when it reported.
func (r *Runner) mathFuncOperands(builtin string, args []string) (string, mathFunc, bool) {
	if len(args) > 4 {
		r.diagf("%s\n", Wording(r.diag().MathFunctionTooManyOperands, "%s: -M: too many arguments", builtin))
		return "", mathFunc{}, false
	}
	name := args[0]
	if !mathFuncNameValid(name) {
		r.diagf("%s\n", Wording(r.diag().MathFunctionBadName, "%[1]s: -M %[2]s: bad math function name", builtin, name))
		return "", mathFunc{}, false
	}
	fn := mathFunc{min: 0, max: mathFuncUnbounded, impl: name}
	if len(args) > 1 {
		n, err := strconv.Atoi(args[1])
		if err != nil || n < 0 {
			r.diagf("%s\n", Wording(r.diag().MathFunctionBadMinimum,
				"%[1]s: -M: invalid min number of arguments: %[2]s", builtin, args[1]))
			return "", mathFunc{}, false
		}
		// A minimum on its own is also the maximum: `functions -M mf 1`
		// takes exactly one argument and refuses `mf()` and `mf(1,2)`.
		fn.min, fn.max = n, n
	}
	if len(args) > 2 {
		n, err := strconv.Atoi(args[2])
		if err != nil || (n < fn.min && n != mathFuncUnbounded) {
			r.diagf("%s\n", Wording(r.diag().MathFunctionBadMaximum,
				"%[1]s: -M: invalid max number of arguments: %[2]s", builtin, args[2]))
			return "", mathFunc{}, false
		}
		fn.max = n
	}
	if len(args) > 3 {
		fn.impl = args[3]
	}
	return name, fn, true
}

// mathFunctionsBuiltin is the `-M` and `+M` halves of the builtin that spells
// them, with the letter's sign and the operands already separated out.
//
// Returned status is the builtin's. Every path here is 0 or 1 and there is no
// third: a registration succeeds or is refused for one of three reasons, a
// removal is always 0, and a listing is always 0.
func (r *Runner) mathFunctionsBuiltin(builtin string, remove bool, args []string) int {
	if remove {
		for _, name := range args {
			r.removeMathFunc(name)
		}
		return 0
	}
	if len(args) == 0 {
		for _, line := range r.mathFuncListing() {
			r.printf("%s\n", line)
		}
		return 0
	}
	name, fn, ok := r.mathFuncOperands(builtin, args)
	if !ok {
		return 1
	}
	r.registerMathFunc(name, fn)
	return 0
}

// mathLetterVerdict says what a `functions` invocation's option letters mean
// for the math-function facility.
type mathLetterVerdict int

const (
	// mathLetterAbsent is no `-M` and no `+M`: an ordinary listing.
	mathLetterAbsent mathLetterVerdict = iota
	// mathLetterAlone is `-M` or `+M` and no other letter, which is every
	// registration, listing and removal.
	mathLetterAlone
	// mathLetterWithMatching is `-M` together with `-m`, whether bundled or
	// written as two words. Measured: status 0 and *nothing happens* — no
	// registration is made and no listing is written, so `functions -Mm mf 1
	// 1 g` leaves `mf` an unknown function. Its own verdict rather than a
	// registration, because registering there is the plausible wrong answer
	// and it is silent.
	mathLetterWithMatching
	// mathLetterMixed is `-M` with any other letter, which the shell refuses
	// outright as `invalid option(s)`. Not this path: the letters go to the
	// option parser, which refuses the other letter by name.
	mathLetterMixed
)

// mathFunctionLetter reads the leading option words for a `-M` or `+M` and
// says which sign it carried, with the operands that follow.
//
// A `--` after the options ends them, which is the spelling the plugin manager
// uses: `functions -M -- zi_scheduler_add 1 1 -zi_scheduler_add_sh`.
func mathFunctionLetter(args []string) (remove bool, rest []string, verdict mathLetterVerdict) {
	letters, sign := "", byte(0)
	i := 0
	for ; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			i++
			break
		}
		if len(arg) < 2 || (arg[0] != '-' && arg[0] != '+') {
			break
		}
		letters += arg[1:]
		if strings.ContainsRune(arg[1:], 'M') {
			sign = arg[0]
		}
	}
	if sign == 0 {
		return false, nil, mathLetterAbsent
	}
	verdict = mathLetterAlone
	for _, c := range letters {
		switch c {
		case 'M':
		case 'm':
			if verdict == mathLetterAlone {
				verdict = mathLetterWithMatching
			}
		default:
			return false, nil, mathLetterMixed
		}
	}
	return sign == '+', args[i:], verdict
}

// mathFuncNameValid is the name a registration will take: an identifier, and
// nothing else. Measured, `functions -M 1bad …` and `functions -M 'a b' …`
// are both `bad math function name` at status 1, and a `*` in the operand is
// refused rather than read as a pattern — which is what says `functions -M`
// with operands is always a registration and never a filtered listing.
func mathFuncNameValid(name string) bool {
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '_' || isLetter(c) || (i > 0 && isDigit(c)) {
			continue
		}
		return false
	}
	return name != ""
}

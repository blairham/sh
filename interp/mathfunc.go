// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"math"
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

// The context is the shell's own, r.ctx: an expression is evaluated in the
// middle of a command and there is no other one to hand it. Passing
// context.Background() instead is an **equivalent mutant** under this
// package's tests and deliberately left as one — cancellation is noticed
// through a field on the Runner rather than through the context (see
// cancel.go), so the only difference is a child process started inside a math
// function no longer dying with the caller's context, and no cheap test
// discriminates that from a test that merely takes a while.
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
	// native is set instead when the implementation is written in Go rather
	// than in shell — see mathnative.go, which is the other way into this
	// same table. A registration has one or the other and never both.
	native MathFunction
	// stringArg is the `-s` registration: the text between the call's
	// parentheses arrives as **one** argument, unevaluated, instead of the
	// arguments being evaluated and passed as numbers. See
	// Runner.mathFuncStringArgument.
	stringArg bool
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
//
// A string registration is written back with the letters it was made with —
// `functions -Ms sf 1` — so the line still makes the registration it
// describes. Measured 2026-09-26 on zsh 5.9.2, in the same listing as the
// ordinary ones and in the same order.
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
		fn := r.mathFuncs[name]
		letters := "-M"
		if fn.stringArg {
			letters = "-Ms"
		}
		out = append(out, "functions "+letters+" "+mathFuncSpec(name, fn))
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
// The three failures are separate sentences in the shells that have them, and
// they are separate because they are separate questions: a name nobody
// registered, a registration called with the wrong count, and a registration
// whose implementation is not there. Each is handed both extents the panel
// blames over — the call as written and the expression around it — because
// the two columns pick different ones; see Diagnostics.MathFunctionUnknown.
//
// The lookup comes first, and that order is measured rather than incidental:
// `$(( nosuchmf() ))` is `unknown function` in ksh93 where `$(( sqrt() ))` is
// a syntax error, so a name that is not there is answered before its argument
// list is weighed at all.
func (r *Runner) evalMathFunc(x *syntax.ArithCall) (arithNum, error) {
	fn, ok := r.mathFuncs[x.Name]
	if ok && r.MathFunctionWithdrawn(x.Name) {
		// A name the shell has and a module selection is not offering. It
		// answers exactly as a name nobody registered does, which is the
		// measured row — see withdrawnmathfunc.go.
		ok = false
	}
	if !ok {
		return intNum(0), arithError{
			msg: Wording(r.diag().MathFunctionUnknown, "unknown function: %[1]s",
				x.Name, x.Within[x.Offset:]),
			complete: true,
		}
	}
	if fn.stringArg {
		// The whole argument text, unevaluated and as one argument — so
		// there is no count to check and nothing to evaluate. See
		// mathFuncStringArgument.
		return r.callMathFunc(fn, x.Name, []string{mathFuncStringArgument(x.Text)})
	}
	if x.ArgumentsAreText {
		// The reader kept the bytes rather than an argument list, because
		// only this line knows whether the registration wanted them raw —
		// see syntax.ArithCall.ArgumentsAreText. This one did not, so the
		// text is read now, as an expression, and the complaint is the one
		// an expression read at run time gets: measured 2026-09-26, `of() {
		// REPLY=$((1)); }; functions -M of; : $(( of( a , b c ) ))` is
		// ``bad math expression: operator expected at `c '`` in zsh 5.9.2,
		// which is the same sentence `v='a , b c'; : $(( $v ))` gets there
		// and here.
		//
		// It cannot come back a tree: this is the same reader over the same
		// bytes from the same offset, and it is here only because that
		// reader already refused them. The branch is written all the same,
		// because "cannot fail" is what a missing check always says.
		text := mathFuncStringArgument(x.Text)
		if _, err := r.arithTreeRead(text); err != nil {
			return intNum(0), arithError{msg: r.expressionFailure(x.Within, err), complete: true}
		}
		return intNum(0), arithError{msg: Wording(r.diag().ArithOperandExpected, "operand expected")}
	}
	if len(x.Args) == 0 && fn.min > 0 && r.diag().MathFunctionNoArgumentIsASyntaxError {
		// A known name with nothing between its parentheses, in the dialect
		// that reads that as an operand missing rather than as a count. Not
		// complete: ArithError wraps it, which is what puts the expression
		// in front of the reason the way every other syntax failure there is
		// worded.
		return intNum(0), arithError{msg: Wording(r.diag().ArithOperandExpected, "operand expected")}
	}
	if len(x.Args) < fn.min || (fn.max != mathFuncUnbounded && len(x.Args) > fn.max) {
		return intNum(0), arithError{
			msg: Wording(r.diag().MathFunctionArgumentCount, "wrong number of arguments: %[1]s",
				x.Text, x.Within),
			complete: true,
		}
	}
	if fn.native != nil {
		// Written in Go, and handed the operands unevaluated: see
		// mathnative.go for the one function in the module this exists for
		// whose operand is a name rather than a number.
		return r.evalNativeMathFunc(fn, x)
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
	return r.callMathFunc(fn, x.Name, args)
}

// callMathFunc runs a registration's implementation with the arguments
// already settled, and answers with what the call is worth.
//
// One place for both readings of the operands — the evaluated numbers and the
// `-s` form's single unevaluated string — because everything past the
// arguments is the same for the two: the same lookup of the implementation,
// the same sentence when it is not there, the same name on the frame, and the
// same answer. Two copies is how the string form would come to miss a fix the
// other one carries.
func (r *Runner) callMathFunc(fn mathFunc, name string, args []string) (arithNum, error) {
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
	if err := runMathFuncBody(r, body, name, args); err != nil {
		return intNum(0), err
	}
	// Whatever arithmetic evaluated last during the call, which is the
	// contract measured at the top of this file. The arguments above are
	// part of "during the call", which is why an implementation that
	// evaluates nothing hands back its last argument.
	return r.lastArith, nil
}

// mathFuncStringArgument is what a `-s` registration is handed: the text
// between the call's parentheses, exactly as it was written.
//
// Exactly — blanks and commas included. Measured 2026-09-26 on zsh 5.9.2 with
// an implementation printing `$#` and `$1`:
//
//	sf(2+2)          n=1  1=[2+2]          — unevaluated, so not 4
//	sf(1,2)          n=1  1=[1,2]          — one argument, comma and all
//	sf()             n=1  1=[]             — still one, and it is empty
//	sf( a , b c )    n=1  1=[ a , b c ]    — the blanks are kept
//
// The first row is the discriminating one: an evaluated argument would be `4`,
// and an implementation reading `${#1}` can tell the two apart where one
// reading `$1` numerically cannot.
//
// Taken from the call's own source text rather than rebuilt from the tree,
// which could not reproduce the blanks. The text runs from the first byte of
// the name through the closing parenthesis, so what is wanted is between the
// first `(` and the last `)`.
func mathFuncStringArgument(text string) string {
	open := strings.IndexByte(text, '(')
	shut := strings.LastIndexByte(text, ')')
	if open < 0 || shut < open {
		return ""
	}
	return text[open+1 : shut]
}

// mathFuncOperands reads the operands of a `functions -M` registration:
// `name [min [max [impl]]]`. The bool is false when it reported.
//
// stringArg is the `-s` form, whose arity is not free: the whole argument text
// is one string, so the registration takes exactly one argument and any other
// count is refused. Measured 2026-09-26 on zsh 5.9.2 — `functions -Ms n 1` and
// `functions -Ms n 1 1` are taken at 0, `functions -Ms n 0`, `-Ms n 2` and
// `-Ms n 1 -1` are each `-Ms: must take a single string argument` at 1 with
// nothing registered, and the implementation operand is still read, so
// `functions -Ms n 1 1 other` stands.
func (r *Runner) mathFuncOperands(builtin string, args []string, stringArg bool) (string, mathFunc, bool) {
	name := args[0]
	if !mathFuncNameValid(name) {
		r.diagf("%s\n", Wording(r.diag().MathFunctionBadName, "%[1]s: -M %[2]s: bad math function name", builtin, name))
		return "", mathFunc{}, false
	}
	fn := mathFunc{min: 0, max: mathFuncUnbounded, impl: name}
	if stringArg {
		// The one argument the letter names, whether or not the arity was
		// written out — which is why a listing of `functions -Ms sf` says
		// `sf 1` where the ordinary form with no arity says `sf`.
		fn.stringArg = true
		fn.min, fn.max = 1, 1
	}
	if len(args) > 1 {
		n, ok := mathFuncCount(args[1])
		if !ok || n < 0 {
			r.diagf("%s\n", Wording(r.diag().MathFunctionBadMinimum,
				"%[1]s: -M: invalid min number of arguments: %[2]s", builtin, args[1]))
			return "", mathFunc{}, false
		}
		// A minimum on its own is also the maximum: `functions -M mf 1`
		// takes exactly one argument and refuses `mf()` and `mf(1,2)`.
		fn.min, fn.max = n, n
		// And only then the arity the letter fixes, because an operand that
		// is not a count at all earns a different complaint and earns it
		// first. The rule is applied per operand rather than once at the end:
		// `functions -Ms mf 2 1` is the arity here, where a maximum below the
		// minimum would otherwise be reported, and `functions -Ms mf 1 0` is
		// the maximum, because that operand's own arity is never reached.
		// Measured 2026-09-27 on zsh 5.9.2 (#4791).
		if !r.mathFuncStringArity(builtin, fn, stringArg) {
			return "", mathFunc{}, false
		}
	}
	if len(args) > 2 {
		n, ok := mathFuncCount(args[2])
		if !ok || (n < fn.min && n != mathFuncUnbounded) {
			r.diagf("%s\n", Wording(r.diag().MathFunctionBadMaximum,
				"%[1]s: -M: invalid max number of arguments: %[2]s", builtin, args[2]))
			return "", mathFunc{}, false
		}
		fn.max = n
		if !r.mathFuncStringArity(builtin, fn, stringArg) {
			return "", mathFunc{}, false
		}
	}
	if len(args) > 4 {
		r.diagf("%s\n", Wording(r.diag().MathFunctionTooManyOperands, "%s: -M: too many arguments", builtin))
		return "", mathFunc{}, false
	}
	if len(args) > 3 {
		fn.impl = args[3]
	}
	return name, fn, true
}

// mathFuncStringArity applies the rule that a `-Ms` registration takes exactly
// one argument, and is false when it reported. It is a no-op for the ordinary
// form, whose arity is whatever was written.
func (r *Runner) mathFuncStringArity(builtin string, fn mathFunc, stringArg bool) bool {
	if !stringArg || (fn.min == 1 && fn.max == 1) {
		return true
	}
	r.diagf("%s\n", Wording(r.diag().MathFunctionStringArity,
		"%s: -Ms: must take a single string argument", builtin))
	return false
}

// mathFuncCount reads an operand that says how many arguments a registration
// takes. The bool is false when the spelling is not a count at all.
//
// It is not [strconv.Atoi], and the difference is the whole of why it is a
// function rather than a call: this shell skips leading blanks, takes an
// optional sign, and then reads digits in whichever base the spelling
// announces — `0x` hexadecimal, `0b` binary, a bare leading `0` octal,
// anything else decimal. A spelling that runs out before any digit is
// **zero** rather than a refusal, and anything left standing after the digits
// refuses the operand.
//
// Measured 2026-09-27 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) run `-f`,
// through `functions -M mf <operand>` and the listing it leaves:
//
//	0x10      16        007     7         ""      0     1abc    refused
//	0X10      16        01      1         " "     0     1_0     refused
//	0b1       1         0       0         "+"     0     08      refused
//	"\n1"     1         -0      0         "-"     0     " 1 "   refused
//	"\t1"     1         -1      -1        0x      0     1+1     refused
//	"  0x1f"  31        +1      1                       1.5     refused
//	                                                    8#10    refused
//
// A `-1` is read and then refused by the caller, which is where the sign
// means something: a minimum may not be negative, and a maximum of -1 is
// [mathFuncUnbounded] rather than a refusal.
//
// An operand too large to hold is refused. The reference truncates it and
// then refuses it too, with a warning line of its own first, which is a
// different question and not this one.
func mathFuncCount(s string) (int, bool) {
	i := 0
	for i < len(s) && isMathCountBlank(s[i]) {
		i++
	}
	negative := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		negative = s[i] == '-'
		i++
	}
	base := 10
	switch {
	case i+1 < len(s) && s[i] == '0' && (s[i+1] == 'x' || s[i+1] == 'X'):
		base, i = 16, i+2
	case i+1 < len(s) && s[i] == '0' && (s[i+1] == 'b' || s[i+1] == 'B'):
		base, i = 2, i+2
	case i < len(s) && s[i] == '0':
		base, i = 8, i+1
	}
	n := 0
	for i < len(s) {
		d := mathCountDigit(s[i])
		if d < 0 || d >= base {
			break
		}
		if n > (math.MaxInt-d)/base {
			return 0, false
		}
		n = n*base + d
		i++
	}
	if i != len(s) {
		return 0, false
	}
	if negative {
		n = -n
	}
	return n, true
}

// isMathCountBlank is the run a count operand may begin with. The three
// measured are a space, a tab and a newline; the other three are the rest of
// what a C library calls a blank, and none of them can stand in a word that
// reached a builtin by any other route.
func isMathCountBlank(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

// mathCountDigit is one digit's value in the widest base a count operand can
// announce, or -1 for a byte that is not a digit at all.
func mathCountDigit(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// mathFunctionsBuiltin is the `-M` and `+M` halves of the builtin that spells
// them, with the letter's sign and the operands already separated out.
//
// Returned status is the builtin's. Every path here is 0 or 1 and there is no
// third: a registration succeeds or is refused for one of three reasons, a
// removal is always 0, and a listing is always 0.
func (r *Runner) mathFunctionsBuiltin(builtin string, remove, stringArg bool, args []string) int {
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
	name, fn, ok := r.mathFuncOperands(builtin, args, stringArg)
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
	// mathLetterMixed is `-M` with a letter it does not compose with, which
	// the shell refuses outright as `invalid option(s)` — a complaint that
	// names no letter, because what is wrong is the *set*.
	//
	// **The doc here used to say "not this path: the letters go to the option
	// parser, which refuses the other letter by name", and that was an
	// argument rather than a measurement.** It is right only for a letter
	// this shell has not got: `functions -Ma …` really is `bad option: -a` in
	// the reference. For a letter it *has*, the reference says `invalid
	// option(s)` and registers nothing, where falling through registered the
	// math function in silence at 0 (#5073).
	//
	// Measured 2026-09-28 on zsh 5.9.2, `g(){ : }` then
	// `functions -M<letter> mf 1 1 g`, every letter of the alphabet in both
	// cases:
	//
	//	-Mm -Ms -MM          0, and the registration is the ordinary one
	//	-Mk -Mt -Mu -Mz      invalid option(s), 1, nothing registered
	//	-MT -MU -MW          the same
	//	-Ma … (not a letter) bad option: -a, 1 — the letter is named
	//	-Mx                  number expected after -x, 1
	//	-Mc                  -c: requires two arguments, 1
	//
	// The order is the same one read twice: a letter the shell has not got is
	// refused by name first, then a letter that **takes an argument** is read
	// and can refuse for its own reason, and only then is the set judged. So
	// the last two rows are not exceptions to exclusivity — they never reach
	// it. See mathFunctionLetter, where `x` has already been taken off the
	// words by the time the set is read.
	mathLetterMixed
)

// mathFunctionLetter reads the leading option words for a `-M` or `+M` and
// says which sign it carried, with the operands that follow.
//
// A `--` after the options ends them, which is the spelling the plugin manager
// uses: `functions -M -- zi_scheduler_add 1 1 -zi_scheduler_add_sh`.
func mathFunctionLetter(args []string) (remove, stringArg bool, rest []string, verdict mathLetterVerdict) {
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
		return false, false, nil, mathLetterAbsent
	}
	verdict = mathLetterAlone
	for _, c := range letters {
		switch c {
		case 'M':
		case 's':
			// The string form, which composes with `-M` in either order and
			// in either spelling: measured, `-M -s`, `-s -M`, `-Ms` and
			// `-sM` all register the same thing.
			stringArg = true
		case 'm':
			if verdict == mathLetterAlone {
				verdict = mathLetterWithMatching
			}
		default:
			// The letter that made the set wrong, carried out so the caller
			// can tell a letter this shell *has* — `invalid option(s)` —
			// from one it has not, which is refused by name further down.
			return false, false, []string{string(c)}, mathLetterMixed
		}
	}
	return sign == '+', stringArg, args[i:], verdict
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

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
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
	"set":      biSet,
	"unset":    biUnset,
	"export":   biExport,
	"shift":    biShift,
	"echo":     biEcho,
	"cd":       biCd,
	"pwd":      biPwd,
	"read":     biRead,
	"wait":     biWait,
	"exit":     biExit,
	"trap":     biTrap,
	"local":    biLocal,
	"typeset":  biDeclare,
	"readonly": biReadonly,
	"break":    biBreak,
	"continue": biContinue,
	"return":   biReturn,
	// `eval` and `.` are added in source.go's init rather than here — they
	// run arbitrary shell, so they reach the dispatcher that reads this map,
	// and Go calls a literal that closes that loop an initialization cycle.
}

// biBreak and biContinue transfer control out of a loop. They are recorded on
// the runner rather than returned as errors, because leaving a loop is
// ordinary control flow and modeling it as a failure would make every caller
// check for something that is not one.
func biBreak(r *Runner, _ context.Context, args []string) int {
	r.ctl, r.ctlDepth = controlBreak, loopDepth(args)
	return 0
}

func biContinue(r *Runner, _ context.Context, args []string) int {
	r.ctl, r.ctlDepth = controlContinue, loopDepth(args)
	return 0
}

func biReturn(r *Runner, _ context.Context, args []string) int {
	r.ctl = controlReturn
	if len(args) > 0 {
		if n, ok := atoi(args[0]); ok {
			return n
		}
	}
	return r.status
}

func loopDepth(args []string) int {
	if len(args) > 0 {
		if n, ok := atoi(args[0]); ok && n > 0 {
			return n
		}
	}
	return 1
}

// specialBuiltins are the ones POSIX marks special. Two consequences follow
// from the same list — an assignment prefixed to one persists, and a failure
// in one is fatal to a non-interactive shell — so it is one concept rather
// than two lists that could drift.
var specialBuiltins = map[string]bool{
	"break": true, ":": true, "continue": true, ".": true, "eval": true,
	"exec": true, "exit": true, "export": true, "readonly": true,
	"return": true, "set": true, "shift": true, "times": true,
	"trap": true, "unset": true,
}

func biTrue(*Runner, context.Context, []string) int  { return 0 }
func biFalse(*Runner, context.Context, []string) int { return 1 }

// biSet implements the part of `set` this slice needs: replacing the
// positional parameters.
//
// `set --` with nothing after it clears them, which is different from `set`
// with no arguments at all — that lists variables and is left unimplemented
// rather than guessed at.
func biSet(r *Runner, _ context.Context, args []string) int {
	if len(args) == 0 {
		r.diagf("set: listing variables is not implemented yet\n")
		return 2
	}
	// Options come before `--`, and each is a letter that may be turned on
	// with `-` or off with `+`. Only the ones with implemented behavior are
	// accepted; the rest are refused rather than silently ignored, which
	// would let a script believe it had asked for something.
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			i++
			break
		}
		if len(a) < 2 || (a[0] != '-' && a[0] != '+') {
			break
		}
		on := a[0] == '-'
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
		if letters, ok := strings.CutSuffix(a[1:], "o"); ok {
			if !r.setLetters(letters, on) {
				return 2
			}
			if i+1 >= len(args) {
				r.diagf("set: -o needs an option name\n")
				return 2
			}
			i++
			if !r.setOption(args[i], on) {
				return 2
			}
			continue
		}
		if !r.setLetters(a[1:], on) {
			return 2
		}
	}
	// `set -C` alone sets an option and leaves the parameters alone; only an
	// explicit `--`, or operands after the options, replaces them.
	if i == 0 || (i <= len(args) && args[min(i-1, len(args)-1)] == "--") || i < len(args) {
		r.Params = append([]string(nil), args[i:]...)
	}
	return 0
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
		switch opt {
		case 'C':
			r.noclobber = on
		case 'e':
			r.errexit = on
		case 'u':
			r.nounset = on
		case 'x':
			r.xtrace = on
		case 'f':
			// Not universal: one shell spells this option the long way only
			// and uses `-f` for something else, which does not touch
			// globbing. Asked rather than assumed, and only here — `set -o
			// noglob` needs no dialect.
			if r.ask(r.sem().SetFTurnsOffGlobbing, "`set -f` turning off pathname expansion") {
				r.noglob = on
			}
		default:
			r.diagf("set: -%c is not implemented\n", opt)
			return false
		}
	}
	return true
}

// setOption applies `set -o name`, reporting whether the name is one we have.
//
// The names are the same in every shell in the panel, which is what makes the
// long form the one that needs no dialect: `set -o noglob` means the same
// thing in all four where `set -f` does not.
func (r *Runner) setOption(name string, on bool) bool {
	switch name {
	case "errexit":
		r.errexit = on
	case "nounset":
		r.nounset = on
	case "xtrace":
		r.xtrace = on
	case "noclobber":
		r.noclobber = on
	case "noglob":
		r.noglob = on
	case "pipefail":
		// The one name here that is not unanimous.
		if r.ask(r.sem().PipefailOption, "`set -o pipefail`") {
			r.pipefail = on
			return true
		}
		if r.unspecified {
			// The refusal is already reported. A second complaint about the
			// same word would only obscure it.
			return true
		}
		// A definite no: the name is not an option in this dialect, and is
		// reported exactly as any other name this shell does not have.
		fallthrough
	default:
		r.diagf("set: %s: invalid option name\n", name)
		return false
	}
	return true
}

func biUnset(r *Runner, _ context.Context, args []string) int {
	args, opts, code := r.builtinOptions("unset", args, "vfn")
	if code != 0 {
		return code
	}
	if strings.ContainsRune(opts, 'f') {
		// `unset -f` is about functions and not about variables, unanimously
		// — and the option was read and then ignored, so a function survived
		// being unset and went on answering to its name. The exported set
		// goes with it: what is not a function cannot be carried as one.
		for _, name := range args {
			delete(r.funcs, name)
			delete(r.funcFiles, name)
			delete(r.exportedFuncs, name)
		}
		return 0
	}
	for _, name := range args {
		delete(r.Vars, name)
		delete(r.exported, name)
		// Recorded as well as deleted: a name that came from the environment
		// is not in Vars to begin with, and deleting nothing left it visible
		// to every lookup — `unset PATH` did not clear PATH.
		if r.removed == nil {
			r.removed = map[string]bool{}
		}
		r.removed[name] = true
	}
	return 0
}

// biExport marks a name for the environment, and assigns when given a value.
func biExport(r *Runner, _ context.Context, args []string) int {
	// `-f` is offered only where the dialect has it. Where it does not, it
	// goes through the ordinary unknown-option path and gets that shell's
	// own refusal, which in two of them ends the script.
	letters := "pn"
	// Asked only where there is an `-f` to decide about. `export A=1` is the
	// same in all four, and refusing it over a question nothing turned on
	// would be refusing to export anything.
	if hasOption(args, 'f') {
		carries := r.ask(r.sem().ExportCarriesFunctions, "`export -f`")
		if r.unspecified {
			return 2
		}
		switch {
		case carries:
			letters = "pfn"
		case r.diag().ExportFunctionOptionRefused != "":
			// A dialect that knows the letter and will not do it, which is
			// not the same as one that has never heard of it — and says so
			// in different words.
			r.diagf("%s\n", r.diag().ExportFunctionOptionRefused)
			return 1
		}
	}
	args, opts, code := r.builtinOptions("export", args, letters)
	if code != 0 {
		return code
	}
	if r.exported == nil {
		r.exported = map[string]bool{}
	}
	if strings.ContainsRune(opts, 'f') {
		return r.exportFuncs(args)
	}
	for _, a := range args {
		if strings.ContainsRune(opts, 'p') {
			r.diagf("export: -p is not implemented yet\n")
			return 2
		}
		name, value, hasValue := strings.Cut(a, "=")
		if hasValue {
			r.setVar(name, value)
		}
		r.exported[name] = true
	}
	return 0
}

// biShift drops the first n positional parameters.
//
// Shifting past the end is where the panel splits: fatal in dash and ksh93,
// survivable in bash and zsh. The dialect answers it rather than this taking
// a side.
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
	if len(args) > 0 {
		v, ok := atoi(args[0])
		if !ok {
			r.diagf("shift: %s: numeric argument required\n", args[0])
			return 2
		}
		n = v
	}
	if n > len(r.Params) {
		// Fatal in dash and ksh93, survivable in bash and zsh.
		if r.ask(r.sem().ShiftPastEndFatal, "shift past the end being fatal") {
			// controlReturn only unwound a function, so at the top level the
			// script carried on past an error the shell calls fatal.
			r.fatal("%s\n", Wording(r.diag().ShiftTooMany, "shift: can't shift that many", n))
			return r.status
		}
		// Survivable, and still worth saying where the dialect says it: zsh
		// prints its complaint and carries on, and bash prints nothing at
		// all. No fallback here for that reason — an empty wording is bash's
		// answer rather than a dialect that has not been asked.
		if w := r.diag().ShiftTooMany; w != "" {
			r.diagf("%s\n", Wording(w, w, n))
		}
		return 1
	}
	r.Params = r.Params[n:]
	return 0
}

// biEcho writes its arguments separated by spaces.
//
// It does not interpret backslash escapes. That is the bash and ksh93
// answer; dash and zsh expand them, which docs/spec/semantics.md records as
// an axis, and taking the majority here is a placeholder rather than a
// decision — the dialect will decide once the interpreter carries one.
func biEcho(r *Runner, _ context.Context, args []string) int {
	newline := true
	for len(args) > 0 && args[0] == "-n" {
		newline = false
		args = args[1:]
	}
	out := strings.Join(args, " ")
	// dash and zsh expand backslash escapes without -e; bash and ksh93 do
	// not. A grouping no other axis produces.
	// Asked only when the text could differ either way, so `echo hi` needs no
	// dialect and `echo 'a\tb'` does.
	if strings.ContainsRune(out, '\\') && r.ask(r.sem().EchoInterpretsEscapes, "echo interpreting backslash escapes") {
		out = expandEchoEscapes(out)
	}
	if newline {
		out += "\n"
	}
	_, _ = r.stdout().Write([]byte(out))
	return 0
}

// expandEchoEscapes interprets the escapes `echo` expands where the dialect
// says it does.
func expandEchoEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// biCd changes the shell's working directory.
//
// This is the definition of a core primitive: it changes the runner's own
// state, every dialect needs it, and no shell function can say it. It lived
// in cmd/bash while that binary was demonstrating Register, which was the
// right place for a demonstration and the wrong one to leave it.
func biCd(r *Runner, _ context.Context, args []string) int {
	dir := ""
	if len(args) > 0 {
		dir = args[0]
	}
	dash := false
	switch dir {
	case "":
		dir, _ = r.getVar("HOME")
		if dir == "" {
			code, _ := r.cdNowhere(r.diag().CdHomeNotSet, "cd: HOME not set")
			// With no HOME there is nowhere to go even for the dialects that
			// do not call it an error, so this stops either way.
			return code
		}
	case "-":
		// The previous directory, which is why cd records one.
		dash = true
		dir, _ = r.getVar("OLDPWD")
		if dir == "" {
			if code, stop := r.cdNowhere(r.diag().CdOldpwdNotSet, "cd: OLDPWD not set"); stop {
				return code
			}
			// Not an error here, and not nothing either: the dialects that
			// survive this go to where they already are, which still prints
			// for the ones that print.
			dir = r.workDir()
		}
	}

	old := r.workDir()
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(old, dir)
	}
	info, err := os.Stat(dir)
	if err == nil && !info.IsDir() {
		err = &os.PathError{Op: "cd", Path: dir, Err: syscall.ENOTDIR}
	}
	if err != nil {
		// The reason the operating system gave, rather than one made up
		// here: three of the four report it, and two of those distinguish a
		// path that is not there from one that is not a directory. Saying
		// "no such directory" for both was a sentence no shell prints and an
		// answer one of them can tell is wrong.
		r.diagf("%s\n", Wording(r.diag().CdCannotChange, "cd: %[1]s: %[2]s",
			args[0], r.diag().reasonText(reason(err))))
		return orDefault(r.diag().CdStatus, 1)
	}
	// Only the runner's own directory moves. Calling os.Chdir would move the
	// whole process, which is wrong for an embedded interpreter and would be
	// shared by every Runner in it.
	r.Dir = dir
	r.setVar("OLDPWD", old)
	r.setVar("PWD", dir)
	if dash && r.ask(r.sem().CdDashPrintsTheDirectory, "`cd -` printing where it went") {
		// Asked only for `cd -`, which is the only form any of them prints.
		r.printf("%s\n", dir)
	}
	return 0
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

func biPwd(r *Runner, _ context.Context, _ []string) int {
	_, _ = fmt.Fprintln(r.stdout(), r.workDir())
	return 0
}

// biRead reads a line into variables.
//
// Also a primitive by the same test: it has to set a variable in the *calling*
// shell, which a child process cannot reach.
//
// Without -r a backslash escapes the character after it, including a newline,
// which is why -r is what scripts should use and rarely do.
func biRead(r *Runner, _ context.Context, args []string) int {
	raw := false
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		if args[0] == "-r" {
			raw = true
		}
		args = args[1:]
	}
	if len(args) == 0 {
		args = []string{"REPLY"}
	}

	line, atEOF := r.readLine(raw)

	// A `read` that fails still assigns. All four shells clear the variables
	// at end of input rather than leaving what was there, and the reason is
	// the loop everyone writes: `while read -r l` leaves `l` behind, and a
	// stale value after the loop reads as the last line rather than as
	// nothing. Assigning happens before the status is decided, not instead
	// of it.

	// The last variable takes the whole remainder, which is what makes
	// `read a b` put "c d" in b for input "a c d".
	ifs, set := r.ifs()
	fields := splitFields(line, ifs, set)
	for i, name := range args {
		switch {
		case i >= len(fields):
			r.setVar(name, "")
		case i == len(args)-1:
			r.setVar(name, strings.Join(fields[i:], " "))
		default:
			r.setVar(name, fields[i])
		}
	}
	if atEOF {
		return 1
	}
	return 0
}

// readLine reads one line, honoring a line continuation unless raw, and says
// whether the input ended.
//
// The two are separate answers because a final line with no newline is both:
// there is a line, and there will not be another. All four shells assign it
// and report failure, which is what stops `while read -r l` from running a
// last unterminated line twice — once as the line, once as the empty read
// after it.
func (r *Runner) readLine(raw bool) (line string, atEOF bool) {
	var b strings.Builder
	var ch [1]byte
	in := r.In()
	for {
		n, err := in.Read(ch[:])
		if n == 0 || err != nil {
			return b.String(), true
		}
		c := ch[0]
		if c == '\n' {
			s := b.String()
			// A trailing backslash continues onto the next line, unless -r.
			if !raw && strings.HasSuffix(s, "\\") {
				b.Reset()
				b.WriteString(strings.TrimSuffix(s, "\\"))
				continue
			}
			return s, false
		}
		b.WriteByte(c)
	}
}

// biLocal makes variables local to the running function.
//
// Shell scoping is *dynamic*, not lexical: a local is visible to everything
// the function calls, and stops existing when the function returns. That is
// why this saves the outer value on a stack rather than creating a new
// environment — there is only ever one set of variables, and `local` says
// which of them to put back.
func biLocal(r *Runner, _ context.Context, args []string) int {
	if len(r.scopes) == 0 {
		r.diagf("local: can only be used in a function\n")
		return 1
	}
	for _, a := range args {
		name, value, hasValue := strings.Cut(a, "=")
		r.shadow(name)
		if hasValue {
			r.setVar(name, value)
			continue
		}
		r.declareEmpty(name)
	}
	return 0
}

// biReadonly marks variables immutable.
func biReadonly(r *Runner, _ context.Context, args []string) int {
	args, _, code := r.builtinOptions("readonly", args, "paAf")
	if code != 0 {
		return code
	}
	for _, a := range args {
		name, value, hasValue := strings.Cut(a, "=")
		if hasValue {
			r.setVar(name, value)
		}
		r.markReadonly(name)
	}
	return 0
}

// biExit ends the shell.
//
// A bare `exit` reports what the last command did, and a status is taken
// modulo 256 because that is all a process can carry — `exit 300` is 44 in
// every shell measured.
func biExit(r *Runner, _ context.Context, args []string) int {
	if len(args) > 0 {
		// strconv rather than the local atoi, which is for file descriptors
		// and rejects a sign — `exit -1` has to parse before it can be
		// judged.
		// strconv rather than the local atoi, which is for file descriptors
		// and rejects a sign — `exit -1` has to parse before it can be
		// judged.
		n, err := strconv.Atoi(strings.TrimSpace(args[0]))
		switch {
		case err == nil && n >= 0:
			r.status = n % 256
		default:
			switch r.exitArgument() {
			case ExitArgStrict:
				return r.badExitArg(args[0])
			case ExitArgNumeric:
				if err != nil {
					return r.badExitArg(args[0])
				}
				r.status = ((n % 256) + 256) % 256
			case ExitArgLenient:
				// Text reads as zero there, which is what `exit abc` gives.
				r.status = ((n % 256) + 256) % 256
			default:
				// No dialect answered; exitArgument has already said so,
				// and the script stops rather than exiting with a status it
				// just refused to choose.
				r.ctl = controlExit
				return r.status
			}
		}
	}
	r.ctl = controlExit
	return r.status
}

// biTrap sets what runs when the shell ends.
//
// Only EXIT is implemented. The other conditions need signal delivery, and
// accepting `trap … INT` without ever firing it would be the silent wrong
// answer this package exists to avoid — so it is refused and says so.
func biTrap(r *Runner, _ context.Context, args []string) int {
	if len(args) == 0 {
		if r.exitTrap != nil {
			r.printf("trap -- %s EXIT\n", singleQuote(*r.exitTrap))
		}
		for _, name := range sortedKeys(r.sigs().traps) {
			r.printf("trap -- %s %s\n", singleQuote(r.sigs().traps[name]), name)
		}
		return 0
	}
	body, conds := args[0], args[1:]
	if len(conds) == 0 {
		r.diagf("trap: usage: trap action condition ...\n")
		return 2
	}
	// Every condition is checked before any is acted on, so a bad one does
	// not leave half the request applied.
	type target struct {
		name string
		sig  syscall.Signal
		exit bool
	}
	targets := make([]target, 0, len(conds))
	for _, c := range conds {
		if strings.EqualFold(c, "EXIT") || c == "0" {
			targets = append(targets, target{exit: true})
			continue
		}
		name, sig, kind := r.canonicalSignal(c)
		switch kind {
		case signalUncatchable:
			// A real signal that nobody can catch. Every shell in the panel
			// takes `trap … KILL` and then never fires it; this refuses
			// instead, which is a deliberate divergence and keeps its own
			// wording, because it is not the dialect's complaint to word.
			r.diagf("trap: %s: not a signal this shell can catch\n", c)
			return 2
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
	for _, tg := range targets {
		switch {
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

// singleQuote renders text the way `trap` lists it, so the output could be
// fed back in.
func singleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// badExitArg reports an argument `exit` will not take.
//
// The status is 2 in both shells that refuse, and it is not the fatal-error
// status: bash exits 1 for a fatal error and 2 for this. A usage error is its
// own thing, which is why it is written here rather than routed through fatal.
func (r *Runner) badExitArg(arg string) int {
	r.diagf("%s\n", Wording(r.diag().NumericArgument, "%[1]s: invalid number: %[2]s", "exit", arg))
	r.status = 2
	r.ctl = controlExit
	return r.status
}

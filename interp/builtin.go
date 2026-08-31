// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"fmt"
	"io"
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
	"readonly": biReadonly,
	"break":    biBreak,
	"continue": biContinue,
	"return":   biReturn,
}

// biBreak and biContinue transfer control out of a loop. They are recorded on
// the runner rather than returned as errors, because leaving a loop is
// ordinary control flow and modelling it as a failure would make every caller
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
	// with `-` or off with `+`. Only the ones with implemented behaviour are
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
		for _, opt := range a[1:] {
			switch opt {
			case 'C':
				r.noclobber = on
			case 'e':
				r.errexit = on
			default:
				r.diagf("set: -%c is not implemented\n", opt)
				return 2
			}
		}
	}
	// `set -C` alone sets an option and leaves the parameters alone; only an
	// explicit `--`, or operands after the options, replaces them.
	if i == 0 || (i <= len(args) && args[min(i-1, len(args)-1)] == "--") || i < len(args) {
		r.Params = append([]string(nil), args[i:]...)
	}
	return 0
}

func biUnset(r *Runner, _ context.Context, args []string) int {
	for _, name := range args {
		if name == "-v" || name == "-f" {
			continue
		}
		delete(r.Vars, name)
		delete(r.exported, name)
	}
	return 0
}

// biExport marks a name for the environment, and assigns when given a value.
func biExport(r *Runner, _ context.Context, args []string) int {
	if r.exported == nil {
		r.exported = map[string]bool{}
	}
	for _, a := range args {
		if a == "-p" {
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
// survivable in bash and zsh. The survivable answer is taken, and the
// difference is a dialect question the interpreter does not yet carry.
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
	switch dir {
	case "":
		dir, _ = r.getVar("HOME")
		if dir == "" {
			r.diagf("cd: HOME not set\n")
			return 1
		}
	case "-":
		// The previous directory, which is why cd records one.
		dir, _ = r.getVar("OLDPWD")
		if dir == "" {
			r.diagf("cd: OLDPWD not set\n")
			return 1
		}
	}

	old := r.workDir()
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(old, dir)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		r.diagf("cd: %s: no such directory\n", args[0])
		return 1
	}
	// Only the runner's own directory moves. Calling os.Chdir would move the
	// whole process, which is wrong for an embedded interpreter and would be
	// shared by every Runner in it.
	r.Dir = dir
	r.setVar("OLDPWD", old)
	r.setVar("PWD", dir)
	return 0
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

	line, err := r.readLine(raw)
	if err != nil {
		return 1
	}

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
	return 0
}

// readLine reads one line, honouring a line continuation unless raw.
func (r *Runner) readLine(raw bool) (string, error) {
	var b strings.Builder
	var ch [1]byte
	in := r.In()
	for {
		n, err := in.Read(ch[:])
		if n == 0 || err != nil {
			if b.Len() > 0 {
				return b.String(), nil
			}
			return "", io.EOF
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
			return s, nil
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
	sc := r.scopes[len(r.scopes)-1]
	for _, a := range args {
		name, value, hasValue := strings.Cut(a, "=")
		if _, seen := sc.saved[name]; !seen {
			old, existed := r.Vars[name]
			sc.saved[name] = old
			sc.existed[name] = existed
		}
		if hasValue {
			r.setVar(name, value)
		} else {
			r.setVar(name, "")
		}
	}
	return 0
}

// biReadonly marks variables immutable.
func biReadonly(r *Runner, _ context.Context, args []string) int {
	if r.readonly == nil {
		r.readonly = map[string]bool{}
	}
	for _, a := range args {
		name, value, hasValue := strings.Cut(a, "=")
		if hasValue {
			r.setVar(name, value)
		}
		r.readonly[name] = true
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
		name, sig, ok := canonicalSignal(c)
		if !ok {
			r.diagf("trap: %s: not a signal this shell can catch\n", c)
			return 2
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
	r.diagf("%s\n", Wording(r.diag().InvalidNumber, "invalid number: %s", arg))
	r.status = 2
	r.ctl = controlExit
	return r.status
}

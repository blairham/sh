// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"

	"github.com/blairham/sh/syntax"
)

// `declare` and `typeset`, which give a name an attribute as well as a value.
//
// Which of the two a shell has is a dialect's answer and not an axis: dash
// has neither, ksh93 has `typeset` alone, and bash and zsh have both under one
// implementation. So they are registered through the extension seam, the same
// way ksh93 registers `source` and takes `local` away.
//
// The attributes are the interesting half. `-r` and `-x` are `readonly` and
// `export` spelled differently and share their machinery, but `-i` is a
// property of the *name* that changes what a later assignment means:
//
//	declare -i n; n=5+2   → 7, because the value is evaluated
//	n=5+2                 → the four characters, because it is not
//
// That is why an attribute has to be recorded against the name rather than
// applied once at the point of declaring it.

// declareFlags is what a declaration asks for.
type declareFlags struct {
	integer  bool
	readonly bool
	export   bool
	remove   bool
}

func biDeclare(r *Runner, _ context.Context, args []string) int {
	var f declareFlags
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		if len(a) < 2 || (a[0] != '-' && a[0] != '+') {
			break
		}
		// `+i` removes the attribute where `-i` adds it, which is the one
		// place a shell spells an option with a plus.
		f.remove = a[0] == '+'
		for _, c := range a[1:] {
			switch c {
			case 'i':
				f.integer = true
			case 'r':
				f.readonly = true
			case 'x':
				f.export = true
			case 'a':
				// Accepted and recorded nowhere. An array here is dynamic,
				// so `typeset -a arr` followed by `arr[0]=x` works without
				// the attribute existing — which is what the flag is used
				// for. The compound form `typeset -a arr=(x y)` is a
				// different thing and is not built: the parser reads the
				// parenthesis as an ordinary word, and making it an array
				// literal in argument position is grammar rather than a
				// flag. Refusing the flag outright would break the common
				// use to be honest about the rare one.
			default:
				r.diagf("declare: -%c: invalid option\n", c)
				return 2
			}
		}
	}

	for _, a := range args[i:] {
		name, value, hasValue := strings.Cut(a, "=")
		// Attributes first, because `-i` changes what the assignment on the
		// same line *means* — but readonly last, because it changes whether
		// that assignment is allowed at all. `declare -r c=1` sets c and then
		// freezes it; applying both up front made the declaration refuse its
		// own value and leave the name empty.
		r.applyAttributes(name, f)
		// Declaring inside a function declares a local, which is unanimous
		// among the three shells that have the name — subject to ksh93's
		// rule about which functions have a scope at all.
		r.shadowTypeset(name)
		switch {
		case hasValue:
			r.setVar(name, value)
		default:
			r.declareEmpty(name)
		}
		if f.readonly && !f.remove {
			r.markReadonly(name)
		}
	}
	return 0
}

// applyAttributes records what a name has been declared to be.
func (r *Runner) applyAttributes(name string, f declareFlags) {
	if f.integer {
		if r.integer == nil {
			r.integer = map[string]bool{}
		}
		if f.remove {
			delete(r.integer, name)
		} else {
			r.integer[name] = true
		}
	}
	if f.export {
		if r.exported == nil {
			r.exported = map[string]bool{}
		}
		r.exported[name] = !f.remove
	}
}

// markReadonly freezes a name.
//
// Never undone: a readonly name cannot be made writable again in any shell in
// the panel, so `+r` does nothing rather than reversing it — which is what
// they do.
func (r *Runner) markReadonly(name string) {
	if r.readonly == nil {
		r.readonly = map[string]bool{}
	}
	r.readonly[name] = true
}

// integerValue evaluates an assignment to a name declared integer.
//
// An empty assignment is zero rather than nothing, and a name that is not set
// is zero rather than an error — `n=abc` leaves 0 and says nothing, because
// `abc` is a perfectly good expression whose value happens to be unset. Text
// that will not parse *is* an error, and a fatal one: bash and zsh both stop
// the script rather than store something.
func (r *Runner) integerValue(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "0", true
	}
	p := syntax.NewParser("", r.dialect())
	e := p.ParseArithFor(text, syntax.Pos{})
	if err := p.Err(); err != nil {
		r.fatal("%s\n", r.diag().ParseFailure(err))
		return "", false
	}
	v, err := r.evalArith(e)
	if err != nil {
		r.fatal("%v\n", err)
		return "", false
	}
	return itoa(v), true
}

// declareEmpty is what declaring a name without a value does.
//
// The name is now local, or attributed, or both — but whether it also *exists*
// is a dialect's answer, so this is the one place that decides it and both
// `local` and `typeset` come through here.
func (r *Runner) declareEmpty(name string) {
	if r.ask(r.sem().DeclaredNameWithoutValueIsEmpty, "a declaration without a value setting the name") {
		r.setVar(name, "")
	}
}

// shadowTypeset is shadow for `typeset`, which unlike `local` does not always
// get a scope to declare into.
//
// ksh93 is the reason: only a function defined with the `function` word has
// one, and in a POSIX-style function the assignment is ordinary and reaches
// the caller. The axis is asked only when the two answers differ — inside a
// keyword-defined function they do not, so that case needs no dialect.
func (r *Runner) shadowTypeset(name string) {
	if len(r.scopes) == 0 {
		return
	}
	if !r.scopes[len(r.scopes)-1].keyword &&
		r.ask(r.sem().TypesetLocalNeedsKeywordFunction, "`typeset` needing a keyword-defined function to declare a local") {
		return
	}
	r.shadow(name)
}

// shadow saves a name in the innermost scope so the function's exit puts it
// back, which is what makes a declaration local.
func (r *Runner) shadow(name string) {
	if len(r.scopes) == 0 {
		return
	}
	sc := r.scopes[len(r.scopes)-1]
	if _, seen := sc.saved[name]; !seen {
		old, existed := r.Vars[name]
		sc.saved[name] = old
		sc.existed[name] = existed
	}
}

// declarationUtilities are the commands whose `name=value` arguments are
// assignments rather than ordinary words.
//
// The list is the core's: `export` and `readonly` are POSIX, and `local` and
// `typeset` are here because every shell in the panel that has them treats
// them the same way. A dialect adds its own names with SetDeclaring — bash and
// zsh add `declare` — because the rule has to follow the name into the shell
// that has it and must not apply in the shell that does not, where the same
// word is an ordinary command.
var declarationUtilities = map[string]bool{
	"export": true, "readonly": true, "local": true, "typeset": true,
}

// SetDeclaring makes a command's `name=value` arguments expand as assignments.
func (r *Runner) SetDeclaring(name string) {
	if r.declaring == nil {
		r.declaring = map[string]bool{}
	}
	r.declaring[name] = true
}

func (r *Runner) declares(name string) bool {
	return declarationUtilities[name] || r.declaring[name]
}

// assignShaped reports whether a word begins with a literal `name=`.
//
// The name has to be literal: `$x=1` is not an assignment in any shell, and
// neither is `"a"=1`. Only what the word says before any expansion counts.
func assignShaped(w *syntax.Word) bool {
	if w == nil || len(w.Spans) == 0 {
		return false
	}
	s := w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted {
		return false
	}
	name, _, ok := strings.Cut(s.Value, "=")
	return ok && name != "" && isNameLike(name)
}

// expandAssignArg expands `name=value` given to a declaration utility, leaving
// the name alone and expanding the value as an assignment's.
func (r *Runner) expandAssignArg(w *syntax.Word) string {
	head := w.Spans[0]
	name, first, _ := strings.Cut(head.Value, "=")
	rest := *w
	rest.Spans = append([]syntax.Span{{
		Kind: syntax.Literal, Value: first, Quoting: head.Quoting, Pos: head.Pos,
	}}, w.Spans[1:]...)
	return name + "=" + r.expandAssignValue(&rest)
}

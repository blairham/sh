// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
)

// `type` says what a name would run, in a sentence rather than as a path.
//
// The same question `command -v` answers and the same lookup, which is why
// they share one: a function first, then a builtin, then a keyword, then
// PATH. What differs is that this one is for a person to read, so every part
// of it is worded — and the four shells word all four parts differently, down
// to whether a missing name gets the shell's name in front of it.
func init() { builtins["type"] = biType }

func biType(r *Runner, _ context.Context, args []string) int {
	// `--` ends the options in three of the four. dash has no options at
	// all and reads it as a name, which is why this is asked rather than
	// assumed: `type -- ls` prints `--: not found` there and then goes on to
	// answer about `ls`.
	names, kind, code := r.typeOperands(args)
	if code != 0 {
		return code
	}
	status := 0
	for _, name := range names {
		if bad := r.typeOne(name, kind); bad != 0 {
			status = bad
		}
	}
	return status
}

// typeOperands separates the options from the names, and says whether `-t`
// asked for the bare kind instead of the sentence.
func (r *Runner) typeOperands(args []string) (names []string, kind bool, code int) {
	// Asked only where there is a `--` to decide about. `type ls` is the
	// same in all four, and refusing it over a question nothing turned on
	// would be refusing to answer.
	if len(args) == 0 || len(args[0]) < 2 || args[0][0] != '-' {
		return args, false, 0
	}
	if !r.ask(r.sem().TypeEndsOptionsWithDashDash, "`type --` ending the options") {
		if r.unspecified {
			return nil, false, 2
		}
		// No options anywhere, so every operand is a name — `--` included.
		return args, false, 0
	}
	if args[0] == "--" {
		return args[1:], false, 0
	}
	// `-t` is the one option implemented, so the letters this shell knows
	// are `t` or nothing, and which is the dialect's answer. Asked only
	// where a `t` rides in the option words: any other letter is refused
	// identically whichever way the answer goes — the ones the dialect
	// really has are named as missing and the rest as unknown — so the
	// question would decide nothing there.
	known := ""
	if typeOptionWordsCarryT(args) {
		if r.ask(r.sem().TypeNamesTheKindWithDashT, "`type -t` naming the bare kind") {
			known = "t"
		}
		if r.unspecified {
			return nil, false, 2
		}
	}
	rest, opts, code := r.builtinOptions("type", args, known)
	if code != 0 {
		return nil, false, code
	}
	return rest, strings.ContainsRune(opts, 't'), 0
}

// typeOptionWordsCarryT says whether a `t` rides in the leading option words —
// the words builtinOptions would read before the first operand.
func typeOptionWordsCarryT(args []string) bool {
	for _, a := range args {
		if len(a) < 2 || a[0] != '-' || a == "--" {
			return false
		}
		if strings.ContainsRune(a[1:], 't') {
			return true
		}
	}
	return false
}

// typeOne accounts for one name — as a sentence, or as `-t`'s bare kind —
// and reports a status if it could not.
//
// The kinds are one dialect's words and every dialect's words at once: only
// one shell in the panel has `-t` at all, so there is no second wording to
// hold a field for. The measured shell also answers `alias`, which is out of
// reach here for the same reason plain `type` never names one: whether
// aliases expand is the parser's fact — see syntax.Dialect.ExpandAliases —
// and the runner holds only the table.
func (r *Runner) typeOne(name string, kind bool) int {
	return r.describeName(name, kind,
		Wording(r.diag().TypeNotFound, "type: %[1]s: not found", name))
}

// describeName is the sentence itself, shared with `command -V`, which asks
// `type`'s question with a complaint of its own for a name that is nothing —
// the one line the two spell differently, so it arrives already worded.
func (r *Runner) describeName(name string, kind bool, notFound string) int {
	dg := r.diag()
	if fn, ok := r.funcs[name]; ok {
		if kind {
			r.printf("function\n")
			return 0
		}
		shows := r.ask(r.sem().TypePrintsFunctionBody, "`type` printing a function's body")
		if r.unspecified {
			return 2
		}
		r.printf("%s\n", Wording(dg.TypeFunction, "%[1]s is a function", name))
		if shows {
			// The function itself, laid out rather than quoted: the tree is
			// what this shell has, and the spelling it was written with is
			// gone by now. Which is why there is a printer — see
			// syntax.PrintWith.
			r.printf("%s\n", r.listedFunction(name, fn))
		}
		return 0
	}
	if _, ok := r.lookupBuiltin(name); ok {
		if kind {
			r.printf("builtin\n")
			return 0
		}
		r.printf("%s\n", Wording(dg.TypeBuiltin, "%[1]s is a shell builtin", name))
		return 0
	}
	if reservedWord(name) {
		if kind {
			r.printf("keyword\n")
			return 0
		}
		r.printf("%s\n", Wording(dg.TypeKeyword, "%[1]s is a shell keyword", name))
		return 0
	}
	// The same guard `command -v` has, and for the same reason: there is an
	// executable called /usr/bin/umask and this shell will not run it, so
	// saying where it is would be answering about the wrong thing.
	if !r.reservedBuiltin(name) {
		if path, err := r.lookPath(name); err == nil {
			if kind {
				// The kind and never the path, which is what keeps the word
				// comparable on any machine.
				r.printf("file\n")
				return 0
			}
			r.printf("%s\n", Wording(dg.TypeExternal, "%[1]s is %[2]s", name, path))
			return 0
		}
	}
	if kind {
		// Nothing at all for a name that is nothing — no line and no
		// diagnostic, measured. The status is the whole of the answer,
		// which is what makes `-t` scriptable in the first place.
		return orDefault(dg.TypeNotFoundStatus, 1)
	}
	msg := notFound
	if dg.TypeNotFoundUnprefixed {
		// Two of the four write this one with nothing in front of it, where
		// every other message they print carries the shell's name.
		r.errf("%s\n", msg)
	} else {
		r.diagf("%s\n", msg)
	}
	return orDefault(dg.TypeNotFoundStatus, 1)
}

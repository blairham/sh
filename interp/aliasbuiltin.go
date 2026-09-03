// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"sort"
	"strings"
)

// `alias` and `unalias` keep the table a shell substitutes from.
//
// This is the table and the two builtins. *Expansion* — replacing the word
// when a line is parsed — is the other half and is deliberately separate: it
// happens in the parser rather than here, needs an input stack to splice text
// in, and is the one component on the keystroke path and under the fuzzer. A
// table that nothing reads yet is still worth having, because `alias` in a
// startup file stops being an error and the two builtins are measurable on
// their own.
//
// Nothing here expands anything, and no test claims it does.

func init() {
	builtins["alias"] = biAlias
	builtins["unalias"] = biUnalias
}

func biAlias(r *Runner, _ context.Context, args []string) int {
	print := false
	// Two questions, because dash answers the first differently from the
	// other three: it reads no options for `alias` at all, so `alias -p` is a
	// *name* there and the answer is "-p not found". zsh does read options
	// and simply has no `-p`, which is a refusal rather than a lookup.
	if r.ask(r.sem().AliasParsesOptions, "`alias` reading options at all") {
		known := ""
		if r.ask(r.sem().AliasHasPrintOption, "`alias -p`") {
			known = "p"
		}
		if r.unspecified {
			return 2
		}
		rest, opts, code := r.builtinOptions("alias", args, known)
		if code != 0 {
			return code
		}
		args, print = rest, strings.ContainsRune(opts, 'p')
	} else if r.unspecified {
		return 2
	}
	if len(args) == 0 {
		names := make([]string, 0, len(r.aliases))
		for name := range r.aliases {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			r.printf("%s\n", r.aliasLine(name, print))
		}
		return 0
	}
	missing := 0
	for _, a := range args {
		name, value, isDefinition := strings.Cut(a, "=")
		if isDefinition {
			if r.aliases == nil {
				r.aliases = map[string]string{}
			}
			r.aliases[name] = value
			continue
		}
		if _, ok := r.aliases[name]; !ok {
			r.aliasNotFound("alias", name)
			missing++
			continue
		}
		r.printf("%s\n", r.aliasLine(name, print))
	}
	if missing == 0 {
		return 0
	}
	// ksh93 answers with how many it could not find — `alias n1 n2 n3` is 3
	// there — where the other three answer 1 however many were missing. Its
	// own `unalias` does not count, which is why this is asked here rather
	// than in aliasNotFound.
	if r.ask(r.sem().AliasNotFoundStatusCounts, "`alias` counting the names it could not find") {
		return missing
	}
	return 1
}

func biUnalias(r *Runner, _ context.Context, args []string) int {
	args, opts, code := r.builtinOptions("unalias", args, "a")
	if code != 0 {
		return code
	}
	if strings.ContainsRune(opts, 'a') {
		// zsh takes `-a` to mean the whole table and nothing else, so a name
		// beside it is too many arguments — and it clears nothing. The other
		// three ignore the names and empty the table.
		if len(args) > 0 && r.ask(r.sem().UnaliasAllRefusesOperands, "`unalias -a` with a name beside it") {
			r.diagf("%s\n", Wording(r.diag().UnaliasAllWithOperands,
				"unalias: -a: too many arguments"))
			return 1
		}
		r.aliases = nil
		return 0
	}
	if len(args) == 0 {
		return r.unaliasWithNothingToRemove()
	}
	status := 0
	for _, name := range args {
		if _, ok := r.aliases[name]; !ok {
			status = r.aliasNotFound("unalias", name)
			continue
		}
		delete(r.aliases, name)
	}
	return status
}

// aliasNotFound reports a name the table does not hold, for whichever of the
// two builtins asked.
//
// Two wordings and two answers about whether to speak at all, because the
// panel does not pair them: ksh93 complains about `alias nope` and says
// nothing about `unalias nope`, and zsh does exactly the reverse.
func (r *Runner) aliasNotFound(builtin, name string) int {
	d := r.diag()
	speaks, wording, unprefixed := r.sem().AliasReportsNotFound, d.AliasNotFound, d.AliasNotFoundUnprefixed
	if builtin == "unalias" {
		speaks, wording, unprefixed = r.sem().UnaliasReportsNotFound, d.UnaliasNotFound, d.UnaliasNotFoundUnprefixed
	}
	if r.ask(speaks, "`"+builtin+"` naming an alias it does not have") {
		line := Wording(wording, "%[1]s: %[2]s: not found", builtin, name)
		// Two of the four write this without the shell and the line in
		// front of it, which no other diagnostic of theirs does.
		if unprefixed {
			r.errf("%s\n", line)
		} else {
			r.diagf("%s\n", line)
		}
	}
	return 1
}

// unaliasWithNothingToRemove is `unalias` with no operand and no -a.
//
// Four answers: bash and ksh93 print a usage line, zsh says it is short of
// arguments, and dash takes it as a no-op and reports success.
func (r *Runner) unaliasWithNothingToRemove() int {
	d := r.diag()
	if d.UnaliasUsage == "" {
		return orDefault(d.UnaliasNoOperandStatus, 0)
	}
	if d.UnaliasUsageUnprefixed {
		r.errf("%s\n", d.UnaliasUsage)
	} else {
		r.diagf("%s\n", d.UnaliasUsage)
	}
	status := orDefault(d.UnaliasNoOperandStatus, 2)
	if r.ask(r.sem().BadOptionToSpecialBuiltinFatal, "a special builtin's usage error ending the script") {
		r.status = status
		r.fatalQuiet()
	}
	return status
}

// aliasLine spells one entry the way `alias` lists it.
func (r *Runner) aliasLine(name string, forcePrefix bool) string {
	prefix := r.diag().AliasListPrefix
	if forcePrefix {
		// `-p` prints the prefix even in the dialect whose plain listing has
		// none, which is what makes the option worth having there.
		prefix = "alias "
	}
	return prefix + name + "=" + r.quoteAliasValue(r.aliases[name])
}

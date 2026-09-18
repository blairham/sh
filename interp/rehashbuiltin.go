// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"sort"
	"strings"
)

// `rehash` and `unhash`, which are one shell's own names for two things this
// package already does: clearing the command hash, and taking one entry out
// of a table.
//
// Not a behavior that was missing — `hash -r` empties the table here and
// always did — but a **name** that was. `grep -rn '"rehash"'` found nothing
// in the tree, so the word fell through to a PATH search and came back 127,
// which reads to a script exactly like a typo. A startup file that adds a
// directory to `$path` and calls `rehash` is an ordinary thing to write, and
// it died on that line (#3109).
//
// Registered through the extension seam rather than built in the dialect, for
// the reason IntegerBuiltin is: the tables are this package's and a dialect
// package cannot reach them, so the alternative is a second implementation of
// a removal beside the first.
//
// Both refuse under their own names. That is why they are builtins rather
// than prelude functions wrapping `hash`: `rehash -x` is `bad option: -x`
// naming `rehash`, and a function calling `hash -r` would have named `hash`.

// RehashBuiltin is `rehash`, for a dialect that has the word to register.
func RehashBuiltin() Builtin { return biRehash }

// UnhashBuiltin is `unhash`, the same.
func UnhashBuiltin() Builtin { return biUnhash }

// rehashOptionLetters is what `rehash` takes. Measured 2026-09-18 on zsh
// 5.9.2 a letter at a time over `-a -d -f -m -p -r -s -v -L -t -i -n`: `-d`
// and `-f` are accepted and silent at 0, and every other letter is `bad
// option` at 1. So the pair is the whole set, and neither changes what the
// call does to the command table here — `-d` names the *directory* table and
// `-f` asks for the fill this shell does lazily anyway.
const rehashOptionLetters = "df"

func biRehash(r *Runner, _ context.Context, args []string) int {
	name := r.builtinComplaintName("rehash")
	args, _, code := r.builtinOptions(name, args, rehashOptionLetters)
	if code != 0 {
		return code
	}
	if len(args) > 0 {
		// A word of its own is not a name to rehash: measured, `rehash foo`
		// and `rehash a b` are both `too many arguments` at 1, and nothing
		// is cleared.
		r.diagf("%s\n", Wording(r.diag().RehashTooManyArguments,
			"%[1]s: too many arguments", name))
		return 1
	}
	r.forgetEveryHashedCommand()
	return 0
}

// unhashOptionLetters is what `unhash` takes, and each letter chooses the
// **table** the operands name. Measured the same way: `-a -d -f -m -s` are
// accepted and `-p -r -v -L -t -i -n` are `bad option` at 1.
const unhashOptionLetters = "adfms"

func biUnhash(r *Runner, _ context.Context, args []string) int {
	name := r.builtinComplaintName("unhash")
	args, opts, code := r.builtinOptions(name, args, unhashOptionLetters)
	if code != 0 {
		return code
	}
	if len(args) == 0 {
		// The same sentence `unset` and `unalias` write with nothing to
		// work on, at 1: a removal with no operand would be a removal of
		// everything, which this builtin has no spelling for.
		r.diagf("%s\n", Wording(r.diag().UnhashNoOperands,
			"%[1]s: not enough arguments", name))
		return 1
	}
	table := unhashCommands
	switch {
	case strings.ContainsRune(opts, 'a'):
		table = unhashAliases
	case strings.ContainsRune(opts, 's'):
		table = unhashSuffixAliases
	case strings.ContainsRune(opts, 'f'):
		table = unhashFunctions
	case strings.ContainsRune(opts, 'd'):
		table = unhashNamedDirs
	}
	if strings.ContainsRune(opts, 'm') {
		return r.unhashByPattern(args, table)
	}
	status := 0
	for _, operand := range args {
		if r.unhashOne(table, operand) {
			continue
		}
		// One sentence for all five tables, which is measured rather than
		// assumed: `unhash nosuch`, `unhash -a nosuch`, `unhash -f nosuch`
		// and `unhash -d nosuch` are the identical `no such hash table
		// element` at 1. The tables differ and the complaint does not.
		r.diagf("%s\n", Wording(r.diag().UnhashElementNotFound,
			"%[1]s: no such hash table element: %[2]s", name, operand))
		status = 1
	}
	return status
}

// unhashTable is which of the five tables one call's operands name. A value
// rather than five branches threaded through two functions, because the
// pattern form and the named form differ only in how they pick the names and
// not in what removing one means.
type unhashTable int

const (
	unhashCommands unhashTable = iota
	unhashAliases
	unhashSuffixAliases
	unhashFunctions
	unhashNamedDirs
)

// unhashOne removes one name from one table, reporting whether it was there.
func (r *Runner) unhashOne(table unhashTable, name string) bool {
	switch table {
	case unhashAliases:
		return r.removeAlias(name, AliasAnyKind)
	case unhashSuffixAliases:
		return r.removeAlias(name, AliasSuffixKind)
	case unhashFunctions:
		if _, ok := r.funcs[name]; !ok {
			return false
		}
		r.removeFunctionQuietly(name)
		return true
	case unhashNamedDirs:
		if _, ok := r.namedDir(name); !ok {
			return false
		}
		delete(r.namedDirs, name)
		return true
	}
	return r.forgetHashedCommand(name)
}

// unhashNames is what a pattern is matched against, sorted so that a removal
// is made in a fixed order rather than in the map's.
func (r *Runner) unhashNames(table unhashTable) []string {
	switch table {
	case unhashAliases:
		return r.aliasNames(AliasAnyKind)
	case unhashSuffixAliases:
		return r.aliasNames(AliasSuffixKind)
	case unhashFunctions:
		names := make([]string, 0, len(r.funcs))
		for name := range r.funcs {
			names = append(names, name)
		}
		sort.Strings(names)
		return names
	case unhashNamedDirs:
		return r.namedDirNames()
	}
	return r.hashedCommandNames()
}

// unhashByPattern is `-m`: every operand is a pattern rather than a name.
//
// A pattern that matches nothing is a **failure** here, unlike `alias -m` and
// `unalias -m` where it is silent success: measured, `unhash -m 'zq*'` is 1
// with no complaint when nothing matches and 0 when something does. So the
// status is the whole of the answer and there is no wording to give.
func (r *Runner) unhashByPattern(patterns []string, table unhashTable) int {
	removed := false
	for _, pattern := range patterns {
		o := r.patternOpts(pattern)
		for _, name := range r.unhashNames(table) {
			if !matchPattern(pattern, name, o) {
				continue
			}
			if r.unhashOne(table, name) {
				removed = true
			}
		}
	}
	if removed {
		return 0
	}
	return 1
}

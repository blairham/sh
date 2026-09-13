// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

// zshReservedWords is `$reswords`: every word this shell's grammar reserves,
// which is the one table a highlighter has to read before it can tell an `if`
// from a command called `if`.
//
// **Measured 2026-09-12 against zsh 5.9.2 under `-f`**, as
// `zmodload zsh/parameter; print -rl -- $reswords`, and confirmed name for
// name by a second instrument that shares none of the first's code:
// `whence -w` answers `reserved` for exactly these thirty-one words and
// `none` for every candidate below that is not one of them. Two probes rather
// than one because a list read out of a parameter and a list a parser
// consults can drift, and a single reading cannot tell a reserved word from a
// word somebody remembered to put in a table.
//
// Five groups of them are where a from-first-principles list goes wrong, and
// each is as it is because it was measured rather than reasoned:
//
//   - `[[` is in and **`]]` is not**. The closer is read by the `[[` parser
//     rather than reserved on its own, so `whence -w ']]'` is `none` — and
//     `]]` is a reserved word in *this* shell today, which is the one place
//     the table below and `whence -w` disagree here. See the note at the end.
//   - **`in` is not in it**, for the same reason: it is part of a `for` and a
//     `case` header rather than a word that opens anything. `whence -w in` is
//     `none`.
//   - `{`, `}` and `!` are all in, and `((` is not.
//   - `coproc`, `nocorrect`, `foreach` and `end` are all in — the four this
//     shell's zsh grammar has and the rest of the panel does not. Each has
//     its flag in the dialect already: Coproc, ReservedPrecommands,
//     Foreach, and `end` as the closer Foreach brings (see
//     syntax.Parser.parseForeach).
//   - The seven declaration keywords — `declare`, `export`, `float`,
//     `integer`, `local`, `readonly`, `typeset` — are in it. They are
//     *builtins* in this shell and *reserved words* in the one being
//     modeled, which is why they would be left out of a list derived from
//     this parser and why this list is not derived from it. A caller reads
//     `$reswords` to find out how zsh classifies a word, and F-Sy-H paints
//     `typeset` as a reserved word because of this entry.
//
// **The order is zsh's, recorded verbatim, and it is neither sorted nor the
// grammar's.** It is the walk order of zsh's own reserved-word hash table —
// stable across runs and across startup modes (measured with `-f`, with
// startup files, and under `emulate sh`, all identical), and an artifact of
// that table rather than a statement about the grammar. It is recorded as
// measured because a listing is a thing a script can print: F-Sy-H needs
// membership alone, since `$reswords[(Ie)$1]` asks only for an index, but
// `print -l $reswords` shows the difference and there is no reason to answer
// it differently from the shell being modeled.
//
// **It is a snapshot and not a view, and that is the exception this file
// earns rather than assumes.** Every other parameter here produces its value
// at the read because the thing it reports on moves — a function is defined,
// an option is set, an alias is added. The reserved-word table does not move:
// this shell's grammar is fixed once the dialect is chosen, and the one thing
// in zsh that takes a word *out* of `$reswords` is `disable -r`, which is
// `not implemented yet` here and which `$dis_reswords` is already parked on.
// Measured, so that the claim is not a guess: `disable -r foreach` in zsh
// 5.9.2 takes `${#reswords}` from 31 to 30 and `$reswords[(Ie)foreach]` to 0.
// The day that letter lands, this list and `dis_reswords` move together.
//
// `unset "reswords[1]"` is the one request this shell answers and zsh does
// not: measured 2026-09-12, zsh 5.9.2 **takes the shell down with SIGSEGV**
// there, exit 139, where `unset "builtins[cd]"` beside it is an ordinary
// `read-only variable: cd`. That is the same crash interp/absentparam.go
// records for `$funcstack` and `$historywords`, and the rule it states covers
// this too: there is nothing to copy, so the readonly refusal stands.
//
// One divergence is left standing deliberately, and it is `whence -w`'s and
// not this table's: this shell answers `reserved` for `]]` and `in`, and
// `builtin`/`none` for the seven declaration keywords, where zsh answers the
// other way round for all nine. That is #2512's classification and not this
// parameter's, and it is recorded here so the next reader does not take the
// disagreement for a defect in the list.
var zshReservedWords = []string{
	"if", "export", "declare", "function", "else", "float", "end", "do",
	"typeset", "then", "integer", "{", "select", "readonly", "coproc", "}",
	"!", "case", "[[", "repeat", "done", "for", "while", "time", "esac",
	"until", "local", "fi", "nocorrect", "foreach", "elif",
}

// zshReservedWordsView is `$reswords` as the parameter reads it.
//
// A copy rather than the slice itself. The parameter is readonly and the
// expansion machinery has no reason to write through the answer, but handing
// out the package's own backing array makes a caller that ever does so
// rewrite the table for every shell in the process, and that is the shape of
// bug nothing here would catch.
func zshReservedWordsView(*interp.Runner) []string {
	out := make([]string, len(zshReservedWords))
	copy(out, zshReservedWords)
	return out
}

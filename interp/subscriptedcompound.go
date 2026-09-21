// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A declaration utility given a **subscripted** operand whose value has the
// shape of a compound assignment stores the characters, and one column says
// so on the way past.
//
// The re-read itself is [Semantics.DeclarationRereadsAParenthesizedValue],
// and a subscript is the one condition that axis excludes: `declare
// a[1]="(var)"` stores the five characters at 1 in bash as here. bash
// declines the re-read there and writes a line about it.
//
// Measured 2026-09-21 on bash 5.3.20, each row its own `-c` under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`:
//
//	declare a[1]="(var)"                  warning: a[1]=(var): quoted
//	                                      compound array assignment deprecated
//	declare a[1]="(v ar)"                 the same — a blank inside does not
//	                                      matter
//	declare a[1]="()"                     the same — nor does an empty pair
//	declare a[1]+="(v)"                   the same, spelled `a[1]+=(v)`
//	typeset a[1]="(v)"                    the same
//	a[1]="(var)"                          silent — no declaration word
//	declare a[1]=" (v) "                  silent — the parentheses are not at
//	                                      the ends
//	declare a[1]="(v)x"                   silent, and `x(v)` too
//	declare -A m; declare m[k]="(v)"      silent — a table
//	declare -a a; declare a[1]="(v)"      silent — already an array
//	a=1; declare a[1]="(v)"               the warning: a scalar is not one
//	declare a[1]="(v)"; declare a[2]="(w)"
//	                                      the warning once, on the first: by
//	                                      the second the name is an array
//	declare -a a[1]="(v)"                 silent — the letter takes the
//	                                      re-read road instead
//	f() { declare a[1]="(v)"; }; f        silent, and so are `local a[1]=…`
//	                                      and `local a; declare a[1]=…`
//	f() { declare -g a[1]="(v)"; }; f     the warning — the letter puts it
//	                                      back on the global cell
//
// Three conditions, each a row above: the value is parenthesized **at both
// ends**, the name is not already an array or a table, and the declaration
// lands on the **global** cell rather than making a local. The last is the
// one that could not be guessed — a declaration inside a function is silent
// whatever the name held, and `-g` from inside one is not.
//
// A [Diagnostics] value rather than a [Semantics] one, because only the
// column that re-reads a parenthesized value at all has anything to decline:
// the others store the characters here and everywhere, with nothing to say
// about it.
func (r *Runner) warnQuotedCompoundAtASubscript(base, sub, value string, appends bool, df declareFlags) {
	w := r.diag().QuotedCompoundArrayAssignmentDeprecated
	if w == "" {
		return
	}
	if len(value) < 2 || value[0] != '(' || value[len(value)-1] != ')' {
		return
	}
	if df.array || df.assoc {
		// The letter is on this very line, and there the value is re-read
		// rather than declined: `declare -a a[1]="(v)"` leaves
		// `declare -a a=([0]="v")` with the subscript dropped and nothing
		// said.
		return
	}
	if r.arrayDeclared(base) || r.assocDeclared(base) {
		return
	}
	if len(r.scopes) > 0 && !df.global {
		return
	}
	op := "="
	if appends {
		op = "+="
	}
	r.DiagnoseAsTheShellf("%s\n", Wording(w,
		"warning: %[1]s: quoted compound array assignment deprecated",
		base+"["+sub+"]"+op+value))
}

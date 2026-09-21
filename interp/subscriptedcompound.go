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

// letterDropsTheSubscript reports whether a declaration's array letter takes
// the subscript out of a subscripted operand whose value is parenthesized.
//
// The same four-part shape the re-read turns on, with the *letter* standing
// where the standing attribute would: a declaration word, a `( … )` value at
// both ends, and `-a` or `-A` written on this line. With it, the subscript is
// not honored at all. Measured 2026-09-21 on bash 5.3.20:
//
//	declare -a a[1]="(v)"                     declare -a a=([0]="v")
//	declare -a a[1]="(v w)"                   declare -a a=([0]="v" [1]="w")
//	declare -a a[2]="(v)"                     declare -a a=([0]="v")
//	declare -A m[k]="([x]=1)"                 declare -A m=([x]="1" )
//	declare -a a=(z); declare -a a[1]="(v)"   ([0]="v") — replaced
//	declare -a a[1]+="(v)"                    ([0]="v") — the append too
//
// Three controls beside them, each already right without this: an ordinary
// value keeps its subscript (`declare -a a[1]=plain` is `([1]="plain")`), a
// line with no letter keeps it and earns the sentence above
// (`declare a[1]="(v)"`), and the shape test still applies
// (`declare a[1]=" (v) "`). #4105.
//
// The axis is read rather than asked, which is [Runner.ask]'s absence on
// purpose: a dialect that does not re-read a parenthesized value has nothing
// to drop the subscript *for*, so the answer is "keep it" and the re-read
// below asks the axis properly when it gets there. Asking twice would put a
// refusal in front of an operand this shell can store either way.
func (r *Runner) letterDropsTheSubscript(value string, df declareFlags) bool {
	if !df.array && !df.assoc {
		return false
	}
	if len(value) < 2 || value[0] != '(' || value[len(value)-1] != ')' {
		return false
	}
	return r.sem().DeclarationRereadsAParenthesizedValue == Yes
}

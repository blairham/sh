// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// An operand a builtin was given where it wanted a name.
//
// `export -` used to export a variable called `-` and report success. That
// hides a typo — `export -n FOO` with a stray space is `export - n FOO` — and
// it hides a shell whose options are not the ones the script was written for,
// which is the same reason a bad option is refused.
//
// Three of the four refuse every operand that is not a name, differing only in
// wording and in whether the script survives it. zsh is the fourth and refuses
// fewer: a special parameter is a name there, so `export -` is not a complaint
// at all.

// specialParameterNames are the one-character parameters a dialect may accept
// where a builtin wants a name.
//
// `$0` is in the set and the other digits are not: zsh takes `export 0` and
// refuses `export 1`, which is the difference between a special parameter and
// a positional one.
const specialParameterNames = "?*@#!-$0"

// isBuiltinName reports whether an operand can stand where a builtin wants a
// name.
//
// Measured on the operand *before* any `=`, because that is the part every
// shell judges: `export 1x=v` complains about the name in all four, and the
// two that quote the whole word back are quoting what they were given rather
// than what they rejected.
func (r *Runner) isBuiltinName(name string, takes NameOperands) bool {
	if takes == AnythingIsAName {
		return true
	}
	if name == "" {
		return false
	}
	if isPlainName(name) {
		return true
	}
	switch takes {
	case NamesAndSpecialParameters:
		return len(name) == 1 && strings.IndexByte(specialParameterNames, name[0]) >= 0
	case NamesAndPositionals:
		return allDigits(name)
	case PlainNamesOnly:
		return false
	}
	r.diagf("%s\n", r.unanswered("what may stand where a name is wanted"))
	r.status = 2
	r.unspecified = true
	return false
}

// isPlainName reports whether s is a name: a letter or `_`, then letters,
// digits and `_`.
//
// Not isNameLike, which trims first — that is right where it is used, for a
// word being judged as an assignment, and wrong here: all four shells refuse
// `export " a "`, and trimming would have made it a name.
func isPlainName(s string) bool {
	// The one caller has already refused the empty operand — it has to, since
	// an "every byte is a digit" loop accepts it — so this is belt and braces
	// rather than the check that does the work.
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || isLetter(c) || (i > 0 && isDigit(c)) {
			continue
		}
		return false
	}
	return true
}

// allDigits reports whether every byte is one, which is what tells a
// positional parameter from a name that starts badly: `12` is one and `1x` is
// not.
func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// nameRules gives the two answers that split by builtin rather than by
// dialect, in one place rather than at each of the three call sites.
// takesASubscript answers whether `a[0]` is a name to this builtin.
//
// Two questions rather than one, because the answer is per builtin as much
// as per dialect: bash takes it for `unset` and refuses it for `export` and
// `readonly`, and the two builtins sit on different name strictnesses in
// every shell, so no rule over that strictness gives all four.
func (r *Runner) takesASubscript(builtin string) bool {
	if builtin == "unset" {
		return r.ask(r.sem().UnsetTakesASubscript, "`unset a[0]` naming an array element")
	}
	return r.ask(r.sem().DeclarationTakesASubscript, "`export a[0]` naming an array element")
}

func (r *Runner) nameRules(builtin string) (fatal Answer, takes NameOperands) {
	if builtin == "unset" {
		return r.sem().BadNameToUnsetFatal, r.sem().UnsetNameOperands
	}
	return r.sem().BadNameToDeclarationFatal, r.sem().DeclarationNameOperands
}

// builtinNames keeps the operands that are names and reports the rest.
//
// Every bad operand is reported, not just the first: bash prints a line for
// each of `export 1x 2y` and goes on to export whatever was well formed. The
// other three print one line because the first is fatal there and the loop
// never reaches the second, which falls out of the fatality rather than
// needing a rule of its own.
//
// Whether a subscripted operand is a name is its own question, and it is
// answered per builtin as well as per dialect: bash refuses `export a[0]`
// and takes `unset a[0]`, ksh93 takes both, dash refuses both. Asked only
// when an operand has a subscript, so nothing else is affected.
func (r *Runner) builtinNames(builtin string, args []string, explicitVariable bool) (rest []string, status int) {
	fatal, takes := r.nameRules(builtin)
	// `unset -v` asks for a variable by name, and gets a name checked even
	// where the bare form checks nothing: bash 5.3 is quiet about `unset 1x`
	// and refuses `unset -v 1x`. Measured in all four; only bash has a bare
	// form loose enough for the difference to show.
	if explicitVariable && takes == AnythingIsAName {
		takes = PlainNamesOnly
	}
	for _, a := range args {
		name, _, _ := strings.Cut(a, "=")
		if _, _, subscripted := r.subscriptOperand(name); subscripted {
			if r.takesASubscript(builtin) {
				rest = append(rest, a)
				continue
			}
			if r.unspecified {
				return nil, 2
			}
			status = r.badSubscriptOperand(builtin, a, name, fatal)
			if r.ctl == controlExit {
				return nil, status
			}
			continue
		} else if r.isBuiltinName(name, takes) {
			rest = append(rest, a)
			continue
		}
		if r.unspecified {
			return nil, 2
		}
		status = r.badBuiltinName(builtin, a, name, fatal)
		if r.ctl == controlExit {
			return nil, status
		}
	}
	return rest, status
}

// badSubscriptOperand reports a subscripted operand the dialect will not take.
//
// Two of the three that refuse it say what they say about any bad name, so
// they need nothing here and fall through to badBuiltinName. The third has a
// complaint of its own per builtin — one about the subscript, naming the base,
// and one about array elements, naming the whole operand — and does not name
// the builtin in the location for the first of them where it does for the
// second. That is the same shape as which declarations name themselves in a
// readonly refusal, and it is a set for the same reason.
func (r *Runner) badSubscriptOperand(builtin, operand, name string, fatal Answer) int {
	d := r.diag()
	wording := d.BuiltinBadSubscript[builtin]
	if wording == "" {
		return r.badBuiltinName(builtin, operand, name, fatal)
	}
	base, _, _ := r.subscriptOperand(name)
	if !d.SubscriptRefusalNamesBuiltin[builtin] {
		// The location does not name the builtin for this one, where it does
		// for the other — put aside for the report and given back, the way a
		// readonly reassignment already does it.
		outer := r.inBuiltin
		r.inBuiltin = ""
		defer func() { r.inBuiltin = outer }()
	}
	r.diagf("%s\n", Wording(wording, "", builtin, operand, base))
	status := orDefault(d.BuiltinBadNameStatus, 1)
	if r.ask(fatal, "a bad name to a special builtin ending the script") {
		r.status = status
		r.fatalQuiet()
		return r.status
	}
	return status
}

// badBuiltinName reports it, and ends the script where the dialect says so.
//
// The wording is per builtin because two dialects word it per builtin: ksh93
// says "is not an identifier" for `export` and "invalid variable name" for
// `readonly` and `unset`, and zsh puts the operand before the reason for
// `unset` alone where it puts it after for the other two.
func (r *Runner) badBuiltinName(builtin, operand, name string, fatal Answer) int {
	d := r.diag()
	if d.BadNameRefusalHidesTheBuiltin[builtin] {
		// The location does not name the builtin for this one where it does
		// for the others — put aside for the report and given back, the way
		// badSubscriptOperand and the readonly refusal already do it.
		outer := r.inBuiltin
		r.inBuiltin = ""
		defer func() { r.inBuiltin = outer }()
	}
	shown := name
	if d.BuiltinBadNameKeepsValue {
		shown = operand
	}
	wording := d.BuiltinBadName[builtin]
	// A leading digit is a different complaint in the one dialect that tells
	// the two apart: zsh says "not an identifier: 1x" for `1x` and "not valid
	// in this context: a-b" for `a-b`. An empty entry means the dialect says
	// the same thing to both, which is three of the four.
	if len(name) > 0 && name[0] >= '0' && name[0] <= '9' {
		if numeric := d.BuiltinBadNameNumeric[builtin]; numeric != "" {
			wording = numeric
		}
	}
	r.diagf("%s\n", Wording(wording, "%[1]s: `%[2]s': not a valid identifier", builtin, shown))
	status := orDefault(d.BuiltinBadNameStatus, 1)
	if r.ask(fatal, "a bad name to a special builtin ending the script") {
		r.status = status
		r.fatalQuiet()
	}
	return status
}

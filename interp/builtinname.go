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
	switch builtin {
	case "unset":
		return r.ask(r.sem().UnsetTakesASubscript, "`unset a[0]` naming an array element")
	case "typeset", "declare", "integer", "local":
		// A third answer rather than the declaration's, because bash gives a
		// third answer: it refuses `export a[1]=v` and takes
		// `typeset a[1]=v`. See Semantics.TypesetTakesASubscript.
		//
		// `local` is on this side of the split rather than with `export`:
		// measured 2026-09-07, bash 5.3 takes `local a[1]=v` and creates a
		// local array holding the element, and zsh 5.9.2 takes the operand
		// too — it refuses it afterwards, for a reason of its own about
		// elements rather than about names. Reading it through the export
		// question made this shell refuse a line bash has always accepted.
		return r.ask(r.sem().TypesetTakesASubscript, "`typeset a[0]=v` naming an array element")
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
// **A fatal refusal does not throw the operands away.** Every shell in the
// panel declares the names that *were* names and only then stops, so this
// hands them back and reports the give-up separately for the caller to raise
// once it has done what they ask — see namesAfterARefusal for what the
// dialects disagree about and endAfterABadName for the raising (#1211).
//
// Whether a subscripted operand is a name is its own question, and it is
// answered per builtin as well as per dialect: bash refuses `export a[0]`
// and takes `unset a[0]`, ksh93 takes both, dash refuses both. Asked only
// when an operand has a subscript, so nothing else is affected.
func (r *Runner) builtinNames(builtin string, args []string, explicitVariable bool) (rest []string, status int, ended bool) {
	fatal, takes := r.nameRules(builtin)
	// `unset -v` asks for a variable by name, and gets a name checked even
	// where the bare form checks nothing: bash 5.3 is quiet about `unset 1x`
	// and refuses `unset -v 1x`. Measured in all four; only bash has a bare
	// form loose enough for the difference to show.
	if explicitVariable && takes == AnythingIsAName {
		takes = PlainNamesOnly
	}
	for i, a := range args {
		name, _, _ := strings.Cut(a, "=")
		if base, appends := appendOperand(a); appends && builtin != "unset" {
			// `declare a+=2` carries the append operator, which one shell
			// takes as an operand and the others refuse. Asked here because
			// this is where an operand becomes a name, and only for the one
			// shape that splits them — a `+` with no `=` after it is not
			// this spelling and is refused as a name everywhere.
			//
			// The refusal names `a+` and not the whole operand, which is
			// what the shells that refuse it say: two of them otherwise
			// quote a bad operand back in full, and for this one they do
			// not. So the name and the operand are the same text here.
			if r.ask(r.sem().DeclarationTakesAnAppendOperand,
				"a declaration taking a `name+=value` operand") {
				name = base
			} else if r.unspecified {
				return nil, 2, false
			} else {
				status = r.badBuiltinName(builtin, name, name, fatal)
				if r.ctl == controlExit {
					return r.namesAfterARefusal(builtin, rest, args[i+1:], takes), status, true
				}
				continue
			}
		}
		// A subscript on something that is not a name is not a subscripted
		// operand at all: measured, `typeset 1x[0]=v` is a bad name in every
		// column that has the word, and bash quotes the *whole* operand back
		// there where it quotes only `a[1]` for a well-formed base. So the
		// brackets are read as a subscript only once the base is a name.
		if base, _, subscripted := r.subscriptOperand(name); subscripted && isPlainName(base) {
			if r.takesASubscript(builtin) {
				rest = append(rest, a)
				continue
			}
			if r.unspecified {
				return nil, 2, false
			}
			status = r.badSubscriptOperand(builtin, a, name, fatal)
			if r.ctl == controlExit {
				return r.namesAfterARefusal(builtin, rest, args[i+1:], takes), status, true
			}
			continue
		} else if r.isBuiltinName(name, takes) {
			rest = append(rest, a)
			continue
		}
		if r.unspecified {
			return nil, 2, false
		}
		status = r.badBuiltinName(builtin, a, name, fatal)
		if r.ctl == controlExit {
			return r.namesAfterARefusal(builtin, rest, args[i+1:], takes), status, true
		}
	}
	return rest, status, false
}

// namesAfterARefusal puts the give-up aside and answers which operands the
// caller still has to declare.
//
// The refusal has already been reported and has already ended the script;
// what the panel disagrees about is how much of the line goes with it.
// Measured 2026-09-07 with the bad name first, in the middle and last, over
// `export`, `readonly`, `typeset` and `unset`, reading the names back from an
// EXIT trap so the fatality could not hide the answer:
//
//   - ksh93 and zsh declare **every** well-formed operand, wherever the bad
//     one stood: `export ok1=1 ":" ok2=2` leaves both set in each.
//   - bash-as-`sh` and dash declare only the ones **in front of** it, so the
//     same line leaves ok1 set and ok2 unset, and the bad name first leaves
//     neither.
//
// bash proper never arrives: its refusal is not fatal, so it reports each bad
// operand and declares all the good ones by carrying on.
//
// The operands after the refusal are collected in silence. One diagnostic is
// what every fatal column writes however many bad names follow the first —
// measured with `export ":" ok1=1 "1x" ok2=2` — so reporting them here would
// add a line no shell writes.
func (r *Runner) namesAfterARefusal(builtin string, kept, remaining []string, takes NameOperands) []string {
	// Put aside rather than cleared: the caller raises it again through
	// endAfterABadName once the operands below have been declared. Holding
	// it is what lets the caller's loop run at all — every step of it stops
	// on controlExit, which is right for a failure of its own and wrong for
	// one that has already happened.
	r.ctl, r.abandon, r.errexitStopped = controlNone, abandonRequested, false
	if !r.ask(r.sem().BadNameDeclaresTheOperandsAfterIt,
		"a fatal bad name leaving the operands after it declared") {
		return kept
	}
	for _, a := range remaining {
		name, _, _ := strings.Cut(a, "=")
		if base, appends := appendOperand(a); appends && builtin != "unset" {
			// The same reading the first pass gave this shape. The axis was
			// answered there — it is the same dialect — so this asks
			// nothing and only has to agree about which operands are names.
			if r.sem().DeclarationTakesAnAppendOperand == Yes {
				name = base
			} else {
				continue
			}
		}
		if base, _, subscripted := r.subscriptOperand(name); subscripted && isPlainName(base) {
			if r.takesASubscript(builtin) {
				kept = append(kept, a)
			}
			continue
		}
		if r.isBuiltinName(name, takes) {
			kept = append(kept, a)
		}
	}
	return kept
}

// endAfterABadName raises the give-up namesAfterARefusal held back.
//
// Called by every builtin that takes names, after its own loop: the operands
// that were names have been declared and the script stops here. A failure of
// the loop's own has already ended it and keeps its own status, which is the
// same rule the readonly refusal follows one function over.
func (r *Runner) endAfterABadName(status int) int {
	if r.ctl == controlExit {
		return r.status
	}
	r.status = status
	r.fatalQuiet()
	return r.status
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
		// The *name* rather than the whole operand, even in the dialect
		// whose bad-name complaint quotes what it was given: measured,
		// bash 5.3 answers `export 1x=v` with `` `1x=v' `` and
		// `export a[1]=v` with `` `a[1]' ``, so a subscript is where it
		// stops quoting the value back.
		return r.badBuiltinName(builtin, name, name, fatal)
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

// isReadName reports whether an operand can stand where `read` wants a name.
//
// A fourth caller of isBuiltinName rather than a second identifier check.
// `read` was the one builtin in this shell that took a word which is not a
// name and assigned to it in silence — status 0, no diagnostic, and the
// variable a caller went on to read left empty (#1440) — where every shell in
// the panel refuses it by name. The rule was already a function precisely so
// that a new caller would not have to restate it, and restating it here is
// how the same bug comes back in a fifth place.
//
// A subscripted operand is not judged here. `read a[0]` fills the element in
// bash, bash 3.2 and ksh93, so refusing it as a bad name would answer three of
// the six wrongly; what this shell does with it is a separate question and
// this leaves it exactly where it was.
func (r *Runner) isReadName(name string) bool {
	if base, _, subscripted := r.subscriptOperand(name); subscripted && isPlainName(base) {
		return true
	}
	return r.isBuiltinName(name, r.sem().ReadNameOperands)
}

// badReadName reports it, through the same wording table and the same
// fatality gate as every other bad name.
//
// The operand and the name are the same word: `read` takes no `=`, and where
// the first operand carried a prompt the part in front of the `?` is both what
// was judged and what every shell that splits there quotes back.
func (r *Runner) badReadName(name string) int {
	return r.badBuiltinName("read", name, name, r.sem().BadNameToReadFatal)
}

// readAfterABadName raises the refusal the assignment pass held back.
//
// Held back rather than raised where it was found, because the panel is
// unanimous that the names in front of a bad one are still filled:
// `printf 'X Y Z\n' | { c=keep; read a 1bad c; }` leaves a as X and c as keep
// in all six shells. So the refusal is what the builtin returns and not what
// stops it, and the status it carries wins over the read's own — a `read` that
// reached the end of its input answers 1, and so does this, which is the whole
// reason the silence was worth a P1: the two were indistinguishable.
func (r *Runner) readAfterABadName(status int, name string, bad bool) int {
	if !bad {
		return status
	}
	return r.badReadName(name)
}

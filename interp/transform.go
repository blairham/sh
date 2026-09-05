// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/blairham/sh/syntax"
)

// The `${x@Q}` transformation family. One grammar has it — the flag
// syntax.Dialect.ParamTransformations gates the parse — so everything here is
// measured against the shell that answers, and recorded in
// docs/spec/grammar/parameter-expansion.md.

// transformParam applies e.Transform to a single value: a scalar, one array
// element named by its subscript, or the target of an indirection. name is
// the name the value came from, which is not e.Name after `${!x@a}` — the
// attributes reported are the target's, measured.
func (r *Runner) transformParam(e *syntax.ParamExpr, name, value string, set bool) string {
	switch e.Transform {
	case 'Q', 'K', 'k':
		// @K and @k differ from @Q only over a stored array, which
		// transformElems answers; a scalar is the quoted value in all three.
		if !set {
			// Unset is empty, not `''`: there is no value to quote.
			return ""
		}
		return quoteForInput(value)
	case 'E':
		return r.expandDollarSingle(value)
	case 'P':
		r.refuseTransform(e)
		return ""
	case 'A':
		return r.assignmentStatement(name, value, set)
	case 'a':
		return r.attributeLetters(name)
	case 'L':
		return strings.ToLower(value)
	case 'U':
		return strings.ToUpper(value)
	case 'u':
		return upperFirst(value)
	}
	return ""
}

// transformElems distributes a transformation over a whole array — the
// elements of `${a[@]…}` and `${a[*]…}`, or the positional parameters for
// `${@…}` and `${*…}`. What comes back is the field list before quoting and
// joining, so the two subscript spellings share it: `[@]` keeps the fields
// and `[*]` joins them, exactly as they do with no operator at all.
func (r *Runner) transformElems(e *syntax.ParamExpr, elems []string) []string {
	switch e.Transform {
	case 'K', 'k':
		return r.keysAndValues(e, elems)
	case 'A':
		return r.assignmentWords(e.Name, elems)
	case 'a':
		// The attributes belong to the name, so every element answers with
		// the same letters — measured `a a` for a two-element array.
		letters := r.attributeLetters(e.Name)
		out := make([]string, len(elems))
		for i := range out {
			out[i] = letters
		}
		return out
	case 'P':
		r.refuseTransform(e)
		return nil
	}
	out := make([]string, 0, len(elems))
	for _, el := range elems {
		out = append(out, r.transformParam(e, e.Name, el, true))
	}
	return out
}

// refuseTransform is @P, which is prompt expansion: `\u`, `\w` and the rest
// of the prompt escapes, expanded against state the front end holds. The
// interpreter has no prompt machinery to hand the value to, and returning the
// value unchanged would be right only when it carries no escape — a silent
// wrong answer for every value that does. Refused loudly instead, naming the
// letter, until something can answer honestly.
func (r *Runner) refuseTransform(e *syntax.ParamExpr) {
	r.diagf("${%s@%c}: the @%c transformation is not implemented\n",
		e.Name, e.Transform, e.Transform)
	r.expandErr = true
}

// quoteForInput is @Q: the value spelled so that reading it back as input
// yields the value. Two spellings, picked by content — single quotes while
// every character is printable, an embedded quote closing the run and coming
// back backslashed, and `$'…'` once anything needs an escape. Measured:
// `a b'c` becomes `'a b'` + `\'` + `'c'`, `has\backslash` stays
// single-quoted, and a tab switches the whole value.
func quoteForInput(v string) string {
	if needsDollarQuoting(v) {
		return dollarQuotedForInput(v)
	}
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

// needsDollarQuoting reports whether the value holds anything single quotes
// cannot carry readably: a control character, or a byte that is not
// character-shaped at all. Printable multibyte text does not count — `café`
// stays as written, measured.
func needsDollarQuoting(v string) bool {
	for i := 0; i < len(v); {
		c, size := utf8.DecodeRuneInString(v[i:])
		if (c == utf8.RuneError && size == 1) || unicode.IsControl(c) {
			return true
		}
		i += size
	}
	return false
}

// dollarQuotedForInput spells a value as `$'…'` the way @Q does — a richer
// set than the listings' dollarQuoted, which never meets most of these. The
// escapes measured: `\a \b \t \n \v \f \r` by name, ESC as `\E`, the quote
// and the backslash escaped, `"` kept plain, and anything else unprintable as
// three-digit octal per *byte* — DEL is `\177` and U+0085 is `\302\205`.
func dollarQuotedForInput(v string) string {
	var b strings.Builder
	b.WriteString("$'")
	for i := 0; i < len(v); {
		switch c := v[i]; c {
		case '\a':
			b.WriteString(`\a`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\v':
			b.WriteString(`\v`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		case 0x1b:
			b.WriteString(`\E`)
		case '\'':
			b.WriteString(`\'`)
		case '\\':
			b.WriteString(`\\`)
		default:
			c2, size := utf8.DecodeRuneInString(v[i:])
			if (c2 == utf8.RuneError && size == 1) || unicode.IsControl(c2) {
				for j := range size {
					b.WriteString(octalByte(v[i+j]))
				}
			} else {
				b.WriteString(v[i : i+size])
			}
			i += size
			continue
		}
		i++
	}
	b.WriteByte('\'')
	return b.String()
}

// octalByte is one byte as `\NNN`, always three digits — measured `\001`.
func octalByte(c byte) string {
	return string([]byte{'\\', '0' + (c>>6)&7, '0' + (c>>3)&7, '0' + c&7})
}

// doubleQuotedForInput is how @K and @A spell an element's value: the
// listings' doubleQuoted — backslash, backquote, dollar and the quote
// escaped — until a control character forces the `$'…'` spelling, exactly as
// @Q switches. Measured: `0 "t w"`, `0 "say \"hi\""`, `0 $'tab\there'`.
func doubleQuotedForInput(v string) string {
	if needsDollarQuoting(v) {
		return dollarQuotedForInput(v)
	}
	return doubleQuoted(v)
}

// attributeLetters is @a: one letter per attribute the name carries, in the
// order the letters were measured to arrive — `ar`, `Ai`, `ix`, `irx`. A name
// with none, and a parameter that is not a name at all, answer empty.
func (r *Runner) attributeLetters(name string) string {
	var b strings.Builder
	if _, ok := r.Arrays[name]; ok && !r.removed[name] {
		b.WriteByte('a')
	}
	if _, ok := r.AssocArrays[name]; ok && !r.removed[name] {
		b.WriteByte('A')
	}
	if r.integer[name] {
		b.WriteByte('i')
	}
	if r.readonly[name] {
		b.WriteByte('r')
	}
	if r.exported[name] {
		b.WriteByte('x')
	}
	return b.String()
}

// assignmentStatement is @A for a scalar view: the statement that would
// reproduce the name and its value. Measured shapes: `x='a b'` bare,
// `declare -irx v='1'` once any attribute needs declaring, `declare -A h`
// when the scalar view holds no value, and empty when there is neither a
// value nor an attribute — or no name to assign to at all, which is what a
// positional parameter is: `${1@A}` is empty.
func (r *Runner) assignmentStatement(name, value string, set bool) string {
	if !isNameLike(name) {
		return ""
	}
	attrs := r.attributeLetters(name)
	if attrs == "" && !set {
		return ""
	}
	var b strings.Builder
	if attrs != "" {
		b.WriteString("declare -")
		b.WriteString(attrs)
		b.WriteByte(' ')
	}
	b.WriteString(name)
	if set {
		b.WriteByte('=')
		b.WriteString(quoteForInput(value))
	}
	return b.String()
}

// assignmentWords is @A over a whole array: the *words* of the statement that
// would reproduce it — `declare` `-a` `a=([0]="1" [1]="x y")`, measured as
// three fields. The positional parameters have no name and come back as a
// `set --` command, one word per parameter, and nothing at all when there
// are none. A scalar read as `${x[@]}` answers as the subscript-less form
// does, over the value its one-element view holds.
func (r *Runner) assignmentWords(name string, elems []string) []string {
	if name == "@" || name == "*" {
		if len(elems) == 0 {
			return nil
		}
		out := []string{"set", "--"}
		for _, el := range elems {
			out = append(out, quoteForInput(el))
		}
		return out
	}
	keys, vals, assoc, stored := r.arrayPairs(name)
	if !stored {
		value, set := "", false
		if len(elems) > 0 {
			value, set = elems[0], true
		}
		st := r.assignmentStatement(name, value, set)
		if st == "" {
			return nil
		}
		return []string{st}
	}
	var b strings.Builder
	b.WriteString(name)
	b.WriteString("=(")
	for i, k := range keys {
		if i > 0 && !assoc {
			b.WriteByte(' ')
		}
		b.WriteString("[")
		b.WriteString(k)
		b.WriteString("]=")
		b.WriteString(doubleQuotedForInput(vals[i]))
		if assoc {
			// The associative list carries a space after every pair rather
			// than between pairs, measured: `h=([k]="v 1" )`.
			b.WriteByte(' ')
		}
	}
	b.WriteString(")")
	return []string{"declare", "-" + r.attributeLetters(name), b.String()}
}

// keysAndValues is @K and @k over a whole array. A stored array answers with
// its pairs — @k as separate words, key then value, and @K as one word with
// the values double-quoted. Anything else — a scalar read as `${x[@]}`, the
// positional parameters — answers exactly as @Q does: quoted values and no
// keys at all, measured.
func (r *Runner) keysAndValues(e *syntax.ParamExpr, elems []string) []string {
	keys, vals, assoc, stored := r.arrayPairs(e.Name)
	if !stored {
		out := make([]string, 0, len(elems))
		for _, el := range elems {
			out = append(out, quoteForInput(el))
		}
		return out
	}
	if e.Transform == 'k' {
		out := make([]string, 0, 2*len(keys))
		for i, k := range keys {
			out = append(out, k, vals[i])
		}
		return out
	}
	var b strings.Builder
	for i, k := range keys {
		if i > 0 && !assoc {
			b.WriteByte(' ')
		}
		b.WriteString(k)
		b.WriteByte(' ')
		b.WriteString(doubleQuotedForInput(vals[i]))
		if assoc {
			// The same trailing-space quirk the @A list has, measured:
			// `k "v 1" ` for one pair.
			b.WriteByte(' ')
		}
	}
	return []string{b.String()}
}

// arrayPairs is the keys and values of a stored array, in the order this
// implementation keeps everywhere: subscript order for an indexed array, the
// keys sorted for an associative one — the shells promise no order there.
func (r *Runner) arrayPairs(name string) (keys, vals []string, assoc, stored bool) {
	if a, ok := r.AssocArrays[name]; ok && !r.removed[name] {
		ks := a.keys()
		vs := make([]string, len(ks))
		for i, k := range ks {
			vs[i] = a[k]
		}
		return ks, vs, true, true
	}
	if a, ok := r.Arrays[name]; ok && !r.removed[name] {
		base := r.arrayBase()
		ks := make([]string, 0, len(a))
		vs := make([]string, 0, len(a))
		for _, k := range r.arrayKeys(a) {
			// Positions are stored from zero and subscripts are written from
			// wherever the dialect counts, the same edge subscriptsOf walks.
			ks = append(ks, itoa(k+base))
			vs = append(vs, a[k])
		}
		return ks, vs, false, true
	}
	return nil, nil, false, false
}

// upperFirst is @u: the first character upper-cased, the rest untouched —
// `abC dEf` → `AbC dEf`, measured.
func upperFirst(v string) string {
	if v == "" {
		return v
	}
	c, size := utf8.DecodeRuneInString(v)
	return string(unicode.ToUpper(c)) + v[size:]
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/internal/syntax"
)

// expandWord turns one word into zero or more fields.
//
// It follows the ordering in docs/spec/grammar/expansion.md, as far as this
// slice goes: parameter expansion, then field splitting of the *unquoted*
// results only. Pathname expansion and quote removal beyond what the lexer
// already did are not here yet.
//
// A word is a sequence of spans rather than a string precisely so this stage
// can tell which parts were quoted. Splitting applies only to the unquoted
// ones, which is why a"b c"d is one field and $x with a space in it is two.
func (r *Runner) expandWord(w *syntax.Word) []string {
	if w == nil {
		return nil
	}

	// Fields are built up span by span. A span joins onto the field before it
	// unless splitting started a new one, which is what makes x$(f)y attach
	// its literal text to the first and last resulting fields.
	fields := []string{""}
	any := false

	for _, s := range w.Spans {
		// `$@` is the one expansion that yields more than one field on its
		// own, so it cannot go through expandSpan, which returns a string.
		// The first parameter joins onto whatever precedes it and the last
		// stays open for whatever follows — which is why `x$@y` attaches its
		// literal text to the first and last fields rather than becoming
		// words of its own.
		if parts, ok := r.expandAt(s); ok {
			if len(parts) == 0 {
				continue
			}
			any = true
			fields[len(fields)-1] += parts[0]
			fields = append(fields, parts[1:]...)
			continue
		}
		text, split := r.expandSpan(s)
		if !split {
			fields[len(fields)-1] += text
			any = any || text != "" || s.Quoting != syntax.Unquoted
			continue
		}
		ifs, set := r.ifs()
		parts := splitFields(text, ifs, set)
		if len(parts) == 0 {
			// An unquoted expansion of an empty value produces no field at
			// all, so nothing is appended and nothing is started.
			continue
		}
		any = true
		fields[len(fields)-1] += parts[0]
		fields = append(fields, parts[1:]...)
	}

	if len(fields) == 1 && fields[0] == "" && !any {
		return nil
	}
	return fields
}

// expandWordNoSplit expands a word without field splitting, for the contexts
// that do not have it: inside `[[ ]]`, and a redirection target. It is a
// separate entry point rather than a flag on the runner because the caller
// knows which context it is in and the expander should not have to guess.
func (r *Runner) expandWordNoSplit(w *syntax.Word) []string {
	if w == nil {
		return nil
	}
	var b strings.Builder
	for _, s := range w.Spans {
		if parts, ok := r.expandAt(s); ok {
			b.WriteString(strings.Join(parts, " "))
			continue
		}
		text, _ := r.expandSpan(s)
		b.WriteString(text)
	}
	return []string{b.String()}
}

// expandAt handles `$@`, the only expansion that produces several fields by
// itself. Quoted, it is one field per parameter, each keeping its own spaces;
// with no parameters it is *zero* fields, which is why `set -- "$@"` is safe
// on an empty list and `set -- "$*"` is not.
func (r *Runner) expandAt(s syntax.Span) ([]string, bool) {
	if s.Kind != syntax.ParamExp || s.Param == nil {
		return nil, false
	}
	e := s.Param
	if e.Name != "@" || e.Op != syntax.ParamNone || e.Length {
		return nil, false
	}
	if s.Quoting != syntax.Unquoted {
		return r.Params, true
	}
	// Unquoted, each parameter is then split like any other expansion.
	ifs, set := r.ifs()
	var out []string
	for _, p := range r.Params {
		out = append(out, splitFields(p, ifs, set)...)
	}
	return out, true
}

// expandSpan expands one span, reporting whether its result is subject to
// field splitting. Only unquoted expansions are; literal text never is,
// however it was written.
func (r *Runner) expandSpan(s syntax.Span) (text string, split bool) {
	unquoted := s.Quoting == syntax.Unquoted
	switch s.Kind {
	case syntax.Literal:
		// Literal text is never split, however it was written.
		return s.Value, false
	case syntax.ParamExp:
		return r.expandParam(s.Param), unquoted
	case syntax.CommandSubst:
		return r.commandSubst(r.ctx, s.Value), unquoted
	case syntax.ArithSubst:
		v, err := r.evalArith(s.Arith)
		if err != nil {
			r.errf("sh: %v\n", err)
			r.status = 1
			return "", false
		}
		return itoa(v), unquoted
	}
	return "", false
}

// expandParam handles the forms this slice implements.
func (r *Runner) expandParam(e *syntax.ParamExpr) string {
	if e == nil {
		return ""
	}
	// The special parameters are not variables and are answered first.
	if v, ok := r.specialParam(e); ok {
		return v
	}

	value, set := r.getVar(e.Name)

	if e.Length {
		return itoa(len(value))
	}

	// The colon extends the test from "unset" to "unset or empty". That one
	// rule is the whole difference between the two rows of conditionals.
	fires := !set
	if e.Colon {
		fires = !set || value == ""
	}

	switch e.Op {
	case syntax.ParamNone:
		return value
	case syntax.ParamDefault:
		if fires {
			return r.joinWord(e.Arg)
		}
		return value
	case syntax.ParamAssign:
		if fires {
			v := r.joinWord(e.Arg)
			// The side effect that outlives the expansion.
			r.setVar(e.Name, v)
			return v
		}
		return value
	case syntax.ParamAlternate:
		if fires {
			return ""
		}
		return r.joinWord(e.Arg)
	}
	// Anything else is left empty rather than guessed at.
	return ""
}

func (r *Runner) joinWord(w *syntax.Word) string {
	return strings.Join(r.expandWord(w), " ")
}

// ifs returns the field separators. Unset means the default; set and empty
// disables splitting, which is a different state rather than a degree of it.
func (r *Runner) ifs() (value string, set bool) {
	v, ok := r.getVar("IFS")
	if !ok {
		return " \t\n", false
	}
	return v, true
}

// splitFields implements docs/spec/grammar/word-splitting.md.
//
// The rule that makes this more than a strings.Split: a run of IFS whitespace
// is one delimiter and leading and trailing runs are discarded, while *each*
// non-whitespace separator delimits — so two adjacent ones produce an empty
// field. A trailing separator is absorbed and a leading one is not, which is
// the asymmetry a symmetric implementation gets wrong.
func splitFields(s string, ifs string, ifsSet bool) []string {
	if ifsSet && ifs == "" {
		// Set and empty disables the stage entirely, which is a different
		// state from unset rather than a degree of it.
		if s == "" {
			return nil
		}
		return []string{s}
	}
	if s == "" {
		return nil
	}

	isWS := func(c byte) bool {
		return strings.IndexByte(ifs, c) >= 0 && (c == ' ' || c == '\t' || c == '\n')
	}
	isSep := func(c byte) bool { return strings.IndexByte(ifs, c) >= 0 }

	var out []string
	i := 0
	for i < len(s) && isWS(s[i]) { // leading IFS whitespace is discarded
		i++
	}
	for i < len(s) {
		start := i
		for i < len(s) && !isSep(s[i]) {
			i++
		}
		out = append(out, s[start:i])
		if i >= len(s) {
			break
		}
		// One delimiter is: a run of IFS whitespace, at most one
		// non-whitespace separator, and another run of whitespace. Consuming
		// exactly that and then letting the loop read the next field is what
		// makes two adjacent non-whitespace separators produce one empty
		// field rather than two — the bug a hand-rolled version invites.
		for i < len(s) && isWS(s[i]) {
			i++
		}
		if i < len(s) && isSep(s[i]) {
			i++
			for i < len(s) && isWS(s[i]) {
				i++
			}
		}
		// A trailing delimiter is absorbed and does not produce a final
		// empty field; a leading one is not, which the loop above already
		// handled by reading an empty field before consuming it.
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// specialParam answers the parameters that are not variables.
func (r *Runner) specialParam(e *syntax.ParamExpr) (string, bool) {
	switch e.Name {
	case "#":
		return itoa(len(r.Params)), true
	case "?":
		return itoa(r.status), true
	case "0":
		return r.Name, true
	case "*":
		if e.Length {
			// docs/spec/grammar/parameter-expansion.md: `${#*}` is the count
			// everywhere but dash, which gives the length of the joined
			// string. The count is the majority and the POSIX reading.
			return itoa(len(r.Params)), true
		}
		// `$*` joins with the *first character* of IFS, not with a space.
		sep := " "
		if v, set := r.ifs(); set {
			if v == "" {
				sep = ""
			} else {
				sep = v[:1]
			}
		}
		return strings.Join(r.Params, sep), true
	case "@":
		if e.Length {
			return itoa(len(r.Params)), true
		}
		// Reached only where expandAt declined — inside another expansion's
		// operand, say — where joining is the sensible answer.
		return strings.Join(r.Params, " "), true
	}
	if n, ok := atoi(e.Name); ok && n >= 1 {
		if n <= len(r.Params) {
			return r.Params[n-1], true
		}
		return "", true
	}
	return "", false
}

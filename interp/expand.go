// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"strings"

	"github.com/blairham/sh/syntax"
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
	// Brace expansion comes first and can turn one word into several, so it
	// wraps the rest rather than being a stage inside it.
	if words := braceExpand(w); len(words) > 1 &&
		r.ask(r.sem().BraceExpansion, "brace expansion") {
		var out []string
		for _, bw := range words {
			out = append(out, r.expandOneWord(bw)...)
		}
		return out
	}
	return r.expandOneWord(w)
}

// expandOneWord is the pipeline for a single word, after braces.
func (r *Runner) expandOneWord(w *syntax.Word) []string {
	if w == nil {
		return nil
	}
	r.expandTilde(w)
	r.expandEquals(w)

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

	// Pathname expansion is the last stage, and it acts on whole fields: a
	// pattern that matches nothing is passed through unchanged.
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if matches := r.glob(f); len(matches) > 0 {
			out = append(out, matches...)
			continue
		}
		out = append(out, globUnescape(f))
	}
	return out
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
	return []string{globUnescape(b.String())}
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
	// `${a[@]}` is one field per element for the same reason `"$@"` is one
	// per parameter: joining them would lose an element containing a space.
	if e.Index != nil && e.Op == syntax.ParamNone && !e.Length {
		if elems, ok := r.arraySubscript(e); ok {
			ifs, set := r.ifs()
			if r.subscriptText(e.Index) == "*" {
				// `[*]` is *one* field with the elements joined, where `[@]`
				// is one field each — the same difference `"$*"` has from
				// `"$@"`, and the reason both spellings exist. Taking the
				// `[@]` path for it produced no field at all inside a larger
				// word, so `echo "[${a[*]}]"` printed `[]`.
				joined := strings.Join(elems, ifsFirst(ifs, set))
				if s.Quoting != syntax.Unquoted {
					return []string{globEscape(joined)}, true
				}
				return splitFields(joined, ifs, set), true
			}
			if s.Quoting != syntax.Unquoted {
				return escapeAll(elems), true
			}
			var out []string
			for _, el := range elems {
				out = append(out, splitFields(el, ifs, set)...)
			}
			return out, true
		}
	}
	if e.Name != "@" || e.Op != syntax.ParamNone || e.Length {
		return nil, false
	}
	if s.Quoting != syntax.Unquoted {
		// Escaped for the same reason every other quoted expansion is: the
		// fields go on to pathname expansion, and a `*` in a *value* is not
		// a pattern. Returning them raw made `set -- "$x"` glob.
		return escapeAll(r.Params), true
	}
	// Unquoted, each parameter is then split like any other expansion.
	ifs, set := r.ifs()
	var out []string
	for _, p := range r.Params {
		out = append(out, splitFields(p, ifs, set)...)
	}
	if !r.ask(r.sem().GlobExpansionResults, "globbing the result of an expansion") {
		out = escapeAll(out)
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
		if s.Quoting == syntax.DollarSingleQuoted {
			// `$'a\tb'` is a tab, and the lexer kept both bytes on purpose so
			// the source text stays recoverable. Decoding it here is what was
			// missing: the quoting was recorded, nothing read it, and the
			// escape reached the output as the two characters it was written
			// as. The result is quoted text like any other.
			return globEscape(expandDollarSingle(s.Value)), false
		}
		// Literal text is never split, however it was written. Its
		// metacharacters stay live only when it was unquoted; quoting is
		// what decides whether text is a pattern at all.
		if unquoted {
			return s.Value, false
		}
		return globEscape(s.Value), false
	case syntax.ParamExp:
		v := r.expandParam(s.Param)
		return r.expansionResult(v, unquoted, r.sem().SplitParamExpansion, "splitting an unquoted parameter expansion")
	case syntax.CommandSubst:
		v := r.commandSubst(r.ctx, s.Value)
		return r.expansionResult(v, unquoted, r.sem().SplitCommandSubstitution, "splitting an unquoted command substitution")
	case syntax.ArithSubst:
		v, err := r.evalArith(s.Arith)
		if err != nil {
			// The expression as written is what dash and ksh93 quote back,
			// and the span still has it: the parser keeps the raw text
			// beside the tree it built from it.
			ae, _ := err.(arithError)
			token := ae.token
			if token == "" {
				// bash blames the whole expression when the failing part is
				// the whole expression, which is also the honest answer when
				// the tree cannot name a smaller piece.
				token = strings.TrimSpace(s.Value)
			}
			if ae.complete {
				r.diagf("%s\n", err.Error())
			} else {
				r.diagf("%s\n", Wording(r.diag().ArithError, "%[2]s",
					strings.TrimSpace(s.Value), err.Error(), token))
			}
			// The command must not run: `echo $((1/0))` fails in every shell
			// in the panel rather than echoing an empty string.
			r.expandErr = true
			return "", false
		}
		return r.expansionResult(itoa(v), unquoted, r.sem().SplitParamExpansion, "splitting an unquoted arithmetic expansion")
	}
	return "", false
}

// expansionResult applies the two axes that govern what happens to the result
// of an expansion: whether it is field-split, and whether its metacharacters
// stay live for pathname expansion.
//
// Quoted, neither applies — that is universal. Unquoted, both are dialect
// questions, and zsh answers no to both while everything else answers yes.
func (r *Runner) expansionResult(v string, unquoted bool, split Answer, axis string) (string, bool) {
	if !unquoted {
		return globEscape(v), false
	}
	// Both axes are asked only when the value could actually differ: a result
	// with no separator in it is not split either way, and one with no
	// metacharacter is not a pattern either way.
	doSplit := false
	// Asked against the *actual* separators, not a guess at them: with
	// IFS=: a value holding no space still splits, and hardcoding whitespace
	// here silently stopped it.
	if ifs, _ := r.ifs(); containsAnyOf(v, ifs) {
		doSplit = r.ask(split, axis)
	}
	if hasUnescapedMeta(v) &&
		!r.ask(r.sem().GlobExpansionResults, "globbing the result of an expansion") {
		// zsh does not treat the result of an expansion as a pattern. The
		// same rule decides `[[ abc == $p ]]`, which is one behavior
		// observed twice rather than two quirks.
		v = globEscape(v)
	}
	return v, doSplit
}

func containsAnyOf(s, chars string) bool {
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(chars, s[i]) >= 0 {
			return true
		}
	}
	return false
}

// expandParam handles the forms this slice implements.
func (r *Runner) expandParam(e *syntax.ParamExpr) string {
	if e == nil {
		return ""
	}
	// An array subscript supplies a value too, and the operators apply to it
	// exactly as they do to a variable.
	if e.Index != nil {
		if elems, ok := r.arraySubscript(e); ok {
			if e.Length {
				// `${#a[@]}` is the number of elements; `${#a[0]}` is the
				// length of one. The subscript decides which question was
				// asked, which is why this is here rather than below.
				idx := r.subscriptText(e.Index)
				if idx == "@" || idx == "*" {
					return itoa(len(elems))
				}
				return itoa(len(strings.Join(elems, "")))
			}
			return strings.Join(elems, " ")
		}
	}

	// A special parameter supplies a *value*; it does not skip the operators.
	// Returning here was a bug: `${1##*/}` left its argument untouched,
	// because the positional parameter answered and the trim never ran.
	value, set := r.specialParam(e)
	if !set {
		value, set = r.getVar(e.Name)
	}

	if !set {
		r.checkNounset(e)
	}

	if e.Indirect {
		// `${!x}` reads x, then reads *that* as a name — in bash. ksh93
		// parses the same text and yields the name itself, so the grammar
		// having accepted it is not enough to know what it means.
		if r.ask(r.sem().IndirectionYieldsName, "${!x} yielding the name") {
			return e.Name
		}
		if !set || value == "" {
			return ""
		}
		value, set = r.getVar(value)
	}

	if e.Length {
		// `${#@}` is the number of parameters, not the length of anything —
		// so the length question is answered once, here, and specialParam
		// supplies only the value.
		if e.Name == "@" || e.Name == "*" {
			return itoa(r.specialLength())
		}
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

	case syntax.ParamTrimPrefix, syntax.ParamTrimPrefixLong,
		syntax.ParamTrimSuffix, syntax.ParamTrimSuffixLong:
		return r.trimWith(value, r.patternOf(e.Arg), e.Op)

	case syntax.ParamReplace:
		return r.replaceWith(value, r.patternOf(e.Arg), r.joinWord(e.Arg2), e)

	case syntax.ParamSubstring:
		return substring(value, r.numOf(e.Arg), e.Arg2, r)

	case syntax.ParamUpper:
		return strings.ToUpper(value)
	case syntax.ParamLower:
		return strings.ToLower(value)
	}
	// Anything else is left empty rather than guessed at.
	return ""
}

// numOf evaluates a word as a number, for a substring's offset and length.
func (r *Runner) numOf(w *syntax.Word) int {
	if w == nil {
		return 0
	}
	n, err := r.parseNum(strings.TrimSpace(r.joinWord(w)))
	if err != nil {
		return 0
	}
	return n
}

// trim removes a matching prefix or suffix.
//
// Doubling the operator is what selects the longer match; there is no
// greediness syntax inside the pattern, so the search order is the whole
// implementation. A pattern that does not match removes nothing.
// trimWith and replaceWith resolve the caret axis for the pattern before
// handing it to the matcher, which has no Runner and should not need one.
func (r *Runner) trimWith(value, pattern string, op syntax.ParamOp) string {
	return trim(value, pattern, op, r.patternOpts(pattern))
}

func (r *Runner) replaceWith(value, pattern, with string, e *syntax.ParamExpr) string {
	return replace(value, pattern, with, e, r.patternOpts(pattern))
}

func trim(value, pattern string, op syntax.ParamOp, o patternOpts) string {
	prefix := op == syntax.ParamTrimPrefix || op == syntax.ParamTrimPrefixLong
	longest := op == syntax.ParamTrimPrefixLong || op == syntax.ParamTrimSuffixLong

	// Candidate split points, ordered so the first match found is the one
	// wanted: shortest first for the single operators, longest first for the
	// doubled ones.
	idx := make([]int, 0, len(value)+1)
	for i := 0; i <= len(value); i++ {
		idx = append(idx, i)
	}
	if (prefix && longest) || (!prefix && !longest) {
		for l, r := 0, len(idx)-1; l < r; l, r = l+1, r-1 {
			idx[l], idx[r] = idx[r], idx[l]
		}
	}
	for _, i := range idx {
		if prefix {
			if matchPattern(pattern, value[:i], o) {
				return value[i:]
			}
			continue
		}
		if matchPattern(pattern, value[i:], o) {
			return value[:i]
		}
	}
	return value
}

// replace substitutes a matching span, once or everywhere.
//
// The anchored forms match only at one end, which is what `/#` and `/%` mean.
func replace(value, pattern, with string, e *syntax.ParamExpr, o patternOpts) string {
	switch e.Anchor {
	case '#':
		for i := len(value); i >= 0; i-- {
			if matchPattern(pattern, value[:i], o) {
				return with + value[i:]
			}
		}
		return value
	case '%':
		for i := 0; i <= len(value); i++ {
			if matchPattern(pattern, value[i:], o) {
				return value[:i] + with
			}
		}
		return value
	}

	var b strings.Builder
	for i := 0; i <= len(value); {
		// The longest match at this position, so `*` behaves as it does
		// everywhere else rather than matching empty and looping.
		end := -1
		for j := len(value); j >= i; j-- {
			if matchPattern(pattern, value[i:j], o) {
				end = j
				break
			}
		}
		if end < 0 || end == i && pattern != "" && !matchPattern(pattern, "", o) {
			if i < len(value) {
				b.WriteByte(value[i])
			}
			i++
			continue
		}
		b.WriteString(with)
		if !e.All {
			b.WriteString(value[end:])
			return b.String()
		}
		if end == i {
			// An empty match must still make progress.
			if i < len(value) {
				b.WriteByte(value[i])
			}
			i++
			continue
		}
		i = end
	}
	return b.String()
}

// substring takes a slice of the value.
func substring(value string, off int, lenWord *syntax.Word, r *Runner) string {
	if off < 0 {
		off += len(value)
	}
	if off < 0 {
		off = 0
	}
	if off > len(value) {
		return ""
	}
	if lenWord == nil {
		return value[off:]
	}
	n := r.numOf(lenWord)
	if n < 0 {
		// A negative length is an offset from the end.
		n = len(value) + n - off
	}
	if n < 0 {
		n = 0
	}
	if off+n > len(value) {
		n = len(value) - off
	}
	return value[off : off+n]
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
// specialParam supplies the value of a parameter that is not a variable. The
// caller applies the operators, exactly as it does for a variable — the two
// differ in where the value comes from and in nothing else.
func (r *Runner) specialParam(e *syntax.ParamExpr) (string, bool) {
	switch e.Name {
	case "#":
		return itoa(len(r.Params)), true
	case "?":
		return itoa(r.status), true
	case "$":
		// The shell's own process id, and the same inside a subshell: POSIX
		// says `$$` is the *invoking* shell's, which is what makes it usable
		// as a lock name. Nothing here forks for a subshell, so the process
		// id is already the right one.
		return itoa(os.Getpid()), true
	case "!":
		if r.lastJob == nil {
			return "", true
		}
		return itoa(r.lastJob.PID), true
	case "0":
		// zsh reports the *function's* name inside a function where every
		// other shell reports the shell's.
		if r.inFunc != "" && r.ask(r.sem().DollarZeroInFunctionIsFunctionName, "$0 inside a function") {
			return r.inFunc, true
		}
		return r.Name, true
	case "*":
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
		// Reached only where expandAt declined — inside another expansion's
		// operand, say — where joining is the sensible answer.
		return strings.Join(r.Params, " "), true
	}
	if n, ok := atoi(e.Name); ok && n >= 1 {
		if n <= len(r.Params) {
			return r.Params[n-1], true
		}
		// Reported as *unset*, not as empty. Saying "set" here made
		// `${1-default}` yield nothing, because the default only fires for a
		// parameter that is not set — and it hid every unset positional from
		// `set -u`, which is what turned this up.
		return "", false
	}
	return "", false
}

// specialLength answers `${#@}` and `${#*}`.
//
// dash gives the length of the joined string where everything else gives the
// count. Both are plausible numbers and neither errors, which is what makes
// it worth a switch rather than a majority verdict.
func (r *Runner) specialLength() int {
	if r.ask(r.sem().LengthOfSpecialIsCount, "${#@} being the count of parameters") {
		return len(r.Params)
	}
	return len(strings.Join(r.Params, " "))
}

// expandRawText expands text the lexer kept raw — a here-document body — by
// lexing it and expanding the spans that come back.
//
// Newlines are preserved, because the text is input to a command rather than
// a word: lexing alone would drop them as token separators.
func (r *Runner) expandRawText(text string) string {
	var b strings.Builder
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			b.WriteByte('\n')
		}
		l := syntax.NewLexer(line, r.dialect())
		var spans []syntax.Span
		last := 0
		for {
			tk := l.Next()
			if tk.Kind == syntax.TokEOF {
				break
			}
			// Blanks between tokens are content here, not separators.
			if tk.Pos.Offset > last {
				spans = append(spans, syntax.Span{Kind: syntax.Literal, Value: line[last:tk.Pos.Offset]})
			}
			last = tk.End.Offset
			if tk.Kind == syntax.TokWord {
				w := r.parseSpans(tk.Spans)
				spans = append(spans, w...)
				continue
			}
			spans = append(spans, syntax.Span{Kind: syntax.Literal, Value: tk.Text})
		}
		if last < len(line) {
			spans = append(spans, syntax.Span{Kind: syntax.Literal, Value: line[last:]})
		}
		for _, s := range spans {
			text, _ := r.expandSpan(s)
			b.WriteString(globUnescape(text))
		}
	}
	return b.String()
}

// parseSpans fills in the parsed form of any expansion the lexer left raw,
// which the parser normally does when it builds a word.
func (r *Runner) parseSpans(spans []syntax.Span) []syntax.Span {
	out := make([]syntax.Span, len(spans))
	copy(out, spans)
	p := syntax.NewParser("", r.dialect())
	for i := range out {
		switch {
		case out[i].Kind == syntax.ParamExp && out[i].Param == nil:
			out[i].Param = p.ParseParamExpFor(out[i].Value, out[i].Pos)
		case out[i].Kind == syntax.ArithSubst && out[i].Arith == nil:
			out[i].Arith = p.ParseArithFor(out[i].Value, out[i].Pos)
		}
	}
	return out
}

// expandTilde replaces a leading `~` with the home directory.
//
// Only unquoted, only at the start of a word, and only up to the first slash:
// `echo ~` expands, `echo "~"` does not, and `echo a~` does not because the
// tilde is not where a word begins.
//
// An assignment's value is also a tilde context, which is what makes
// `PATH=~/bin` work — and it comes for free here, because the parser gives an
// assignment's value its own word.
// expandEquals replaces `=cmd` with the path of cmd, which zsh alone does.
//
// It sits beside expandTilde because it is the same kind of thing and zsh
// groups them together: both rewrite the head of an unquoted word into a
// filename before anything else looks at it. Quoting removes it, and so does
// anything other than `=` in first position — `a=b` is an assignment and stays
// one.
func (r *Runner) expandEquals(w *syntax.Word) {
	if len(w.Spans) == 0 {
		return
	}
	s := &w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted ||
		!strings.HasPrefix(s.Value, "=") || len(s.Value) == 1 {
		return
	}
	if !r.ask(r.sem().EqualsExpansion, "`=cmd` expanding to a path") {
		return
	}
	name := s.Value[1:]
	// The script's PATH, like every other lookup here.
	path, err := r.lookPath(name)
	if err != nil {
		// zsh reports the name without a colon and abandons the script,
		// which is what any failed expansion does here.
		r.diagf("%s\n", Wording(r.diag().EqualsNotFound, "%s not found", name))
		r.expandErr = true
		return
	}
	s.Value = path
}

func (r *Runner) expandTilde(w *syntax.Word) {
	if len(w.Spans) == 0 {
		return
	}
	s := &w.Spans[0]
	if s.Kind != syntax.Literal || s.Quoting != syntax.Unquoted ||
		!strings.HasPrefix(s.Value, "~") {
		return
	}
	rest := s.Value[1:]
	name, tail := rest, ""
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		name, tail = rest[:i], rest[i:]
	}
	if name != "" {
		// `~user` needs a user database this package does not carry, so it is
		// left alone rather than guessed at.
		return
	}
	home, ok := r.getVar("HOME")
	if !ok {
		return
	}
	s.Value = home + tail
}

// escapeAll marks every field's metacharacters as literal, for the fields
// that reach pathname expansion without having gone through expandSpan.
func escapeAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = globEscape(s)
	}
	return out
}

// checkNounset reports an unset parameter under `set -u`.
//
// Only where the expansion would actually *use* the value. `${x:-d}` and
// `${x-d}` supply one, `${x+d}` asks whether it is set, and `${x:?m}` reports
// in its own words — none of those is an error, and all four shells agree.
func (r *Runner) checkNounset(e *syntax.ParamExpr) {
	if !r.nounset {
		return
	}
	switch e.Op {
	case syntax.ParamDefault, syntax.ParamAssign, syntax.ParamAlternate, syntax.ParamError:
		return
	}
	switch e.Name {
	case "@", "*":
		// No parameters is not the same as unset: `"$@"` with none is empty
		// and quiet in all four.
		return
	}
	if isPositional(e.Name) && !r.ask(r.sem().UnsetPositionalIsAllowed, "an unset positional parameter under set -u") {
		// ksh93 alone lets `$1` be empty here. bash writes the `$` back for
		// a positional and not for a name, which is why the wording is its
		// own field rather than a decoration applied here.
		format := r.diag().UnboundPositional
		if format == "" {
			format = r.diag().UnboundVariable
		}
		r.fatal("%s\n", Wording(format, "%s: parameter not set", e.Name))
		return
	}
	if !isPositional(e.Name) {
		r.fatal("%s\n", Wording(r.diag().UnboundVariable, "%s: parameter not set", e.Name))
	}
}

// isPositional reports whether a parameter name is a positional one.
func isPositional(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// expandDollarSingle decodes the escapes `$'…'` gives meaning to.
//
// The same set `printf` reads, plus `\e` for escape and the hexadecimal and
// unicode forms, and without `\c`: there is no output to stop here, only a
// word being built.
func expandDollarSingle(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		switch c := s[i+1]; c {
		case 'n':
			b.WriteByte('\n')
			i += 2
		case 't':
			b.WriteByte('\t')
			i += 2
		case 'r':
			b.WriteByte('\r')
			i += 2
		case 'a':
			b.WriteByte('\a')
			i += 2
		case 'b':
			b.WriteByte('\b')
			i += 2
		case 'f':
			b.WriteByte('\f')
			i += 2
		case 'v':
			b.WriteByte('\v')
			i += 2
		case 'e', 'E':
			b.WriteByte(0x1b)
			i += 2
		case '\\', '\'', '"', '?':
			b.WriteByte(c)
			i += 2
		case 'x':
			n, used := scanBase(s[i+2:], 16, 2)
			if used == 0 {
				b.WriteString(`\x`)
				i += 2
				continue
			}
			b.WriteByte(byte(n))
			i += 2 + used
		case 'u', 'U':
			width := 4
			if c == 'U' {
				width = 8
			}
			n, used := scanBase(s[i+2:], 16, width)
			if used == 0 {
				b.WriteByte('\\')
				b.WriteByte(c)
				i += 2
				continue
			}
			b.WriteRune(rune(n))
			i += 2 + used
		case '0', '1', '2', '3', '4', '5', '6', '7':
			n, used := scanBase(s[i+1:], 8, 3)
			b.WriteByte(byte(n))
			i += 1 + used
		default:
			// An escape with no meaning keeps both characters, which is what
			// the panel does rather than dropping the backslash.
			b.WriteByte('\\')
			b.WriteByte(c)
			i += 2
		}
	}
	return b.String()
}

// scanBase reads up to max digits in the given base, reporting how many it
// used so the caller can tell "no digits at all" from a zero.
func scanBase(s string, base, maxDigits int) (int, int) {
	n, used := 0, 0
	for used < maxDigits && used < len(s) {
		d := digitValue(s[used])
		if d < 0 || d >= base {
			break
		}
		n = n*base + d
		used++
	}
	return n, used
}

func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// ifsFirst is the character `*` joins with: the first of IFS, a space when
// IFS is unset, and nothing at all when IFS is set but empty.
func ifsFirst(ifs string, set bool) string {
	if !set {
		return " "
	}
	if ifs == "" {
		return ""
	}
	return ifs[:1]
}

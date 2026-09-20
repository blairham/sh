// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"fmt"
	"reflect"
	"strings"
)

// What "the same program" means.
//
// Printing normalizes, so tree *identity* is the wrong question: a `case`
// arm's optional `(` is dropped, `;;` gains a space, a body is re-indented,
// and none of that changes what runs. Four kinds of difference are allowed
// below and nothing else is, and each of them is either a field the tree
// keeps for a diagnostic or a normalization the printer documents where it
// makes it.
//
// The list is deliberately short and deliberately awkward to extend: a
// printer change that requotes a character or drops a marker fails this
// test, and the fix is either the printer or a row here with the reason
// written out. That is the whole value of it to a formatter — a formatter is
// a printer whose output nobody re-reads character by character.

var (
	posType         = reflect.TypeOf(Pos{})
	arithBinaryType = reflect.TypeOf(ArithBinary{})
	arithNumType    = reflect.TypeOf(ArithNum{})
	wordType        = reflect.TypeOf(Word{})
	stmtType        = reflect.TypeOf(Stmt{})
	redirType       = reflect.TypeOf(Redirect{})
	spanType        = reflect.TypeOf(Span{})
	assignType      = reflect.TypeOf(Assign{})
	paramExprType   = reflect.TypeOf(ParamExpr{})
	fileType        = reflect.TypeOf(File{})
)

// SameProgram reports whether two trees are the same program, and where they
// first differ if they are not.
//
// This is the question a formatter has to be able to ask about its own
// output, and #1401 is where it was decided that asking it belongs here
// rather than in a test file: the formatter and the printer's round trip must
// mean the same thing by "the same program", and two copies of the definition
// would eventually not.
//
// Positions are ignored, and so is anything above that keeps the source
// spelling for a diagnostic. Everything else must match.
func SameProgram(a, b *File) (string, bool) {
	return sameNode(reflect.ValueOf(a), reflect.ValueOf(b), "file")
}

// spellingOnly reports whether a field holds the source *spelling* of a node
// rather than anything the program does.
//
// Six of them, and each exists because something has to quote the input
// back long after it was read: a background statement's text is what a jobs
// listing shows, a redirection's is what one grammar's ambiguous-redirect
// diagnostic names, a subscript's is what a `bad array subscript` names, and
// a loop or `case` header's is what an execution trace writes. All four are
// the input's spelling by definition, so a printer that re-spelled the
// construct re-spells them with it — `a[${i}]=v` printed back as `a[$i]=v`
// is the same program, and the diagnostic it would earn names the spelling
// that is now there.
//
// The sixth is Stmt.Term, which is that rule about a statement's own
// terminator rather than about a word: a listing that says the source's
// separator back needs to know a `;` from a newline, and the two are one
// program written two ways.
//
// The fourth is Bare, which says a parameter expansion was written without
// braces, and which the span and the expansion node each carry. `${x}` and
// `$x` are one program and the printer moves between them freely — it adds
// braces wherever what follows would run into the name — so the flag on its
// own is spelling. What is *not* spelling is the reading the short form can
// carry, and that has a field of its own: ParamExpr.BareIndexText is set
// exactly where an unbraced expansion took a subscript, and it is compared
// like any other node. So `$a[1]` printed as `${a[1]}` is still caught, by
// the field that says the two mean different things, rather than by the one
// that says they are spelled differently.
//
// ParamExpr.Bare is the same flag on the node, kept there because one
// diagnostic reads it: `set -u` writes the `$` back in front of a positional
// in one dialect and drops it for the braced spelling of the same parameter.
// That is the rule the first paragraph states — a spelling kept so something
// can quote the input back — so `${7}` printed as `$7` is the same program,
// and the refusal it would earn names the spelling that is now there.
func spellingOnly(t reflect.Type, name string) bool {
	if name == "Text" {
		return t == stmtType || t == redirType
	}
	if name == "Term" {
		// Which token terminated a statement — see [Stmt.Term]. `a;` and `a`
		// followed by a newline are one program written two ways, and a
		// printer moves between them by arrangement: an engine that gives
		// each statement a line writes `;` and a newline where the source
		// wrote one of them alone. So this is the input's spelling in
		// exactly the sense the fields above are, kept because a listing has
		// to say the source's own separator back.
		return t == stmtType
	}
	if name == "IndexText" {
		return t == assignType || t == paramExprType
	}
	if name == "Bare" {
		return t == spanType || t == paramExprType
	}
	if name == "Tail" {
		// The expression's text from a numeral to the end of it, kept so one
		// diagnostic can quote the input back — see [ArithNum].Tail. It is
		// the input's spelling by definition and it carries the blanks
		// around what follows the numeral, so `for ((i=0 ; ; ))` printed
		// back without the space before the `;` is the same program with a
		// shorter tail.
		return t == arithNumType
	}
	if name == "Substitutions" {
		// An **index** over the tree rather than a part of it: every span it
		// lists is compared where it stands in the word that holds it, and
		// what is gathered here is a second reference to the same spans for
		// one reader. Printing may also put two statements on lines the
		// source did not, which moves nothing about the program and would
		// move this list. See [File.Substitutions].
		return t == fileType
	}
	if name == "YStart" {
		// A position, spelled as an offset rather than as a Pos because it
		// indexes one expression's own text. It is skipped for the reason
		// every Pos is skipped: printed source has its own offsets, and
		// `1/(0)` reprinted with a space somewhere else is the same program
		// written down differently.
		return t == arithBinaryType
	}
	// Header, which only the clauses that keep one have.
	return name == "Header"
}

func sameNode(a, b reflect.Value, path string) (string, bool) {
	if a.Type() != b.Type() {
		return path + ": a different node", false
	}
	switch a.Type() {
	case posType:
		// Printed source has its own positions, by construction: the whole
		// point of printing is that the text is not the text that was read.
		return "", true
	case wordType:
		return sameWord(a, b, path)
	case redirType:
		return sameRedirect(a, b, path)
	}
	switch a.Kind() {
	case reflect.Pointer, reflect.Interface:
		if a.IsNil() != b.IsNil() {
			return fmt.Sprintf("%s: one is absent", path), false
		}
		if a.IsNil() {
			return "", true
		}
		return sameNode(a.Elem(), b.Elem(), path)
	case reflect.Struct:
		for i := range a.NumField() {
			f := a.Type().Field(i)
			// Unexported fields are skipped, which is not an omission: this
			// walk grew up in an external test package where they were
			// invisible, and every promise it makes was measured with them
			// invisible. Letting it descend into them here would silently
			// widen what it compares.
			if !f.IsExported() || spellingOnly(a.Type(), f.Name) {
				continue
			}
			if why, ok := sameNode(a.Field(i), b.Field(i), path+"."+f.Name); !ok {
				return why, false
			}
		}
		return "", true
	case reflect.Slice, reflect.Array:
		if a.Len() != b.Len() {
			return fmt.Sprintf("%s: %d against %d", path, a.Len(), b.Len()), false
		}
		for i := range a.Len() {
			if why, ok := sameNode(a.Index(i), b.Index(i), fmt.Sprintf("%s[%d]", path, i)); !ok {
				return why, false
			}
		}
		return "", true
	case reflect.String:
		if a.String() != b.String() {
			return fmt.Sprintf("%s: %q against %q", path, a.String(), b.String()), false
		}
	case reflect.Bool:
		if a.Bool() != b.Bool() {
			return fmt.Sprintf("%s: %v against %v", path, a.Bool(), b.Bool()), false
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if a.Int() != b.Int() {
			return fmt.Sprintf("%s: %d against %d", path, a.Int(), b.Int()), false
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if a.Uint() != b.Uint() {
			return fmt.Sprintf("%s: %d against %d", path, a.Uint(), b.Uint()), false
		}
	}
	return "", true
}

// sameRedirect compares two redirections, allowing the two normalizations
// the printer's `dup` documents and measures.
//
// It writes out the descriptor being redirected where the target is one a
// reader can resolve — `>&2` comes back as `1>&2` — which adds nothing the
// source did not mean, since the operator's own default is what it adds. And
// it writes a close as `>&-` whichever operator asked for it, so `0<&-`
// comes back as `0>&-`: both close the same descriptor.
func sameRedirect(a, b reflect.Value, path string) (string, bool) {
	an, bn := a.FieldByName("N"), b.FieldByName("N")
	if an.IsNil() && !bn.IsNil() {
		want := "1"
		if Kind(a.FieldByName("Op").Uint()) == TokLessAmp {
			want = "0"
		}
		if got := wordLiteral(bn); got != want {
			return fmt.Sprintf("%s.N: the printer put %q where the operator means %q", path, got, want), false
		}
		bn = an
	}
	if why, ok := sameNode(an, bn, path+".N"); !ok {
		return why, false
	}
	aop, bop := Kind(a.FieldByName("Op").Uint()), Kind(b.FieldByName("Op").Uint())
	closing := wordLiteral(a.FieldByName("Word")) == "-"
	if (!closing || aop != TokLessAmp || bop != TokGreatAmp) && aop != bop {
		return fmt.Sprintf("%s.Op: %s against %s", path, aop, bop), false
	}
	// HeredocAtEOF is the input having run out, and finishing it is the
	// printer's job on that shape: #962 has it supply the delimiter the text
	// never had, so printed source is a finished program by construction.
	// Allowed in that direction alone — a printer that turned a *terminated*
	// here-document into one running to the end of the input would have
	// swallowed the delimiter, which is the bug #962 was.
	//
	// It is not free, and writing it down is the point of not simply
	// skipping the field. The flag decides whether the body gains the newline
	// its last line never had — see
	// interp.Semantics.UnterminatedHeredocGainsATrailingNewline — so under
	// the three dialects that supply none, `cat <<X` / `body` prints to a
	// program whose body is one byte longer. A printer cannot both end the
	// input and keep that byte, because the delimiter needs a line of its
	// own.
	if !a.FieldByName("HeredocAtEOF").Bool() && b.FieldByName("HeredocAtEOF").Bool() {
		return path + ".HeredocAtEOF: the printer left the here-document running to the end of the input", false
	}
	for i := range a.NumField() {
		f := a.Type().Field(i)
		if !f.IsExported() || f.Name == "N" || f.Name == "Op" ||
			f.Name == "HeredocAtEOF" || spellingOnly(redirType, f.Name) {
			continue
		}
		if why, ok := sameNode(a.Field(i), b.Field(i), path+"."+f.Name); !ok {
			return why, false
		}
	}
	return "", true
}

// wordLiteral joins a word's span values, which is the word with its quotes
// taken off and nothing else done to it. A word nothing wrote is empty: the
// redirection a `|&` stands for is in the tree without anyone having typed
// it.
func wordLiteral(w reflect.Value) string {
	if w.IsNil() {
		return ""
	}
	spans := w.Elem().FieldByName("Spans")
	var b strings.Builder
	for i := range spans.Len() {
		b.WriteString(spans.Index(i).FieldByName("Value").String())
	}
	return b.String()
}

// sameWord compares two words by what they mean rather than span for span.
//
// Requoting *within* a word is not free — a protected `(` is a parenthesis
// where a bare one is a group, which is the whole of #1221 — but where a
// character means nothing to the matcher, protecting it changes nothing, and
// the printer does protect two such characters for reasons of its own. So a
// word is compared as its literal text, character by character, each with
// whether the source protected it, and the runs are joined across span
// boundaries: the printer may split one span into three putting quotes
// around the middle of it, and does.
//
// [Span.PatternGroup] itself is not compared, and does not need to
// be: it is what the scanner reads off an *unprotected* `(`, so comparing
// the protection compares the flag with it.
func sameWord(a, b reflect.Value, path string) (string, bool) {
	x, y := atomsOf(a), atomsOf(b)
	if len(x) != len(y) {
		return fmt.Sprintf("%s: %s against %s", path, describe(x), describe(y)), false
	}
	for i := range x {
		p, q := x[i], y[i]
		switch {
		case p.literal != q.literal:
			return fmt.Sprintf("%s: %s against %s", path, describe(x), describe(y)), false
		case !p.literal:
			if why, ok := sameNode(p.span, q.span, fmt.Sprintf("%s[%d]", path, i)); !ok {
				return why, false
			}
		case p.ch != q.ch:
			return fmt.Sprintf("%s: %s against %s", path, describe(x), describe(y)), false
		case p.protected != q.protected:
			if !p.protected && freeToProtect(p.ch) {
				continue
			}
			what := "protected"
			if p.protected {
				what = "left bare"
			}
			return fmt.Sprintf("%s: %q was %s: %s against %s",
				path, string(p.ch), what, describe(x), describe(y)), false
		}
	}
	return "", true
}

// atom is one character of a word's literal text, or one substitution in it.
type atom struct {
	literal   bool
	ch        byte
	protected bool
	span      reflect.Value
}

func atomsOf(w reflect.Value) []atom {
	var out []atom
	spans := w.FieldByName("Spans")
	for i := range spans.Len() {
		s := spans.Index(i)
		if SpanKind(s.FieldByName("Kind").Uint()) != Literal {
			out = append(out, atom{span: s})
			continue
		}
		protected := Quoting(s.FieldByName("Quoting").Uint()) != Unquoted
		v := s.FieldByName("Value").String()
		for j := 0; j < len(v); j++ {
			out = append(out, atom{literal: true, ch: v[j], protected: protected})
		}
	}
	return out
}

// freeToProtect are the characters a print may protect without changing the
// program. Two of them, both reached by the corpus, and the set is this
// small on purpose.
//
// A literal dollar, which the printer backslashes because bare it could open
// something in a grammar that has a construct this one does not: `$"a"` is a
// dollar and a quoted string under one flag and a translatable string under
// another. Neither reading is a pattern, so `$` and `\$` match the same
// subject.
//
// And a newline, which the printer puts in single quotes rather than behind
// a backslash — a backslash before a newline is a line continuation and the
// next read removes it, so that is the one protection that would lose the
// character. Reached by a `case` arm whose pattern list spans a line.
//
// One direction only. A protection *removed* is a change even for a
// character that means nothing to a matcher: an unquoted here-document
// delimiter is a body that expands, and its letters mean nothing to anything
// else.
func freeToProtect(c byte) bool { return c == '$' || c == '\n' }

// describe renders a word's atoms for a failure message, marking a protected
// character with a leading backslash so the difference is legible.
func describe(atoms []atom) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, a := range atoms {
		if !a.literal {
			b.WriteString("${…}")
			continue
		}
		if a.protected {
			b.WriteByte('\\')
		}
		if a.ch == '\n' {
			b.WriteString("\\n")
			continue
		}
		b.WriteByte(a.ch)
	}
	b.WriteByte('"')
	return b.String()
}

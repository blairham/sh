// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// The newlines around a here-document's body are not the printer's to assume.
//
// A body carries the newlines the input gave it, and the last of those is the
// only reason the delimiter after it begins a line. Assume it and a body that
// ran to the end of a file whose last line had none comes back with the
// delimiter joined onto it; assume the one *before* a body and a second body
// on the same command opens with a blank line that was never in it.
//
// Both are silent. The text comes back valid, parses, and means something
// else — which is why these assert the printed bytes rather than that the
// result re-parses. A re-parse check passes against both defects.
func TestAHereDocumentBodyKeepsItsOwnLines(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want string
		why  string
	}{
		{
			name: "a body that ended its own last line",
			src:  "cat <<END\nbody\nEND\n",
			want: "cat << END\nbody\nEND\n",
			why:  "the ordinary shape, where the input supplied the newline the delimiter needs",
		},
		{
			name: "a delimiter with nothing after it",
			src:  "cat <<END\nbody\nEND",
			want: "cat << END\nbody\nEND\n",
			why:  "the input ran out after the delimiter rather than inside the body, so the body is unaffected",
		},
		{
			name: "a body that ran to the end of the input",
			src:  "cat <<END\nbody\n",
			want: "cat << END\nbody\nEND\n",
			why:  "no delimiter ever arrived, but the last line ended, so the body still ends where a line does",
		},
		{
			name: "a body whose last line never ended",
			src:  "cat <<END\nbody",
			want: "cat << END\nbody",
			why:  "the whole of the defect. The delimiter written onto that line made the body `bodyEND`; a newline and then the delimiter would make it `body\\n`, which is a different body. The document ran to the end of the input, so it is written back open",
		},
		{
			name: "the tab-stripping operator with the same body",
			src:  "cat <<-END\n\tbody",
			want: "cat <<- END\nbody",
			why:  "`<<-` reads the body by a different rule and writes it by the same one, so it shared the defect and shares the fix",
		},
		{
			name: "a quoted delimiter and a body whose last line never ended",
			src:  "cat <<'END'\nbody",
			want: "cat << 'END'\nbody",
			why:  "quoting the operator's word says whether the body expands and changes nothing about where the body ends",
		},
		{
			name: "no body at all",
			src:  "cat <<END",
			want: "cat << END\nEND\n",
			why:  "an empty body is already at the start of a line, so nothing is owed and no blank line appears",
		},
		{
			name: "two bodies on one command",
			src:  "cat <<A <<B\na\nA\nb\nB\n",
			want: "cat << A << B\na\nA\nb\nB\n",
			why:  "the second body follows a delimiter line that already ended, so a newline written before it would open B's body with a blank line — and `cat <<A <<B` reads B",
		},
		{
			name: "two bodies where the second never ended its line",
			src:  "cat <<A <<B\na\nA\nb",
			want: "cat << A << B\na\nA\nb",
			why:  "both halves of the question in one command",
		},
		{
			name: "three bodies on one command",
			src:  "cat <<A <<B <<C\na\nA\nb\nB\nc\nC\n",
			want: "cat << A << B << C\na\nA\nb\nB\nc\nC\n",
			why:  "the rule is per body rather than about the first one, so a third gains no blank line either",
		},
		{
			name: "the first body ran to the end and the second got nothing",
			src:  "cat <<A <<B\nbody",
			want: "cat << A << B\nbody",
			why:  "A took everything and B has an empty body, which is what this parses back to — writing B's delimiter after it would hand A `body\\nB\\n` instead",
		},
		{
			name: "a body and the rest of its line",
			src:  "cat <<END; echo after\nbody\nEND\n",
			want: "cat << END\nbody\nEND\necho after",
			why:  "the body ended the line, so what followed the semicolon follows the body — and takes no separator, since there is nothing in front of it on the line to separate from",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(c.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			got := syntax.Print(f)
			if got != c.want {
				t.Errorf("Print(%q) =\n  %q\nwant\n  %q\n%s", c.src, got, c.want, c.why)
			}
			// And the promise itself, which the bytes above are one
			// spelling of: the bodies that come back are the bodies that
			// went in. Every defect here produced text that parsed, so
			// this is what a re-parse has to be asked rather than whether
			// it succeeded.
			if want, gotBodies := heredocBodies(t, c.src), heredocBodies(t, got); !equalStrings(want, gotBodies) {
				t.Errorf("Print(%q) re-reads bodies %q, want %q\n%s", c.src, gotBodies, want, c.why)
			}
		})
	}
}

// The same question where a caller asked for an arrangement.
//
// A separator, a keyword terminator and the newline that opens the next line
// are all written on the assumption that something is in front of them on the
// line. A here-document body ends the line, so after one there is not — and a
// separator alone on a line of its own is not a statement terminator anywhere:
// it is a syntax error. This is the route `type f` and `export -f` take, so a
// function with a here-document in it came back as text no shell would read.
func TestALayoutWritesNoSeparatorAfterAHereDocumentBody(t *testing.T) {
	// Four spaces, statements on lines of their own, `;` between them and
	// after the last of a body a keyword closes. Written out rather than
	// taken from a dialect: what is under test is the arrangement, and which
	// shell arranges things this way is not this package's business.
	layout := syntax.Layout{
		Indent:                     "    ",
		Nested:                     true,
		Lines:                      true,
		Separator:                  ";",
		KeywordTerminator:          ";",
		DoAfterWordsOnItsOwnLine:   true,
		BraceOpenSuffix:            " ",
		OutermostBraceOpensALine:   true,
		DoAfterCommandOnItsOwnLine: false,
	}
	for _, c := range []struct {
		name string
		src  string
		want string
		why  string
	}{
		{
			name: "a statement follows one",
			src:  "f() {\ncat <<END\nbody\nEND\necho after\n}\n",
			want: "f() { \n    cat << END\nbody\nEND\n\n    echo after\n}",
			why:  "the separator between two statements has nothing before it once the body has ended the line",
		},
		{
			name: "a keyword closes the body it is the last of",
			src:  "f() {\nif true; then\ncat <<END\nbody\nEND\nfi\n}\n",
			want: "f() { \n    if true; then\n        cat << END\nbody\nEND\n\n    fi\n}",
			why:  "the terminator a keyword-closed body ends with has nothing before it either",
		},
		{
			name: "a loop body closed by a keyword",
			src:  "f() {\nfor i in a; do\ncat <<END\nbody\nEND\ndone\n}\n",
			want: "f() { \n    for i in a;\n    do\n        cat << END\nbody\nEND\n\n    done\n}",
			why:  "the same terminator, reached through the other keyword-closed body",
		},
		{
			name: "the keyword that opens a body",
			src:  "f() {\nif cat <<END\nbody\nEND\nthen echo y\nfi\n}\n",
			want: "f() { \n    if cat << END\nbody\nEND\n    then\n        echo y;\n    fi\n}",
			why:  "the header owed the body, so `then` opens the line the body ended rather than following a separator on it",
		},
		{
			name: "nothing owed a body",
			src:  "f() {\necho a\necho b\n}\n",
			want: "f() { \n    echo a;\n    echo b\n}",
			why:  "the control: where no body ended the line the separator is written exactly as before",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			f, err := syntax.Parse(c.src, syntax.Core())
			if err != nil {
				t.Fatalf("parse %q: %v", c.src, err)
			}
			decl := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0]
			got := syntax.PrintWith(decl, layout)
			if got != c.want {
				t.Errorf("PrintWith(%q) =\n%q\nwant\n%q\n%s", c.src, got, c.want, c.why)
			}
			// And it has to be readable, which is the part a shell cares
			// about: three of these four came back with a `;` on a line of
			// its own, which bash, dash and ksh93 all refuse.
			if _, err := syntax.Parse(got, syntax.Core()); err != nil {
				t.Errorf("the arrangement does not parse: %v\n%s", err, got)
			}
		})
	}
}

// heredocBodies is every here-document body in a source, in the order the
// operators were written.
func heredocBodies(t *testing.T, src string) []string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out []string
	for _, st := range f.Stmts {
		for _, cmd := range st.Expr.(*syntax.Pipeline).Cmds {
			sc, ok := cmd.(*syntax.SimpleCmd)
			if !ok {
				continue
			}
			for _, rd := range sc.Redirs {
				if rd.Op.IsHeredoc() {
					out = append(out, rd.Heredoc.Literal())
				}
			}
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// A here-document whose body is text of an alias value.
//
// Substitution is textual in every column: the value stands in the input where
// the alias word stood, and a body is read from the lines after the operator's
// — which, for a value holding newlines, are the value's own, then the text of
// any value it was spliced into, then the input. The rows are the shapes
// measured against bash 5.3.20, ksh93u+ and dash on 2026-09-16, which all three
// answer alike; the program each one reads is compared as the printer writes it.
func TestAHereDocumentBodyIsReadFromTheAliasText(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.AliasBodyBackslashJoinsTheNextLine = true
	for _, c := range []struct {
		name  string
		table syntax.Aliases
		src   string
		want  string
	}{
		{
			name:  "the value holds the whole body, delimiter last",
			table: table("hd", "cat <<EOF\nhello\nEOF"),
			src:   "hd\necho after\n",
			want:  "cat <<EOF\nhello\nEOF\necho after",
		},
		{
			name:  "the value holds the whole body and a command after it",
			table: table("hd", "cat <<EOF\nhello\nEOF\necho two"),
			src:   "hd\necho after\n",
			want:  "cat <<EOF\nhello\nEOF\necho two\necho after",
		},
		{
			name:  "two documents on the operator's line",
			table: table("hd", "cat <<A <<B\na\nA\nb\nB\n"),
			src:   "hd\necho after\n",
			want:  "cat <<A <<B\na\nA\nb\nB\necho after",
		},
		{
			name:  "the body goes on into the input",
			table: table("hd", "cat <<EOF\nin alias\n"),
			src:   "hd\nfrom file\nEOF\necho after\n",
			want:  "cat <<EOF\nin alias\n\nfrom file\nEOF\necho after",
		},
		{
			name:  "the rest of the alias word's line is a body line",
			table: table("hd", "cat <<EOF\nin alias\nEOF"),
			src:   "hd; echo same\necho after\nEOF\necho end\n",
			want:  "cat <<EOF\nin alias\nEOF; echo same\necho after\nEOF\necho end",
		},
		{
			name:  "the operator in one value, the body in the value around it",
			table: table("Y", `cat <<\END`, "X", "Y\ntext\nEND\necho inX"),
			src:   "X\necho after\n",
			want:  "cat <<\\END\ntext\nEND\necho inX\necho after",
		},
		{
			name:  "a body split between the two values and the input",
			table: table("Y", "cat <<END\nin Y", "X", "Y\nin X"),
			src:   "X\nin file\nEND\necho after\n",
			want:  "cat <<END\nin Y\nin X\nin file\nEND\necho after",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := parsedWith(t, d, c.table, c.src); got != c.want {
				t.Errorf("%q read as\n%s\nwant\n%s", c.src, got, c.want)
			}
		})
	}
}

// The controls, which read the same before and after.
func TestAliasHeredocControls(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.AliasBodyBackslashJoinsTheNextLine = true
	for _, c := range []struct {
		name  string
		table syntax.Aliases
		src   string
		want  string
	}{
		{
			name:  "the operator alone in the value reads the body from the input",
			table: table("hd", "cat <<EOF"),
			src:   "hd\nhello\nEOF\necho after\n",
			want:  "cat <<EOF\nhello\nEOF\necho after",
		},
		{
			name:  "no here-document at all",
			table: table("two", "echo a\necho b"),
			src:   "two\necho after\n",
			want:  "echo a\necho b\necho after",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := parsedWith(t, d, c.table, c.src); got != c.want {
				t.Errorf("%q read as\n%s\nwant\n%s", c.src, got, c.want)
			}
		})
	}
}

// Where the seam between a value and the input reads as a blank — zsh 5.9.2 —
// a body line the value ends in the middle of gains one, unless the input
// already has one there.
func TestTheSeamIsABlankWhereABackslashDoesNotJoin(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.AliasBodyBackslashJoinsTheNextLine = false
	for _, c := range []struct {
		name, value, src, want string
	}{
		{"a line the value ends mid-way", "cat <<END\nin Y", "hd\nEND\n", "cat <<END\nin Y \nEND"},
		{"a blank already there", "cat <<END\nin Y", "hd x\nEND\n", "cat <<END\nin Y x\nEND"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := parsedWith(t, d, table("hd", c.value), c.src); got != c.want {
				t.Errorf("%q read as\n%s\nwant\n%s", c.src, got, c.want)
			}
		})
	}
}

// A body that runs out at the end of the input, which is the shape the value
// route used to hand back: the delimiter written as the value's last line is
// followed on that line by the rest of the alias word's line, so it is body,
// and nothing below it ever closes the document.
//
// Measured 2026-09-19 from script files, `env -i PATH=/usr/bin:/bin LC_ALL=C`,
// standard input on the null device, with `shopt -s expand_aliases` where bash
// needs it: bash 5.3.20, ksh93u+ 2012-08-01, dash 0.5.12 and BusyBox ash
// 1.37.0 all run `cat` with the three body lines `in alias`, `EOF; echo same`
// and `echo after`, and zsh 5.9.2 does the same with its seam blank. Only bash
// says anything on the way past, and what it says is the remark below.
func TestAHereDocumentBodyRunsOutAtTheEndOfTheInput(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.AliasBodyBackslashJoinsTheNextLine = true
	const want = "cat <<EOF\nin alias\nEOF; echo same\necho after\nEOF"
	for _, counts := range []bool{false, true} {
		d.AliasBodyCountsLines = counts
		got := parsedWith(t, d, table("hd", "cat <<EOF\nin alias\nEOF"),
			"hd; echo same\necho after\n")
		if got != want {
			t.Errorf("counting %v: read as\n%s\nwant\n%s", counts, got, want)
		}
	}
}

// And the remark the probe made about it is re-sited onto the input, because
// the two lines it carries are lines of the input and the text it was read
// from was not.
//
// `At` is where the document began, which is where the alias word stands: the
// operator is text of the value and has no line of its own. `Pos` is where the
// input ran out, and it moves with Dialect.AliasBodyCountsLines exactly as
// every other position in a substituted body does — the value's two newlines
// are two lines of the input where that axis is on and none where it is off.
func TestTheRemarkForABodyThatRanOutIsSitedOnTheInput(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.AliasBodyBackslashJoinsTheNextLine = true
	for _, c := range []struct {
		counts  bool
		wantPos int32
	}{
		{false, 2},
		{true, 4},
	} {
		d.AliasBodyCountsLines = c.counts
		rs := remarksWith(t, d, table("hd", "cat <<EOF\nin alias\nEOF"),
			"hd; echo same\necho after\n")
		if len(rs) != 1 {
			t.Fatalf("counting %v: got %d remarks, want 1: %+v", c.counts, len(rs), rs)
		}
		if rs[0].Kind != syntax.RemarkHeredocAtEOF {
			t.Errorf("counting %v: kind = %v, want RemarkHeredocAtEOF", c.counts, rs[0].Kind)
		}
		if rs[0].At.Line != 1 {
			t.Errorf("counting %v: At.Line = %d, want the alias word's line", c.counts, rs[0].At.Line)
		}
		if rs[0].Pos.Line != c.wantPos {
			t.Errorf("counting %v: Pos.Line = %d, want %d", c.counts, rs[0].Pos.Line, c.wantPos)
		}
		if rs[0].Token != "EOF" {
			t.Errorf("counting %v: Token = %q, want the delimiter that never arrived", c.counts, rs[0].Token)
		}
	}
}

// The control, and it is the row that says the remark is about the input
// running out rather than about a body read from a value: the same value with
// a later `EOF` in the input closes the document and nothing is remarked on.
func TestABodyFromAValueThatIsClosedIsNotRemarkedOn(t *testing.T) {
	t.Parallel()
	d := syntax.Core()
	d.AliasBodyBackslashJoinsTheNextLine = true
	for _, counts := range []bool{false, true} {
		d.AliasBodyCountsLines = counts
		if rs := remarksWith(t, d, table("hd", "cat <<EOF\nin alias\nEOF"),
			"hd; echo same\necho after\nEOF\necho end\n"); len(rs) != 0 {
			t.Errorf("counting %v: got %d remarks, want none: %+v", counts, len(rs), rs)
		}
	}
}

// remarksWith parses as parsedWith does and hands back what the parser had to
// say rather than what it read.
func remarksWith(t *testing.T, d syntax.Dialect, a syntax.Aliases, src string) []syntax.Remark {
	t.Helper()
	p := syntax.NewParser(src, d)
	p.Aliases = a
	p.Parse()
	if err := p.Err(); err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	return p.Remarks()
}

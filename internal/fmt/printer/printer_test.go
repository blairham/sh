// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package printer_test

import (
	"sort"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"

	"github.com/blairham/sh/internal/fmt/comments"
	"github.com/blairham/sh/internal/fmt/printer"
)

// Every sample is held to the three promises of docs/design.md: the output
// parses to the same tree (checked through the substrate's canonical
// printer, which erases layout and keeps meaning), formatting is idempotent,
// and no comment is lost.
var samples = []struct {
	name string
	src  string
	zsh  bool
	// loose skips the stderr comparison in the behavior tier: a `time`
	// report's numbers differ between two runs of the same script.
	loose bool
}{
	{name: "spacing", src: "echo    a     b\n"},
	{name: "words-verbatim", src: "echo \"a  b\" 'c  d' $x ${y} $(cat f) `echo sub`\n"},
	{name: "runs", src: "a; b; c\nd\n"},
	{name: "background", src: "sleep 1 &\nwait\n"},
	{name: "pipeline", src: "cat f | grep x | wc -l\n"},
	{name: "pipeline-broken", src: "cat f |\n  grep x |\n  wc -l\n"},
	{name: "andor", src: "true && echo yes || echo no\n"},
	{name: "andor-broken", src: "true &&\n  echo yes\n"},
	{name: "negate", src: "! grep -q x f\n"},
	{name: "redirects", src: "echo hi >out 2>&1\ncmd <in >>log\n"},
	{name: "redirect-order", src: ">first echo one >second two\n"},
	{name: "heredoc", src: "cat <<EOF\nkeep  spacing   # not a comment\nEOF\n"},
	{name: "heredoc-quoted", src: "cat <<'EOF'\n$x stays\nEOF\n"},
	{name: "heredoc-dash", src: "cat <<-EOF\n\tbody\nEOF\n"},
	{name: "heredoc-pipe", src: "cat <<EOF |\nbody\nEOF\ngrep b\n"},
	{name: "if", src: "if true; then\n  echo a\nfi\n"},
	{name: "if-elif-else", src: "if a; then\n  b\nelif c; then\n  d\nelse\n  e\nfi\n"},
	{name: "if-oneline", src: "if true; then echo a; fi\n"},
	{name: "while", src: "while read -r l; do\n  echo \"$l\"\ndone <f\n"},
	{name: "until-oneline", src: "until test -e .; do sleep 1; done\n"},
	{name: "for", src: "for i in 1 2 3; do\n  echo \"$i\"\ndone\n"},
	{name: "for-no-items", src: "for a\ndo\n  echo \"$a\"\ndone\n"},
	{name: "case", src: "case $1 in\na) echo one ;;\nb | c)\n  echo two\n  ;;\n*) : ;;\nesac\n"},
	{name: "func", src: "greet() {\n  echo hi\n}\n"},
	{name: "func-keyword", src: "function greet {\n  echo hi\n}\n"},
	// `cd /` and `pwd`, not `cd /tmp` and `ls`. The tier runs a sample twice
	// and compares, so a sample that reads shared mutable state compares two
	// different worlds: listing /tmp failed whenever anything else on the
	// machine wrote a file between the two runs. `/` is a fixed string and
	// the construct under test — a subshell holding an and-or with a cd — is
	// exercised exactly as before. TestEverySampleIsDeterministic is the
	// guard that keeps the next one out.
	{name: "subshell", src: "(cd / && pwd)\n"},
	{name: "group", src: "{ echo a; echo b; } >log\n"},
	{name: "group-multiline", src: "{\n  echo a\n  echo b\n} >log\n"},
	{name: "assign", src: "x=1 y='a b' cmd\nPATH=/bin:$PATH\n"},
	{name: "array", src: "a=(one two 'three four')\necho \"${a[1]}\"\n"},
	{name: "test-clause", src: "[[ -n $x && $x != y ]] && echo set\n"},
	{name: "arith-cmd", src: "((x = x + 1))\n"},
	{name: "arith-for", src: "for ((i = 0; i < 3; i++)); do\n  echo \"$i\"\ndone\n"},
	{name: "comment-own-line", src: "# header\necho a\n"},
	{name: "comment-trailing", src: "echo a # trailing\n"},
	{name: "comment-indented", src: "if true; then\n  # inside\n  echo a\nfi\n"},
	{name: "comment-runs", src: "# one\n# two\n\n# three\necho a\n"},
	{name: "comment-hash-literal", src: "echo a#b '#lit' \"x # y\"\n"},
	{name: "blank-lines", src: "echo a\n\n\n\necho b\n"},
	{name: "mixed", src: "#!/bin/sh\n# what it does\nset -eu\n\nmain() {\n  local x=1\n  if [ \"$x\" = 1 ]; then\n    echo one # yes\n  fi\n}\n\nmain \"$@\"\n"},
	{name: "zsh-repeat", src: "repeat 3; do\n  echo hi\ndone\n", zsh: true},
	{name: "zsh-anon", src: "() {\n  echo hi\n}\n", zsh: true},
	{name: "zsh-try", src: "{\n  echo t\n} always {\n  echo a\n}\n", zsh: true},
	{name: "coproc", src: "coproc cat\n"},
	// Regressions from the wild sweep, one sample per found bug.
	{name: "procsub-redirect", src: "while read -r l; do\n\techo \"$l\"\ndone < <(printf '%s\\n' a b)\n"},
	{name: "subshell-arith-fuse", src: "if ( ((x)) || y ) && z; then\n\techo ok\nfi\n"},
	{name: "cond-list-newlines", src: "if ((x > 2)) # trailing\n\t# own line\n\t[[ $a != b ]]; then\n\techo ok\nfi\n"},
	{name: "run-after-continuation", src: "{ a ||\n\tb\n\tc; } 2>/dev/null | cat\n"},
	{name: "case-oneline", src: "if { case $x in a) true ;; *) false ;; esac; }; then\n\techo ok\nfi\n"},
	{name: "heredoc-unterminated", src: "cat <<'EOF'\nbody\n\n\n", zsh: true},
	{name: "anon-func-args", src: "() {\n\tprint -r -- \"$1\"\n} \"$arg\" 2>/dev/null\n", zsh: true},
	{name: "select", src: "select x in a b; do\n  echo \"$x\"\ndone\n"},
	{name: "time", src: "time sleep 0\n", loose: true},
}

func dialect(useZsh bool) syntax.Dialect {
	if useZsh {
		return zsh.Dialect()
	}
	return bash.Dialect()
}

// style is the layout that goes with the dialect. The two travel together —
// a sample read as zsh is laid out as zsh — which is the point of the fourth
// vector, and the reason every helper here takes both.
func style(useZsh bool) syntax.Style {
	if useZsh {
		return zsh.Style()
	}
	return bash.Style()
}

// formatStyle is format under a layout the dialect did not choose, for the
// tests that have to show a field changes something.
func formatStyle(t *testing.T, src string, d syntax.Dialect, st syntax.Style) string {
	t.Helper()
	return format(t, src, d, st)
}

func format(t *testing.T, src string, d syntax.Dialect, st syntax.Style) string {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("input does not parse: %v", err)
	}
	return printer.Format(src, f, comments.Recover(src, f), st)
}

func TestFormattingKeepsTheTree(t *testing.T) {
	for _, s := range samples {
		t.Run(s.name, func(t *testing.T) {
			d, st := dialect(s.zsh), style(s.zsh)
			out := format(t, s.src, d, st)
			fIn, err := syntax.Parse(s.src, d)
			if err != nil {
				t.Fatal(err)
			}
			fOut, err := syntax.Parse(out, d)
			if err != nil {
				t.Fatalf("output does not parse: %v\noutput:\n%s", err, out)
			}
			// SameProgram rather than comparing canonical prints: printing
			// both through the canonicalizer erases exactly the differences
			// a formatter is most likely to introduce, so two scripts that
			// differ can print alike. #1401 asked for this comparison by
			// name, and it is now the substrate's rather than a test's.
			if why, ok := syntax.SameProgram(fIn, fOut); !ok {
				t.Errorf("the program changed at %s\ninput:\n%s\noutput:\n%s", why, s.src, out)
			}
		})
	}
}

func TestFormattingIsIdempotent(t *testing.T) {
	for _, s := range samples {
		t.Run(s.name, func(t *testing.T) {
			d, st := dialect(s.zsh), style(s.zsh)
			once := format(t, s.src, d, st)
			twice := format(t, once, d, st)
			if once != twice {
				t.Errorf("not a fixed point\nonce:\n%s\ntwice:\n%s", once, twice)
			}
		})
	}
}

func TestFormattingKeepsEveryComment(t *testing.T) {
	for _, s := range samples {
		t.Run(s.name, func(t *testing.T) {
			d, st := dialect(s.zsh), style(s.zsh)
			out := format(t, s.src, d, st)
			if got, want := commentTexts(t, out, d), commentTexts(t, s.src, d); !equal(got, want) {
				t.Errorf("comments changed\nbefore: %q\nafter:  %q\noutput:\n%s", want, got, out)
			}
		})
	}
}

func commentTexts(t *testing.T, src string, d syntax.Dialect) []string {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, c := range comments.Recover(src, f) {
		texts = append(texts, c.Text)
	}
	sort.Strings(texts)
	return texts
}

func equal(a, b []string) bool {
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

// A few exact outputs, to pin the style itself rather than only the
// properties. Kept small on purpose: every golden line is a style decision.
func TestStyle(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			name: "spacing collapses",
			src:  "echo    a     b\n",
			want: "echo a b\n",
		},
		{
			name: "indent is two spaces",
			src:  "if true; then\n\techo a\nfi\n",
			want: "if true; then\n  echo a\nfi\n",
		},
		{
			name: "trailing comment keeps one space",
			src:  "echo a    # note\n",
			want: "echo a # note\n",
		},
		{
			name: "blank runs cap at one",
			src:  "echo a\n\n\n\necho b\n",
			want: "echo a\n\necho b\n",
		},
		{
			name: "redirect attaches",
			src:  "echo hi > out\n",
			want: "echo hi >out\n",
		},
		{
			name: "same-line run survives",
			src:  "a;    b;c\n",
			want: "a; b; c\n",
		},
		{
			name: "no blank under an opener",
			src:  "main() {\n\techo hi\n}\n",
			want: "main() {\n  echo hi\n}\n",
		},
		{
			name: "blank inside a body survives",
			src:  "main() {\n  echo a\n\n  echo b\n}\n",
			want: "main() {\n  echo a\n\n  echo b\n}\n",
		},
		{
			name: "comment runs align",
			src:  "short=1 # one\nmuch_longer=2 # two\n",
			want: "short=1       # one\nmuch_longer=2 # two\n",
		},
		{
			name: "continuations keep their lines",
			src:  "cmd alpha \\\n  beta \\\n  gamma\n",
			want: "cmd alpha \\\n  beta \\\n  gamma\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := format(t, c.src, bash.Dialect(), bash.Style()); got != c.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, c.want)
			}
		})
	}
}

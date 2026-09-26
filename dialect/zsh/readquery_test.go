// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// `read -q`, this shell's letter for reading one key and answering whether it
// was a yes — the `read -q "?Delete? " && rm ...` confirmation.
//
// What it *does* is asserted in interp, where the shape is graded against a
// synthetic vector. What is here is this package's own: the pair of tables
// that decide whether the letter exists at all, and the answers the **shipped**
// preset gives, because a corpus case cannot tell which of the two tables is
// wrong and a synthetic vector cannot tell whether this shell ships the letter.
//
// Measured 2026-09-26 against zsh 5.9.2 at /opt/homebrew/bin/zsh — `go version
// -m` says *not a Go executable* for it — run `-f`, with `REPLY` seeded ahead
// of every row so that a name left alone cannot be read as a name filled.

// The whole of the answer, over the keys that decide it and the two ends of
// the input.
//
// `-u0` rather than a terminal, which is how zsh's own `B04read.ztst` asks
// this question and the only route that needs no pseudo-terminal: the letter
// reads the terminal by default, and a here-string on standard input does not
// redirect it.
func TestReadQueryAnswersYesOrNo(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a small yes", `read -q -u0 <<<y`, "st=0 [y]"},
		{"a capital yes is normalized", `read -q -u0 <<<Y`, "st=0 [y]"},
		{"a small no", `read -q -u0 <<<n`, "st=1 [n]"},
		{"a capital no is normalized", `read -q -u0 <<<N`, "st=1 [n]"},
		{"any other key is a no", `read -q -u0 <<<X`, "st=1 [n]"},
		{"a bare newline is a no", `read -q -u0 <<<''`, "st=1 [n]"},
		{"only the first key is read", `read -q -u0 <<<yes`, "st=0 [y]"},
		{"and it is the first", `read -q -u0 <<<ny`, "st=1 [n]"},
		{"end of input is neither", `read -q -u0 </dev/null`, "st=2 [n]"},
		{"a named parameter takes the answer", `read -q -u0 ans <<<Y; REPLY=$ans`, "st=0 [y]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), `REPLY=seed
`+c.src+`
print -r -- "st=$? [$REPLY]"
`)
			if strings.Contains(out, "not implemented yet") {
				t.Fatalf("got %q, want the letter answered rather than named as missing", out)
			}
			if strings.Contains(out, "bad option") {
				t.Fatalf("got %q, want the letter known", out)
			}
			if want := c.want + "\n"; out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
}

// A named parameter takes the answer and `REPLY` is left as it was, which is
// the row the table above cannot carry: it reassigns `REPLY` from the name to
// print it, so it would read the same whether or not the builtin had also
// written `REPLY` itself.
func TestReadQueryWithANameLeavesTheDefaultAlone(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `REPLY=seed; ans=unset
read -q -u0 ans <<<Y
print -r -- "st=$? [$ans][$REPLY]"
`)
	if want := "st=0 [y][seed]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// The letter reads **the terminal**, so a script with none is the measured
// refusal at 1 with nothing read and nothing assigned — not the `n` a read
// that reached end of input would leave.
//
// The same sentence `read -k` makes, and carrying no location and no builtin
// name for the same measured reason.
func TestReadQueryWithNoTerminalSaysSo(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `REPLY=seed
read -q <<<y
print -r -- "st=$? [$REPLY]"
`)
	if want := "not interactive and can't open terminal\nst=1 [seed]\n"; out != want || st != 0 {
		t.Errorf("got %q status %d, want %q", out, st, want)
	}
}

// `-q` and `-k` compose, and what the pair says is that the verdict is on the
// **text** the read collected rather than on its first character.
//
// Both rows hold the first character at `y` and move only how much is read.
// With no count the read is one character and the two readings agree on every
// row above, so this is the only place the noun is pinned.
func TestReadQueryJudgesTheTextItRead(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"one character of yy", `read -q -k1 -u0 <<<yy`, "st=0 [y]"},
		{"two characters of yy", `read -q -k2 -u0 <<<yy`, "st=1 [n]"},
		{"more than there are", `read -q -k3 -u0 <<<y`, "st=2 [n]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), `REPLY=seed
`+c.src+`
print -r -- "st=$? [$REPLY]"
`)
			if want := c.want + "\n"; out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
}

// No letter is in both of `read`'s tables at once.
//
// The comments beside those tables have said for two letters now that they
// "move together", and nothing graded it. They cannot: a letter in
// `ReadOptions` is accepted by the option reader and never reaches
// `UnimplementedOptionLetters` at all — see interp.Runner.refuseOption, which
// is called only for a letter the known set does not hold. So a stale entry
// left behind is **inert**, and a mutation run is what said so: putting `q`
// back into the missing-letters string while it stayed in `ReadOptions` broke
// nothing and no test noticed.
//
// Inert is not harmless. It is a sentence in the shipped data saying this
// shell has not built something it has, and it comes alive the day the letter
// leaves the accepted set for any reason. Asserting on the two strings
// directly is the only place the claim can be made, because the behavior they
// are supposed to agree about is exactly what one of them cannot reach.
func TestReadsTwoLetterTablesDoNotOverlap(t *testing.T) {
	accepted := zsh.Semantics().ReadOptions
	missing := zsh.Diagnostics().UnimplementedOptionLetters["read"]
	if accepted == "" || missing == "" {
		t.Fatalf("accepted=%q missing=%q, want both tables to have letters in them",
			accepted, missing)
	}
	for _, letter := range missing {
		// The accepted set is an optstring, so `:` and `#` in it are shapes
		// rather than letters — neither is a letter any table names.
		if strings.ContainsRune(accepted, letter) {
			t.Errorf("-%c is in ReadOptions %q and in the missing letters %q: "+
				"accepted wins and the second entry is a refusal that can never be made",
				letter, accepted, missing)
		}
	}
}

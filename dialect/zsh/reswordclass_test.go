// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The class this shell gives a name is its `$reswords` table and not its
// parser's (#3291).
//
// `type export` was `export is a shell builtin` here and is `export is a
// reserved word` in zsh 5.9.2 — and not for one name: all seven declaration
// commands answer the same way, four words the rest of the panel has no
// construct for were `none`, and two words the POSIX grammar reserves were
// `reserved` where this shell says they are not.
//
// Measured 2026-09-16 against zsh 5.9.2 under `-f`, `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, by two instruments that share no code:
//
//	zmodload zsh/parameter; print -rl -- $reswords     thirty-one words
//	whence -w NAME                                     `reserved` for those
//	                                                   thirty-one and no other
//
// Twelve names moved. Before, and after, with the shell's answer beside it:
//
//	name         before          zsh 5.9.2 and now
//	export       builtin         reserved
//	readonly     builtin         reserved
//	local        builtin         reserved
//	typeset      builtin         reserved
//	declare      builtin         reserved
//	integer      builtin         reserved
//	float        none            reserved
//	nocorrect    none            reserved
//	foreach      none            reserved
//	end          none            reserved
//	repeat       none            reserved
//	in           reserved        none
//	]]           reserved        none
//
// `cd` and `echo` are the controls and are `builtin` throughout, so a shell
// that had simply started calling everything reserved fails here.

// TestTheClassIsTheReservedWordTable walks the very table `$reswords` prints
// and asks the shell to class each word, which is the pairing that had come
// apart: one list was printed and another was consulted.
func TestTheClassIsTheReservedWordTable(t *testing.T) {
	dir := t.TempDir()
	for _, word := range []string{
		"if", "export", "declare", "function", "else", "float", "end", "do",
		"typeset", "then", "integer", "{", "select", "readonly", "coproc", "}",
		"!", "case", "[[", "repeat", "done", "for", "while", "time", "esac",
		"until", "local", "fi", "nocorrect", "foreach", "elif",
	} {
		out, st := runZsh(t, dir, "whence -w -- "+singleQuote(word))
		if want := word + ": reserved\n"; out != want || st != 0 {
			t.Errorf("whence -w %s = %q (status %d), want %q at 0", word, out, st, want)
		}
	}
	// And the words that are *not* in it, which is the half a table read as
	// a union would get wrong. `in` and `]]` are the POSIX grammar's and
	// this shell does not reserve either; `cd` and `echo` are the controls
	// that say the answer is still a classification and not a constant.
	for _, c := range []struct{ word, want string }{
		{"in", "in: none\n"},
		{"]]", "]]: none\n"},
		{"((", "((: none\n"},
		{"cd", "cd: builtin\n"},
		{"echo", "echo: builtin\n"},
	} {
		out, _ := runZsh(t, dir, "whence -w -- "+singleQuote(c.word))
		if out != c.want {
			t.Errorf("whence -w %s = %q, want %q", c.word, out, c.want)
		}
	}
}

// TestTheDeclarationCommandsAreReservedWordsInEveryForm pins the four
// spellings of the same question together, because each had its own road to
// the answer and a fix to one would have left the others behind.
//
// Measured 2026-09-16 on zsh 5.9.2 under `-f`, with a function of that name
// defined for the listing, which is what puts the resolutions in an order
// rather than a pair.
func TestTheDeclarationCommandsAreReservedWordsInEveryForm(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"export", "readonly", "local", "typeset", "declare", "integer", "float"} {
		// The plain answer names the first resolution, which is the word.
		out, st := runZsh(t, dir, "type "+name)
		if want := name + " is a reserved word\n"; out != want || st != 0 {
			t.Errorf("type %s = %q (status %d), want %q at 0", name, out, st, want)
		}
		// `whence -v` is the same sentence by another road.
		if out, _ := runZsh(t, dir, "whence -v "+name); out != name+" is a reserved word\n" {
			t.Errorf("whence -v %s = %q, want the reserved word", name, out)
		}
		// The listing has both, the word first and the builtin after it.
		out, _ = runZsh(t, dir, "type -a "+name)
		if want := name + " is a reserved word\n" + name + " is a shell builtin\n"; out != want {
			t.Errorf("type -a %s = %q, want %q", name, out, want)
		}
		// And `whence -a` is that listing in the bare form: a line each.
		out, _ = runZsh(t, dir, "whence -a "+name)
		if want := name + "\n" + name + "\n"; out != want {
			t.Errorf("whence -a %s = %q, want %q", name, out, want)
		}
		// `command -v` finds it, which it did not for `float`.
		out, st = runZsh(t, dir, "command -v "+name)
		if want := name + "\n"; out != want || st != 0 {
			t.Errorf("command -v %s = %q (status %d), want %q at 0", name, out, st, want)
		}
	}
	// The control that says the order is the resolution's and not a
	// preference: `cd` is a builtin alone, and its file follows it.
	out, _ := runZsh(t, dir, "type -a cd")
	if !strings.HasPrefix(out, "cd is a shell builtin\n") {
		t.Errorf("type -a cd began %q, want the builtin line first", out)
	}
}

// TestFloatIsADeclarationHere is the second, plainer half of #3291: the word
// resolved to nothing at all, in a shell whose own `$reswords` names it.
//
// Measured 2026-09-16 on zsh 5.9.2 under `-f`. The default attribute is `E`
// and not `F` — `float c=1.5; typeset -p c` is `typeset -E
// c=1.500000000e+00`, byte for byte what `typeset -E c=1.5` lists — and a
// written letter wins over the name's, which is the discriminating case: a
// `float` that always added `E` would answer `typeset -E d=1.50e+00` for the
// line below and look right on every other row.
func TestFloatIsADeclarationHere(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ src, want string }{
		{"float c=1.5; typeset -p c", "typeset -E c=1.500000000e+00\n"},
		{"float -F 2 d=1.5; typeset -p d", "typeset -F d=1.50\n"},
		{"float e; e=3.75; typeset -p e", "typeset -E e=3.750000000e+00\n"},
		{"float -x g=2.5; typeset -p g", "export -E g=2.500000000e+00\n"},
	} {
		out, st := runZsh(t, dir, c.src)
		if out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
	// The letters it does *not* take, measured a letter at a time against
	// `float -X zz=1.5`: this shell refuses these eight and takes the rest.
	for _, letter := range []string{"a", "A", "G", "i", "m", "T", "U", "z"} {
		out, st := runZsh(t, dir, "float -"+letter+" zz=1.5")
		if !strings.Contains(out, "bad option: -"+letter) || st == 0 {
			t.Errorf("float -%s = %q (status %d), want a bad option", letter, out, st)
		}
	}
	// And the operand is a declaration's, so the value is not globbed. The
	// same probe `integer` has, and for the reason written there: this shell
	// splits no unquoted parameter, so only the glob separates a declaring
	// word from an ordinary one.
	out, st := runZsh(t, dir, `float n=*; echo "n=[$n]"`)
	if strings.Contains(out, "no matches found") || st == 0 {
		t.Errorf("float n=* = %q (status %d); the operand was globbed, so the word is not declaring", out, st)
	}
}

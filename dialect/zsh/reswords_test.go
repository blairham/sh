// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// zshReswordsListing is `print -rl -- $reswords` in zsh 5.9.2, one word per
// line and in the order it wrote them.
//
// Spelled out here rather than built from zshReservedWords, for the reason
// emptyModuleParams gives: a test that asked the implementation what it
// thought the reserved words were would agree with it whatever it said. This
// is the measurement, and the package's list is what is being graded against
// it — so a word added to one and not the other fails here.
const zshReswordsListing = "if\nexport\ndeclare\nfunction\nelse\nfloat\nend\n" +
	"do\ntypeset\nthen\ninteger\n{\nselect\nreadonly\ncoproc\n}\n!\ncase\n" +
	"[[\nrepeat\ndone\nfor\nwhile\ntime\nesac\nuntil\nlocal\nfi\nnocorrect\n" +
	"foreach\nelif\n"

// `$reswords` is the reserved-word table, whole and in zsh's own order.
//
// Measured 2026-09-12 against zsh 5.9.2 under `-f`, and the count is pinned
// beside the listing on purpose: a truncated list and a reordered one both
// read as "close" in a diff, and `${#reswords}` is the one number a reader
// can check against the shell being modeled in a single line.
func TestReswordsIsZshsReservedWordTable(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`zmodload zsh/parameter; print -r -- "n=${#reswords}"; print -rl -- $reswords`)
	want := "n=31\n" + zshReswordsListing
	if out != want || st != 0 {
		t.Errorf("$reswords = %q (status %d), want %q", out, st, want)
	}
}

// And it reads before anything has loaded the module, which is zsh's own
// answer: the parameter is autoloaded there, so `${(t)reswords}` and
// `${#reswords}` both answer in a shell that has run no `zmodload`. Here the
// view is registered when the dialect is built, which reaches the same place
// by a different route — see the note on `$terminfo` in declaretail_test.go,
// where the difference is recorded rather than matched.
func TestReswordsReadsWithoutTheModuleBeingLoaded(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `print -r -- "n=${#reswords} t=${(t)reswords}"`)
	want := "n=31 t=array-readonly-hideval-special\n"
	if out != want || st != 0 {
		t.Errorf("$reswords unloaded = %q (status %d), want %q", out, st, want)
	}
}

// It is read-only, as most of `zsh/parameter`'s tables are — and the refusal
// is what keeps a write from leaving a stored array standing in front of the
// producer, which is the same argument `builtins` and `parameters` carry.
//
// Measured in zsh 5.9.2 from a script file: `reswords=(a b)`,
// `reswords[1]=x` and `unset reswords` are each
// `read-only variable: reswords`, and each **ends the script** at status 1 —
// the two lines after the assignment below never run there either. That is
// what the missing `assign=` and `n=` lines record, and it is the same
// fatality every frozen name in this module has.
func TestReswordsRefusesAWrite(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `reswords=(a b) 2>&1
echo "assign=$?"
print -r -- "n=${#reswords}"`)
	want := "zsh:1: read-only variable: reswords\n"
	if out != want || st != 1 {
		t.Errorf("writing $reswords = %q (status %d), want %q at 1", out, st, want)
	}
}

// The shape a highlighter actually asks in — `$reswords[(Ie)$1]`, the index
// of an exact match — because membership is the whole of what F-Sy-H reads
// this parameter for, on `fast-highlight` line 311.
//
// The four probes are the ones that separate a measured list from a
// from-first-principles one. `[[` is reserved and `]]` is not, because the
// closer is read by the `[[` parser rather than reserved on its own; `in` is
// not, for the same reason; `typeset` is, even though it is a builtin in this
// shell's grammar, because the caller is asking how zsh classifies the word.
// A list assembled from this parser's own reserved words would answer the
// opposite for all four.
func TestReswordsAnswersTheIndexAHighlighterAsks(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/parameter
for w in if typeset foreach '[[' ']]' in ls; do
  print -r -- "$w ${reswords[(Ie)$w]}"
done`)
	want := "if 1\ntypeset 9\nforeach 30\n[[ 19\n]] 0\nin 0\nls 0\n"
	if out != want || st != 0 {
		t.Errorf("$reswords index = %q (status %d), want %q", out, st, want)
	}
}

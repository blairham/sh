// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `read` skipped every leading `-` word, whatever it was. Two of those are
// worth naming: `-s` would have echoed what was meant to be hidden, and `-n 1`
// would have read a whole line. Both silently.

// TestReadKeepsItsOwnOption is the control: `-r` is the one option this shell
// implements, and it has to keep working now that the others are refused.
//
// A backslash at the end of a line is where it shows: without -r the line
// continues, with it the backslash survives and the line ends. The backslash
// *inside* a line is TestReadWithoutRawRemovesTheBackslash.
func TestReadKeepsItsOwnOption(t *testing.T) {
	joined, _ := run(t, "printf 'a\\\\\nb\n' | { read v; echo \"[$v]\"; }", nil)
	if !strings.Contains(joined, "[ab]") {
		t.Errorf("read: got %q, want the line continued", joined)
	}
	kept, _ := run(t, "printf 'a\\\\\nb\n' | { read -r v; echo \"[$v]\"; }", nil)
	if !strings.Contains(kept, `[a\]`) {
		t.Errorf("read -r: got %q, want the backslash kept and the line ended", kept)
	}
}

// TestReadWithoutRawRemovesTheBackslash: without -r a backslash removes the
// special meaning of the character after it and is itself removed — measured,
// unanimous across the panel (#320). The subtle half is the separator: an
// escaped IFS character is data and does not split, which is why the escape
// positions ride into the splitter as a mask instead of the processing being
// a pre-pass over the string — after the pre-pass, an escaped space and a
// separating one would be the same byte.
func TestReadWithoutRawRemovesTheBackslash(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an ordinary character keeps only itself",
			`printf 'a\\tb\n' | { read v; echo "[$v]"; }`, "[atb]",
		},
		{
			"an escaped separator does not split",
			`printf 'a\\ b c\n' | { read x y; echo "[$x][$y]"; }`, "[a b][c]",
		},
		{
			"an escaped non-whitespace separator does not split",
			`printf 'a\\:b:c\n' | { IFS=: read x y; echo "[$x][$y]"; }`, "[a:b][c]",
		},
		{
			"an escaped backslash is one literal backslash",
			`printf 'a\\\\b\n' | { read v; echo "[$v]"; }`, `[a\b]`,
		},
		{
			"an escaped leading space is not trimmed",
			`printf '\\ a b\n' | { read x y; echo "[$x][$y]"; }`, "[ a][b]",
		},
		{
			"an escaped space still ends at a real one",
			`printf 'a\\  b\n' | { read x y; echo "[$x][$y]"; }`, "[a ][b]",
		},
		{
			"a backslash the input ends on is dropped",
			`printf 'a\\' | { read v; echo "st=$? [$v]"; }`, "st=1 [a]",
		},
		{
			"the count is of delivered characters",
			`printf 'a\\tbcd\n' | { read -n 3 v; echo "[$v]"; }`, "[atb]",
		},
		{
			"raw keeps every backslash",
			`printf 'a\\tb\n' | { read -r v; echo "[$v]"; }`, `[a\tb]`,
		},
		{
			"raw splits on a backslashed separator",
			`printf 'a\\ b c\n' | { read -r x y; echo "[$x][$y]"; }`, `[a\][b c]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, nil)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestReadRefusesAnOptionItDoesNotHave rather than skipping it, which is what
// makes the difference between a shell that says it cannot do something and
// one that quietly does something else.
func TestReadRefusesAnOptionItDoesNotHave(t *testing.T) {
	out, _ := run(t, `read -q v </dev/null`, func(r *Runner) {
		s := *r.Semantics
		s.BadOptionToSpecialBuiltinFatal = No
		r.Semantics = &s
		dg := Diagnostics{BuiltinBadOption: "read: %[2]s: bad"}
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "read: -q: bad") {
		t.Errorf("got %q, want the option refused", out)
	}
}

// TestReadSaysWhenAnOptionIsMerelyMissing, which is the answer for the ones
// the dialect really has — the -p prompt, now that the letters this shell
// does implement are read rather than refused.
func TestReadSaysWhenAnOptionIsMerelyMissing(t *testing.T) {
	out, _ := run(t, `read -p v </dev/null`, func(r *Runner) {
		s := *r.Semantics
		s.BadOptionToSpecialBuiltinFatal = No
		r.Semantics = &s
		dg := Diagnostics{
			BuiltinBadOption:           "read: %[2]s: bad",
			UnimplementedOptionLetters: map[string]string{"read": "p"},
		}
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "not implemented yet") {
		t.Errorf("got %q, want it said to be missing rather than unknown", out)
	}
}

// TestReadSplitsAClusteredBundle: `-rr` is two options in one word, both of
// them the letter this shell implements. Reading the word whole refused it —
// the bundle was not `-r`, so it went to the refusal path with an option the
// builtin has inside it (#347).
func TestReadSplitsAClusteredBundle(t *testing.T) {
	out, _ := run(t, "printf 'x\\\\\ny\n' | { read -rr v; echo \"[$v]\"; }", nil)
	if !strings.Contains(out, `[x\]`) {
		t.Errorf("read -rr: got %q, want the bundle read as two -r flags", out)
	}
}

// TestAMissingOptionInABundleIsNamedAlone: the half of #347 that shows for a
// letter still missing — the -p prompt, since #321 filled the rest in.
// `read -rp` is `-r -p`, and the complaint is about `-p` — the missing
// letter — not about a word `-rp` that no shell would refuse.
func TestAMissingOptionInABundleIsNamedAlone(t *testing.T) {
	out, _ := run(t, `read -rp v </dev/null`, func(r *Runner) {
		s := *r.Semantics
		s.BadOptionToSpecialBuiltinFatal = No
		r.Semantics = &s
		dg := Diagnostics{
			BuiltinBadOption:           "read: %[2]s: bad",
			UnimplementedOptionLetters: map[string]string{"read": "p"},
		}
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "-p is not implemented yet") {
		t.Errorf("got %q, want -p said to be missing", out)
	}
	if strings.Contains(out, "-rp") {
		t.Errorf("got %q, want the letter named without the bundle", out)
	}
}

// TestAnUnknownOptionInABundleIsNamedAlone is the same rule on the refusal
// path: the shells walk the bundle and stop at the letter they cannot use, so
// the complaint names that letter and not the word it rode in on.
func TestAnUnknownOptionInABundleIsNamedAlone(t *testing.T) {
	out, _ := run(t, `read -rq v </dev/null`, func(r *Runner) {
		s := *r.Semantics
		s.BadOptionToSpecialBuiltinFatal = No
		r.Semantics = &s
		dg := Diagnostics{BuiltinBadOption: "read: %[2]s: bad"}
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "read: -q: bad") {
		t.Errorf("got %q, want the offending letter named alone", out)
	}
}

// TestDashDashEndsReadsOptions, so a variable named like an option can still
// be read into.
func TestDashDashEndsReadsOptions(t *testing.T) {
	out, _ := run(t, `printf 'x\n' | { read -- v; echo "[$v]"; }`, nil)
	if !strings.Contains(out, "[x]") {
		t.Errorf("got %q, want -- taken as the end of the options", out)
	}
}

// The tests below are #321: the letters read takes are the dialect's —
// ReadOptions — and each behavior is measured against the shells that have
// the letter. The default vector here carries `a:` for the array and counts
// after -n/-N; the tests that need the other spellings say so inline.

// TestReadArrayOptionSplitsIntoTheNamedArray: the fields land in the array
// the option's argument names, replacing whatever the array held, and the
// operands after it are left untouched — measured, `x=keep` survives
// `read -a arr x`.
func TestReadArrayOptionSplitsIntoTheNamedArray(t *testing.T) {
	out, _ := run(t, `arr=(1 2 3 4); printf 'a b c\n' | { read -a arr; echo "[${arr[0]}][${arr[2]}][${#arr[@]}]"; }`, nil)
	if !strings.Contains(out, "[a][c][3]") {
		t.Errorf("got %q, want the three fields and nothing of the old array", out)
	}
	kept, _ := run(t, `x=keep; printf 'a b\n' | { read -a arr x; echo "x=[$x]"; }`, nil)
	if !strings.Contains(kept, "x=[keep]") {
		t.Errorf("got %q, want the operand after the array's name ignored", kept)
	}
}

// TestReadArrayFlagTakesTheFirstOperand is the other spelling: a bare `A`
// in ReadOptions, the array named by the first operand — and the names after
// it cleared, which is the measured difference from the `a:` form.
func TestReadArrayFlagTakesTheFirstOperand(t *testing.T) {
	withA := func(r *Runner) {
		s := *r.Semantics
		s.ReadOptions = "rsAd:n:N:t:u:"
		r.Semantics = &s
	}
	out, _ := run(t, `printf 'a b c\n' | { read -A arr; echo "[${arr[1]}]"; }`, withA)
	if !strings.Contains(out, "[b]") {
		t.Errorf("got %q, want the fields in the operand-named array", out)
	}
	cleared, _ := run(t, `x=keep; printf 'a b\n' | { read -A arr x; echo "x=[$x]"; }`, withA)
	if !strings.Contains(cleared, "x=[]") {
		t.Errorf("got %q, want the names after the array cleared", cleared)
	}
}

// TestReadDelimiterEndsTheRead: -d renames the delimiter — the first
// character of its argument speaks — and the newline becomes ordinary input.
// An input that ends without the delimiter is the same failure-with-a-value
// as a last line without its newline.
func TestReadDelimiterEndsTheRead(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"stops at the delimiter", `printf 'a:b c\n' | { read -d : v; echo "[$v]"; }`, "[a]"},
		{"first character speaks", `printf 'axyb' | { read -d xy v; echo "[$v]"; }`, "[a]"},
		{"newline is ordinary", `printf 'a\nb:c' | { read -d : v w; echo "[$v][$w]"; }`, "[a][b]"},
		{"splitting still applies", `printf 'a b:c' | { read -d : x y; echo "[$x][$y]"; }`, "[a][b]"},
		{"missing delimiter is EOF", `printf 'ab' | { read -d : v; echo "st=$? [$v]"; }`, "st=1 [ab]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, nil)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestReadDelimiterHonorsTheBackslash, unless -r: an escaped delimiter is
// data with the backslash dropped, and an escaped newline is still the line
// continuation even when the newline is no longer the delimiter.
func TestReadDelimiterHonorsTheBackslash(t *testing.T) {
	kept, _ := run(t, `printf 'a\\:b:c' | { read -d : v; echo "[$v]"; }`, nil)
	if !strings.Contains(kept, "[a:b]") {
		t.Errorf("got %q, want the escaped delimiter kept as data", kept)
	}
	folded, _ := run(t, "printf 'a\\\\\nb:c' | { read -d : v; echo \"[$v]\"; }", nil)
	if !strings.Contains(folded, "[ab]") {
		t.Errorf("got %q, want the escaped newline folded away", folded)
	}
	raw, _ := run(t, `printf 'a\\:b:c' | { read -rd : v; echo "[$v]"; }`, nil)
	if !strings.Contains(raw, `[a\]`) {
		t.Errorf("got %q, want -r to make the backslash ordinary", raw)
	}
}

// TestReadCountStopsAtTheCount: -n reads at most that many characters, the
// delimiter still ends it early, and what arrived splits as any read does.
func TestReadCountStopsAtTheCount(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the count", `printf 'abcdef' | { read -n 3 v; echo "st=$? [$v]"; }`, "st=0 [abc]"},
		{"bundled count", `printf 'abcdef' | { read -n3 v; echo "[$v]"; }`, "[abc]"},
		{"the delimiter still ends it", `printf 'ab\ncd' | { read -n 3 v; echo "st=$? [$v]"; }`, "st=0 [ab]"},
		{"fields still split", `printf 'a b c d' | { read -n 5 x y; echo "[$x][$y]"; }`, "[a][b c]"},
		{"a count of zero reads nothing", `printf 'abc' | { read -n 0 v; echo "st=$? [$v]"; }`, "st=0 []"},
		{"nothing at all is a failure", `read -n 3 v </dev/null; echo "st=$? [$v]"`, "st=1 []"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, nil)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestAShortCountAsksTheDialect: input that ends after some but fewer than N
// characters is the one -n case the shells answer differently, so it is the
// one that asks ReadPartialCountSucceeds. Both answers keep the text.
func TestAShortCountAsksTheDialect(t *testing.T) {
	src := `printf 'ab' | { read -n 3 v; echo "st=$? [$v]"; }`
	failed, _ := run(t, src, nil)
	if !strings.Contains(failed, "st=1 [ab]") {
		t.Errorf("got %q, want the short fill kept and reported", failed)
	}
	succeeded, _ := run(t, src, func(r *Runner) {
		s := *r.Semantics
		s.ReadPartialCountSucceeds = Yes
		r.Semantics = &s
	})
	if !strings.Contains(succeeded, "st=0 [ab]") {
		t.Errorf("got %q, want the short fill kept and called a success", succeeded)
	}
}

// TestReadExactCountTakesTheTextWhole: -N makes the delimiter and the
// backslash ordinary and hands the text over unsplit — the second name gets
// nothing rather than a field.
func TestReadExactCountTakesTheTextWhole(t *testing.T) {
	out, _ := run(t, "printf 'ab\ncd\n' | { read -N 4 v; printf '<%s>' \"$v\"; }", nil)
	if !strings.Contains(out, "<ab\nc>") {
		t.Errorf("got %q, want the newline read as ordinary input", out)
	}
	unsplit, _ := run(t, `printf 'a b c' | { read -N 5 x y; echo "[$x][$y]"; }`, nil)
	if !strings.Contains(unsplit, "[a b c][]") {
		t.Errorf("got %q, want the text whole in the first name", unsplit)
	}
	raw, _ := run(t, `printf 'a\\zb' | { read -N 4 v; echo "[$v]"; }`, nil)
	if !strings.Contains(raw, `[a\zb]`) {
		t.Errorf("got %q, want the backslash ordinary under -N", raw)
	}
}

// TestAShortExactCountAsksWhatSurvives: input that ends before N is a
// failure in both shells with the letter; whether the partial text reaches
// the variable is ReadExactCountKeepsPartial.
func TestAShortExactCountAsksWhatSurvives(t *testing.T) {
	src := `printf 'ab' | { read -N 5 v; echo "st=$? [$v]"; }`
	kept, _ := run(t, src, nil)
	if !strings.Contains(kept, "st=1 [ab]") {
		t.Errorf("got %q, want the partial text kept", kept)
	}
	dropped, _ := run(t, src, func(r *Runner) {
		s := *r.Semantics
		s.ReadExactCountKeepsPartial = No
		r.Semantics = &s
	})
	if !strings.Contains(dropped, "st=1 []") {
		t.Errorf("got %q, want the partial text dropped", dropped)
	}
}

// TestReadSilentIsANoOpAwayFromATerminal: -s is about a terminal's echo and
// this runner never echoes, so the flag parses, the read happens, and
// nothing is said — the exact opposite of skipping the word, which is how a
// password once echoed (#321).
func TestReadSilentIsANoOpAwayFromATerminal(t *testing.T) {
	out, _ := run(t, `printf 'secret\n' | { read -s v; echo "st=$? v=$v"; }`, nil)
	if want := "st=0 v=secret\n"; out != want {
		t.Errorf("got %q, want %q — read, quietly, with nothing extra said", out, want)
	}
}

// TestReadFromANamedDescriptor: -u resolves the number against the shell's
// own table — standard input by its number, and what `exec 5<file` recorded
// for anything past the named three.
func TestReadFromANamedDescriptor(t *testing.T) {
	out, _ := run(t, `printf 'x y\n' | { read -u 0 v; echo "[$v]"; }`, nil)
	if !strings.Contains(out, "[x y]") {
		t.Errorf("got %q, want -u 0 to be standard input", out)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("hello\nthere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	held, _ := run(t, `exec 5<f; read -u 5 v; echo "[$v]"; read -u 5 w; echo "[$w]"`, func(r *Runner) {
		r.Dir = dir
	})
	if !strings.Contains(held, "[hello]") || !strings.Contains(held, "[there]") {
		t.Errorf("got %q, want two lines off descriptor 5 in turn", held)
	}
}

// TestReadRefusesADeadDescriptor with status 1, speaking only where the
// dialect has words — one shell in the panel reports this in silence, so an
// empty wording is an answer and there is no fallback.
func TestReadRefusesADeadDescriptor(t *testing.T) {
	quiet, st := run(t, `read -u 7 v; echo "st=$?"`, nil)
	if !strings.Contains(quiet, "st=1") || strings.Contains(quiet, "descriptor") {
		t.Errorf("got %q (status %d), want a silent failure by default", quiet, st)
	}
	spoken, _ := run(t, `read -u 7 v; echo "st=$?"`, func(r *Runner) {
		dg := Diagnostics{ReadBadFileDescriptor: "read: %[1]s: no such descriptor"}
		r.Diagnostics = &dg
	})
	if !strings.Contains(spoken, "read: 7: no such descriptor") {
		t.Errorf("got %q, want the dialect's wording with the number in it", spoken)
	}
}

// TestReadRefusesANumberThatIsNotOne: the counts, the timeout and the
// descriptor all take numbers, and a word that is not one is refused with
// the substrate's wording until a dialect measures its own.
func TestReadRefusesANumberThatIsNotOne(t *testing.T) {
	for _, src := range []string{
		`read -n bogus v </dev/null; echo "st=$?"`,
		`read -u bogus v </dev/null; echo "st=$?"`,
		`read -t bogus v </dev/null; echo "st=$?"`,
	} {
		out, _ := run(t, src, nil)
		if !strings.Contains(out, "invalid number") || !strings.Contains(out, "st=1") {
			t.Errorf("%s: got %q, want the word refused with status 1", src, out)
		}
	}
}

// TestReadTimeoutStillReadsWaitingInput is the deterministic half of -t:
// input that is already there arrives well inside any deadline, so the only
// visible effect is that nothing visible happens.
func TestReadTimeoutStillReadsWaitingInput(t *testing.T) {
	out, _ := run(t, `printf 'x y\n' | { read -t 30 a b; echo "st=$? [$a][$b]"; }`, nil)
	if !strings.Contains(out, "st=0 [x][y]") {
		t.Errorf("got %q, want the waiting line read as if -t were absent", out)
	}
}

// blockedInput never delivers a byte until the test ends, which is the one
// input a timeout can be tested against without a race: nothing arriving is
// deterministic where something arriving slowly is not.
type blockedInput struct{ done chan struct{} }

func (b blockedInput) Read([]byte) (int, error) { <-b.done; return 0, io.EOF }

// TestReadTimeoutExpires: the deadline passes, the variables are cleared —
// a failing read still assigns — and the status is the dialect's number,
// 1 unless it says otherwise.
func TestReadTimeoutExpires(t *testing.T) {
	in := blockedInput{done: make(chan struct{})}
	t.Cleanup(func() { close(in.done) })
	out, _ := run(t, `v=keep; read -t 0.05 v; echo "st=$? [$v]"`, func(r *Runner) {
		r.Stdin = in
		dg := Diagnostics{ReadTimeoutStatus: 142}
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "st=142 []") {
		t.Errorf("got %q, want the dialect's timeout status and a cleared variable", out)
	}
}

// TestReadCountLetterAsAFlag is the third shape of -n: a dialect whose
// ReadOptions carries a bare `n` reads it, takes no count, and changes
// nothing — the word after it is an ordinary operand.
func TestReadCountLetterAsAFlag(t *testing.T) {
	out, _ := run(t, `printf 'abc\n' | { read -n v; echo "[$v]"; }`, func(r *Runner) {
		s := *r.Semantics
		s.ReadOptions = "rsnAd:t:u:"
		r.Semantics = &s
	})
	if !strings.Contains(out, "[abc]") {
		t.Errorf("got %q, want -n read as a flag and the operand read into", out)
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The declaration long tail, measured against zsh 5.9.2 (2026-09-04).

// zsh keeps a said-back function's opening brace on the header's line,
// indents with tabs, terminates statements with nothing, and gives `then`
// and `do` lines of their own.
func TestTypesetFSaysTheFunctionBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f() { if true; then echo one; fi; for w in a b; do echo $w; done; }
typeset -f f`)
	want := "f () {\n\tif true\n\tthen\n\t\techo one\n\tfi\n\tfor w in a b\n\tdo\n\t\techo $w\n\tdone\n}\n"
	if out != want || st != 0 {
		t.Errorf("typeset -f f = %q (status %d), want %q", out, st, want)
	}
}

// `-F` is a float's precision here, not bash's function listing, and this
// engine has no float attribute to record it in — so it is taken in silence
// at 0, which is what the real shell answers to every shape of it (#1037).
//
// The assertions are on **both streams by byte**, not on the status. A check
// that only read the status would pass against a shell that said something
// harmless, and the whole shape this case exists for is the *silent* answer:
// `declare -F` is how a state capture asks what functions exist, so a caller
// gets nothing either way and must not also get a complaint in the output it
// shows a person.
func TestTypesetCapitalFIsTakenInSilence(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"a bare listing, which is what a state capture writes", "declare -F"},
		{"the same under the other name", "typeset -F"},
		{"with a function's name, which is what it means elsewhere", "f() { :; }\ndeclare -F f"},
		{"with a name that is nothing at all", "declare -F nosuch"},
		{"a listing with functions already defined", "f() { :; }\ng() { :; }\ndeclare -F"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(), c.src)
			if out != "" {
				t.Errorf("stdout = %q, want not one byte written", out)
			}
			if errs != "" {
				t.Errorf("stderr = %q, want not one byte written", errs)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// The letter is silent and not *ignored*: a declaration carrying only it is
// still a declaration, so it must not fall through to the bare word's
// listing — which writes the whole variable table and is the one answer
// worse than the refusal this replaced.
func TestTypesetCapitalFIsNotTheBareWord(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(), "marked=here\ndeclare -F")
	if out != "" || errs != "" || st != 0 {
		t.Errorf("got stdout %q stderr %q status %d, want a silent 0", out, errs, st)
	}
	// The control: the bare word really does list, so the row above is a
	// statement about the letter rather than about an engine that lists
	// nothing.
	bare, st, _ := runZshSplit(t, t.TempDir(), "marked=here\ndeclare")
	if st != 0 || !strings.Contains(bare, "marked=here\n") {
		t.Errorf("bare declare = %q (status %d), want the name listed", bare, st)
	}
}

// What the silence costs, written down as a test so it cannot drift into a
// claim nobody checks: the precision is the whole of what the letter does in
// that shell, and a value declared with it reads back unformatted here. The
// refusal it replaced set the name to nothing at all, so this is the better
// of two wrong answers rather than a right one.
func TestTypesetCapitalFDoesNotFormatTheValue(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -F v=1.5; echo "[$v]"`)
	if out != "[1.5]\n" || st != 0 {
		t.Errorf("got %q (status %d), want the value assigned and unformatted", out, st)
	}
}

// `-E` is the same float family spelled for scientific notation, and it is
// deliberately still refused: nothing measured asks for it, and the case for
// silence rests on `-F` being the letter a *function* listing is spelled with
// somewhere else.
func TestTypesetCapitalEIsStillUnimplemented(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `typeset -E v 2>&1; echo st=$?`)
	// The builtin's name is in the location prefix here, as it is for every
	// message this shell writes.
	wantWholeLines(t, out, "zsh:typeset:1: -E is not implemented yet")
	if !strings.Contains(out, "st=2") {
		t.Errorf("got %q, want status 2", out)
	}
}

// The case attributes fold assignments here too, later ones included.
func TestCaseAttributesFold(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `typeset -u w=abc
echo $w
w=def
echo $w`)
	if out != "ABC\nDEF\n" {
		t.Errorf("got %q, want the attribute folding both assignments", out)
	}
}

// A bare `set` lists the variables as assignments — quoted the way this shell
// quotes them, and with no functions after them, which is where it parts from
// bash's listing rather than from dash's. Asserted on whole rendered lines,
// through a filter, because the rest of the listing is the machine's.
func TestBareSetListsAssignments(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`v1=plain; v2='has space'; v3="quo'te"; a=(x 'y z'); myfn() { echo hi; }; set`)
	if st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	for _, want := range []string{
		"v1=plain\n",
		"v2='has space'\n",
		`v3='quo'\''te'` + "\n",
		"a=( x 'y z' )\n",
		// The NUL this shell keeps in IFS is written, not emitted raw: a
		// listing with a NUL in it is binary, and `grep` says so of it.
		`IFS=$' \t\n\C-@'` + "\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("bare set: %q missing from %q", want, out)
		}
	}
	if strings.Contains(out, "myfn") {
		t.Errorf("bare set: got %q, want no functions in this shell's listing", out)
	}
}

// A bare `local` lists every parameter with its attributes in *words* — the
// order measured against zsh 5.9.2, one name per combination.
func TestBareLocalListsEveryParameterWithItsAttributes(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`f() { local x=1; local -r r=2; local -i i=3; local -a c=(x "y z"); `+
			`local -A d=([k]="v w"); local -x e=4; local -ir ir=5; local -xr q=1; local u; local; }; f`)
	if st != 0 {
		t.Fatalf("status = %d, want 0", st)
	}
	for _, want := range []string{
		"local x=1\n",
		"local readonly r=2\n",
		"integer local i=3\n",
		"array local c=( x 'y z' )\n",
		"association local d=( [k]='v w' )\n",
		"local exported e=4\n",
		"integer local readonly ir=5\n",
		"local readonly exported q=1\n",
		"local u=''\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("bare local: %q missing from %q", want, out)
		}
	}
	// A global the function never made local carries no `local` word.
	if !strings.Contains(out, "\nPATH=") && !strings.HasPrefix(out, "PATH=") {
		t.Errorf("bare local: got %q, want the globals listed with no local word", out)
	}
}

// The bare `export` and `readonly` drop the command word, which their own
// `-p` does not — and this shell's `readonly -p` is not even `readonly`.
//
// **Seven produced parameters are in both readonly listings and carry no
// value in either**, which is measured rather than an artifact: each is a
// readonly parameter this shell *produces*, and each is readonly in zsh too —
// `set` there writes the bare name and `typeset -r` writes the bare name,
// because the parameter is hidden. Writing the table out instead would put
// every builtin this shell has into a listing a caller sources back (#1060).
//
// `builtins` is the one with contents. Three are `zsh/parameter`'s
// empty-and-right-to-be ones that zsh marks readonly — `dis_functions_source`,
// `dis_patchars` and `dis_reswords` — and readonly is what keeps a write from
// leaving a stored table in front of the view (#1146). The last three are
// `zsh/datetime`'s clock reads, readonly for the same reason and readonly in
// zsh too, where they list exactly here: `EPOCHREALTIME`, `EPOCHSECONDS` and
// `epochtime`, bare, between `R=2` and the rest by name (#1154). The `-Ar`
// against the bare `-r` is the association attribute showing through.
//
// `zsh_scheduled_events` is the seventh and arrived with `sched` (#1376),
// readonly for the same reason and readonly in zsh too — measured,
// `zmodload zsh/sched; readonly` there writes the name bare, exactly as it
// writes `epochtime`.
//
// zsh writes a kind letter with the readonly one — `-Fr`, `-ir`, `-ar` — and
// this shell writes `-r` alone, because a produced parameter has no integer
// or float attribute here to show. Recorded rather than pinned: it is a
// letter in a listing of a hidden name, and the alternative is an attribute
// seam nothing else wants.
func TestBareExportAndReadonlyAreAssignmentsAlone(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`export V='a b'; readonly R=2; export; readonly; export -p; readonly -p`)
	want := "V='a b'\nEPOCHREALTIME\nEPOCHSECONDS\nR=2\n" +
		"builtins\ndis_functions_source\ndis_patchars\ndis_reswords\nepochtime\n" +
		"zsh_scheduled_events\n" +
		"export V='a b'\n" +
		"typeset -r EPOCHREALTIME\ntypeset -r EPOCHSECONDS\ntypeset -r R=2\n" +
		"typeset -Ar builtins\ntypeset -Ar dis_functions_source\n" +
		"typeset -r dis_patchars\ntypeset -r dis_reswords\ntypeset -r epochtime\n" +
		"typeset -r zsh_scheduled_events\n"
	if st != 0 || out != want {
		t.Errorf("got %q status %d, want %q at 0", out, st, want)
	}
}

// A bad `local` name: the digit-led complaint, fatal as every declaration's
// bad name is here, with the builtin named by the location machinery.
func TestLocalBadNameIsFatal(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `f() { local 1x=5; echo unreached; }
f
echo after`)
	if !strings.Contains(out, "not an identifier: 1x") {
		t.Errorf("got %q, want the digit-led complaint", out)
	}
	if strings.Contains(out, "after") || st != 1 {
		t.Errorf("got %q (status %d), want the script ended with 1", out, st)
	}
}

// `command -V` says it the way `type` does, shell's name off the line.
func TestCommandCapitalV(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `command -V echo
command -V nosuch; echo st=$?`)
	if !strings.Contains(out, "echo is a shell builtin\n") ||
		!strings.Contains(out, "nosuch not found\n") || !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want the sentence, the bare complaint and status 1", out)
	}
}

// `typeset -gA` is `typeset -A` at the global scope, and the associative
// attribute has to survive the letter — measured against zsh 5.9.2 on
// 2026-09-06, where every row below is what the real shell says. The letter
// once skipped every step of the declaration but the assignment, so
// `typeset -gA ZI` left an ordinary name and the `ZI[...]=` lines after it
// were refused with `assignment to invalid subscript range` (#989).
func TestGlobalAssociativeDeclaration(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the subscript is a key at the top level",
			"typeset -gA m\nm[k]=v\necho \"[$m[k]]\"", "[v]\n",
		},
		{
			"the letters in either order",
			"typeset -Ag m\nm[k]=v\necho \"[$m[k]]\"", "[v]\n",
		},
		{
			"the declaration outlives the function that made it",
			"f() { typeset -gA m; }\nf\nm[k]=v\necho \"[$m[k]]\"", "[v]\n",
		},
		{
			// The whole line as a script writes it: several names, no values,
			// and the elements assigned afterwards.
			"several names on one line",
			"typeset -gA m n\nm[a]=1\nn[b]=2\necho \"[$m[a]][$n[b]]\"", "[1][2]\n",
		},
		{
			// The listing is what says the attribute is really there, rather
			// than an index that happens to read back.
			"the listing says associative",
			"f() { typeset -gA m; }\nf\ntypeset -p m", "typeset -A m=( )\n",
		},
		{
			"a valueless global declaration brings the name into being",
			"typeset -g p\ntypeset -p p", "typeset p=''\n",
		},
		{
			"a valueless global declaration leaves a standing value alone",
			"p=1\ntypeset -g p\necho \"[$p]\"", "[1]\n",
		},
		{
			// Declared and not assigned, which this shell's listing shows as
			// an empty value — the same shape `typeset -x` without the letter
			// already has.
			"a valueless global export is listed with an empty value",
			"typeset -gx e2\ncase \"$(export -p)\" in (*\"export e2=''\"*) echo yes;; (*) echo no;; esac",
			"yes\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}

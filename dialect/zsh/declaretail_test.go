// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"regexp"
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

// `-F` is a float's precision here, not bash's function listing, so every
// shape a *function* listing would be written in answers 0 and says nothing
// — which is what the real shell does (#1037) and what it keeps doing now
// that the letter is an attribute rather than a silent no-op (#1453). A bare
// one is a filtered listing of the float names, of which a fresh shell has
// none; a named one declares that name a float and reports it to nobody.
//
// The assertions are on **both streams by byte**, not on the status. A check
// that only read the status would pass against a shell that said something
// harmless, and the whole shape this case exists for is the *silent* answer:
// `declare -F` is how a state capture asks what functions exist, so a caller
// gets nothing either way and must not also get a complaint in the output it
// shows a person.
func TestTypesetCapitalFIsTakenInSilence(t *testing.T) {
	for _, c := range []struct{ name, src, out string }{
		// `EPOCHREALTIME` is the shell's own float and is in the filtered
		// listing, which is measured rather than incidental: real zsh with
		// `zsh/datetime` loaded writes exactly that one name for
		// `typeset -F`, and nothing else. The rows here used to expect not
		// one byte, from a build where the parameter carried no float
		// attribute to be selected by (#2451).
		{"a bare listing, which is what a state capture writes", "declare -F", "EPOCHREALTIME\n"},
		{"the same under the other name", "typeset -F", "EPOCHREALTIME\n"},
		{"with a function's name, which is what it means elsewhere", "f() { :; }\ndeclare -F f", ""},
		{"with a name that is nothing at all", "declare -F nosuch", ""},
		{"a listing with functions already defined", "f() { :; }\ng() { :; }\ndeclare -F", "EPOCHREALTIME\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(), c.src)
			if out != c.out {
				t.Errorf("stdout = %q, want %q", out, c.out)
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
	// The shell's own float and nothing the script wrote — measured, real
	// zsh writes the same one name here — where the bare word below writes
	// the whole table.
	if out != "EPOCHREALTIME\n" || errs != "" || st != 0 {
		t.Errorf("got stdout %q stderr %q status %d, want the shell's own float alone at 0", out, errs, st)
	}
	// The control: the bare word really does list, so the row above is a
	// statement about the letter rather than about an engine that lists
	// nothing.
	bare, st, _ := runZshSplit(t, t.TempDir(), "marked=here\ndeclare")
	if st != 0 || !strings.Contains(bare, "marked=here\n") {
		t.Errorf("bare declare = %q (status %d), want the name listed", bare, st)
	}
}

// The precision is the whole of what the letter does in that shell, and it
// used to cost nothing here because there was no float attribute to record
// it in — `typeset -F v=1.5` read back as `1.5` where the real shell writes
// ten places (#1037). There is one now, so the letter formats (#1453).
//
// The number after the letter is its *argument* and not a second name, under
// both spellings: reading the `3` of `typeset -F 3 SECONDS=0` as a name is
// what made a plugin loader complain once per plugin.
func TestTypesetCapitalFFormatsTheValueAtItsPrecision(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		// shape is asserted in place of want where the value is the
		// *machine's* and only its form is this shell's. See the `SECONDS`
		// row, which is the only one that has one.
		shape *regexp.Regexp
	}{
		{
			"the bare letter, at its default of ten places",
			`typeset -F v=1.5; echo "[$v]"`, "[1.5000000000]\n", nil,
		},
		{
			"a precision as a word of its own",
			`typeset -F 3 v=1.5; echo "[$v]"`, "[1.500]\n", nil,
		},
		{
			"the same precision attached to the letter",
			`typeset -F3 v=1.5; echo "[$v]"`, "[1.500]\n", nil,
		},
		{
			// The elapsed time is the machine's and the *shape* is the
			// shell's, so only the shape is asserted: three decimal places,
			// which is the precision the letter was given, and a status of 0,
			// which is the `3` having been read as that argument rather than
			// as a second name. Both are what this row is for.
			//
			// It asserted the exact string `0.000` until #1952, which holds
			// only while less than 0.0005s of wall clock passes between the
			// assignment and the echo — and `SECONDS` is counted on each read
			// rather than stored, as the note twelve lines below says. On a
			// box running eight agents at once that is a coin toss, and a
			// test that passes on timing luck reads as weather: it trains
			// everyone to rerun the job rather than look at it.
			//
			// The integer part is held to one digit rather than left open, so
			// the row still says the assignment took effect — ten seconds
			// between two commands on one line is a broken machine rather
			// than a slow one, and `SECONDS` unzeroed would be the shell's
			// whole uptime.
			"the line a plugin loader opens with",
			`typeset -F 3 SECONDS=0; echo "st=$? [$SECONDS]"`, "",
			regexp.MustCompile(`^st=0 \[[0-9]\.[0-9]{3}\]\n$`),
		},
		{
			"a word that is not a number, which is a second name after all",
			`typeset -F abc v=1; echo "[$v][$abc]"`, "[1.0000000000][0.0000000000]\n", nil,
		},
		{
			"one number and not a list, the second being an operand that ends the script",
			`typeset -F 3 4 v=1.5 2>&1; echo "st=$?"`,
			"zsh:typeset:1: not an identifier: 4\n", nil,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if c.shape != nil {
				if !c.shape.MatchString(out) {
					t.Errorf("got %q, want it to match %v", out, c.shape)
				}
				return
			}
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// The value is rendered at the precision rather than printed at it: what the
// name *holds* is the formatted text, which is the shape the integer letter's
// output base has and the one every read has to agree with. A test that only
// echoed the parameter could not tell the two apart.
func TestAFloatNameHoldsTheRenderedText(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"its length is of the digits written", `typeset -F 3 v=1.5; echo "${#v}"`, "5\n"},
		{
			"a listing writes the letter and not the number again",
			`typeset -F 3 v=3.14159; typeset -p v`, "typeset -F v=3.142\n",
		},
		{
			"an assignment after the declaration folds too",
			`typeset -F 3 v; v=7; echo "[$v]"`, "[7.000]\n",
		},
		{
			"the value is an expression, as an integer name's is",
			`typeset -F 3 v=1+2; echo "[$v]"`, "[3.000]\n",
		},
		{
			"a precision arriving over a value re-renders what is standing there",
			`v=1.5; typeset -F 3 v; echo "[$v]"`, "[1.500]\n",
		},
		{
			"and a new precision re-renders it again",
			`typeset -F 3 v=1.5; typeset -F 6 v; echo "[$v]"`, "[1.500000]\n",
		},
		{
			"the bare letter over a name that has one keeps it",
			`typeset -F 3 v=1.5; typeset -F v; echo "[$v]"`, "[1.500]\n",
		},
		{
			"the plus form takes the attribute off and leaves the text",
			`typeset -F 3 v=1.5; typeset +F v; echo "[$v]"; typeset -p v`,
			"[1.500]\ntypeset v=1.500\n",
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

// The number belongs to the first number-taking letter of its word and ends
// that word: what follows the letter there is the letter's own argument, so
// those letters are lost. Only when a number really is taken — the same
// spellings with no number behind them are ordinary options, which is the
// half a rule written as "the letter ends its word" would get wrong.
func TestANumberTakingLetterEndsItsWordOnlyWhenItTakesOne(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the letters behind it are lost when a number follows",
			`typeset -Fx 3 v=1.5; typeset -p v`, "typeset -F v=1.500\n",
		},
		{
			"and are ordinary options when none does",
			`typeset -Fx v=1.5; typeset -p v`, "export -F v=1.5000000000\n",
		},
		{
			"the letters in front of it are kept either way",
			`typeset -rF 3 v=1.5; typeset -p v`, "typeset -Fr v=1.500\n",
		},
		{
			"a letter standing before it takes the number instead",
			`typeset -iF 3 v=1.5; typeset -p v`, "typeset -i3 v=1\n",
		},
		{
			"and standing after it does not",
			`typeset -Fi 3 v=1.5; typeset -p v`, "typeset -F v=1.500\n",
		},
		{
			"the integer letter reads its base the same way",
			`typeset -ix 16 n=255; typeset -p n`, "typeset -i16 n=255\n",
		},
		{
			"and keeps the letter behind it with no base to read",
			`typeset -ix n=255; typeset -p n`, "export -i n=255\n",
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

// `SECONDS` is *counted* on each read rather than stored, so it never meets
// the fold a declared name's value goes through — its producer has to ask for
// the attribute itself. Without that, the line a plugin loader opens with
// declares a float and then reads whole seconds back out of it.
func TestTheFloatAttributeReachesACountedParameter(t *testing.T) {
	for _, c := range []struct {
		name, src string
		places    int
	}{
		{"the precision a plugin loader names", `typeset -F 3 SECONDS=0; echo "[$SECONDS]"`, 3},
		{
			"the letter's default, over a counter that was never assigned",
			`typeset -F SECONDS; echo "[$SECONDS]"`, 10,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			// The digits past the decimal point are a clock, so only their
			// *number* is asserted; the leading `0.` is what says the count
			// is still near its start rather than a whole second away.
			line := strings.TrimSuffix(out, "\n")
			got := strings.TrimSuffix(strings.TrimPrefix(line, "[0."), "]")
			if st != 0 || !strings.HasPrefix(line, "[0.") || len(got) != c.places {
				t.Errorf("got %q (status %d), want a `0.` and %d decimal places",
					out, st, c.places)
			}
		})
	}
	// The control: with no float attribute the same counter reads in whole
	// seconds, so the rows above are about the attribute rather than about a
	// producer that always wrote a fraction.
	if out, st := runZsh(t, t.TempDir(), `SECONDS=0; echo "[$SECONDS]"`); out != "[0]\n" || st != 0 {
		t.Errorf("without the letter: got %q (status %d), want [0]", out, st)
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
	// A global the function never made local carries no `local` word. `PATH`
	// carries the *tie* word instead, which is measured rather than assumed:
	// zsh 5.9.2 writes `tied path PATH=/usr/bin:/bin` in this listing, naming
	// the array half the scalar is joined to.
	if !strings.Contains(out, "\ntied path PATH=") {
		t.Errorf("bare local: got %q, want PATH listed with its tie and no local word", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "tied path PATH=") && strings.Contains(line, "local ") {
			t.Errorf("bare local: %q, want no local word on a global", line)
		}
	}
}

// The bare `export` and `readonly` drop the command word, which their own
// `-p` does not — and this shell's `readonly -p` is not even `readonly`.
//
// **Ten produced parameters are in both readonly listings and carry no
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
// this shell writes the *array* one and not the integer or float one, because
// a produced parameter has no integer or float attribute here to show. The
// array letter arrived with #1618, which needed it for `$errnos` and
// `$keymaps` and gave it to the produced arrays already here at the same
// time: `typeset -ar epochtime` where this listing used to say `typeset -r
// epochtime`, which is zsh's own answer. The two scalars keep `-r` alone and
// that is recorded rather than pinned — a kind letter for a float in a
// listing of a hidden name is an attribute seam nothing else wants.
//
// Two more are `$terminfo` and `$termcap` (#1388), readonly in zsh —
// measured, `terminfo[colors]=9` there is `read-only variable: terminfo` and
// `typeset -p terminfo` is `typeset -Ar terminfo`. **When** zsh marks them is
// the one thing that differs and it is recorded rather than matched: there
// they are autoloaded, so `${+terminfo}` is 1 from the start and the readonly
// association only exists after `zmodload zsh/terminfo`, where here the view
// is registered when the dialect is built. That is the same call
// `zsh/datetime`'s three and `zsh/sched`'s one above already make, so this
// listing carries all eleven where a fresh real zsh carries none of them and
// a fully loaded one carries every one.
//
// Five more arrived with #1618 and are the four modules that issue brought:
// `$langinfo`, `$widgets`, `$keymaps` and `$sysparams` — plus `$errnos`, the
// only one of the five that is an array. Every one of them is readonly in zsh
// as well, and each is readonly here for the reason all eleven before them
// were: a produced table without the attribute would take an assignment into
// a stored table that then stands in front of the view.
//
// `ARGC` is the first *scalar special of the shell itself* in the listing
// (#1682), and the one place the two forms below disagree with zsh. Measured
// 2026-09-10 against zsh 5.9.2 with `-f`: bare `readonly` there writes
// `ARGC=0` among a dozen more of its own — `!`, `#`, `$`, `*`, `-`, `?`, `@`,
// `HISTCMD`, `LINENO`, `PPID`, `TTYIDLE`, `ZSH_EVAL_CONTEXT`, `ZSH_SUBSHELL`,
// `status` and `zsh_eval_context` — so the bare form here agrees on the one
// name it has and is still missing the other fourteen. `readonly -p` there
// writes `typeset -r R=2` alone: its `-p` skips the shell's own specials, and
// `typeset -p ARGC` prints nothing at all while `typeset -p EPOCHSECONDS`
// beside it prints `typeset -ir EPOCHSECONDS`. That distinction — a special
// the shell was born with against a parameter a module brought — is not one
// this engine draws, and drawing it is the same missing seam as the fourteen
// absent names rather than anything about `ARGC`. Recorded here, not pinned
// elsewhere.
//
// The eleventh is `$parameters` (#1599), which joins for exactly the reasons
// `builtins` did: zsh answers `parameters[x]=y` with `read-only variable:
// parameters`, and a produced table without the attribute would take an
// assignment into a stored table that then stands in front of the view. It
// lists as `typeset -Ar parameters` there too.
//
// `OLDPWD` is on both listings because this shell exports it from the first
// command, which is InheritedOldpwdIgnored's other half: measured 2026-09-12,
// `env -i zsh -c "export V='a b'; export -p"` writes `export OLDPWD=$PWD`
// beside `export PWD=$PWD` and `export -i10 SHLVL=1`. Only the first of the
// three is answered here, so this row is one name closer to the real shell's
// listing rather than equal to it (#1490).
func TestBareExportAndReadonlyAreAssignmentsAlone(t *testing.T) {
	dir := t.TempDir()
	out, st := runZsh(t, dir,
		`export V='a b'; readonly R=2; export; readonly; export -p; readonly -p`)
	want := "OLDPWD=" + dir + "\nV='a b'\nARGC=0\nEPOCHREALTIME\nEPOCHSECONDS\nR=2\n" +
		"builtins\ndis_functions_source\ndis_patchars\ndis_reswords\nepochtime\n" +
		"errnos\nkeymaps\nlanginfo\nparameters\nsysparams\ntermcap\nterminfo\n" +
		"widgets\nzsh_scheduled_events\n" +
		"export OLDPWD=" + dir + "\nexport V='a b'\n" +
		// The kind letters beside the readonly one, measured: real zsh's
		// `readonly -p` writes `typeset -Fr EPOCHREALTIME` and
		// `typeset -ir EPOCHSECONDS` (#2451).
		"typeset -r ARGC=0\ntypeset -Fr EPOCHREALTIME\ntypeset -ir EPOCHSECONDS\n" +
		"typeset -r R=2\n" +
		"typeset -Ar builtins\ntypeset -Ar dis_functions_source\n" +
		"typeset -ar dis_patchars\ntypeset -ar dis_reswords\ntypeset -ar epochtime\n" +
		"typeset -ar errnos\ntypeset -ar keymaps\ntypeset -Ar langinfo\n" +
		"typeset -Ar parameters\ntypeset -Ar sysparams\ntypeset -Ar termcap\n" +
		"typeset -Ar terminfo\ntypeset -Ar widgets\n" +
		"typeset -ar zsh_scheduled_events\n"
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

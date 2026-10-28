// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The two glob qualifiers that run shell code for each file: `e`, whose
// argument is the code, and `+`, whose argument names a command to run with
// no arguments. A file is kept when the code's status is 0, and the code may
// change what the file is reported as.
//
// Measured 2026-10-15 on zsh 5.9.2 (`-f -c`), in a directory holding regular
// files `a b big c10 c9`, a directory `d` and a link `lnk` to `big`, whose own
// order (`ls -f`) is `big c9 a c10 lnk d b`:
//
//	*(e:'[[ -d $REPLY ]]':)         d          REPLY is the name
//	*(e:'REPLY=x$REPLY':)           xa xb …    and the name it leaves is the word
//	*(e:'reply=(x$REPLY y)':)       xa … xlnk y y y y y y y
//	                                           an array `reply` is several
//	                                           words, each sorted as a match
//	*(e:'reply=()':)                no matches found
//	*(e:'REPLY=q; reply=(r s)':)    r … s …    `reply` wins over `REPLY`
//	*(e:'unset REPLY':)             a b …      and no REPLY is the name again
//	*(e:'REPLY=x$REPLY':e:'REPLY=y$REPLY':)
//	                                ya yb …    each run starts from the name
//	*(^e:'REPLY=z$REPLY; false':)   za zb …    a rewrite stands though `^`
//	                                           turned the status around
//	*(e:'REPLY=z$REPLY; false':,/)  zd         and though the section failed
//	*(e:'REPLY=/x/$REPLY'::t)       a b …      the modifiers see the new word
//	*(e:…:On), *(…:[1,3])           sorted and picked by the new words, a
//	                                size or a time still the file's own
//	n=0; *(e:'(( n++ ))':)          every name but `big`: the code runs in
//	                                the directory's own order, and in the
//	                                shell itself, so `n` is 7 afterwards
//	REPLY=orig; *(e:true:)          REPLY is left at the last file's name,
//	                                and `reply` is unset
//	./*, d/*, */, **/*              REPLY is the word as the pattern spells it
//
// Failures inside the code are the code's and not the shell's, as they are
// inside `eval`: `nosuchcmd`, `echo [x]`, a parse failure or `break` outside a
// loop is written once per file and the file is not kept, and then the
// pattern reports its own miss. `${x?boom}`, `exit 3` and `set -e` with a
// failing command end the shell, as they do in `eval`. A diagnostic is
// located where the pattern is — `zsh:3:` for code on line 3 of a `-c`
// program, its second line at 4, and `f:2:` inside a function — and
// `$zsh_eval_context` gains `globqual`.

// globCodeRun is the outcome of one file's run of qualifier code.
type globCodeRun struct {
	// kept is the code's status being 0.
	kept bool
	// words is what the file is reported as from here on — for a sort
	// specifier, its one key.
	words []string
	// gaveUp is the code having been given up over an error rather than
	// having ended, which only a sort specifier acts on.
	gaveUp bool
}

// globCodeUse is what a run of glob code is for: a test, which keeps or drops
// the file and may rename it, or a sort specifier's key.
type globCodeUse uint8

const (
	globCodeTests globCodeUse = iota
	globCodeSortKey
)

// runGlobCode runs a code qualifier for one file whose word is word. ok is
// false where the code stopped the shell, and the expansion must stop with
// it.
//
// A sort key's run differs from a test's in what it touches: `reply` is
// neither unset for it nor read from it, and the key is `REPLY` alone —
// measured 2026-10-15 on zsh 5.9.2, `reply=(q); print *(oe:'print -r --
// $reply >&2':)` writes `q` for each file, and `reply=(w)` in the code is
// still `w` after the line. See interp/globqualsort.go.
func (r *Runner) runGlobCode(code, word string, use globCodeUse) (globCodeRun, bool) {
	if r.ctl != controlNone {
		// An earlier file's code asked to leave a function or a loop. The
		// code does not run again, and every file after that one is kept
		// **where the status that stood as the pattern's code began was
		// 0**, and dropped where it was not — measured 2026-10-15 on zsh
		// 5.9.2, `f() { print X *(e:'return 4':); echo inf; }; f` writes
		// every name but the first file's and no `inf`, and `break` in a
		// loop keeps them all; and 2026-10-27, `(exit 5)` in front of the
		// same `print` writes `X` alone, as it does under `break` and
		// `continue` and for a `return` raised on a later file whose own
		// earlier files' code left 0. It is the status before the pattern
		// and not before the file: `(exit 5); print X *(e:'[[ $REPLY = big
		// ]] || return 3':N)` keeps `big` alone, though its code left 0
		// (#6752).
		return globCodeRun{kept: r.globCodeStatusBefore == 0}, true
	}
	// The pattern being expanded around this has state of its own on the
	// runner, and a pattern inside the code would read it as its own: the
	// walk's order, its `n`, the unit a `cshnullglob` line is counting,
	// whether it missed, and the refusal a program's word holds.
	unsorted, numeric, unit, missed := r.globUnsorted, r.globNumeric, r.globUnit, r.globMissed
	before := r.globCodeStatusBefore
	r.globUnsorted, r.globNumeric, r.globUnit, r.globMissed = false, globNumericUnsaid, nil, false
	defer func() {
		r.globUnsorted, r.globNumeric, r.globUnit, r.globMissed = unsorted, numeric, unit, missed
		r.globCodeStatusBefore = before
	}()

	// And the line the shell is on, which the code's own statements move:
	// the pattern's miss is reported at the pattern's line however many
	// lines the code ran over.
	line := r.line
	defer func() { r.line = line }()

	defer r.enterProgramGlob()()

	// And the failure flags the command the pattern belongs to reads to
	// decide whether it runs: an expansion that failed inside the code —
	// `x=$(( 1/0 ))`, `${(j)x}` — is the code's, which `eval` gives up, and
	// not a failure of the outer command's own words. Measured 2026-10-15
	// on zsh 5.9.2, `n=0; print [ab](e:'(( n++ )); x=$(( 1/0 )); true':N);
	// print n=$n` writes the two complaints, an empty line and `n=2`; this
	// shell abandoned the `print` and the line after it.
	defer r.assignBookkeeping()()

	r.setVar("REPLY", word)
	if use == globCodeTests {
		r.unsetName("reply")
	}
	var gaveUp bool
	status := r.runSourced(r.ctx, code, sourced{
		eval:         true,
		callersPlace: true,
		globSort:     use == globCodeSortKey,
		gaveUp:       &gaveUp,
		label:        "eval",
		syntaxStatus: r.diag().SyntaxStatus(),
	})
	if r.ctl == controlReturn && !r.hasSomethingToReturnFrom() && !r.AtPrompt {
		// A `return` with nothing to return from, in a program rather than
		// at a prompt, ends the shell there and then, at the code's status:
		// measured 2026-10-15 on zsh 5.9.2, `print X *(e:'echo r; return
		// 3':); echo next` writes `r` once and nothing else and ends at 3 —
		// by `-c`, `-i -c`, a script file and standard input alike, in a
		// subshell (which ends at 3) and in a trap action, with the EXIT
		// trap still run and a top-level `always` half not. Inside a
		// function or a sourced file the `return` waits for the command
		// instead (the controlNone test above). So does one typed at a
		// prompt, which costs the rest of the line rather than the shell.
		r.stopTheShell()
	}
	if r.givingUpAlready() {
		// `exit`, an error the code could not contain — `${x?word}` — or
		// errexit: the shell is stopping, and the expansion with it.
		return globCodeRun{}, false
	}
	run := globCodeRun{kept: status == 0, gaveUp: gaveUp}
	if words, ok := r.arrayElems("reply"); ok && use == globCodeTests {
		run.words = words
	} else if v, ok := r.getVar("REPLY"); ok {
		run.words = []string{v}
	} else {
		run.words = []string{word}
	}
	return run, true
}

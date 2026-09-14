// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `typeset -h` and `typeset +h`, measured against zsh 5.9.2 with `-f` and a
// two-entry `PATH` (2026-09-09). It is this shell's letter alone: bash 5.3 and
// bash 3.2 answer `+h: invalid option` under `local`, `typeset` and `declare`
// alike, dash has no `typeset` and reads `local +h` as a bad variable name,
// and ksh93's `-h` is a help string on a type definition rather than anything
// that detaches a parameter — so nothing here is an axis.
//
// `compaudit` opens with `local -a -U +h fpath`, and while the letter was
// refused that line did not run: the function then walked the *caller's*
// `fpath` when deciding whether the completion directories are secure
// (#1621).

// The letter and its plus form, side by side with the shell's own answers.
// The last two rows are the pair that makes `+h` more than a letter parsed
// and dropped: same command, different answer, because only the second had an
// inherited attribute to take off.
func TestTypesetHideDetachesALocalFromTheSpecialParameter(t *testing.T) {
	// `PATH` is written again before every call on purpose: a `local` of one
	// half of a tie does not yet shadow the other half here (#1630), so a
	// call that leaves the array moved would decide the next row rather than
	// the letter under test. Each row is measured from the same start.
	out, st := runZsh(t, t.TempDir(), `PATH=/bin:/usr/bin
plain() { local PATH=/x; print -r -- "plain: ${(j:,:)path}"; }
plain
PATH=/bin:/usr/bin
plus() { local +h PATH=/x; print -r -- "plus: ${(j:,:)path} st=$?"; }
plus
PATH=/bin:/usr/bin
minus() { local -h PATH=/x; print -r -- "minus: ${(j:,:)path} st=$?"; }
minus
print -r -- "after: $PATH"
typeset -h PATH
PATH=/bin:/usr/bin
print -r -- "top: ${(j:,:)path}"
inherited() { local PATH=/x; print -r -- "inherited: ${(j:,:)path}"; }
inherited
PATH=/bin:/usr/bin
unhidden() { local +h PATH=/x; print -r -- "unhidden: ${(j:,:)path}"; }
unhidden`)
	want := "plain: /x\nplus: /x st=0\nminus: /bin,/usr/bin st=0\nafter: /bin:/usr/bin\n" +
		"top: /bin,/usr/bin\ninherited: /bin,/usr/bin\nunhidden: /x\n"
	if out != want || st != 0 {
		t.Errorf("the hide letter = %q (status %d), want %q", out, st, want)
	}
}

// The attribute a call adds lasts as long as the call, so the caller's `PATH`
// drives `path` again the moment the function returns.
func TestTypesetHideLastsOnlyAsLongAsTheCall(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `PATH=/bin:/usr/bin
f() { local -h PATH=/x; }
f
PATH=/y
print -r -- "after: ${(j:,:)path}"`)
	want := "after: /y\n"
	if out != want || st != 0 {
		t.Errorf("a returned `local -h` = %q (status %d), want %q", out, st, want)
	}
}

// A function called from inside a hidden scope inherits the attribute too:
// its own plain `local` shadows a name that is already hidden, so it is
// ordinary as well and the outer `path` is what it sees.
func TestTypesetHideIsInheritedThroughANestedCall(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `PATH=/bin:/usr/bin
inner() { local PATH=/z; print -r -- "inner: ${(j:,:)path}"; }
outer() { local -h PATH=/x; inner; }
outer`)
	want := "inner: /bin,/usr/bin\n"
	if out != want || st != 0 {
		t.Errorf("a nested local under a hidden one = %q (status %d), want %q", out, st, want)
	}
}

// The three spellings of the declaration all take the letter here — `local`,
// `typeset` and `declare` — and so does `integer`, whose letter set is its
// own and narrower.
func TestThePlusHideFormIsTakenUnderEverySpelling(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `l() { local +h a=1; print -r -- "local: $a st=$?"; }
l
ty() { typeset +h b=2; print -r -- "typeset: $b st=$?"; }
ty
de() { declare +h c=3; print -r -- "declare: $c st=$?"; }
de
it() { integer +h d=4; print -r -- "integer: $d st=$?"; }
it`)
	want := "local: 1 st=0\ntypeset: 2 st=0\ndeclare: 3 st=0\ninteger: 4 st=0\n"
	if out != want || st != 0 {
		t.Errorf("+h under the four spellings = %q (status %d), want %q", out, st, want)
	}
}

// And the line the issue was about, run as `compaudit` writes it: three
// letters in three words, one of them the plus form, and the declaration
// reports 0 with not a byte on either stream. While the letter was refused
// this was `local: +h is not implemented yet` and a status of 2, which left
// the function walking the caller's `fpath` when it went on to decide whether
// the completion directories are secure.
//
// The declaration and its status are the whole of the assertion. What the
// local `fpath` then *holds* is a different question and this shell answers
// it differently for reasons that have nothing to do with the letter — a
// local of one half of a tie does not shadow the other half here (#1630) —
// so asserting on it would pin that bug rather than this fix.
func TestTheCompauditDeclarationRuns(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `FPATH=/a:/b
f() { local -a -U +h fpath; print -r -- "st=$?"; }
f`)
	want := "st=0\n"
	if out != want || st != 0 {
		t.Errorf("compaudit's declaration = %q (status %d), want %q", out, st, want)
	}
}

// The letter's other half, and the one this shell described for a while
// without doing (#2586): a local of a parameter the shell *produces* is an
// ordinary parameter, not a second view of the producer.
//
// Every module parameter carries `hide` from its registration — see
// moduleparam.go — so a plain `local` of one is a hidden shadow with no
// letter written anywhere, which is the shape a real script hits. Measured
// against zsh 5.9.2 on 2026-09-13, `env -i PATH=/usr/bin:/bin` with a scratch
// `HOME`, `ZDOTDIR` and `HISTFILE`, over a script file with the modules
// loaded.
func TestALocalOfAModuleParameterIsAnOrdinaryParameter(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/parameter
zmodload zsh/datetime
f() { local parameters; print -r -- "bare=[$parameters] ${(t)parameters}"; }
f
g() { local parameters; parameters=(a b); print -r -- "set=[$parameters] ${(t)parameters}"; }
g
h() { local EPOCHSECONDS=5; print -r -- "clock=[$EPOCHSECONDS] ${(t)EPOCHSECONDS}"; }
h
print -r -- "after=${(t)EPOCHSECONDS}"`)
	// `scalar-local` rather than `association-local-hide-special` is the half
	// a value alone cannot show: the kind and the `special` word are both
	// read off the produced tables, so they go quiet exactly when the
	// producer does — and `hide` is not among them either, because the
	// shadow's binding is a fresh one that carries the letter only if the
	// declaration writes it.
	want := "bare=[] scalar-local\nset=[a b] array-local\n" +
		"clock=[5] scalar-local\nafter=integer-readonly-hide-hideval-special\n"
	if out != want || st != 0 {
		t.Errorf("a local of a module parameter = %q (status %d), want %q", out, st, want)
	}
}

// And the same for one of the shell's *own* produced parameters, which is
// what says this is the attribute and not "modules are different": `ARGC`
// carries neither hiding letter, so a plain shadow of it is still the
// produced view and `-h` on the declaration is the only thing asking for the
// ordinary parameter.
//
// The middle row is the control that makes the letter the subject: without
// it the declaration is refused, because the freeze on a produced name
// survives its shadow. Not written as `( … )` — a subshell is not where the
// scope is, and the whole row would pass on a shell that ignored the letter.
func TestTheHideLetterDetachesALocalOfAProducedParameter(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `plain() { local ARGC; print -r -- "plain=[$ARGC] ${(t)ARGC}"; }
plain
hidden() { local -h ARGC=5; print -r -- "hidden=[$ARGC] ${(t)ARGC}"; }
hidden
print -r -- "after=[$ARGC] ${(t)ARGC}"`)
	want := "plain=[0] integer-local-readonly-special\nhidden=[5] scalar-local-hide\n" +
		"after=[0] integer-readonly-special\n"
	if out != want || st != 0 {
		t.Errorf("`local -h` over a produced parameter = %q (status %d), want %q", out, st, want)
	}
}

// `+h` over a module parameter is the second view asked for by name, and it
// brings the freeze back with the producer: `EPOCHSECONDS` is readonly there,
// so the declaration's own value is refused rather than taken.
func TestThePlusHideFormPutsAModuleParameterBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/parameter
f() { local +h parameters; print -r -- "kind=${(t)parameters}"; }
f
g() { local +h EPOCHSECONDS=5; print -r -- "never"; }
g
print -r -- "after"`)
	// `association` and `special` are the words the producer brings back, and
	// `readonly` the freeze with it. The two hiding words stay gone, because
	// they are the shadow's binding's to carry and it carries neither — zsh
	// 5.9.2 writes exactly this, `association-local-readonly-special`.
	if !strings.Contains(out, "kind=association-local-readonly-special") {
		t.Errorf("`local +h` over a module parameter = %q, want the produced view back", out)
	}
	if !strings.Contains(out, "read-only variable: EPOCHSECONDS") || strings.Contains(out, "never") {
		t.Errorf("`local +h` over a frozen producer = %q (status %d), want the refusal "+
			"the freeze makes — the plus form puts that back too", out, st)
	}
}

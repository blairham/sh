// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"
)

// A container letter over an array literal has to be recorded before the
// literal is stored, so `readonly -a a=(1)` leaves the store until the builtin
// has returned — and a refusal it raises then is located under the enclosing
// *call* rather than under a builtin, because by that point no builtin is
// speaking.
//
// Measured 2026-09-23 on bash 5.3.20, each with `declare -air a=(1)` standing
// and each reported at status 0:
//
//	                       top level        inside `g() { … }`
//	readonly a=(1)         a: readonly …    a: readonly …
//	readonly -- a=(1)      a: readonly …    a: readonly …
//	readonly -p a=(1)      a: readonly …    a: readonly …
//	readonly -a a=(1)      a: readonly …    g: a: readonly …
//	readonly -A a=(1)      a: readonly …    g: a: readonly …
//	readonly -a a=1        readonly: a: …   readonly: a: …
//	readonly -a a="1"      readonly: a: …   readonly: a: …
//
// The letter is what moves it, which is why this is not "a refusal inside a
// function names the function": the same letter over a *scalar* operand names
// the builtin instead, because that store is the builtin's own. And the top
// level answers with no name at all rather than with the script's, which is
// what makes it the enclosing call rather than a location.
//
// **The call speaks for the first command it runs and for nothing after it.**
// That half was measured wrong when this was first written (#4382) and is
// corrected here: the name was being given to every refusal inside a call.
// It was right on the row it was taken for — `attr.tests` calls functions
// with one statement in them — and wrong everywhere else. Measured, with the
// same freeze standing:
//
//	g() { readonly -a a=(1); }                g: a: readonly variable
//	g() { :; readonly -a a=(1); }             a: readonly variable
//	g() { echo x >/dev/null; readonly … }     a: readonly variable
//	g() { zz=1; readonly -a a=(1); }          a: readonly variable
//	g() { { readonly -a a=(1); }; }           g: a: readonly variable
//	h() { readonly -a a=(1); }; g(){ :; h; }  h: a: readonly variable
//
// Uniform over nine shapes: anything the call has already dispatched takes
// the name away, a compound wrapper does not because it is not a command, and
// a fresh call gets its own name back.
//
// The two scalar rows are measured here and **not** asserted below, because
// this shell does not answer them yet: a scalar operand under the letter is
// the builtin's own store and names the builtin there, where this shell still
// leaves the name out. That is a divergence of its own rather than this one,
// and writing it into a test as an expectation would freeze the wrong answer
// — see Diagnostics.LiteralOperandAfterADeclarationIsLocatedUnderTheCall.
func TestALiteralOperandRefusedAfterItsDeclarationNamesTheCall(t *testing.T) {
	const frozen = "declare -air a=(1)\n"

	out, st := runBash(t, t.TempDir(), frozen+"g() { readonly -a a=(1); }\ng\necho after")
	want := "bash: line 2: g: a: readonly variable\nafter\n"
	if out != want || st != 0 {
		t.Errorf("inside a function gave %q at %d, want %q at 0", out, st, want)
	}

	// The same command outside a function names nothing at all. This is the
	// discriminator against "the letter names the builtin": if it did, the
	// two rows would read alike.
	out, st = runBash(t, t.TempDir(), frozen+"readonly -a a=(1)\necho after")
	want = "bash: line 2: a: readonly variable\nafter\n"
	if out != want || st != 0 {
		t.Errorf("at the top level gave %q at %d, want %q at 0", out, st, want)
	}

	// Without the letter the store runs *before* the builtin, so the function
	// is not what is speaking and the refusal carries no name — in a function
	// exactly as at the top level.
	out, _ = runBash(t, t.TempDir(), frozen+"g() { readonly a=(1); }\ng")
	if want := "bash: line 2: a: readonly variable\n"; out != want {
		t.Errorf("with no letter gave %q, want %q", out, want)
	}

	// The innermost call, not the outermost.
	out, _ = runBash(t, t.TempDir(), frozen+"h() { readonly -a a=(1); }\ng() { h; }\ng")
	if want := "bash: line 2: h: a: readonly variable\n"; out != want {
		t.Errorf("two calls deep gave %q, want %q", out, want)
	}

	// The call speaks for the first command only. Anything dispatched before
	// it takes the name away — this is the half #4382 got wrong.
	out, _ = runBash(t, t.TempDir(), frozen+"g() { :; readonly -a a=(1); }\ng")
	if want := "bash: line 2: a: readonly variable\n"; out != want {
		t.Errorf("after a command in the same call gave %q, want %q", out, want)
	}

	// A compound wrapper is not a command, so the call still speaks.
	out, _ = runBash(t, t.TempDir(), frozen+"g() { { readonly -a a=(1); }; }\ng")
	if want := "bash: line 2: g: a: readonly variable\n"; out != want {
		t.Errorf("inside a group gave %q, want %q", out, want)
	}
}

// A declaration builtin that names itself refuses an **array literal** twice:
// once for the store and once for itself. Two writers rather than one
// sentence repeated — the declaration refuses the shadow it was asked to
// make, and the literal's store refuses the write it was going to make into
// it — which is why a *scalar* operand under the same builtin is one refusal.
//
// Measured 2026-09-23 on bash 5.3.20, inside a function with the name frozen
// by an enclosing `readonly`:
//
//	local qux=7             local: qux: readonly variable
//	local qux=(one two)     qux: readonly variable, then local: qux: …
//	local -a qux=(one two)  both, the letter changing nothing
//	declare qux=(one two)   both, under `declare`
//	typeset qux=(one two)   both, under `typeset`
//	local qux=()            both — an empty literal is still a literal
//	local qux=7 quux=(a b)  one, for the scalar reached first
//
// The store's line comes first and carries the enclosing call's name under
// the same first-command rule as everything else on this path, which is what
// the two assertions below are for: opening the function, the pair reads
// `g: …` then `local: …`; later in it, `qux: …` then `local: …`.
func TestAnArrayLiteralRefusedByADeclarationSpeaksTwice(t *testing.T) {
	const frozen = "readonly qux=42\n"

	// Opening the function: the call speaks for the store's line.
	out, st := runBash(t, t.TempDir(), frozen+"f() { local qux=(one two); }\nf\necho after")
	want := "bash: line 2: f: qux: readonly variable\n" +
		"bash: line 2: local: qux: readonly variable\nafter\n"
	if out != want || st != 0 {
		t.Errorf("opening the function gave %q at %d, want %q at 0", out, st, want)
	}

	// Later in the same call: the store's line carries no name, and the pair
	// is still a pair. This is the shape the suite row turned on.
	out, _ = runBash(t, t.TempDir(),
		frozen+"f() {\nlocal qux=7\nlocal qux=(one two)\n}\nf")
	want = "bash: line 3: local: qux: readonly variable\n" +
		"bash: line 4: qux: readonly variable\n" +
		"bash: line 4: local: qux: readonly variable\n"
	if out != want {
		t.Errorf("after a first statement gave %q, want %q", out, want)
	}

	// A scalar operand is one refusal: there is no separate array store to
	// raise a second. The discriminator against "a declaration refuses
	// twice".
	out, _ = runBash(t, t.TempDir(), frozen+"f() { local qux=7; }\nf")
	if want := "bash: line 2: local: qux: readonly variable\n"; out != want {
		t.Errorf("a scalar operand gave %q, want %q", out, want)
	}

	// An empty literal is still a literal.
	out, _ = runBash(t, t.TempDir(), frozen+"f() { local qux=(); }\nf")
	want = "bash: line 2: f: qux: readonly variable\n" +
		"bash: line 2: local: qux: readonly variable\n"
	if out != want {
		t.Errorf("an empty literal gave %q, want %q", out, want)
	}

	// `declare` and `typeset` answer alike, so this is the builtin naming
	// itself rather than anything about `local`.
	for _, word := range []string{"declare", "typeset"} {
		out, _ = runBash(t, t.TempDir(), frozen+"f() { "+word+" qux=(one two); }\nf")
		want = "bash: line 2: f: qux: readonly variable\n" +
			"bash: line 2: " + word + ": qux: readonly variable\n"
		if out != want {
			t.Errorf("%s gave %q, want %q", word, out, want)
		}
	}
}

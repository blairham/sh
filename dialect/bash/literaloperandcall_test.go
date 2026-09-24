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

	// A declaration utility that names *itself* keeps doing so: its refusal
	// is raised inside the builtin, where the builtin is still speaking.
	out, _ = runBash(t, t.TempDir(), frozen+"g() { declare -a a=(1); }\ng")
	if want := "bash: line 2: declare: a: readonly variable\n"; out != want {
		t.Errorf("declare gave %q, want %q", out, want)
	}
}

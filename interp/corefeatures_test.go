// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// `$'…'` gives its escapes meaning, which is the whole of why the quoting is
// a third kind rather than an ordinary single quote.
//
// The lexer records the quoting and keeps both bytes so the source stays
// recoverable; decoding is this side's job, and for a long time nothing did
// it — `$'a\tb'` reached the output as a backslash and a t, which looks
// almost right in a terminal.
func TestDollarSingleDecodesItsEscapes(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`printf '%s' $'a\tb'`, "a\tb"},
		{`printf '%s' $'x\ny'`, "x\ny"},
		{`printf '%s' $'\x41'`, "A"},
		{`printf '%s' $'\101'`, "A"},
		{`printf '%s' $'é'`, "é"},
		{`printf '%s' $'\e'`, "\x1b"},
		{`printf '%s' $'it\'s'`, "it's"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			sem := CoreSemantics()
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// `+=` appends, and appends to the *end* of an array rather than to its first
// element — the same spelling doing two different things.
func TestAppendAssignment(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a scalar", `x=a; x+=b; echo "$x"`, "ab\n"},
		{"an unset scalar", `x+=first; echo "[$x]"`, "[first]\n"},
		{"an array", `a=(one two); a+=(three); echo "${a[*]} ${#a[@]}"`, "one two three 3\n"},
		{"an unset array", `a+=(one); echo "${a[*]} ${#a[@]}"`, "one 1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// `[*]` joins and `[@]` does not, and the subscript is not a pattern.
func TestArraySubscriptStarJoins(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"star joins", `a=(one two); echo "[${a[*]}]"`, "[one two]\n"},
		{"at does not", `a=(one two); echo "[${a[@]}]"`, "[one two]\n"},
		{"star uses IFS", `a=(one two); IFS=-; echo "[${a[*]}]"`, "[one-two]\n"},
		{"star counts too", `a=(one two); echo "[${#a[*]}]"`, "[2]\n"},
		// The subscript was going through ordinary expansion, where `*` is a
		// pattern that matched no file and became the empty string — which is
		// why `[@]` worked and `[*]` did not.

	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// A C-style `for` iterates on a condition, and an *omitted* condition is
// true — `for ((;;))` is the endless loop, not one that never runs.
func TestCStyleFor(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"counting up", `for ((i=0;i<3;i++)); do printf "%s" "$i"; done`, "012"},
		{"counting down", `for ((i=3;i>0;i--)); do printf "%s" "$i"; done`, "321"},
		{"no initializer", `i=0; for ((;i<2;)); do i=$((i+1)); printf "%s" "$i"; done`, "12"},
		{"break leaves it", `for ((i=0;i<9;i++)); do [ "$i" -eq 2 ] && break; printf "%s" "$i"; done`, "01"},
		{"continue skips", `for ((i=0;i<4;i++)); do [ "$i" -eq 1 ] && continue; printf "%s" "$i"; done`, "023"},
		{"the body may never run", `for ((i=0;i<0;i++)); do printf x; done; printf done`, "done"},
		{
			// An omitted condition is *true*: `for ((;;))` is the endless
			// loop every shell writes it as. Reading a missing expression as
			// zero would make it run no times at all, which is the quietest
			// way to get this wrong and the one nothing else here would show.
			"no condition loops forever",
			`i=0; for ((;;)); do i=$((i+1)); [ "$i" -ge 3 ] && break; done; printf "%s" "$i"`,
			"3",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := CoreSemantics()
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// A subscript that is *not* a plain literal still expands, which is the half
// the fix had to keep: only `@` and `*` are read as written.
func TestComputedSubscriptStillExpands(t *testing.T) {
	sem := CoreSemantics()
	sem.ArrayBaseIsZero = Yes
	out, _ := run(t, `a=(x y z); i=1; echo "[${a[$i]}]"`, func(r *Runner) { r.Semantics = &sem })
	if out != "[y]\n" {
		t.Errorf("got %q, want [y]", out)
	}
}

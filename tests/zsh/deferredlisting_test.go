// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// What a listing writes for a parameter nothing has referred to yet.
//
// The state is deferredparameters.go's and the arrival is measured there;
// this is the **shape** it has in the two listings that write such a name at
// all. Both had it wrong in the same direction — this shell wrote the
// parameter's registered attributes, which describe a name that is not there
// yet, and the reference writes a kind word of its own (#4923), or wrote
// nothing where the reference writes a row (#4924).
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` says *not a Go executable* —
// from a script file under `env -i PATH=/usr/bin:/bin TERM=dumb` with a
// scratch `HOME`, each line in a shell that has referred to nothing.
//
// **Every row is a pair with a primed one**, for the reason the arrival test
// gives: a grid taken only after a read agrees with a shell that defers
// nothing, and a grid taken only before agrees with a shell that has no
// notion of the state. Only a row that moves between the two says anything.
func TestADeferredParameterListsAsUndefined(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// #4923's case.
			"a bare typeset writes the word and the name",
			`typeset | while IFS= read -r l; do case $l in (*funcstack) print -r -- "[$l]";; esac; done`,
			"[undefined funcstack]\n",
		},
		{
			"and after a read it writes the attributes",
			`: ${#funcstack}
			 typeset | while IFS= read -r l; do case $l in (*funcstack) print -r -- "[$l]";; esac; done`,
			"[array readonly funcstack]\n",
		},
		{
			// The word sits where the **kind** word sits, which is what the
			// rest of these rows are evidence for: a declaration can reach
			// one of these names without bringing it into being, and what it
			// leaves is written behind the word in the ordinary order. The
			// first draft of this read `undefined` as "no attributes at
			// all", and these three falsify that.
			"a freeze the script asked for is written behind the word",
			`readonly funcstack
			 typeset | while IFS= read -r l; do case $l in (*funcstack) print -r -- "[$l]";; esac; done`,
			"[undefined readonly funcstack]\n",
		},
		{
			"and so is the unique attribute",
			`typeset -U funcstack
			 typeset | while IFS= read -r l; do case $l in (*funcstack) print -r -- "[$l]";; esac; done`,
			"[undefined unique funcstack]\n",
		},
		{
			"and the case attribute",
			`typeset -u funcstack
			 typeset | while IFS= read -r l; do case $l in (*funcstack) print -r -- "[$l]";; esac; done`,
			"[undefined uppercase funcstack]\n",
		},
		{
			// The registration's own freeze is not written, which is the
			// half of the word that is not cosmetic: every module table is
			// registered readonly here and none of them carries the word
			// until the parameter exists. The row above is what says this
			// one is about *where the attribute came from* rather than about
			// suppressing the word.
			"and the registration's own freeze is not",
			`typeset | while IFS= read -r l; do case $l in (*\ builtins) print -r -- "[$l]";; esac; done`,
			"[undefined builtins]\n",
		},
		{
			// #4924's case. The whole-table plus writes the same row, which
			// had been written down as a form that passes such a name over:
			// the probe grepped it for `^funcstack$`, and `undefined
			// funcstack` is exactly what an anchored bare name cannot see.
			"the whole-table plus writes the same row",
			`typeset + | while IFS= read -r l; do case $l in (*funcstack) print -r -- "[$l]";; esac; done`,
			"[undefined funcstack]\n",
		},
		{
			"and after a read it writes the attributes there too",
			`: ${#funcstack}
			 typeset + | while IFS= read -r l; do case $l in (*funcstack) print -r -- "[$l]";; esac; done`,
			"[array readonly funcstack]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runZshPrelude(t, t.TempDir(), tc.src)
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}

// And the name as an **operand** of a valueless declaration, which is the
// other half of #4924 and a different question from the walk above: the walk
// decides whether the row exists, and this decides what a line that names it
// writes.
//
// The ten shapes are measured in interp/deferredparam.go beside
// Runner.deferredNameListsAsAnOperand. The three that write the name are the
// three spellings of a line with no attribute letter on it, and every row
// below that writes nothing is a control for one reason a line might not:
// a letter, an attribute word for a command word, the `-p` form, and a
// declaration that really does make the binding.
func TestADeferredNameIsWrittenBackAsAnOperand(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the plus spelling writes the bare name",
			`typeset + funcstack; print -r -- "st=$?"`,
			"funcstack\nst=0\n",
		},
		{
			"so does the word with no sign at all",
			`typeset funcstack; print -r -- "st=$?"`,
			"funcstack\nst=0\n",
		},
		{
			"and the minus spelling",
			`typeset - funcstack; print -r -- "st=$?"`,
			"funcstack\nst=0\n",
		},
		{
			// A letter is a declaration rather than a listing, which is the
			// same rule the valued form carries and is measured under this
			// name too.
			"a letter writes nothing",
			`typeset -g funcstack; print -r -- "st=$?"`,
			"st=0\n",
		},
		{
			"and a word that is an attribute writes nothing",
			`readonly funcstack; print -r -- "st=$?"`,
			"st=0\n",
		},
		{
			"and the -p form writes nothing",
			`typeset -p funcstack; print -r -- "st=$?"`,
			"st=0\n",
		},
		{
			// `local` at the top level is this dialect's ordinary
			// declaration, so it lists; inside a function it really does
			// take a shadow and writes nothing. The pair is what says the
			// row turns on the declaration making a binding rather than on
			// the word.
			"local at the top level writes the name",
			`local funcstack; print -r -- "st=$?"`,
			"funcstack\nst=0\n",
		},
		{
			"and inside a function it writes nothing",
			`f() { local funcstack; print -r -- "st=$?" }
			 f`,
			"st=0\n",
		},
		{
			// The operand is not a reference and the row is not a
			// materialization: the name is left exactly where it was, which
			// is what keeps a `declareEmpty` from turning the next listing
			// of it into an ordinary row.
			"and writing the name leaves it undefined",
			`typeset funcstack
			 typeset | while IFS= read -r l; do case $l in (*funcstack) print -r -- "[$l]";; esac; done`,
			"funcstack\n[undefined funcstack]\n",
		},
		{
			// The control that says none of this is about the name being
			// hidden or produced: an ordinary name writes its value back
			// under every one of the three spellings, before and after.
			"an ordinary name keeps the answer it had",
			`v=1; typeset + v; typeset v; typeset - v`,
			"v=1\nv=1\nv=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runZshPrelude(t, t.TempDir(), tc.src)
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}

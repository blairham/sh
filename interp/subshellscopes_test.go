// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// What a subshell's own scope stack looks like *to a script*.
//
// The guarantee behind it is asserted structurally — TestACloneOwnsEveryStack
// and TestACloneOwnsEveryScopeTable compare the two runners' pointers, which
// is deterministic and is the right shape for a check about memory. But a
// pointer comparison cannot say what a shell *does*, and the two questions
// come apart: `owner` is copied verbatim rather than repointed at the clone,
// and repointing it would leave every one of those checks green while
// changing what `( … )` in a function body may write. These rows are the
// other half — the behavior, stated in script (#1783).
//
// Measured across the panel and unanimous, so they name no shell and there is
// no axis: bash 5.3, bash 3.2, bash-as-sh, dash and zsh all agree, and ksh93
// agrees by having no `local` to shadow with. The one place the panel does
// split is which shell the *last* pipeline element runs in, which is an axis
// that already exists — see the row that asks it by name below.

// TestASubshellsLocalDoesNotShadowInTheCaller is the visible half of the
// shared stack, and it needs no concurrency at all.
//
// The name is declared *only* inside the subshell, so a save recorded in the
// caller's frame has nowhere else to come from — and the caller's return then
// puts it back, undoing an assignment made after the subshell finished.
func TestASubshellsLocalDoesNotShadowInTheCaller(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The subject. `q=setbyg` happens *after* the subshell, so a save
		// taken in the caller's frame is a value the return reverts to.
		{
			"a subshell's local, then an assignment in the caller",
			`g() { ( local q=sub ); q=setbyg; }; q=global; g; echo "[$q]"`,
			"[setbyg]\n",
		},
		// The same through a pipeline element, which is a clone on a
		// goroutine rather than a clone in line.
		{
			"an element's local, then an assignment in the caller",
			`g() { { local q=elem; } | cat; q=setbyg; }; q=global; g; echo "[$q]"`,
			"[setbyg]\n",
		},
		// The control for both, and what keeps the two rows above from
		// passing for the wrong reason: a `local` the *caller* declares
		// still shadows and still unwinds at the return. A clone that threw
		// the stack away, or a caller that stopped saving, would pass the
		// rows above and fail this one.
		{
			"the caller's own local still unwinds",
			`g() { local q=inner; q=changed; echo "[$q]"; }; q=global; g; echo "[$q]"`,
			"[changed]\n[global]\n",
		},
		// And the other control: a subshell must still *read* the caller's
		// locals, or the stack has been copied to nothing.
		{
			"a subshell reads the caller's local",
			`g() { local q=inner; ( echo "[$q]" ); }; q=global; g`,
			"[inner]\n",
		},
		// A local declared in the subshell is the subshell's while it runs,
		// which says the copy is a working stack and not a frozen one.
		{
			"a subshell's local is the subshell's own",
			`g() { local q=inner; ( local q=sub; echo "[$q]" ); echo "[$q]"; }; g`,
			"[sub]\n[inner]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, st := run(t, tc.src, nil)
			if got != tc.want || st != 0 {
				t.Errorf("%s\ngot  %q status %d\nwant %q status 0", tc.src, got, st, tc.want)
			}
		})
	}
}

// TestTwoPipelineElementsDeclaringLocalsDoNotShareAScope is the concurrent
// half: the one that ended the process with no diagnostic.
//
// Both elements call a function that declares a local, so both push a frame
// and both write that frame's tables — and behind two slice headers over one
// backing array they append at the same index and read the *same* frame back.
// That is `fatal error: concurrent map writes`, which is the runtime's own
// detector rather than a Go panic, so panicguard cannot catch it and every
// recovery seam is bypassed: exit 2, nothing said.
//
// The shape is load-bearing and is easy to get wrong. A pipeline written at
// the top level of a script shares nothing, because the shell's stack is nil
// there and `append` to a nil slice allocates a fresh array for each element
// — so `f | f` on line one passes with the copy removed and proves nothing.
// What is needed is spare *capacity* behind the shared header, and that is
// what a returned call leaves: `warm` grows the stack to two frames and gives
// one back, so the pipeline below runs at length one with room for two and
// both elements write slot one.
//
// The two sides declare *different* values on purpose. A name holding the
// same value on both sides of a pipe cannot tell a shared scope from a copied
// one; what is asserted is which value each side saw and that the global
// outside is neither.
func TestTwoPipelineElementsDeclaringLocalsDoNotShareAScope(t *testing.T) {
	got, st := run(t, `
warm()  { local w=1; }
left()  { local x=L; echo "$x"; }
right() { local x=R; read got; echo "[$got$x]"; }
g() { warm; left | right; }
x=outer
g
echo "[$x]"
`, nil)
	if want := "[LR]\n[outer]\n"; got != want || st != 0 {
		t.Fatalf("got %q status %d, want %q status 0", got, st, want)
	}

	// And the same shape repeated, which is what makes a plain run
	// informative: a race needing both goroutines inside `shadow` at once is
	// not certain on one pass. Under `-race` one pass is usually enough and
	// the suite runs with it.
	got, st = run(t, `
warm() { local w=1; }
f()    { local x=1; echo "$x"; }
g() {
	warm
	i=0
	while [ $i -lt 200 ]; do
		f | f
		i=$((i+1))
	done
}
x=outer
g >/dev/null
echo "[$x]"
`, nil)
	if got != "[outer]\n" || st != 0 {
		t.Errorf("got %q status %d, want %q status 0", got, st, "[outer]\n")
	}
}

// TestTheLastPipelineElementInTheCurrentShellDoesNotShareAScope is the same
// question on the other side of the axis, and it is the shape the crash was
// reported from.
//
// Where the last element runs on the shell itself, the shell pops that
// element's frame when its function returns — which leaves the stack one
// frame long with room for two, so the *next* pipeline's first element
// appends into the slot the shell is about to append into as well. The loop
// is the mechanism here rather than a way of making a race likely: the first
// round is what leaves the capacity the second round collides in, which is
// why a single `f | f` cannot ask this at all.
//
// Named by the axis rather than by a shell, which is the rule for a test
// under interp. Two panel members answer Yes to it.
func TestTheLastPipelineElementInTheCurrentShellDoesNotShareAScope(t *testing.T) {
	got, st := run(t, `
f() { local x=1; echo "$x"; }
i=0
while [ $i -lt 200 ]; do
	f | f >/dev/null
	i=$((i+1))
done
echo survived
`, func(r *Runner) {
		r.Semantics.LastPipelineElementInCurrentShell = Yes
	})
	if got != "survived\n" || st != 0 {
		t.Errorf("got %q status %d, want %q status 0", got, st, "survived\n")
	}
}

// TestANestedPipelineDoesNotShareAScope is the same question one level down,
// where the runner that spawns the elements is itself a clone.
//
// Composition rather than components: each half can be right on its own while
// the nesting is broken, because the inner pipeline's elements are cloned
// from a runner that was already sharing.
func TestANestedPipelineDoesNotShareAScope(t *testing.T) {
	got, st := run(t, `
warm() { local w=1; }
f()    { local x=1; echo "$x"; }
g() {
	warm
	i=0
	while [ $i -lt 100 ]; do
		{ f | f; } | { f | f; }
		i=$((i+1))
	done
}
g >/dev/null
echo done
`, nil)
	if got != "done\n" || st != 0 {
		t.Errorf("got %q status %d, want %q status 0", got, st, "done\n")
	}
}

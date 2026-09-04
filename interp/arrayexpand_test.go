// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/zsh"
	. "github.com/blairham/sh/interp"
)

// An operator applies to a subscripted value exactly as it does to a variable.
//
// It did not: the subscript answered and the function returned, so every
// operator was skipped and each of these came back as the untouched element —
// silently, with status 0.
func TestAnOperatorReachesAnArrayElement(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(hello); echo "${a[0]#h}"`, "ello"},
		{`a=(hello); echo "${a[0]##*l}"`, "o"},
		{`a=(hello); echo "${a[0]%o}"`, "hell"},
		{`a=(hello); echo "${a[0]%%l*}"`, "he"},
		{`a=(hello); echo "${a[0]/l/L}"`, "heLlo"},
		{`a=(hello); echo "${a[0]//l/L}"`, "heLLo"},
		{`a=(hello); echo "${a[0]:1}"`, "ello"},
		{`a=(hello); echo "${a[0]:1:2}"`, "el"},
		{`a=(hello); echo "${a[0]^^}"`, "HELLO"},
		// A subscript that is not there fires the test; one that is does not,
		// which is what says the operators are reading the element and not
		// something else.
		{`a=(hello); echo "${a[0]:-d}"`, "hello"},
		{`a=(hello); echo "${a[9]:-d}"`, "d"},
		{`a=(hello); echo "${a[9]+set}"`, ""},
		{`a=(hello); echo "${a[0]+set}"`, "set"},
		// An element holding the empty string is *set*, which is the whole
		// difference between `:-` and `-`.
		{`a=("" x); echo "[${a[0]:-d}][${a[0]-d}]"`, "[d][]"},
	} {
		if out, _ := runBash(t, c.src); strings.TrimSpace(out) != c.want {
			t.Errorf("%s = %q, want %q", c.src, strings.TrimSpace(out), c.want)
		}
	}
}

// `${a[i]:=v}` assigns to the element. Assigning to the name instead would
// replace the whole array with one string, which is worse than the nothing
// this used to do.
func TestAssigningThroughASubscript(t *testing.T) {
	out, _ := runBash(t, `a=(x y); echo "${a[1]:=new}"; echo "[${a[@]}]"`)
	if got := strings.TrimSpace(out); got != "y\n[x y]" {
		t.Errorf("set element: got %q, want the test not to fire", got)
	}
	out, _ = runBash(t, `a=(x y); echo "${a[1]:=new}"; a=(p ""); echo "${a[1]:=new}"; echo "[${a[@]}]"`)
	if !strings.Contains(out, "[p new]") {
		t.Errorf("empty element: got %q, want the element assigned", out)
	}
	// And the array is not flattened into a scalar.
	out, _ = runBash(t, `a=(x ""); : "${a[1]:=new}"; echo "${#a[@]}"`)
	if got := strings.TrimSpace(out); got != "2" {
		t.Errorf("got %q elements, want 2 — the array must survive", got)
	}
}

// `${a[@]:off:len}` slices the list. `${a[0]:off}` does not: it names one
// element and is a substring of it, and slicing a one-element list would be no
// field at all.
func TestSlicingTheWholeArrayButNotOneElement(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(p q r s); echo "${a[@]:1}"`, "q r s"},
		{`a=(p q r s); echo "${a[@]:1:2}"`, "q r"},
		{`a=(p q r s); echo "${a[@]:0:2}"`, "p q"},
		{`a=(p q r s); echo "${a[@]: -2}"`, "r s"},
		{`a=(p q r s); echo "${a[@]: -2:1}"`, "r"},
		{`a=(p q r s); echo "${a[@]:9}"`, ""},
		{`a=(p q r s); echo "${a[*]:1}"`, "q r s"},
		// One element, and a substring of it.
		{`a=(hello); echo "${a[0]:1}"`, "ello"},
	} {
		if out, _ := runBash(t, c.src); strings.TrimSpace(out) != c.want {
			t.Errorf("%s = %q, want %q", c.src, strings.TrimSpace(out), c.want)
		}
	}
	// The slice is a list, so an element holding a space stays one field —
	// which is the whole reason it is not a substring of the joined text.
	out, _ := runBash(t, `a=("a b" c d); printf "[%s]" "${a[@]:0:2}"`)
	if got := strings.TrimSpace(out); got != "[a b][c]" {
		t.Errorf("got %q, want two fields", got)
	}
}

// `${!a[@]}` is the array's subscripts, not its elements. Answering with the
// elements made the loop that exists to use it iterate the wrong thing.
func TestArrayIndices(t *testing.T) {
	out, _ := runBash(t, `a=(p q r); echo "${!a[@]}"`)
	if got := strings.TrimSpace(out); got != "0 1 2" {
		t.Errorf("got %q, want the subscripts", got)
	}
	out, _ = runBash(t, `a=(p q r); echo "${!a[*]}"`)
	if got := strings.TrimSpace(out); got != "0 1 2" {
		t.Errorf("star form: got %q, want the subscripts", got)
	}
	out, _ = runBash(t, `a=(p q r); for i in "${!a[@]}"; do printf "%s=%s " "$i" "${a[$i]}"; done`)
	if got := strings.TrimSpace(out); got != "0=p 1=q 2=r" {
		t.Errorf("the loop: got %q", got)
	}
	// The elements are still the elements.
	out, _ = runBash(t, `a=(p q r); echo "${a[@]}"`)
	if got := strings.TrimSpace(out); got != "p q r" {
		t.Errorf("got %q, want the elements", got)
	}
}

// An operator on `${a[@]}` applies to every element. It applied to the first
// alone — the elements were joined before the operator ran — so `${a[@]#a}`
// on `(aa ab)` came back `a ab`, silently, with status 0. Unanimous in the
// three shells with arrays, the anchored and case forms included.
func TestAnOperatorDistributesOverTheElements(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(aa ab); printf "[%s]" "${a[@]#a}"`, "[a][b]"},
		{`a=(aa ab); printf "[%s]" "${a[@]##*a}"`, "[][b]"},
		{`a=(aa ab); printf "[%s]" "${a[@]%a}"`, "[a][ab]"},
		{`a=(aa ab); printf "[%s]" "${a[@]%%a*}"`, "[][]"},
		{`a=(aa ab); printf "[%s]" "${a[@]/a/X}"`, "[Xa][Xb]"},
		{`a=(aa ab); printf "[%s]" "${a[@]//a/X}"`, "[XX][Xb]"},
		{`a=(a-b c-d); printf "[%s]" "${a[@]/#?/X}"`, "[X-b][X-d]"},
		{`a=(aa AB); printf "[%s]" "${a[@]^^}"`, "[AA][AB]"},
		{`a=(aa AB); printf "[%s]" "${a[@],,}"`, "[aa][ab]"},
		// Each element stays a field of its own, spaces and all — the whole
		// reason the operator cannot run on the elements joined.
		{`a=("x y" "x z"); printf "[%s]" "${a[@]#x }"`, "[y][z]"},
		// An associative array's values distribute the same way, in the key
		// order this implementation keeps everywhere.
		{`typeset -A m; m[x]=aa; m[y]=ab; printf "[%s]" "${m[@]#a}"`, "[a][b]"},
		// `${#a[@]}` is still the count, not a trimmed anything.
		{`a=(aa ab); printf "[%s]" "${#a[@]}"`, "[2]"},
	} {
		if out, _ := runBash(t, c.src); out != c.want {
			t.Errorf("%s = %q, want %q", c.src, out, c.want)
		}
	}
}

// `${a[*]#p}` splits the panel: two shells trim each element and join what is
// left, the third joins first and trims the joined string once — so which
// string the operator sees is an axis, asked only on the star form.
func TestTheStarFormOperatorIsAnAxis(t *testing.T) {
	src := `a=(aa ab); printf "%s" "${a[*]#a}"`
	if out, _ := run(t, src, withSem(bash.Semantics())); out != "a b" {
		t.Errorf("distributing gave %q, want %q", out, "a b")
	}
	if out, _ := run(t, src, withSem(zsh.Semantics())); out != "a ab" {
		t.Errorf("joining first gave %q, want %q", out, "a ab")
	}
	// The join is on the first character of IFS either way.
	src = `a=(aa ab); IFS=-; printf "%s" "${a[*]/a/X}"`
	if out, _ := run(t, src, withSem(bash.Semantics())); out != "Xa-Xb" {
		t.Errorf("distributing gave %q, want %q", out, "Xa-Xb")
	}
	if out, _ := run(t, src, withSem(zsh.Semantics())); out != "Xa-ab" {
		t.Errorf("joining first gave %q, want %q", out, "Xa-ab")
	}
}

// Asked only where the two readings differ: a suffix trim that stops at the
// last element reads the same both ways, so a shell with no answers still
// gets it — and `${a[@]}` never asks, because every shell distributes there.
func TestTheStarAxisIsAskedOnlyWhereTheReadingsDiffer(t *testing.T) {
	out, st := run(t, `a=(aa ab); printf "%s" "${a[*]%b}"`, withSem(CoreSemantics()))
	if out != "aa a" || st != 0 {
		t.Errorf("got %q status %d, want %q and 0", out, st, "aa a")
	}
	out, st = run(t, `a=(aa ab); printf "[%s]" "${a[@]#a}"`, withSem(CoreSemantics()))
	if out != "[a][b]" || st != 0 {
		t.Errorf("got %q status %d, want no axis asked on the at form", out, st)
	}
	out, _ = run(t, `a=(aa ab); printf "%s" "${a[*]#a}"`, withSem(CoreSemantics()))
	if !strings.Contains(out, "disagree") {
		t.Errorf("got %q, want the star form refused without an answer", out)
	}
}

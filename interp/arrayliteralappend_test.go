// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `[sub]+=value` element of a literal joins what that element already
// holds, where the plain spelling replaces it.
//
// It was not read at all: the append spelling fell through to being an
// ordinary word, so the literal grew an element holding the seven characters
// `[1]+=Z` and the array came out the wrong length at status 0 (#2405).
func TestALiteralElementAppendsToTheElement(t *testing.T) {
	out, st := runArray(t, `a=(p q r); a+=( [1]+=Z ); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`)
	if strings.TrimSpace(out) != "[p][qZ][r] n=3" {
		t.Errorf("got %q, want the value joined to the element at 1", strings.TrimSpace(out))
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	if strings.Contains(out, "[1]+=Z") {
		t.Errorf("got %q, want the append read rather than stored", out)
	}
}

// The standalone spelling was the control and was already right, so the two
// are pinned together: a rule about what `+=` does to an element must not
// come to differ between the line that writes it and the literal that does.
func TestTheStandaloneElementAppendAgreesWithTheLiteral(t *testing.T) {
	line, _ := runArray(t, `a=(p q r); a[1]+=Z; printf "[%s]" "${a[@]}"`)
	lit, _ := runArray(t, `a=(p q r); a+=( [1]+=Z ); printf "[%s]" "${a[@]}"`)
	if line != lit {
		t.Errorf("`a[1]+=Z` gave %q and `a+=( [1]+=Z )` gave %q, want one answer", line, lit)
	}
}

// What is joined is the array the literal is building. A *replacing* literal
// starts from nothing, however much the name was holding, so the same element
// that appends under `+=` assigns under `=`.
func TestAReplacingLiteralAppendsToWhatItHasBuilt(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(p q r); a=( [1]+=Z ); printf "[%s]" "${a[@]}"; echo " n=${#a[@]}"`, "[Z] n=1"},
		{`a=( [1]=A [1]+=B ); printf "[%s]" "${a[@]}"`, "[AB]"},
		{`a=(p q [1]+=Z); printf "[%s]" "${a[@]}"`, "[p][qZ]"},
	} {
		if out, _ := runArray(t, c.src); strings.TrimSpace(out) != c.want {
			t.Errorf("%s gave %q, want %q", c.src, strings.TrimSpace(out), c.want)
		}
	}
}

// An element the array has not got yet has nothing to join, so the append is
// the value alone and the gap below it stays a gap.
func TestALiteralAppendToAnAbsentElement(t *testing.T) {
	out, _ := runArray(t, `a=(p q r); a+=( [5]+=Z ); echo "keys=${!a[@]} 5=${a[5]}"`)
	if strings.TrimSpace(out) != "keys=0 1 2 5 5=Z" {
		t.Errorf("got %q, want the value alone at 5", strings.TrimSpace(out))
	}
}

// A bare element after an appending one continues from that subscript, which
// is the rule the plain spelling already follows: the *subscript* is what
// moves the position, not which of the two operators wrote it.
func TestABareElementAfterAnAppendContinuesFromTheSubscript(t *testing.T) {
	out, _ := runArray(t, `a=(x [2]+=y z); echo "keys=${!a[@]} n=${#a[@]}"`)
	if strings.TrimSpace(out) != "keys=0 2 3 n=3" {
		t.Errorf("got %q, want the bare element one past the appended one", strings.TrimSpace(out))
	}
}

// The join is the name's, not the operator's: an integer-attributed name adds
// where a plain one concatenates, exactly as it does on its own line.
func TestALiteralAppendGoesThroughTheAttribute(t *testing.T) {
	src := `typeset -i n=(1 2 3); n+=( [1]+=5 ); printf "[%s]" "${n[@]}"`
	out, st := runArray(t, src)
	if strings.TrimSpace(out) != "[1][7][3]" {
		t.Errorf("got %q, want 2+5 rather than the two digits joined", strings.TrimSpace(out))
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}

	// And a keyed table the same way, which is a second join and so a second
	// place to get it wrong: the two paths reach the same helper rather than
	// each doing what an element append "obviously" is.
	sem := permissive()
	sem.CompoundAttribute = CompoundAttributeFoldsEveryElement
	sem.NumericAttributeReplacesTheArrayAttribute = No
	sem.ArrayLiteralSubscriptIsAKey = No
	out, st = run(t, `typeset -Ai m; m[k]=2; m+=( [k]+=5 ); echo "[${m[k]}]"`, withSem(sem))
	if strings.TrimSpace(out) != "[7]" {
		t.Errorf("a keyed table gave %q, want 2+5", strings.TrimSpace(out))
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// A keyed table takes the spelling too, and an *appending* literal joins what
// the table holds — unanimous in the three shells that have the attribute,
// and so no axis.
func TestAKeyedLiteralAppendsToTheTable(t *testing.T) {
	out, st := runArray(t, `typeset -A m; m=([k]=v); m+=([k]+=x); echo "[${m[k]}]"`)
	if strings.TrimSpace(out) != "[vx]" {
		t.Errorf("got %q, want the value joined under the key", strings.TrimSpace(out))
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// KeyedLiteralAppendJoinsTheReplacedValue, both answers. A *replacing* keyed
// literal empties the table before it fills it, and the two readings part
// over which table the append reads: the one being discarded, or the one
// being built.
func TestAKeyedReplacingLiteralAppendAsksWhichTable(t *testing.T) {
	for _, c := range []struct {
		src             string
		replaced, built string
	}{
		{`typeset -A m; m[k]=v; m=([k]+=x); echo "[${m[k]}]"`, "[vx]", "[x]"},
		{`typeset -A m; m[k]=v; m=([k]+=x [k]+=y); echo "[${m[k]}]"`, "[vy]", "[xy]"},
		{`typeset -A m; m[k]=v; m=([k]=A [k]+=B); echo "[${m[k]}]"`, "[vB]", "[AB]"},
	} {
		old := permissive()
		old.KeyedLiteralAppendJoinsTheReplacedValue = Yes
		if out, _ := run(t, c.src, withSem(old)); strings.TrimSpace(out) != c.replaced {
			t.Errorf("%s joining the replaced value gave %q, want %q",
				c.src, strings.TrimSpace(out), c.replaced)
		}

		fresh := permissive()
		fresh.KeyedLiteralAppendJoinsTheReplacedValue = No
		if out, _ := run(t, c.src, withSem(fresh)); strings.TrimSpace(out) != c.built {
			t.Errorf("%s joining what the literal built gave %q, want %q",
				c.src, strings.TrimSpace(out), c.built)
		}
	}
}

// Unanswered, the axis refuses and names itself — but only where the two
// answers differ. A key the name was not holding has one empty value under
// both readings, and an *appending* literal never replaces a table at all, so
// neither reaches the question in a core that has chosen no shell.
func TestAKeyedLiteralAppendAsksOnlyWhereItMatters(t *testing.T) {
	none := permissive()
	for _, src := range []string{
		`typeset -A m; m=([k]+=x); echo "[${m[k]}]"`,
		`typeset -A m; m[k]=v; m+=([k]+=x); echo "[${m[k]}]"`,
		`typeset -A m; m[j]=v; m=([k]+=x); echo "[${m[k]}]"`,
	} {
		out, _ := run(t, src, withSem(none))
		if strings.Contains(out, "disagree") {
			t.Errorf("%s named the axis, want it answered without one: %q", src, out)
		}
	}
	out, _ := run(t, `typeset -A m; m[k]=v; m=([k]+=x); echo "[${m[k]}]"`, withSem(none))
	if !strings.Contains(out, "disagree") {
		t.Errorf("a replaced key gave %q, want the axis named", out)
	}
}

// The subscript ends at the first `]` an `=` or a `+=` follows, so a `]+=`
// standing inside the *value* a plain element already delimited is text.
// Searching for the two spellings separately and taking whichever matched
// read `[a]=b]+=c` as an append of `c` to the element keyed `a]=b`.
func TestTheFirstTerminatorWinsInALiteralElement(t *testing.T) {
	out, _ := runArray(t, `a=([1]=b]+=c); echo "1=[${a[1]}] n=${#a[@]}"`)
	if strings.TrimSpace(out) != "1=[b]+=c] n=1" {
		t.Errorf("got %q, want the later `]+=` kept as part of the value", strings.TrimSpace(out))
	}
}

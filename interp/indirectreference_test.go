// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runIndirect is run() with the grammar an indirection and a subscript need,
// and nothing else: the default test semantics already answer
// IndirectionYieldsName with No, which is the dialect these rows are about.
//
// ParamBangNameContinues carries the second half of that grammar. Where an
// indirection *begins* is a dialect answer too, and a digit is on the far side
// of that split — `${!1}` is an indirection through the first positional under
// the value set here and the parameter `!` under the empty one. These rows
// name the positional, so the flag has to say a digit carries the name on.
func runIndirectRef(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamIndirection = true
		d.ParamBangNameContinues = "0123456789#?@*["
	}, nil)
}

// `${!v}` reads what `v` came to as the **parameter reference** it spells, so
// a positional parameter, a special parameter and a subscripted element all
// resolve. Reading it as a plain variable name found none of them and answered
// the empty string at status 0 (#3213).
//
// Measured 2026-09-16 on bash 5.3.20 and bash 3.2.57, which agree on every row
// here; ksh93u+ yields the name it was written on and zsh, dash and BusyBox
// ash have no such expansion. See interp/indirectread.go for the panel.
//
// The value is printed one field at a time inside brackets, because for four
// of these rows the *field count* is the answer and a joined string would
// render the same either way.
func TestAnIndirectionResolvesAParameterReference(t *testing.T) {
	const preamble = `set -- p q r; a=(A B C); `
	for _, tc := range []struct{ name, spec, want string }{
		{"the positional list", "@", "<p><q><r>"},
		{"the joined positional list", "*", "<p q r>"},
		{"the count of parameters", "#", "<3>"},
		{"one positional parameter", "1", "<p>"},
		{"a later positional parameter", "2", "<q>"},
		{"a positional parameter nobody passed", "9", "<>"},
		{"the last status", "?", "<0>"},
		{"one element of an array", "a[1]", "<B>"},
		{"the whole of an array", "a[@]", "<A><B><C>"},
		{"the whole of an array, joined", "a[*]", "<A B C>"},
		{"a bare array name, which is its first element", "a", "<A>"},
		{"an ordinary name that is not set", "nosuch", "<>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := preamble + `v='` + tc.spec + `'; printf '<%s>' "${!v}"`
			out, st := runIndirectRef(t, src)
			if out != tc.want {
				t.Errorf("${!v} with v=%q: got %q, want %q", tc.spec, out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// The field count on its own, because the row above prints it and a reader
// could take the brackets for decoration. `@` and `[@]` keep one field per
// parameter and per element; `*` and `[*]` are one joined field. A scalar read
// of the target answered one field for all four, which is where an element
// holding a space goes missing.
func TestAnIndirectionKeepsTheFieldsOfAList(t *testing.T) {
	for _, tc := range []struct{ spec, want string }{
		{"@", "3"},
		{"a[@]", "3"},
		{"*", "1"},
		{"a[*]", "1"},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			src := `set -- p q r; a=(A B C); v='` + tc.spec +
				`'; set -- "${!v}"; echo "$#"`
			out, _ := runIndirectRef(t, src)
			if strings.TrimSpace(out) != tc.want {
				t.Errorf("${!v} with v=%q: %s fields, want %s", tc.spec, strings.TrimSpace(out), tc.want)
			}
		})
	}
}

// A subscript inside a resolved text is live arithmetic, and it is evaluated
// **once per expansion** — the guarantee #3104 wrote down for a written
// subscript, which this road now shares rather than having a second answer.
//
// The counter is the instrument: a read that happened twice leaves it at two
// and names the wrong element, which is a wrong value at status 0 rather than
// anything a status could show. Measured on bash 5.3.20 and bash 3.2.57 alike,
// `a=(x y z); i=0; v='a[i++]'` gives `x` with `i` at 1, and two spellings in
// one word give `x` and `y` with `i` at 2.
func TestAnIndirectionReadsItsTargetOnce(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"one expansion moves it one place",
			`a=(x y z); i=0; v='a[i++]'; printf '[%s] i=%s' "${!v}" "$i"`,
			"[x] i=1",
		},
		{
			"two expansions move it two",
			`a=(x y z); i=0; v='a[i++]'; printf '[%s][%s] i=%s' "${!v}" "${!v}" "$i"`,
			"[x][y] i=2",
		},
		{
			"a conditional that does not fire still reads once",
			`a=(x y z); i=0; v='a[i++]'; printf '[%s] i=%s' "${!v-D}" "$i"`,
			"[x] i=1",
		},
		{
			"and an operator on the value reads once",
			`a=(xq y z); i=0; v='a[i++]'; printf '[%s] i=%s' "${!v%q}" "$i"`,
			"[x] i=1",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runIndirectRef(t, tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// `set -u` is about the read that produced the value, and for an indirection
// that is the **second** one. A set `v` naming an unset parameter expanded to
// the empty string at status 0 here, where both bash columns end the script —
// which is exactly what the option is written to prevent (#3214).
//
// `echo after` is in every row because the status alone cannot show it: a
// shell that wrote the sentence and carried on would look the same without it.
func TestNounsetReachesTheTargetOfAnIndirection(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a name naming an unset name",
			`set -u; v=nope; echo "[${!v}]"; echo after`,
			"!v: parameter not set",
		},
		{
			"a name naming a positional parameter nobody passed",
			`set -u; q=1; echo "[${!q}]"; echo after`,
			"!q: parameter not set",
		},
		{
			"a subscript that named no element",
			`a=(x y z); set -u; echo "[${!a[9]}]"; echo after`,
			"!a[9]: parameter not set",
		},
		{
			"a name naming an element that is not there",
			`a=(x y z); set -u; w='a[9]'; echo "[${!w}]"; echo after`,
			"!w: parameter not set",
		},
		{
			"and the outer name unset is still refused",
			`set -u; echo "[${!v}]"; echo after`,
			"!v: parameter not set",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runIndirectRef(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q in it", out, tc.want)
			}
			if strings.Contains(out, "after") {
				t.Errorf("got %q, want the script abandoned", out)
			}
			if st == 0 {
				t.Error("status = 0, want a refusal")
			}
		})
	}
}

// What the refusal must *not* reach, and each row is a way it could have been
// made too wide. The operator guard is the one every other expansion gets, and
// a target that names the positional list is not an unset parameter — `"$@"`
// with none is empty and quiet, and the *outer* name being `q` or `v` says
// nothing about that.
func TestNounsetLeavesAnIndirectionThatSuppliesAValue(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a default", `set -u; v=nope; echo "[${!v-D}]"; echo after`, "[D]\nafter\n"},
		{"a colon default", `set -u; v=nope; echo "[${!v:-D}]"; echo after`, "[D]\nafter\n"},
		{"an alternate", `set -u; v=nope; echo "[${!v+S}]"; echo after`, "[]\nafter\n"},
		{"the positional list with none", `set -u; v='@'; echo "[${!v}]"; echo after`, "[]\nafter\n"},
		{"the count of no parameters", `set -u; v='#'; echo "[${!v}]"; echo after`, "[0]\nafter\n"},
		{"a target that is set", `set -u; t=V; v=t; echo "[${!v}]"; echo after`, "[V]\nafter\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runIndirectRef(t, tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0", st)
			}
		})
	}
}

// The `!` goes with the indirection being **taken**, which
// Semantics.IndirectionYieldsName already decides — so it is derived and not a
// second axis, and there is no column that could disagree: the other four
// shells have no `${!…}` at all.
//
// Measured 2026-09-16 under `set -u` from a script file: bash 5.3.20 and bash
// 3.2.57 write `!v: unbound variable` and `!a[9]: unbound variable`, and
// ksh93u+ 2012 — where the expansion is the name — writes `v: parameter not
// set` and `nosucharr[9]: parameter not set` with no sigil at all.
func TestAnIndirectionsRefusalWearsTheSigilOnlyWhereItIndirects(t *testing.T) {
	for _, tc := range []struct {
		name  string
		yield Answer
		src   string
		want  string
	}{
		{"taken, a plain name", No, `set -u; echo "[${!v}]"`, "!v: parameter not set"},
		{"taken, with a subscript", No, `set -u; echo "[${!nosucharr[9]}]"`, "!nosucharr[9]: parameter not set"},
		{"yielding the name, a plain name", Yes, `set -u; echo "[${!v}]"`, "v: parameter not set"},
		{"yielding the name, with a subscript", Yes, `set -u; echo "[${!nosucharr[9]}]"`, "nosucharr[9]: parameter not set"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := permissive()
			arraySemantics(&sem)
			sem.IndirectionYieldsName = tc.yield
			out, _ := runGrammar(t, tc.src, func(d *syntax.Dialect) {
				d.ParamIndirection = true
			}, withSem(sem))
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q in it", out, tc.want)
			}
			if strings.Contains(out, "!"+tc.want) {
				t.Errorf("got %q, which wears a sigil it should not", out)
			}
		})
	}
}

// `${!v?word}` names the same subject, which is what says the `!` belongs to
// the parameter rather than to the `set -u` sentence. Measured, `${!v?msg}` is
// `!v: msg` and `${!a[9]?msg}` is `!a[9]: msg` in both bash columns.
func TestAnIndirectionsErrorOperatorNamesTheSameSubject(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a plain name", `v=nope; echo "[${!v?missing}]"; echo after`, "!v: missing"},
		{"a name holding a reference", `a=(x y z); w='a[9]'; echo "[${!w?missing}]"; echo after`, "!w: missing"},
		{"a written subscript", `a=(x y z); echo "[${!a[9]?missing}]"; echo after`, "!a[9]: missing"},
		{"and a positional the indirection reached", `q=1; echo "[${!q?missing}]"; echo after`, "!q: missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runIndirectRef(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("got %q, want %q in it", out, tc.want)
			}
			if strings.Contains(out, "after") {
				t.Errorf("got %q, want the script abandoned", out)
			}
			if st == 0 {
				t.Error("status = 0, want a refusal")
			}
		})
	}
}

// A positional parameter standing as the *outer* name, which is the one shape
// where the refusal's name tests could have been asked of the wrong parameter:
// the subject is the whole `!1` and not the `$1` a bare positional wears, and
// the sigil branch reads the written name rather than the one the text
// resolved to. Measured 2026-09-16, `set -- nope; set -u; echo "${!1}"` is
// `!1: unbound variable` in bash 5.3.20 and bash 3.2.57, and so is the same
// line with no parameters passed at all.
func TestAnIndirectionThroughAPositionalNameIsStillTheIndirectionsRefusal(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"the positional holds an unset name", `set -- nope; set -u; echo "[${!1}]"; echo after`},
		{"the positional itself is unset", `set --; set -u; echo "[${!1}]"; echo after`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runIndirectRef(t, tc.src)
			if !strings.Contains(out, "!1: parameter not set") {
				t.Errorf("got %q, want %q in it", out, "!1: parameter not set")
			}
			if strings.Contains(out, "$1") {
				t.Errorf("got %q, which wears the sigil a bare positional gets", out)
			}
			if strings.Contains(out, "after") {
				t.Errorf("got %q, want the script abandoned", out)
			}
			if st == 0 {
				t.Error("status = 0, want a refusal")
			}
		})
	}
}

// The refusal is written **once**. It is one parameter however many reads it
// took to fail, and the outer check running beside the target's wrote the
// sentence twice for `set -u; ${!v}` with nothing set — which no shell does
// and which a Contains assertion cannot see. The count is the assertion for
// that reason.
func TestAnIndirectionIsRefusedOnlyOnce(t *testing.T) {
	for _, src := range []string{
		`set -u; echo "[${!v}]"`,
		`set -u; v=nope; echo "[${!v}]"`,
		`set --; set -u; echo "[${!1}]"`,
	} {
		out, _ := runIndirectRef(t, src)
		if n := strings.Count(out, "parameter not set"); n != 1 {
			t.Errorf("%s: refused %d times, want 1 — %q", src, n, out)
		}
	}
}

// A command substitution written into a resolved subscript costs this road no
// more than it costs the written one. The two are printed side by side rather
// than counted against a number, because the number is not this change's:
// measured 2026-09-16, bash 5.3.20 runs it **once** for both spellings and
// this shell runs it once per reader for both, which is a defect of its own
// (#3240). What belongs here is that the indirection adds nothing — asking
// wholeArrayIndex about the parsed node instead of about the text added one
// more run, and the arithmetic rows above cannot see it.
func TestAnIndirectionAddsNoReadOfItsSubscript(t *testing.T) {
	const sub = `$(printf . >&2; echo 1)`
	written, _ := runIndirectRef(t, `a=(x y z); echo "[${a[`+sub+`]}]"`)
	indirect, _ := runIndirectRef(t, `a=(x y z); d='a[`+sub+`]'; echo "[${!d}]"`)
	if !strings.Contains(written, "[y]") || !strings.Contains(indirect, "[y]") {
		t.Fatalf("written %q and indirect %q, want both to reach the second element", written, indirect)
	}
	if w, i := strings.Count(written, "."), strings.Count(indirect, "."); w != i {
		t.Errorf("the written subscript ran %d times and the indirect one %d, want the same", w, i)
	}
}

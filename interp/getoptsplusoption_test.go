// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A word beginning with `+` read as an option word (#2946).
//
// One dialect has it and six read `+a` as an operand that ends the scan.
// Where it exists the two spellings are one option with two senses — `+a` is
// how several real tools spell "turn this off" — and the sign is the only
// thing that tells a script which it was given, so the name holds `+a`.

func plusSem(a Answer) Semantics {
	s := getoptsSem()
	s.GetoptsTakesAPlusPrefixedOption = a
	return s
}

func plusDiag() *Diagnostics {
	// The sign as the wording's second verb, which is the shape the dialect
	// that reads both signs uses.
	return &Diagnostics{
		GetoptsBadOption:       "bad option: %[2]s%[1]s",
		GetoptsMissingArgument: "argument expected after %[2]s%[1]s option",
	}
}

// The axis itself: the same word is an option in one answer and the operand
// that ends the scan in the other.
func TestGetoptsTakesAPlusPrefixedOptionIsAnAxis(t *testing.T) {
	const src = `set -- +a rest; o=INIT
getopts "a" o
echo "st=$? [$o] ind=$OPTIND"`
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		// bash 5.3.20, bash as `sh`, bash 3.2.57, ksh93u+, dash 0.5.12 and
		// BusyBox ash 1.37.0: `+a` is an operand, so the scan is over.
		{"an operand", No, "st=1 [?] ind=1\n"},
		// zsh 5.9.2, measured 2026-09-17 over a script file under `env -i
		// PATH=/usr/bin:/bin LC_ALL=C` with stdin on /dev/null.
		{"an option word", Yes, "st=0 [+a] ind=2\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := plusSem(tc.answer)
			out, _ := run(t, src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, plusDiag() })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// Everything else about the word is the scan it already is: a cluster is a
// cluster, an argument is taken in both spellings, and the two signs mix in
// one command line.
func TestGetoptsPlusPrefixedOptionScansLikeAnyOther(t *testing.T) {
	for _, tc := range []struct{ name, params, spec, want string }{
		{"a cluster", `set -- +ab`, `ab`, "[+a][+b] ind=2\n"},
		{"both signs in one line", `set -- +a -b`, `ab`, "[+a][b] ind=3\n"},
		{"an argument in the next word", `set -- +a v`, `a:`, "[+a=v] ind=3\n"},
		{"an argument in the same word", `set -- +av`, `a:`, "[+a=v] ind=2\n"},
		{"a `-` word after a `+` one", `set -- +a -a`, `a`, "[+a][a] ind=3\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := plusSem(Yes)
			src := tc.params + "\n" + `while getopts '` + tc.spec + `' o; do
  if [ "${OPTARG-}" = "" ]; then printf '[%s]' "$o"; else printf '[%s=%s]' "$o" "$OPTARG"; fi
done
printf ' ind=%s\n' "$OPTIND"`
			out, _ := run(t, src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, plusDiag() })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The sign reaches the complaint and OPTARG as well as the name, which is
// what makes it *the letter as reported* rather than a flag on the scan.
func TestGetoptsPlusPrefixedOptionCarriesTheSignIntoWhatItReports(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"an unknown letter",
			`set -- +z; getopts "a" o; echo "[$o]"`,
			"sh: bad option: +z\n[?]\n",
		},
		{
			"a missing argument",
			`set -- +a; getopts "a:" o; echo "[$o]"`,
			"sh: argument expected after +a option\n[?]\n",
		},
		{
			"an unknown letter, silently",
			`set -- +z; getopts ":a" o; echo "[$o][$OPTARG]"`,
			"[?][+z]\n",
		},
		{
			"a missing argument, silently",
			`set -- +a; getopts ":a:" o; echo "[$o][$OPTARG]"`,
			"[:][+a]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := plusSem(Yes)
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, plusDiag() })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The sign and the letter are two verbs and not one string, which only a
// word whose *letter* is a sign can show: `-+` is a `-` word naming `+`, and
// `++` is a `+` word naming `+`. Gluing them together would write one of
// these as the other.
func TestGetoptsSignAndLetterAreSeparateVerbs(t *testing.T) {
	for _, tc := range []struct{ words, want string }{
		{`-+`, "sh: bad option: -+\n"},
		{`++`, "sh: bad option: ++\n"},
	} {
		t.Run(tc.words, func(t *testing.T) {
			sem := plusSem(Yes)
			out, _ := run(t, `set -- `+tc.words+`; getopts "a" o`,
				func(r *Runner) { r.Semantics, r.Diagnostics = &sem, plusDiag() })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// Two words that are not options under either answer, and the second is the
// one worth pinning: `++` is **not** the `--` that ends the options — it is
// the option `+`, which no ordinary option string has.
func TestGetoptsPlusIsNotAnEndOfOptionsWord(t *testing.T) {
	// The `?` in the rows that end the scan is
	// Semantics.GetoptsEndOfOptionsNamesIt, which this vector answers yes; the
	// dialect that reads a `+` word as an option answers it no and leaves the
	// name alone, which is a different axis and #3182's.
	for _, tc := range []struct{ name, params, want string }{
		{"a lone plus", `set -- + a`, "st=1 [?] ind=1\n"},
		{"a lone dash", `set -- - a`, "st=1 [?] ind=1\n"},
		{"a double plus", `set -- ++ a`, "st=0 [?] ind=2\n"},
		{"a double dash", `set -- -- +a`, "st=1 [?] ind=2\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := plusSem(Yes)
			out, _ := run(t, tc.params+"\no=INIT\ngetopts \"a\" o 2>/dev/null\necho \"st=$? [$o] ind=$OPTIND\"",
				func(r *Runner) { r.Semantics, r.Diagnostics = &sem, plusDiag() })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// An unanswered axis is reported rather than guessed — but only where a `+`
// word is the one the scan is at. An ordinary scan over `-` words and
// ordinary operands reaches no question at all, which is every `getopts` a
// portable script writes.
func TestGetoptsPlusPrefixedOptionIsAskedOnlyAtAPlusWord(t *testing.T) {
	const axis = "a `+`-prefixed word read as an option rather than as an operand"
	t.Run("a plus word", func(t *testing.T) {
		sem := plusSem(Unspecified)
		out, _ := run(t, `set -- +a; getopts "a" o`,
			func(r *Runner) { r.Semantics, r.Diagnostics = &sem, plusDiag() })
		if !strings.Contains(out, axis) {
			t.Fatalf("got %q, want it to carry %q", out, axis)
		}
	})
	for _, tc := range []struct{ name, src string }{
		{"dash words and an operand", `set -- -a -b rest; while getopts "ab" o; do printf '[%s]' "$o"; done; echo " ind=$OPTIND"`},
		{"nothing to scan", `set --; getopts "a" o; echo "[$o]"`},
		{"a lone plus", `set -- +; getopts "a" o; echo "[$o]"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := plusSem(Unspecified)
			out, _ := run(t, tc.src, func(r *Runner) { r.Semantics, r.Diagnostics = &sem, plusDiag() })
			if strings.Contains(out, axis) {
				t.Errorf("got %q, want no question asked", out)
			}
		})
	}
}

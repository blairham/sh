// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// `@A` and `@K` over a keyed table spell a key the way the declaration
// listing does, because `@A` *is* that listing — the statement that would
// reproduce the name. Both routes printed the key raw, so a key holding a
// quote produced a listing no shell can read back (#2298).

func transformKeySem() Semantics {
	s := testSemantics()
	s.DeclareListing = DeclareListingClustered
	s.DeclareValueQuoting = ListingQuoteAlwaysDouble
	return s
}

// runTransformKey is runGrammar with the one flag the family needs and the
// listing the transformation is measured against.
func runTransformKey(t *testing.T, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamTransformations = true
	}, withSem(transformKeySem()))
}

func TestTheKeyTransformsQuoteAKeyTheListingQuotes(t *testing.T) {
	for _, tc := range []struct{ name, key, listing, transform string }{
		{"a blank", "a b", `["a b"]="v"`, `"a b" "v" `},
		{"a double quote", `"`, `["\""]="v"`, `"\"" "v" `},
		{"a backslash", `\`, `["\\"]="v"`, `"\\" "v" `},
		{"a backquote", "`", "[\"\\`\"]=\"v\"", "\"\\`\" \"v\" "},
		{"a dollar", "$", `["\$"]="v"`, `"\$" "v" `},
		{"the whole-array subscript", "@", `["@"]="v"`, `"@" "v" `},
		{"a closing bracket", "]", `["]"]="v"`, `"]" "v" `},
		// A key needing nothing is bare in both, which is what says the
		// quoting is the listing's rule and not an always.
		{"a plain key", "plain", `[plain]="v"`, `plain "v" `},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Through a parameter, so the subscript the script writes holds
			// no quotes of its own.
			setup := "k=" + shellSingleQuoted(tc.key) + "; typeset -A m; m[$k]=v; "
			out, _ := runTransformKey(t, setup+`typeset -p m`)
			want := "declare -A m=(" + tc.listing + " )"
			if got := strings.TrimSpace(out); got != want {
				t.Fatalf("the listing this is measured against: got %q, want %q", got, want)
			}
			// `echo` joins the words with a blank, and @A over a whole
			// array is three of them — the command word, the letters and
			// the assignment.
			out, _ = runTransformKey(t, setup+`echo "${m[@]@A}"`)
			if got := strings.TrimSpace(out); got != "declare -A "+want[len("declare -A "):] {
				t.Errorf("@A: got %q, want %q", got, want)
			}
			out, _ = runTransformKey(t, setup+`printf '[%s]\n' "${m[@]@K}"`)
			if got := strings.TrimSpace(out); got != "["+tc.transform+"]" {
				t.Errorf("@K: got %q, want %q", got, "["+tc.transform+"]")
			}
		})
	}
}

// `@k` is the other half of the pair and is *not* this: it answers the key as
// itself, one word per key and per value, so the caller can read the table
// back with `read` or a loop. Measured, and the reason the quoting is not
// simply applied to every key transform.
func TestTheLowercaseKeyTransformLeavesAKeyAlone(t *testing.T) {
	src := `k='a b'; typeset -A m; m[$k]=v; printf '[%s]' "${m[@]@k}"; echo`
	out, _ := runTransformKey(t, src)
	if got := strings.TrimSpace(out); got != "[a b][v]" {
		t.Errorf("@k: got %q, want %q", got, "[a b][v]")
	}
}

// An indexed array's keys are its subscripts, and a subscript is bare under
// every rule — the row that says the change did not reach the other container.
func TestTheKeyTransformsLeaveAnIndexedSubscriptBare(t *testing.T) {
	src := `a=(one "t w"); echo "${a[@]@A}"; printf '[%s]\n' "${a[@]@K}"`
	out, _ := runTransformKey(t, src)
	want := "declare -a a=([0]=\"one\" [1]=\"t w\")\n[0 \"one\" 1 \"t w\"]"
	if got := strings.TrimSpace(out); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

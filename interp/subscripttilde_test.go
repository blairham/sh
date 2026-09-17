// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Whether the leading `~` of an associative subscript becomes the home
// directory before the text becomes a key (#2298).

func subscriptTildeSem(tilde Answer) Semantics {
	s := testSemantics()
	s.DeclareListing = DeclareListingClustered
	s.DeclareValueQuoting = ListingQuoteAlwaysDouble
	s.SubscriptKeyExpandsALeadingTilde = tilde
	// Two axes the rows below would otherwise reach with no answer, which
	// would measure three questions at once: how a listing spells a key
	// holding a `~`, and whether a quoted subscript is a quoting context —
	// the rows with quotes in them are about the *tilde* surviving the quote
	// and not about which characters the key keeps.
	s.ListedTildeIsBareWhereItCannotExpand = No
	s.SubscriptIsAQuotingContext = Yes
	return s
}

func runSubscriptTilde(t *testing.T, src string, tilde Answer) string {
	t.Helper()
	out, _ := run(t, "HOME=/h; "+src, withSem(subscriptTildeSem(tilde)))
	return strings.TrimSpace(out)
}

// The axis is asked only where the two readings differ, which is a subscript
// opening with an unquoted `~`: every other spelling is the same key either
// way, and the rows for those are the guards below.
func TestASubscriptsLeadingTildeIsExpanded(t *testing.T) {
	for _, tc := range []struct{ name, sub, expanded, taken string }{
		{"a tilde and a path", "~/k", `[/h/k]="v"`, `["~/k"]="v"`},
		{"a tilde alone", "~", `[/h]="v"`, `["~"]="v"`},
		// Quoting takes it away, and so does any position but the first —
		// the same three conditions a command word's tilde has. These are
		// the same key under both answers.
		{"double quoted", `"~/k"`, `["~/k"]="v"`, `["~/k"]="v"`},
		{"backslashed", `\~/k`, `["~/k"]="v"`, `["~/k"]="v"`},
		{"not at the front", "x~/k", `["x~/k"]="v"`, `["x~/k"]="v"`},
		// A subscript is not an assignment's value: the `:` positions a
		// value's tilde also expands at are content here, in every column.
		{"after a colon", "a:~/k", `["a:~/k"]="v"`, `["a:~/k"]="v"`},
		// And a tilde a *substitution* produced is text: the expansion ran
		// before there was a tilde to see.
		{"out of a parameter", "$t", `["~/k"]="v"`, `["~/k"]="v"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `t='~/k'; typeset -A m; m[` + tc.sub + `]=v; typeset -p m`
			want := "declare -A m=(" + tc.expanded + " )"
			if got := runSubscriptTilde(t, src, Yes); got != want {
				t.Errorf("expanded: got %q, want %q", got, want)
			}
			want = "declare -A m=(" + tc.taken + " )"
			if got := runSubscriptTilde(t, src, No); got != want {
				t.Errorf("taken: got %q, want %q", got, want)
			}
		})
	}
}

// A *read* takes the same rule, which is what makes the expanding answer
// self-consistent rather than only surprising: the element stored under the
// path is the element `${m[~/k]}` finds.
func TestASubscriptsLeadingTildeIsExpandedOnARead(t *testing.T) {
	src := `typeset -A m; m[/h/k]=stored; echo "[${m[~/k]}]"`
	if got := runSubscriptTilde(t, src, Yes); got != "[stored]" {
		t.Errorf("expanded: got %q, want %q", got, "[stored]")
	}
	if got := runSubscriptTilde(t, src, No); got != "[]" {
		t.Errorf("taken: got %q, want %q", got, "[]")
	}
}

// And every route that reaches an element through a builtin's *operand* takes
// it, because the subscript arrives there as text and each of those routes
// would otherwise answer a different element from the one the same subscript
// names in a word. `printf -v` and `${!ref}` are two more of them and are
// graded in dialect/bash, where the grammar has both.
func TestAnOperandSubscriptsLeadingTildeIsExpanded(t *testing.T) {
	for _, tc := range []struct{ name, src, expanded, taken string }{
		{
			"unset",
			`typeset -A m; m[/h/k]=v; unset 'm[~/k]'; typeset -p m`,
			`declare -A m=()`, `declare -A m=([/h/k]="v" )`,
		},
		{
			"read",
			`typeset -A m; m[x]=y; read 'm[~/k]' <<< "RD"; typeset -p m`,
			`declare -A m=([/h/k]="RD" [x]="y" )`, `declare -A m=([x]="y" ["~/k"]="RD" )`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runSubscriptTilde(t, tc.src, Yes); got != tc.expanded {
				t.Errorf("expanded: got %q, want %q", got, tc.expanded)
			}
			if got := runSubscriptTilde(t, tc.src, No); got != tc.taken {
				t.Errorf("taken: got %q, want %q", got, tc.taken)
			}
		})
	}
}

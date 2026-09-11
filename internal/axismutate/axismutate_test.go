// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axismutate

import "testing"

type policy uint8

type inner struct {
	Login string
	Count int
}

type vector struct {
	Answer   policy
	Enabled  bool
	Wording  string
	Nested   inner
	unexport int //nolint:unused // present so the walk has something it must refuse
}

func TestApplyWritesEachKind(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		spec string
		want vector
	}{
		{"Answer=2", vector{Answer: 2}},
		{"Enabled=true", vector{Enabled: true}},
		{"Wording=bad substitution", vector{Wording: "bad substitution"}},
		{"Nested.Login=-l", vector{Nested: inner{Login: "-l"}}},
		{"Nested.Count=7", vector{Nested: inner{Count: 7}}},
	} {
		t.Run(tc.spec, func(t *testing.T) {
			t.Parallel()
			spec, err := ParseSpec(tc.spec)
			if err != nil {
				t.Fatal(err)
			}
			var got vector
			if err := Apply(&got, spec); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("%s left %+v, want %+v", tc.spec, got, tc.want)
			}
		})
	}
}

// TestAValueWithAnEqualsSignSurvives: a string axis may hold anything, and a
// diagnostic's wording is the one most likely to. Cutting on the last `=`
// would truncate it.
func TestAValueWithAnEqualsSignSurvives(t *testing.T) {
	t.Parallel()
	spec, err := ParseSpec("Wording=a=b")
	if err != nil {
		t.Fatal(err)
	}
	var got vector
	if err := Apply(&got, spec); err != nil {
		t.Fatal(err)
	}
	if got.Wording != "a=b" {
		t.Fatalf("the value came through as %q", got.Wording)
	}
}

// TestWhatItRefuses is the discipline the sweep rests on. Every one of these
// would otherwise leave the vector untouched, and an untouched vector answers
// the sweep's question with "nothing objected" — a bug filed against a field
// nobody moved.
func TestWhatItRefuses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		spec Spec
		into any
	}{
		{"a field that is not there", Spec{Path: "Missing", Value: "1"}, &vector{}},
		{"a path through something that is not a struct", Spec{Path: "Wording.Login", Value: "x"}, &vector{}},
		{"an unexported field", Spec{Path: "unexport", Value: "1"}, &vector{}},
		{"a value that is not a number", Spec{Path: "Answer", Value: "yes"}, &vector{}},
		{"a value that does not fit", Spec{Path: "Answer", Value: "999"}, &vector{}},
		{"a value that is not a bool", Spec{Path: "Enabled", Value: "maybe"}, &vector{}},
		{"something that is not a struct pointer", Spec{Path: "Answer", Value: "1"}, vector{}},
		{"a nil pointer", Spec{Path: "Answer", Value: "1"}, (*vector)(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := Apply(tc.into, tc.spec); err == nil {
				t.Fatalf("%s was accepted", tc.name)
			}
		})
	}
}

func TestParseSpecWantsAName(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"", "novalue", "=1"} {
		if _, err := ParseSpec(bad); err == nil {
			t.Errorf("%q parsed as a mutation", bad)
		}
	}
}

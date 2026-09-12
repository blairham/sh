// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package axissweep

import (
	"reflect"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/axismutate"
	"github.com/blairham/sh/interp"
)

func init() { SetModuleRoot("../..") }

// TestEveryAxisCanBeMoved is the totality claim, and it is the whole reason
// this package enumerates rather than lists.
//
// #1416 and #1808 were both a guarantee that was checked rather than
// enumerated, and so was only ever as wide as the set it walked. The failure
// mode here is the same and worse: a field the sweep cannot move is reported
// as one nothing objected to, which is a bug filed against an axis that was
// never touched. So an axis whose type nothing here knows how to write has to
// break this test on the commit that adds it, rather than quietly leaving the
// sweep narrower than the struct.
func TestEveryAxisCanBeMoved(t *testing.T) {
	t.Parallel()
	fields, err := Fields(reflect.TypeOf(interp.Semantics{}))
	if err != nil {
		t.Fatalf("enumerating the vector: %v", err)
	}
	if len(fields) < 300 {
		t.Fatalf("only %d axes enumerated; the vector is much larger than that", len(fields))
	}
	for _, vector := range []struct {
		name string
		sem  interp.Semantics
	}{
		{"bash", bash.Semantics()},
		{"zsh", zsh.Semantics()},
		{"ksh", ksh.Semantics()},
		{"dash", dash.Semantics()},
		{"core", interp.CoreSemantics()},
		{"posix", interp.PosixSemantics()},
	} {
		held := reflect.ValueOf(vector.sem)
		for _, f := range fields {
			cur, err := At(held, f.Path)
			if err != nil {
				t.Fatalf("%s: %v", vector.name, err)
			}
			values, err := Values(f, cur)
			if err != nil {
				t.Errorf("%s: %v", vector.name, err)
				continue
			}
			if len(values) == 0 {
				t.Errorf("%s: %s has no value it does not already hold, so no flip can be asked about it", vector.name, f.Path)
			}
			for _, v := range values {
				moved := vector.sem
				spec := axismutate.Spec{Path: f.Path, Value: v.Literal}
				if err := axismutate.Apply(&moved, spec); err != nil {
					t.Errorf("%s: %v", vector.name, err)
					continue
				}
				after, err := At(reflect.ValueOf(moved), f.Path)
				if err != nil {
					t.Fatalf("%s: %v", vector.name, err)
				}
				if literalOf(after) == literalOf(cur) {
					t.Errorf("%s: %s = %s left the axis where it was", vector.name, f.Path, v.Name)
				}
			}
		}
	}
}

// TestAnAxisTypeWithNoConstantsIsAnError is the other half of the same claim.
// A named integer type whose values the source does not declare cannot be
// enumerated, and the sweep has to say so rather than sweep it with nothing.
func TestAnAxisTypeWithNoConstantsIsAnError(t *testing.T) {
	t.Parallel()
	f := Field{Path: "Invented", Type: "NoSuchPolicyType", Kind: reflect.Int}
	if _, err := Values(f, reflect.ValueOf(0)); err == nil {
		t.Fatal("a type with no declared constants was swept anyway")
	}
}

// TestAKindTheSweepCannotWriteIsAnError covers the shape a future axis is
// most likely to arrive in: a slice, a map or a pointer, none of which this
// knows how to move.
func TestAKindTheSweepCannotWriteIsAnError(t *testing.T) {
	t.Parallel()
	type unreachable struct{ Spellings []string }
	if _, err := Fields(reflect.TypeOf(unreachable{})); err != nil {
		t.Fatalf("enumerating a slice field should reach Values, not fail here: %v", err)
	}
	f := Field{Path: "Spellings", Type: "[]string", Kind: reflect.Slice}
	if _, err := Values(f, reflect.ValueOf([]string{"a"})); err == nil {
		t.Fatal("a slice axis was swept anyway")
	}
}

// TestConstantsAreReadFromTheSource pins the scan itself. Answer's three
// states are declared in one iota block with the type on the first spec only,
// which is how every answer type in the file is written — a scan that lost
// the type after the first line would find one constant and call the other
// two absent.
func TestConstantsAreReadFromTheSource(t *testing.T) {
	t.Parallel()
	consts, err := typeConstants()
	if err != nil {
		t.Fatal(err)
	}
	want := []Value{
		{Name: "Unspecified", Literal: "0", Unspecified: true},
		{Name: "Yes", Literal: "1"},
		{Name: "No", Literal: "2"},
	}
	if got := consts["Answer"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("Answer's values read as %v, want %v", got, want)
	}
}

// TestNestedVectorsAreSweptFieldByField: StartupFileOptions is held by value
// and its fields are axes in their own right. Sweeping the struct as one unit
// could not say which of them nothing objected to.
func TestNestedVectorsAreSweptFieldByField(t *testing.T) {
	t.Parallel()
	fields, err := Fields(reflect.TypeOf(interp.Semantics{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"StartupFileOptions.Login", "VersionOption.Spellings", "VersionOption.Status"} {
		found := false
		for _, f := range fields {
			if f.Path == want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s is not swept; a struct the vector holds by value was taken as one field", want)
		}
	}
}

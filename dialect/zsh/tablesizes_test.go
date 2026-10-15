// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"io"
	"reflect"
	"testing"

	"github.com/blairham/sh/interp"
)

// TestApplyReservesTheTablesItFills holds zshTables to what Apply registers,
// in both directions.
//
// Under, and a table is grown again at every start, which is the cost the
// reservation exists to take away; the failure says which field to raise.
// Far over, and a start allocates room nobody uses; that bound is loose on
// purpose — twice what the largest emulation fills — so removing a few names
// does not fail it, and removing a module's worth does.
//
// Every emulation a shell can be started in is checked, because two of them
// hide parameters zsh's own has, and the reservation has to hold the largest.
func TestApplyReservesTheTablesItFills(t *testing.T) {
	largest := map[string]int{}
	for _, emulation := range []string{"", "sh", "ksh", "csh"} {
		sem, diag, dl := Semantics(), Diagnostics(), Dialect()
		r := &interp.Runner{
			Stdout: io.Discard, Stderr: io.Discard,
			Semantics: &sem, Diagnostics: &diag, Dialect: &dl,
			Name: "zsh", InvocationEmulation: emulation,
		}
		Apply(r)
		held := reflect.ValueOf(r.TableSizes())
		for i := range held.NumField() {
			name := held.Type().Field(i).Name
			largest[name] = max(largest[name], int(held.Field(i).Int()))
		}
	}
	// The positive control: a fresh runner holds a few dozen names in its
	// parameter table alone, so a measurement that read nothing — every
	// field zero — is a broken instrument and not a fit.
	if largest["Parameters"] < 50 {
		t.Fatalf("Apply left %d parameters, so TableSizes is not measuring the tables", largest["Parameters"])
	}
	reserved := reflect.ValueOf(zshTables)
	for i := range reserved.NumField() {
		name := reserved.Type().Field(i).Name
		want, got := int(reserved.Field(i).Int()), largest[name]
		if got > want {
			t.Errorf("Apply fills %d names into %s and zshTables reserves %d: raise it to at least %d", got, name, want, got)
		}
		if want > 2*max(got, 8) {
			t.Errorf("zshTables reserves %d names for %s and Apply fills %d: lower it", want, name, got)
		}
	}
}

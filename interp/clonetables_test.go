// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"reflect"
	"testing"

	"github.com/blairham/sh/syntax"
)

// sharedTables names every map a clone is allowed to share with its parent.
//
// A positive claim rather than a hole in the check: the test asserts these
// *are* shared, so taking one out of ownTables without taking it out of here
// fails, and adding one here without a reason in ownTables reads as the
// deliberate act it has to be.
var sharedTables = map[string]string{
	"preludeFuncs": "written only while the prelude is sourced and never deleted from, so no subshell can change it",
}

// seedTables gives every map on a Runner an entry.
//
// Seeding is what makes the check able to see anything at all, and it is the
// reason `removed` survived unnoticed: a table copy that keeps a nil map nil
// looks exactly like a table copy that shares one, because the clone's first
// write builds a map its parent's field never pointed at. A parent with
// nothing in a table cannot leak it.
//
// So this is written by hand and exhaustively, and the test below fails on any
// map field it left out — which is the whole mechanism. A table added to
// Runner is nil here, the check says so by name, and somebody has to decide
// whether it belongs in ownTables or in sharedTables.
func seedTables(r *Runner) {
	r.Vars = map[string]string{"seed": "v"}
	r.Arrays = map[string]Array{"seed": {0: "v"}}
	r.AssocArrays = map[string]AssocArray{"seed": {"k": "v"}}
	r.Dynamic = map[string]func(*Runner) string{"seed": func(*Runner) string { return "" }}
	r.DynamicArrays = map[string]func(*Runner) []string{"seed": func(*Runner) []string { return nil }}
	r.DynamicAssocs = map[string]func(*Runner) AssocArray{"seed": func(*Runner) AssocArray { return nil }}
	r.dynamicAssocWriters = map[string]func(*Runner, string, string, bool){
		"seed": func(*Runner, string, string, bool) {},
	}
	r.dynamicWriters = map[string]func(*Runner, string){"seed": func(*Runner, string) {}}
	r.absentElements = map[string]string{"seed": "v"}
	r.absentParams = map[string]string{"seed": "v"}
	r.aliases = map[string]string{"seed": "v"}
	r.assigned = map[string]string{"seed": "v"}
	r.completions = map[string]string{"seed": "v"}
	r.custom = map[string]Builtin{"seed": func(*Runner, context.Context, []string) int { return 0 }}
	r.declaredEmpty = map[string]bool{"seed": true}
	r.declaring = map[string]bool{"seed": true}
	r.disabledBuiltins = map[string]bool{"seed": true}
	r.execFds = map[int]bool{7: true}
	r.exported = map[string]bool{"seed": true}
	r.exportedFuncs = map[string]bool{"seed": true}
	r.extraOptions = map[string]bool{"seed": true}
	r.fds = map[int]any{7: nil}
	r.freezing = map[string]bool{"seed": true}
	r.funcFiles = map[string]string{"seed": "v"}
	r.funcs = map[string]*syntax.FuncDecl{"seed": nil}
	r.hidden = map[string]bool{"seed": true}
	r.inheritedIgnored = map[string]bool{"seed": true}
	r.integer = map[string]bool{"seed": true}
	r.integerBase = map[string]int{"seed": 16}
	r.lowered = map[string]bool{"seed": true}
	r.preludeFuncs = map[string]*syntax.FuncDecl{"seed": nil}
	r.readonly = map[string]bool{"seed": true}
	r.removed = map[string]bool{"seed": true}
	r.tied = map[string]tie{"seed": {}}
	r.traps = map[string]string{"seed": "v"}
	r.unique = map[string]bool{"seed": true}
	r.uppered = map[string]bool{"seed": true}
}

// TestACloneOwnsEveryTable is the check behind ownTables, and it is a check
// rather than a convention because the enumeration it replaced looked
// complete while leaving twenty-two tables shared.
//
// It asks the invariant directly: after clone, no map may still be the
// parent's map. Comparing the map headers rather than mutating one is what
// makes it deterministic — a shared table is a fact about the two runners the
// moment the clone exists, where a leak is only observable once two shells
// happen to write the same name, and a *crash* only once they do it at the
// same instant on two goroutines.
func TestACloneOwnsEveryTable(t *testing.T) {
	parent := newTestRunner(t, &Runner{})
	seedTables(parent)

	pv := reflect.ValueOf(parent).Elem()
	for i := range pv.NumField() {
		f := pv.Type().Field(i)
		if f.Type.Kind() != reflect.Map {
			continue
		}
		if pv.Field(i).IsNil() {
			t.Errorf("%s is a map on Runner that seedTables does not fill in.\n"+
				"Seed it, then decide: ownTables must copy it, or sharedTables must say why "+
				"a subshell may share it. An unseeded table cannot be checked at all, "+
				"because a nil map copies as a nil map and the leak does not exist yet.", f.Name)
		}
	}
	if t.Failed() {
		return
	}

	child := parent.clone()
	cv := reflect.ValueOf(child).Elem()
	for i := range pv.NumField() {
		f := pv.Type().Field(i)
		if f.Type.Kind() != reflect.Map {
			continue
		}
		pp, cp := pv.Field(i).UnsafePointer(), cv.Field(i).UnsafePointer()
		shared := pp != nil && pp == cp
		reason, allowed := sharedTables[f.Name]
		switch {
		case shared && !allowed:
			t.Errorf("%s is one map shared by a subshell and its parent.\n"+
				"Add it to ownTables. A write on either side is the other's, and where the "+
				"subshell is a process substitution the two run on different goroutines, "+
				"which ends the process with a runtime fatal error no recover can catch.", f.Name)
		case !shared && allowed:
			t.Errorf("%s is named in sharedTables (%s) but the clone no longer shares it.\n"+
				"If that is now the intent, take it out of sharedTables.", f.Name, reason)
		}
	}
}

// TestACloneKeepsAnEmptyTableItsOwn is the seeding trap from the other side.
//
// maps.Clone keeps a nil map nil, so a clone taken before its parent has
// written a table gets nil and allocates its own on first write. That is
// correct and is why the leak is invisible early — but it must stay true of
// an *empty non-nil* table too, or a parent that has written and cleared one
// hands out a shared table again. Asserted for one representative table
// rather than all of them, because it is a property of maps.Clone.
func TestACloneKeepsAnEmptyTableItsOwn(t *testing.T) {
	parent := newTestRunner(t, &Runner{})
	parent.removed = map[string]bool{}
	child := parent.clone()
	child.removed["zq"] = true
	if parent.removed["zq"] {
		t.Error("a subshell's `unset` reached the parent through an empty table it was handed")
	}
}

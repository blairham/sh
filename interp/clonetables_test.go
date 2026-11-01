// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"os"
	"reflect"
	"slices"
	"strconv"
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
	"preludeFuncs":        "written only while the prelude is sourced and never deleted from, so no subshell can change it",
	"optionLetterNames":   "the dialect's `set` option letters, handed in whole by Apply and never written to afterwards — the same terms optionLists is on, one field kind along",
	"dialectCompgen":      "the `compgen -A` actions a dialect registered with SetCompgenAction, written at setup and never again — a script can ask compgen for one but has no way to add, replace or remove one",
	"onParameterArrival":  "what a dialect wants run when a deferred name is first referred to, registered by SetParameterArrival at setup and never again — a script can trigger one but has no way to add, replace or remove one",
	"firstUseSeeds":       "the values a front end asked to be laid down on first use, replaced whole by SetVarOnFirstUse at setup and never written in place — settling one clears a flag in the runner's own cell and leaves the map alone",
	"shellOwnWhileSet":    "the names a dialect owns while they hold a value, marked at setup by MarkShellOwnParameterWhileSet and never written again",
	"equalsTildeBuiltins": "the builtins a dialect says read an argument's `=` as an assignment's, marked at setup by MarkBuiltinWhoseEqualsOpensATildeContext and never written again",
	"localKeepsOuter":     "the names a dialect says keep their outer value in a valueless local, marked at setup by MarkLocalKeepsTheOuterValue and never written again",
	"preludeCommands":     "the commands a dialect gave its prelude, registered by RegisterPreludeCommand at setup and never written again — a script cannot add, replace or remove one",
	"noticedJobs":         "never written, only replaced whole by noticeFinishedJobs — so a subshell that shares its parent's set and then notices a job gets a set of its own, and neither runner ever sees the other's notices",
	// Copied on write rather than at the clone. The test for these is
	// TestASubshellWriteToASharedTableStaysInTheSubshell, which writes each
	// one from either side and on two goroutines at once.
	"cellIndex":   "copy-on-write: the variable store's index of names, shared until either side adds a name, which copies it first — see varstore.go",
	"funcOrigins": "copy-on-write: shared until either side writes, which copies it first — see sharedtable.go",
	"cmdHash":     "copy-on-write: shared until either side writes, which copies it first — see sharedtable.go",
}

// sharedStacks names every slice a clone is allowed to share with its parent,
// on the same terms as sharedTables: a positive claim, asserted in both
// directions, so a name here without a reason reads as the deliberate act it
// has to be.
//
// A slice is the second half of #1783 and it hides better than a map. The
// header is copied by value, so the two runners have their own length — and
// `append` still writes the parent's *backing array* whenever the capacity is
// there. Nothing aliases while the slice is nil, which is why this survived
// #1416: a stack has to be pushed and popped once before the array exists,
// and popping keeps the capacity.
var sharedStacks = map[string]string{
	"cmdHashOrder":    "copy-on-write with cmdHash, the table it lists: copied with it on either side's first write — see sharedtable.go",
	"Env":             "the environment the shell was started with, never appended to after setup",
	"InheritedFiles":  "the files the embedder handed in, never appended to at all",
	"ProcessAnchor":   "the placeholder command the front end handed in, never appended to at all",
	"trapSnapshot":    "only ever replaced wholesale or set to nil, and inheritTraps rebuilds a subshell's traps from scratch",
	"pipeStatus":      "rebuilt with append([]int(nil), …) on every pipeline, so a write never lands in an array anyone else holds",
	"optionLists":     "the option namespaces a dialect bound in Apply, appended to at setup and never again",
	"RlimitOrder":     "the order this kernel numbers its limits in, handed in by the front end at setup and never appended to — a fact about the machine rather than anything a script can move",
	"carriedHeredocs": "the running file's own list, replaced wholesale by RunPart and never appended to — a subshell reads the same file's list, which is what it should see",
	"substEnds":       "the running file's list of ends of input, kept beside carriedHeredocs and shared for the same reason",
	"flagWordBare":    "a handoff from flaggedWords to expandFlagged within one expansion, assigned whole and cleared by the reader, never appended to",
	"dialectCompgenAfter": "where each dialect action sits in `compgen -A`'s order, " +
		"appended to by SetCompgenAction at setup and never again — the order is the " +
		"dialect's declaration, on the same terms as optionLists",
	"listNulls": "the marks one unquoted list expansion left for the call reading it, " +
		"only ever assigned wholesale or set to nil and cleared at the entry to every " +
		"expandAt — a subshell expanding a word of its own overwrites it before it " +
		"reads it, and nothing appends to it at all",
	"restrictedFreezes": "the extra names this dialect's restricted mode freezes, " +
		"appended to in Apply at setup and never again — Runner.FreezeInRestrictedMode " +
		"is a dialect's declaration and not anything a script can reach",
	"restrictedFreezesExact": "the whole frozen list a dialect names, assigned once in " +
		"Apply by Runner.FreezeOnlyInRestrictedMode and never appended to",
}

// seedStacks gives every slice on a Runner an element and spare capacity.
//
// The capacity is the point and not a detail: a slice at its exact length
// allocates on the next append and cannot alias, so seeding without room to
// grow would give a clean run for the same reason `f | f` *once* is clean.
// Popping is what leaves the room in the real thing — `popFrame` shortens the
// length and keeps the array — so the seed models a stack that has been used.
//
// Hand-written and exhaustive for the reason seedTables is: reflection cannot
// set an unexported field, so the check can only see what this fills in, and
// the test below fails on any slice it left out.
func seedStacks(r *Runner) {
	r.Env = append(make([]string, 0, 4), "SEED=v")
	r.restrictedFreezes = append(make([]string, 0, 4), "SEEDNAME")
	r.restrictedFreezesExact = append(make([]string, 0, 4), "SEEDNAME")
	r.Params = append(make([]string, 0, 4), "seed")
	r.cellSlab = append(make([]varCell, 0, 4), varCell{name: "seed"})
	r.listNulls = append(make([]bool, 0, 4), false)
	r.procSubs = append(make([]procSubPipe, 0, 4), procSubPipe{})
	r.heldProcSubs = append(make([]procSubPipe, 0, 4), procSubPipe{})
	r.openRun = append(make([]string, 0, 4), "if")
	r.frames = append(make([]Frame, 0, 4), Frame{})
	r.evalUnits = append(make([]evalUnit, 0, 4), evalUnit{})
	r.evalContexts = append(make([]EvalContext, 0, 4), EvalContextEval)
	r.leavingContexts = append(make([]EvalContext, 0, 4), EvalContextEval)
	r.callArgs = append(make([]callArgs, 0, 4), callArgs{})
	r.borrowed = append(make([]borrowedText, 0, 4), borrowedText{})
	r.InheritedFiles = append(make([]*os.File, 0, 4), nil)
	r.ProcessAnchor = append(make([]string, 0, 4), "seed")
	r.RlimitOrder = append(make([]Resource, 0, 4), ResourceCPUTime)
	r.redirFds = append(make([]int, 0, 4), 0)
	r.arithValueNames = append(make([]string, 0, 4), "seed")
	r.jobs = append(make([]*Job, 0, 4), nil)
	r.jobOrder = append(make([]*Job, 0, 4), nil)
	r.outerSlots = append(make([]int, 0, 4), 0)
	r.reaped = append(make([]*Job, 0, 4), nil)
	r.dropped = append(make([]*Job, 0, 4), nil)
	r.flagWordBare = append(make([]bool, 0, 4), false)
	r.procSubJobs = append(make([]*Job, 0, 4), nil)
	r.enclosingProcSubs = append(make([]procSubPipe, 0, 4), procSubPipe{})
	r.releasedSubstFds = append(make([]int, 0, 4), 63)
	r.heldNumbers = append(make([]heldNumber, 0, 4), heldNumber{fd: 11, keptByAFork: true})
	r.madeFirst = append(make([]madeFirstSubst, 0, 4), madeFirstSubst{path: "/dev/fd/11", ok: true})
	r.carriedHeredocs = append(make([]syntax.CarriedHeredoc, 0, 4), syntax.CarriedHeredoc{})
	r.substEnds = append(make([]syntax.SubstitutionEnds, 0, 4), syntax.SubstitutionEnds{})
	seed := &shadowRecord{
		name: "seed",
		has: restoredRecords | recExportedShadow | flagHeldAnElementBefore |
			flagScalarHeldBefore | flagTieMirrorOnly,
		bits:           recValue | recArray | recAssoc,
		value:          "v",
		array:          ArrayOf(Element{Str: "v"}),
		assoc:          AssocArray{"k": {Str: "v"}},
		producer:       &suspendedProducer{},
		exportedShadow: "v",
	}
	r.scopes = append(make([]*scope, 0, 4), &scope{
		records:              append(make([]*shadowRecord, 0, 4), seed),
		recordIndex:          map[string]*shadowRecord{"seed": seed},
		namespaceOwned:       map[string]bool{"seed": true},
		spares:               &r.spares,
		private:              map[string]bool{"seed": true},
		privateSealed:        map[string]sealedName{"seed": {}},
		privateShielded:      append(make([]string, 0, 4), "seed"),
		savedTraps:           map[string]savedTrapState{"seed": {}},
		savedOptions:         append(make([]bool, 0, 4), true),
		sealed:               map[string]sealedName{"seed": {}},
		onReturn:             append(make([]func(), 0, 4), func() {}),
		badNamesForTheReturn: append(make([]string, 0, 4), "1x"),
	})
	r.aroundFunctionCalls = append(make([]func(*Runner) func(), 0, 4), nil)
	r.atFunctionDefinition = append(make([]func(*Runner, string), 0, 4), nil)
	r.selfPending = append(make([]string, 0, 4), "seed")
	r.selfHeldForInput = append(make([]string, 0, 4), "seed")
	r.substLevelsOut = append(make([]substLevel, 0, 4), substLevel{})
	r.trapSnapshot = append(make([]savedTrap, 0, 4), savedTrap{})
	r.debugHeld = append(make([]debugHeld, 0, 4), debugHeld{})
	r.trapContexts = append(make([]trapContext, 0, 4), trapContext(0))
	r.pipeStatus = append(make([]int, 0, 4), 0)
	r.freezeAfter = append(make([]string, 0, 4), "seed")
	r.mathOrder = append(make([]string, 0, 4), "seed")
	r.declaredTypes = append(make([]string, 0, 4), "seed")
	r.cmdHashOrder = append(make([]string, 0, 4), "seed")
	r.optionLists = append(make([]optionList, 0, 4), optionList{})
	r.dialectCompgenAfter = append(make([][2]string, 0, 4), [2]string{"seed", "seed"})
	r.prefixTraceAssigns = append(make([]*syntax.Assign, 0, 4), nil)
	r.declarationOperands = append(make([]int, 0, 4), 0)
	r.arrayOperands = append(make([]arrayOperand, 0, 4), arrayOperand{})
	r.lexedSubscriptOperands = append(make([]string, 0, 4), "seed")
	r.sourceClosedSubscriptOperands = append(make([]string, 0, 4), "seed")
	r.prefixTraceValues = append(make([]string, 0, 4), "seed")
	r.prefixTracedEarly = append(make([]*syntax.Assign, 0, 4), nil)
	r.prefixTraceJoins = append(make([]string, 0, 4), "seed")
	r.prefixGlobMatches = append(make([]prefixGlobMatch, 0, 4), prefixGlobMatch{})
	r.prefixSubscripts = append(make([]*prefixSubscriptRead, 0, 4), &prefixSubscriptRead{})
	r.coprocOwnEnds = append(make([]coprocNearEnd, 0, 4), coprocNearEnd{})
	r.prefixHeldNames = append(make([]string, 0, 4), "seed")
	r.fdVarStores = append(make([]fdVarStore, 0, 4), fdVarStore{"seed", "10"})
	r.prefixKeptNames = append(make([]string, 0, 4), "seed")
	r.prefixShadowed = append(make([]string, 0, 4), "seed")
	r.globalUnderItsOwnPrefix = append(make([]string, 0, 4), "seed")
	r.prefixHeldUndo = append(make([]savedVar, 0, 4), savedVar{name: "seed"})
	r.functionPrefixNames = append(make([]string, 0, 4), "seed")
	r.callPrefixes = append(make([]callPrefixFrame, 0, 4),
		callPrefixFrame{names: []string{"seed"}, undo: []savedVar{{name: "seed"}}})
}

// runnerFields is every field of a Runner a clone could share, including the
// ones a struct embedded in it promotes. The two tests below walked the
// Runner's own fields, so the records interp/commandrecords.go gathers into an
// embedded struct would have dropped out of both guarantees the moment they
// moved — measured: with the clone of prefixTraceAssigns deleted, the stack
// test still passed. The embedded struct itself is a struct and not a table
// or a stack, so the kind checks below pass over it and read what it holds.
func runnerFields() []reflect.StructField {
	return reflect.VisibleFields(reflect.TypeFor[Runner]())
}

// TestACloneOwnsEveryStack is TestACloneOwnsEveryTable for the slices, and it
// exists because that test could not see them: it considers a field only when
// its kind is exactly reflect.Map, so `scopes []*scope` was skipped outright
// and the fourteen maps inside a scope were not reachable by the guarantee at
// all. The guarantee was one level deep and read as though it were total.
//
// What that cost: a `local` in a function on two sides of one pipe ended the
// process with `fatal error: concurrent map writes`, because the last element
// of a zsh pipeline runs on the shell itself while the others run on clones
// that shared its scope stack — two goroutines appending to one array, both
// reading back the same `*scope`, both writing its `saved` map (#1783).
func TestACloneOwnsEveryStack(t *testing.T) {
	parent := newTestRunner(t, &Runner{})
	seedStacks(parent)

	pv := reflect.ValueOf(parent).Elem()
	for _, f := range runnerFields() {
		if f.Type.Kind() != reflect.Slice {
			continue
		}
		fv := pv.FieldByIndex(f.Index)
		if fv.IsNil() {
			t.Errorf("%s is a slice on Runner that seedStacks does not fill in.\n"+
				"Seed it with spare capacity, then decide: ownTables must copy it, or "+
				"sharedStacks must say why a subshell may share it. An unseeded stack "+
				"cannot be checked, because a nil slice copies as a nil slice and two "+
				"clones each allocate their own array.", f.Name)
			continue
		}
		if fv.Cap() == fv.Len() {
			t.Errorf("%s is seeded at its exact capacity, so an append cannot alias and "+
				"the check would pass for the wrong reason. Give it room to grow.", f.Name)
		}
	}
	if t.Failed() {
		return
	}

	child := parent.clone()
	cv := reflect.ValueOf(child).Elem()
	for _, f := range runnerFields() {
		if f.Type.Kind() != reflect.Slice {
			continue
		}
		pp, cp := pv.FieldByIndex(f.Index).UnsafePointer(), cv.FieldByIndex(f.Index).UnsafePointer()
		shared := pp != nil && pp == cp
		reason, allowed := sharedStacks[f.Name]
		switch {
		case shared && !allowed:
			t.Errorf("%s is one backing array shared by a subshell and its parent.\n"+
				"Add it to ownTables. Both sides append into the same array while their "+
				"lengths disagree, so each overwrites what the other pushed and both read "+
				"back the same element — and where the subshell runs on a goroutine, as a "+
				"pipeline element and a process substitution both do, that is a data race "+
				"on whatever the element holds.", f.Name)
		case !shared && allowed:
			t.Errorf("%s is named in sharedStacks (%s) but the clone no longer shares it.\n"+
				"If that is now the intent, take it out of sharedStacks.", f.Name, reason)
		}
	}
}

// TestACloneOwnsEveryScopeTable is the level the two tests above cannot reach:
// a scope's tables are inside the scope, so giving the clone its own backing
// array is not enough — the elements are pointers, and a copied array of the
// same pointers still hands two shells one `*scope` to write.
//
// This is the crash of #1783 stated as an invariant rather than as a race:
// `shadow` writes a name's record on whatever sits at the top of the stack,
// so if that scope is the parent's scope, two goroutines write one record.
func TestACloneOwnsEveryScopeTable(t *testing.T) {
	parent := newTestRunner(t, &Runner{})
	seedStacks(parent)

	child := parent.clone()
	if len(child.scopes) != len(parent.scopes) {
		t.Fatalf("the clone's scope stack is %d deep and its parent's is %d: a clone "+
			"inherits the stack it was made inside, and only the memory changes hands",
			len(child.scopes), len(parent.scopes))
	}
	ps, cs := parent.scopes[0], child.scopes[0]
	if ps == cs {
		t.Fatalf("the clone's innermost scope is its parent's scope. Every table on it " +
			"is then one table with two shells writing it, which is the concurrent map " +
			"write of #1783.")
	}
	if cs.owner != ps.owner {
		t.Errorf("the copied scope's owner changed. It records which runner *pushed* the "+
			"scope and ownScope reads it to keep a subshell from writing the caller's "+
			"scope; repointing it at the clone would change what that check answers. "+
			"Got %p, want %p.", cs.owner, ps.owner)
	}

	sv := reflect.ValueOf(ps).Elem()
	cvv := reflect.ValueOf(cs).Elem()
	for i := range sv.NumField() {
		f := sv.Type().Field(i)
		switch f.Type.Kind() {
		case reflect.Map, reflect.Slice:
		default:
			continue
		}
		if sv.Field(i).IsNil() {
			t.Errorf("scope.%s is not seeded by seedStacks, so nothing here can say "+
				"whether the clone shares it.", f.Name)
			continue
		}
		if pp, cp := sv.Field(i).UnsafePointer(), cvv.Field(i).UnsafePointer(); pp == cp {
			t.Errorf("scope.%s is one table shared by a subshell and its parent. "+
				"cloneScopes must copy it: `local` writes the innermost scope, and a "+
				"pipeline runs its elements on goroutines.", f.Name)
		}
	}

	// The records are pointers, so a copied list of the same pointers is one
	// record with two shells writing it — and each record holds two tables of
	// its own under the name.
	if len(cs.records) != len(ps.records) {
		t.Fatalf("the clone's scope holds %d records and its parent's %d", len(cs.records), len(ps.records))
	}
	for i, prec := range ps.records {
		crec := cs.records[i]
		if prec == crec {
			t.Errorf("the record of %q is one record shared by a subshell and its parent", prec.name)
			continue
		}
		if crec.array == prec.array ||
			reflect.ValueOf(crec.assoc).UnsafePointer() == reflect.ValueOf(prec.assoc).UnsafePointer() {
			t.Errorf("the record of %q shares a saved table with its parent's", prec.name)
		}
		if crec.producer == prec.producer {
			t.Errorf("the record of %q shares its suspended producer with its parent's", prec.name)
		}
		if cs.recordIndex[prec.name] != crec {
			t.Errorf("the clone's index of %q points at a record that is not in its list", prec.name)
		}
	}
	if cs.spares != nil {
		t.Errorf("the clone's scope takes records from its parent's free list, which the " +
			"parent writes on its own goroutine")
	}
}

// TestACloneOwnsEveryCell is TestACloneOwnsEveryTable for the variable store,
// whose cells sit in a slab the clone must copy and whose index it may share:
// a clone holding its parent's slab would be one cell per name with two shells
// writing it, and no map-level check can see that. Every slot is seeded,
// because a slot a cell never held cannot show whether it was copied.
func TestACloneOwnsEveryCell(t *testing.T) {
	t.Parallel()
	parent := newTestRunner(t, &Runner{})
	parent.varsPut("all", "v")
	parent.arraysPut("all", ArrayOf(Element{Str: "x"}))
	parent.assocsPut("all", AssocArray{"k": Scalar("v")})
	parent.producersPut("all", func(*Runner) string { return "p" })
	parent.arrayProducersPut("all", func(*Runner) []string { return nil })
	parent.assocProducersPut("all", func(*Runner) AssocArray { return nil })
	// And the attributes, which live in the cell beside the slots (#6373):
	// one on a name holding a value, and one on a name holding nothing else.
	parent.attrSet("all", attrReadonly|attrInteger)
	parent.attrSet("attronly", attrLower)

	child := parent.clone()
	if len(child.cellSlab) != len(parent.cellSlab) {
		t.Fatalf("the clone's slab holds %d cells and its parent's %d", len(child.cellSlab), len(parent.cellSlab))
	}
	if &child.cellSlab[0] == &parent.cellSlab[0] {
		t.Fatal("the clone's slab is its parent's: every cell is one cell with two shells writing it")
	}
	for i := range parent.cellSlab {
		pc, cc := &parent.cellSlab[i], &child.cellSlab[i]
		if cc.name != pc.name || cc.has != pc.has || cc.attrs != pc.attrs {
			t.Errorf("cell %d is %q holding %b/%b in the clone and %q holding %b/%b in its parent",
				i, cc.name, cc.has, cc.attrs, pc.name, pc.has, pc.attrs)
		}
		if pc.array != nil && cc.array == pc.array {
			t.Errorf("the indexed array of %q is shared by a subshell and its parent", pc.name)
		}
	}
	if child.cellCount != parent.cellCount || child.liveCells != parent.liveCells || child.attrCount != parent.attrCount {
		t.Errorf("the clone counts %v slots and %v attributes in %d cells and its parent %v and %v in %d",
			child.cellCount, child.attrCount, child.liveCells, parent.cellCount, parent.attrCount, parent.liveCells)
	}
	// An attribute the subshell gives or takes stays in the subshell.
	child.attrClear("all", attrReadonly)
	child.attrSet("attronly", attrUpper)
	if !parent.attrHas("all", attrReadonly) || parent.attrHas("attronly", attrUpper) {
		t.Error("an attribute the subshell wrote reached its parent's cell")
	}

	// The index is shared, and the first name either side adds copies it.
	child.varsPut("childonly", "c")
	if _, ok := parent.cellIndex["childonly"]; ok {
		t.Error("a name the subshell made reached its parent's index")
	}
	parent.varsPut("parentonly", "p")
	if _, ok := child.cellIndex["parentonly"]; ok {
		t.Error("a name the parent made reached the subshell's index")
	}
	if got := child.varsAt("all"); got != "v" {
		t.Errorf("the subshell reads %q for a name both had, want v", got)
	}
}

// TestASparseStoreIsCompacted: a script that makes names by the thousand and
// unsets them must not carry an empty cell for each one into every subshell
// it starts.
func TestASparseStoreIsCompacted(t *testing.T) {
	t.Parallel()
	r := newTestRunner(t, &Runner{})
	r.varsPut("keep", "k")
	for i := range 2000 {
		r.varsPut("tmp"+strconv.Itoa(i), "x")
	}
	for i := range 2000 {
		r.varsDel("tmp" + strconv.Itoa(i))
	}
	if len(r.cellSlab) > 600 {
		t.Errorf("the slab holds %d cells for %d live names", len(r.cellSlab), r.liveCells)
	}
	if got, ok := r.varsLookup("keep"); !ok || got != "k" {
		t.Errorf("compacting lost a live name: %q, %v", got, ok)
	}
	if r.varsLen() != 1 {
		t.Errorf("%d scalars counted after compacting, want 1", r.varsLen())
	}
}

// TestACellHoldingOnlyAnAttributeIsKept: a name the shell knows only by an
// attribute — `readonly x` with x unset — is a live cell, so compacting the
// slab must keep it and the counts must say so (#6373).
func TestACellHoldingOnlyAnAttributeIsKept(t *testing.T) {
	t.Parallel()
	r := newTestRunner(t, &Runner{})
	r.attrSet("frozen", attrReadonly)
	for i := range 2000 {
		r.varsPut("tmp"+strconv.Itoa(i), "x")
	}
	for i := range 2000 {
		r.varsDel("tmp" + strconv.Itoa(i))
	}
	if len(r.cellSlab) > 600 {
		t.Errorf("the slab holds %d cells for %d live names", len(r.cellSlab), r.liveCells)
	}
	if !r.attrHas("frozen", attrReadonly) {
		t.Error("compacting lost a name holding only an attribute")
	}
	if r.attrLen(attrReadonly) != 1 || r.liveCells != 1 {
		t.Errorf("%d readonly names in %d live cells after compacting, want 1 in 1", r.attrLen(attrReadonly), r.liveCells)
	}
	r.attrClear("frozen", attrReadonly)
	if r.attrLen(attrReadonly) != 0 || r.liveCells != 0 {
		t.Errorf(
			"%d readonly names in %d live cells after the last attribute went, want 0 in 0",
			r.attrLen(attrReadonly),
			r.liveCells,
		)
	}
	var names []string
	for n := range r.attrNames(attrReadonly) {
		names = append(names, n)
	}
	if len(names) != 0 {
		t.Errorf("attrNames lists %v after the attribute went", names)
	}
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
	r.noticedJobs = map[*Job]bool{{}: true}
	r.firstUseSeeds = map[string]*firstUseSeed{"x": {}}
	r.keptSubstBodies = map[substBodyKey]keptSubstBody{{text: "seed"}: {}}
	r.dialectMoves = map[dialectMoveKey]*syntax.Dialect{{}: nil}
	r.bodyTexts = map[*syntax.FuncDecl]string{{}: "seed"}
	r.pipeCounts = map[syntax.Command]bool{&syntax.Group{}: true}
	r.SeedVars(map[string]string{"seed": "v"})
	r.arraysPut("seed", ArrayOf(Element{Str: "v"}))
	r.assocsPut("seed", AssocArray{"k": {Str: "v"}})
	r.producersPut("seed", func(*Runner) string { return "" })
	r.arrayProducersPut("seed", func(*Runner) []string { return nil })
	r.assocProducersPut("seed", func(*Runner) AssocArray { return nil })
	r.dynamicAssocElements = map[string]func(*Runner, string) (string, bool){
		"seed": func(*Runner, string) (string, bool) { return "", false },
	}
	r.dynamicAssocKeys = map[string]func(*Runner) []string{"seed": func(*Runner) []string { return nil }}
	r.dynamicArrayWriters = map[string]func(*Runner, []string){"seed": func(*Runner, []string) {}}
	r.silentToPrint = map[string]bool{"seed": true}
	r.subscriptValuesHeld = map[string]heldSubscriptValue{"seed": {}}
	r.hideKindsWatched = map[string]ParameterKind{"seed": ScalarParameter}
	r.unsetRefused = map[string]bool{"seed": true}
	r.tableFrozenByKey = map[string]bool{"seed": true}
	r.dynamicAssocWriters = map[string]func(*Runner, string, string, bool){
		"seed": func(*Runner, string, string, bool) {},
	}
	r.dynamicAssocEmptied = map[string]bool{"seed": true}
	r.dynamicAssocSetUnbuilt = map[string]bool{"seed": true}
	r.dynamicAssocKeyOrder = map[string]func([]string) []string{
		"seed": func(keys []string) []string { return keys },
	}
	r.dynamicWriters = map[string]func(*Runner, string){"seed": func(*Runner, string) {}}
	r.assignmentActions = map[string]func(*Runner, string){"seed": func(*Runner, string) {}}
	r.assignmentGuards = map[string]func(*Runner, string) (string, bool){
		"seed": func(*Runner, string) (string, bool) { return "", false },
	}
	r.valueRefusals = map[string]func(string) (string, bool){"seed": func(string) (string, bool) { return "", false }}
	r.unsetActions = map[string]func(*Runner){"seed": func(*Runner) {}}
	r.inheritedParameterActions = map[string]func(*Runner, string){"seed": func(*Runner, string) {}}
	r.restrictedFrozen = map[string]bool{"seed": true}
	r.optionTies = map[string]func(*Runner, bool){"seed": func(*Runner, bool) {}}
	r.dynamicPresence = map[string]func(*Runner) bool{"seed": func(*Runner) bool { return true }}
	r.dynamicDeclarations = map[string]ProducedDeclaration{"seed": {Integer: true}}
	r.rejoinedOperands = map[string]bool{"seed": true}
	r.producedReading = map[string]string{"seed": "1"}
	r.endedProducers = map[string]bool{"seed": true}
	r.absentElements = map[string]string{"seed": "v"}
	r.absentParams = map[string]string{"seed": "v"}
	r.deferredParams = map[string]bool{"seed": true}
	r.onParameterArrival = map[string]func(*Runner){"seed": func(*Runner) {}}
	r.readonlyByDeclaration = map[string]bool{"seed": true}
	r.removedShellOwn = map[string]bool{"seed": true}
	r.conditionAnswers = map[string]ConditionAnswer{
		"seed": func(*Runner, context.Context, string, []string) (bool, bool) { return false, false },
	}
	r.aliases = map[string]aliasDef{"seed": {value: "v"}}
	r.suffixAliases = map[string]string{"seed": "v"}
	r.namedAliases = map[string]bool{"seed": true}
	r.markedAliases = map[string]bool{"seed": true}
	r.unreportedAliases = map[string]bool{"seed": true}
	r.cmdHash = map[string]hashedCommand{"seed": {path: "/bin/seed", hits: 1}}
	r.cmdHashHits = map[string]int{"seed": 1}
	r.namedDirs = map[string]string{"seed": "/seed"}
	r.assigned = map[string]string{"seed": "v"}
	r.completions = map[string]completionSpec{"seed": {options: []string{"nospace"}, words: []string{"-F", "f"}}}
	r.custom = map[string]Builtin{"seed": func(*Runner, context.Context, []string) int { return 0 }}
	r.preludeCommands = map[string]Builtin{"seed": func(*Runner, context.Context, []string) int { return 0 }}
	r.compoundVariable = map[string]bool{"seed": true}
	r.namespaces = map[string]bool{"seed": true}
	r.declaring = map[string]bool{"seed": true}
	r.disabledBuiltins = map[string]bool{"seed": true}
	r.reservedOff = map[string]bool{"seed": true}
	r.withdrawnBuiltins = map[string]bool{"seed": true}
	r.withdrawnParams = map[string]withdrawnParameter{"seed": {}}
	r.withdrawnMathFuncs = map[string]bool{"seed": true}
	r.execFds = map[int]bool{7: true}
	r.cloexecFds = map[int]bool{8: true}
	r.dupFds = map[int]bool{9: true}
	r.readMarks = map[int]readMark{10: {stream: os.Stdin, stuck: true}}
	r.exportedFuncs = map[string]bool{"seed": true}
	r.withdrawnFuncs = map[string]withdrawnFunction{"seed": {}}
	r.readonlyFuncs = map[string]bool{"seed": true}
	r.tracedFuncs = map[string]bool{"seed": true}
	r.caseFuncs = map[string]rune{"seed": 'u'}
	r.funcTraceMarks = map[string]string{"seed": "t"}
	r.warnNestedFuncs = map[string]bool{"seed": true}
	r.extraOptions = map[string]bool{"seed": true}
	r.negatedOptions = map[string]string{"seed": "noseed"}
	r.immovableOptions = map[string]bool{"seed": true}
	r.inertOptions = map[string]bool{"seed": true}
	r.defaultOnOptions = map[string]bool{"seed": true}
	r.recordedOptions = map[string]bool{"seed": true}
	r.fds = map[int]any{7: nil}
	r.concurrent = map[string]*Concurrent{"seed": {name: "seed"}}
	r.freezing = map[string]bool{"seed": true}
	r.literalOperands = map[string]bool{"seed": true}
	r.containerByThisLinesLetter = map[string]bool{"seed": true}
	r.operandsOverANonArray = map[string]bool{"seed": true}
	r.compoundOperands = map[string]bool{"seed": true}
	r.compoundOperandUnset = map[string]bool{"seed": true}
	r.indexedLetterHere = map[string]bool{"seed": true}
	r.tableLetterHere = map[string]bool{"seed": true}
	r.globalLetterHere = map[string]bool{"seed": true}
	r.funcOrigins = map[string]funcOrigin{"seed": {file: "v"}}
	r.funcs = map[string]*syntax.FuncDecl{"seed": nil}
	r.disciplined = map[string]bool{"seed": true}
	r.disciplineRunning = map[string]bool{"seed": true}
	r.trapFuncs = map[string]string{"seed": "v"}
	r.privateShield = map[string]int{"seed": 1}
	r.inheritedIgnored = map[string]bool{"seed": true}
	r.localMarked = map[string]bool{"seed": true}
	r.integerBase = map[string]int{"seed": 16}
	r.declarationKeeps = map[string]bool{"seed": true}
	r.floatPrecision = map[string]int{"seed": 3}
	r.fieldWidth = map[string]fieldWidth{"seed": {letter: 76, width: 3}}
	r.caseListing = map[string]byte{"seed": 'l'}
	r.mathFuncs = map[string]mathFunc{"seed": {}}
	r.precommands = map[string]PrecommandModifier{"seed": PrecommandNoGlob}
	r.commandWordModifiers = map[string]bool{"seed": true}
	r.optionLetterNames = map[rune]string{'Z': "seed"}
	r.dialectCompgen = map[string]func(*Runner, string) []string{"seed": nil}
	r.preludeFuncs = map[string]*syntax.FuncDecl{"seed": nil}
	r.tied = map[string]tie{"seed": {}}
	r.traps = map[string]string{"seed": "v"}
	r.shellOwn = map[string]bool{"seed": true}
	r.notShellOwn = map[string]bool{"seed": true}
	r.shellOwnWhileSet = map[string]bool{"seed": true}
	r.localKeepsOuter = map[string]bool{"seed": true}
	r.equalsTildeBuiltins = map[string]bool{"seed": true}
	r.envNotAdopted = map[string]bool{"seed": true}
	r.envCopyHeld = map[string]heldEnvCopy{"seed": {value: "seed", present: true}}
	r.scopeFixed = map[string]bool{"seed": true}
	r.kindFixed = map[string]parameterKinds{"seed": kindBit(ArrayParameter)}
	r.nameref = map[string]string{"seed": "target"}
	r.floatExact = map[string]storedFloat{"seed": {text: "1.500", value: 1.5}}
	r.capitalized = map[string]bool{"seed": true}
	r.subshellWroteArrays = map[string]bool{"seed": true}
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
	for _, f := range runnerFields() {
		if f.Type.Kind() != reflect.Map {
			continue
		}
		if pv.FieldByIndex(f.Index).IsNil() {
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
	for _, f := range runnerFields() {
		if f.Type.Kind() != reflect.Map {
			continue
		}
		pp, cp := pv.FieldByIndex(f.Index).UnsafePointer(), cv.FieldByIndex(f.Index).UnsafePointer()
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
	t.Parallel()
	parent := newTestRunner(t, &Runner{})
	parent.removedShellOwn = map[string]bool{}
	child := parent.clone()
	child.removedShellOwn["zq"] = true
	if parent.removedShellOwn["zq"] {
		t.Error("a subshell's `unset` reached the parent through an empty table it was handed")
	}
}

// TestTheStoresOneLookupReadDeclinesWhatTheChainAnswers names, state by
// state, which reads the one-lookup path answers and which it leaves to the
// chain, so the comparison in TestAPlainParameterWordAnswersAsTheFullRoadDoes
// is known to cover both halves.
func TestTheStoresOneLookupReadDeclinesWhatTheChainAnswers(t *testing.T) {
	t.Parallel()
	r := newTestRunner(t, &Runner{})
	r.varsPut("plain", "v")
	if v, ok := r.storedScalarOnly("plain"); !ok || v != "v" {
		t.Fatalf("an ordinary scalar was not answered: %q, %v", v, ok)
	}
	r.varsPut("both", "e0")
	r.arraysPut("both", ArrayOf(Element{Str: "e0"}))
	r.varsPut("made", "x")
	r.producersPut("made", func(*Runner) string { return "p" })
	r.varsPut("ref", "x")
	r.nameref = map[string]string{"ref": "plain"}
	r.varsPut("c.m", "x")
	for _, name := range []string{"both", "made", "ref", "c.m", "absent"} {
		if _, ok := r.storedScalarOnly(name); ok {
			t.Errorf("%q was answered from the store where the chain answers it", name)
		}
	}
	r.pipeStatusName = "plain"
	if _, ok := r.storedScalarOnly("plain"); ok {
		t.Error("the produced pipeline status was answered from the store")
	}
}

// TestAnUnchangedTableIsListedWithoutSortingAgain is #6384's acceptance: the
// second listing of a table nothing has written costs a copy, not a sort, and
// a key added or removed in place is seen by the next listing.
func TestAnUnchangedTableIsListedWithoutSortingAgain(t *testing.T) {
	r := newTestRunner(t, &Runner{})
	for i := range 3000 {
		r.setAssocElem("h", "k"+strconv.Itoa(i), "v")
	}
	before := assocKeySorts.Load()
	first := r.assocKeys("h", r.assocsAt("h"))
	second := r.assocKeys("h", r.assocsAt("h"))
	if sorts := assocKeySorts.Load() - before; sorts != 1 {
		t.Errorf("two listings of an unchanged table sorted %d times, want 1", sorts)
	}
	if !slices.Equal(first, second) || !slices.IsSorted(first) || len(first) != 3000 {
		t.Fatalf("the listings differ or are not in order: %d and %d keys", len(first), len(second))
	}
	first[0] = "scribbled"
	if got := r.assocKeys("h", r.assocsAt("h")); got[0] == "scribbled" {
		t.Error("a caller's write into its list reached the next listing")
	}

	r.setAssocElem("h", "a-new-key", "v")
	if got := r.assocKeys("h", r.assocsAt("h")); len(got) != 3001 || got[0] != "a-new-key" {
		t.Errorf("a key added in place is missing from the next listing: %d keys, first %q", len(got), got[0])
	}
	r.unsetAssocElem("h", "a-new-key")
	if got := r.assocKeys("h", r.assocsAt("h")); len(got) != 3000 || got[0] == "a-new-key" {
		t.Errorf("a key removed in place is still listed: %d keys, first %q", len(got), got[0])
	}
	// Overwriting a key keeps the set, and the listing with it.
	before = assocKeySorts.Load()
	r.setAssocElem("h", "k1", "w")
	r.assocKeys("h", r.assocsAt("h"))
	if sorts := assocKeySorts.Load() - before; sorts != 0 {
		t.Errorf("rewriting an existing key cost %d sorts, want none", sorts)
	}
}

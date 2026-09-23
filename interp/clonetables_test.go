// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"os"
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
	"preludeFuncs":      "written only while the prelude is sourced and never deleted from, so no subshell can change it",
	"optionLetterNames": "the dialect's `set` option letters, handed in whole by Apply and never written to afterwards — the same terms optionLists is on, one field kind along",
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
	"Env":             "the environment the shell was started with, never appended to after setup",
	"InheritedFiles":  "the files the embedder handed in, never appended to at all",
	"ProcessAnchor":   "the placeholder command the front end handed in, never appended to at all",
	"trapSnapshot":    "only ever replaced wholesale or set to nil, and inheritTraps rebuilds a subshell's traps from scratch",
	"pipeStatus":      "rebuilt with append([]int(nil), …) on every pipeline, so a write never lands in an array anyone else holds",
	"optionLists":     "the option namespaces a dialect bound in Apply, appended to at setup and never again",
	"RlimitOrder":     "the order this kernel numbers its limits in, handed in by the front end at setup and never appended to — a fact about the machine rather than anything a script can move",
	"substLevelsOut":  "rebuilt with append([]substLevel{}, …) every time a level opens, so a write never lands in an array anyone else holds",
	"carriedHeredocs": "the running file's own list, replaced wholesale by RunPart and never appended to — a subshell reads the same file's list, which is what it should see",
	"restrictedFreezes": "the extra names this dialect's restricted mode freezes, " +
		"appended to in Apply at setup and never again — Runner.FreezeInRestrictedMode " +
		"is a dialect's declaration and not anything a script can reach",
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
	r.Params = append(make([]string, 0, 4), "seed")
	r.procSubs = append(make([]procSubPipe, 0, 4), procSubPipe{})
	r.heldProcSubs = append(make([]procSubPipe, 0, 4), procSubPipe{})
	r.frames = append(make([]Frame, 0, 4), Frame{})
	r.callArgs = append(make([]callArgs, 0, 4), callArgs{})
	r.borrowed = append(make([]borrowedText, 0, 4), borrowedText{})
	r.InheritedFiles = append(make([]*os.File, 0, 4), nil)
	r.ProcessAnchor = append(make([]string, 0, 4), "seed")
	r.RlimitOrder = append(make([]Resource, 0, 4), ResourceCPUTime)
	r.redirFds = append(make([]int, 0, 4), 0)
	r.arithValueNames = append(make([]string, 0, 4), "seed")
	r.jobs = append(make([]*Job, 0, 4), nil)
	r.jobOrder = append(make([]*Job, 0, 4), nil)
	r.reaped = append(make([]*Job, 0, 4), nil)
	r.procSubJobs = append(make([]*Job, 0, 4), nil)
	r.enclosingProcSubs = append(make([]procSubPipe, 0, 4), procSubPipe{})
	r.releasedSubstFds = append(make([]int, 0, 4), 63)
	r.carriedHeredocs = append(make([]syntax.CarriedHeredoc, 0, 4), syntax.CarriedHeredoc{})
	r.scopes = append(make([]*scope, 0, 4), &scope{
		saved:               map[string]string{"seed": "v"},
		existed:             map[string]bool{"seed": true},
		savedArrays:         map[string]Array{"seed": {0: {Str: "v"}}},
		arrayExisted:        map[string]bool{"seed": true},
		removedBefore:       map[string]bool{"seed": true},
		declaredOnlyBefore:  map[string]bool{"seed": true},
		memberNamespaces:    map[string]bool{"seed": true},
		compoundMarkBefore:  map[string]bool{"seed": true},
		namespaceOwned:      map[string]bool{"seed": true},
		heldAnElementBefore: map[string]bool{"seed": true},
		savedAssoc:          map[string]AssocArray{"seed": {"k": {Str: "v"}}},
		assocExisted:        map[string]bool{"seed": true},
		savedReadonly:       map[string]bool{"seed": true},
		savedHideInScope:    map[string]bool{"seed": true},
		hiddenShadow:        map[string]bool{"seed": true},
		suspendedProducers:  map[string]suspendedProducer{"seed": {}},
		savedAttrs:          map[string]nameAttributes{"seed": {}},
		savedAssigned:       map[string]string{"seed": "v"},
		assignedSpoken:      map[string]bool{"seed": true},
		savedExported:       map[string]bool{"seed": true},
		exportedSpoken:      map[string]bool{"seed": true},
		exportedShadow:      map[string]string{"seed": "v"},
		savedTraps:          map[string]savedTrapState{"seed": {}},
		savedOptions:        map[string]bool{"seed": true},
		sealed:              map[string]sealedName{"seed": {}},
		onReturn:            append(make([]func(), 0, 4), func() {}),
	})
	r.aroundFunctionCalls = append(make([]func(*Runner) func(), 0, 4), nil)
	r.selfPending = append(make([]string, 0, 4), "seed")
	r.substLevelsOut = append(make([]substLevel, 0, 4), substLevel{})
	r.trapSnapshot = append(make([]savedTrap, 0, 4), savedTrap{})
	r.trapContexts = append(make([]trapContext, 0, 4), trapContext(0))
	r.pipeStatus = append(make([]int, 0, 4), 0)
	r.freezeAfter = append(make([]string, 0, 4), "seed")
	r.mathOrder = append(make([]string, 0, 4), "seed")
	r.declaredTypes = append(make([]string, 0, 4), "seed")
	r.cmdHashOrder = append(make([]string, 0, 4), "seed")
	r.optionLists = append(make([]optionList, 0, 4), optionList{})
	r.prefixTraceAssigns = append(make([]*syntax.Assign, 0, 4), nil)
	r.declarationOperands = append(make([]int, 0, 4), 0)
	r.arrayOperands = append(make([]arrayOperand, 0, 4), arrayOperand{})
	r.lexedSubscriptOperands = append(make([]string, 0, 4), "seed")
	r.prefixTraceValues = append(make([]string, 0, 4), "seed")
	r.prefixHeldNames = append(make([]string, 0, 4), "seed")
	r.prefixKeptNames = append(make([]string, 0, 4), "seed")
	r.prefixShadowed = append(make([]string, 0, 4), "seed")
	r.globalUnderItsOwnPrefix = append(make([]string, 0, 4), "seed")
	r.prefixHeldUndo = append(make([]savedVar, 0, 4), savedVar{name: "seed"})
	r.functionPrefixNames = append(make([]string, 0, 4), "seed")
	r.callPrefixes = append(make([]callPrefixFrame, 0, 4),
		callPrefixFrame{names: []string{"seed"}, undo: []savedVar{{name: "seed"}}})
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
	for i := range pv.NumField() {
		f := pv.Type().Field(i)
		if f.Type.Kind() != reflect.Slice {
			continue
		}
		fv := pv.Field(i)
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
	for i := range pv.NumField() {
		f := pv.Type().Field(i)
		if f.Type.Kind() != reflect.Slice {
			continue
		}
		pp, cp := pv.Field(i).UnsafePointer(), cv.Field(i).UnsafePointer()
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
// `shadow` writes `sc.saved[name]` on whatever sits at the top of the stack,
// so if that scope is the parent's scope, two goroutines write one map.
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
	r.Arrays = map[string]Array{"seed": {0: {Str: "v"}}}
	r.AssocArrays = map[string]AssocArray{"seed": {"k": {Str: "v"}}}
	r.Dynamic = map[string]func(*Runner) string{"seed": func(*Runner) string { return "" }}
	r.DynamicArrays = map[string]func(*Runner) []string{"seed": func(*Runner) []string { return nil }}
	r.DynamicAssocs = map[string]func(*Runner) AssocArray{"seed": func(*Runner) AssocArray { return nil }}
	r.dynamicAssocElements = map[string]func(*Runner, string) (string, bool){
		"seed": func(*Runner, string) (string, bool) { return "", false },
	}
	r.dynamicArrayWriters = map[string]func(*Runner, []string){"seed": func(*Runner, []string) {}}
	r.unsetRefused = map[string]bool{"seed": true}
	r.dynamicAssocWriters = map[string]func(*Runner, string, string, bool){
		"seed": func(*Runner, string, string, bool) {},
	}
	r.dynamicAssocEmptied = map[string]bool{"seed": true}
	r.dynamicWriters = map[string]func(*Runner, string){"seed": func(*Runner, string) {}}
	r.assignmentActions = map[string]func(*Runner, string){"seed": func(*Runner, string) {}}
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
	r.conditionAnswers = map[string]ConditionAnswer{
		"seed": func(*Runner, context.Context, string, []string) (bool, bool) { return false, false },
	}
	r.aliases = map[string]aliasDef{"seed": {value: "v"}}
	r.suffixAliases = map[string]string{"seed": "v"}
	r.namedAliases = map[string]bool{"seed": true}
	r.markedAliases = map[string]bool{"seed": true}
	r.unreportedAliases = map[string]bool{"seed": true}
	r.cmdHash = map[string]hashedCommand{"seed": {path: "/bin/seed", hits: 1}}
	r.namedDirs = map[string]string{"seed": "/seed"}
	r.assigned = map[string]string{"seed": "v"}
	r.completions = map[string]completionSpec{"seed": {options: []string{"nospace"}, words: []string{"-F", "f"}}}
	r.custom = map[string]Builtin{"seed": func(*Runner, context.Context, []string) int { return 0 }}
	r.declaredEmpty = map[string]bool{"seed": true}
	r.declaredOnlyCompound = map[string]bool{"seed": true}
	r.compoundHeldAnElement = map[string]bool{"seed": true}
	r.declaredBare = map[string]bool{"seed": true}
	r.unsetLeftItDeclared = map[string]bool{"seed": true}
	r.compoundVariable = map[string]bool{"seed": true}
	r.namespaces = map[string]bool{"seed": true}
	r.declaring = map[string]bool{"seed": true}
	r.disabledBuiltins = map[string]bool{"seed": true}
	r.withdrawnBuiltins = map[string]bool{"seed": true}
	r.withdrawnParams = map[string]withdrawnParameter{"seed": {}}
	r.execFds = map[int]bool{7: true}
	r.cloexecFds = map[int]bool{8: true}
	r.exported = map[string]bool{"seed": true}
	r.exportedFuncs = map[string]bool{"seed": true}
	r.readonlyFuncs = map[string]bool{"seed": true}
	r.tracedFuncs = map[string]bool{"seed": true}
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
	r.hidden = map[string]bool{"seed": true}
	r.hideInScope = map[string]bool{"seed": true}
	r.inheritedIgnored = map[string]bool{"seed": true}
	r.integer = map[string]bool{"seed": true}
	r.localMarked = map[string]bool{"seed": true}
	r.integerBase = map[string]int{"seed": 16}
	r.floatPrecision = map[string]int{"seed": 3}
	r.fieldWidth = map[string]fieldWidth{"seed": {letter: 76, width: 3}}
	r.lowered = map[string]bool{"seed": true}
	r.mathFuncs = map[string]mathFunc{"seed": {}}
	r.precommands = map[string]PrecommandModifier{"seed": PrecommandNoGlob}
	r.optionLetterNames = map[rune]string{'Z': "seed"}
	r.preludeFuncs = map[string]*syntax.FuncDecl{"seed": nil}
	r.readonly = map[string]bool{"seed": true}
	r.removed = map[string]bool{"seed": true}
	r.tied = map[string]tie{"seed": {}}
	r.traps = map[string]string{"seed": "v"}
	r.unique = map[string]bool{"seed": true}
	r.traced = map[string]bool{"seed": true}
	r.nameref = map[string]string{"seed": "target"}
	r.floatExponent = map[string]bool{"seed": true}
	r.uppered = map[string]bool{"seed": true}
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

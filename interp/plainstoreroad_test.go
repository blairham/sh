// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"
)

// theFullRoad is every method and field the full road of a bare assignment
// reaches on the runner, in `assign` and in `setVarAs`, as
// interp/plainstore.go was checked against it.
//
// The plain store answers each of these questions for the assignments it
// takes, so a question added to the full road is one it does not ask. This
// list is what makes that visible: change either function's calls and the
// test below fails, and the fix is to read storesAPlainScalar again — decide
// whether the new question can answer anything but "nothing to do" for an
// assignment it accepts, guard it there if so — and then update the list.
var theFullRoad = map[string][]string{
	"assign": {
		"appendScalarToArray", "appendedOldValue", "appendedValue", "arrayLiteralStartsTheNameOver",
		"arraysLookup", "assign", "assignArrayLiteral", "assignAssocElement", "assignAssocLiteral",
		"assignChainedElement", "assignCompoundBodyIntoElement", "assignCompoundVariable",
		"assignElemLiteral", "assignElementMember", "assignFlaggedSpan", "assignFlaggedTableElement",
		"assignPositional", "assignSpan", "assignSubscriptedCompound", "assignTableCompoundBase",
		"assignValue", "assignWholeArraySubscript", "assocDeclared", "assocsAt", "tracedElementPosition",
		"clearAttributesAReCreationDrops", "clearTypeAttributes", "compoundAssignedFromAName",
		"disciplineWrite", "elementValue", "endHiddenWatch", "endProducerUnsetEnded",
		"fatal", "fatalQuiet", "fixedContainerKeepsItsKind", "fixedSlotRefusesABareArrayLiteral",
		"flaggedAssignPlace", "frozenNameTakesACompoundBody", "frozenScalarRetyped",
		"globbedElementIsAList", "globbedElementValue",
		"indirectElement", "isCompoundVariable", "isNameref", "markForAllexport",
		"namerefArrayLiteralTarget", "namerefCompoundBodyStore", "namerefTarget",
		"referredToParameter", "refuseNamerefAim", "refuseUnaimedReference",
		"refusesEmptyAssignSubscript", "resolvedSubscript", "scalarAssignValue",
		"scalarStoreReplacesACompound", "setArrayElem", "setAssocElem", "setVarAs",
		"spanIsBelowTheFirstElement", "spanReplacesElements", "spliceCharacterSpan",
		"spliceElementSpan", "storeGlobbedScalarAssign", "storePlainScalar", "storesAPlainScalar",
		"subscriptAsWritten", "subscriptSplicesCharacters",
		"suppressDiscipline", "tableRefusesAScalarStore",
		"unaimedReferenceBase", "watchHiddenKinds", "appendArrayElem", "arrayElemsOfTheName",
		"diag", "retypingFrozen", "unspecified", "assignmentGaveUp",
		// The freeze, which a plain store never meets: a frozen name carries
		// an attribute and a private one a shield (#6674).
		"heldFreeze", "refuseAFrozenAssignment", "refuseTheHeldFreeze",
		// And what it is spoken as through a reference (#6763), on the same road.
		"frozenWriteSpokenAs",
		// The element road only — storesAPlainScalar refuses every subscript (#6604).
		"armAssignSubscript", "expanded", "valueAheadOfTheSubscript",
		// A refused element's subscript as written (#6762), on the same road.
		"writtenIndex",
		// An indexed element's two readings of its subscript, and the
		// record of a prefix walk that made them ahead of the store — the
		// element road, which storesAPlainScalar refuses (#6771).
		"indexedAssignSubscriptText", "indexedAssignPosition", "subscriptReadAhead", "indexReadAhead",
		// A dotted name's missing parent — storesAPlainScalar takes only an
		// identifier, which has no dot in it (#6628).
		"refuseAStoreWithNoParent",
	},
	"setVarAs": {
		"registeredGuard", "attributeFolded", "compoundMemberThroughAReference",
		"compoundVariableRetyped", "disciplineWrite", "endProducerUnsetEnded", "fatal",
		"fatalQuiet", "forgetEveryHashedCommand", "ignoredNamesAssigned", "indirectElement",
		"localizeMemberWrite", "markForAllexport", "mirrorScalarToArray", "nameIsBack",
		"namerefAssignmentTarget", "namespaceWriteName", "noteHiddenWrite", "pipelineStatuses",
		"producersLookup", "refuseReadonly", "refuseTheValue", "refuseUnaimedReference", "scalarOverCompound",
		"scopeWarningsAreOff", "selfNameref", "selfNamerefAssignment", "settleCtypeBeforeChanging",
		"storeIntoSelectedFrame", "storeThroughNamerefElement", "tiedUniqueFold", "unaimedReferenceBase",
		"varsPut", "warnAboutTheScope", "assigned", "registeredAction", "attrClear", "releaseEnvCopy",
		"declaringWithoutAValue", "registeredArrayWriter", "dynamicWriters", "inBuiltin", "optindAssigned",
		"storingSelfNameref", "underscoreWritten", "viewIsAlreadyFolded",
		// OPTIND only, which storesAPlainScalar refuses by name (#6633, #6653).
		"optindCursorScope", "optScanInDoubt",
		// `0` only, which is no identifier and so never a plain store (#6809).
		"namesAPosition", "storeDollarZero",
		// A name with a declared base only, which carries the integer
		// attribute and so is never a plain store (#6758).
		"producedNumberOfRendered",
	},
}

// TestTheFullRoadOfAnAssignmentIsTheOneThePlainStoreWasCheckedAgainst fails
// when what `assign` or `setVarAs` reaches on the runner changes. See theFullRoad.
func TestTheFullRoadOfAnAssignmentIsTheOneThePlainStoreWasCheckedAgainst(t *testing.T) {
	t.Parallel()
	f, err := parser.ParseFile(token.NewFileSet(), "runner.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Body == nil {
			continue
		}
		want, ok := theFullRoad[fn.Name.Name]
		if !ok {
			continue
		}
		found[fn.Name.Name] = true
		calls := map[string]bool{}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "r" {
				calls[sel.Sel.Name] = true
			}
			return true
		})
		var got []string
		for name := range calls {
			got = append(got, name)
		}
		slices.Sort(got)
		w := slices.Clone(want)
		slices.Sort(w)
		added, removed := difference(got, w), difference(w, got)
		if len(added) > 0 || len(removed) > 0 {
			t.Errorf("%s now reaches [%s] and no longer [%s]: read interp/plainstore.go's storesAPlainScalar against "+
				"the change, then update theFullRoad", fn.Name.Name, strings.Join(added, ", "), strings.Join(removed, ", "))
		}
	}
	for name := range theFullRoad {
		if !found[name] {
			t.Errorf("found no method %s in runner.go: the guard is looking in the wrong place", name)
		}
	}
}

func difference(a, b []string) []string {
	var out []string
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}

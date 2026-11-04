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

// thePlainLineSetup is every method and field each declaration utility's full
// road reaches on the runner before its operand loop — the setup a plain
// line's route leaves out — as interp/plainline.go was checked against it.
//
// The route skips these for a line of plain names over nothing on the
// argument that each of them does nothing for such a line, so a call added
// here is one the route does not make. When this test fails, read the new
// call against plainLineWords and plainLineRunner: decide whether it can do
// anything for a line every operand of which is a plain name, with or without
// a value, that the empty shadow takes; refuse such a line in the route's
// gate if it can; and then update the list.
var thePlainLineSetup = map[string][]string{
	"localLine": {
		"ask", "badNamesHeldForTheReturn", "bareLocalListing", "builtinNames", "ctl",
		"declarationEvaluatesItsValue", "declarationListing", "declarationOperand", "declareNames",
		"declarePlacedLiteralsAlone", "declarePrint", "declarePrintPerformsItsOperands", "declaringPrivate",
		"endAfterABadName", "endHiddenWatch", "filteredPrintListing", "localDeclarationOptions",
		"localDeclarationWord", "localListingIsTheRunningCallsOwn", "localOutsideAFunction",
		"localUnderCommandPrefix", "noteTheHideLetter", "parseDeclareFlags", "plainLocalLine",
		"privateDeclarationRan", "refuseTheValue", "refusedLineDeclaresItsLiterals",
		"restrictedRefusalStatus", "saveOptionsForThisCall", "scopes", "sem",
		"signAloneIsNotAPrivateDeclaration", "status", "tieDeclaration", "unspecified",
		"watchDeclarationKinds",
	},
	"biDeclare": {
		"ask", "declareInTheKeywordCallersScope", "declareNames", "fatalUsageQuiet", "inBuiltin",
		"listsOneElementPerLine", "parseDeclareFlags", "refusedLineDeclaresItsLiterals", "sem", "status",
	},
	"declareNames": {
		"LocatesFunctions", "ask", "badNameWordedAs", "bareDeclarationListing", "builtinComplaintName",
		"builtinNames", "caseFoldHeld", "caseFoldMayBeHeld", "declarationEvaluatesItsValue",
		"declarationListing", "declarationOperand", "declareFunctions", "declareMapping", "declareMatching",
		"declareMove", "declareOptionTakesANumber", "declarePrint", "declarePrintPerformsItsOperands",
		"declareType", "diag", "diagf", "dottedOperandRefusedByALetter", "dottedOperandRefusedWithAValue",
		"emptyNameIsASelfReference", "endHiddenWatch", "errf", "filteredPrintListing",
		"functionAttributeLettersRemoved", "functionAttributeLettersWritten", "functionLineNamesNoFunction",
		"functionLineUnderBothSigns", "functionsHoldingATraceMark", "functionsHoldingAttributes",
		"globalDeclarationRunsOnTheShellsOwnCell", "globalDeclarationRunsUnderItsOwnPrefix",
		"globalLetterHere", "line", "listingWithoutValues", "literalOperands", "markUndefinedFunctions",
		"markedFunctionListing", "markingLetters", "name", "noteTheHideLetter", "plainTypesetLine",
		"plusLettersSelectNothing", "refusalSpokenAs", "refuseWithUsage", "rereadingAQuotedLiteral",
		"restrictedRefusalStatus", "retypingFrozen", "scopes", "sem", "setFunctionAttributes",
		"setFunctionTraceMarks", "status", "tieDeclaration", "traceLettersWritten", "unanswered",
		"unspecified", "watchDeclarationKinds",
	},
}

// TestThePlainLineSkipsOnlyWhatItWasCheckedAgainst fails when the setup the
// route for a plain declaration line leaves out changes. See
// thePlainLineSetup.
func TestThePlainLineSkipsOnlyWhatItWasCheckedAgainst(t *testing.T) {
	fset := token.NewFileSet()
	bodies := map[string][]ast.Stmt{}
	for _, file := range []string{"builtin.go", "declarebuiltin.go"} {
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && thePlainLineSetup[fn.Name.Name] != nil {
				bodies[fn.Name.Name] = fn.Body.List
			}
		}
	}
	for name, want := range thePlainLineSetup {
		body, ok := bodies[name]
		if !ok {
			t.Errorf("found no function %s: the guard is looking in the wrong place", name)
			continue
		}
		// Up to the operand loop: the first range over the operands at the
		// top of the body. biDeclare has none and is its setup whole, but for
		// the route itself.
		calls := map[string]bool{}
		for _, s := range body {
			if rs, ok := s.(*ast.RangeStmt); ok {
				if id, ok := rs.X.(*ast.Ident); ok && id.Name == "args" {
					break
				}
			}
			if strings.Contains(nodeText(s), "typesetPlainLine") {
				continue
			}
			ast.Inspect(s, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == "r" {
						calls[sel.Sel.Name] = true
					}
				}
				return true
			})
		}
		var got []string
		for c := range calls {
			got = append(got, c)
		}
		slices.Sort(got)
		want = slices.Clone(want)
		slices.Sort(want)
		if len(got) == 0 {
			t.Errorf("%s reaches nothing before its operand loop: the guard is looking in the wrong place", name)
		}
		if added, removed := difference(got, want), difference(want, got); len(added) > 0 || len(removed) > 0 {
			t.Errorf("%s's setup now reaches [%s] and no longer [%s]: read interp/plainline.go against the "+
				"change, then update thePlainLineSetup", name, strings.Join(added, ", "), strings.Join(removed, ", "))
		}
	}
}

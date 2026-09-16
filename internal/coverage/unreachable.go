// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package coverage

// Unreachable is an element of the surface that no case can mention, together
// with the measurement that says so.
//
// This is the one hand-kept list in the package, and it is kept on the terms
// the package comment sets for everything else. An element here is not
// forgiven — it is *measured rather than covered*, which is a different fact
// and one the report prints beside the work-list rather than folding into
// it. So each entry carries the issue that holds the evidence and the
// measurement itself, where the next reader meets it instead of re-deriving
// it; and the report checks every entry against the columns it was handed:
// an entry some case mentions after all, or one no dialect's surface holds
// any more, is printed as **stale** rather than quietly subtracted. A ledger
// that could only grow would be the overstatement this instrument exists to
// prevent, arriving by a side door.
//
// What does not belong here is an element the instrument itself made
// unreachable — a constant no node can carry, a token that never reaches the
// tree. Those are defects in the enumeration and are fixed there, the way
// [OperatorTypes] leaves out the grammar readings (#3258) and
// [operatorElementKind] leaves out the lexer's tokens.
type Unreachable struct {
	Element Element
	// Issue is where the measurement is recorded in full.
	Issue int
	// Measured is the measurement, short enough to print.
	Measured string
}

// UnreachableByConstruction is the ledger. See [Unreachable].
var UnreachableByConstruction = []Unreachable{
	{
		Element: Element{Kind: KindBuiltin, Name: "suspend"},
		Issue:   3259,
		Measured: "ksh93 93u+ 2012 stops on `suspend`, `suspend x` and `suspend -q` " +
			"alike, and zsh 5.9.2 stops on a bare `suspend` (re-measured 2026-09-16, " +
			"each probe in a process group of its own, killed from outside). " +
			"suite.OnlyHere runs every dialect tier under every reference on the " +
			"machine and core/ and ext/ are cross-checked the same way, so a line " +
			"that runs `suspend` stops the harness rather than failing it; the " +
			"corpus has the same property (dialect/bash/suspend_test.go). " +
			"`if false; then suspend; fi` would retire the element and ask nothing, " +
			"and a `$BASH_VERSION` guard would make the line alone by the guard " +
			"rather than by the shell, which is the grade OnlyHere exists to give.",
	},
}

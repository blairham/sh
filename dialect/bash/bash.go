// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package bash answers the substrate's questions the way bash 5 does.
package bash

// procSubstBodyLaidOut is a process substitution's body in a subscript, laid
// out — see syntax.Dialect.ProcSubstInSubscriptIsReprinted. A named function
// rather than a closure, so two Dialect values hold the same one and compare
// equal by identity.
func procSubstBodyLaidOut(body string) (string, bool) {
	return FunctionLayout().CommandSubstitutionIsReprinted(body)
}

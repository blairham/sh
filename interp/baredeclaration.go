// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The scalar sibling of compounddeclaredonly.go: a name a declaration brought
// into being with no letters, no value and nothing in it.
//
//	$ bash -c 'typeset xyz; typeset -p xyz; echo "st=$?"; echo "[${xyz-unset}]"'
//	declare -- xyz
//	st=0
//	[unset]
//
// Two facts at once, and only one of them is a value: the expansion fires its
// default because the name holds nothing, and the listing writes a row
// because the declaration was recorded. Every other path in this shell had
// one of the two and could not have both — an attributed operand leaves a
// letter in an attribute table for declarationOf to find, and a dialect that
// sets a declared name empty leaves a value in Vars — so the *unattributed*
// operand fell through leaving nothing, and `typeset -p xyz` answered `xyz:
// not found` at status 1. That is no column's answer: the two shells that
// record say 0 with a row, and the one that does not says 0 in silence
// (#2999).
//
// Whether the record is kept at all is the dialect's, because the panel
// splits three ways over it — see Semantics.ValuelessDeclarationRecordsTheName
// for the measurement and for why it is a second question rather than a
// second reading of DeclaredNameWithoutValueIsEmpty.
//
// # Why the record lives with the attributes
//
// It is kept in nameAttributes rather than in a map of its own, and that is
// the point rather than an economy. Everything this record has to do is
// something the attribute tables already do:
//
//	a local declaration starts it over    the shadow calls dropNameAttributes
//	the call's end gives the outer back   the scope saved captureAttributes
//	`unset` takes it away                 clearAttributes is that plus one line
//
// A map beside them would have needed all three wired again, in the three
// declaration loops and the two scope kinds, and localattributes.go says in
// so many words what happens when a per-name record is added to the runner
// without a line there: it is remembered in one place and forgotten in the
// other. The listing is the only reader either way.
//
// It is not written back as a letter. attributeLetters and every listing form
// read the fields beside it and none of them read this one, which is what
// makes the row `declare -- xyz` rather than a letter nobody spells.

// recordBareDeclaration notes that a declaration named this name with no
// attribute letters and no value, in a dialect where that leaves the name
// unset rather than empty.
func (r *Runner) recordBareDeclaration(name string) {
	setBool(&r.declaredBare, name, true)
}

// bareDeclarationListed reports whether the listing has such a record to write
// for this name. It is the only reader.
func (r *Runner) bareDeclarationListed(name string) bool {
	return r.declaredBare[name]
}

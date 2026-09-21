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
// # Two records, one state
//
// `unset` of a local the running scope declared arrives at the same state
// from the other side — the binding stands, its value and its letters are
// gone — so it is the same listing and the same axis, and
// Runner.unsetLeftItDeclared is where that route's record lives. A second
// bool rather than a second write of the one above, because one reader has
// to tell them apart: the `unset` that falls through to the *function* table
// counts a declaration as a parameter and does not count this. Everything
// else here is shared, the scope's save and restore included.
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
// for this name. It is the only reader, and so it is where the axis is asked.
//
// Not where the record is *made*, and that is the placement rather than an
// accident of it. A declaration with no value and no letters is a line every
// dialect runs — `local u` is in dash and in BusyBox ash as much as in
// bash — and an axis asked there would be an axis four dialects have to
// answer before they can run a script that says it. Where the panel actually
// disagrees is one step later, over what a listing does with the name, and
// a dialect with no declaration listing never arrives.
//
// The `&&` is load-bearing: the ask only happens for a name that has such a
// record, so a dialect that never makes one is never asked.
func (r *Runner) bareDeclarationListed(name string) bool {
	return r.hasAValuelessRecord(name) &&
		r.ask(r.sem().ValuelessDeclarationRecordsTheName,
			"what a listing does with a name declared with neither a value nor an attribute")
}

// hasAValuelessRecord reports whether either route into the state has left a
// record for this name — a declaration that carried neither a value nor an
// attribute, or an `unset` of a local the running scope declared.
//
// The two are two fields for one reader and are one question everywhere else;
// see "Two records, one state" above. Spelled once so that the second reader
// — the `-p` that has to tell a name it still has from one it has never heard
// of — cannot come to disagree with the first about which names are in it.
func (r *Runner) hasAValuelessRecord(name string) bool {
	return r.declaredBare[name] || r.unsetLeftItDeclared[name]
}

// valuelessRecordIsStillAName is the question a `-p` asks about such a record
// in a dialect whose listing writes **no row** for it: is the name one the
// shell still has — written as nothing, at 0 — or one it has never heard of,
// which is the missing-name route.
//
// A third state, and it took a dialect other than the one the record was
// built for to make it reachable. Measured 2026-09-21, `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME, from a script file, on
// zsh 5.9.2:
//
//	v=global; f() { local v; unset v; typeset -p v; echo "st=$?"; }; f
//	                            nothing, st=0 — and `${v-UNSET}` is UNSET,
//	                            so the outer value stays hidden
//	f() { local nov; unset nov; typeset -p nov; }
//	                            the same, with nothing underneath it
//	typeset -p nosuchvar        `no such variable: nosuchvar` at 1
//
// So that shell distinguishes a name it has never heard of from a local whose
// value an `unset` took, and neither answer to
// ValuelessDeclarationRecordsTheName is the second one: `Yes` writes a row and
// `No` takes the missing-name route, which in this dialect is the refusal the
// third line shows. Hence a field of its own rather than a third value on
// that one — what they answer is different, one being what the *listing*
// writes and this being whether the **name is there at all**.
//
// It is narrow by construction: the `&&` means a dialect that makes no such
// record is never asked, and a dialect whose listing writes a row for it
// never arrives, because the row is what `known` then is (#4053).
func (r *Runner) valuelessRecordIsStillAName(name string) bool {
	return r.hasAValuelessRecord(name) &&
		r.ask(r.sem().ValuelessRecordIsStillAName,
			"whether a `-p` knows a valueless record this dialect's listing writes no row for")
}

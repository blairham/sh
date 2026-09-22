// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// TypeExternalSentence is the line `type`, `command -V` and one dialect's
// `whence` write for a name that resolves to a file.
//
// One function because there are **two** wordings and the choice between them
// is a fact about the operand rather than about the caller: a word that
// already held a slash was never searched for, so a dialect whose sentence
// says how the name was found has nothing to say about it. See
// Diagnostics.TypePathnameOperand, which is empty in every column whose two
// wordings are the same string.
//
// Exported because the dialect with the second wording has a `whence` of its
// own, and a copy of this choice there is a copy that would drift: it already
// had the tracked-alias sentence written out as a literal in two places, and
// the pathname operand was wrong in both.
func (r *Runner) TypeExternalSentence(name, path string) string {
	_, hashed := r.hashedCommandPath(name)
	return r.typeExternalSentence(name, path, hashed)
}

// typeExternalSentence is that line with the command hash read as it stood
// **before** the lookup this sentence is about.
//
// The distinction is measured rather than tidy: dash's own `type ls` puts the
// name in the table and still writes the plain sentence for that same
// lookup — `hash` afterwards shows `/bin/ls` and the line said `ls is
// /bin/ls` — so a report that asked the table after its own search would
// never write the first of the two sentences at all. See
// Diagnostics.TypeHashedExternal.
func (r *Runner) typeExternalSentence(name, path string, hashed bool) string {
	pathname := strings.ContainsRune(name, '/')
	// The name as this dialect writes a word it could not otherwise write
	// bare, which is the same function the not-found sentence uses and is
	// empty in every column but one. The path arrived quoted already, from
	// Runner.reportedPath, because the bare-path forms write it with no
	// sentence around it and the two must agree — measured 2026-09-18 on
	// ksh93u+ 2012-08-01: with a directory holding a file called `a b` on
	// PATH, `whence -v 'a b'` is `'a b' is a tracked alias for '/…/a b'` and
	// `whence 'a b'` is `'/…/a b'`, and a plain name whose *path* holds a
	// blank quotes the path alone. So they are two words each written back
	// the same way rather than one quoted sentence.
	//
	// After the slash test, which is a fact about the operand as written.
	name = r.NameReportWord(name)
	// And the path again, for the one column where the sentence and the
	// bare-path forms *disagree*: zsh 5.9.2 quotes the path inside the
	// sentence and writes it plain for `command -v`, `whence`, `where` and
	// `which`. Asked here rather than in Runner.reportedPath because that is
	// what the disagreement is — the sentence is the only route that quotes.
	// Empty in every column whose two routes agree, ksh93's included, where
	// the quoting is Diagnostics.NameReportQuoting's and is already on the
	// path by the time it arrives here.
	path = r.typeSentencePathWord(path)
	if pathname {
		if w := r.diag().TypePathnameOperand; w != "" {
			return Wording(w, "%[1]s is %[2]s", name, path)
		}
	}
	if w := r.diag().TypeHashedExternal; w != "" && hashed {
		// The third wording, in the two columns that have one. Behind the
		// slash, because a word that was never searched for is never in the
		// table either.
		return Wording(w, "%[1]s is %[2]s", name, path)
	}
	return Wording(r.diag().TypeExternal, "%[1]s is %[2]s", name, path)
}

// typeSentencePathWord is the path a `type`-family sentence writes, spelled
// the way the dialect writes a word it could not otherwise write bare.
//
// The same alphabet NameReportWord reads, because it is the same shell's own
// word-writing function — measured character for character rather than
// assumed. See Diagnostics.TypeSentencePathQuoting.
func (r *Runner) typeSentencePathWord(path string) string {
	d := r.diag()
	return r.traceQuote(path, d.TypeSentencePathQuoting, d.TraceMetacharacters)
}

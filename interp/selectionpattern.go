// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A `-m` operand is a pattern that selects names out of a table — parameters,
// functions, aliases, hashed commands, named directories — rather than a
// pattern matched against a subject the script supplied. The difference is
// what this file is for: at a matching surface a pattern the engine cannot
// read is a miss, and at a selection surface it is a **refusal**, because
// there is no subject for it to miss against and "no names" is exactly what a
// correct pattern with nothing to pick out also says.
//
// That is not a nicety. A script that mistyped a bracket is told its table is
// empty, at 0, by a shell that could not read the question — a null nobody
// downstream can falsify (#4741).

// badSelectionPattern reports whether a `-m` operand will not compile as a
// pattern.
//
// Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0), 2026-09-26, `-f` under
// `env -i PATH=/usr/bin:/bin`, one `typeset -m <pattern>` per row. The 31
// rows the scan below was written against, `1` being `bad pattern : <p>`:
//
//	[  1    [a  1    []  0    []a 0    [!] 0    [!  1    a[b 1
//	[a- 1   [)] 0    [(] 0    a\[b 0   \[  0
//	[[:alpha:]  1    [[:alpha:]] 0
//	a(b 1   (   1    )   1    a)b 1    (a|b  1  (a|b) 0
//	((a)) 0 (a)) 1   [a](b 1  [a](b) 0 a\(b 0   a\)b 0
//	*   0   x   0    (#i)a 0  <1-3> 0  {a,b} 0
//
// Two of those rows are the reason this is its own scan rather than
// [hasUnterminatedBracket] with a caller:
//
//   - `[]` and `[]a` are **valid** here, so the POSIX rule that the first `]`
//     after the opening bracket is an ordinary character — which
//     hasUnterminatedBracket implements, and which the *matching* surfaces
//     need, since `[]]` matches `]` in the same shell — does not hold for
//     this question. Validity and matching are two scans of one pattern
//     because the shell measurably answers them differently.
//   - `[[:alpha:]` is **invalid**, so a `[:name:]` inside a bracket is one
//     element whose own `]` does not close the bracket around it. That is the
//     case hasUnterminatedBracket explicitly does not see; see
//     bracketLeftOpenBySubExpression, which is the same reading reached from
//     the matching side.
//
// A backslash protects the character behind it on every row measured, which
// is why `a\(b` and `a\[b` are patterns and `a(b` and `a[b` are not.
func badSelectionPattern(p string) bool {
	depth := 0
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '\\':
			i++
		case '[':
			end := selectionBracketEnd(p, i)
			if end < 0 {
				return true
			}
			i = end
		case '(':
			depth++
		case ')':
			// A close with nothing open is refused on its own — `a)b` is a
			// bad pattern here where the same word is ordinary text at a
			// matching surface, measured — so this is not the mirror of the
			// unterminated-group test but a rule of its own.
			if depth == 0 {
				return true
			}
			depth--
		}
	}
	return depth != 0
}

// selectionBracketEnd is the index of the `]` closing the bracket that opens
// at i, or -1 when nothing closes it.
//
// No leading-`]` exception and no `!`/`^` step, because neither is needed for
// the question: the first `]` closes, so `[]` and `[!]` are both terminated
// whether or not the character before the `]` was read as negation.
func selectionBracketEnd(p string, i int) int {
	for j := i + 1; j < len(p); j++ {
		switch p[j] {
		case '\\':
			j++
		case '[':
			// `[:alpha:]`, `[.x.]` and `[=x=]` are single elements and the
			// `]` ending one does not end the bracket holding it.
			if j+1 < len(p) {
				if delim := p[j+1]; delim == ':' || delim == '.' || delim == '=' {
					end := elementEnd(p, j+2, delim)
					if end < 0 {
						return -1
					}
					j = end
				}
			}
		case ']':
			return j
		}
	}
	return -1
}

// elementEnd is the index of the `]` ending a `[:name:]`-shaped element whose
// body starts at i, or -1.
func elementEnd(p string, i int, delim byte) int {
	for j := i; j+1 < len(p); j++ {
		if p[j] == delim && p[j+1] == ']' {
			return j + 1
		}
	}
	return -1
}

// refusedSelectionPattern reports whether a `-m` operand is a pattern this
// dialect will not compile, writing the complaint when it is.
//
// Keyed on [Semantics.UnterminatedBracket] rather than on a new axis, and on
// the same value the matching surfaces read: `BracketBadPattern` is the
// dialect saying that a pattern which will not compile is an error rather
// than a miss, and this is that same answer asked where there is no subject.
// A dialect that calls an unterminated bracket a literal or a class matching
// nothing has no uncompilable pattern to refuse.
//
// The complaint only. The status is the caller's, because the four surfaces
// do not agree about what a *good* pattern's status is — `alias -m` with no
// match is 0 and `unhash -m` with no match is 1 — and a refusal there is 1 in
// both, which the callers already reach by their own route.
func (r *Runner) refusedSelectionPattern(pattern string) bool {
	if r.sem().UnterminatedBracket != BracketBadPattern || !badSelectionPattern(pattern) {
		return false
	}
	// A space before the colon, which is not the `bad pattern: %s` the same
	// dialect writes for `print -r -- a(b`. Two sentences one byte apart,
	// measured at each surface rather than shared with that one — see
	// Diagnostics.BadSelectionPattern.
	r.diagf("%s\n", Wording(r.diag().BadSelectionPattern, "bad pattern : %s", escapeControlBytes(pattern)))
	return true
}

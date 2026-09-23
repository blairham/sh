// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// What a POSIX extended regular expression is, over and above what the engine
// underneath will compile.
//
// `regexp` is RE2 and accepts a wider language than ERE in two directions that
// matter: it reads `(?` as the opening of a flag or non-capturing group, where
// ERE has a `?` with no operand and the panel refuses the expression outright;
// and it takes an empty branch in an alternation, which the panel refuses as
// well. Both are silent acceptances of something no shell in the panel runs,
// which is the failure mode this file exists to close — a script written
// against `(a|)` gets a status of 2 from every real shell and got 0 here.
//
// The other direction — ERE constructs the engine has no answer for — is
// unsupportedERE's, which is a separate question and a separate file.

// errNotAnERE is a pattern the engine would compile and a POSIX ERE is not.
// The reason travels with it, because the panel writes one and they are a
// fixed vocabulary rather than this shell's words: see regexReason.
type errNotAnERE struct{ reason string }

func (e errNotAnERE) Error() string { return e.reason }

// The reasons, which are POSIX regcomp's own and are what every column in the
// panel writes. Named here so the two callers cannot drift and so the set is
// readable as the vocabulary it is.
const (
	regexBadRepeat    = "repetition-operator operand invalid"
	regexEmptySubExpr = "empty (sub)expression"
	regexBadCollate   = "invalid collating element"
	regexBadBrace     = "braces not balanced"
	regexBadRange     = "invalid character range"
)

// validERE reports what the graded reference would refuse about a pattern the
// engine would take, or nil where the two agree.
//
// **Measured against the reference the suite is graded in**, which is bash
// 5.3.15 in the digest-pinned `debian:sid-slim` the `bash's own suite` job
// runs, and *not* only against the bash on this machine. The two C libraries
// disagree about three of these, and the first reading of this file took the
// macOS answer for all three and was wrong in CI for two of them:
//
//	pattern     glibc          BSD (macOS)   what RE2 does
//	(?:a)bc     refuses        refuses       a non-capturing group
//	(?i)abc     refuses        refuses       a flag group
//	a{x         refuses        refuses       a literal brace
//	(a|)        **accepts**    refuses       accepts
//	(|a), a|    **accepts**    refuses       accepts
//	()          accepts        accepts       accepts
//
// So an empty branch is **not** refused here: glibc takes it, and a rule
// written from the macOS answer turned six patterns the graded column runs
// into a status of 2. That is the "derived column is not measured" trap, and
// the container run is one command (#4173).
//
// `a**` and `a{1}{2}` are the other side of the same coin — glibc accepts a
// repetition of a repetition and RE2 refuses it — and they are handled by
// rewriting rather than by refusing. See asERE.

// condRegexMark marks a byte of a `=~` operand that the **script quoted**, so
// that the readers below know it is a character and not syntax.
//
// The same NUL convention, and for the same reason, as syntax.ArithValueMark:
// two readings of one text that cannot be told apart once the text exists. A
// quoted `.` and a quoted `]` are ordinary characters wherever they stand,
// and inside a bracket expression they are ordinary *members* — where a
// backslash written by the script is a member in its own right. Escaping the
// quoted one with a backslash, which is what a plain `regexp.QuoteMeta` does,
// makes the two identical and the bracket rewriter then reads the escape as
// that member. So the quoting is carried rather than spelled (#4173).
//
// A mark in front of a mark is a NUL the value held.
const condRegexMark = 0

// quotedAt reports the rune the script quoted at this offset, and how many
// bytes the mark and it take together.
func quotedAt(pat string, i int) (string, int, bool) {
	if i >= len(pat) || pat[i] != condRegexMark {
		return "", 0, false
	}
	if i+1 >= len(pat) {
		return "", 1, true
	}
	_, w := utf8.DecodeRuneInString(pat[i+1:])
	return pat[i+1 : i+1+w], 1 + w, true
}

// unmarkedHere steps past a mark, because **a bracket expression takes the
// quoting off and nothing more**.
//
// Outside one a character the script quoted reaches the engine escaped, so
// `\.` is the character and not the operator. Inside one every character is a
// member already, and the reference does not protect the two that are still
// syntax there: measured 2026-09-23 against bash 5.3.15 in the graded image,
// `[[ a]  =~ [\a\]] ]]` is 0 with `BASH_REMATCH` `a]` — so the quoted `]`
// closed the set and left a literal `]` behind it, exactly as `[a]]` would.
// `a`, `]`, `\`, `\a` and `\]` against the same expression are all 1 there,
// which is what says the set holds `a` alone. Protecting the member instead —
// writing `\]` for it — made every one of those 0 here.
func unmarkedHere(pat string, i int) int {
	if i < len(pat)-1 && pat[i] == condRegexMark {
		return i + 1
	}
	return i
}

// unmarkRegex is the pattern as the script wrote it, for a message that quotes
// it back.
func unmarkRegex(pat string) string {
	if strings.IndexByte(pat, condRegexMark) < 0 {
		return pat
	}
	var b strings.Builder
	for i := 0; i < len(pat); {
		if ch, w, ok := quotedAt(pat, i); ok {
			b.WriteString(ch)
			i += w
			continue
		}
		b.WriteByte(pat[i])
		i++
	}
	return b.String()
}

func validERE(pat string) error {
	// Where an atom would have to stand: the start of the expression, just
	// inside a `(`, and just after a `|`. A repetition operator there has
	// nothing to repeat, which is the one refusal both libraries make and RE2
	// does not — `(?` is that shape, the `?` standing where an atom belongs.
	atomWanted := true
	for i := 0; i < len(pat); {
		if _, w, ok := quotedAt(pat, i); ok {
			// A character the script quoted is an atom and never syntax.
			i += w
			atomWanted = false
			continue
		}
		switch c := pat[i]; c {
		case '\\':
			// The escape and whatever it protects are one atom. A trailing
			// backslash is the engine's own refusal and is left to it.
			_, w := utf8.DecodeRuneInString(pat[i+1:])
			i += 1 + w
			atomWanted = false
			continue
		case '[':
			i = skipEREBracket(pat, i)
			atomWanted = false
			continue
		case '(':
			atomWanted = true
		case ')':
			atomWanted = false
		case '|':
			// An empty branch is accepted by the graded reference, so nothing
			// is refused here. The flag is moved because what follows a `|` is
			// the start of a branch, where a repetition operator still has
			// nothing to repeat.
			atomWanted = true
		case '^', '$':
			atomWanted = true
			// **An anchor is not an atom**, so a repetition operator behind
			// one has nothing to repeat. Measured 2026-09-23 against bash
			// 5.3.15 in the graded image: `^*`, `^+`, `^?`, `^{2}`, `$*`,
			// `a^*` and `a$*` are all 2 there and were all 0 here, RE2 being
			// content to repeat a zero-width assertion.
		case '*', '+', '?':
			if atomWanted {
				return errNotAnERE{regexBadRepeat}
			}
		case '{':
			if atomWanted {
				return errNotAnERE{regexBadRepeat}
			}
			end, ok := readInterval(pat, i)
			if !ok {
				// A `{` behind an atom that is not an interval: the reference
				// reads every one of them as the start of a bound and refuses
				// the pattern, where RE2 reads a literal brace. Measured on
				// both libraries — `a{x` is 2 in each and 0 here.
				return errNotAnERE{regexBadBrace}
			}
			i = end
			continue
		default:
			atomWanted = false
		}
		i++
	}
	return nil
}

// readInterval is the extent of a bound — `{n}`, `{n,}` or `{n,m}` — and
// whether the brace opened one at all.
func readInterval(pat string, at int) (int, bool) {
	i := at + 1
	digits := 0
	for i < len(pat) && pat[i] >= '0' && pat[i] <= '9' {
		i++
		digits++
	}
	if i < len(pat) && pat[i] == ',' {
		i++
		after := 0
		for i < len(pat) && pat[i] >= '0' && pat[i] <= '9' {
			i++
			after++
		}
		// `{,m}` is an interval with no lower bound written, which the graded
		// reference reads as `{0,m}` — measured, `[[ a =~ a{,2} ]]` is 0 there
		// and was 2 here, RE2 reading the braces as text. One side or the
		// other has to hold a digit.
		if digits == 0 && after == 0 {
			return 0, false
		}
	} else if digits == 0 {
		return 0, false
	}
	if i < len(pat) && pat[i] == '}' {
		return i + 1, true
	}
	return 0, false
}

// skipEREBracket is the extent of one bracket expression, so the scan above reads
// no metacharacter inside one as syntax.
//
// A `]` immediately after the opening — or after the negating `^` — is the
// character and not the closer, which is POSIX's rule and is what makes `[]]`
// the set holding one bracket. A `[:name:]`, `[.c.]` or `[=c=]` inside it is
// skipped whole, because their own `]` is not the set's either.
func skipEREBracket(pat string, at int) int {
	i := at + 1
	i = unmarkedHere(pat, i)
	if i < len(pat) && pat[i] == '^' {
		i = unmarkedHere(pat, i+1)
	}
	if i < len(pat) && pat[i] == ']' {
		i++
	}
	for i < len(pat) {
		i = unmarkedHere(pat, i)
		if i >= len(pat) {
			break
		}
		switch {
		case pat[i] == ']':
			return i + 1
		case pat[i] == '[' && i+1 < len(pat) && strings.IndexByte(":.=", pat[i+1]) >= 0:
			closer := string([]byte{pat[i+1], ']'})
			end := strings.Index(pat[i+2:], closer)
			if end < 0 {
				// The engine's own `brackets ([ ]) not balanced`, left to it.
				return len(pat)
			}
			i += 2 + end + 2
		default:
			i++
		}
	}
	// Unbalanced, which the engine refuses in the same words the panel does.
	return len(pat)
}

// asERE rewrites the two bracket constructs a POSIX ERE has and the engine has
// not: a **collating element** `[.c.]` and an **equivalence class** `[=c=]`.
//
// In the C locale — the only one this shell's own runs are made in, and the one
// the corpus and the suite are graded under — every character is a collating
// element of itself and an equivalence class of itself, so both reduce to the
// character. Measured 2026-09-23 against bash 5.3.20:
//
//	[[ a =~ [[.a.]] ]]           0, and `b` against it is 1
//	[[ a =~ [[=a=]] ]]           0, and `b` against it is 1
//	[[ a =~ [[.a.]x] ]]          0, and so is `x` — a member beside others
//	[[ b =~ [[.a.]-[.c.]] ]]     0 — a range's end
//	[[ b =~ [[.a.]-c] ]]         0 — and one end of it written either way
//	[[ b =~ [^[.a.]] ]]          0, and `a` against it is 1
//	[[ . =~ [[..]] ]]            2, `invalid collating element`
//
// Every one of those answered **1** here before, silently: RE2 reads `[[.a.]]`
// as the set `[`, `.`, `a` and then two ordinary brackets, so it compiled and
// matched the wrong thing (#4173).
//
// **A name longer than one character is refused**, which is narrower than the
// reference and deliberately so. `[[.space.]]` is a name in the *locale's*
// collating table, and a table of locale names is exactly what this shell does
// not carry — see AGENTS.md on generating rather than importing, and the
// platform split that makes it worse: the name set is the C library's, so it
// differs between the machine a case is measured on and the image the suite is
// graded in. A refusal in the reference's own words is the honest answer for a
// construct this shell does not have; a silent mismatch is what it replaces.
func asERE(pat string) (string, error) {
	if !strings.ContainsAny(pat, "[\\*+?{\x00") {
		// Nothing any of the rewrites could reach: the bracket constructs
		// need a `[`, the escape rule a `\`, the repeated repetition an
		// operator, and a character the script quoted carries a mark that has
		// to come off before the engine sees it. Every one of those was left
		// out of this guard at some point and the rewrite it gates then never
		// ran — the mark most recently, which sent NUL bytes to the engine
		// and turned every quoted operand into a pattern that matched
		// nothing.
		return pat, nil
	}
	var b strings.Builder
	// Where the atom now being written began in the *output*, and whether what
	// was written last was a repetition operator. A second operator behind the
	// first is a repetition of a repetition, which the graded reference takes
	// and RE2 refuses — see wrapRepeatedRepeat.
	atomAt, afterRepeat := 0, false
	// Where each open `(` began in the output, so that the atom a repetition
	// operator behind a group would repeat is the **group** and not its closing
	// parenthesis. Without it `([^a])?{2}` wrapped from the `)` and came out as
	// a pattern neither shell has — found by fuzzing 400 generated expressions
	// against the graded reference, which is the one thing a hand-written
	// battery had not covered (#4173).
	var groups []int
	for i := 0; i < len(pat); {
		if ch, w, ok := quotedAt(pat, i); ok {
			// A character the script quoted stands for itself, so it reaches
			// the engine escaped rather than as the syntax it spells.
			atomAt = b.Len()
			b.WriteString(regexp.QuoteMeta(ch))
			i += w
			continue
		}
		switch {
		case pat[i] == '(':
			groups = append(groups, b.Len())
			atomAt, afterRepeat = b.Len(), false
			b.WriteByte(pat[i])
			i++
		case pat[i] == ')':
			// The atom is the whole group, so a quantifier behind this closer
			// repeats from where the group opened.
			atomAt = b.Len()
			if n := len(groups); n > 0 {
				atomAt = groups[n-1]
				groups = groups[:n-1]
			}
			afterRepeat = false
			b.WriteByte(pat[i])
			i++
		case pat[i] == '\\' && i+1 < len(pat):
			atomAt, afterRepeat = b.Len(), false
			_, w := utf8.DecodeRuneInString(pat[i+1:])
			b.WriteString(escapedOrdinary(pat[i+1 : i+1+w]))
			i += 1 + w
		case pat[i] == '[':
			atomAt, afterRepeat = b.Len(), false
			// **The quoting comes off before the set is read, not inside
			// it.** A bracket expression takes every character as a member,
			// so a mark in there says nothing the set does not already say —
			// and the constructs it holds are several characters long, so a
			// mark between them would hide `[=a=]` from the reader that
			// reduces it. Measured: `[\[[=A=][=a=]]` is an equivalence class
			// beside a literal `[` in the graded reference, and reading it a
			// character at a time left the class standing as three members
			// and a pair of brackets.
			end := skipEREBracket(pat, i)
			if _, err := rewriteBracket(unmarkRegex(pat[i:end]), 0, &b); err != nil {
				return "", err
			}
			i = end
		case pat[i] == '*' || pat[i] == '+' || pat[i] == '?' || pat[i] == '{':
			end := i + 1
			text := pat[i:end]
			if pat[i] == '{' {
				if to, ok := readInterval(pat, i); ok {
					end = to
					text = pat[i:end]
					if text[1] == ',' {
						// `{,m}` written out as the `{0,m}` the reference reads
						// it as, because RE2 reads the braces as text.
						text = "{0" + text[1:]
					}
				}
			}
			if afterRepeat {
				wrapRepeatedRepeat(&b, atomAt)
			}
			b.WriteString(text)
			afterRepeat = true
			i = end
		default:
			atomAt, afterRepeat = b.Len(), false
			b.WriteByte(pat[i])
			i++
		}
	}
	return b.String(), nil
}

// wrapRepeatedRepeat puts the atom and the operator already written into a
// **non-capturing** group, so that a second operator behind them repeats the
// repetition rather than standing where RE2 will not have it.
//
// `a**` becomes `(?:a*)*` and `a{1}{2}` becomes `(?:a{1}){2}`. Measured
// 2026-09-23 against bash 5.3.15 in the digest-pinned image the suite is
// graded in: `[[ abc =~ a** ]]` is 0 there and `[[ abc =~ a{1}{2} ]]` is 1,
// where RE2 refuses both outright and this shell answered 2. The group has to
// be non-capturing or the rewrite would shift every group number behind it,
// and the record a script reads back is numbered.
func wrapRepeatedRepeat(b *strings.Builder, atomAt int) {
	whole := b.String()
	b.Reset()
	b.WriteString(whole[:atomAt])
	b.WriteString("(?:")
	b.WriteString(whole[atomAt:])
	b.WriteString(")")
}

// rewriteBracket copies one bracket expression, reducing the two constructs
// asERE is about, and gives back where it ended.
func rewriteBracket(pat string, at int, b *strings.Builder) (int, error) {
	i := at
	b.WriteByte(pat[i])
	i++
	if i < len(pat) && pat[i] == '^' {
		b.WriteByte(pat[i])
		i++
	}
	if i < len(pat) && pat[i] == ']' {
		// The closer as a member, which has to be written the engine's way.
		b.WriteString(`\]`)
		i++
	}
	// The three things a range needs tracked. A `-` is a range operator only
	// between two members: first or last in the set it is the character, which
	// is POSIX's rule and what `[--/]` and `[-a]` rest on. A **class** may be
	// neither end of one, and a range may not be an end of another. Measured
	// 2026-09-23 against bash 5.3.15 in the graded image: `[[:alpha:]-z]` and
	// `[a-b-c]` are 2 there and were 0 here, and `[--/]` and `[+--]` are
	// ordinary sets in both.
	first := true
	wasClass, wasRange, pendingRange := false, false, false
	member := func(isClass bool) error {
		switch {
		case isClass && pendingRange:
			return errNotAnERE{regexBadRange}
		case pendingRange:
			wasRange, pendingRange = true, false
		default:
			wasRange = false
		}
		wasClass = isClass
		first = false
		return nil
	}
	for i < len(pat) {
		if pat[i] == '-' && !first && i+1 < len(pat) && pat[i+1] != ']' {
			if wasClass || wasRange {
				return 0, errNotAnERE{regexBadRange}
			}
			b.WriteByte(pat[i])
			i++
			wasClass, wasRange, pendingRange = false, false, true
			continue
		}
		switch {
		case pat[i] == ']':
			b.WriteByte(pat[i])
			return i + 1, nil
		case pat[i] == '[' && i+1 < len(pat) && pat[i+1] == ':':
			// A character class, which the engine has of its own.
			end := strings.Index(pat[i+2:], ":]")
			if end < 0 {
				b.WriteByte(pat[i])
				i++
				continue
			}
			if err := member(true); err != nil {
				return 0, err
			}
			b.WriteString(pat[i : i+2+end+2])
			i += 2 + end + 2
		case pat[i] == '[' && i+1 < len(pat) && (pat[i+1] == '.' || pat[i+1] == '='):
			closer := string([]byte{pat[i+1], ']'})
			end := strings.Index(pat[i+2:], closer)
			if end < 0 {
				b.WriteByte(pat[i])
				i++
				continue
			}
			name := pat[i+2 : i+2+end]
			if utf8.RuneCountInString(name) != 1 {
				return 0, errNotAnERE{regexBadCollate}
			}
			if err := member(false); err != nil {
				return 0, err
			}
			b.WriteString(escapeBracketMember(name))
			i += 2 + end + 2
		case pat[i] == '\\':
			// **A backslash inside a bracket expression is an ordinary
			// member**, which POSIX states outright and RE2 reads as an
			// escape. Written back doubled, so the engine reads the character
			// the expression meant. Measured 2026-09-23, the pattern held in a
			// parameter so the shell's own quote removal does not take the
			// backslash first:
			//
			//	r='[d].[\Gg]'; [[ d.G =~ $r ]]   0 — and `\G` is no escape,
			//	                                 where RE2 refuses the pattern
			//	r='[\n]'; [[ n =~ $r ]]          0, and `\` against it is 0
			//	                                 too: the set holds both
			//	r='[\d]'; [[ d =~ $r ]]          0, and `1` against it is 1 —
			//	                                 no digit class in here
			//	r='[a\-z]'; [[ b =~ $r ]]        0, and `-` against it is 1:
			//	                                 the backslash is a range end
			//
			// All four answered the other way here, and the first was refused
			// outright once this file started validating (#4173).
			if err := member(false); err != nil {
				return 0, err
			}
			b.WriteString(`\\`)
			i++
		default:
			if err := member(false); err != nil {
				return 0, err
			}
			b.WriteByte(pat[i])
			i++
		}
	}
	return len(pat), nil
}

// escapeBracketMember writes one character so that it is a member of a bracket
// expression and nothing else: the four the engine reads as syntax there take a
// backslash and every other character stands.
func escapeBracketMember(s string) string {
	if len(s) == 1 && strings.IndexByte(`]\^-`, s[0]) >= 0 {
		return `\` + s
	}
	return s
}

// escapedOrdinary is what a backslash and the character behind it come to,
// written the way the engine must read it.
//
// **A backslash before an ordinary character is that character**, which is
// where POSIX leaves it undefined and where both C libraries the panel is
// measured on agree. RE2 does not: it gives a dozen letters meanings of its
// own, so `\d` was a digit class here and is the letter `d` in every shell in
// the panel. Measured 2026-09-23, each letter of the alphabet in both cases as
// `r='\X'; [[ X =~ $r ]]` and `[[ 7 =~ $r ]]`, against bash 5.3.20 on macOS
// and bash 5.3.15 in the digest-pinned `debian:sid-slim` the suite is graded
// in — 37 rows each:
//
//	every letter but six   the letter itself, and no class: `\d` matches `d`
//	                       and not `7`. Both libraries, every row.
//	b B s S w W            the GNU extensions — a word boundary, a space
//	                       class and a word class. glibc has them, BSD has
//	                       not, and RE2's readings are glibc's: measured cell
//	                       for cell, so they are left to the engine.
//
// The six are the whole of the disagreement between the two libraries, which
// is what makes the rest portable enough to write down. `\0` goes with the
// letters — the literal zero in glibc where RE2 reads a NUL — and `\1` to
// `\9` do not: ERE has no backreference and glibc refuses them, which RE2
// does too, so they are left alone as well.
//
// Every other character keeps its backslash. `\.`, `\*`, `\[` and their
// siblings mean the character in both, which is what the escape is for.
func escapedOrdinary(ch string) string {
	if len(ch) != 1 {
		return `\` + ch
	}
	c := ch[0]
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		if strings.IndexByte("bBsSwW", c) >= 0 {
			// The six the two libraries disagree about, and the six RE2
			// reads the way the one the suite is graded against does.
			return `\` + ch
		}
		return ch
	case c == '0':
		return ch
	case c == '`':
		// The start of the subject, which is what the reference's library
		// makes of it and what RE2 spells `\A`. Measured: a backquote escape
		// in front of `a` matches `a` there and did not here, RE2 having read
		// a literal backquote. `\<` and `\>` — the word-start and word-end
		// anchors of the same family — are left alone: RE2 has one boundary
		// and no way to say which side of a word it is on.
		return `\A`
	case c == '\'':
		// And the end of it, which RE2 spells `\z`.
		return `\z`
	}
	return `\` + ch
}

// regexQuotingIsLiteral reports whether a quoted portion of a `=~` operand is
// matched as text rather than as an expression.
//
// The dialect's own answer, **unless a compatibility level older than the
// release that changed it is in force**. Quoting a `=~` operand became literal
// in bash 3.2; at level 31 the operand is an expression again, quotes and all.
// Measured 2026-09-23 against bash 5.3.15 in the digest-pinned image the suite
// is graded in, `[[ axb =~ "a.b" ]]` unless another pattern is shown:
//
//	default, and level 32 and above    1 — the quoted `.` is a dot
//	BASH_COMPAT=31                     **0**
//	BASH_COMPAT=3.1                    0 — the dot is legibility only
//	shopt -s compat31                  0 — the same state by its other name
//	`[[ axb =~ 'a.b' ]]`               0 at 31 — every spelling of quoting
//	`r='a.b'; [[ axb =~ "$r" ]]`       0 at 31
//	`[[ axb =~ "a"'.'b ]]`             0 at 31 — a partly quoted operand too
//	`[[ abc =~ "(b)" ]]`               0 at 31, and it captures `b`
//
// The axis is asked only where the level does not already decide it, which is
// the rule for every axis here: a question with no bearing on the answer is a
// refusal waiting to happen in a dialect that left it unanswered.
//
// The level reaches only the column that has the parameter —
// Semantics.CompatibilityLevelVariable is empty everywhere else — so this is
// one shell's state and not a reading the others acquire.
func (r *Runner) regexQuotingIsLiteral() bool {
	if r.compatAtMost(31) {
		return false
	}
	return r.ask(r.sem().RegexQuotingMakesLiteral, "quoting a =~ regex making it literal")
}

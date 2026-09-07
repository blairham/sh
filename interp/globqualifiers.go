// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io/fs"
	"strings"
)

// Glob qualifiers: the parenthesized list a pattern may carry at its end,
// which narrows what it matched. One dialect has them; see
// [syntax.Dialect.GlobQualifiers] for the grammar half.
//
// Measured on zsh 5.9.2, 2026-09-06, against a directory holding `d1/`,
// regular files `f1` and `f2`, a symbolic link `l1`, and `.dot`:
//
//	echo *        d1 f1 f2 l1     everything but the hidden name
//	echo *(.)     f1 f2           regular files
//	echo *(/)     d1              directories
//	echo *(@)     l1              symbolic links, by their own type
//	echo *(^.)    d1 l1           `^` turns the sense of what follows
//	echo *(./)    no matches      two qualifiers are an *and*
//	echo *(.,/)   d1 f1 f2        a comma is an *or*
//	echo *(D)     .dot d1 f1 f2   hidden names included
//	echo zz*(N)   (nothing, 0)    a miss is not an error, and the word goes
//
// The permission and remaining type tests were measured the same way, on
// 2026-09-07, against a second directory holding a regular `plain` (0644), a
// `suid` file at 4755, a sticky directory `stick` at 1777, and a FIFO `fifo`
// — plus, for the nine permission letters, the first directory with `x` made
// executable:
//
//	echo *(x)     d1 l1 x         executable by its owner
//	echo *(r)     everything      readable by its owner
//	echo *(w)     everything      writable by its owner
//	echo *(A)     everything      readable by its group
//	echo *(I)     no matches      writable by its group
//	echo *(E)     d1 l1 x         executable by its group
//	echo *(R)     everything      readable by the world
//	echo *(W)     no matches      writable by the world
//	echo *(X)     d1 l1 x         executable by the world
//	echo *(s)     suid            set-user-ID
//	echo *(t)     stick           the sticky bit
//	echo *(p)     fifo            a FIFO, by its own type
//	echo *(%)     null zero disk0 a device, measured in /dev
//	echo *(.x)    suid            side by side is still an and
//	echo *(^x)    fifo plain      and `^` still turns the sense
//
// Three letters are read here and unproven in the positive direction, which
// is a fact about what this machine would let a test create rather than about
// the shell: `S` (set-group-ID) and a block or character `%` need privileges,
// and the run recorded them as *accepted* — `no matches found: *(S)`, not
// `unknown file attribute: S` — which is what says the letter is claimed.
//
// **The three permission triples are the measurement's own finding.** Owner
// is `r w x`, group is `A I E` and world is `R W X`, which no naming
// convention would have suggested: the directory's modes were 0644 and 0755,
// so `A` listing everything and `I` listing nothing places `A` and `I` on the
// group triple, and `E` and `X` agreeing on the 0755 names separates group
// from world only because `W` — world write — is empty where `w` is not.
//
// **The list is qualifiers only when it has no `|` in it.** A group holding
// one is the alternation the dialect already reads — `echo f*(1|2)` is
// `f1 f2`, where `1` on its own is `unknown file attribute: 1`. That single
// character is the whole of the disambiguation, and it was measured rather
// than assumed: `f*(z|y)` is an alternation matching nothing, and `f*(.|/)`
// is a *bad pattern* rather than the two qualifiers it looks like.
//
// A group anywhere but the end of the word is an alternation too:
// `echo *(.)x` is `no matches found: *(.)x`, so being last is a condition
// and not a convenience.

// splitGlobQualifiers separates a trailing qualifier list from the pattern in
// front of it. ok is false when the field carries none.
//
// The parentheses are found in the field's escaped form, so a `(` that was
// quoted or came out of an expansion is skipped — which is measured and is
// the whole reason quoting still works here: `echo "( x )"` prints those
// five characters, and `p="*(.)"; echo $p` prints `*(.)` rather than
// globbing, because parentheses arriving from a value are never a group.
func splitGlobQualifiers(field string) (pattern, list string, ok bool) {
	if !strings.HasSuffix(field, ")") || escapedAt(field, len(field)-1) {
		return field, "", false
	}
	depth := 0
	for i := len(field) - 1; i >= 0; i-- {
		if escapedAt(field, i) {
			continue
		}
		switch field[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth != 0 {
				continue
			}
			inner := field[i+1 : len(field)-1]
			if inner == "" || strings.ContainsRune(inner, '|') {
				// `()` is not a group at all, and a group holding an
				// alternation is that alternation.
				return field, "", false
			}
			return field[:i], inner, true
		}
	}
	return field, "", false
}

// escapedAt reports whether the byte at i is preceded by an odd number of
// backslashes, which is what says a metacharacter there is literal.
func escapedAt(s string, i int) bool {
	n := 0
	for j := i - 1; j >= 0 && s[j] == '\\'; j-- {
		n++
	}
	return n%2 == 1
}

// globQualifiers is a parsed list: sections joined by `,`, each a run of type
// tests that must all hold, plus the two options that are about the *search*
// rather than about a file.
type globQualifiers struct {
	sections  [][]globTypeTest
	seeHidden bool
	// allowNoMatch is `N`: a pattern that matched nothing is no error and
	// the word is deleted, which is `null_glob` for one pattern.
	allowNoMatch bool
}

// globTypeTest is one file-type qualifier and whether a `^` before it turned
// its sense.
type globTypeTest struct {
	kind    byte
	negated bool
}

// parseGlobQualifiers reads a list. The second result is the character no
// qualifier claims, for the diagnostic that names it.
func parseGlobQualifiers(list string) (globQualifiers, byte, bool) {
	var q globQualifiers
	section := []globTypeTest{}
	negate := false
	for i := range len(list) {
		switch c := list[i]; c {
		case ',':
			// A section may be empty — every file passes it — which is what
			// keeps `(.,)` from being a special case.
			q.sections = append(q.sections, section)
			section, negate = []globTypeTest{}, false
		case '^':
			// Turns the sense of everything after it in this section, until
			// another one turns it back.
			negate = !negate
		case 'N':
			q.allowNoMatch = true
		case 'D':
			q.seeHidden = true
		case '.', '/', '@', 'p', '%',
			'r', 'w', 'x', 'A', 'I', 'E', 'R', 'W', 'X',
			's', 'S', 't':
			section = append(section, globTypeTest{kind: c, negated: negate})
		default:
			return q, c, false
		}
	}
	q.sections = append(q.sections, section)
	return q, 0, true
}

// keep reports whether one path survives the list: every test in a section
// has to hold, and any section will do.
func (q globQualifiers) keep(mode fs.FileMode) bool {
	for _, section := range q.sections {
		if sectionKeeps(section, mode) {
			return true
		}
	}
	return false
}

func sectionKeeps(section []globTypeTest, mode fs.FileMode) bool {
	for _, t := range section {
		if globTypeMatches(t.kind, mode) == t.negated {
			return false
		}
	}
	return true
}

// globTypeMatches is what each type qualifier asks of a file's mode, read
// from an lstat: `@` is a symbolic link by its *own* type, so a link to a
// regular file is `@` and not `.`.
//
// A permission letter reads the same mode, and reading it from the lstat is
// what makes a symbolic link answer for *itself* rather than for what it
// points at — measured, `*(x)` lists `l1`, whose target `f1` is 0600.
//
// The question is about the bit and not about this process: `x` is "the owner
// may execute", which is a property of the file, and it stays true of a file
// this shell could not execute. zsh has separate letters for the effective
// user's own access, and they are refused by name; see patterns.md.
func globTypeMatches(kind byte, mode fs.FileMode) bool {
	switch kind {
	case '.':
		return mode.IsRegular()
	case '/':
		return mode.IsDir()
	case '@':
		return mode&fs.ModeSymlink != 0
	case 'p':
		return mode&fs.ModeNamedPipe != 0
	case '%':
		// Block and character devices alike. The `%b` and `%c` spellings
		// that separate them take an argument this list does not read yet,
		// and are refused rather than folded in here.
		return mode&fs.ModeDevice != 0
	case 's':
		return mode&fs.ModeSetuid != 0
	case 'S':
		return mode&fs.ModeSetgid != 0
	case 't':
		return mode&fs.ModeSticky != 0
	}
	if bit, ok := globPermissionBits[kind]; ok {
		return mode.Perm()&bit != 0
	}
	return false
}

// globPermissionBits is the nine permission letters, in the three triples the
// measurement put them in rather than in the three a reader would guess.
//
// Only Perm() is consulted, so the set-ID and sticky bits — which live
// outside it in fs.FileMode — cannot leak into a permission answer.
var globPermissionBits = map[byte]fs.FileMode{
	'r': 0o400, 'w': 0o200, 'x': 0o100,
	'A': 0o040, 'I': 0o020, 'E': 0o010,
	'R': 0o004, 'W': 0o002, 'X': 0o001,
}

// fieldQualifiers reads the qualifier list a field carries, if the dialect
// has them at all. ok is false when the expansion has already been failed.
func (r *Runner) fieldQualifiers(field string) (pattern string, q globQualifiers, has, ok bool) {
	if !r.dialect().GlobQualifiers {
		return field, globQualifiers{}, false, true
	}
	pattern, list, found := splitGlobQualifiers(field)
	if !found {
		return field, globQualifiers{}, false, true
	}
	if strings.HasPrefix(list, "#") && !strings.HasPrefix(list, "#q") &&
		r.MatchOption(ExtendedPatternOperators) {
		// A trailing `(#…)` that is not the `(#q…)` spelling is a pattern
		// flag group standing at the end of the last component, and not a
		// qualifier list at all — so the field goes to the matcher whole.
		//
		// Measured on zsh 5.9.2 in a directory holding `ax` and `cx`:
		// `*x(#i)` lists both with `extendedglob` on and is `unknown file
		// attribute: #` with it off, and `*(#c1,9)` is `bad pattern` rather
		// than a qualifier complaint — which is the matcher's answer and
		// arrives by handing the field over rather than by a check here.
		//
		// It is what makes `(#e)` reachable in pathname expansion at all:
		// the end of a component is exactly where the end anchor is
		// written, so reading it as a qualifier list left `*x(#e)` naming a
		// file attribute that was never in the pattern.
		return field, globQualifiers{}, false, true
	}
	if after, cut := strings.CutPrefix(list, "#q"); cut {
		// `(#q…)` is the same list wearing the extended flag group's
		// spelling, and it is the only spelling that works where the bare
		// one has been turned off. Measured: `*(#q.)` lists regular files
		// with `extendedglob` on and is `unknown file attribute: #` with it
		// off, which is the answer this shell already gave — so the prefix
		// is read only while the option is on.
		if !r.MatchOption(ExtendedPatternOperators) {
			r.fatal("unknown file attribute: %c\n", '#')
			return "", globQualifiers{}, true, false
		}
		list = after
	}
	q, bad, valid := parseGlobQualifiers(list)
	if !valid {
		// Named, because the message is the whole of what a reader has to go
		// on: the list may be long and only one character in it was wrong.
		// The shell's own wording, and a space is a character like any other
		// — `echo MY ( x )` names the space, which is what says the group
		// was read as qualifiers rather than as anything of the shell's.
		r.fatal("unknown file attribute: %c\n", bad)
		return "", globQualifiers{}, true, false
	}
	return pattern, q, true, true
}

// keepQualified narrows a match list to the files the qualifiers admit.
//
// The paths are the absolute ones the walk produced rather than the relative
// ones reported, because a type test is a question about a file and the
// working directory is not the shell's process directory. A path that cannot
// be read is dropped rather than kept: it is not a regular file, not a
// directory and not a link, so no qualifier can hold of it.
func (r *Runner) keepQualified(paths []string, q globQualifiers) []string {
	kept := make([]string, 0, len(paths))
	for _, p := range paths {
		info, err := r.lstat(strings.TrimSuffix(p, "/"))
		if err != nil {
			continue
		}
		if q.keep(info.Mode()) {
			kept = append(kept, p)
		}
	}
	return kept
}

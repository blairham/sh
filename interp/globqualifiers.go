// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"io/fs"
	"strings"
	"time"
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
	sections  [][]globTest
	seeHidden bool
	// allowNoMatch is `N`: a pattern that matched nothing is no error and
	// the word is deleted, which is `null_glob` for one pattern.
	allowNoMatch bool
	// modifiers is the history-style modifier text a `:` in the list opens,
	// applied to every name the pattern reported. Empty where the list has
	// no `:` in it, which is every list that is only qualifiers.
	modifiers string
	// ranges is the `[n,m]` subscripts the list carried, in the order they
	// were written. They are not tests and cannot be: every qualifier beside
	// them asks something of one file, and this one asks *where in the match
	// list* a file is — so it is settled after the walk rather than during
	// it, and two of them compose (`*([1,3][2])` is the second of the first
	// three). See globRange, and keepQualified for where they land.
	ranges []globRange
	// sorts is the `o` and `O` specifiers, in the order they were written:
	// the first decides, and each later one breaks the ties the ones before
	// it left. See globSortSpec.
	sorts []globSortSpec
	// unsorted is `oN`: the directory's own order, with no sort at all.
	unsorted bool
	// numeric is `n`, and `^n`: whether a run of digits in a name sorts as a
	// number for this pattern, whatever `numericglobsort` says. Zero leaves
	// the option to decide; see globNumeric.
	numeric globNumeric
	// picks is ranges with the arithmetic done, filled by fieldQualifiers.
	// The parser has no Runner and a subscript is an *expression* — `*([i])`
	// and `*([1+1])` both name the second match — so the two halves are
	// necessarily in different places.
	picks []globPick
}

// globNumeric is the `n` qualifier's three states: not written, written, and
// written behind a `^`.
type globNumeric uint8

const (
	globNumericUnsaid globNumeric = iota
	globNumericOn
	globNumericOff
)

// globSortSpec is one `o` or `O` specifier: the key letter, which way it
// runs, and whether a `-` in front of it said to ask a symbolic link's target.
//
// Measured on zsh 5.9.2, 2026-10-05, under `LC_ALL=C`, in a directory where
// `a2 a9 a10 lnk c.lis big d a b C` is newest first, `big` holds six bytes,
// `lnk` points at it and `d` is a directory:
//
//	*(on)      C a a10 a2 a9 b big c.lis d lnk   by name, the default order
//	*(On)      lnk d c.lis big b a9 a2 a10 a C   `O` is the reverse
//	*(^on)     the same as `On`                  and so is a `^` in front
//	*(om)      a2 a9 a10 lnk c.lis big d a b C   the youngest first
//	*(om[1])   a2                                and the pick runs after it
//	*(oL-on)   … lnk big d                       a later spec breaks ties,
//	                                             and the link is its own size
//	*(-oL)     … big lnk d                       until `-` follows it
//	*(nOn)     the numbers reversed as numbers   `n` reaches `on`
//	*(ox)      unknown sort specifier            at status 1
//
// And with `**/*.lis` over `a/b/x.lis a/y.lis c.lis d/a.lis d/e/b.lis`:
//
//	(odon)     a/b/x.lis a/y.lis d/e/b.lis d/a.lis c.lis
//	(Odon)     c.lis a/y.lis a/b/x.lis d/a.lis d/e/b.lis
//
// which is the manual's "files in subdirectories appear before those in the
// current directory at each level of the search", and not a count of
// slashes: `a/y.lis` is ahead of `d/e/b.lis` because the two part at `a`
// and `d`, both directories, and `d` makes no claim over `a` there.
//
// Ties the specifiers leave are broken by name. The reference leaves them in
// whatever order its sort happened to produce — `*(oL)` over a set of empty
// files is in neither name order nor the directory's — so no tie order is
// that shell's to reproduce, and the name is the one that cannot move between
// runs.
type globSortSpec struct {
	key    byte
	desc   bool
	follow bool
}

// globRange is one `[…]` subscript as written: the expressions either side of
// its comma, unevaluated.
//
// Measured on zsh 5.9.2, 2026-09-28, in a directory whose matches sort to
// `a b c d dir e`:
//
//	*([2,4])       b c d        the n-th through the m-th
//	*([1])         a            one expression is one element
//	*([-1])        e            a negative counts from the end
//	*([2,-1])      b c d dir e
//	*([-2,-1])     dir e
//	*([0])         (nothing)    and an empty selection is *not* an error:
//	*([9])         (nothing)      status 0, the word simply goes
//	*([3,99])      c d dir e    the far end is clamped
//	*([0,3])       a b c        and so is the near one
//	*([4,2])       (nothing)    a reversed range selects nothing
//	*(^.[1,2])     dir          the tests run first, then the pick
//	*([1,3][2])    b            two subscripts compose, left to right
//	*([i]) i=2     b            the subscript is arithmetic
//	*([1+1])       b
//	*([ 2 , 3 ])   b c          and spaces in it are the evaluator's
//
// The empty-selection row is the one that shapes the code: a *pattern* that
// matches nothing is `no matches found`, and a pattern whose matches a
// subscript then empties is silence at 0. So this cannot be written as a test
// that every file fails.
type globRange struct {
	// from and to are the two expressions, as written. to is empty and
	// single is true where the subscript held no comma.
	from, to string
	single   bool
}

// globPick is a globRange with its arithmetic done: the two positions, still
// as written, so that a negative is resolved against the list that is there
// when the pick is applied rather than against one measured earlier.
type globPick struct {
	from, to int
	single   bool
}

// globTest is one qualifier that asks something of a file: which question,
// whether a `^` before it turned the sense, and whether a `-` before it said
// to ask the question of a symbolic link's *target* rather than of the link.
//
// The argument-taking qualifiers keep theirs here, already read: `f` carries
// the mode conditions its spec came to, and `u` and `g` carry the numeric id
// a name was resolved to. Both are settled while the list is parsed, which is
// what makes `zz*(Nu:nosuchuser:)` name the user in a directory where the
// pattern matches nothing at all.
type globTest struct {
	kind    byte
	negated bool
	follow  bool
	// mode is `f`'s conditions, every one of which has to hold.
	mode modeSpec
	// id is `u`'s user or `g`'s group, and it is a uint64 because a written
	// number is not obliged to fit a uid — one that does not fit matches
	// nothing, which is what a uid no file carries would do anyway.
	id uint64
	// unit is how long one of `m`'s units is — the letter after the
	// qualifier, defaulting to a day. Zero for every test that is not about
	// a time, which is what keeps fileAge from being asked about one.
	unit time.Duration
	// count is `l`'s comparison, and `m`'s: the number written and which way
	// the file's own number has to stand against it.
	count globCount
}

// parseGlobQualifiers reads a list. The second result is the diagnostic when
// the list is bad — the shell's own wording, chosen at the point that knows
// which of the four complaints applies.
func parseGlobQualifiers(list string) (globQualifiers, string, bool) {
	var q globQualifiers
	section := []globTest{}
	negate, follow := false, false
	for i := 0; i < len(list); {
		c := list[i]
		i++
		switch c {
		case ',':
			// A section may be empty — every file passes it — which is what
			// keeps `(.,)` from being a special case. It is also where both
			// prefixes end: `^` and `-` are read again from nothing in the
			// next section, measured — `*(-@,@)` lists the link that could
			// not be followed *and* the two that could, so the second
			// section did not inherit the first's `-`.
			q.sections = append(q.sections, section)
			section, negate, follow = []globTest{}, false, false
		case '^':
			// Turns the sense of everything after it in this section, until
			// another one turns it back.
			negate = !negate
		case '-':
			// Not an attribute: a toggle saying that the qualifiers after it
			// ask about what a symbolic link points at. A second one turns it
			// back — measured, `*(--.)` is the plain `.` again — which is why
			// this is a toggle and not a flag being set.
			follow = !follow
		case ':':
			// Not a qualifier and not the start of one: a `:` opens the
			// modifier list, and *everything* after it is modifier text —
			// measured, `*(N:t.)` lists the tails of every name and the `.`
			// asks nothing, where `*(N.:t)` lists the tails of the regular
			// files. So the qualifiers end here rather than resuming behind
			// the modifiers.
			q.modifiers = list[i:]
			q.sections = append(q.sections, section)
			return q, "", true
		case '[':
			// Not a file attribute at all: a position in the match list. The
			// text is kept rather than evaluated, because this function has
			// no Runner and `*([i])` names an element by a *parameter*.
			end := strings.IndexByte(list[i:], ']')
			if end < 0 {
				// zsh's own sentence for a subscript that never closes,
				// measured: `*([1,2)` is `invalid subscript` at 1.
				return q, "invalid subscript", false
			}
			text := list[i : i+end]
			i += end + 1
			from, to, comma := strings.Cut(text, ",")
			q.ranges = append(q.ranges, globRange{from: from, to: to, single: !comma})
		case 'o', 'O':
			if i >= len(list) {
				return q, "unknown sort specifier", false
			}
			key := list[i]
			i++
			switch key {
			case 'N':
				q.unsorted = true
			case 'n', 'L', 'l', 'a', 'm', 'c', 'd':
				q.sorts = append(q.sorts, globSortSpec{key: key, desc: (c == 'O') != negate, follow: follow})
			case 'e', '+':
				// The specifiers that sort by a string of code run per match,
				// which this engine does not carry — the same refusal the `e`
				// qualifier makes, said as what it is rather than as the
				// reference's sentence for a letter it does not know.
				return q, fmt.Sprintf("the o%c sort specifier is not implemented", key), false
			default:
				return q, "unknown sort specifier", false
			}
		case 'n':
			// `numericglobsort` for one pattern, and `^n` is it turned off
			// for one: measured, `setopt numericglobsort; print *(^n)` is the
			// plain name order.
			q.numeric = globNumericOn
			if negate {
				q.numeric = globNumericOff
			}
		case 'N':
			q.allowNoMatch = true
		case 'D':
			q.seeHidden = true
		case 'f':
			spec, n, diag := parseModeSpec(list[i:])
			if diag != "" {
				return q, diag, false
			}
			i += n
			section = append(section, globTest{kind: 'f', negated: negate, follow: follow, mode: spec})
		case 'l':
			// The link count, and the first qualifier here whose argument is
			// a *number* rather than a mode or a name. `-` and `+` in front
			// of it are the comparison and not the follow toggle — that one
			// is written before the letter, so `*(-l1)` follows the link and
			// `*(l-1)` asks for fewer than one.
			count, n, diag := parseGlobCount(list[i:])
			if diag != "" {
				return q, diag, false
			}
			i += n
			section = append(section, globTest{kind: 'l', negated: negate, follow: follow, count: count})
		case 'a', 'c', 'm':
			// The three file times, as an age in whole units — `m` for the
			// modification, `a` for the access and `c` for the inode change.
			// One arm because they are one qualifier with three clocks
			// behind it: the unit letter, the sign and the number are read
			// the same way for each, and only the stat field differs. See
			// parseGlobAge for the unit letter and fileAge for why the
			// comparison is over a truncated number.
			//
			// Measured 2026-09-18 on zsh 5.9.2 in a directory holding no
			// match: `zz*(a+1)` and `zz*(c1)` are `no matches found` where
			// this engine said `unknown file attribute`, and `zz*(a)` with
			// no number behind it is `number expected` — the same refusal
			// `m` already gave, which is what says the three share a reader
			// rather than only a letter (#3533).
			count, unit, n, diag := parseGlobAge(list[i:])
			if diag != "" {
				return q, diag, false
			}
			i += n
			section = append(section, globTest{
				kind: c, negated: negate, follow: follow, count: count, unit: unit,
			})
		case 'u', 'g':
			id, n, diag := parseOwnerArgument(c, list[i:])
			if diag != "" {
				return q, diag, false
			}
			i += n
			section = append(section, globTest{kind: c, negated: negate, follow: follow, id: id})
		case 'U', 'G':
			// The same two tests with the argument this process already
			// answers, so they carry no spelling of their own past here:
			// `U` is `u` with the effective user filled in.
			kind, id := byte('u'), uint64(uint32(osGeteuid()))
			if c == 'G' {
				kind, id = 'g', uint64(uint32(osGetegid()))
			}
			section = append(section, globTest{kind: kind, negated: negate, follow: follow, id: id})
		case 'e':
			// The qualifier that runs a string of code per match, which this
			// engine does not carry. Its argument is still *read*, and a list
			// that leaves it unclosed is refused for that before the letter is
			// weighed at all: measured 2026-10-04 on zsh 5.9.2, `echo x(e)`,
			// `x(echo)` and `x=(echo hi)` are all `missing end of string` —
			// the last being a mid-word group read as qualifiers, its first
			// letter `e` and its delimiter `c`.
			if _, _, closed := delimitedArgumentOrNone(list[i:]); !closed {
				return q, "missing end of string", false
			}
			return q, "unknown file attribute: e", false
		case '.', '/', '@', 'p', '%',
			'r', 'w', 'x', 'A', 'I', 'E', 'R', 'W', 'X',
			's', 'S', 't':
			section = append(section, globTest{kind: c, negated: negate, follow: follow})
		default:
			// Named, because the message is the whole of what a reader has to
			// go on: the list may be long and only one character in it was
			// wrong. A space is a character like any other — `echo MY ( x )`
			// names the space, which is what says the group was read as
			// qualifiers rather than as anything of the shell's.
			return q, fmt.Sprintf("unknown file attribute: %c", c), false
		}
	}
	q.sections = append(q.sections, section)
	return q, "", true
}

// keep reports whether one path survives the list: every test in a section
// has to hold, and any section will do.
//
// **A section with no test in it counts only when it is the last one.**
// Measured 2026-10-05 on zsh 5.9.2 under -f, in a directory holding regular
// files, a directory `d` and a link `lnk`: `*(,/)` is `d`, `*(N,/)` and
// `*(on,/)` are `d` as well (a letter that is no test leaves its section
// empty), and `*(/,,.)` is everything but `lnk` — while `*(/,)`, `*(.,)` and
// `*(,)` are every name, and so is `*(/,:t)`, the section a `:` ends being the
// last (#5995).
func (q globQualifiers) keep(f *globFile) bool {
	for i, section := range q.sections {
		if len(section) == 0 && i < len(q.sections)-1 {
			continue
		}
		if sectionKeeps(section, f) {
			return true
		}
	}
	return false
}

func sectionKeeps(section []globTest, f *globFile) bool {
	for _, t := range section {
		info := f.info(t.follow)
		if info == nil {
			return false
		}
		if globTestMatches(t, info, f.now()) == t.negated {
			return false
		}
	}
	return true
}

// globTestMatches is what each qualifier asks of a file, from the stat the
// `-` prefix chose: `@` is a symbolic link by its *own* type, so a link to a
// regular file is `@` and not `.` — until a `-` in front of it says to ask
// the target instead.
//
// A permission letter reads the same mode, and reading it from the lstat is
// what makes a symbolic link answer for *itself* rather than for what it
// points at — measured, `*(x)` lists `l1`, whose target `f1` is 0600.
//
// The question is about the bit and not about this process: `x` is "the owner
// may execute", which is a property of the file, and it stays true of a file
// this shell could not execute. zsh has separate letters for the effective
// user's own access, and they are refused by name; see patterns.md.
func globTestMatches(t globTest, info fs.FileInfo, now time.Time) bool {
	mode := info.Mode()
	switch t.kind {
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
	case 'f':
		return t.mode.holds(rawMode(mode))
	case 'u', 'g':
		id, ok := fileOwner(info, t.kind)
		return ok && id == t.id
	case 'l':
		links, ok := fileLinks(info)
		return ok && t.count.holds(links)
	case 'm':
		return t.count.holds(fileAge(info.ModTime(), now, t.unit))
	case 'a', 'c':
		when, ok := fileTime(info, t.kind)
		return ok && t.count.holds(fileAge(when, now, t.unit))
	}
	if bit, ok := globPermissionBits[t.kind]; ok {
		return mode.Perm()&bit != 0
	}
	return false
}

// rawMode is the twelve bits a mode is written with, which is not how Go
// spells a mode: fs.FileMode keeps set-user-ID, set-group-ID and the sticky
// bit outside Perm() in bits of its own, and `f` compares against a number
// somebody wrote as `4755`.
func rawMode(mode fs.FileMode) uint32 {
	raw := uint32(mode.Perm())
	if mode&fs.ModeSetuid != 0 {
		raw |= 0o4000
	}
	if mode&fs.ModeSetgid != 0 {
		raw |= 0o2000
	}
	if mode&fs.ModeSticky != 0 {
		raw |= 0o1000
	}
	return raw
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
	if !r.lang().GlobQualifiers {
		return field, globQualifiers{}, false, true
	}
	pattern, list, found := splitGlobQualifiers(field)
	if !found {
		return field, globQualifiers{}, false, true
	}
	// A refusal here sets its status wherever it is written, as an unmatched
	// pattern does: measured 2026-10-05 on zsh 5.9.2, `(exit 4); print
	// *(.zzq)` and `(exit 4); print *([1,])` exit 1, where `$((1/0))` in the
	// same place leaves the 4. See Runner.refuseUnmatchedPattern (#6067).
	defer r.globRefusalSetsItsStatus()()
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
	if !strings.HasPrefix(list, "#q") && r.MatchOption(TrailingGroupIsPartOfThePattern) {
		// The bare reading turned off: the parentheses are pattern text
		// again, so the field goes to the matcher whole and a `(N)` matches
		// the letter rather than emptying a miss. Measured — `echo *.md(N)`
		// under `NO_BARE_GLOB_QUAL` is `no matches found: *.md(N)`, which is
		// the ordinary answer for a pattern that matched nothing in a shell
		// that treats a miss as an error.
		//
		// Before the `#q` spelling rather than after it, and reading the
		// prefix rather than the flag, because that spelling is the one the
		// option does not reach: `*(#q.)` still qualifies with the bare
		// reading off, measured in the same run.
		//
		// It matters beyond a script that opts in. The preamble an agent
		// harness puts in front of **every** command it runs is
		// `{ shopt -u extglob || setopt NO_EXTENDED_GLOB NO_BARE_GLOB_QUAL; }`,
		// so this is the state every command under one runs in, and reading
		// a qualifier there is being more permissive than the tool asked for
		// — which it asked precisely so a generated pattern could not be
		// reinterpreted (#1729).
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
	q, diag, valid := parseGlobQualifiers(list)
	if !valid {
		// The wording is the parser's, because only it knows which of the
		// four complaints applies — a character no qualifier claims, a mode
		// spec that would not read, a name argument with no delimiter around
		// it, or a user or group of that name that does not exist. Each is
		// the shell's own sentence, and the diagnostic is the whole of what a
		// reader has to go on: a list may be long and only one character in
		// it was wrong.
		r.fatal("%s\n", diag)
		return "", globQualifiers{}, true, false
	}
	// The subscripts, evaluated here because they are *expressions* and the
	// parser above has no Runner to resolve a name against. A refusal is the
	// evaluator's own — measured, `*([1,])` is `bad math expression: empty
	// string` in the reference, which is the sentence its arithmetic gives
	// any empty expression rather than anything this qualifier says.
	for _, sel := range q.ranges {
		pick, ok := r.globRangePick(sel)
		if !ok {
			return "", globQualifiers{}, true, false
		}
		q.picks = append(q.picks, pick)
	}
	return pattern, q, true, true
}

// globRangePick evaluates one `[…]` subscript. ok is false where the
// expression was refused, the diagnostic already written.
func (r *Runner) globRangePick(sel globRange) (globPick, bool) {
	from, ok := r.globRangeIndex(sel.from)
	if !ok {
		return globPick{}, false
	}
	if sel.single {
		return globPick{from: from, to: from, single: true}, true
	}
	to, ok := r.globRangeIndex(sel.to)
	if !ok {
		return globPick{}, false
	}
	return globPick{from: from, to: to}, true
}

// globRangeIndex is one side of a subscript, read as arithmetic.
//
// Through arithTreeRead rather than arithTree, for the reason a subscript
// already has: the word reached this point expanded, and reading it as an
// expression a second time would run what the first pass produced.
func (r *Runner) globRangeIndex(text string) (int, bool) {
	// An empty or blank side is the empty-subscript question and not a
	// zero: measured, `*([1,])` is `bad math expression: empty string` in
	// the reference where `$(( ))` there is 0. Through the reader the
	// expansion's own subscripts use, so the two spellings cannot drift
	// apart — see Runner.emptySubscriptText.
	if err := r.emptySubscriptText(text); err != nil {
		r.fatal("%s\n", err)
		return 0, false
	}
	tree, err := r.arithTreeRead(text)
	if err != nil {
		r.fatal("%s\n", err)
		return 0, false
	}
	n, err := r.evalArith(tree)
	if err != nil {
		r.fatal("%s\n", err)
		return 0, false
	}
	return n, true
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
		f := globFile{r: r, path: strings.TrimSuffix(p, "/")}
		if f.info(false) == nil {
			continue
		}
		if q.keep(&f) {
			kept = append(kept, p)
		}
	}
	return kept
}

// pickRanges applies the `[n,m]` subscripts, in the order they were written.
//
// Kept apart from keepQualified and called after it, because the two empty a
// list for different reasons and the caller has to tell them apart: a list the
// *tests* emptied is `no matches found`, and a list a subscript emptied is
// silence at status 0. See globRange for both rows.
func pickRanges(names []string, picks []globPick) []string {
	for _, pick := range picks {
		names = globPicked(names, pick)
	}
	return names
}

// globPicked is one `[n,m]` over a match list.
//
// One-based, with a negative counting from the end and both ends clamped to
// the list — see globRange for the rows. An empty result is a list of no
// names and never an error: a pattern that matched nothing is the shell's
// `no matches found`, and a subscript that empties what it matched is not.
func globPicked(names []string, pick globPick) []string {
	n := len(names)
	resolve := func(v int) int {
		if v < 0 {
			return n + 1 + v
		}
		return v
	}
	from := resolve(pick.from)
	to := from
	if !pick.single {
		to = resolve(pick.to)
	}
	if from < 1 {
		from = 1
	}
	if to > n {
		to = n
	}
	if from > to || from > n || to < 1 {
		return nil
	}
	return names[from-1 : to]
}

// now is the clock this file's tests are asked against, through the Runner so
// that a test can pin it — see Runner.Clock, which exists for exactly this.
//
// Taken per file rather than once for the walk, which costs a clock read per
// candidate and is the answer that cannot drift: a walk over a large tree
// takes real time, and a file written *during* it is younger than the walk's
// start, not older.
func (f *globFile) now() time.Time {
	if f == nil || f.r == nil {
		return time.Time{}
	}
	return f.r.Now()
}

// globFile is one candidate path, and the two answers a qualifier list may
// want about it: the link itself, and what it points at.
//
// Both are asked at most once and only when something asks. A list with no
// `-` in it costs the one lstat the walk always paid, which is what keeps the
// toggle from being a tax on every other pattern.
type globFile struct {
	r    *Runner
	path string

	link, target fs.FileInfo
	askedLink    bool
	askedTarget  bool
}

// info is the stat a test reads, following the link or not.
//
// A link whose target cannot be stat'd answers with the link — "treated as a
// file in its own right", and measured: `*(-@)` lists the dangling link and
// neither of the two that resolve, because following those two reaches a
// regular file and a directory while following this one reaches nothing.
func (f *globFile) info(follow bool) fs.FileInfo {
	if !f.askedLink {
		f.askedLink = true
		if info, err := f.r.lstat(f.path); err == nil {
			f.link = info
		}
	}
	if !follow || f.link == nil {
		return f.link
	}
	if !f.askedTarget {
		f.askedTarget = true
		if info, err := f.r.stat(f.path); err == nil {
			f.target = info
		}
	}
	if f.target == nil {
		return f.link
	}
	return f.target
}

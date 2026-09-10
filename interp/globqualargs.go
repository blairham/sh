// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

// The three glob qualifiers that take an argument: `f` for access rights and
// `u` and `g` for ownership. They are the reason the list is scanned by index
// rather than character by character — everything else in it is one letter.
//
// Measured on zsh 5.9.2, 2026-09-10, against a directory holding one regular
// file per mode: 0000, 0600, 0644, 0664, 0666, 0700, 0755, 1777 and 4755.
//
//	*(f0666)      the file at exactly that mode
//	*(f755)       0755 *and* 4755 — three digits compare three digits
//	*(f0755)      0755 alone — a fourth digit compares the fourth
//	*(f=755)      the same as writing no operator at all
//	*(f+022)      every bit of 022 set: 0666 and 1777
//	*(f-022)      no bit of 022 set: 0000 0600 0644 0700 0755 4755
//	*(f70?)       owner 7, group 0, and `?` asks nothing of the third
//	*(f:g+w:)     0664 0666 1777 — group-writable, the chmod-style spelling
//	*(f:u=rw:)    the owner's bits are exactly rw
//	*(f:g+w,o+w:) both clauses hold: 0666 1777, not 0664
//	*(u0)         owned by uid 0
//	*(u:root:)    the same by name — the delimited form is *always* a name,
//	              and `u[501]` is `unknown username '501'`
//	*(U)          owned by this process's effective user; `G` is the group
//
// **`+` is every bit and `-` is no bit**, which a run had to establish
// because the alternative reading of `-` — "not all of them" — agrees with it
// on every number whose bits a file either has or lacks entirely. `*(f-0700)`
// lists nothing where 0666 is present, and 0666 has two of those three bits.
//
// **The mask is as wide as the number is long.** `f755` listing a 4755 file
// is what says the fourth digit is not compared unless it is written, and the
// `?` the manual documents for the same purpose is the same rule spelled per
// digit.
//
// **A sub-spec that is a number ends the spec.** `f:u+w,+022:` is read and
// `f:+022,u+w:` is `invalid mode specification`, as are `f:755,644:` and
// `f:u+w,+022,g+w:` — so a number may be the last sub-spec or the only one,
// and never a middle one.
//
// **`s` and `t` are the class's own bit and not a fourth permission.** `u+s`
// is set-user-ID, `g+s` is set-group-ID and `o+t` is the sticky bit, while
// `u+t`, `g+t` and `o+s` ask for a bit that class does not have and hold of
// everything. `a+s` is the two set-ID bits together and holds of neither file
// that has only one.
//
// **`=` compares the class's whole triple, set-ID bit included.** `f:u=rwx:`
// leaves out the 4755 file and `f:u=rwxs:` is the one that finds it, which
// puts 04000 inside `u`'s mask rather than beside it — and the same probe
// against `o` puts the sticky bit inside `o`'s.

// invalidModeSpec is the shell's wording for every way `f`'s argument can
// fail to read: an empty spec, a delimiter with no partner, a class letter
// that is not one of four, an operator that is not one of three, a permission
// character that is neither a letter nor an octal digit, and a number where a
// number may not stand.
const invalidModeSpec = "invalid mode specification"

// modeSpec is what `f`'s argument came to: conditions on the twelve mode
// bits, every one of which has to hold. A spec written as one number is one
// condition; the delimited form is one per comma-separated sub-spec.
type modeSpec struct {
	conds []modeCond
}

// modeCond is one comparison. `=` compares the bits inside mask, `+` asks for
// every bit of value, and `-` asks for none of them.
type modeCond struct {
	op    byte
	value uint32
	mask  uint32
}

func (s modeSpec) holds(mode uint32) bool {
	for _, c := range s.conds {
		switch c.op {
		case '=':
			if mode&c.mask != c.value {
				return false
			}
		case '+':
			if mode&c.value != c.value {
				return false
			}
		case '-':
			if mode&c.value != 0 {
				return false
			}
		}
	}
	return true
}

// parseModeSpec reads `f`'s argument off the front of what follows it and
// says how many characters it took.
//
// Which of the two forms this is turns on the first character: an operator, an
// octal digit or a `?` starts the number, and anything else is the delimiter
// around a list of sub-specs. That is what makes `f:g+w:` and `f755` the same
// qualifier, and it is also why `f` with nothing after it is a failure rather
// than a spec that asks nothing — there is no character to delimit with.
func parseModeSpec(s string) (modeSpec, int, string) {
	if s == "" {
		return modeSpec{}, 0, invalidModeSpec
	}
	if startsModeNumber(s[0]) {
		cond, n := parseModeNumber(s)
		return modeSpec{conds: []modeCond{cond}}, n, ""
	}
	body, n, ok := delimitedArgument(s)
	if !ok || body == "" {
		return modeSpec{}, 0, invalidModeSpec
	}
	spec, diag := parseModeSubSpecs(body)
	if diag != "" {
		return modeSpec{}, 0, diag
	}
	return spec, n, ""
}

// startsModeNumber reports whether a character begins the numeric form.
func startsModeNumber(c byte) bool {
	return c == '=' || c == '+' || c == '-' || c == '?' || (c >= '0' && c <= '7')
}

// parseModeNumber reads an optional operator and the octal digits behind it,
// stopping at the first character that is neither, and says how far it got.
//
// It cannot fail. Where the digits are is where the qualifier list resumes —
// `*(f644.)` is the mode and then the regular-file test — and a number with
// no digits at all asks nothing, which is measured: `*(f=)` and `*(f-)` list
// everything.
func parseModeNumber(s string) (modeCond, int) {
	cond := modeCond{op: '='}
	i := 0
	if s[0] == '=' || s[0] == '+' || s[0] == '-' {
		cond.op = s[0]
		i = 1
	}
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '7':
			cond.value = cond.value<<3 | uint32(c-'0')
			cond.mask = cond.mask<<3 | 7
		case c == '?':
			// The digit is written and not compared, which is the only way to
			// leave a *middle* digit out — the width of the number takes care
			// of the leading ones.
			cond.value <<= 3
			cond.mask <<= 3
		default:
			return cond, i
		}
	}
	return cond, i
}

// parseModeSubSpecs reads the comma-separated list inside the delimiters.
// Every sub-spec has to hold, so the list is an and.
func parseModeSubSpecs(body string) (modeSpec, string) {
	var spec modeSpec
	for {
		sub, rest := body, ""
		last := true
		if i := strings.IndexByte(body, ','); i >= 0 {
			sub, rest, last = body[:i], body[i+1:], false
		}
		if sub == "" {
			return modeSpec{}, invalidModeSpec
		}
		if startsModeNumber(sub[0]) {
			// A number ends the spec: it may be the last sub-spec or the
			// only one, and `f:755,644:` and `f:+022,u+w:` are both refused.
			cond, n := parseModeNumber(sub)
			if n != len(sub) || !last {
				return modeSpec{}, invalidModeSpec
			}
			spec.conds = append(spec.conds, cond)
			return spec, ""
		}
		cond, diag := parseModeClause(sub)
		if diag != "" {
			return modeSpec{}, diag
		}
		spec.conds = append(spec.conds, cond)
		if last {
			return spec, ""
		}
		body = rest
	}
}

// modeClass is what one of `u`, `g` and `o` owns: the bits `=` compares for
// it, and which bit each permission character means when it is written for
// that class.
type modeClass struct {
	mask   uint32
	read   uint32
	write  uint32
	exec   uint32
	setID  uint32
	sticky uint32
	shift  uint
}

// modeClasses is the three classes. `a` is not here because it is the three
// of them at once rather than a fourth.
//
// The mask is the class's triple *plus* its own high bit, which is the run's
// finding rather than an assumption: `f:u=rwx:` does not list a 4755 file and
// `f:o=rwx:` does not list a 1777 one, so 04000 is inside `u` and 01000 is
// inside `o`.
var modeClasses = map[byte]modeClass{
	'u': {mask: 0o4700, read: 0o400, write: 0o200, exec: 0o100, setID: 0o4000, shift: 6},
	'g': {mask: 0o2070, read: 0o040, write: 0o020, exec: 0o010, setID: 0o2000, shift: 3},
	'o': {mask: 0o1007, read: 0o004, write: 0o002, exec: 0o001, sticky: 0o1000, shift: 0},
}

// parseModeClause reads one chmod-style sub-spec: which classes, then the
// operator, then what to ask of them.
func parseModeClause(sub string) (modeCond, string) {
	var classes []modeClass
	i := 0
	for ; i < len(sub); i++ {
		if sub[i] == 'a' {
			classes = append(classes, modeClasses['u'], modeClasses['g'], modeClasses['o'])
			continue
		}
		class, ok := modeClasses[sub[i]]
		if !ok {
			break
		}
		classes = append(classes, class)
	}
	// The operator may be left out, and leaving it out is `=` — the same
	// default the number form has. `f:u:` is the files whose owner bits are
	// all clear, exactly as `f:u=:` is, and `f:g:` and `f:a:` answer the
	// same way about their own classes.
	op, perms := byte('='), ""
	if i < len(sub) {
		op = sub[i]
		if op != '=' && op != '+' && op != '-' {
			// Which is also what refuses a clause with no class at all: a
			// first character that is neither a class nor an operator lands
			// here. A clause that *begins* with an operator never arrives —
			// `f:+w:` is read as the number form, and `+w` is a number with
			// a `w` left over, which is the same refusal by another route.
			return modeCond{}, invalidModeSpec
		}
		perms = sub[i+1:]
	}
	cond := modeCond{op: op}
	for _, class := range classes {
		cond.mask |= class.mask
	}
	for _, c := range []byte(perms) {
		for _, class := range classes {
			bit, ok := classPermission(class, c)
			if !ok {
				return modeCond{}, invalidModeSpec
			}
			cond.value |= bit
		}
	}
	return cond, ""
}

// classPermission is the bit one permission character means for one class.
//
// `s` and `t` name a bit two of the three classes do not have, and asking for
// a bit that is not there is asking for nothing: `*(f:u+t:)` and `*(f:o+s:)`
// list every file, where `*(f:o+t:)` lists the sticky one.
func classPermission(class modeClass, c byte) (uint32, bool) {
	switch c {
	case 'r':
		return class.read, true
	case 'w':
		return class.write, true
	case 'x':
		return class.exec, true
	case 's':
		return class.setID, true
	case 't':
		return class.sticky, true
	}
	if c >= '0' && c <= '7' {
		// An octal digit stands for the triple it would occupy in a number,
		// so `u+7` is `u+rwx` — measured, the two list the same names.
		return uint32(c-'0') << class.shift, true
	}
	return 0, false
}

// parseOwnerArgument reads `u`'s or `g`'s argument and says how many
// characters it took.
//
// A number is a uid or gid; anything else is a delimiter around a *name*,
// always — `u[501]` is `unknown username '501'` and not the uid 501, which is
// the row that says the two forms do not overlap.
func parseOwnerArgument(kind byte, s string) (uint64, int, string) {
	// The wording zsh gives where the argument is neither: nothing after the
	// letter at all, or a delimiter with no partner behind it.
	//
	// An operator is a delimiter like any other and is *not* the exception it
	// looks like — `u+bhamilton+` is the same as `u:bhamilton:`, and
	// `u+500` is this complaint only because there is no second `+`. That
	// was worth measuring rather than assuming: `u+500`, `u-502` and `u=501`
	// all give the complaint, which reads exactly like a rule about the three
	// characters and is not one.
	missing := fmt.Sprintf("missing delimiter for '%c' glob qualifier", kind)
	if s == "" {
		return 0, 0, missing
	}
	if s[0] >= '0' && s[0] <= '9' {
		i := 0
		for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		}
		id, err := strconv.ParseUint(s[:i], 10, 64)
		if err != nil {
			// Too long to be a number at all, and no file carries an id that
			// wide: the qualifier is read and matches nothing, which is what
			// an unused uid does anyway.
			id = ^uint64(0)
		}
		return id, i, ""
	}
	name, n, ok := delimitedArgument(s)
	if !ok {
		return 0, 0, missing
	}
	id, diag := lookupOwner(kind, name)
	if diag != "" {
		return 0, 0, diag
	}
	return id, n, ""
}

// lookupOwner resolves a login name or a group name, while the list is being
// read rather than while a file is being tested — measured:
// `zz*(Nu:nosuchuser:)` names the user in a directory the pattern misses
// entirely, so the lookup is not something a match set can avoid.
//
// The two complaints are not the same sentence, and that is the shell's doing
// rather than ours: the user's names the name and the group's does not.
func lookupOwner(kind byte, name string) (uint64, string) {
	if kind == 'g' {
		g, err := user.LookupGroup(name)
		if err != nil {
			return 0, "unknown group"
		}
		id, err := strconv.ParseUint(g.Gid, 10, 64)
		if err != nil {
			return 0, "unknown group"
		}
		return id, ""
	}
	u, err := user.Lookup(name)
	if err != nil {
		return 0, fmt.Sprintf("unknown username '%s'", name)
	}
	id, err := strconv.ParseUint(u.Uid, 10, 64)
	if err != nil {
		return 0, fmt.Sprintf("unknown username '%s'", name)
	}
	return id, ""
}

// delimitedArgument reads `<delimiter>text<delimiter>` off the front of s and
// says how many characters it took. ok is false when the closing delimiter is
// not there.
//
// The three bracketing characters close with their partners and everything
// else closes with itself, so `u[foo]`, `u{foo}` and `u:foo:` are the same
// argument. `<` is documented alongside them and is unreachable in a qualifier
// list, the `<` ending the word before the list is ever read; it is here
// because the rule is the rule and not because a probe could show it.
func delimitedArgument(s string) (string, int, bool) {
	closing := s[0]
	switch closing {
	case '[':
		closing = ']'
	case '{':
		closing = '}'
	case '<':
		closing = '>'
	}
	end := strings.IndexByte(s[1:], closing)
	if end < 0 {
		return "", 0, false
	}
	return s[1 : 1+end], end + 2, true
}

// fileOwner is the uid or gid behind a stat.
//
// The interpreter is built for Unix — a named pipe is a syscall away in
// procsubst.go — so the platform structure is read here rather than behind a
// second file that would have nothing to say on the platforms that do not
// have it.
func fileOwner(info fs.FileInfo, kind byte) (uint64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	if kind == 'g' {
		return uint64(st.Gid), true
	}
	return uint64(st.Uid), true
}

// osGeteuid and osGetegid are what `U` and `G` compare against.
//
// Process identity rather than process state: nothing a script does changes
// either, two Runners in one program genuinely share them, and $UID and $EUID
// are already read this way in the dialects. See the forbidigo note in
// .golangci.yml, which draws that line explicitly.
func osGeteuid() int { return os.Geteuid() }

func osGetegid() int { return os.Getegid() }

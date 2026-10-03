// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// tildeThroughMark stands in front of a `~` whose prefix runs on through a
// quote or an expansion, in the dialect that reads the name through them
// (Semantics.TildePrefixStopsAtAQuoteOrAnExpansion answering no): the name is
// whatever the word comes to between the tilde and the first `/`, so it is
// read once the word has been expanded. Measured 2026-10-03 on zsh 5.9.2
// under -f:
//
//	~"root"  ~'root'/x  ~\root  ~ro"o"t     /var/root, /var/root/x, …
//	v=root; ~$v  ~${v}  ~$(echo root)      /var/root
//	v=ro; ~${v}ot  ~"root/x"                /var/root, /var/root/x
//	~"/x"  e=; ~$e/x                        the home, and /x behind it
//	~"+"  ~"1"                              $PWD; a stack entry
//	~root"x"  v=root; ~${v}x                no such user or named directory: rootx
//	~"a b"  ~"root":x                       as written: not a name
//	a=(root x); ~$a                         /var/root x: the first field
//
// tildeThroughColonMark is the same for an assignment's value and the words
// shaped like one, where a `:` ends the name as well: `x=~"root":~"root"` is
// two homes (#5655).
const (
	tildeThroughMark      = ""
	tildeThroughColonMark = ""
)

// resolveTildesThrough reads every marked tilde in s, which is in the escaped
// form when escaped says so, and takes the marks out.
func (r *Runner) resolveTildesThrough(s string, escaped bool) string {
	return r.resolveTildesThroughWith(s, escaped, globEscape)
}

// resolveTildesThroughWith is resolveTildesThrough with the escape a
// directory takes in the escaped form named.
func (r *Runner) resolveTildesThroughWith(s string, escaped bool, escape func(string) string) string {
	if !strings.Contains(s, tildeThroughMark) && !strings.Contains(s, tildeThroughColonMark) {
		return s
	}
	var b strings.Builder
	for {
		i, ends, n := nextTildeThroughMark(s)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		s = s[i+n:]
		if !strings.HasPrefix(s, "~") {
			continue
		}
		end := tildeThroughNameEnd(s, ends, escaped)
		name := s[1:end]
		if escaped {
			name = globUnescape(name)
		}
		dir, _, ok, miss := r.tildeSplit("~" + name)
		if !ok {
			r.refuseTilde(miss)
			b.WriteByte('~')
			s = s[1:]
			continue
		}
		if escaped {
			dir = escape(dir)
		}
		b.WriteString(dir)
		s = s[end:]
	}
}

// nextTildeThroughMark finds the next mark of either kind, with the bytes
// that end the name it opens and the mark's own length.
func nextTildeThroughMark(s string) (at int, ends string, n int) {
	i := strings.Index(s, tildeThroughMark)
	j := strings.Index(s, tildeThroughColonMark)
	switch {
	case i < 0 && j < 0:
		return -1, "", 0
	case j < 0 || (i >= 0 && i < j):
		return i, tildeEndsAtASlash, len(tildeThroughMark)
	default:
		return j, tildeEndsAtASlashOrColon, len(tildeThroughColonMark)
	}
}

// tildeThroughNameEnd is where the name after the tilde at s[0] ends: the first
// byte of ends, written or escaped, another mark, or the end of s.
func tildeThroughNameEnd(s, ends string, escaped bool) int {
	for i := 1; i < len(s); i++ {
		c := s[i]
		if escaped && c == '\\' && i+1 < len(s) {
			if strings.IndexByte(ends, s[i+1]) >= 0 {
				return i
			}
			i++
			continue
		}
		if strings.IndexByte(ends, c) >= 0 ||
			strings.HasPrefix(s[i:], tildeThroughMark) || strings.HasPrefix(s[i:], tildeThroughColonMark) {
			return i
		}
	}
	return len(s)
}

// resolveFieldsTildesThrough is resolveTildesThrough over escaped fields.
func (r *Runner) resolveFieldsTildesThrough(fields []string) []string {
	for i, f := range fields {
		fields[i] = r.resolveTildesThrough(f, true)
	}
	return fields
}

// resolvePatternTildesThrough is resolveTildesThrough over a pattern's text,
// where the directory is matched as text, as a written tilde's is. See
// Runner.patternTilde.
func (r *Runner) resolvePatternTildesThrough(s string) string {
	return r.resolveTildesThroughWith(s, true, escapePatternMeta)
}

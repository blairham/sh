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
	tildeThroughMark      = "\uf8fb"
	tildeThroughColonMark = "\uf8fa"
	// tildeThroughWrittenMark opens a name that ends at tildeThroughEndMark,
	// placed where the word writes the closing byte, or at the end of the
	// text where it writes none — the dialect of
	// Semantics.TildeNameEndsOnlyAtAWrittenSlash. A name holding a `/`
	// there is no name, and the word stands as written.
	tildeThroughWrittenMark = "\uf8f9"
	tildeThroughEndMark     = "\uf8f8"
)

// resolveTildesThrough reads every marked tilde in s, which is in the escaped
// form when escaped says so, and takes the marks out.
func (r *Runner) resolveTildesThrough(s string, escaped bool) string {
	return r.resolveTildesThroughWith(s, escaped, globEscape)
}

// resolveTildesThroughWith is resolveTildesThrough with the escape a
// directory takes in the escaped form named.
func (r *Runner) resolveTildesThroughWith(s string, escaped bool, escape func(string) string) string {
	if !strings.Contains(s, tildeThroughMark) && !strings.Contains(s, tildeThroughColonMark) &&
		!strings.Contains(s, tildeThroughWrittenMark) {
		return strings.ReplaceAll(s, tildeThroughEndMark, "")
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
		after := end
		if ends == "" {
			// The written kind: up to its end mark, which is taken out.
			end = len(s)
			if k := strings.Index(s, tildeThroughEndMark); k >= 0 {
				end, after = k, k+len(tildeThroughEndMark)
			} else {
				after = end
			}
		}
		name := s[1:end]
		if escaped {
			name = globUnescape(name)
		}
		if ends == "" && strings.ContainsRune(name, '/') {
			// A slash the word did not write is part of the name, and a name
			// with one in it names nothing.
			b.WriteString(s[:end])
			s = s[after:]
			continue
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
		if ends == "" {
			s = s[after:]
			continue
		}
		s = s[end:]
	}
}

// nextTildeThroughMark finds the next mark of either kind, with the bytes
// that end the name it opens and the mark's own length.
func nextTildeThroughMark(s string) (at int, ends string, n int) {
	at, n = -1, 0
	for _, m := range []struct{ mark, ends string }{
		{tildeThroughMark, tildeEndsAtASlash},
		{tildeThroughColonMark, tildeEndsAtASlashOrColon},
		{tildeThroughWrittenMark, ""},
	} {
		if i := strings.Index(s, m.mark); i >= 0 && (at < 0 || i < at) {
			at, ends, n = i, m.ends, len(m.mark)
		}
	}
	return at, ends, n
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

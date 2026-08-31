// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// globEscape marks a metacharacter as literal while a field is carried around.
//
// A field has to remember which of its metacharacters were quoted, because
// quoting is what decides whether text is a pattern at all — `$p` globs and
// `"$p"` does not. The fields expansion produces are therefore in this escaped
// form and are unescaped once globbing has had its look.
func globEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(`*?[\`, s[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// globUnescape removes the marks, giving the literal field.
func globUnescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// hasUnescapedMeta reports whether a field is a pattern at all.
func hasUnescapedMeta(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == '[' {
			// An unterminated bracket expression is not a pattern: `[` on
			// its own is a literal in every shell in the panel, which is
			// what makes `[ a = a ]` run the test builtin rather than being
			// globbed. Treating it as a metacharacter reported "no matches
			// found: [" on every use of `test`; the report was ignored until
			// an unmatched pattern became fatal, and then the builtin
			// stopped running at all.
			//
			// zsh alone goes further and rejects `[a` as a bad pattern where
			// the others take it literally. That divergence is recorded in
			// the corpus rather than guessed at here.
			if closesBracket(s, i) {
				return true
			}
			continue
		}
		if s[i] == '*' || s[i] == '?' {
			return true
		}
	}
	return false
}

// closesBracket reports whether the bracket expression opened at i is closed.
//
// A `!` or `^` directly after the bracket negates, and a `]` directly after
// that is a literal member rather than the terminator — so `[]]` is a
// one-member class and `[]` is not a class at all.
func closesBracket(s string, i int) bool {
	j := i + 1
	if j < len(s) && (s[j] == '!' || s[j] == '^') {
		j++
	}
	if j < len(s) && s[j] == ']' {
		j++
	}
	for ; j < len(s); j++ {
		if s[j] == '\\' {
			j++
			continue
		}
		if s[j] == ']' {
			return true
		}
	}
	return false
}

// glob expands one field against the filesystem.
//
// The two restrictions live here rather than in the matcher, which is what
// docs/spec/grammar/patterns.md required: no metacharacter matches `/`, so
// patterns are matched one component at a time, and none matches a *leading*
// period, so a component is skipped unless its pattern begins with one.
// `case` and parameter expansion use the same matcher and have neither
// restriction, because they have no filesystem and no components.
//
// A pattern matching nothing is passed through unchanged, which is what dash,
// bash and ksh93 do; zsh reports an error, and that is a recorded axis.
func (r *Runner) glob(field string) []string {
	if r.sem().UnterminatedBracket == BracketBadPattern &&
		field != "[" && hasUnterminatedBracket(field) {
		// zsh rejects an unterminated bracket against the filesystem too,
		// with one exception it is worth stating because it is what keeps
		// `[ a = a ]` working: a field that is exactly `[` is left alone.
		// `a[` is not, so the rule is the whole field rather than where the
		// bracket sits in it.
		r.fatalPattern(field, 1)
		return nil
	}
	if !hasUnescapedMeta(field) {
		return nil
	}
	defer func() {
		if r.globMissed && r.ask(r.sem().GlobNoMatchIsError, "an unmatched pattern being an error") {
			// An error, which in zsh means the command does not run and the
			// script stops. Reporting it and then passing the pattern
			// through was the same report-then-continue bug as the others.
			r.fatal("no matches found: %s\n", globUnescape(field))
		}
		r.globMissed = false
	}()
	parts := strings.Split(field, "/")

	// An absolute pattern starts at the root; a relative one at the working
	// directory, which is the shell's rather than the process's.
	dirs := []string{r.workDir()}
	prefix := ""
	if parts[0] == "" {
		dirs, prefix, parts = []string{"/"}, "/", parts[1:]
	}

	for i, part := range parts {
		if part == "" {
			continue
		}
		var next []string
		for _, dir := range dirs {
			next = append(next, matchIn(dir, part, r.patternOpts(part))...)
		}
		if len(next) == 0 {
			r.globMissed = true
			return nil
		}
		sort.Strings(next)
		dirs = next
		if i < len(parts)-1 {
			// Only directories can be descended into.
			var kept []string
			for _, d := range dirs {
				if info, err := os.Stat(d); err == nil && info.IsDir() {
					kept = append(kept, d)
				}
			}
			dirs = kept
			if len(dirs) == 0 {
				r.globMissed = true
				return nil
			}
		}
	}

	// Results are reported the way the pattern was written: relative if it
	// was relative, so `echo *` lists names and not paths.
	base := r.workDir()
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if prefix == "" {
			if rel, err := filepath.Rel(base, d); err == nil {
				d = rel
			}
		}
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

// matchIn lists the entries of dir matching one pattern component.
func matchIn(dir, pattern string, o patternOpts) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	literal := globUnescape(pattern)
	hidden := strings.HasPrefix(literal, ".")

	var out []string
	for _, e := range entries {
		name := e.Name()
		// Only a *leading* period is special, and only in pathname
		// expansion: `*.b` matches `a.b`, and `.hid` needs `.*id`.
		if strings.HasPrefix(name, ".") && !hidden {
			continue
		}
		if matchPattern(pattern, name, o) {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out
}

func (r *Runner) workDir() string {
	if r.Dir != "" {
		return r.Dir
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}

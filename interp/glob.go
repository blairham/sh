// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
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
// bash and ksh93 do by default; zsh reports an error, and that is a recorded
// axis. The second result is the run-time exception: with
// UnmatchedPatternIsEmpty on, a miss deletes the word, and true says so —
// distinct from a nil match list, which means the field was never a pattern
// or should stand as written.
func (r *Runner) glob(field string) ([]string, bool) {
	if r.noglob {
		// `set -f`. Only the filesystem half is switched off: a pattern in a
		// `case` arm or after `==` still matches, which is measured and is
		// why this is here rather than in the matcher.
		return nil, false
	}
	if r.sem().UnterminatedBracket == BracketBadPattern &&
		field != "[" && hasUnterminatedBracket(field) {
		// zsh rejects an unterminated bracket against the filesystem too,
		// with one exception it is worth stating because it is what keeps
		// `[ a = a ]` working: a field that is exactly `[` is left alone.
		// `a[` is not, so the rule is the whole field rather than where the
		// bracket sits in it.
		r.fatalPattern(field, 1)
		return nil, false
	}
	if !hasUnescapedMeta(field) {
		return nil, false
	}
	defer func() {
		if r.globMissed && r.ask(r.sem().GlobNoMatchIsError, "an unmatched pattern being an error") &&
			!r.MatchOption(UnmatchedPatternIsEmpty) {
			// An error, which in zsh means the command does not run and the
			// script stops. Reporting it and then passing the pattern
			// through was the same report-then-continue bug as the others.
			//
			// Deleting the word wins over complaining about it, which is the
			// only ordering the two settings can have: measured, `setopt
			// nullglob; echo "[" zz* "]"` prints `[ ]` at 0 in a zsh where
			// nomatch is still on. The axis is still asked, so a dialect
			// that answered nothing about it is still told so.
			r.fatal("no matches found: %s\n", globUnescape(field))
		}
		r.globMissed = false
	}()
	// A miss is a miss wherever it is noticed, and what it means is decided
	// once: the word is deleted if the option says so, kept otherwise.
	missed := func() ([]string, bool) {
		r.globMissed = true
		return nil, r.MatchOption(UnmatchedPatternIsEmpty)
	}
	seeHidden := r.MatchOption(PatternsMatchHidden)
	starstar := r.MatchOption(StarStarCrossesDirectories)
	parts := strings.Split(field, "/")

	// An absolute pattern starts at the root; a relative one at the working
	// directory, which is the shell's rather than the process's.
	base := r.workDir()
	dirs := []string{base}
	prefix := ""
	if parts[0] == "" {
		dirs, prefix, parts = []string{"/"}, "/", parts[1:]
	}

	// The directories `**` matched zero levels deep, when it was the last
	// component: the one shell with the option reports those with a trailing
	// slash — `d/**` lists `d/` ahead of what is inside it.
	var selfDirs map[string]bool
	crossed := false

	for i, part := range parts {
		if part == "" {
			continue
		}
		var next []string
		if starstar && part == "**" {
			// The component is the directory itself and everything beneath
			// it. Exactly `**`: anything more — `a**`, an escaped star — is
			// an ordinary component, where adjacent stars collapse to one.
			crossed = true
			last := lastComponent(parts, i)
			for _, dir := range dirs {
				next = append(next, dir)
				if last {
					if selfDirs == nil {
						selfDirs = map[string]bool{}
					}
					selfDirs[dir] = true
				}
				next = r.appendDescendants(next, dir, seeHidden)
			}
			sortMatches(next)
			next = compactSorted(next)
		} else {
			o := r.patternOpts(part)
			o.fold = r.MatchOption(GlobFoldsCase)
			for _, dir := range dirs {
				next = append(next, r.matchIn(dir, part, o, seeHidden)...)
			}
		}
		if len(next) == 0 {
			return missed()
		}
		sortMatches(next)
		dirs = next
		if i < len(parts)-1 {
			// Only directories can be descended into.
			// Through the gate, like every stat: a match the policy hides
			// is not descended into, the same as a match that is no
			// directory.
			var kept []string
			for _, d := range dirs {
				if info, err := r.stat(d); err == nil && info.IsDir() {
					kept = append(kept, d)
				}
			}
			dirs = kept
			if len(dirs) == 0 {
				return missed()
			}
		}
	}

	// Results are reported the way the pattern was written: relative if it
	// was relative, so `echo *` lists names and not paths.
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		self := selfDirs[d]
		if prefix == "" {
			if rel, err := filepath.Rel(base, d); err == nil {
				d = rel
			}
		}
		if d == "." {
			// The starting point itself, which only a zero-level `**` can
			// produce, and which the shell with the option leaves out: `**`
			// lists what is beneath the directory, never the directory.
			continue
		}
		if self {
			d += "/"
		}
		out = append(out, d)
	}
	sortMatches(out)
	if crossed {
		out = compactSorted(out)
	}
	if len(out) == 0 {
		// Everything matched was the starting point itself — `**` over an
		// empty directory — which is no match at all.
		return missed()
	}
	return out, false
}

// lastComponent reports whether nothing but trailing slashes follows parts[i].
func lastComponent(parts []string, i int) bool {
	for _, p := range parts[i+1:] {
		if p != "" {
			return false
		}
	}
	return true
}

// appendDescendants adds everything beneath dir, however deep: files and
// directories both, because whether only directories survive is the caller's
// question — the same split the main loop already makes.
//
// Hidden names are skipped, and skipped for descent too, unless the option
// says otherwise. A symbolic link is listed and never followed: following one
// is how a walk finds the same file twice and a looped link forever.
//
// A method so each directory read passes the gate — `echo /**` enumerates
// whatever it can reach, which is exactly the walk a policy wants to see. A
// denied directory reads as empty and the walk goes no deeper there.
func (r *Runner) appendDescendants(out []string, dir string, seeHidden bool) []string {
	entries, err := r.readDir(dir)
	if err != nil {
		return out
	}
	for _, e := range entries {
		name := e.Name()
		if !seeHidden && strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		out = append(out, path)
		if e.IsDir() {
			out = r.appendDescendants(out, path, seeHidden)
		}
	}
	return out
}

// compactSorted removes adjacent duplicates, which is all the duplicates a
// sorted list has. Only `**` can produce one: two components can expand to
// the same directory by different routes.
func compactSorted(names []string) []string {
	out := names[:0]
	for i, n := range names {
		if i > 0 && n == names[i-1] {
			continue
		}
		out = append(out, n)
	}
	return out
}

// sortMatches puts a glob's matches in order, which is byte order — and that
// is a decision rather than the absence of one.
//
// **In the C locale it is unanimous.** All four shells give
// `1digit Apple Cherry _under banana date` for a directory holding those
// names, on macOS and on Linux alike, and that is what this produces. Both
// sweeps here run under LC_ALL=C, so it is also the only ordering the corpus
// can record.
//
// **Outside it, three of the four collate and dash never does.** That much is
// a clean axis. What cannot be done is the collation itself, and the reason
// is worth keeping next to the code so it is not attempted again:
//
//   - The platforms disagree. Same shells, same locale name, opposite
//     answers: macOS gives `_under 1digit Apple banana Cherry date` and glibc
//     gives `1digit Apple banana Cherry date _under`. No single table is
//     right on both.
//   - A dependency does not settle it. golang.org/x/text/collate implements
//     CLDR, which is close to glibc and not to macOS — so taking this
//     library's first direct dependency would buy a third answer, and be
//     wrong on the platform the panel is measured on.
//   - An approximation is not close enough, and this was tried rather than
//     assumed. "Digits before letters, letters case-insensitively" gets the
//     obvious cases right and is still wrong twice over on an ordinary
//     directory: macOS orders `_` before `-`, which needs the real
//     punctuation weights, and sorts `Ápple` next to `Apple` and `éclair`
//     next to `date`, which needs base-letter folding. Both come from the
//     full table and neither can be derived from what the standard library
//     ships.
//
// So the shell sorts by byte, which is right in the C locale, right for one
// dialect everywhere, and wrong for three outside it — knowingly, and in a
// place that says so.
func sortMatches(names []string) { sort.Strings(names) }

// matchIn lists the entries of dir matching one pattern component. seeHidden
// lifts the leading-period rule, which is the run-time option's doing and not
// the pattern's. A method so the listing passes the gate; a denied directory
// matches nothing, as an unreadable one does.
func (r *Runner) matchIn(dir, pattern string, o patternOpts, seeHidden bool) []string {
	entries, err := r.readDir(dir)
	if err != nil {
		return nil
	}
	literal := globUnescape(pattern)
	hidden := seeHidden || strings.HasPrefix(literal, ".")

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

// workDir is where relative paths start: r.Dir, or `.` when nothing set one.
//
// Never os.Getwd. The process's directory is one answer shared by every
// Runner in the program, and asking for it here is how two embedded shells
// end up fighting over one cwd — the PATH lesson again, this time for
// directories. `.` keeps everything relative and lets the operating system
// resolve each use against wherever the process is, which is the only
// reading of "no directory was handed in" that stays true when the process
// moves. A shell binary wants the absolute answer, and driver — the binary,
// where process-wide questions belong — seeds Dir at construction.
func (r *Runner) workDir() string {
	if r.Dir != "" {
		return r.Dir
	}
	return "."
}

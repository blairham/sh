// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"sort"
	"strings"
)

// globEscape marks a metacharacter as literal while a field is carried around.
//
// A field has to remember which of its metacharacters were quoted, because
// quoting is what decides whether text is a pattern at all — `$p` globs and
// `"$p"` does not. The fields expansion produces are therefore in this escaped
// form and are unescaped once globbing has had its look.
// `<` is in the set for the dialect that has numeric ranges, and `(` and `)`
// for the one that has pattern groups and glob qualifiers. Marking them where
// nothing reads one costs nothing: an escaped ordinary character is that
// character, so `\<` and `<` match the same text everywhere else.
//
// The parentheses are measured rather than added for symmetry. A `(` that
// arrives from a *value* is never a group in the shell that has them:
// `p="f(1|2)"; echo $p` prints `f(1|2)` and `p="*(.)"; echo $p` prints
// `*(.)` without globbing at all, where the same text written literally is
// an alternation and a qualifier list. Quoting says the same from the other
// side — `echo "( x )"` is five characters.
//
// **So is the `|`, and the discriminating case needs a group the value did
// not bring.** The rows above cannot see it: with the `(` already marked
// there is no alternation for a `|` to divide, so a value carrying both
// answers the same either way. The case that tells them apart is a group
// written *literally* around an expansion — which nothing could reach until
// an expansion inside a group was read as one at all (#1331). Measured on
// zsh 5.9.2, 2026-09-08, in a directory holding `ice.zsh`, `other.zsh` and
// one file literally named `ice|x.zsh`:
//
//	L="ice|other"; print -r -- ($L).zsh   no matches found: (ice|other).zsh
//	L="ice|x";     print -r -- ($L).zsh   ice|x.zsh
//	L="ice|other"; print -r -- (${~L}).zsh   ice.zsh other.zsh
//
// The middle row is the one that says it: the `|` is a character the name
// has to contain. The third is the same value with the flag that asks for
// the other reading, which is where an alternation from a value does come
// from.
func globEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(`*?[\<()|`+extendedPatternMeta, s[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// escapeValueBackslashes puts an expansion result into the escaped form
// without touching its live metacharacters.
//
// The escaped form spells "this character was quoted" as a backslash in front
// of it, so `\` is the one byte a value cannot carry unmarked: a backslash
// that was *in the value* is otherwise read as the mark for whatever follows
// it and removed with the marks, which is how `v='a\b'; w=$v` assigned `ab`
// and `v='a\\b'` assigned one backslash where every shell in the panel keeps
// both (#1222). Only globEscape's caller knew to mark them, and it is the
// caller that runs when the result is *not* a pattern — so the loss was
// exactly on the path where the value stays live.
//
// The character behind the backslash is marked too, and that is measured
// rather than symmetry. With files `a\b` and `a*` present, `v='a\*'; echo $v`
// prints `a\*` in bash, bash 3.2, bash-as-sh, dash and zsh: the `*` matched
// neither the name holding a backslash nor the name holding an asterisk, so a
// value's backslash takes the metacharacter status off what follows it while
// staying in the text itself. ksh93 is the one shell that reads it the other
// way, matching `a\b`; that difference is an axis and is #1367's.
//
// What this form cannot express is #1370: where a live metacharacter is still
// beside the backslash the field *is* globbed, and bash and dash want the
// backslash to quote for the match and to reappear in the text a failed match
// restores. One string cannot be both, since the fallback is the unescape of
// the pattern — a backslash that quotes is removed by it, which was this bug.
func escapeValueBackslashes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		b.WriteString(`\\`)
		if i+1 < len(s) {
			i++
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
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
//
// numericRange, patternGroup and extendedPattern are the three constructs
// here whose being a metacharacter is a dialect question rather than a
// universal: a `<`, a `(` and an `@` are ordinary characters in a field
// everywhere else, and in the shells without ranges, bare groups or
// quantified groups none of them ever reaches a pattern at all.
//
// The group is what makes `echo f(1|2)` list `f1` and `f2` — an alternation
// with no `*` or `?` beside it is still a pattern — and it is the same
// answer, read the other way round, that keeps `p="f(1|2)"; echo $p` printing
// six characters: a field with a metacharacter in it is escaped where the
// dialect does not glob the result of an expansion, and one with none is not
// escaped because it has nothing to protect.
//
// **The quantified group is the same answer a third time**, and it was
// missing: `echo @(a|b)` reached the filesystem in no dialect, because `@`
// is not a metacharacter and the `(` behind it is only counted where bare
// groups are. `*(a|b)` and `?(a|b)` worked all along and hid it, their
// quantifier being a metacharacter in its own right — which is why the gap
// showed up as three of the five quantifiers rather than as the construct
// (#1042).
func hasUnescapedMeta(s string, numericRange, patternGroup, extendedPattern, extendedOperators bool) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if extendedOperators && strings.IndexByte(extendedPatternMeta, s[i]) >= 0 {
			// The closure, the exclusion and the negation. They are only
			// metacharacters while the option is on, which is why the answer
			// is threaded in rather than read off the byte: `echo a#` reaches
			// the filesystem there and prints two characters everywhere else.
			return true
		}
		if extendedPattern && quantifiesAGroup(s, i) {
			// The quantifier and its group are one construct, so the `(` is
			// consumed with it rather than left to be counted again below —
			// which matters in the dialect that has both, where a bare `(`
			// is a metacharacter on its own.
			//
			// `i+1` is where the `(` is, and asking at `i` instead is an
			// equivalent mutant rather than a gap: closesGroup skips every
			// byte that is not a parenthesis, and the byte at `i` is a
			// quantifier. Recorded so the next reader does not go looking
			// for the row that would kill it.
			if closesGroup(s, i+1) {
				return true
			}
			continue
		}
		if s[i] == '(' && patternGroup {
			if closesGroup(s, i) {
				return true
			}
			continue
		}
		if s[i] == '<' && numericRange {
			if _, ok := numericRangeWidth(s[i:]); ok {
				return true
			}
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

// hasUnescapedByte reports whether c stands in s outside an escape, for the
// caller that has a character in mind rather than a whole alphabet.
func hasUnescapedByte(s string, c byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == c {
			return true
		}
	}
	return false
}

// numericRangeWidth is the length of the numeric range at the start of s, for
// the caller that needs to know one is there rather than what it means.
// splitNumericRange is the reading of it; this is only the shape.
func numericRangeWidth(s string) (int, bool) {
	o := patternOpts{numericRange: true}
	_, _, rest, ok := splitNumericRange(s, &o)
	if !ok {
		return 0, false
	}
	return len(s) - len(rest), true
}

// closesGroup reports whether the parenthesized group opened at i is closed,
// counting the nested pairs and skipping the ones a backslash claims. An
// unclosed `(` is an ordinary character, the same way an unterminated bracket
// expression is.
// quantifiesAGroup reports whether the byte at i is one of the five
// quantifiers with a `(` behind it.
//
// The `(` is the whole of the test: `@` and `+` and `!` are ordinary
// characters anywhere else in a field, and `echo @x` looks for a file called
// `@x` in every shell in the panel.
func quantifiesAGroup(s string, i int) bool {
	if i+1 >= len(s) || s[i+1] != '(' {
		return false
	}
	switch s[i] {
	case '@', '?', '+', '*', '!':
		return true
	}
	return false
}

func closesGroup(s string, i int) bool {
	depth := 0
	for ; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return true
			}
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
	if r.noglob || r.globSuspended {
		// `set -f`, or a context that reads a word as text. Only the
		// filesystem half is switched off: a pattern in a `case` arm or
		// after `==` still matches, which is measured and is why this is
		// here rather than in the matcher.
		return nil, false
	}
	// The qualifier list a pattern may carry at its end, read before
	// anything else looks at the field — it decides what the *pattern* is.
	// After `set -f`, because a word globbing is the condition for the group
	// being a list at all: measured, `setopt no_glob; echo MY ( x )` prints
	// those five characters rather than naming a file attribute.
	whole := field
	field, quals, hasQuals, qok := r.fieldQualifiers(field)
	if !qok {
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
	if !hasQuals && !hasUnescapedMeta(field, r.dialect().NumericRangePattern,
		r.dialect().PatternAlternation, r.dialect().ExtendedPattern,
		r.MatchOption(ExtendedPatternOperators)) {
		return nil, false
	}
	if r.MatchOption(ExtendedPatternOperators) &&
		hasTopLevelExclusion(field, patternOpts{extended: true}) &&
		strings.Contains(field, "/") {
		// The exclusion is looser than `/` — measured, `**/x~*bar*` takes
		// `bar/x` out by matching the whole path — and this walk reads a
		// pattern one component at a time, so an exclusion that crosses a
		// component is not something it can answer. Refused by name rather
		// than answered per component, which would quietly compare the right
		// side against a file's name alone.
		r.diagf("%s: a `~` exclusion spanning a path component is not implemented\n",
			globUnescape(whole))
		r.status = 1
		r.ctl = controlExit
		return nil, false
	}
	// A list makes the field a pattern whatever is in front of it: `f1(.)`
	// is `f1` where the name alone is no pattern at all, so the qualifiers
	// are what sent it to the filesystem.
	defer func() {
		if r.ctl == controlExit {
			// The pattern was rejected while it was being read, and the
			// script is already being abandoned. Reporting a miss on top of
			// that says the pattern matched nothing, which is a different
			// and weaker claim than the one already made.
			r.globMissed = false
			return
		}
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
			r.fatal("no matches found: %s\n", globUnescape(whole))
		}
		r.globMissed = false
	}()
	// A miss is a miss wherever it is noticed, and what it means is decided
	// once: the word is deleted if the option says so, kept otherwise.
	missed := func() ([]string, bool) {
		if quals.allowNoMatch {
			// `N` is `null_glob` for one pattern: the word is deleted and
			// nothing is said. Measured, `echo zz*(N)` prints an empty line
			// at status 0 where `echo zz*` is fatal.
			return nil, true
		}
		r.globMissed = true
		return nil, r.MatchOption(UnmatchedPatternIsEmpty)
	}
	// `D` is `glob_dots` for one pattern, and the option is the other way
	// into the same question.
	seeHidden := r.MatchOption(PatternsMatchHidden) || quals.seeHidden
	starstar := r.MatchOption(StarStarCrossesDirectories)
	starstarAlone := r.MatchOption(StarStarAloneCrossesDirectories)
	parts := strings.Split(field, "/")

	// The slashes a pattern ends with are text, and they come back on every
	// match. `*/` is the standard spelling of "directories only" and the
	// whole panel answers `d1/ d2/` where this walk answered `d1 d2` — a
	// quiet wrong answer rather than a loud one, because the name alone is
	// still usable for `cd` and only stops being right once something joins
	// it to a second path or compares the two spellings (#1350).
	//
	// The filtering that makes the idiom mean what it means was already
	// here: the empty component the trailing slash leaves in parts is what
	// puts the real one under `i < len(parts)-1` and keeps only directories.
	// So all that was missing is writing the slash back.
	//
	// **The run is reproduced as written rather than normalized to one**,
	// which is measured and is where the panel splits. dash, ksh93 and zsh
	// answer `d1//` for `*//` and `d1///` for `*///` — the trailing text
	// comes back byte for byte — while bash alone collapses the run to a
	// single slash. Reproducing it is the same rule the mid-pattern case
	// already follows unanimously, `cx//*` being `cx//ax` in all six
	// columns, so it is the reading that stays consistent rather than the
	// one that needs a second rule for the end of the word. bash's collapse
	// is a divergence recorded in the corpus and not implemented; it is a
	// question about a shape no script writes.
	trail := field[len(strings.TrimRight(field, "/")):]

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
		// The two questions a `**` component raises, and they are separate:
		// whether it crosses levels at all, and whether it still does with
		// nothing behind it. A **slash** is what the second one asks about,
		// so the index is the test rather than lastComponent — `**/` has a
		// component after it, empty and written, and is level-crossing in
		// all three shells that have the construct, where bare `**` is not.
		slashed := i < len(parts)-1
		if starstar && part == "**" && (slashed || starstarAlone) {
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
		} else if lit := globUnescape(part); lit == "." || lit == ".." {
			// `.` and `..` **name** a directory rather than describe one, so
			// this component is joined and never matched. No listing reports
			// either name — Go's ReadDir does not, and neither does any
			// shell's — so matching it against one answers nothing, which is
			// why the whole pattern used to be a miss. Measured unanimous
			// across the six: `./cx/*` is `./cx/ax`, `cx/./*` is `cx/./ax`,
			// `cx/../*` is `cx/../ax`, and `*/..` is `cx/..`.
			//
			// The literal behind the quoting marks is what is tested,
			// because quoting a component does not change what it names:
			// `"."/cx/*` and `\./cx/*` both list `./cx/ax` in all six. Today
			// the two spellings coincide — globEscape marks only the
			// metacharacters, and a period is not one — so this normalizing
			// is defensive rather than load-bearing, and a mutation that
			// drops it survives. It is written against the literal so that
			// it stays right if that set ever grows.
			//
			// Nothing here checks that the join exists, and nothing needs to.
			// Every directory standing at this point came out of a listing or
			// through the descent gate below, so `dir/.` and `dir/..` both
			// do. That gate is also the reason `ax/./*` is a miss in all six
			// and stays one here: `ax` is a file, and it is dropped before
			// this component is reached.
			for _, dir := range dirs {
				next = append(next, globJoin(dir, lit))
			}
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

	if hasQuals {
		// Narrowed here, on the absolute paths the walk produced, because a
		// type test is a question about a file and the relative names below
		// are not what would answer it.
		dirs = r.keepQualified(dirs, quals)
		if len(dirs) == 0 {
			return missed()
		}
	}

	// Results are reported the way the pattern was written: relative if it
	// was relative, so `echo *` lists names and not paths — and spelled the
	// way the pattern spelled it, which is why this strips a prefix rather
	// than asking filepath.Rel. Rel *cleans*, so it would answer `cx/ax`
	// where all six columns answer `./cx/ax` even once the walk carries the
	// component; the whole point of globJoin is undone by one call here.
	rel := strings.TrimSuffix(base, "/") + "/"
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		self := selfDirs[d]
		if prefix == "" {
			if d == base {
				// The starting point itself, which only a zero-level `**`
				// can produce, and which the shell with the option leaves
				// out: `**` lists what is beneath the directory, never the
				// directory. Asked against the base rather than against a
				// rendered `.`, because `.` is now a spelling a pattern can
				// legitimately produce — zsh's `.(/)` is `.` — and the two
				// are different strings here: this one is `<base>`, that one
				// is `<base>/.`.
				continue
			}
			d = strings.TrimPrefix(d, rel)
		}
		if self && trail == "" {
			// The zero-level `**` writes its own separator, and only when
			// the pattern did not already ask for one. `cx/**/` is
			// `cx/ cx/dx/` in the two shells that cross levels, not
			// `cx// cx/dx/`, so the two sources of a trailing slash are one
			// slash and not two.
			d += "/"
		}
		d += trail
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
		path := globJoin(dir, name)
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

	// Whether a unit is a character is a question about the subject as well
	// as the pattern, and here the subjects are the names in this directory —
	// `?` is ASCII and still has to consume a whole character of a filename.
	// Asked once for the listing rather than once per name, so a directory
	// full of them earns one diagnostic from the core rather than one each.
	o.chars = r.patternCountsCharacters(append(entryNames(entries), pattern)...)

	var out []string
	for _, e := range entries {
		name := e.Name()
		// Only a *leading* period is special, and only in pathname
		// expansion: `*.b` matches `a.b`, and `.hid` needs `.*id`.
		if strings.HasPrefix(name, ".") && !hidden {
			continue
		}
		if matchPattern(pattern, name, o) {
			out = append(out, globJoin(dir, name))
		}
	}
	return out
}

// globJoin appends one name to a directory the walk is holding, and — unlike
// filepath.Join — does not clean.
//
// Cleaning is what destroyed the spelling. `filepath.Join("<base>", ".")` is
// `<base>`, so the written form of a `.` or `..` component was gone at the
// first join, long before anything relativized; joining the component instead
// of matching it would have made `./cx/*` *match* and still answer `cx/ax`
// where all six columns answer `./cx/ax`. Both helpers that build a path have
// to agree about this — matchIn and appendDescendants — or a `**` descent
// quietly cleans back what the component walk kept.
//
// An uncleaned path is what the kernel resolves anyway, and it is the more
// faithful answer where `..` meets a symbolic link: `sym/../ax` resolves
// through the link, which is what every shell in the panel reports, rather
// than textually back to the link's own parent.
//
// A directory already ending in a separator is the one case worth a branch:
// the root, and a Dir written with a trailing slash.
func globJoin(dir, name string) string {
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}

// entryNames is the names of a directory listing, for the question above.
func entryNames(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
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

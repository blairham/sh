// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// Two answers this shell gives about `alias` that no other column does, both
// of them reachable only through the preset table this dialect ships — which
// is why they are asked here rather than in interp, where the axes
// themselves are pinned.
//
// Every row measured 2026-09-16 on AT&T ksh93u+ 2012-08-01, as a script file
// under `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin `/dev/null`. An
// interactive probe would have measured a different table: this shell ships
// nineteen aliases before a script has run a line (#2926, #2927).
func TestUnaliasCountsANameItHasNamed(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// The name does not go when the value does, so a second removal
		// finds an entry and succeeds. The third row is the control — a
		// name this shell never saw fails, so it is not `unalias` being
		// lenient.
		{
			"a removed alias is removable again",
			`alias h=1
unalias h; echo $?
unalias h 2>/dev/null; echo $?
unalias h 2>/dev/null; echo $?
unalias nevernamed_zz 2>/dev/null; echo $?`,
			"0\n0\n0\n1\n",
		},
		// Naming is enough: a lookup that failed leaves the name behind.
		{
			"a failed lookup leaves the name",
			`alias z 2>/dev/null; echo $?
unalias z 2>/dev/null; echo $?`,
			"1\n0\n",
		},
		// A preset is an ordinary entry and goes the same way.
		{
			"a preset removed twice",
			`unalias r; echo $?
unalias r 2>/dev/null; echo $?`,
			"0\n0\n",
		},
		// And `unalias -a` clears the names with the table, so a preset's
		// name fails afterwards like any other.
		{
			"removing everything forgets the names",
			`alias h=1
unalias h
unalias -a
unalias h 2>/dev/null; echo $?
unalias autoload 2>/dev/null; echo $?`,
			"1\n1\n",
		},
		// A remembered name is not an alias: nothing looks it up and no
		// listing has it.
		{
			"a remembered name is no alias",
			`alias h=1
unalias h
alias h 2>/dev/null; echo $?
IFS='
'
set -- $(alias)
for e in "$@"; do case $e in h=*) echo listed;; esac; done
echo end`,
			"1\nend\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKshWithPrelude(t, c.src+"\n")
			if out != c.want {
				t.Errorf("%s =\n%q\nwant\n%q", c.src, out, c.want)
			}
		})
	}
}

// `alias -x` is the second letter this shell's own usage line advertises —
// `Usage: alias [-ptx] [name[=value]...]` — and it was refused here while
// that line named it, which is the accepted set and the message apart
// (#2927).
func TestAliasTakesItsExportLetter(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// The call succeeds and the shell carries on, which is the whole of
		// what the suite file asks: an option `alias` has not got is fatal
		// here, so a refusal would leave the rest of a file unrun.
		{
			"a definition is taken and the script carries on",
			"alias -x xx=1; echo $?\nalias xx\necho after",
			"0\nxx=1\nafter\n",
		},
		// The listing is narrowed to the marked entries, which is why a
		// stock shell answers `alias -x` with nothing at all while `alias`
		// lists nineteen presets.
		{
			"the listing is narrowed to the marked entries",
			"alias -x\necho ---\nalias -x xx=1\nalias -x",
			"---\nxx=1\n",
		},
		// A name already defined is marked rather than looked up: silent,
		// at 0, and a preset can be marked like anything else.
		{
			"a bare name marks what the table holds",
			"alias -x r; echo $?\nalias -x",
			"0\nr='hist -s'\n",
		},
		// A name the table has not got is neither a complaint nor a
		// definition — where the same call without the letter is `alias not
		// found` at 1.
		{
			"a bare name the table has not got",
			"alias -x zz; echo $?\nalias -x\nunalias zz; echo $?",
			"0\n0\n",
		},
		// The mark is on the name rather than on the value.
		{
			"the mark survives a redefinition",
			"alias -x e=3\nalias e=4\nalias -x",
			"e=4\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKshWithPrelude(t, c.src+"\n")
			if out != c.want {
				t.Errorf("%s =\n%q\nwant\n%q", c.src, out, c.want)
			}
		})
	}
}

// The three answers left over from the two above, all of them about the same
// table and none of them reachable without this shell's own presets.
//
// Measured 2026-09-18 on AT&T ksh93u+ 2012-08-01, as script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with stdin `/dev/null`, and again over
// `-c` and stdin with the same bytes on each (#3063, #3429, #3430).
func TestTheSeparatorTheCarryOverAndTheValuelessMark(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// `--` ends the lookup here and not only the options. The first
		// two rows are the pair that says so: a preset is an alias and its
		// line does not appear either.
		{
			"a name behind the separator is not looked up",
			"alias -- r; echo $?\nalias -- nosuch; echo $?",
			"0\n0\n",
		},
		{
			"and the same names without it",
			"alias r; echo $?\nalias nosuch 2>/dev/null; echo $?",
			"r='hist -s'\n0\n1\n",
		},
		{
			"a definition behind the separator still defines",
			"alias -- z=1; echo $?\nalias z",
			"0\nz=1\n",
		},
		// What a `( … )` or a `$( … )` *named* is remembered out here, and
		// what it defined is not.
		{
			"a name the parentheses named counts in the parent",
			"( alias z 2>/dev/null ); unalias z; echo $?",
			"0\n",
		},
		{
			"a command substitution's too",
			"x=$(alias z 2>/dev/null); unalias z; echo $?",
			"0\n",
		},
		{
			"the value it defined is not kept",
			"( alias z=1 ); alias z 2>/dev/null; echo $?\nunalias z; echo $?",
			"1\n0\n",
		},
		{
			"a pipeline element's name is not",
			"alias z 2>/dev/null | :; unalias z 2>/dev/null; echo $?",
			"1\n",
		},
		// A mark with no value behind it is in the prefixed listing alone,
		// as a prefix with nothing after it and no newline.
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKshWithPrelude(t, c.src+"\n")
			if out != c.want {
				t.Errorf("%s =\n%q\nwant\n%q", c.src, out, c.want)
			}
		})
	}
}

// A mark with no value behind it is in the **prefixed** listing alone, where
// it writes the prefix and then nothing — no name, no `=` and no newline — so
// the entry after it glues onto the same line.
//
// Measured 2026-09-18 on ksh93u+ 2012-08-01 from a script file: `alias -x bb;
// alias -p` writes `alias alias command='command '` at `bb`'s sorted
// position, between `autoload` and `command`. The plain listing and `alias
// -x` alike are empty of it, and `alias bb` is still `alias not found` at 1
// (#3430).
func TestAValuelessMarkIsInThePrefixedListingAlone(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the glue", "alias -x bb\nalias -p", "alias alias command='command '\n"},
		{"the narrowed listing glues too", "alias -x bb\nalias -px", "alias "},
		{"a definition takes the mark over", "alias -x bb\nalias bb=1\nalias -p", "\nalias bb=1\n"},
		{"a removal drops it", "alias -x bb\nunalias bb\nalias -p", "\nalias command='command '\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKshWithPrelude(t, c.src+"\n")
			if !strings.Contains(out, c.want) {
				t.Errorf("%s =\n%q\nwant it to contain\n%q", c.src, out, c.want)
			}
		})
	}
}

// And the two rows that say the mark is nowhere else, which is what keeps it
// out of every reader of the table but one.
func TestAValuelessMarkIsInNoOtherListing(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"not in the narrowed listing", "alias -x bb\nalias -x", ""},
		{"and the name is no alias", "alias -x bb\nalias bb 2>/dev/null; echo $?", "1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKshWithPrelude(t, c.src+"\n")
			if out != c.want {
				t.Errorf("%s =\n%q\nwant\n%q", c.src, out, c.want)
			}
		})
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `comparguments -i`'s first word is the `auto-description` style, and this
// read it as the matcher `-M` answers with (#6192). Every row was measured on
// zsh 5.9.2, 2026-10-05, from inside a real `zle -C` widget: the same spec
// set under each style, `-O`'s four arrays and `-M` read back.

const autoDescSpecs = `'-v:verbosity level:(1 2)' '-a[all]' '-q' ` +
	`'-w+:width:' '--long=:lval:' '-e:' '-y: :' '-u::opt msg:' ` +
	`'-k:first:(a):second:(b)' '-j[]:jm:'`

func TestTheAutoDescriptionStyleDescribesAnOptionByItsArgument(t *testing.T) {
	for _, c := range []struct{ style, want string }{
		{"", "-v -a:all -q -e -y -u -k -j:|-w|--long|r:|[_-]=* r:|=*"},
		{"%d", "-v:verbosity level -a:all -q -e -y -u:opt msg -k -j:|-w:width|--long:lval|r:|[_-]=* r:|=*"},
		{"X %d Y", "-v:X verbosity level Y -a:all -q -e -y -u:X opt msg Y -k -j:|-w:X width Y|--long:X lval Y|r:|[_-]=* r:|=*"},
		// No `%d`, no description — and still not a matcher.
		{"plain", "-v -a:all -q -e -y -u -k -j:|-w|--long|r:|[_-]=* r:|=*"},
		// Only the first `%d` is replaced.
		{"%d%d", "-v:verbosity level%d -a:all -q -e -y -u:opt msg%d -k -j:|-w:width%d|--long:lval%d|r:|[_-]=* r:|=*"},
	} {
		t.Run("style "+c.style, func(t *testing.T) {
			got := reported(t, `comparguments -i '`+c.style+`' : `+autoDescSpecs+`
				local -a n d o e; local m
				comparguments -O n d o e
				comparguments -M m
				say "${n[*]}|${o[*]}|${e[*]}|$m"`, "cmd -")
			if got != c.want {
				t.Errorf("auto-description %q:\n got %q\nwant %q", c.style, got, c.want)
			}
		})
	}
}

// TestAnEmptyBracketIsStillADescription — `-j[]` is offered as `-j:`, by
// comparguments and compvalues alike, and the colon is what files it among
// the described names.
func TestAnEmptyBracketIsStillADescription(t *testing.T) {
	got := reported(t, `comparguments -i '' : '-j[]:jm:' '-k[]' '-l[ ]' '-m[x]:mm:' '-n'
		local -a n d o e
		comparguments -O n d o e
		say "${(j:,:)n}"`, "cmd -")
	if want := "-j:,-k:,-l: ,-m:x,-n"; got != want {
		t.Errorf("comparguments -O: got %q, want %q", got, want)
	}
	got = reported(t, `compvalues -i dd 'aa[]' bb 'cc[x]' 'dd[]:m:'
		local -a n a
		compvalues -V n a x
		say "${(j:,:)n}|${(j:,:)a}"`, "cmd ")
	if want := "aa:,bb,cc:x|dd:"; got != want {
		t.Errorf("compvalues -V: got %q, want %q", got, want)
	}
}

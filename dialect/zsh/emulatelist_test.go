// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `emulate -l MODE` prints the options the emulation would set, rather than
// setting them (#5249). See listEmulation for the whole rule. Measured on
// zsh 5.9.2, `-f`, 2026-09-30; on main before this change every row was `bad
// option: -l`.

// emulateListSh is the reference's `emulate -l sh`, whole.
const emulateListSh = `aliases aliasfuncdef noallexport appendcreate noautocd nobadpattern
nobareglobqual nobgnice nobraceccl bsdecho nocdablevars nochasedots
nochaselinks nocheckjobs nocheckrunningjobs clobber cprecedences
nocshjunkiehistory nocshjunkieloops nocshjunkiequotes nocshnullcmd
nocshnullglob noequals noerrexit noerrreturn noevallineno
noextendedglob nofunctionargzero glob noglobalexport noglobassign
noglobdots noglobstarshort globsubst nohistsubstpattern nohup
ignorebraces noignoreclosebraces ksharrays kshautoload nokshglob
nokshoptionprint nolocalloops nolocaloptions nolocalpatterns
nolocaltraps nomagicequalsubst nomultifuncdef nomultios nonomatch
nonullglob nonumericglobsort octalzeroes nopathdirs pathscript
nopipefail posixaliases noposixargzero posixbuiltins posixcd
posixidentifiers posixjobs posixstrings posixtraps nopushdignoredups
nopushdminus nopushdtohome norcexpandparam norcquotes shfileexpansion
shglob shnullcmd shoptionletters noshortloops noshortrepeat
shwordsplit typesetsilent typesettounset unset nowarncreateglobal
nowarnnestedvar`

func TestEmulateListPrintsTheEmulationsOwnValues(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "emulate -l sh")
	want := strings.Join(strings.Fields(emulateListSh), "\n") + "\n"
	if st != 0 || out != want {
		t.Errorf("status %d, got\n%s\nwant\n%s", st, out, want)
	}
}

func TestEmulateListForms(t *testing.T) {
	// A summary of one listing: its length, its ends, and the rows the forms
	// move.
	sum := func(c string) string {
		return `x=("${(@f)$(` + c + `)}"); print n=${#x} first=$x[1] last=$x[-1] ` +
			`ng=${x[(r)nonullglob]}${x[(r)nullglob]} sw=${x[(r)noshwordsplit]}${x[(r)shwordsplit]} ` +
			`lo=${x[(r)nolocaloptions]}${x[(r)localoptions]} ex=${x[(r)exec]}`
	}
	rows := []struct{ name, src, want string }{
		{
			"zsh's own values", sum("emulate -l zsh"),
			"n=81 first=aliases last=nowarnnestedvar ng=nonullglob sw=noshwordsplit lo=nolocaloptions ex=",
		},
		{
			"ksh's", sum("emulate -l ksh"),
			"n=81 first=aliases last=nowarnnestedvar ng=nonullglob sw=shwordsplit lo=localoptions ex=",
		},
		{
			"-R lists the 95 more and exec", sum("emulate -lR sh"),
			"n=177 first=aliases last=noxtrace ng=nonullglob sw=shwordsplit lo=nolocaloptions ex=exec",
		},
		{
			"-L lists the local switches on", sum("emulate -lL sh"),
			"n=81 first=aliases last=nowarnnestedvar ng=nonullglob sw=shwordsplit lo=localoptions ex=",
		},
		{
			"the mode word by its first letter", sum("emulate -l bash"),
			"n=81 first=aliases last=nowarnnestedvar ng=nonullglob sw=shwordsplit lo=nolocaloptions ex=",
		},
		{
			"not the shell's own state", "setopt nullglob; " + sum("emulate -l zsh"),
			"n=81 first=aliases last=nowarnnestedvar ng=nonullglob sw=noshwordsplit lo=nolocaloptions ex=",
		},
		{"alone it prints the mode", "emulate csh; emulate -l; print st=$?", "csh\nst=0"},
		{"-lR alone", "emulate -lR; print st=$?", "not enough arguments\nst=1"},
		{"anything after the mode", "emulate -l sh -o bad; print st=$?", "too many arguments for -l\nst=1"},
		{"a -c after the mode", "emulate -l sh -c 'print ran'; print st=$?", "too many arguments for -l\nst=1"},
		// It switches the mode word and what follows it, and no option.
		{"the mode word moves", "emulate csh; emulate -l sh >/dev/null; emulate", "sh"},
		{
			"no option moves",
			"setopt nullglob; emulate -l sh >/dev/null; [[ -o nullglob ]] && print ng; [[ -o shwordsplit ]] || print no-sw",
			"ng\nno-sw",
		},
		{"the last pipeline element moves with it", "emulate -l sh >/dev/null; x=1; echo | x=2; print x=$x", "x=1"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), row.src)
			var got []string
			for _, l := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
				// The diagnostic's prefix names the route: keep what
				// follows `emulate:N: `.
				if i := strings.Index(l, "emulate:"); i >= 0 {
					if j := strings.Index(l[i+len("emulate:"):], ": "); j >= 0 {
						l = l[i+len("emulate:")+j+2:]
					}
				}
				got = append(got, l)
			}
			if g := strings.Join(got, "\n"); g != row.want {
				t.Errorf("got %q, want %q", g, row.want)
			}
		})
	}
}

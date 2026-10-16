// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// globCodeFixture is the directory every row below was measured in: regular
// files `a b big c10 c9`, `big` holding six bytes, a directory `d` and a link
// `lnk` to `big`.
func globCodeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"c9", "big", "a", "c10", "b"} {
		body := ""
		if name == "big" {
			body = "hello\n"
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("big", filepath.Join(dir, "lnk")); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The `e` and `+` glob qualifiers run shell code for each file, keep the file
// when the code's status is 0, and take what the code leaves in `REPLY` or
// `reply` as the word the file is reported as (#6612). Every row was measured
// on zsh 5.9.2 under `-fc` in the fixture above; see interp/globqualcode.go.
func TestAGlobQualifierRunsCodeForEachFile(t *testing.T) {
	const all = "a b big c10 c9 d lnk\n"
	for _, tc := range []struct{ src, out, errs string }{
		// What the code is asked, and the delimiters it may be written in.
		{`print *(e:'[[ -d $REPLY ]]':)`, "d\n", ""},
		{`print *(e"%[[ -d \$REPLY ]]%")`, "d\n", ""},
		{`print *(e'*[[ -d $REPLY ]]*')`, "d\n", ""},
		{`print *(e/true/) *(e,false,N)`, all, ""},
		{`print *(e[true])`, all, ""},
		{`print *(e:false:); print after`, "", "zsh:1: no matches found: *(e:false:)\n"},
		// A delimiter is a character whether or not it was quoted, and a
		// bracket closes at its first partner.
		{`print *(e'[[ -d $REPLY ]]')`, "", "zsh:1: unknown file attribute: ]\n"},
		{`print *(e:'a:b':)`, "", "zsh:1: unknown file attribute: b\n"},
		{`print *(e)`, "", "zsh:1: missing end of string\n"},
		// A quoted bar is code, not an alternation; a quoted letter is the
		// letter.
		{`print *(e:'true||false':)`, all, ""},
		{`print *(.\|)`, "", "zsh:1: unknown file attribute: |\n"},
		{`print *(\/)`, "d\n", ""},
		// What the code leaves the name as.
		{`print *(e:'REPLY=x$REPLY':)`, "xa xb xbig xc10 xc9 xd xlnk\n", ""},
		{`print *(e:'reply=(x$REPLY y)':)`, "xa xb xbig xc10 xc9 xd xlnk y y y y y y y\n", ""},
		{`print *(e:'reply=(x$REPLY y)':On)`, "y y y y y y y xlnk xd xc9 xc10 xbig xb xa\n", ""},
		{`print *(e:'REPLY=q; reply=(r s)':[1,3])`, "r r r\n", ""},
		{`print *(e:'reply=()':); print after`, "", "zsh:1: no matches found: *(e:reply=():)\n"},
		{`print *(e:'unset REPLY':)`, all, ""},
		{`print *(e:'REPLY=x$REPLY':e:'REPLY=y$REPLY':)`, "ya yb ybig yc10 yc9 yd ylnk\n", ""},
		{`print *(^e:'REPLY=z$REPLY; false':)`, "za zb zbig zc10 zc9 zd zlnk\n", ""},
		{`print *(e:'REPLY=z$REPLY; false':,/)`, "zd\n", ""},
		{`print *(/,e:'REPLY=z$REPLY':)`, "d za zb zbig zc10 zc9 zlnk\n", ""},
		{`print *(e:'REPLY=/x/$REPLY'::t)`, all, ""},
		{`print *(e:'[[ $REPLY = [bc]* ]]':[2,3])`, "big c10\n", ""},
		{`print *(.e:'[[ $REPLY = b* ]]':)`, "b big\n", ""},
		{`setopt markdirs; print *(e:'REPLY=x$REPLY':)`, "xa xb xbig xc10 xc9 xd/ xlnk\n", ""},
		// REPLY is the word as the pattern spells it.
		{`print ./a*(e:'REPLY=@$REPLY@':) */(e:'REPLY=@$REPLY@':)`, "@./a@ @d/@\n", ""},
		// The code runs in this shell, in the caller's scope.
		{`f() { local L; print *(e:'L=1':) >/dev/null; print $L; }; f; print "[$L]"`, "1\n[]\n", ""},
		{`reply=(o1); print a(e:true:) >/dev/null; print ${+reply} $REPLY`, "0 a\n", ""},
		{`print a(e:'print -r -- $zsh_eval_context >&2':)`, "a\n", "toplevel globqual\n"},
		{`x=(*(e:'y=($REPLY*); (( $#y > 1 ))':)); print $x`, "b\n", ""},
		// Failures in the code are the code's, once per file, located where
		// the pattern is; a few still end the shell.
		{
			`print *(e:'[[ $REPLY = a ]] && nosuchcmd':)`, "",
			"zsh:1: command not found: nosuchcmd\nzsh:1: no matches found: *(e:[[ $REPLY = a ]] && nosuchcmd:)\n",
		},
		{
			"\n\nprint *(e:'[[ $REPLY != a ]] || {\nnosuchcmd; }':)",
			"b big c10 c9 d lnk\n", "zsh:4: command not found: nosuchcmd\n",
		},
		// The code's lines move the shell's, and the pattern's line comes
		// back after it.
		{"\n\nprint [ab](e:'true\n\nfalse':N) $LINENO", "3\n", ""},
		{
			`print [ab](e:'if':N); print after`, "\nafter\n",
			"zsh:1: parse error near `if'\nzsh:1: parse error near `if'\n",
		},
		{
			`print [ab](e:'echo [x]':N); print after`, "\nafter\n",
			"zsh:1: no matches found: [x]\nzsh:1: no matches found: [x]\n",
		},
		{`print *(e:'echo ${x?boom}':); print after`, "", "zsh:1: x: boom\n"},
		{`print *(e:'exit 3':); print after`, "", ""},
		{`set -e; print a(e:'false':N); print after`, "", ""},
		{`f() { print X *(e:'[[ $REPLY != a ]] || return 4':); print inf; }; f; print $?`, "X b big c10 c9 d lnk\n0\n", ""},
		// A program's word holds the refusal for the program, and the code's
		// own refusals are its own.
		{
			`/usr/bin/true [ab](e:'echo [x]':); print after $?`, "after 1\n",
			"zsh:1: no matches found: [x]\nzsh:1: no matches found: [x]\nzsh:1: no matches found: [ab](e:echo [x]:)\n",
		},
		{
			`/usr/bin/true [ab](e:'for i in [x]; do true; done':); print after $?`, "after 1\n",
			"zsh:1: no matches found: [x]\nzsh:1: no matches found: [x]\nzsh:1: no matches found: [ab](e:for i in [x]; do true; done:)\n",
		},
		// `+name` is `e:name:`.
		{`isd() { [[ -d $REPLY ]] }; print *(+isd) *(^+isd)`, "d a b big c10 c9 lnk\n", ""},
		{`isd() { [[ -d $REPLY ]] }; print *(+isd.); print after`, "", "zsh:1: no matches found: *(+isd.)\n"},
		{`f() { REPLY=x$REPLY }; print *(+f[1,2])`, "xa xb\n", ""},
		{`f() { reply=(1 2) }; print a(+f)`, "1 2\n", ""},
		{`print a(+true) a(+false,.)`, "a a\n", ""},
		{`print *(+)`, "", "zsh:1: missing identifier after `+'\n"},
		{`print *(+.)`, "", "zsh:1: missing identifier after `+'\n"},
		{
			`print [ab](+nosuchfn)`, "",
			"zsh:1: command not found: nosuchfn\nzsh:1: command not found: nosuchfn\nzsh:1: no matches found: [ab](+nosuchfn)\n",
		},
	} {
		out, _, errs := runZshSplit(t, globCodeFixture(t), tc.src)
		if out != tc.out || errs != tc.errs {
			t.Errorf("%q:\n out %q errs %q\nwant %q errs %q", tc.src, out, errs, tc.out, tc.errs)
		}
	}
}

// The code sees the files in the directory's own order, the order `ls -f`
// lists them in, and not in the order the matches are reported in.
func TestAGlobQualifiersCodeRunsInTheDirectorysOrder(t *testing.T) {
	dir := globCodeFixture(t)
	f, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := f.Readdirnames(-1)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	out, _, errs := runZshSplit(t, dir, `print -r -- *(e:'print -rn -- "$REPLY " >&2':)`)
	if want := strings.Join(raw, " ") + " "; errs != want {
		t.Errorf("the code saw %q, want the listing's own order %q", errs, want)
	}
	sorted := slices.Clone(raw)
	slices.Sort(sorted)
	if want := strings.Join(sorted, " ") + "\n"; out != want {
		t.Errorf("out %q, want the matches in name order %q", out, want)
	}
}

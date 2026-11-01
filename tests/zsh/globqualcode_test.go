// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
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
	t.Parallel()
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
		// The code globs, tests and sets by the shell's own numericglobsort,
		// not by the `n` of the pattern it is running for.
		{`print -r -- *(ne:'[[ $REPLY == a ]] && print -r -- in c*':)`, "in c10 c9\na\n", ""},
		{`print -r -- *(e:'[[ $REPLY == a ]] && print -r -- in c*':n)`, "in c10 c9\na\n", ""},
		{
			`setopt numericglobsort; print -r -- *(^ne:'[[ $REPLY == a ]] && print -r -- in c*':)`,
			"in c9 c10\nb big c10 c9 d lnk\n", "",
		},
		{`print -r -- [ab](ne:'[[ -o numericglobsort ]] && print on || print off':)`, "off\noff\na b\n", ""},
		{`print -r -- [ab](e:'setopt numericglobsort':n); [[ -o numericglobsort ]] && print on`, "a b\non\n", ""},
		{
			`setopt numericglobsort; print -r -- [ab](e:'unsetopt numericglobsort':^n); [[ -o numericglobsort ]] || print off`,
			"a b\noff\n", "",
		},
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
		// An expansion that fails inside the code is the code's, and the
		// command the pattern is a word of still runs.
		{
			`n=0; print [ab](e:'(( n++ )); x=$(( 1/0 )); true':N); print n=$n`, "\nn=2\n",
			"zsh:1: division by zero\nzsh:1: division by zero\n",
		},
	} {
		out, _, errs := runZshSplit(t, globCodeFixture(t), tc.src)
		if out != tc.out || errs != tc.errs {
			t.Errorf("%q:\n out %q errs %q\nwant %q errs %q", tc.src, out, errs, tc.out, tc.errs)
		}
	}
}

// `oe:code:` and `o+name` sort by what the code leaves in `REPLY` for each
// match (#6691). Rows whose answer is the directory's own order are in
// TestAGlobQualifiersCodeRunsInTheDirectorysOrder, since that order is the
// filesystem's; every row here was measured on zsh 5.9.2 under `-fc`. See
// interp/globqualsort.go.
func TestAGlobSortSpecifierSortsByWhatTheCodeLeaves(t *testing.T) {
	t.Parallel()
	const all = "a b big c10 c9 d lnk\n"
	for _, tc := range []struct{ src, out, errs string }{
		{`print *(oe:'REPLY=${REPLY[-1]}':)`, "c10 c9 a b d big lnk\n", ""},
		{`rev() { REPLY=${REPLY[-1]} }; print *(o+rev)`, "c10 c9 a b d big lnk\n", ""},
		{`print *(Oe:'REPLY=${REPLY[-1]}':)`, "lnk big d b a c9 c10\n", ""},
		{`print *(Oe:'REPLY=${REPLY[-1]}':[1,2])`, "lnk big\n", ""},
		{`print [ab]*(^oe:'REPLY=${REPLY[-1]}':)`, "big b a\n", ""},
		{`print [ab]*(oe:'REPLY=${REPLY[1]}':Oe:'REPLY=${REPLY[-1]}':)`, "a big b\n", ""},
		// The status and `reply` are not read.
		{`print *(oe:'false':)`, all, ""},
		{`print *(oe:'reply=(z $REPLY)':)`, all, ""},
		{`print *(oe:'reply=(${REPLY[-1]})':)`, all, ""},
		// The keys compare as names do, numbers and all.
		{
			`print *(oe:'REPLY=$(( 12 - ${#REPLY} ))x$REPLY':) / *(noe:'REPLY=$(( 12 - ${#REPLY} ))x$REPLY':)`,
			"c9 a b d big c10 lnk / big c10 lnk c9 a b d\n", "",
		},
		// What the code sees: the word after every test, every `e`, the
		// modifiers and markdirs. `reply` is left alone, and REPLY is left
		// at the last key.
		{`print *(oe:'print -r -- $REPLY >&2':/)`, "d\n", "d\n"},
		{`print a(oe:'print -r -- o$REPLY >&2':e:'print -r -- e$REPLY >&2':)`, "a\n", "ea\noa\n"},
		{`print ./a(oe:'print -r -- $REPLY >&2'::t) ./a(oe:'print -r -- $REPLY >&2':)`, "a ./a\n", "a\n./a\n"},
		{`setopt markdirs; print d(oe:'print -r -- $REPLY >&2':)`, "d/\n", "d/\n"},
		{`reply=(q); print a(oe:'print -r -- $reply >&2; reply=(w)':); print $reply`, "a\nw\n", "q\n"},
		{`REPLY=orig; print a(oe:'REPLY=x$REPLY':); print $REPLY`, "a\nxa\n", ""},
		{`print a(oe:'print -r -- $zsh_eval_context >&2':)`, "a\n", "toplevel globsort\n"},
		// One `:` closes the code, so a letter after it is a qualifier:
		// `t` is the sticky bit.
		{`print [ab]*(oe:true:t)`, "", "zsh:1: no matches found: [ab]*(oe:true:t)\n"},
		{`print *(oe)`, "", "zsh:1: missing end of string\n"},
		{`print *(o+)`, "", "zsh:1: missing identifier after `+'\n"},
		// A failure is the code's. One that gives the code up stops it for
		// every later match, which keys on its name — the one that gave up
		// included — while another specifier still runs for all of them.
		{`n=0; print *(oe:'(( n++ )); nosuchcmd':) >/dev/null; print $n`, "7\n", strings.Repeat("zsh:1: command not found: nosuchcmd\n", 7)},
		{`n=0; print *(oe:'(( n++ )); echo [x]':); print $n`, all + "1\n", "zsh:1: no matches found: [x]\n"},
		{`print *(oe:'if':)`, all, "zsh:1: parse error near `if'\n"},
		{
			`print *(oe:'REPLY=zzz; echo [x]':) *(oe:'REPLY=0; echo [x]':)`,
			"a b big c10 c9 d lnk a b big c10 c9 d lnk\n", "zsh:1: no matches found: [x]\nzsh:1: no matches found: [x]\n",
		},
		{`n=0; print *(oe:'(( n++ )); echo [x]':oe:'(( n+=10 ))':) >/dev/null; print $n`, "71\n", "zsh:1: no matches found: [x]\n"},
		{`n=0; print *(oe:'(( n++ )); x=$(( 1/0 ))':) >/dev/null; print $n`, "1\n", "zsh:1: division by zero\n"},
		// `oN` is the directory's order only as the first specifier; later
		// it does nothing, and behind it a specifier's code does not run.
		{`print *(OnoN) / *(oe:'REPLY=${REPLY[-1]}':oN)`, "lnk d c9 c10 big b a / c10 c9 a b d big lnk\n", ""},
		{`print -r -- *(oNoe:'print x >&2':) >/dev/null`, "", ""},
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
	t.Parallel()
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

// A sort specifier's code runs in the directory's own order too, and a key
// that counts the runs sorts the matches into that order; behind a leading
// `oN` nothing is sorted at all.
func TestAGlobSortSpecifiersCodeRunsInTheDirectorysOrder(t *testing.T) {
	t.Parallel()
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
	listing := strings.Join(raw, " ")
	for _, tc := range []struct{ src, out, errs string }{
		{`print -r -- *(oe:'print -rn -- "$REPLY " >&2':[1])`, "a\n", listing + " "},
		{`n=0; print -r -- *(oe:'REPLY=$(( n++ ))':)`, listing + "\n", ""},
		{`print -r -- *(oNOn)`, listing + "\n", ""},
		{`print -r -- *(oNoe:'REPLY=${REPLY[-1]}':)`, listing + "\n", ""},
	} {
		out, _, errs := runZshSplit(t, dir, tc.src)
		if out != tc.out || errs != tc.errs {
			t.Errorf("%q:\n out %q errs %q\nwant %q errs %q", tc.src, out, errs, tc.out, tc.errs)
		}
	}
}

// A `return` with nothing to return from, in a program, ends the shell as the
// code runs and at the code's status: the command the pattern is a word of
// never runs, nor do later files' code (#6692). Inside a function or a sourced
// file it waits for the command instead; see TestAGlobQualifierRunsCodeForEachFile.
// Measured on zsh 5.9.2 by `-c`, `-i -c`, a script file and standard input.
func TestATopLevelReturnInGlobQualifierCodeEndsTheShell(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		src, out string
		status   int
	}{
		{`print X *(e:'echo r; return 3':); print next`, "r\n", 3},
		{`print X *(oe:'echo r; return 3':); print next`, "r\n", 3},
		{`false; print X *(e:'return':); print next`, "", 1},
		{`x=( *(e:'return 3':) ); print next`, "", 3},
		{`print X *(e:'return 3':) && print and; print next`, "", 3},
		{`for i in 1; do print X *(e:'return 3':); done; print next`, "", 3},
		{`{ print X *(e:'return 3':) } always { print alw }; print next`, "", 3},
		// A subshell is a top level of its own.
		{`( print X *(e:'return 3':); print in ); print after $?`, "after 3\n", 0},
		// A sourced file is something to return from: one of the two names
		// passes, the file returns, and the shell goes on.
		{
			`print -r -- 'x=( [ab](e:"return 3":) ); print inx' > s; . ./s; print after $? $#x`,
			"after 0 1\n", 0,
		},
	} {
		out, status, errs := runZshSplit(t, globCodeFixture(t), tc.src)
		if out != tc.out || status != tc.status || errs != "" {
			t.Errorf("%q:\n out %q status %d errs %q\nwant %q status %d", tc.src, out, status, errs, tc.out, tc.status)
		}
	}
}

// A `return`, `break` or `continue` in qualifier code that is a word of an
// array literal or of an assignment prefix takes effect once the construct
// has finished: the literal is stored and the command runs. Measured on zsh
// 5.9.2; both used to be dropped.
func TestControlFlowInGlobQualifierCodeLetsTheConstructFinish(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ src, out string }{
		{`f() { x=( [ab](e:"return 3":) ); print inf; }; f; print after $? $#x`, "after 0 1\n"},
		{`f() { x=( [ab](e:"return 3":) ) print in; print inf; }; f; print after $?`, "in\nafter 0\n"},
		{`for j in 1; do x=( [ab](e:"break":) ); print in; done; print after $#x`, "after 2\n"},
		{`for j in 1 2; do x=( [ab](e:"continue":) ); print in; done; print after $#x`, "after 2\n"},
		{`print -r -- 'x=( [ab](e:"return 3":) ); print inx' > s; . ./s; print after $? $#x`, "after 0 1\n"},
	} {
		out, _, errs := runZshSplit(t, globCodeFixture(t), tc.src)
		if out != tc.out || errs != "" {
			t.Errorf("%q:\n out %q errs %q\nwant %q", tc.src, out, errs, tc.out)
		}
	}
}

// At a prompt a `return` with nothing to return from costs the line and not
// the shell, so in qualifier code it waits for the command, as it does in a
// function. Measured on zsh 5.9.2 through a pseudo-terminal: `print X
// *(e:'return 3':); echo next` writes `X a b c10 c9 d lnk` — every name but
// `big`, the first the directory lists there — and no `next`.
func TestATopLevelReturnInGlobQualifierCodeAtAPromptWaitsForTheCommand(t *testing.T) {
	t.Parallel()
	src := `print X *(e:'return 3':); print next`
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var o, e bytes.Buffer
	sem, diag := zsh.Semantics(), zsh.Diagnostics()
	r := (&interp.Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &diag,
		Dir: globCodeFixture(t), Name: "zsh", Dialect: presetDialect(),
		Interactive: true, AtPrompt: true,
	}).SeedVars(map[string]string{"PATH": "/nonexistent"})
	zsh.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	// The file whose code returned is the first the directory lists, which
	// is the filesystem's choice; every other file passes without its code.
	got := strings.Fields(o.String())
	if len(got) != 7 || got[0] != "X" || e.Len() != 0 || strings.Contains(o.String(), "next") {
		t.Errorf("out %q errs %q, want `X` and six of the seven names, and no `next`", o.String(), e.String())
	}
}

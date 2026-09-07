// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A substring range is this shell's history-modifier syntax as well, so a
// segment beginning with an unquoted letter is a modifier rather than an
// arithmetic offset — and `i` names no modifier. Measured against zsh 5.9.2
// (2026-09-05); the other three take the substring and answer `cd`.
func TestARangeThatNamesAVariableIsRefused(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `x=abcdef; i=2; echo "[${x:i:2}]"; echo after`)
	if !strings.Contains(out, "unrecognized modifier `i'") {
		t.Errorf("got %q, want the modifier named", out)
	}
	if strings.Contains(out, "after") || st == 0 {
		t.Errorf("got %q (status %d), want the command abandoned at a failure", out, st)
	}
}

// The length is the same question, reached by a different call site.
func TestALengthThatNamesAVariableIsRefusedToo(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `x=abcdef; i=2; echo "[${x:2:i}]"; echo after`)
	if !strings.Contains(out, "unrecognized modifier `i'") || st == 0 {
		t.Errorf("got %q (status %d), want the modifier named at a failure", out, st)
	}
}

// The modifiers this dialect performs. The variables are set on purpose: with
// `h` unset the arithmetic reading would answer from offset 0 and the case
// would look like a disagreement about the whole string.
func TestTheModifiersThisShellPerforms(t *testing.T) {
	for _, c := range []struct{ mod, want string }{
		{"h", "/tmp/Dir"},
		{"t", "File.Txt"},
		{"r", "/tmp/Dir/File"},
		{"e", "Txt"},
		{"l", "/tmp/dir/file.txt"},
		{"u", "/TMP/DIR/FILE.TXT"},
		{"h:t", "Dir"},
	} {
		src := `x=/tmp/Dir/File.Txt; h=9; t=9; echo "[${x:` + c.mod + `}]"`
		out, st := runZsh(t, t.TempDir(), src)
		if got, want := strings.TrimSpace(out), "["+c.want+"]"; got != want || st != 0 {
			t.Errorf(":%s = %q (status %d), want %q", c.mod, got, st, want)
		}
	}
}

// An offset and then a modifier, which is what says the two readings are
// segments of one range rather than alternatives.
func TestAModifierMayFollowAnOffset(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `x=/tmp/Dir/File.Txt; t=3; echo "[${x:2:t}]"`)
	if got := strings.TrimSpace(out); got != "[File.Txt]" || st != 0 {
		t.Errorf("got %q (status %d), want %q", got, st, "[File.Txt]")
	}
}

// A segment that does not begin with a letter is a range here as it is
// everywhere else — an underscore, a leading space, a parenthesis, an
// expansion that already happened, and a quoted letter.
func TestARangeThatDoesNotBeginWithALetterIsAnOffset(t *testing.T) {
	for _, src := range []string{
		`x=abcdef; _q=1; echo "[${x:_q:2}]"`,
		`x=abcdef; i=1; echo "[${x: i:2}]"`,
		`x=abcdef; i=1; echo "[${x:(i):2}]"`,
		`x=abcdef; i=1; echo "[${x:$i:2}]"`,
		`x=abcdef; h=1; echo "[${x:"h":2}]"`,
	} {
		out, st := runZsh(t, t.TempDir(), src)
		if got := strings.TrimSpace(out); got != "[bc]" || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", src, got, st, "[bc]")
		}
	}
}

// A good modifier with something after it in the same segment is refused with
// nothing named, where an unknown letter is named. Two shapes of one sentence.
func TestALeftoverAfterAModifierIsRefusedWithoutAName(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `x=/tmp/a.b; echo "[${x:ha}]"; echo after`)
	if !strings.Contains(out, "unrecognized modifier") || strings.Contains(out, "`") {
		t.Errorf("got %q, want the complaint with nothing named", out)
	}
	if strings.Contains(out, "after") || st == 0 {
		t.Errorf("got %q (status %d), want the command abandoned at a failure", out, st)
	}
}

// All thirteen are performed now, and this is the half of them that needs
// something a string does not carry. Measured against zsh 5.9.2, 2026-09-07,
// with a scratch HOME and no startup files.
//
// The values are chosen so no machine can disagree: an absolute path needs no
// working directory, nothing exists under `/no/such`, and a name holding a
// slash is never searched for. The cases that *do* need the disk get a
// directory from the framework and compare against a path built from it.
func TestTheModifiersThatNeedTheWorldArePerformed(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a", `x=/x/y/../z/.; echo "[${x:a}]"`, "[/x/z]\n"},
		{"a leaves an empty value empty", `x=; echo "[${x:a}]"`, "[]\n"},
		{"A on a path that is nowhere", `x=/no/such/path; echo "[${x:A}]"`, "[/no/such/path]\n"},
		{"A drops a trailing slash", `x=/no/such/; echo "[${x:A}]"`, "[/no/such]\n"},
		{"P keeps one it could not resolve", `x=/no/such/; echo "[${x:P}]"`, "[/no/such/]\n"},
		{"c leaves what it cannot find", `x=nosuchcommand12345; echo "[${x:c}]"`, "[nosuchcommand12345]\n"},
		{"c never touches a name with a slash", `x=./nosuch; echo "[${x:c}]"`, "[./nosuch]\n"},
		{"q", `x="a b*c"; echo "[${x:q}]"`, "[a\\ b\\*c]\n"},
		{"q on empty is empty, where the flag is two quotes", "x=; echo \"[${x:q}][${(q)x}]\"", "[]['']\n"},
		{"Q", "x=\"'a b'\"; echo \"[${x:Q}]\"", "[a b]\n"},
		{"q and Q round trip", `x="a b"; echo "[${x:q:Q}]"`, "[a b]\n"},
		{"s", `x=aXbXc; echo "[${x:s/X/-/}]"`, "[a-bXc]\n"},
		{"gs", `x=aXbXc; echo "[${x:gs/X/-/}]"`, "[a-b-c]\n"},
		{"s with a colon delimiter", `x=aXbXc; echo "[${x:s:X:-:}]"`, "[a-bXc]\n"},
		{"s puts the matched text where & is", `x=aXbXc; echo "[${x:s/X/[&]/}]"`, "[a[X]bXc]\n"},
		{"s takes a literal, not a pattern", `x=abc; echo "[${x:s/?/Z/}]"`, "[abc]\n"},
		{"and replaces it where it is really there", `x="a?c"; echo "[${x:s/?/Z/}]"`, "[aZc]\n"},
		{"a chain after a substitution", `x=/tmp/Dir/f; echo "[${x:s/Dir/X/:h:t}]"`, "[X]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s\ngot  %q (status %d)\nwant %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// `:a` puts a relative value on the working directory, and `:A` resolves the
// link the two of them are given. The directory comes from the framework, so
// the expectation is built rather than written down.
func TestAModifierResolvesAgainstTheDiskItIsGiven(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "real", "sub"), 0o700); err != nil {
		t.Fatalf("making the tree: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "link")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "nowhere"), filepath.Join(dir, "dangling")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	// The *physical* directory, which is what that shell puts a relative
	// value on — and on this platform it is not the one the framework handed
	// over, since a temporary directory sits under a link. Asserting the
	// whole path rather than its ends is the point: "absolute and ending in
	// rel/f" is equally true of the logical answer, so it would grade
	// nothing.
	physical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("resolving the directory: %v", err)
	}
	out, st := runZsh(t, dir, `x=rel/f; echo "[${x:a}]"
y=`+filepath.Join(dir, "link", "sub")+`; echo "[${y:A}]"
z=`+filepath.Join(dir, "link", "sub")+`; echo "[${z:a}]"
w=`+filepath.Join(dir, "dangling")+`; echo "[${w:A}]"`)
	if st != 0 {
		t.Fatalf("status %d, out %q", st, out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %q, want four lines", out)
	}
	if want := "[" + filepath.Join(physical, "rel", "f") + "]"; lines[0] != want {
		t.Errorf(":a on a relative path = %s, want %s", lines[0], want)
	}
	// `:A` follows the link; `:a` does not, and the pair is the whole
	// difference between them.
	if want := "[" + filepath.Join(physical, "real", "sub") + "]"; lines[1] != want {
		t.Errorf(":A = %s, want %s", lines[1], want)
	}
	if !strings.HasSuffix(lines[2], "/link/sub]") {
		t.Errorf(":a = %s, want the link left alone", lines[2])
	}
	// A link whose target is not there is its own answer, not the name it
	// points at. The walk has to stop *at* it, which is the one thing the
	// resolver cannot do by following links until one fails.
	if want := "[" + filepath.Join(physical, "dangling") + "]"; lines[3] != want {
		t.Errorf(":A on a dangling link = %s, want %s", lines[3], want)
	}
}

// `:A` cancels `..` by name and `:P` applies it to what has already been
// resolved. That is the primary difference between them, and a link to a
// *sub*directory is the only shape that shows it: `link2/..` is the link's
// parent by name and the parent of what it points at on the disk.
func TestResolvedAndRealPathPartOverDotDot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "real", "sub"), 0o700); err != nil {
		t.Fatalf("making the tree: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "real", "sub"), filepath.Join(dir, "link2")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "real"), filepath.Join(dir, "link")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	physical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("resolving the directory: %v", err)
	}
	want := "[" + physical + "]" +
		"[" + filepath.Join(physical, "real") + "]" +
		// And a path only *partly* on the disk: the prefix that exists is
		// resolved and the rest is appended as written, rather than the whole
		// thing being handed back untouched.
		"[" + filepath.Join(physical, "real", "no", "such") + "]\n"
	// Written rather than joined: filepath.Join *cleans*, so it would cancel
	// the `..` here and hand the shell a path with nothing left to disagree
	// about — which is the very thing being measured.
	out, st := runZsh(t, dir, `x=`+dir+`/link2/..
y=`+dir+`/link/no/such
echo "[${x:A}][${x:P}][${y:A}]"`)
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// An empty PATH finds nothing, where this shell's command *lookup* runs a
// `mycmd` sitting in the current directory. Measured both ways in the same
// shell, which is what makes it a rule about `:c` rather than about PATH.
func TestACommandModifierWithAnEmptyPathFindsNothing(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mycmd"), []byte("#!/bin/sh\necho ran\n"), 0o700); err != nil {
		t.Fatalf("writing the command: %v", err)
	}
	out, st := runZsh(t, dir, `x=mycmd
PATH=
echo "[${x:c}]"`)
	if out != "[mycmd]\n" || st != 0 {
		t.Errorf("`:c` with an empty PATH = %q (status %d), want the name unchanged", out, st)
	}
}

// `:c` finds a command on PATH and leaves alone everything that is not one.
// A function is not a command here and a builtin resolves to the external
// file of that name — both measured, and both the opposite of what a reader
// would guess from the name.
func TestACommandModifierSearchesPathAndNothingElse(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mycmd"), []byte("#!/bin/sh\n:\n"), 0o700); err != nil {
		t.Fatalf("writing the command: %v", err)
	}
	want := "[" + filepath.Join(dir, "mycmd") + "]\n[myfn]\n[nosuchcommand12345]\n"
	out, st := runZsh(t, dir, `x=mycmd; echo "[${x:c}]"
myfn(){ :; }; y=myfn; echo "[${y:c}]"
z=nosuchcommand12345; echo "[${z:c}]"`)
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// A count after `h` or `t` counts separators — from the left and from the
// right — rather than repeating the modifier, which is what it looks like.
func TestACountCountsSeparators(t *testing.T) {
	want := "[/][/a][/a/b][/a/b/c/d/e][d/e][a/b/c/d/e][/a/b/c/d/e]\n"
	out, st := runZsh(t, t.TempDir(),
		`x=/a/b/c/d/e; echo "[${x:h1}][${x:h2}][${x:h3}][${x:h9}][${x:t2}][${x:t5}][${x:t9}]"`)
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
	// A trailing run of slashes separates nothing, so this path has two
	// separators and `:h3` is the whole value.
	want = "[/a][/a/b//]\n"
	if out, st := runZsh(t, t.TempDir(), `x=/a/b//; echo "[${x:h2}][${x:h3}]"`); out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// The substitution is remembered for the shell, not for the parameter.
func TestASubstitutionIsRememberedAcrossParameters(t *testing.T) {
	got, status := runZsh(t, t.TempDir(),
		`x=aXbXc; y=aXd; z=aXbXc; echo "[${x:s/X/-/}][${y:s//+/}][${z:&}]"`)
	// The third is `a+bXc` and not `a-bXc`, which is the sharp end of the
	// memory being one thing rather than a stack: `${y:s//+/}` did not only
	// *read* the remembered pattern, it wrote the new replacement back — so
	// `:&` afterwards repeats `X → +` and not the `X → -` two expansions
	// earlier. Measured against zsh 5.9.2, and the expectation here was wrong
	// before the shell was asked.
	if got != "[a-bXc][a+d][a+bXc]\n" || status != 0 {
		t.Errorf("got %q (status %d), want %q", got, status, "[a-bXc][a+d][a+bXc]\n")
	}
	// `:&` with nothing before it is a silent no-op, where an empty pattern
	// with nothing before it is refused by name. The two reach for the same
	// memory and answer differently when it is empty.
	if got, status := runZsh(t, t.TempDir(), `x=aXbXc; echo "[${x:&}]"`); got != "[aXbXc]\n" || status != 0 {
		t.Errorf("`:&` with no previous substitution = %q (status %d), want a silent no-op", got, status)
	}
	got, status = runZsh(t, t.TempDir(), `x=aXbXc; echo "[${x:s//+/}]"`)
	if !strings.Contains(got, "no previous substitution") || status == 0 {
		t.Errorf("an empty pattern with none before it = %q (status %d), want a refusal", got, status)
	}
}

// The refusal names one byte. `${x:zz}` is a complaint about `z`; naming `zz`
// would say the pair is the modifier that is missing.
func TestAnUnrecognizedModifierNamesOneByte(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`x=/tmp/a.b; echo "[${x:zz}]"`, "zsh:1: unrecognized modifier `z'\n"},
		{`x=/tmp/a.b; echo "[${x:iq}]"`, "zsh:1: unrecognized modifier `i'\n"},
		{`x=/tmp/a.b; echo "[${x:g}]"`, "zsh:1: unrecognized modifier `g'\n"},
		{`x=/tmp/a.b; echo "[${x:hzz}]"`, "zsh:1: unrecognized modifier\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), tc.src); out != tc.want || st == 0 {
			t.Errorf("%s\ngot  %q (status %d)\nwant %q", tc.src, out, st, tc.want)
		}
	}
	// `${x:s}` is not a modifier complaint at all — a substitution with no
	// body is a bad substitution, the same as any other malformed `${ }`.
	if out, st := runZsh(t, t.TempDir(), `x=abc; echo "[${x:s}]"`); !strings.Contains(out, "bad substitution") || st == 0 {
		t.Errorf("`${x:s}` = %q (status %d), want a bad substitution", out, st)
	}
}

// A modifier after **both** an offset and a length, which the parser cannot
// split on its own: a range is split once, so `5:t` arrived whole and reached
// the evaluator as an expression.
func TestAModifierMayFollowBothAnOffsetAndALength(t *testing.T) {
	want := "[D][BCDEF][File.Txt.gz][mp]\n"
	out, st := runZsh(t, t.TempDir(),
		`x=/tmp/Dir/File.Txt.gz; y=abcdefgh; echo "[${x:1:5:t}][${y:1:5:u}][${x:2:t}][${x:2:2}]"`)
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

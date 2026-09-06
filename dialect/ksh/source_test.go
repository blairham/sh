// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

func runKsh(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": dir},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// TestApplyAddsSourceAndStillRemovesLocal covers both halves of Apply, because
// the addition arrived later and the removal is easy to lose to it.
func TestApplyAddsSourceAndStillRemovesLocal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(path, []byte("echo via-source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := runKsh(t, dir, `source `+path)
	if st != 0 || strings.TrimSpace(out) != "via-source" {
		t.Errorf("`source` should work: %q status %d", out, st)
	}

	out, st = runKsh(t, dir, `f() { local v=in; }; f`)
	if !strings.Contains(out, "local: not found") {
		t.Errorf("ksh93 still has no `local`: %q", out)
	}
	if st != 127 {
		t.Errorf("status = %d, want 127", st)
	}
}

// TestTheTwoHalvesOfTheFatalRuleDisagree is why one axis was not enough.
//
// POSIX makes any special builtin's failure fatal. ksh93 kept half of it: a
// file `.` cannot open ends the script, and unparseable text handed to `eval`
// does not. dash kept both halves, bash and zsh neither.
func TestTheTwoHalvesOfTheFatalRuleDisagree(t *testing.T) {
	s := ksh.Semantics()
	if got := s.BuiltinSyntaxErrorFatal; got != interp.No {
		t.Errorf("BuiltinSyntaxErrorFatal = %v, want No", got)
	}
	if got := s.DotMissingFileFatal; got != interp.Yes {
		t.Errorf("DotMissingFileFatal = %v, want Yes", got)
	}

	dir := t.TempDir()
	// `$?` is read immediately, because the trailing echo succeeds and would
	// otherwise be the status this asserts on — the script exits 0 here in the
	// real shell too, and the 3 belongs to the eval.
	out, _ := runKsh(t, dir, `eval "if"; echo REACHED st=$?`)
	if !strings.Contains(out, "REACHED") {
		t.Errorf("an unparseable eval is survivable here: %q", out)
	}
	if !strings.Contains(out, "st=3") {
		t.Errorf("output = %q, want st=3 — ksh93's syntax status", out)
	}

	out, st := runKsh(t, dir, `. `+filepath.Join(dir, "absent.sh")+`; echo NOT-REACHED`)
	if strings.Contains(out, "NOT-REACHED") {
		t.Errorf("a file `.` cannot open ends the script here: %q", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// TestNoOperandIsFatalWithItsOwnStatus is the case that caught a bug: the
// generic fatal status for ksh93 is 1, and this failure reports 2. Taking the
// status from the fatal path rather than from the field was wrong by one.
func TestNoOperandIsFatalWithItsOwnStatus(t *testing.T) {
	if got := ksh.Diagnostics().DotNoOperandStatus; got != 2 {
		t.Fatalf("DotNoOperandStatus = %d, want 2", got)
	}
	out, st := runKsh(t, t.TempDir(), `. ; echo NOT-REACHED`)
	if strings.Contains(out, "NOT-REACHED") {
		t.Errorf("`.` with no operand ends the script here: %q", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2 — not the generic fatal status of 1", st)
	}
}

func TestDotPassesArgumentsHere(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.sh")
	if err := os.WriteFile(path, []byte("echo got=$1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runKsh(t, dir, `set -- OUTER; . `+path+` INNER; echo after=$1`)
	if got := strings.TrimSpace(out); got != "got=INNER\nafter=OUTER" {
		t.Errorf("output = %q, want the file to see INNER and the caller OUTER", got)
	}
}

// TestExecAxes records what ksh93 does about `exec`.
func TestExecAxes(t *testing.T) {
	s := ksh.Semantics()
	if got := s.ExecFailureRunsExitTrap; got != interp.No {
		t.Errorf("ExecFailureRunsExitTrap = %v, want No", got)
	}
	if got := s.ExecTakesOptions; got != interp.Yes {
		t.Errorf("ExecTakesOptions = %v, want Yes", got)
	}
	out, st := runKsh(t, t.TempDir(), `trap "echo TRAP" EXIT; exec nosuchcmd-xyz`)
	if strings.Contains(out, "TRAP") {
		t.Errorf("ksh93 drops the EXIT trap after a failed exec: %q", out)
	}
	if st != 127 {
		t.Errorf("status = %d, want 127", st)
	}
}

// TestDoubleEqualInTest: ksh93 takes `==` as a second spelling of `=`.
func TestDoubleEqualInTest(t *testing.T) {
	dir := t.TempDir()
	if _, st := runKsh(t, dir, `test a == a`); st != 0 {
		t.Errorf("equal operands: status %d, want 0", st)
	}
	if _, st := runKsh(t, dir, `test a == b`); st != 1 {
		t.Errorf("unequal operands: status %d, want 1", st)
	}
	// The single `=` is unanimous, and pins the difference to the operator
	// rather than to anything else about how the words are read.
	if _, st := runKsh(t, dir, `test a = a`); st != 0 {
		t.Errorf("single equals: status %d, want 0", st)
	}
}

// TestABracketNamesItself: `[` is `test` under another name, and the
// diagnostic blames the name that was typed. Run rather than asserted against
// the wording string, since the wording is what would be wrong.
func TestABracketNamesItself(t *testing.T) {
	dir := t.TempDir()
	if out, _ := runKsh(t, dir, `test a b c`); !strings.Contains(out, `test: b: unknown operator`) {
		t.Errorf("test: said %q, want %q", out, `test: b: unknown operator`)
	}
	if out, _ := runKsh(t, dir, `[ a b c ]`); !strings.Contains(out, `[: b: unknown operator`) {
		t.Errorf("bracket: said %q, want %q", out, `[: b: unknown operator`)
	}
	// The unary wording is a separate string and carries the name too.
	if out, _ := runKsh(t, dir, `[ -Q x ]`); !strings.Contains(out, `[: -Q: unknown operator`) {
		t.Errorf("unary bracket: said %q, want %q", out, `[: -Q: unknown operator`)
	}
}

// TestAKilledCommandsStatus: ksh93 counts from 256, so 256 + 13 — measured across eight signals, not a special case for one.
func TestAKilledCommandsStatus(t *testing.T) {
	dir := t.TempDir()
	// PATH is the temp directory alone, so the command that dies has to be
	// made here rather than borrowed from the host.
	exe := filepath.Join(dir, "selfkill")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nkill -PIPE $$\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _ := runKsh(t, dir, "selfkill\necho st=$?\n")
	if !strings.Contains(out, "st=269") {
		t.Errorf("said %q, want st=269", out)
	}
}

// TestPipefail: ksh93 has the option.
func TestPipefail(t *testing.T) {
	dir := t.TempDir()
	out, _ := runKsh(t, dir, "set -o pipefail\n(exit 3) | (exit 4) | true\necho st=$?\n")
	if !strings.Contains(out, "st=4") {
		t.Errorf("said %q, want st=4", out)
	}
}

// TestARedirectFailure: this dialect's own wording and status for a redirect
// that could not be opened, and for one that could not be created.
//
// The whole line, location included, rather than a substring or a suffix of
// it. A shape that gained a verb in *front* — `cannot open f: …` where this
// dialect says `f: …` — still ends with the shape that did not, so neither
// Contains nor HasSuffix can tell the two apart.
func TestARedirectFailure(t *testing.T) {
	dir := t.TempDir()
	out, _ := runKsh(t, dir, "cat < nope\necho st=$?\n")
	if got := strings.SplitN(strings.TrimSpace(out), "\n", 2)[0]; got != `ksh: nope: cannot open [No such file or directory]` {
		t.Errorf("read: said %q, want %q", got, `ksh: nope: cannot open [No such file or directory]`)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want st=1", out)
	}
	// The other direction, which two of the four word differently.
	out, _ = runKsh(t, dir, "echo x > nodir/out\n")
	if got := strings.TrimSpace(out); got != `ksh: nodir/out: cannot create [No such file or directory]` {
		t.Errorf("create: said %q, want %q", got, `ksh: nodir/out: cannot create [No such file or directory]`)
	}
}

// TestSetOInABundle: `-o` as the last letter of a bundle, which is how every
// real script writes it.
func TestSetOInABundle(t *testing.T) {
	dir := t.TempDir()
	out, _ := runKsh(t, dir, "set -uo noglob\necho ok=$?\necho *\n")
	if !strings.Contains(out, "ok=0") || !strings.Contains(out, "*") {
		t.Errorf("said %q, want the bundle's letters and its named option", out)
	}
}

// TestErrexitAndPipefail: whether `set -e` stops for a failure only pipefail
// produced. An ordinary failing pipeline stops every shell in the panel, so
// this is about the failure the option adds and not about pipelines.
func TestErrexitAndPipefail(t *testing.T) {
	dir := t.TempDir()
	out, _ := runKsh(t, dir, "set -eo pipefail\nfalse | true\necho reached\n")
	if got := strings.Contains(out, "reached"); got != true {
		t.Errorf("said %q, want reached=true", out)
	}
	// Unanimous, and must not move with the answer above.
	out, _ = runKsh(t, dir, "set -eo pipefail\ntrue | false\necho unreached\n")
	if strings.Contains(out, "unreached") {
		t.Errorf("ordinary failure: said %q, want it to stop", out)
	}
}

// TestPrintfOptions: this dialect's answers about `printf`'s options, run
// rather than asserted against the fields — the wordings and the usage line
// are what would be wrong.
// ksh93 takes the same set bash does, and names the conversion alone rather
// than the rest of the format after it.
func TestPrintfLengthModifiers(t *testing.T) {
	dir := t.TempDir()
	out, _ := runKsh(t, dir, "printf \"[%ld][%zX][%jd][%lld][%hhd]\\n\" 42 255 42 42 300\n")
	if want := "[42][FF][42][42][300]"; !strings.Contains(out, want) {
		t.Errorf("said %q, want %q", out, want)
	}
	if out, _ := runKsh(t, dir, "printf \"%v]xY\" 1\n"); !strings.Contains(out, "printf: v: unknown format specifier") {
		t.Errorf("said %q, want the conversion character alone", out)
	}
}

func TestPrintfOptions(t *testing.T) {
	dir := t.TempDir()
	out, _ := runKsh(t, dir, "printf -v o \"%05d\" 42\necho \"[$o]\"\n")
	if !strings.Contains(out, `printf: -v: unknown option`) {
		t.Errorf("-v: said %q, want %q", out, `printf: -v: unknown option`)
	}
	// `--` ends the options everywhere.
	if out, _ := runKsh(t, dir, "printf -- \"x\n\"\n"); strings.TrimSpace(out) != "x" {
		t.Errorf("--: said %q, want x", out)
	}
	out, _ = runKsh(t, dir, "printf -q x\n")
	if !strings.Contains(out, `printf: -q: unknown option`) {
		t.Errorf("unknown option: said %q, want %q", out, `printf: -q: unknown option`)
	}
	// Whether the complaint is followed by a usage line.
	if got := strings.Contains(out, `Usage`) || strings.Contains(out, `usage`); got != true {
		t.Errorf("usage line present=%v, want true (said %q)", got, out)
	}
}

// TestUmask: this dialect's answers about `umask`, run through a hook that
// keeps the mask in a variable — nothing here touches the machine's own.
func TestUmask(t *testing.T) {
	run := func(src string) (string, int) {
		t.Helper()
		f, err := syntax.Parse(src, ksh.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		sem, dg := ksh.Semantics(), ksh.Diagnostics()
		held := 0o022
		r := &interp.Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "ksh", Dialect: presetDialect()}
		r.SetUmask = func(mask int) (int, error) { old := held; held = mask; return old, nil }
		ksh.Apply(r)
		st, rerr := r.Run(context.Background(), f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		return buf.String(), st
	}
	if out, _ := run("umask"); strings.TrimSpace(out) != "0022" {
		t.Errorf("read: said %q, want %q", out, "0022")
	}
	// Whether setting with -S echoes the new mask.
	out, _ := run("umask -S 077")
	if got := strings.TrimSpace(out); got != "" {
		t.Errorf("-S with a mask: said %q, want %q", got, "")
	}
}

// TestLet: this dialect has `let`, and words an empty one its own way.
func TestLet(t *testing.T) {
	dir := t.TempDir()
	if out, _ := runKsh(t, dir, "let \"x = 2 + 3\"\necho $x\n"); strings.TrimSpace(out) != "5" {
		t.Errorf("said %q, want 5", out)
	}
	// Zero is a failure, which is the half a caller has to know about.
	if _, st := runKsh(t, dir, "let \"x=0\"\n"); st != 1 {
		t.Errorf("zero: status %d, want 1", st)
	}
	if _, st := runKsh(t, dir, "let \"x=5\"\n"); st != 0 {
		t.Errorf("nonzero: status %d, want 0", st)
	}
	out, st := runKsh(t, dir, "let\n")
	if st != 2 {
		t.Errorf("empty: status %d, want 2", st)
	}
	// Whether the complaint carries this shell's name and a location.
	if got := strings.HasPrefix(strings.TrimSpace(out), "ksh"); got != false {
		t.Errorf("empty: said %q, want prefixed=false", out)
	}
}

// TestUlimit: this dialect's answers about `ulimit`, through hooks that keep
// the limits in a map — nothing here touches the process's own.
func TestUlimit(t *testing.T) {
	held := map[interp.Resource][2]int64{
		interp.ResourceFileSize: {2048, interp.RlimitInfinity},
		interp.ResourceCPUTime:  {100, interp.RlimitInfinity},
	}
	run := func(src string) (string, int) {
		t.Helper()
		f, err := syntax.Parse(src, ksh.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		sem, dg := ksh.Semantics(), ksh.Diagnostics()
		r := &interp.Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "ksh", Dialect: presetDialect()}
		r.GetRlimit = func(res interp.Resource) (int64, int64, error) { p := held[res]; return p[0], p[1], nil }
		r.SetRlimit = func(res interp.Resource, soft, hard int64) error { held[res] = [2]int64{soft, hard}; return nil }
		ksh.Apply(r)
		st, rerr := r.Run(context.Background(), f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		return buf.String(), st
	}
	// 2048 bytes is 4 blocks of 512 and 2 of 1024.
	if out, _ := run("ulimit -f"); strings.TrimSpace(out) != "4" {
		t.Errorf("block size: said %q, want 4", out)
	}
	// The two letters that are not universal.
	if _, st := run("ulimit -m"); st != 0 {
		t.Errorf("-m: status %d, want ksh93 to have it", st)
	}
	if _, st := run("ulimit -u"); st != 0 {
		t.Errorf("-u: status %d, want present=yes", st)
	}
	// Whether setting lowers the hard limit with the soft one.
	run("ulimit -t 50")
	if got := held[interp.ResourceCPUTime]; got[1] != 50 {
		t.Errorf("hard limit is %d, want both", got[1])
	}
}

// TestABadBuiltinOption: this dialect's wording, status, usage line and
// whether a special builtin's bad option ends the script.
func TestABadBuiltinOption(t *testing.T) {
	dir := t.TempDir()
	out, _ := runKsh(t, dir, "export -Q x\necho \"st=$?\"\necho after\n")
	if !strings.Contains(out, "-Q") {
		t.Errorf("said %q, want the option named", out)
	}
	if reached := strings.Contains(out, "after"); reached != false {
		t.Errorf("said %q, want reached=false", out)
	}
	if got := strings.Contains(strings.ToLower(out), "usage"); got != true {
		t.Errorf("said %q, want usage=true", out)
	}
}

// A format that ends before its conversion character is not an error here:
// the whole unfinished conversion becomes one literal percent and the
// command succeeds.
func TestPrintfUnfinishedConversionIsALiteralPercent(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`printf 'a%'`, "a%"},
		{`printf 'a%5'`, "a%"},
		{`printf 'a%ll'`, "a%"},
		{`printf 'a%%b%'`, "a%b%"},
	} {
		out, st := runKsh(t, dir, tc.src+"\n")
		if out != tc.want || st != 0 {
			t.Errorf("%s: said %q status %d, want %q and 0", tc.src, out, st, tc.want)
		}
	}
}

// `\x` reads every digit that follows, and more than two of them make the
// value a code point rather than a byte.
func TestPrintfHexEscapeReadsACodePoint(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`printf 'a\x41Z'`, "aAZ"},
		{`printf 'a\xffZ'`, "a\xffZ"},
		{`printf '[\x0ff]'`, "[\u00ff]"},
		{`printf 'a\xZ'`, "a\x00Z"},
	} {
		if out, _ := runKsh(t, dir, tc.src+"\n"); out != tc.want {
			t.Errorf("%s: said % x, want % x", tc.src, out, tc.want)
		}
	}
}

// The `%b` escape table this shell answers for, and it is the reason every
// one of these is a per-site axis: the format reads `\x41` as an `A` and the
// `%b` writes the four characters, the format reads `\cb` as control-B and
// the `%b` stops, and `\E` is the escape character where `\e` is two
// ordinary ones — the opposite of zsh.
func TestPrintfBEscapesAreKshs(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`printf '%b' 'a\0101Z'`, "aAZ"},
		{`printf '%b' 'a\0300Z'`, "a\xc0Z"},
		{`printf '%b' 'a\101Z'`, `a\101Z`},
		{`printf '%b' 'a\eZ:a\EZ'`, "a\\eZ:a\x1bZ"},
		{`printf '%b' 'a\x41Z'`, `a\x41Z`},
		{`printf '%b' 'a\cbZ'`, "a"},
		{`printf 'a\0101Z'`, "a\b1Z"},
		{`printf 'a\x41Z'`, "aAZ"},
		{`printf 'a\cbZ'`, "a\x02Z"},
	} {
		if out, st := runKsh(t, dir, tc.src+"\n"); out != tc.want || st != 0 {
			t.Errorf("%s: said % x status %d, want % x and 0", tc.src, out, st, tc.want)
		}
	}
}

// `shift` here reads *every* dash word as an option, digits and all, which is
// what makes the marker load-bearing: `-1` is an option this shell does not
// have and `-- -1` is a count below zero, worded the same way a count above
// `$#` is. Both end the script, as a failed special builtin does here.
func TestShiftReadsEveryDashWordAsAnOption(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src  string
		want string
		st   int
	}{
		{`set -- a b c; shift -- 2; echo "st=$? rest=[$*]"`, "st=0 rest=[c]\n", 0},
		{`set -- a b c; shift --; echo "st=$? rest=[$*]"`, "st=0 rest=[b c]\n", 0},
		{`set -- a b c; shift -1; echo "st=$? n=$#"`, "ksh: shift: -1: unknown option\nUsage: shift [ options ] [n]\n", 2},
		{`set -- a b c; shift -0; echo "st=$? n=$#"`, "ksh: shift: -0: unknown option\nUsage: shift [ options ] [n]\n", 2},
		{`set -- a b c; shift -- -1; echo "st=$? n=$#"`, "ksh: shift: -1: bad number\n", 1},
		{`set -- a b c; shift +1; echo "st=$? rest=[$*]"`, "st=0 rest=[b c]\n", 0},
	} {
		if out, st := runKsh(t, dir, tc.src+"\n"); out != tc.want || st != tc.st {
			t.Errorf("%s: said %q status %d, want %q and %d", tc.src, out, st, tc.want, tc.st)
		}
	}
}

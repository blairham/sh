// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What dash does about `eval` and `.`. It is the POSIX-faithful member of the
// panel and the odd one out on almost every question here, which is why the
// substrate's preset follows it and the other three override.

func runDash(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": dir},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// TestDashHasNoSource is the reason `source` is a dialect's answer rather than
// a substrate builtin. If the core offered it, dash would have no way to say
// no — and `command -v source` in the real binary reports "not found".
func TestDashHasNoSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(path, []byte("echo via-source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := runDash(t, dir, `source `+path)
	if strings.Contains(out, "via-source") {
		t.Errorf("dash has no `source`: %q", out)
	}
	if st != 127 {
		t.Errorf("status = %d, want 127 for a command that is not there", st)
	}
	// `.` is POSIX and works.
	out, st = runDash(t, dir, `. `+path)
	if st != 0 || strings.TrimSpace(out) != "via-source" {
		t.Errorf("`.` should work: %q status %d", out, st)
	}
}

// TestAnUnparseableEvalIsFatal is dash alone in the panel, and it is the POSIX
// rule the other three abandoned: a special builtin's failure ends a
// non-interactive shell.
func TestAnUnparseableEvalIsFatal(t *testing.T) {
	out, st := runDash(t, t.TempDir(), `eval "if"; echo NOT-REACHED`)
	if strings.Contains(out, "NOT-REACHED") {
		t.Errorf("dash ends the script on an unparseable eval: %q", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
}

// TestDotWithNoOperandIsNotAnError is dash's oddest answer here: the other
// three complain, and dash does nothing and reports success.
func TestDotWithNoOperandIsNotAnError(t *testing.T) {
	if got := dash.Semantics().DotWithNoOperandIsAnError; got != interp.No {
		t.Fatalf("DotWithNoOperandIsAnError = %v, want No", got)
	}
	out, st := runDash(t, t.TempDir(), `. ; echo st=$?`)
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	if strings.TrimSpace(out) != "st=0" {
		t.Errorf("output = %q, want st=0 and no complaint", out)
	}
}

// TestDotIgnoresArguments is the other dash-only answer: a sourced file still
// sees the caller's positional parameters.
func TestDotIgnoresArguments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.sh")
	if err := os.WriteFile(path, []byte("echo got=$1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runDash(t, dir, `set -- OUTER; . `+path+` INNER`)
	if strings.TrimSpace(out) != "got=OUTER" {
		t.Errorf("output = %q, want got=OUTER: dash ignores the words after the file", out)
	}
}

// TestDotHasTwoMessages is why DotNotFound exists. dash says "not found" for a
// bare name PATH did not have and "cannot open …" for a path, where the other
// three use one message for both.
func TestDotHasTwoMessages(t *testing.T) {
	d := dash.Diagnostics()
	if d.DotNotFound == "" {
		t.Fatal("dash needs a separate not-found wording")
	}

	dir := t.TempDir()
	out, _ := runDash(t, dir, `. nosuchname.sh`)
	if !strings.Contains(out, "not found") {
		t.Errorf("a bare name off PATH: %q, want \"not found\"", out)
	}

	out, _ = runDash(t, dir, `. `+filepath.Join(dir, "absent.sh"))
	if !strings.Contains(out, "cannot open") {
		t.Errorf("a path that will not open: %q, want \"cannot open\"", out)
	}
	// And dash truncates strerror for that one, which is spelled out in the
	// format rather than taken from the error.
	if strings.Contains(out, "No such file or directory") {
		t.Errorf("output = %q: dash says \"No such file\" here, not the full strerror", out)
	}
}

// TestExecAxes records what dash does about `exec`, where it keeps the POSIX
// answer on one axis and is the odd one out on the other.
func TestExecAxes(t *testing.T) {
	s := dash.Semantics()
	// With bash, and against ksh93 and zsh.
	if got := s.ExecFailureRunsExitTrap; got != interp.Yes {
		t.Errorf("ExecFailureRunsExitTrap = %v, want Yes", got)
	}
	// Alone: `exec -a name cmd` is a command called "-a" here.
	if got := s.ExecTakesOptions; got != interp.No {
		t.Errorf("ExecTakesOptions = %v, want No", got)
	}
	out, _ := runDash(t, t.TempDir(), `exec -a myname echo hi`)
	if strings.Contains(out, "hi") {
		t.Errorf("dash does not read options here: %q", out)
	}
	if !strings.Contains(out, "-a") {
		t.Errorf("output = %q, want -a reported as the command", out)
	}

	// dash hands a directory to execve rather than checking first.
	if dash.Diagnostics().DirectoryReason == "" {
		t.Error("dash reports execve's own error for a directory")
	}
}

// TestDoubleEqualInTest: dash has only `=`, so `==` is not an operator and the three words are a malformed expression — 2 for both pairs of operands, where the others answer 0 and 1.
func TestDoubleEqualInTest(t *testing.T) {
	dir := t.TempDir()
	if _, st := runDash(t, dir, `test a == a`); st != 2 {
		t.Errorf("equal operands: status %d, want 2", st)
	}
	if _, st := runDash(t, dir, `test a == b`); st != 2 {
		t.Errorf("unequal operands: status %d, want 2", st)
	}
	// The single `=` is unanimous, and pins the difference to the operator
	// rather than to anything else about how the words are read.
	if _, st := runDash(t, dir, `test a = a`); st != 0 {
		t.Errorf("single equals: status %d, want 0", st)
	}
}

// TestABracketNamesItself: `[` is `test` under another name, and the
// diagnostic blames the name that was typed. Run rather than asserted against
// the wording string, since the wording is what would be wrong.
func TestABracketNamesItself(t *testing.T) {
	dir := t.TempDir()
	if out, _ := runDash(t, dir, `test a b c`); !strings.Contains(out, `test: a: unexpected operator`) {
		t.Errorf("test: said %q, want %q", out, `test: a: unexpected operator`)
	}
	if out, _ := runDash(t, dir, `[ a b c ]`); !strings.Contains(out, `[: a: unexpected operator`) {
		t.Errorf("bracket: said %q, want %q", out, `[: a: unexpected operator`)
	}
	// The unary wording is a separate string and carries the name too.
	if out, _ := runDash(t, dir, `[ -Q x ]`); !strings.Contains(out, `[: -Q: unexpected operator`) {
		t.Errorf("unary bracket: said %q, want %q", out, `[: -Q: unexpected operator`)
	}
}

// TestAKilledCommandsStatus: 128 + 13.
func TestAKilledCommandsStatus(t *testing.T) {
	dir := t.TempDir()
	// PATH is the temp directory alone, so the command that dies has to be
	// made here rather than borrowed from the host.
	exe := filepath.Join(dir, "selfkill")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nkill -PIPE $$\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _ := runDash(t, dir, "selfkill\necho st=$?\n")
	if !strings.Contains(out, "st=141") {
		t.Errorf("said %q, want st=141", out)
	}
}

// TestPipefail: dash has no such option: the name is refused and the pipeline goes on reporting its last element, which is the 0 the option exists to avoid.
// This shell has no `pipefail`, and asking for it does not merely fail — it
// ends the script. Measured: `dash -c 'set -o pipefail; …'` prints the
// refusal and nothing else, and exits 2.
//
// This test used to assert that the script carried on and reported `st=0`,
// which is what *we* did rather than what dash does.
func TestPipefail(t *testing.T) {
	dir := t.TempDir()
	out, st := runDash(t, dir, "set -o pipefail\n(exit 3) | (exit 4) | true\necho st=$?\n")
	if !strings.Contains(out, "Illegal option -o pipefail") {
		t.Errorf("said %q, want the refusal this shell words", out)
	}
	if strings.Contains(out, "st=") {
		t.Errorf("said %q, want the script abandoned at the refusal", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
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
	out, _ := runDash(t, dir, "cat < nope\necho st=$?\n")
	if got := strings.SplitN(strings.TrimSpace(out), "\n", 2)[0]; got != `dash: 1: cannot open nope: No such file` {
		t.Errorf("read: said %q, want %q", got, `dash: 1: cannot open nope: No such file`)
	}
	if !strings.Contains(out, "st=2") {
		t.Errorf("said %q, want st=2", out)
	}
	// The other direction, which two of the four word differently.
	out, _ = runDash(t, dir, "echo x > nodir/out\n")
	if got := strings.TrimSpace(out); got != `dash: 1: cannot create nodir/out: Directory nonexistent` {
		t.Errorf("create: said %q, want %q", got, `dash: 1: cannot create nodir/out: Directory nonexistent`)
	}
}

// TestSetOInABundle: `-o` as the last letter of a bundle, which is how every
// real script writes it.
func TestSetOInABundle(t *testing.T) {
	dir := t.TempDir()
	out, _ := runDash(t, dir, "set -uo noglob\necho ok=$?\necho *\n")
	if !strings.Contains(out, "ok=0") || !strings.Contains(out, "*") {
		t.Errorf("said %q, want the bundle's letters and its named option", out)
	}
}

// TestPrintfOptions: this dialect's answers about `printf`'s options, run
// rather than asserted against the fields — the wordings and the usage line
// are what would be wrong.
// dash has no length modifiers at all, so the letter is read as a conversion
// it does not have — and named with the flags and width in front of it.
func TestPrintfLengthModifiers(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{"printf \"[%ld]\" 42\n", "printf: %l: invalid directive"},
		{"printf \"[%5ld]\" 42\n", "printf: %5l: invalid directive"},
		{"printf \"[%zX]\" 255\n", "printf: %z: invalid directive"},
	} {
		if out, _ := runDash(t, dir, tc.src); !strings.Contains(out, tc.want) {
			t.Errorf("%s: said %q, want %q", tc.src, out, tc.want)
		}
	}
}

func TestPrintfOptions(t *testing.T) {
	dir := t.TempDir()
	out, _ := runDash(t, dir, "printf -v o \"%05d\" 42\necho \"[$o]\"\n")
	if !strings.Contains(out, `printf: Illegal option -v`) {
		t.Errorf("-v: said %q, want %q", out, `printf: Illegal option -v`)
	}
	// `--` ends the options everywhere.
	if out, _ := runDash(t, dir, "printf -- \"x\n\"\n"); strings.TrimSpace(out) != "x" {
		t.Errorf("--: said %q, want x", out)
	}
	out, _ = runDash(t, dir, "printf -q x\n")
	if !strings.Contains(out, `printf: Illegal option -q`) {
		t.Errorf("unknown option: said %q, want %q", out, `printf: Illegal option -q`)
	}
	// Whether the complaint is followed by a usage line.
	if got := strings.Contains(out, `Usage`) || strings.Contains(out, `usage`); got != false {
		t.Errorf("usage line present=%v, want false (said %q)", got, out)
	}
}

// TestUmask: this dialect's answers about `umask`, run through a hook that
// keeps the mask in a variable — nothing here touches the machine's own.
func TestUmask(t *testing.T) {
	run := func(src string) (string, int) {
		t.Helper()
		f, err := syntax.Parse(src, dash.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		sem, dg := dash.Semantics(), dash.Diagnostics()
		held := 0o022
		r := &interp.Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "dash", Dialect: presetDialect()}
		r.SetUmask = func(mask int) (int, error) { old := held; held = mask; return old, nil }
		dash.Apply(r)
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

// TestDashHasNoLet: dash evaluates arithmetic only with `$(( ))`, and reports
// `let` as a command it never heard of — so the substrate's builtin has to be
// taken away rather than left to answer.
func TestDashHasNoLet(t *testing.T) {
	dir := t.TempDir()
	// `let`'s own status, not the script's — the echo after it succeeds.
	out, _ := runDash(t, dir, "let \"x = 2 + 3\"\necho \"st=$? [$x]\"\n")
	if !strings.Contains(out, "not found") {
		t.Errorf("said %q, want it reported as not found", out)
	}
	if strings.Contains(out, "[5]") {
		t.Errorf("said %q, want no arithmetic to have happened", out)
	}
	if !strings.Contains(out, "st=127") {
		t.Errorf("said %q, want the not-found status", out)
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
		f, err := syntax.Parse(src, dash.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		sem, dg := dash.Semantics(), dash.Diagnostics()
		r := &interp.Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "dash", Dialect: presetDialect()}
		r.GetRlimit = func(res interp.Resource) (int64, int64, error) { p := held[res]; return p[0], p[1], nil }
		r.SetRlimit = func(res interp.Resource, soft, hard int64) error { held[res] = [2]int64{soft, hard}; return nil }
		dash.Apply(r)
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
		t.Errorf("-m: status %d, want present=yes", st)
	}
	if _, st := run("ulimit -u"); st == 0 {
		t.Errorf("-u: status %d, want present=no", st)
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
	out, _ := runDash(t, dir, "export -Q x\necho \"st=$?\"\necho after\n")
	if !strings.Contains(out, "-Q") {
		t.Errorf("said %q, want the option named", out)
	}
	if reached := strings.Contains(out, "after"); reached != false {
		t.Errorf("said %q, want reached=false", out)
	}
	if got := strings.Contains(strings.ToLower(out), "usage"); got != false {
		t.Errorf("said %q, want usage=false", out)
	}
}

// The complaint names nothing at all here, and reports 2 where the others
// report 1.
func TestPrintfMissingFormatCharacter(t *testing.T) {
	dir := t.TempDir()
	out, st := runDash(t, dir, `printf 'a%5'`+"\n")
	if !strings.Contains(out, "printf: missing format character") || st != 2 {
		t.Errorf("said %q status %d, want an unnamed complaint and 2", out, st)
	}
}

// There is no `\x` in a format here, so the backslash and the letter stand.
func TestPrintfHasNoHexEscape(t *testing.T) {
	dir := t.TempDir()
	if out, _ := runDash(t, dir, `printf 'a\x41Z'`+"\n"); out != `a\x41Z` {
		t.Errorf("said %q, want the escape as written", out)
	}
}

// The `%b` escape table this shell answers for. dash has no `\x` and neither
// spelling of the escape character at either site, and it reads a bare
// `\101` as an `A` — the one thing it and bash agree on that ksh93 and zsh
// do not. `\c` ends the output here where a format writes the characters.
func TestPrintfBEscapesAreDashs(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`printf '%b' 'a\0101Z'`, "aAZ"},
		{`printf '%b' 'a\0300Z'`, "a\xc0Z"},
		{`printf '%b' 'a\101Z'`, "aAZ"},
		{`printf '%b' 'a\eZ:a\EZ'`, `a\eZ:a\EZ`},
		{`printf '%b' 'a\x41Z'`, `a\x41Z`},
		{`printf '%b' 'a\cbZ'`, "a"},
		{`printf 'a\0101Z'`, "a\b1Z"},
		{`printf 'a\cbZ'`, `a\cbZ`},
	} {
		if out, st := runDash(t, dir, tc.src+"\n"); out != tc.want || st != 0 {
			t.Errorf("%s: said % x status %d, want % x and 0", tc.src, out, st, tc.want)
		}
	}
}

// `shift` here has neither options nor an end-of-options marker, so every
// dash word is a number it calls illegal — `--` included, which is the one
// answer in the panel that makes `shift -- 2` fail. A negative count is the
// same complaint rather than a count out of range, and all of it ends the
// script.
func TestShiftHasNoOptionsAndNoMarker(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src  string
		want string
		st   int
	}{
		{`set -- a b c; shift -- 2; echo "st=$? rest=[$*]"`, "dash: 1: shift: Illegal number: --\n", 2},
		{`set -- a b c; shift --; echo "st=$? rest=[$*]"`, "dash: 1: shift: Illegal number: --\n", 2},
		{`set -- a b c; shift -1; echo "st=$? n=$#"`, "dash: 1: shift: Illegal number: -1\n", 2},
		{`set -- a b c; shift -x; echo "st=$? n=$#"`, "dash: 1: shift: Illegal number: -x\n", 2},
		{`set -- a b c; shift +1; echo "st=$? rest=[$*]"`, "st=0 rest=[b c]\n", 0},
		{`set -- a b c; shift -0; echo "st=$? rest=[$*]"`, "st=0 rest=[a b c]\n", 0},
	} {
		if out, st := runDash(t, dir, tc.src+"\n"); out != tc.want || st != tc.st {
			t.Errorf("%s: said %q status %d, want %q and %d", tc.src, out, st, tc.want, tc.st)
		}
	}
}

// Neither spelling of the escape character: this shell's set is the XSI list
// alone, and it has no -e either, so the option word is an operand (#908).
func TestEchoTakesNeitherSpellingOfTheEscapeCharacter(t *testing.T) {
	out, st := runDash(t, t.TempDir(), "echo -e 'a\\eZ:a\\EZ'\n")
	if out != "-e a\\eZ:a\\EZ\n" || st != 0 {
		t.Errorf("said %q status %d, want both letters as written and the -e printed", out, st)
	}
}

// The field applies to what a `\c` left here too (#910).
func TestPrintfBStopIsPadded(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`printf '[%5b]' 'a\cb'`, "[    a"},
		{`printf '[%.1b]' 'ab\cc'`, "[a"},
		{`printf '[%5b]' 'ab'`, "[   ab]"},
	} {
		if out, st := runDash(t, dir, tc.src+"\n"); out != tc.want || st != 0 {
			t.Errorf("%s: said %q status %d, want %q and 0", tc.src, out, st, tc.want)
		}
	}
}

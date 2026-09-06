// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// parseZsh reads a program with this dialect's grammar and nothing else, for
// the cases about what the parser refuses.
func parseZsh(src string) (*syntax.File, error) {
	return syntax.Parse(src, zsh.Dialect())
}

func runZsh(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": dir},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

func TestSourceIsASynonymForDot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(path, []byte("echo via-source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := runZsh(t, dir, `source `+path)
	if st != 0 || strings.TrimSpace(out) != "via-source" {
		t.Errorf("output = %q status %d, want via-source", out, st)
	}
}

// TestADiagnosticNamesTheSourcedFile: a failure at the top level of a sourced
// file names the file as written — `./inc.sh:1: command not found: …` — while
// one inside a function defined there still names the function, which is
// zsh's pairing of the two location answers and the reason the function flag
// wins.
func TestADiagnosticNamesTheSourcedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "inc.sh"),
		[]byte("nosuchcmd-xyz\nf() {\n  nosuchcmd-xyz\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runZsh(t, dir, ". ./inc.sh\nf\n")
	lines := strings.SplitN(out, "\n", 3)
	if got, want := lines[0], "./inc.sh:1: command not found: nosuchcmd-xyz"; got != want {
		t.Errorf("while sourcing: said %q, want %q", got, want)
	}
	if len(lines) < 2 {
		t.Fatalf("output = %q, want two diagnostics", out)
	}
	if got, want := lines[1], "f:1: command not found: nosuchcmd-xyz"; got != want {
		t.Errorf("in the function: said %q, want %q", got, want)
	}
}

// TestASyntaxErrorDependsOnWhereItWasRead is zsh's alone in the panel, and the
// only reason SourcedSyntaxErrorStatus exists: the same unparseable text is 1
// from -c and 126 from a file `.` opened. bash says 2 for both and ksh93 3 for
// both, so folding the two together would have lost one of zsh's answers.
func TestASyntaxErrorDependsOnWhereItWasRead(t *testing.T) {
	d := zsh.Diagnostics()
	if d.SyntaxErrorStatus != 1 {
		t.Errorf("SyntaxErrorStatus = %d, want 1", d.SyntaxErrorStatus)
	}
	if d.SourcedSyntaxErrorStatus != 126 {
		t.Errorf("SourcedSyntaxErrorStatus = %d, want 126", d.SourcedSyntaxErrorStatus)
	}

	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.sh")
	if err := os.WriteFile(bad, []byte("if\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, st := runZsh(t, dir, `eval "if"`); st != 1 {
		t.Errorf("eval of unparseable text = %d, want 1", st)
	}
	if _, st := runZsh(t, dir, `. `+bad); st != 126 {
		t.Errorf("sourcing unparseable text = %d, want 126", st)
	}
}

func TestEvalAndDotAxes(t *testing.T) {
	s := zsh.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		{"BuiltinSyntaxErrorFatal", s.BuiltinSyntaxErrorFatal, interp.No},
		{"DotMissingFileFatal", s.DotMissingFileFatal, interp.No},
		{"DotPassesArguments", s.DotPassesArguments, interp.Yes},
		// Not bash: PATH missing the file is the end of it here.
		{"DotFallsBackToCurrentDirectory", s.DotFallsBackToCurrentDirectory, interp.No},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
	// 127 for a file it cannot open, where bash says 1 — the most divergent
	// answer in the panel, and it survives rather than ending the script.
	if got := zsh.Diagnostics().DotCannotOpenStatus; got != 127 {
		t.Errorf("DotCannotOpenStatus = %d, want 127", got)
	}
}

// TestExecAxesAndWording records what zsh does about `exec`, which is the most
// divergent of the four.
func TestExecAxesAndWording(t *testing.T) {
	s := zsh.Semantics()
	// zsh and ksh93 drop the EXIT trap where dash and bash run it.
	if got := s.ExecFailureRunsExitTrap; got != interp.No {
		t.Errorf("ExecFailureRunsExitTrap = %v, want No", got)
	}
	if got := s.ExecTakesOptions; got != interp.Yes {
		t.Errorf("ExecTakesOptions = %v, want Yes", got)
	}
	d := zsh.Diagnostics()
	// The only dialect that lowercases every strerror string it quotes.
	if !d.LowercaseReason {
		t.Error("LowercaseReason should be true for zsh")
	}
	// zsh hands the path to execve rather than checking for a directory, so it
	// reports the permission error that comes back.
	if d.DirectoryReason == "" {
		t.Error("zsh reports execve's own error for a directory")
	}
	if d.NamesResolvedPath {
		t.Error("zsh reports the operand as written, not resolved")
	}
}

// TestAFailedExecDropsTheExitTrap is the axis as behavior.
func TestAFailedExecDropsTheExitTrap(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `trap "echo TRAP" EXIT; exec nosuchcmd-xyz`)
	if strings.Contains(out, "TRAP") {
		t.Errorf("zsh drops the EXIT trap after a failed exec: %q", out)
	}
	if st != 127 {
		t.Errorf("status = %d, want 127", st)
	}
}

// TestDoubleEqualInTest: zsh takes `==` as a second spelling of `=`, and is
// the one dialect where it has to be quoted to reach the builtin at all — an
// unquoted word starting with `=` is expanded to the path of the command
// named after it, so bare `==` is a search for a command called `=`. Real zsh
// says "= not found" and exits 1, and so do we.
func TestDoubleEqualInTest(t *testing.T) {
	dir := t.TempDir()
	if _, st := runZsh(t, dir, `test a "==" a`); st != 0 {
		t.Errorf("equal operands: status %d, want 0", st)
	}
	if _, st := runZsh(t, dir, `test a "==" b`); st != 1 {
		t.Errorf("unequal operands: status %d, want 1", st)
	}
	// The single `=` is unanimous, and pins the difference to the operator
	// rather than to anything else about how the words are read.
	if _, st := runZsh(t, dir, `test a = a`); st != 0 {
		t.Errorf("single equals: status %d, want 0", st)
	}
	// Unquoted, the expansion gets it first and the builtin never sees it.
	if out, st := runZsh(t, dir, `test a == a`); st != 1 || !strings.Contains(out, "not found") {
		t.Errorf("unquoted: %q status %d, want the `=` expansion to miss", out, st)
	}
}

// TestABracketNamesItself: `[` is `test` under another name, and the
// diagnostic blames the name that was typed. Run rather than asserted against
// the wording string, since the wording is what would be wrong.
func TestABracketNamesItself(t *testing.T) {
	dir := t.TempDir()
	if out, _ := runZsh(t, dir, `test 1 -eq a`); !strings.Contains(out, `:test:`) {
		t.Errorf("test: said %q, want %q", out, `:test:`)
	}
	if out, _ := runZsh(t, dir, `[ 1 -eq a ]`); !strings.Contains(out, `:[:`) {
		t.Errorf("bracket: said %q, want %q", out, `:[:`)
	}
	// The unary wording is a separate string and carries the name too.
	if out, _ := runZsh(t, dir, `[ -Q x ]`); !strings.Contains(out, `:[:`) {
		t.Errorf("unary bracket: said %q, want %q", out, `:[:`)
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
	out, _ := runZsh(t, dir, "selfkill\necho st=$?\n")
	if !strings.Contains(out, "st=141") {
		t.Errorf("said %q, want st=141", out)
	}
}

// TestPipefail: zsh has the option.
func TestPipefail(t *testing.T) {
	dir := t.TempDir()
	out, _ := runZsh(t, dir, "set -o pipefail\n(exit 3) | (exit 4) | true\necho st=$?\n")
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
	out, _ := runZsh(t, dir, "cat < nope\necho st=$?\n")
	if got := strings.SplitN(strings.TrimSpace(out), "\n", 2)[0]; got != `zsh:1: no such file or directory: nope` {
		t.Errorf("read: said %q, want %q", got, `zsh:1: no such file or directory: nope`)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want st=1", out)
	}
	// The other direction, which two of the four word differently.
	out, _ = runZsh(t, dir, "echo x > nodir/out\n")
	if got := strings.TrimSpace(out); got != `zsh:1: no such file or directory: nodir/out` {
		t.Errorf("create: said %q, want %q", got, `zsh:1: no such file or directory: nodir/out`)
	}
}

// TestSetOInABundle: `-o` as the last letter of a bundle, which is how every
// real script writes it.
func TestSetOInABundle(t *testing.T) {
	dir := t.TempDir()
	out, _ := runZsh(t, dir, "set -uo noglob\necho ok=$?\necho *\n")
	if !strings.Contains(out, "ok=0") || !strings.Contains(out, "*") {
		t.Errorf("said %q, want the bundle's letters and its named option", out)
	}
}

// TestErrexitAndPipefail: whether `set -e` stops for a failure only pipefail
// produced. An ordinary failing pipeline stops every shell in the panel, so
// this is about the failure the option adds and not about pipelines.
func TestErrexitAndPipefail(t *testing.T) {
	dir := t.TempDir()
	out, _ := runZsh(t, dir, "set -eo pipefail\nfalse | true\necho reached\n")
	if got := strings.Contains(out, "reached"); got != false {
		t.Errorf("said %q, want reached=false", out)
	}
	// Unanimous, and must not move with the answer above.
	out, _ = runZsh(t, dir, "set -eo pipefail\ntrue | false\necho unreached\n")
	if strings.Contains(out, "unreached") {
		t.Errorf("ordinary failure: said %q, want it to stop", out)
	}
}

// TestPrintfOptions: this dialect's answers about `printf`'s options, run
// rather than asserted against the fields — the wordings and the usage line
// are what would be wrong.
// zsh takes C89's three letters and exactly one of them, so the C99
// additions are invalid directives — named as the whole directive.
func TestPrintfLengthModifiers(t *testing.T) {
	dir := t.TempDir()
	out, _ := runZsh(t, dir, "printf \"[%ld][%hd][%Lf]\\n\" 42 42 1.5\n")
	if want := "[42][42][1.500000]"; !strings.Contains(out, want) {
		t.Errorf("said %q, want %q", out, want)
	}
	for _, tc := range []struct{ src, want string }{
		{"printf \"[%zX]\" 255\n", "%z: invalid directive"},
		{"printf \"[%lld]\" 42\n", "%ll: invalid directive"},
		{"printf \"[%jd]\" 42\n", "%j: invalid directive"},
	} {
		if out, _ := runZsh(t, dir, tc.src); !strings.Contains(out, tc.want) {
			t.Errorf("%s: said %q, want %q", tc.src, out, tc.want)
		}
	}
}

func TestPrintfOptions(t *testing.T) {
	dir := t.TempDir()
	out, _ := runZsh(t, dir, "printf -v o \"%05d\" 42\necho \"[$o]\"\n")
	if !strings.Contains(out, `[00042]`) {
		t.Errorf("-v: said %q, want %q", out, `[00042]`)
	}
	// `--` ends the options everywhere.
	if out, _ := runZsh(t, dir, "printf -- \"x\n\"\n"); strings.TrimSpace(out) != "x" {
		t.Errorf("--: said %q, want x", out)
	}
	out, _ = runZsh(t, dir, "printf -q x\n")
	// The whole output, not a substring of it: zsh takes the word as the
	// *format* and prints it, and a refusal would print `-q` too — inside
	// `printf: -q: invalid option`. Contains cannot tell those apart.
	if got := strings.TrimSpace(out); got != "-q" {
		t.Errorf("unknown option: said %q, want exactly %q", got, "-q")
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
		f, err := syntax.Parse(src, zsh.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		sem, dg := zsh.Semantics(), zsh.Diagnostics()
		held := 0o022
		r := &interp.Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "zsh", Dialect: presetDialect()}
		r.SetUmask = func(mask int) (int, error) { old := held; held = mask; return old, nil }
		zsh.Apply(r)
		st, rerr := r.Run(context.Background(), f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		return buf.String(), st
	}
	if out, _ := run("umask"); strings.TrimSpace(out) != "022" {
		t.Errorf("read: said %q, want %q", out, "022")
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
	if out, _ := runZsh(t, dir, "let \"x = 2 + 3\"\necho $x\n"); strings.TrimSpace(out) != "5" {
		t.Errorf("said %q, want 5", out)
	}
	// Zero is a failure, which is the half a caller has to know about.
	if _, st := runZsh(t, dir, "let \"x=0\"\n"); st != 1 {
		t.Errorf("zero: status %d, want 1", st)
	}
	if _, st := runZsh(t, dir, "let \"x=5\"\n"); st != 0 {
		t.Errorf("nonzero: status %d, want 0", st)
	}
	out, st := runZsh(t, dir, "let\n")
	if st != 1 {
		t.Errorf("empty: status %d, want 1", st)
	}
	// Whether the complaint carries this shell's name and a location.
	if got := strings.HasPrefix(strings.TrimSpace(out), "zsh"); got != true {
		t.Errorf("empty: said %q, want prefixed=true", out)
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
		f, err := syntax.Parse(src, zsh.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		sem, dg := zsh.Semantics(), zsh.Diagnostics()
		r := &interp.Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "zsh", Dialect: presetDialect()}
		r.GetRlimit = func(res interp.Resource) (int64, int64, error) { p := held[res]; return p[0], p[1], nil }
		r.SetRlimit = func(res interp.Resource, soft, hard int64) error { held[res] = [2]int64{soft, hard}; return nil }
		zsh.Apply(r)
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
	if _, st := run("ulimit -m"); st == 0 {
		t.Errorf("-m: status %d, want present=no", st)
	}
	if _, st := run("ulimit -u"); st != 0 {
		t.Errorf("-u: status %d, want present=yes", st)
	}
	// Whether setting lowers the hard limit with the soft one.
	run("ulimit -t 50")
	if got := held[interp.ResourceCPUTime]; got[1] == 50 {
		t.Errorf("hard limit is %d, want soft", got[1])
	}
}

// TestABadBuiltinOption: this dialect's wording, status, usage line and
// whether a special builtin's bad option ends the script.
func TestABadBuiltinOption(t *testing.T) {
	dir := t.TempDir()
	out, _ := runZsh(t, dir, "export -Q x\necho \"st=$?\"\necho after\n")
	if !strings.Contains(out, "-Q") {
		t.Errorf("said %q, want the option named", out)
	}
	if reached := strings.Contains(out, "after"); reached != true {
		t.Errorf("said %q, want reached=true", out)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want st=1", out)
	}
	if got := strings.Contains(strings.ToLower(out), "usage"); got != false {
		t.Errorf("said %q, want usage=false", out)
	}
}

// The same complaint as an unknown conversion, spelled with the directive.
func TestPrintfMissingFormatCharacter(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`printf 'a%'`, "%: invalid directive"},
		{`printf 'a%5'`, "%5: invalid directive"},
	} {
		out, st := runZsh(t, dir, tc.src+"\n")
		if !strings.Contains(out, tc.want) || st != 1 {
			t.Errorf("%s: said %q status %d, want %q and 1", tc.src, out, st, tc.want)
		}
	}
}

// `\x` reads at most two digits as one byte, and an empty digit run as a
// zero rather than as an escape left standing.
func TestPrintfHexEscapeIsAByteOrANul(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`printf 'a\x41Z'`, "aAZ"},
		{`printf '[\x0ff]'`, "[\x0ff]"},
		{`printf 'a\xZ'`, "a\x00Z"},
	} {
		if out, _ := runZsh(t, dir, tc.src+"\n"); out != tc.want {
			t.Errorf("%s: said % x, want % x", tc.src, out, tc.want)
		}
	}
}

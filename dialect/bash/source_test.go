// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What bash does about `eval` and `.`, measured against the real binary and
// recorded here rather than in the substrate.

func runBash(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": dir},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// runBashPrelude runs src with the dialect's prelude installed the way the
// front end installs it, rather than pasted on the front of the snippet.
//
// The difference is the whole of what these directory-stack tests assert.
// Pasted on, `pushd` and `popd` arrive as the *script's* functions: they speak
// with no location in front of them, and the line a diagnostic names is a line
// of the prelude. Installed, they are the shell's, and what they say is what
// bash says down to the `bash: line N: ` (#603).
func runBashPrelude(t *testing.T, dir, src string) (string, int) {
	t.Helper()
	out, st, err := preset.CombinedWithPrelude(t, dialecttest.Base{
		Dir: dir, Vars: map[string]string{"PATH": dir},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// wantWholeLines fails unless each named line appears in the output entire.
//
// Not strings.Contains of a fragment, which is what let these cases pass while
// the location in front of every one of them was missing: a prefix added
// *before* the text a Contains check names is invisible to it. The line is
// compared from its start, so `bash: line 5: ` is part of the assertion rather
// than something the assertion cannot see.
func wantWholeLines(t *testing.T, out string, want ...string) {
	t.Helper()
	lines := strings.Split(out, "\n")
	for _, w := range want {
		if !slices.Contains(lines, w) {
			t.Errorf("output = %q, want the whole line %q in it", out, w)
		}
	}
}

func TestSourceIsASynonymForDot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.sh")
	if err := os.WriteFile(path, []byte("echo via-source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := runBash(t, dir, `source `+path)
	if st != 0 || strings.TrimSpace(out) != "via-source" {
		t.Errorf("output = %q status %d, want via-source", out, st)
	}
}

func TestEvalAndDotAxes(t *testing.T) {
	s := bash.Semantics()
	for _, tc := range []struct {
		axis string
		got  interp.Answer
		want interp.Answer
	}{
		// Measured: `eval "if"; echo REACHED` prints REACHED here and does
		// not in dash.
		{"BuiltinSyntaxErrorFatal", s.BuiltinSyntaxErrorFatal, interp.No},
		{"DotMissingFileFatal", s.DotMissingFileFatal, interp.No},
		{"DotPassesArguments", s.DotPassesArguments, interp.Yes},
		// The one shell in the panel that does this.
		{"DotFallsBackToCurrentDirectory", s.DotFallsBackToCurrentDirectory, interp.Yes},
		// An error inside a sourced file ends the shell here, where ksh93 and
		// zsh end only the file — and `${x?word}` is one of those errors
		// rather than a request to stop, which shows at the startup-file
		// boundary this shell does give up a file at.
		{"FatalErrorEndsBorrowedTextOnly", s.FatalErrorEndsBorrowedTextOnly, interp.No},
		{"ParamErrorIsAnExitRequest", s.ParamErrorIsAnExitRequest, interp.No},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.axis, tc.got, tc.want)
		}
	}
}

// TestADiagnosticNamesTheSourcedFile: a failure inside a sourced file names
// that file rather than the script — `./inc.sh: line 1: …` — and the name is
// the operand as written, not the absolute path it resolved to. A function
// defined in the file and called after the sourcing has finished still names
// the defining file.
//
// The whole first line rather than a substring: the shell's own name in its
// place would still leave the message's tail intact, so Contains could not
// tell the fixed shape from the broken one.
func TestADiagnosticNamesTheSourcedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "inc.sh"),
		[]byte("nosuchcmd-xyz\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runBash(t, dir, ". ./inc.sh\necho st=$?\n")
	lines := strings.SplitN(out, "\n", 3)
	if got, want := lines[0], "./inc.sh: line 1: nosuchcmd-xyz: command not found"; got != want {
		t.Errorf("while sourcing: said %q, want %q", got, want)
	}
	if len(lines) < 2 || lines[1] != "st=127" {
		t.Errorf("output = %q, want st=127", out)
	}

	// The defining file, remembered past the end of the sourcing.
	if err := os.WriteFile(filepath.Join(dir, "def.sh"),
		[]byte("f() {\n  nosuchcmd-xyz\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ = runBash(t, dir, ". ./def.sh\nf\n")
	if got, want := strings.SplitN(out, "\n", 2)[0],
		"./def.sh: line 2: nosuchcmd-xyz: command not found"; got != want {
		t.Errorf("from a sourced function: said %q, want %q", got, want)
	}
}

// TestAnUnparseableEvalIsSurvivable is the axis as behavior rather than as a
// field, which is what stops the two drifting apart.
func TestAnUnparseableEvalIsSurvivable(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `eval "if"; echo REACHED`)
	if !strings.Contains(out, "REACHED") {
		t.Errorf("bash reports an unparseable eval and carries on: %q", out)
	}
}

// TestDotFindsAFileInTheCurrentDirectory is bash's alone. PATH here has the
// temp directory in it, so the file is found without the fallback; the point
// of the second half is that the fallback is what finds it when PATH cannot.
func TestDotFindsAFileInTheCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fb.sh"), []byte("echo cwd-hit\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := syntax.Parse(`. fb.sh`, bash.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sem, diag := bash.Semantics(), bash.Diagnostics()
	r := &interp.Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "bash",
		// PATH deliberately cannot reach it, so only the fallback can.
		Vars:    map[string]string{"PATH": t.TempDir()},
		Dialect: presetDialect(),
	}
	bash.Apply(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "cwd-hit") {
		t.Errorf("bash looks in the current directory once PATH misses: %q", buf.String())
	}
}

func TestDotStatusesAreBashs(t *testing.T) {
	d := bash.Diagnostics()
	// Two numbers for what reads like one failure, which is why they are two
	// fields: a file it cannot open is 1 and a missing operand is 2.
	if got := d.DotCannotOpenStatus; got != 1 {
		t.Errorf("DotCannotOpenStatus = %d, want 1", got)
	}
	if got := d.DotNoOperandStatus; got != 2 {
		t.Errorf("DotNoOperandStatus = %d, want 2", got)
	}
	// bash prints a complaint and a usage line, and only the first carries the
	// shell's prefix — so the wording itself has to hold the newline.
	if !strings.Contains(d.DotNoOperand, "\n") {
		t.Errorf("DotNoOperand = %q, want the two lines bash prints", d.DotNoOperand)
	}
	// A syntax error is 2 whether it was read from -c or from a sourced file,
	// so the sourced override stays empty here. zsh is the one that needs it.
	if got := d.SourcedSyntaxErrorStatus; got != 0 {
		t.Errorf("SourcedSyntaxErrorStatus = %d, want 0: bash answers both the same", got)
	}
}

// TestExecAxesAndWording records what bash does about `exec`.
func TestExecAxesAndWording(t *testing.T) {
	s := bash.Semantics()
	if got := s.ExecFailureRunsExitTrap; got != interp.Yes {
		t.Errorf("ExecFailureRunsExitTrap = %v, want Yes", got)
	}
	if got := s.ExecTakesOptions; got != interp.Yes {
		t.Errorf("ExecTakesOptions = %v, want Yes", got)
	}
	d := bash.Diagnostics()
	// bash alone names the path it tried rather than the operand as written,
	// and only for `exec` — the same bash reports `. ./nosuch.sh` as written.
	if !d.NamesResolvedPath {
		t.Error("NamesResolvedPath should be true for bash")
	}
	// It checks for a directory itself rather than reporting execve's EACCES,
	// so it needs no override for that reason.
	if d.DirectoryReason != "" {
		t.Errorf("DirectoryReason = %q, want empty: bash says what the OS said", d.DirectoryReason)
	}
	if d.LowercaseReason {
		t.Error("bash prints the C strerror string as it comes")
	}
}

// TestAFailedExecRunsTheExitTrap is the axis as behavior. bash and dash run
// it; ksh93 and zsh drop it.
func TestAFailedExecRunsTheExitTrap(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `trap "echo TRAP" EXIT; exec nosuchcmd-xyz`)
	if !strings.Contains(out, "TRAP") {
		t.Errorf("bash runs the EXIT trap after a failed exec: %q", out)
	}
	if st != 127 {
		t.Errorf("status = %d, want 127", st)
	}
}

// TestDoubleEqualInTest: bash takes `==` as a second spelling of `=`.
func TestDoubleEqualInTest(t *testing.T) {
	dir := t.TempDir()
	if _, st := runBash(t, dir, `test a == a`); st != 0 {
		t.Errorf("equal operands: status %d, want 0", st)
	}
	if _, st := runBash(t, dir, `test a == b`); st != 1 {
		t.Errorf("unequal operands: status %d, want 1", st)
	}
	// The single `=` is unanimous, and pins the difference to the operator
	// rather than to anything else about how the words are read.
	if _, st := runBash(t, dir, `test a = a`); st != 0 {
		t.Errorf("single equals: status %d, want 0", st)
	}
}

// TestABracketNamesItself: `[` is `test` under another name, and the
// diagnostic blames the name that was typed. Run rather than asserted against
// the wording string, since the wording is what would be wrong.
func TestABracketNamesItself(t *testing.T) {
	dir := t.TempDir()
	if out, _ := runBash(t, dir, `test a b c`); !strings.Contains(out, `test: b: binary operator expected`) {
		t.Errorf("test: said %q, want %q", out, `test: b: binary operator expected`)
	}
	if out, _ := runBash(t, dir, `[ a b c ]`); !strings.Contains(out, `[: b: binary operator expected`) {
		t.Errorf("bracket: said %q, want %q", out, `[: b: binary operator expected`)
	}
	// The unary wording is a separate string and carries the name too.
	if out, _ := runBash(t, dir, `[ -Q x ]`); !strings.Contains(out, `[: -Q: unary operator expected`) {
		t.Errorf("unary bracket: said %q, want %q", out, `[: -Q: unary operator expected`)
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
	out, _ := runBash(t, dir, "selfkill\necho st=$?\n")
	if !strings.Contains(out, "st=141") {
		t.Errorf("said %q, want st=141", out)
	}
}

// TestPipefail: bash has the option, and the last *failing* element is 4 rather than the last element's 0.
func TestPipefail(t *testing.T) {
	dir := t.TempDir()
	out, _ := runBash(t, dir, "set -o pipefail\n(exit 3) | (exit 4) | true\necho st=$?\n")
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
	out, _ := runBash(t, dir, "cat < nope\necho st=$?\n")
	if got := strings.SplitN(strings.TrimSpace(out), "\n", 2)[0]; got != `bash: line 1: nope: No such file or directory` {
		t.Errorf("read: said %q, want %q", got, `bash: line 1: nope: No such file or directory`)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want st=1", out)
	}
	// The other direction, which two of the four word differently.
	out, _ = runBash(t, dir, "echo x > nodir/out\n")
	if got := strings.TrimSpace(out); got != `bash: line 1: nodir/out: No such file or directory` {
		t.Errorf("create: said %q, want %q", got, `bash: line 1: nodir/out: No such file or directory`)
	}
}

// TestSetOInABundle: `-o` as the last letter of a bundle, which is how every
// real script writes it.
func TestSetOInABundle(t *testing.T) {
	dir := t.TempDir()
	out, _ := runBash(t, dir, "set -uo noglob\necho ok=$?\necho *\n")
	if !strings.Contains(out, "ok=0") || !strings.Contains(out, "*") {
		t.Errorf("said %q, want the bundle's letters and its named option", out)
	}
}

// TestErrexitAndPipefail: whether `set -e` stops for a failure only pipefail
// produced. An ordinary failing pipeline stops every shell in the panel, so
// this is about the failure the option adds and not about pipelines.
func TestErrexitAndPipefail(t *testing.T) {
	dir := t.TempDir()
	out, _ := runBash(t, dir, "set -eo pipefail\nfalse | true\necho reached\n")
	if got := strings.Contains(out, "reached"); got != false {
		t.Errorf("said %q, want reached=false", out)
	}
	// Unanimous, and must not move with the answer above.
	out, _ = runBash(t, dir, "set -eo pipefail\ntrue | false\necho unreached\n")
	if strings.Contains(out, "unreached") {
		t.Errorf("ordinary failure: said %q, want it to stop", out)
	}
}

// TestPrintfOptions: this dialect's answers about `printf`'s options, run
// rather than asserted against the fields — the wordings and the usage line
// are what would be wrong.
// bash takes the whole C99 set of length modifiers and ignores every one of
// them, and names the *conversion* when it cannot read one.
func TestPrintfLengthModifiers(t *testing.T) {
	dir := t.TempDir()
	out, _ := runBash(t, dir, "printf \"[%ld][%zX][%jd][%lld][%hhd][%Lf]\\n\" 42 255 42 42 300 1.5\n")
	if want := "[42][FF][42][42][300][1.500000]"; !strings.Contains(out, want) {
		t.Errorf("said %q, want %q", out, want)
	}
	// A run of the letters rather than a list of spellings.
	if out, _ := runBash(t, dir, "printf \"[%llld]\\n\" 42\n"); !strings.Contains(out, "[42]") {
		t.Errorf("a run: said %q, want [42]", out)
	}
	// The modifier is read and thrown away, so the conversion named here is
	// the `Q` after it and not the `l`.
	if out, _ := runBash(t, dir, "printf \"%lQ\" 1\n"); !strings.Contains(out, "`Q': invalid format character") {
		t.Errorf("said %q, want the conversion named", out)
	}
}

func TestPrintfOptions(t *testing.T) {
	dir := t.TempDir()
	out, _ := runBash(t, dir, "printf -v o \"%05d\" 42\necho \"[$o]\"\n")
	if !strings.Contains(out, `[00042]`) {
		t.Errorf("-v: said %q, want %q", out, `[00042]`)
	}
	// `--` ends the options everywhere.
	if out, _ := runBash(t, dir, "printf -- \"x\n\"\n"); strings.TrimSpace(out) != "x" {
		t.Errorf("--: said %q, want x", out)
	}
	out, _ = runBash(t, dir, "printf -q x\n")
	if !strings.Contains(out, `printf: -q: invalid option`) {
		t.Errorf("unknown option: said %q, want %q", out, `printf: -q: invalid option`)
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
		f, err := syntax.Parse(src, bash.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		sem, dg := bash.Semantics(), bash.Diagnostics()
		held := 0o022
		r := &interp.Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "bash", Dialect: presetDialect()}
		r.SetUmask = func(mask int) (int, error) { old := held; held = mask; return old, nil }
		bash.Apply(r)
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
	if got := strings.TrimSpace(out); got != "u=rwx,g=,o=" {
		t.Errorf("-S with a mask: said %q, want %q", got, "u=rwx,g=,o=")
	}
}

// TestLet: this dialect has `let`, and words an empty one its own way.
func TestLet(t *testing.T) {
	dir := t.TempDir()
	if out, _ := runBash(t, dir, "let \"x = 2 + 3\"\necho $x\n"); strings.TrimSpace(out) != "5" {
		t.Errorf("said %q, want 5", out)
	}
	// Zero is a failure, which is the half a caller has to know about.
	if _, st := runBash(t, dir, "let \"x=0\"\n"); st != 1 {
		t.Errorf("zero: status %d, want 1", st)
	}
	if _, st := runBash(t, dir, "let \"x=5\"\n"); st != 0 {
		t.Errorf("nonzero: status %d, want 0", st)
	}
	out, st := runBash(t, dir, "let\n")
	if st != 1 {
		t.Errorf("empty: status %d, want 1", st)
	}
	// Whether the complaint carries this shell's name and a location.
	if got := strings.HasPrefix(strings.TrimSpace(out), "bash"); got != true {
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
		f, err := syntax.Parse(src, bash.Dialect())
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		sem, dg := bash.Semantics(), bash.Diagnostics()
		r := &interp.Runner{Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &dg, Name: "bash", Dialect: presetDialect()}
		r.GetRlimit = func(res interp.Resource) (int64, int64, error) { p := held[res]; return p[0], p[1], nil }
		r.SetRlimit = func(res interp.Resource, soft, hard int64) error { held[res] = [2]int64{soft, hard}; return nil }
		bash.Apply(r)
		st, rerr := r.Run(context.Background(), f)
		if rerr != nil {
			t.Fatal(rerr)
		}
		return buf.String(), st
	}
	// 2048 bytes is 4 blocks of 512 and 2 of 1024.
	if out, _ := run("ulimit -f"); strings.TrimSpace(out) != "2" {
		t.Errorf("block size: said %q, want 2", out)
	}
	// The two letters that are not universal.
	if _, st := run("ulimit -m"); st != 0 {
		t.Errorf("-m: status %d, want present=yes", st)
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
	out, _ := runBash(t, dir, "export -Q x\necho \"st=$?\"\necho after\n")
	if !strings.Contains(out, "-Q") {
		t.Errorf("said %q, want the option named", out)
	}
	if reached := strings.Contains(out, "after"); reached != true {
		t.Errorf("said %q, want reached=true", out)
	}
	if !strings.Contains(out, "st=2") {
		t.Errorf("said %q, want st=2", out)
	}
	if got := strings.Contains(strings.ToLower(out), "usage"); got != true {
		t.Errorf("said %q, want usage=true", out)
	}
}

// TestPrintfTimeConversion: `%(fmt)T`, which is this dialect's alone in the
// panel. The zone is fixed in the script rather than taken from whatever the
// machine running the test is set to.
func TestPrintfTimeConversion(t *testing.T) {
	dir := t.TempDir()
	out, _ := runBash(t, dir, `TZ=UTC
printf '%(%Y-%m-%dT%H:%M:%S %Z)T\n' 1000000000
printf '[%()T][%12(%Y)T]\n' 1000000000 1100000000
printf '%(%Y)T\n' abc; echo "bad=$?"`)
	for _, want := range []string{
		"2001-09-09T01:46:40 UTC\n",
		"[01:46:40][        2004]\n",
		// The complaint, and then the epoch zero anyway.
		"printf: abc: invalid number\n",
		"1970\nbad=1\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want %q in it", out, want)
		}
	}
}

// A format that ends before its conversion character has a second wording
// here, and it names the whole directive where the ordinary bad-conversion
// complaint names the character.
func TestPrintfMissingFormatCharacter(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`printf 'a%'`, "printf: `%': missing format character"},
		{`printf 'a%5'`, "printf: `%5': missing format character"},
		{`printf 'a%ll'`, "printf: `%ll': missing format character"},
	} {
		out, st := runBash(t, dir, tc.src+"\n")
		if !strings.Contains(out, tc.want) || st != 1 {
			t.Errorf("%s: said %q status %d, want %q and 1", tc.src, out, st, tc.want)
		}
	}
}

// `\x` reads at most two digits as one byte, and an empty digit run leaves
// the escape standing with a complaint that does not fail the command.
func TestPrintfHexEscapeIsAByte(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`printf 'a\x41Z'`, "aAZ"},
		{`printf '[\x0ff]'`, "[\x0ff]"},
		{`printf 'a\x80Z'`, "a\x80Z"},
	} {
		if out, _ := runBash(t, dir, tc.src+"\n"); out != tc.want {
			t.Errorf("%s: said % x, want % x", tc.src, out, tc.want)
		}
	}
	out, st := runBash(t, dir, `printf 'a\xZ'`+"\n")
	if !strings.Contains(out, `printf: missing hex digit for \x`) || st != 0 {
		t.Errorf("said %q status %d, want the warning and a zero status", out, st)
	}
}

// The `%b` escape table this shell answers for, which is not its format's:
// `\0101` is an `A` here and a backspace and a `1` in a format, `\101` is an
// `A` too, both spellings of the escape character are 0x1b, `\x41` is an `A`,
// and `\c` ends the output where a format writes the two characters.
func TestPrintfBEscapesAreBashs(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`printf '%b' 'a\0101Z'`, "aAZ"},
		{`printf '%b' 'a\0300Z'`, "a\xc0Z"},
		{`printf '%b' 'a\101Z'`, "aAZ"},
		{`printf '%b' 'a\eZ:a\EZ'`, "a\x1bZ:a\x1bZ"},
		{`printf '%b' 'a\x41Z'`, "aAZ"},
		{`printf '%b' 'a\cbZ'`, "a"},
		{`printf 'a\0101Z'`, "a\b1Z"},
		{`printf 'a\cbZ'`, `a\cbZ`},
	} {
		if out, st := runBash(t, dir, tc.src+"\n"); out != tc.want || st != 0 {
			t.Errorf("%s: said % x status %d, want % x and 0", tc.src, out, st, tc.want)
		}
	}
}

// `shift` here reads no dash word as an option — `-x` is a count that is not
// a number and `-1` is a count that is out of range — and honors the
// end-of-options marker all the same, which is why those are two questions.
func TestShiftReadsNoOptionsAndStillTakesTheMarker(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src  string
		want string
		st   int
	}{
		{`set -- a b c; shift -- 2; echo "st=$? rest=[$*]"`, "st=0 rest=[c]\n", 0},
		{`set -- a b c; shift --; echo "st=$? rest=[$*]"`, "st=0 rest=[b c]\n", 0},
		{`set -- a b c; shift +1; echo "st=$? rest=[$*]"`, "st=0 rest=[b c]\n", 0},
		{`set -- a b c; shift -0; echo "st=$? rest=[$*]"`, "st=0 rest=[a b c]\n", 0},
		{`set -- a b c; shift -1; echo "st=$? n=$#"`, "bash: line 1: shift: -1: shift count out of range\nst=1 n=3\n", 0},
		{`set -- a b c; shift -- -1; echo "st=$? n=$#"`, "bash: line 1: shift: -1: shift count out of range\nst=1 n=3\n", 0},
		{`set -- a b c; shift -x; echo "st=$? n=$#"`, "bash: line 1: shift: -x: numeric argument required\nst=2 n=3\n", 0},
	} {
		if out, st := runBash(t, dir, tc.src+"\n"); out != tc.want || st != tc.st {
			t.Errorf("%s: said %q status %d, want %q and %d", tc.src, out, st, tc.want, tc.st)
		}
	}
}

// `read -i` takes its argument and does nothing with it. The seed is the text
// a line editor opens with, so it has an effect only where there is a
// terminal and an editor on it, and `-e` — the letter that would open one —
// is refused here as unimplemented. bash answers the same way wherever its
// own input is not a terminal.
//
// The seed is not a *default* for an empty line, which is the reading of the
// manual that looks right and is not: an empty line leaves the variable
// empty.
func TestReadInitialValueIsTakenAndIgnored(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		src  string
		want string
		st   int
	}{
		{`printf "x\n" | { l=keep; read -i pre -r l; echo "st=$? l=[$l]"; }`, "st=0 l=[x]\n", 0},
		{`printf "\n" | { l=keep; read -i pre -r l; echo "st=$? l=[$l]"; }`, "st=0 l=[]\n", 0},
		{`printf "x\n" | { l=keep; read -ipre -r l; echo "st=$? l=[$l]"; }`, "st=0 l=[x]\n", 0},
		{
			// The argument is consumed, so the word after it is not the
			// variable name: without that `pre` would be assigned to.
			`printf "x\n" | { pre=keep; read -i pre -r l; echo "pre=[$pre] l=[$l]"; }`,
			"pre=[keep] l=[x]\n", 0,
		},
		{
			`printf "x\n" | { read -i; echo "st=$?"; }`,
			"bash: line 1: read: -i: option requires an argument\n" +
				"read: usage: read [-Eers] [-a array] [-d delim] [-i text] [-n nchars] [-N nchars] [-p prompt] [-t timeout] [-u fd] [name ...]\n" +
				"st=2\n", 0,
		},
		{
			// The editor itself is still refused by name, which is what
			// keeps the seed from ever having somewhere to go.
			`printf "x\n" | { read -e -r l; echo "st=$?"; }`,
			"bash: line 1: read: -e is not implemented yet\nst=2\n", 0,
		},
	} {
		if out, st := runBash(t, dir, tc.src+"\n"); out != tc.want || st != tc.st {
			t.Errorf("%s: said %q status %d, want %q and %d", tc.src, out, st, tc.want, tc.st)
		}
	}
}

// Both spellings of the escape character are ESC here, which is this shell
// alone in the panel: ksh93 has only `\E` and zsh only `\e` (#908).
func TestEchoTakesBothSpellingsOfTheEscapeCharacter(t *testing.T) {
	out, st := runBash(t, t.TempDir(), "echo -e 'a\\eZ:a\\EZ'\n")
	if out != "a\x1bZ:a\x1bZ\n" || st != 0 {
		t.Errorf("said % x status %d, want both letters as ESC", out, st)
	}
}

// What a `\c` leaves of a `%b` still goes through the conversion's field
// here, which is five of the six — ksh93 alone writes it as it stands (#910).
func TestPrintfBStopIsPadded(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ src, want string }{
		{`printf '[%5b]' 'a\cb'`, "[    a"},
		{`printf '[%-5b]' 'a\cb'`, "[a    "},
		{`printf '[%.1b]' 'ab\cc'`, "[a"},
		{`printf '[%5b]' 'ab'`, "[   ab]"},
	} {
		if out, st := runBash(t, dir, tc.src+"\n"); out != tc.want || st != 0 {
			t.Errorf("%s: said %q status %d, want %q and 0", tc.src, out, st, tc.want)
		}
	}
}

// TestAnErrorInASourcedFileEndsTheShellHere is the other side of the axis
// ksh93 and zsh answer the other way. Measured on bash 5.3.15, on the same
// binary under an argv[0] of `sh`, and on bash 3.2.57: the sourced file stops
// at the failure and so does everything above it, including the `echo` on the
// same line as the `.`.
func TestAnErrorInASourcedFileEndsTheShellHere(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.sh"),
		[]byte("echo IN-BEFORE\nset -u\necho X${NOPE}\necho IN-AFTER\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st := runBash(t, dir, ". ./p.sh\necho \"OUT-AFTER st=$?\"\n")
	const want = "IN-BEFORE\n" +
		"./p.sh: line 3: NOPE: unbound variable\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

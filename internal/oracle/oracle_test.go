// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
)

func TestNormalizeRemovesWhatIsTrueOfTheMachine(t *testing.T) {
	// Diagnostics quote the shell's own path and the script's path, both of
	// which differ between a laptop and a CI runner. Leaving them in would
	// make the golden record drift for everyone but its author.
	sh := Found{Shell: Shell{Name: "bash"}, Path: "/opt/homebrew/bin/bash"}
	dir := "/var/folders/xy/oracle-123"

	got := normalize("/opt/homebrew/bin/bash: line 1: oops\n", sh, dir)
	if strings.Contains(got, "homebrew") {
		t.Errorf("shell path survived normalization: %q", got)
	}
	if !strings.Contains(got, "<shell>") {
		t.Errorf("want <shell> placeholder, got %q", got)
	}

	if got := normalize(dir+"/case.sh: bad\n", sh, dir); !strings.Contains(got, "<script>") {
		t.Errorf("script path not normalized: %q", got)
	}
	if got := normalize(dir+"/other: bad\n", sh, dir); !strings.Contains(got, "<tmp>") {
		t.Errorf("temp dir not normalized: %q", got)
	}
}

func TestNormalizeFlattensNewlinesAndTrailing(t *testing.T) {
	sh := Found{Shell: Shell{Name: "x"}, Path: "/bin/x"}
	got := normalize("a\nb\n", sh, "/tmp/d")
	if got != "a~b" {
		t.Errorf("normalize = %q, want %q", got, "a~b")
	}
}

func TestCompareOnlyComparesShellsPresentInBoth(t *testing.T) {
	// CI and a laptop have different panels. Failing on a shell that only one
	// side has would make the check useless exactly where it is most wanted.
	golden := &Run{Results: map[string]map[string]Result{
		"c1": {"bash": {Stdout: "x"}, "ksh93": {Stdout: "y"}},
	}}
	now := &Run{Results: map[string]map[string]Result{
		"c1": {"bash": {Stdout: "x"}}, // ksh93 absent here
	}}
	if d := now.Compare(golden); len(d) != 0 {
		t.Errorf("missing shell reported as drift: %v", d)
	}
}

func TestCompareDetectsChangedOutputAndStatus(t *testing.T) {
	golden := &Run{Results: map[string]map[string]Result{
		"c1": {"bash": {Stdout: "x", Status: 0}},
		"c2": {"bash": {Stdout: "y", Status: 0}},
	}}
	now := &Run{Results: map[string]map[string]Result{
		"c1": {"bash": {Stdout: "CHANGED", Status: 0}},
		// Same output, different status is still a difference.
		"c2": {"bash": {Stdout: "y", Status: 1}},
	}}
	d := now.Compare(golden)
	if len(d) != 2 {
		t.Fatalf("want 2 drifts, got %d: %v", len(d), d)
	}
	if d[0].CaseID != "c1" || d[1].CaseID != "c2" {
		t.Errorf("drifts not sorted by case: %v", d)
	}
}

func TestNewCasesAreNotDrift(t *testing.T) {
	golden := &Run{Results: map[string]map[string]Result{"old": {"bash": {}}}}
	now := &Run{Results: map[string]map[string]Result{
		"old": {"bash": {}},
		"new": {"bash": {Stdout: "z"}},
	}}
	if d := now.Compare(golden); len(d) != 0 {
		t.Errorf("a new case has nothing to drift from, got %v", d)
	}
	got := now.NewCases(golden)
	if len(got) != 1 || got[0] != "new" {
		t.Errorf("NewCases = %v, want [new]", got)
	}
}

func TestCorpusIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Corpus {
		switch {
		case c.ID == "":
			t.Error("a case has no ID; the ID is the golden-file key")
		case seen[c.ID]:
			t.Errorf("duplicate case ID %q — one would silently overwrite the other", c.ID)
		case c.Snippet == "":
			t.Errorf("%s: no snippet", c.ID)
		case c.Category == "":
			t.Errorf("%s: no category", c.ID)
		case c.Why == "":
			// A case nobody can evaluate when it changes is worse than no
			// case, because it will be "fixed" by updating the golden file.
			t.Errorf("%s: no Why; it could not be judged when it drifts", c.ID)
		}
		seen[c.ID] = true
		// An argv that spells the snippet out is the drift #535 is about: two
		// copies of one text with nothing keeping them in step, so an edit to
		// either makes the case test something other than what it records.
		// There is no longer a reason to write it twice — a placeholder is
		// interpolated wherever it appears — so the second copy is now an
		// error rather than a documented cost.
		for _, a := range c.Args {
			if strings.Contains(a, c.Snippet) {
				t.Errorf("%s: argv %q spells the snippet out; write %q instead",
					c.ID, a, strings.ReplaceAll(a, c.Snippet, ArgSnippet))
			}
		}
		if err := c.validate(); err != nil {
			// A case whose invocation cannot be built records a harness error
			// in place of a measurement, which looks like a shell that
			// disagreed with everyone.
			t.Errorf("%s: %v", c.ID, err)
		}
	}
}

func TestArgsPlaceTheSnippetWhereTheCaseSaysAndKeepTheShellsFlagsFirst(t *testing.T) {
	// The binary under test is told which dialect to be by flags of its own
	// — `-dialect bash` — and those have to come before anything the case
	// spells out, because cmd/sh stops reading its own flags at the first
	// word that is not one.
	sh := Found{Shell: Shell{Name: "ours", Args: []string{"-dialect", "bash"}}, Path: "/bin/sh"}
	c := Case{ID: "t", Snippet: `echo hi`, Args: []string{"-o", "errexit", "-c", ArgSnippet, "name"}}

	cmd := command(t.Context(), sh, c, t.TempDir())
	want := []string{"/bin/sh", "-dialect", "bash", "-o", "errexit", "-c", "echo hi", "name"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("argv = %q, want %q", cmd.Args, want)
	}
}

func TestArgScriptWritesTheSnippetToTheFileTheNormalizerKnows(t *testing.T) {
	// The path has to be the one normalize() rewrites to <script>, or a case
	// whose diagnostic names the script would record the temp directory it
	// happened to run in and drift for everyone else.
	dir := t.TempDir()
	sh := Found{Shell: Shell{Name: "ours"}, Path: "/bin/sh"}
	c := Case{ID: "t", Snippet: `echo hi`, Args: []string{"-e", ArgScript, "a"}}

	cmd := command(t.Context(), sh, c, dir)
	want := []string{"/bin/sh", "-e", filepath.Join(dir, "case.sh"), "a"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("argv = %q, want %q", cmd.Args, want)
	}
	b, err := os.ReadFile(filepath.Join(dir, "case.sh"))
	if err != nil || string(b) != "echo hi\n" {
		t.Errorf("script file = %q, %v; want the snippet", b, err)
	}
	if got := normalize(filepath.Join(dir, "case.sh")+": bad\n", sh, dir); !strings.Contains(got, "<script>") {
		t.Errorf("the file ArgScript wrote does not normalize: %q", got)
	}
}

// TestAPlaceholderIsReplacedInsideAWordToo is the expressiveness #535 was
// about, and the reason the duplication it removes existed.
//
// `-c` takes its command string as its own word only when it is written that
// way. `sh -c'echo hi'` attaches the string to the letter — the shape a hand
// and a generated command line both produce, and the one all four panel
// shells refuse — and a whole-word placeholder cannot spell it. The only way
// left was to write the snippet text a second time, literally, in the argv.
func TestAPlaceholderIsReplacedInsideAWordToo(t *testing.T) {
	sh := Found{Shell: Shell{Name: "ours"}, Path: "/bin/sh"}
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "the command string attached to its letter",
			args: []string{"-c" + ArgSnippet},
			want: []string{"/bin/sh", "-cecho hi"},
		},
		{
			// The word it always worked in stays exactly as it was: this is
			// an addition and not a change of meaning.
			name: "a placeholder that is the whole word still works",
			args: []string{"-c", ArgSnippet},
			want: []string{"/bin/sh", "-c", "echo hi"},
		},
		{
			name: "text on both sides of it",
			args: []string{"before" + ArgSnippet + "after"},
			want: []string{"/bin/sh", "beforeecho hiafter"},
		},
		{
			// A word that names no placeholder is passed through untouched,
			// which is what makes the deliberate no-snippet shapes work.
			name: "a word with no placeholder is left alone",
			args: []string{"-s", "a"},
			want: []string{"/bin/sh", "-s", "a"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Case{ID: "t", Snippet: "echo hi", Args: tc.args}
			if err := c.validate(); err != nil {
				t.Fatalf("validate: %v", err)
			}
			cmd := command(t.Context(), sh, c, dir)
			if !slices.Equal(cmd.Args, tc.want) {
				t.Errorf("argv = %q, want %q", cmd.Args, tc.want)
			}
		})
	}
}

// TestArgScriptIsReplacedInsideAWordToo is the same rule for the other
// placeholder, and it checks the path is still the one the normalizer knows —
// an embedded path that did not normalize would put the machine's temp
// directory in the record.
func TestArgScriptIsReplacedInsideAWordToo(t *testing.T) {
	dir := t.TempDir()
	sh := Found{Shell: Shell{Name: "ours"}, Path: "/bin/sh"}
	c := Case{ID: "t", Snippet: "echo hi", Args: []string{"--rcfile=" + ArgScript}}

	cmd := command(t.Context(), sh, c, dir)
	want := []string{"/bin/sh", "--rcfile=" + filepath.Join(dir, "case.sh")}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("argv = %q, want %q", cmd.Args, want)
	}
	if got := normalize(cmd.Args[1], sh, dir); !strings.Contains(got, "<script>") {
		t.Errorf("the embedded path does not normalize: %q", got)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "case.sh")); err != nil || string(b) != "echo hi\n" {
		t.Errorf("script file = %q, %v; want the snippet", b, err)
	}
}

// TestTheScriptFileIsOnlyWrittenWhenACaseAsksForIt is why the replacement is
// guarded rather than unconditional, and it is a measurement bug and not
// tidiness.
//
// The scratch directory is the shell's *working* directory, so a file written
// into it is a file the snippet can see: several cases glob the current
// directory, and `echo *` would list a case.sh nobody asked for. Replacing
// ArgScript unconditionally would write one for every case that uses Args.
func TestTheScriptFileIsOnlyWrittenWhenACaseAsksForIt(t *testing.T) {
	sh := Found{Shell: Shell{Name: "ours"}, Path: "/bin/sh"}
	dir := t.TempDir()

	command(t.Context(), sh, Case{ID: "t", Snippet: "echo *", Args: []string{"-c" + ArgSnippet}}, dir)
	if _, err := os.Stat(filepath.Join(dir, "case.sh")); !os.IsNotExist(err) {
		t.Errorf("case.sh exists in the working directory of a case that never named it (%v); a snippet that globs would see it", err)
	}
	// The control, so this cannot pass by the file never being written at all.
	other := t.TempDir()
	command(t.Context(), sh, Case{ID: "t", Snippet: "echo hi", Args: []string{ArgScript}}, other)
	if _, err := os.Stat(filepath.Join(other, "case.sh")); err != nil {
		t.Errorf("case.sh missing for a case that did name it: %v", err)
	}
}

// TestAPlaceholderNamedTwiceIsRefusedInsideAWordToo keeps validate's rule in
// step with the replacement rule.
//
// Counting whole words while replacing substrings would let a case name the
// snippet's place twice and be told it had named it once — the harness error
// exists so that a case which quietly ran something other than what it says
// cannot be measured, and it has to count what is actually replaced.
func TestAPlaceholderNamedTwiceIsRefusedInsideAWordToo(t *testing.T) {
	for _, args := range [][]string{
		{"-c" + ArgSnippet, ArgSnippet},
		{"-c" + ArgSnippet + ArgSnippet},
		{"-c" + ArgSnippet, ArgScript},
		{ArgSnippet, ArgSnippet},
	} {
		c := Case{ID: "t", Snippet: "echo hi", Args: args}
		if err := c.validate(); err == nil {
			t.Errorf("validate(%q) = nil; want a harness error: the snippet's place is named twice", args)
		}
	}
	// And the one that is still legal, so the rule is not simply "refuse".
	c := Case{ID: "t", Snippet: "echo hi", Args: []string{"-c" + ArgSnippet, "name"}}
	if err := c.validate(); err != nil {
		t.Errorf("validate: %v; naming the place once is what the field is for", err)
	}
}

func TestArgsWithNoPlaceholderDoNotHandOverTheSnippet(t *testing.T) {
	// The shape an invocation that fails before it reads anything needs — a
	// script path that does not exist — and the shape Case.Stdin needs, so
	// that the program can arrive on standard input instead.
	// The snippet is still written down, as the thing that would have run.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	c := Case{ID: "t", Snippet: `echo the-snippet`, Args: []string{"-c", "echo the-argv"}}

	got := Exec(context.Background(), found[0], c)
	if got.Stdout != "the-argv" {
		t.Errorf("Stdout = %q, want %q: Args are the whole invocation", got.Stdout, "the-argv")
	}
}

func TestStdinReachesBothShellsIdentically(t *testing.T) {
	// The point of the field. A case that fed only one side would record the
	// difference between two inputs and call it a difference between two
	// shells.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	ref := found[0]
	ours := Found{Shell: Shell{Name: "ours"}, Path: ref.Path}
	c := Case{ID: "t", Snippet: `read a; read b; echo "[$a][$b]"`, Stdin: "one\ntwo\n"}

	a, b := Exec(context.Background(), ref, c), Exec(context.Background(), ours, c)
	if a != b {
		t.Errorf("same case, different input: %q vs %q", a.Stdout, b.Stdout)
	}
	if a.Stdout != "[one][two]" {
		t.Errorf("Stdout = %q, want %q", a.Stdout, "[one][two]")
	}
}

func TestNoStdinLeavesTheShellWithNothingToRead(t *testing.T) {
	// The default has to stay closed rather than inherited. A harness that
	// passed its own input through would let the *test binary's* standard
	// input decide what a case measured, which is a wrong measurement that
	// looks like a shell.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	got := Exec(context.Background(), found[0], Case{ID: "t", Snippet: `read a; echo "st=$?|[$a]"`})
	if got.Stdout != "st=1|[]" {
		t.Errorf("Stdout = %q, want %q: a case with no Stdin reads nothing", got.Stdout, "st=1|[]")
	}
}

func TestStdinCarriesTheProgramWhenArgsNameNoPlaceholder(t *testing.T) {
	// The combination that makes the standard-input invocation route
	// reachable: nothing on the argv hands the shell its snippet, so the
	// snippet has to arrive on the input, and $0 stays the shell.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	sh := found[0]
	c := Case{ID: "t", Snippet: `echo "0=[$0]|n=$#"`, Args: []string{"--"}, Stdin: ArgSnippet + "\n"}

	if got := Exec(context.Background(), sh, c); got.Stdout != "0=[<shell>]|n=0" {
		t.Errorf("Stdout = %q, want %q", got.Stdout, "0=[<shell>]|n=0")
	}
}

func TestStdinPlacesTheScriptPathTheNormalizerKnows(t *testing.T) {
	// ArgScript means the same file in Stdin as it does in Args, so a case
	// can name the script from its input without hard-coding a path that
	// would differ on every machine.
	dir := t.TempDir()
	sh := Found{Shell: Shell{Name: "ours"}, Path: "/bin/sh"}
	c := Case{ID: "t", Snippet: `echo hi`, Args: []string{"-s"}, Stdin: ". " + ArgScript + "\n"}

	cmd := command(t.Context(), sh, c, dir)
	in, err := io.ReadAll(cmd.Stdin)
	if err != nil {
		t.Fatalf("reading the case's input: %v", err)
	}
	want := ". " + filepath.Join(dir, "case.sh") + "\n"
	if string(in) != want {
		t.Errorf("stdin = %q, want %q", in, want)
	}
	b, err := os.ReadFile(filepath.Join(dir, "case.sh"))
	if err != nil || string(b) != "echo hi\n" {
		t.Errorf("script file = %q, %v; want the snippet", b, err)
	}
}

func TestArgsAndScriptAreRefusedTogether(t *testing.T) {
	// Not a precedence rule: a case that quietly ran something other than
	// what it says would still be measured, and the measurement is the
	// product.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	for _, c := range []Case{
		{ID: "both", Snippet: "echo hi", Script: true, Args: []string{ArgScript}},
		{ID: "twice", Snippet: "echo hi", Args: []string{"-c", ArgSnippet, ArgScript}},
	} {
		got := Exec(context.Background(), found[0], c)
		// On standard error, and nothing on standard output: the harness's
		// own failure must not read as something a shell printed.
		if !strings.HasPrefix(got.Stderr, "harness error:") || got.Stdout != "" || got.Status != -1 {
			t.Errorf("%s: Exec = out %q err %q (status %d), want a harness error on stderr",
				c.ID, got.Stdout, got.Stderr, got.Status)
		}
	}
}

func TestArgsRunTheSameInvocationOnBothSides(t *testing.T) {
	// The point of the field. A reference shell and the binary under test
	// have to be handed the same words, or a conformance run grades two
	// different invocations and reports the difference as a bug.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	ref := found[0]
	ours := Found{Shell: Shell{Name: "ours"}, Path: ref.Path}
	c := Case{ID: "t", Snippet: `echo "$0|$#"`, Args: []string{"-c", ArgSnippet, "name", "a"}}

	if a, b := Exec(context.Background(), ref, c), Exec(context.Background(), ours, c); a != b {
		t.Errorf("same case, different invocations: %q vs %q", a.Stdout, b.Stdout)
	}
}

func TestExecRunsASnippetAndCapturesStatus(t *testing.T) {
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	sh := found[0]

	got := Exec(context.Background(), sh, Case{ID: "t", Snippet: `printf hi; exit 3`})
	if got.Stdout != "hi" {
		t.Errorf("Stdout = %q, want %q", got.Stdout, "hi")
	}
	if got.Status != 3 {
		t.Errorf("Status = %d, want 3", got.Status)
	}
}

func TestExecKeepsTheTwoStreamsApart(t *testing.T) {
	// The regression a merged capture cannot see. A diagnostic that moves
	// from standard error to standard output is a behavior change, and
	// cmd.CombinedOutput() recorded the two as the same result — which is
	// why the harness could not back up its own claim that a shell printing
	// to the other stream has not behaved the same way.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	sh := found[0]

	got := Exec(context.Background(), sh, Case{ID: "t", Snippet: `echo out; echo err >&2`})
	if got.Stdout != "out" || got.Stderr != "err" {
		t.Errorf("Exec = out %q err %q, want out %q err %q", got.Stdout, got.Stderr, "out", "err")
	}
	moved := Exec(context.Background(), sh, Case{ID: "t", Snippet: `echo out >&2; echo err`})
	if moved == got {
		t.Error("two snippets differing only in which stream they wrote to recorded the same result")
	}
}

func TestExecNormalizesStandardErrorToo(t *testing.T) {
	// Nearly everything worth normalizing — the shell naming itself, the
	// script's path — arrives on standard error, so a split that normalized
	// only standard output would put the machine straight back into the
	// record.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	got := Exec(context.Background(), found[0], Case{ID: "t", Snippet: `nosuchcmd-xyz`})
	if got.Stdout != "" {
		t.Errorf("Stdout = %q, want nothing: the diagnostic belongs on the other stream", got.Stdout)
	}
	if !strings.HasPrefix(got.Stderr, "<shell>:") {
		t.Errorf("Stderr = %q, want a normalized diagnostic naming <shell>", got.Stderr)
	}
}

func TestCellNamesTheStreamAMessageCameOutOn(t *testing.T) {
	// The convention docs/spec/measurements.md is read with. The marker is
	// bold, and therefore outside the code span, so a shell that prints the
	// characters `2>` cannot be mistaken for the harness saying "stderr".
	for _, tc := range []struct {
		in   Result
		want string
	}{
		{Result{Stdout: "a"}, "`a`"},
		{Result{Stderr: "oops", Status: 1}, "**2>** `oops` *(status 1)*"},
		{Result{Stdout: "a", Stderr: "oops", Status: 1}, "`a` **2>** `oops` *(status 1)*"},
		{Result{Status: 2}, "*(no output, status 2)*"},
		{Result{TimedOut: true, Status: -1}, "*(timeout)*"},
		// A signal death is named rather than given the -1 that stands for
		// the exit status it does not have. The word comes out of the record
		// beside the number, so a cell reads the same on any machine (#776) —
		// which is why these two spell it rather than leaving it derived.
		{
			Result{Status: -1, Signal: syscall.SIGTERM, SignalName: "terminated"},
			"*(no output, killed by signal 15 (terminated))*",
		},
		{
			Result{Stdout: "a", Status: -1, Signal: syscall.SIGINT, SignalName: "interrupt"},
			"`a` *(killed by signal 2 (interrupt))*",
		},
	} {
		if got := cell(tc.in); got != tc.want {
			t.Errorf("cell(%+v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestExecRecordsWhichSignalKilledTheShell: an exit status and a signal death
// are alternatives in a wait status, so Go answers -1 for the exit status of a
// process a signal ended. Without the signal beside it every death looked the
// same in the record, and a shell's whole exit-on-signal discipline — dying of
// the signal rather than exiting with 128 plus its number — was unrecordable.
//
// The shell kills *itself*, so nothing here touches this process's own signal
// dispositions: those are process-wide and survive an exec, and changing one
// in a test has broken unrelated runs before.
func TestExecRecordsWhichSignalKilledTheShell(t *testing.T) {
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	for _, tc := range []struct {
		name    string
		snippet string
		want    syscall.Signal
	}{
		{"terminated", `kill -TERM $$`, syscall.SIGTERM},
		// A second one, because a field that always held the same number
		// would pass a test that only ever asked about one signal.
		{"killed outright", `kill -KILL $$`, syscall.SIGKILL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Exec(context.Background(), found[0], Case{ID: "t", Snippet: tc.snippet})
			if got.Signal != tc.want {
				t.Errorf("Signal = %v, want %v", got.Signal, tc.want)
			}
			if got.Status != -1 {
				t.Errorf("Status = %d, want the -1 that says there is no exit status", got.Status)
			}
			// The word beside the number, taken from the kernel that
			// produced it rather than from whoever renders it later (#776).
			// Asserted against this machine's own spelling, because this
			// machine is the one measuring: the point of recording it is
			// that a *reader* elsewhere no longer computes it.
			if want := tc.want.String(); got.SignalName != want {
				t.Errorf("SignalName = %q, want this kernel's %q", got.SignalName, want)
			}
		})
	}
	// And a shell that ends by itself carries no signal, so zero really does
	// mean "not killed" rather than "not recorded".
	got := Exec(context.Background(), found[0], Case{ID: "t", Snippet: `exit 3`})
	if got.Signal != 0 || got.Status != 3 {
		t.Errorf("an ordinary exit = status %d signal %v, want status 3 and no signal", got.Status, got.Signal)
	}
	// And no word either: a name beside no signal would be a name for
	// signal 0, which is the argument to kill(2) that sends nothing.
	if got.SignalName != "" {
		t.Errorf("an ordinary exit carries the name %q", got.SignalName)
	}
}

// TestATimeoutIsNotASignalDeath: both answer -1 for the exit status, which is
// how a shell that hung could be recorded as one that was killed. The signal
// is what tells them apart, and it is what a case grading an implementation
// compares.
func TestATimeoutIsNotASignalDeath(t *testing.T) {
	timedOut := Result{Status: -1, TimedOut: true}
	killed := Result{Status: -1, Signal: syscall.SIGTERM}
	if timedOut == killed {
		t.Fatal("a timeout and a signal death are the same record")
	}
	if describe(timedOut) == describe(killed) {
		t.Errorf("both read as %q", describe(killed))
	}
}

// TestConformanceGradesHowARunEnded: three ways of not having an exit status
// — killed by one signal, killed by another, and never finishing — agree on
// every other field, so grading on the status alone scores them as the same
// behavior.
func TestConformanceGradesHowARunEnded(t *testing.T) {
	term := Result{Status: -1, Signal: syscall.SIGTERM}
	for _, tc := range []struct {
		name  string
		other Result
		want  bool
	}{
		{"the same death", Result{Status: -1, Signal: syscall.SIGTERM}, true},
		{"a different signal", Result{Status: -1, Signal: syscall.SIGINT}, false},
		{"a run that never finished", Result{Status: -1, TimedOut: true}, false},
		{"an exit that says the same number", Result{Status: -1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.other.Stdout != term.Stdout || tc.other.Stderr != term.Stderr ||
				tc.other.Status != term.Status {
				t.Fatal("the fixture no longer isolates how the run ended")
			}
			if got := matches(term, tc.other); got != tc.want {
				t.Errorf("matches = %v, want %v", got, tc.want)
			}
			if got := sameOutcome(term, tc.other); got != tc.want {
				t.Errorf("sameOutcome = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExecDoesNotInheritTheDevelopersEnvironment(t *testing.T) {
	// IFS or HOME leaking in from whoever ran the harness would make the
	// record depend on their shell rather than on the shell under test.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	t.Setenv("IFS", ":")
	t.Setenv("ORACLE_LEAK_CANARY", "leaked")

	got := Exec(context.Background(), found[0], Case{ID: "t", Snippet: `echo "[${ORACLE_LEAK_CANARY-clean}]"`})
	if got.Stdout != "[clean]" {
		t.Errorf("environment leaked into the run: %q", got.Stdout)
	}
}

// ignoreTheWayNohupDoes puts the process in the state a caller that ignores
// signals hands the harness, and puts it back afterwards. SIG_IGN is what
// survives exec — that is the whole of what `nohup` does — so this is the
// real condition rather than an approximation of it.
func ignoreTheWayNohupDoes(t *testing.T, sigs ...syscall.Signal) {
	t.Helper()
	for _, sig := range sigs {
		signal.Ignore(sig)
		// Put it back the way the harness leaves it — taken over — rather
		// than with signal.Reset. Reset after a Notify restores the
		// disposition the *process started with*, and Go reports the result
		// as not ignored while leaving SIG_IGN in place at the operating
		// system, so a shell a later test runs still sees `trap -- '' SIGHUP`
		// and no amount of asking would have said so. That cost an afternoon
		// once, as a failure in the test that ran next.
		t.Cleanup(func() { signal.Notify(ignoredSink, sig) })
	}
}

func TestExecDoesNotInheritTheCallersSignalDispositions(t *testing.T) {
	// The sibling of the environment scrub, by a route that is easier to
	// miss. An ignored disposition survives exec, so a harness launched under
	// `nohup` hands every shell it measures a SIGHUP that is already ignored:
	// the shell reports `trap -- '' SIGHUP` and outlives `kill -HUP $$` to
	// print what came after. That makes the conformance number a function of
	// how the harness was launched, and two runs that disagree read as a
	// flaky implementation.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	// SIGHUP is the one `nohup` sets, and SIGQUIT is one a background job in
	// a non-interactive shell gets. The job-control four are deliberately not
	// here: an ignore this process sets is reported, so they would pass
	// without saying anything about the case that actually leaks — which
	// TestTheJobControlSignalsAreAKnownGapAndStillAre measures instead.
	ignoreTheWayNohupDoes(t, syscall.SIGHUP, syscall.SIGQUIT)

	for _, sh := range found {
		t.Run(sh.Name, func(t *testing.T) {
			got := Exec(context.Background(), sh, Case{ID: "t", Snippet: `trap`})
			if got.Stdout != "" {
				t.Errorf("the caller's dispositions reached the shell: trap said %q", got.Stdout)
			}
			// The sharper half: reporting it is a wording, outliving it is a
			// different measurement. Every shell in the panel either dies of
			// an untrapped hangup or exits on it, and none of them prints.
			got = Exec(context.Background(), sh, Case{ID: "t", Snippet: `kill -HUP $$; echo after`})
			if got.Stdout != "" {
				t.Errorf("the shell survived a hangup it should not have: %q", got.Stdout)
			}
		})
	}
}

func TestTheHarnessKeepsIgnoringWhatItsCallerIgnored(t *testing.T) {
	// The scrub is allowed to change what the *children* inherit and nothing
	// else. A caller that said "ignore hangups" said it about this process
	// too, and a harness that started dying of them under `nohup` would have
	// traded one launch-dependent behavior for a worse one. If this is wrong
	// the test binary is killed rather than failed, which is as loud as it
	// gets.
	ignoreTheWayNohupDoes(t, syscall.SIGHUP)
	scrubSignalDispositions()
	if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
		t.Fatalf("kill: %v", err)
	}
	if signal.Ignored(syscall.SIGHUP) {
		t.Error("the scrub left the disposition at SIG_IGN, so a child would still inherit it")
	}
}

func TestTheScrubDisarmsItselfAndLeavesTheRestAlone(t *testing.T) {
	// Nothing guards the scrub against being run per case, because it needs
	// no guard: a signal it has taken over stops being reported as ignored.
	// Running it every time is what lets it catch a disposition that arrives
	// after the first measurement.
	ignoreTheWayNohupDoes(t, syscall.SIGHUP)

	first := scrubSignalDispositions()
	if len(first) != 1 || first[0] != syscall.SIGHUP {
		t.Fatalf("took over %v, want only SIGHUP — a signal the caller left alone must not be touched", first)
	}
	if again := scrubSignalDispositions(); len(again) != 0 {
		t.Errorf("took over %v on a second call; the loop did not disarm", again)
	}
}

func TestAJobControlSignalIsCoveredWhereTheRuntimeReportsIt(t *testing.T) {
	// The four job-control signals are listed with the rest even though an
	// *inherited* ignore for them is never reported, so nothing more has to
	// be done the day a Go release starts reporting one. This exercises the
	// state where it already is reported — an ignore this process set itself
	// — which is the only way to reach that path today.
	found, _ := Resolve(context.Background())
	if len(found) == 0 {
		t.Skip("no reference shells on this machine")
	}
	ignoreTheWayNohupDoes(t, syscall.SIGTSTP)
	if !signal.Ignored(syscall.SIGTSTP) {
		t.Fatal("could not set up the state this test measures")
	}
	for _, sh := range found {
		got := Exec(context.Background(), sh, Case{ID: "t", Snippet: `trap`})
		if got.Stdout != "" {
			t.Errorf("%s: a reported job-control disposition reached the shell: %q", sh.Name, got.Stdout)
		}
	}
	// Asked of the runtime as well as of the panel, because three of the six
	// shells do not report an inherited ignore through `trap` and a machine
	// with only those would pass the loop above without measuring anything.
	if signal.Ignored(syscall.SIGTSTP) {
		t.Error("the scrub left SIGTSTP ignored, so a child would still inherit it")
	}
}

// jobControlChildEnv marks the re-executed half of the test below.
const jobControlChildEnv = "ORACLE_JOB_CONTROL_CHILD"

func TestJobControlGapChild(t *testing.T) {
	if os.Getenv(jobControlChildEnv) != "1" {
		t.Skip("the child half of TestTheJobControlSignalsAreAKnownGapAndStillAre")
	}
	// The launcher ignored these four before this binary started. Whether the
	// runtime admits it is the whole question.
	for _, sig := range []syscall.Signal{syscall.SIGTSTP, syscall.SIGTTIN, syscall.SIGTTOU} {
		fmt.Printf("job-control gap: %v reported-as-ignored=%v\n", sig, signal.Ignored(sig))
	}
}

func TestTheJobControlSignalsAreAKnownGapAndStillAre(t *testing.T) {
	// An inverted test: it pins a limit rather than a fix, so the limit cannot
	// quietly stop being true.
	//
	// The Go runtime keeps an inherited SIG_IGN for SIGTSTP, SIGTTIN, SIGTTOU
	// and SIGCONT — stopping a process that was started with stopping turned
	// off would be wrong — and does not report that it has, so the scrub
	// cannot detect the condition. They are listed with the rest anyway, so
	// the day a Go release starts reporting them nothing more is needed. This
	// fails on that day, which is the notice to delete it.
	//
	// It asks the runtime and not a shell. An earlier version measured the
	// panel instead — does any shell still see the ignore — and was red on a
	// macOS runner for a legitimate reason: bash 3.2, dash and ksh93 do not
	// report an inherited ignore through `trap` at all, so the answer was
	// about which shells happened to be installed rather than about Go. A
	// check that is red for a legitimate reason is one people learn to
	// ignore.
	//
	// The disposition has to come from a launcher: signal.Ignore records the
	// runtime's own state and signal.Ignored answers truthfully about it
	// afterwards, so the interesting case cannot be built in this process.
	exe, err := os.Executable()
	if err != nil {
		t.Skip("cannot find this test binary to re-execute")
	}
	cmd := exec.Command("/bin/sh", "-c",
		`trap '' TSTP TTIN TTOU; exec "$1" -test.run='^TestJobControlGapChild$' -test.v`, "sh", exe)
	cmd.Env = append(os.Environ(), jobControlChildEnv+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the child failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "job-control gap:") {
		t.Fatalf("the child skipped instead of measuring anything:\n%s", out)
	}
	if !strings.Contains(string(out), "reported-as-ignored=false") {
		t.Errorf("the runtime now reports an inherited ignore for the job-control signals: "+
			"the gap this works around is closed, so delete this test\n%s", out)
	}
}

func TestMarkdownEndsWithExactlyOneNewline(t *testing.T) {
	// The generated file is committed, so end-of-file-fixer rewrites it if it
	// ends in a blank line — which fails the commit on every regeneration.
	r := &Run{
		Shells:  []ShellRecord{{Name: "bash", Version: "5"}},
		Results: map[string]map[string]Result{"c1": {"bash": {Stdout: "x"}}},
	}
	got := r.Markdown([]Case{{ID: "c1", Category: "cat", Snippet: "echo x", Why: "because"}})
	if !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("want exactly one trailing newline, got %q", got[max(0, len(got)-12):])
	}
}

func TestNormalizeDoesNotEatShellNameInsideWords(t *testing.T) {
	// dash's basename is "sh", and replacing it anywhere turned
	// "can't shift that many" into "can't <shell>ift that many".
	sh := Found{Shell: Shell{Name: "bash-as-sh"}, Path: "/bin/sh"}
	got := normalize("sh: 1: shift: can't shift that many\n", sh, "/tmp/d")
	want := "<shell>: 1: shift: can't shift that many"
	if got != want {
		t.Errorf("normalize = %q, want %q", got, want)
	}
}

func TestResolveRejectsAPathThatIsNotTheShellItNames(t *testing.T) {
	// /bin/sh is bash on macOS and dash on Debian. A column labeled
	// bash-as-sh that actually ran dash is worse than a missing column,
	// because nothing about it looks wrong.
	var entry Shell
	for _, s := range Panel {
		if s.Name == "bash-as-sh" {
			entry = s
		}
	}
	if entry.MustReport == "" {
		t.Fatal("the bash-as-sh panel entry must assert what it expects to find")
	}

	found, missing := Resolve(context.Background())
	for _, f := range found {
		if f.MustReport != "" && !strings.Contains(strings.ToLower(f.Version), f.MustReport) {
			t.Errorf("%s resolved to %s reporting %q, which does not contain %q",
				f.Name, f.Path, f.Version, f.MustReport)
		}
	}
	_ = missing
}

func TestNormalizeUsesTheNameTheShellWasInvokedUnder(t *testing.T) {
	// bash invoked as sh reports "sh:" in diagnostics, not "bash:", so
	// normalizing only the binary's basename leaves the machine-specific
	// name in the record.
	sh := Found{Shell: Shell{Name: "bash-as-sh", Argv0: "sh"}, Path: "/opt/homebrew/bin/bash"}
	got := normalize("sh: -c: line 1: syntax error\n", sh, "/tmp/d")
	if !strings.HasPrefix(got, "<shell>:") {
		t.Errorf("argv0 name not normalized: %q", got)
	}
}

// TestEveryRouteIsInvokedUnderTheNameThePanelGivesIt guards the one thing
// that decides which shell a column is measuring.
//
// The script route did not honor Argv0, so the column headed bash-as-sh held
// plain bash for every case that runs from a file — a mislabeled column of
// exactly the kind MustReport was added to prevent, and one that looks
// entirely well from the outside. The routes are enumerated rather than
// spot-checked because a fourth one would otherwise arrive with the same hole
// and nothing would say so.
func TestEveryRouteIsInvokedUnderTheNameThePanelGivesIt(t *testing.T) {
	sh := Found{Shell: Shell{Name: "bash-as-sh", Argv0: "sh"}, Path: "/opt/homebrew/bin/bash"}
	for _, tc := range []struct {
		route string
		c     Case
	}{
		{"-c", Case{ID: "t", Snippet: "echo hi"}},
		{"script", Case{ID: "t", Snippet: "echo hi", Script: true}},
		{"args", Case{ID: "t", Snippet: "echo hi", Args: []string{"-c", ArgSnippet}}},
		{"args-script", Case{ID: "t", Snippet: "echo hi", Args: []string{ArgScript}}},
	} {
		t.Run(tc.route, func(t *testing.T) {
			cmd := command(t.Context(), sh, tc.c, t.TempDir())
			if cmd.Args[0] != "sh" {
				t.Errorf("argv[0] = %q, want %q: the %s route is measuring a shell other than the one its column names",
					cmd.Args[0], "sh", tc.route)
			}
			if cmd.Path != sh.Path {
				t.Errorf("path = %q, want %q: argv[0] names the shell, it does not choose the binary", cmd.Path, sh.Path)
			}
		})
	}
}

// TestArgv0ReachesTheBinaryOnTheScriptRoute is the same claim measured rather
// than inspected: the argv the harness builds is only worth checking because a
// real shell reads it.
//
// A readonly reassignment is the sharpest probe available. bash keeps going
// and prints "survived"; the same binary called sh treats the failed special
// builtin as fatal and stops with status 1. Nothing about the snippet says
// which — only argv[0] does, which is what makes it a test of this and of
// nothing else.
func TestArgv0ReachesTheBinaryOnTheScriptRoute(t *testing.T) {
	found, _ := Resolve(t.Context())
	var asSh, plain Found
	for _, f := range found {
		switch f.Name {
		case "bash-as-sh":
			asSh = f
		case "bash":
			plain = f
		}
	}
	if asSh.Path == "" || plain.Path == "" {
		t.Skip("bash is not on this machine, so there is nothing to invoke under two names")
	}
	c := Case{ID: "t", Snippet: "readonly r=1\nr=2\necho survived", Script: true}

	got := Exec(t.Context(), asSh, c)
	if got.Status != 1 || strings.Contains(got.Stdout, "survived") {
		t.Errorf("bash-as-sh: status %d, stdout %q; want status 1 and no 'survived' — a failed special builtin is fatal in sh mode",
			got.Status, got.Stdout)
	}
	// And the control: the same binary, the same script, the other name. If
	// this stopped disagreeing, the probe would have stopped measuring argv[0]
	// and would pass for the wrong reason.
	if ref := Exec(t.Context(), plain, c); ref.Status != 0 || !strings.Contains(ref.Stdout, "survived") {
		t.Errorf("bash: status %d, stdout %q; want status 0 and 'survived' — the probe no longer distinguishes the two names",
			ref.Status, ref.Stdout)
	}
}

// TestGradedOnRefusalCasesAreActuallyRefused is the guard that keeps the
// relaxation from becoming a way to pass.
//
// The mode forgives the wording of a diagnostic, which is only defensible on a
// case where every shell genuinely declines. Put the flag on a case the panel
// *runs* and it would forgive an ordinary difference instead — so the claim
// the flag makes is checked here against the golden record, which is checked
// in. That costs no shells and runs everywhere, and it fails at the moment
// someone marks the wrong case rather than at the moment a score quietly
// improves.
//
// The record is the right authority for it. matchesRefusal already refuses to
// pass a case whose reference did not refuse, so a misused flag cannot make
// conformance green; what it could do without this is sit in the corpus
// looking like a claim that had been checked.
//
// # Why declined() is accepted here and not by matchesRefusal
//
// The panel stopped being unanimous about what a refusal *looks like* when
// ash joined it. Measured by hand against the pinned image, not inferred:
//
//	$ ash -o nosuchoption -c 'echo hi'
//	/bin/ash: illegal option -o nosuchoption
//	$ echo $?
//	0
//
// BusyBox declines the option, does not run the command string, and exits
// zero anyway. So refused() — nonzero *and* a diagnostic — is false for a
// column that plainly did not run the case, and the guard would fail on a
// correctly marked row.
//
// What the guard is protecting is that no column *ran* the snippet, because a
// case the panel runs would have the flag forgiving an ordinary difference.
// Empty standard output with a diagnostic on standard error says that
// precisely: every snippet in this corpus prints something when it runs, and
// this one would print `hi`.
//
// The grading rule is deliberately not widened to match. matchesRefusal still
// demands refused() on both sides, so against the ash column this case fails
// rather than being forgiven — which is the right answer and the reason it is
// safe to relax the *guard* alone.
func TestGradedOnRefusalCasesAreActuallyRefused(t *testing.T) {
	golden, err := Load(filepath.Join("testdata", "golden.json"))
	if err != nil {
		t.Fatalf("golden record: %v", err)
	}
	marked := 0
	for _, c := range Corpus {
		if !c.GradedOnRefusal {
			continue
		}
		marked++
		row, ok := golden.Results[c.ID]
		if !ok {
			// A brand new case has nothing recorded yet; `make oracle` is what
			// puts it there, and until then there is nothing to check it
			// against.
			continue
		}
		for _, shell := range golden.Shells {
			res, ok := row[shell.Name]
			if !ok {
				continue
			}
			if !refused(res) && !declined(res) {
				t.Errorf("%s is GradedOnRefusal but %s ran it: %s\n"+
					"\tthe flag forgives the wording of a refusal, so a case the panel runs must not carry it",
					c.ID, shell.Name, describe(res))
			}
		}
	}
	if marked == 0 {
		t.Error("no case carries GradedOnRefusal; the mode has no coverage and this guard proves nothing")
	}
}

// TestARefusalIsNotForgivenUnlessBothSidesRefused fixes the boundary of the
// relaxation, which is the whole of its safety.
//
// Every row here is a case the exact comparison would fail. The question is
// which of them the refusal mode should *also* fail, and the answer is all but
// one: it forgives a wording, and nothing else.
func TestARefusalIsNotForgivenUnlessBothSidesRefused(t *testing.T) {
	refusal := Result{Stdout: "", Stderr: "sh: -c: option requires an argument", Status: 2}
	for _, tc := range []struct {
		name string
		want Result
		got  Result
		ok   bool
	}{
		{
			// The case the mode exists for: two shells that declined alike
			// and said so in their own words.
			name: "same refusal, different words",
			want: refusal,
			got:  Result{Stderr: "our-sh: -c needs an operand", Status: 2},
			ok:   true,
		},
		{
			// The bug the motivating case was written to catch. Running what
			// the reference declined to run is not a wording difference.
			name: "we ran what the reference refused",
			want: refusal,
			got:  Result{Stdout: "hi", Status: 0},
		},
		{
			// A silent failure has not diagnosed anything, and "it complained
			// on stderr" is half the claim being graded.
			name: "we refused without saying anything",
			want: refusal,
			got:  Result{Status: 2},
		},
		{
			// The status is still exact. Two shells that refuse with
			// different numbers have not behaved the same way.
			name: "refused with a different status",
			want: refusal,
			got:  Result{Stderr: "our-sh: no", Status: 1},
		},
		{
			// Standard output is still exact, which is what stops the mode
			// forgiving a stream of output nobody produced — bash's `set -o`
			// table on the attached-command-string case is exactly this.
			name: "refused alike but printed something extra",
			want: Result{Stdout: "allexport\toff", Stderr: "sh: - : invalid option", Status: 1},
			got:  Result{Stderr: "our-sh: - : invalid option", Status: 1},
		},
		{
			// The flag on a case that is not a refusal. It fails rather than
			// passing loosely: a misused flag has to be louder than a correct
			// one, not quieter.
			name: "the reference did not refuse at all",
			want: Result{Stdout: "hi", Status: 0},
			got:  Result{Stdout: "different", Status: 0},
		},
		{
			// The same requirement where it is the only one left doing work.
			// Once the outcomes match, "both refused" reduces to "both wrote
			// a diagnostic" — so the case that separates the two halves is a
			// reference that failed *silently*, which is an ordinary failure
			// and not a refusal. Complaining where the reference did not is a
			// divergence, and forgiving it would be the misused-flag hole
			// with the reference-side check taken out.
			name: "the reference failed without diagnosing anything",
			want: Result{Status: 1},
			got:  Result{Stderr: "our-sh: no", Status: 1},
		},
		{
			// A shell that never finished declined nothing, and a case that
			// hangs has stopped measuring. Both sides carry a diagnostic on
			// purpose: without one the outcome check alone would reject this,
			// and the row would not be testing the timeout clause at all.
			name: "a timeout is not a refusal",
			want: Result{TimedOut: true, Status: -1, Stderr: "sh: still going"},
			got:  Result{TimedOut: true, Status: -1, Stderr: "our-sh: still going"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Case{ID: "t", GradedOnRefusal: true}
			if matches(tc.want, tc.got) {
				t.Fatal("the exact comparison already passes this, so it says nothing about the relaxation")
			}
			ok, relaxed := verdict(c, tc.want, tc.got)
			if ok != tc.ok {
				t.Errorf("verdict = %v, want %v", ok, tc.ok)
			}
			if relaxed != tc.ok {
				t.Errorf("relaxed = %v, want %v: a pass here is only ever a relaxed one", relaxed, tc.ok)
			}
			// And the flag is the only thing that grants it: the same two
			// results without it must fail, or the mode is not opt-in.
			if ok, _ := verdict(Case{ID: "t"}, tc.want, tc.got); ok {
				t.Error("passed without GradedOnRefusal; the relaxation is not opt-in")
			}
		})
	}
}

// TestAnExactMatchIsNeverCountedAsRelaxed keeps the discount honest.
//
// The count has to mean "this much of the score is currently being forgiven".
// Counting every flagged case would make it measure how the corpus is labeled
// instead, and it would grow when a dialect got *better* — the number moving
// the wrong way at the moment the gap closes.
func TestAnExactMatchIsNeverCountedAsRelaxed(t *testing.T) {
	same := Result{Stderr: "sh: -c: option requires an argument", Status: 2}
	ok, relaxed := verdict(Case{ID: "t", GradedOnRefusal: true}, same, same)
	if !ok || relaxed {
		t.Errorf("verdict = (%v, %v), want (true, false): an exact match needs no relaxation", ok, relaxed)
	}
}

// TestTheRecordMarksARelaxedRow is the visibility requirement, which is the
// condition the mode was allowed on.
//
// A reader of the generated tables has to be able to tell which rows are
// graded loosely. Without that, the objection to the mode stands — it would be
// indistinguishable from a score that is quietly wrong.
func TestTheRecordMarksARelaxedRow(t *testing.T) {
	cases := []Case{
		{ID: "loose", Category: "invocation", Snippet: "echo hi", GradedOnRefusal: true},
		{ID: "strict", Category: "invocation", Snippet: "echo hi"},
	}
	run := &Run{
		Shells:  []ShellRecord{{Name: "dash", Version: "x"}},
		Results: map[string]map[string]Result{"loose": {"dash": {Status: 2, Stderr: "no"}}, "strict": {"dash": {Stdout: "hi"}}},
	}
	md := run.Markdown(cases)
	for _, line := range strings.Split(md, "\n") {
		if strings.Contains(line, "`loose`") && !strings.Contains(line, "(refusal)") {
			t.Errorf("a relaxed row is not marked: %q", line)
		}
		if strings.Contains(line, "`strict`") && strings.Contains(line, "(refusal)") {
			t.Errorf("an exactly graded row is marked as relaxed: %q", line)
		}
	}
	if !strings.Contains(md, "**(refusal)**") {
		t.Error("the mark never appears at all")
	}
}

// TestSyntaxErrorCasesAreGraded is the point of grading them at all.
//
// They were skipped alongside the cases whose reference races, and the two
// exclusions are not alike: a racing reference cannot grade anything, but a
// rejection is as deterministic as an acceptance and its wording is exactly
// what the Diagnostics vector exists for. Skipping them left a whole vector
// ungraded and a report of 100% silent about it — nine real gaps, at the time
// this was written.
func TestSyntaxErrorCasesAreGraded(t *testing.T) {
	var syntaxErrors int
	for _, c := range Corpus {
		if c.SyntaxError {
			syntaxErrors++
			if !graded(c) {
				t.Errorf("%s is not graded, but a rejection is as deterministic as an acceptance", c.ID)
			}
		}
	}
	if syntaxErrors == 0 {
		t.Fatal("no SyntaxError cases in the corpus, so this proves nothing")
	}
	// The one thing that does disqualify a case still does.
	if graded(Case{ReferenceRaces: true}) {
		t.Error("a racing reference cannot grade anything and must stay excluded")
	}
}

// TestRegeneratingKeepsARacingRow is the whole of the fix: a marked row is
// exempt from grading and from drift, and was exempt from neither of the two
// things that actually wrote to the record. Regeneration rewrote it with
// whichever answer the coin gave, which was the only way it could move, so
// every move was noise in somebody else's pull request.
func TestRegeneratingKeepsARacingRow(t *testing.T) {
	cases := []Case{
		{ID: "racy", Category: "shell options", Snippet: "echo hi", ReferenceRaces: true},
		{ID: "steady", Category: "shell options", Snippet: "echo hi"},
	}
	panel := []ShellRecord{{Name: "ksh93", Version: "AJM 93u+"}}
	recorded := &Run{
		Shells: panel,
		Results: map[string]map[string]Result{
			"racy":   {"ksh93": {Stderr: "+ cat~+ echo a"}},
			"steady": {"ksh93": {Stdout: "hi"}},
		},
	}
	// The coin lands the other way, and the steady row genuinely moved.
	fresh := &Run{
		Shells: panel,
		Results: map[string]map[string]Result{
			"racy":   {"ksh93": {Stderr: "+ echo a~+ cat"}},
			"steady": {"ksh93": {Stdout: "bye"}},
		},
	}
	fresh.KeepRacingRows(recorded, cases)

	if got := fresh.Results["racy"]["ksh93"].Stderr; got != "+ cat~+ echo a" {
		t.Errorf("the racing row was resampled: %q", got)
	}
	if got := fresh.Results["steady"]["ksh93"].Stdout; got != "bye" {
		t.Errorf("an ordinary row was pinned as well: %q, want the new measurement", got)
	}
}

// TestARacingRowIsResampledWhenTheShellChanged: the pin is not a freeze. A
// build that moved is the one event that makes the old sample stale rather
// than merely unlucky, and it is exactly when somebody should look again —
// including at a row nothing else checks.
func TestARacingRowIsResampledWhenTheShellChanged(t *testing.T) {
	cases := []Case{{ID: "racy", Category: "shell options", Snippet: "echo hi", ReferenceRaces: true}}
	recorded := &Run{
		Shells:  []ShellRecord{{Name: "ksh93", Version: "AJM 93u+"}, {Name: "zsh", Version: "5.9"}},
		Results: map[string]map[string]Result{"racy": {"ksh93": {Stdout: "old"}, "zsh": {Stdout: "old"}}},
	}
	fresh := &Run{
		Shells:  []ShellRecord{{Name: "ksh93", Version: "AJM 93u+ 2020"}, {Name: "zsh", Version: "5.9"}},
		Results: map[string]map[string]Result{"racy": {"ksh93": {Stdout: "new"}, "zsh": {Stdout: "new"}}},
	}
	fresh.KeepRacingRows(recorded, cases)

	if got := fresh.Results["racy"]["ksh93"].Stdout; got != "new" {
		t.Errorf("an upgraded shell was pinned to its old sample: %q", got)
	}
	if got := fresh.Results["racy"]["zsh"].Stdout; got != "old" {
		t.Errorf("an unchanged shell was resampled: %q", got)
	}
}

// TestKeepRacingRowsInventsNothing: a shell or a case the record has never
// seen has no value to carry forward, and pinning one that does not exist
// would put an empty result where a measurement belongs.
func TestKeepRacingRowsInventsNothing(t *testing.T) {
	cases := []Case{
		{ID: "racy", Category: "shell options", Snippet: "echo hi", ReferenceRaces: true},
		{ID: "added", Category: "shell options", Snippet: "echo hi", ReferenceRaces: true},
	}
	recorded := &Run{
		Shells:  []ShellRecord{{Name: "ksh93", Version: "AJM 93u+"}},
		Results: map[string]map[string]Result{"racy": {"ksh93": {Stdout: "old"}}},
	}
	fresh := &Run{
		Shells: []ShellRecord{{Name: "ksh93", Version: "AJM 93u+"}, {Name: "dash", Version: "0.5"}},
		Results: map[string]map[string]Result{
			"racy":  {"ksh93": {Stdout: "new"}, "dash": {Stdout: "fresh"}},
			"added": {"ksh93": {Stdout: "fresh"}},
		},
	}
	fresh.KeepRacingRows(recorded, cases)

	if got := fresh.Results["racy"]["dash"].Stdout; got != "fresh" {
		t.Errorf("a shell the record never saw was overwritten: %q", got)
	}
	if got := fresh.Results["added"]["ksh93"].Stdout; got != "fresh" {
		t.Errorf("a case the record never had was overwritten: %q", got)
	}
	if n := len(fresh.Results["racy"]); n != 2 {
		t.Errorf("the row grew or shrank: %d shells, want 2", n)
	}
	// And a run with nothing to carry forward from is left alone rather than
	// panicking, which is the first run on a machine with no record.
	fresh.KeepRacingRows(nil, cases)
	if got := fresh.Results["racy"]["ksh93"].Stdout; got != "old" {
		t.Errorf("a nil record changed the run: %q", got)
	}
}

// TestTheRecordMarksARacingRow: the exemption has to be visible to a reader
// of the generated tables. A row that is pinned, ungraded and undrifted while
// looking exactly like its neighbors is the asymmetry somebody later reads as
// a bug — and the reason a marked row was described in a comment nobody
// generating the document could see.
func TestTheRecordMarksARacingRow(t *testing.T) {
	cases := []Case{
		{ID: "racy", Category: "shell options", Snippet: "echo hi", ReferenceRaces: true},
		{ID: "both", Category: "shell options", Snippet: "echo hi", ReferenceRaces: true, GradedOnRefusal: true},
		{ID: "steady", Category: "shell options", Snippet: "echo hi"},
	}
	run := &Run{
		Shells: []ShellRecord{{Name: "ksh93", Version: "x"}},
		Results: map[string]map[string]Result{
			"racy":   {"ksh93": {Stdout: "hi"}},
			"both":   {"ksh93": {Stdout: "hi"}},
			"steady": {"ksh93": {Stdout: "hi"}},
		},
	}
	md := run.Markdown(cases)
	for _, line := range strings.Split(md, "\n") {
		switch {
		case strings.Contains(line, "`racy`") && !strings.Contains(line, "(unordered)"):
			t.Errorf("a racing row is not marked: %q", line)
		case strings.Contains(line, "`steady`") && strings.Contains(line, "(unordered)"):
			t.Errorf("a deterministic row is marked as racing: %q", line)
		case strings.Contains(line, "`both`") &&
			(!strings.Contains(line, "(refusal)") || !strings.Contains(line, "(unordered)")):
			// Two marks say different things, and showing only the first
			// tells a reader the wrong thing about the second.
			t.Errorf("a row carrying both marks shows only one: %q", line)
		}
	}
	if !strings.Contains(md, "**(unordered)**") {
		t.Error("the mark never appears at all")
	}
}

// TestRecordPinsBeforeItRenders: the two artifacts are produced from one Run,
// so pinning the racing rows after the document is rendered would leave a
// stable golden record beside a measurements table that still churned — half
// a fix, and the half nobody diffs is the half that stays broken. Neither
// write says anything about the order on its own, so it is asserted here.
func TestRecordPinsBeforeItRenders(t *testing.T) {
	cases := []Case{{ID: "racy", Category: "shell options", Snippet: "echo hi", ReferenceRaces: true}}
	panel := []ShellRecord{{Name: "ksh93", Version: "AJM 93u+"}}
	prev := &Run{
		Shells:  panel,
		Results: map[string]map[string]Result{"racy": {"ksh93": {Stdout: "the-kept-sample"}}},
	}
	got := &Run{
		Shells:  panel,
		Results: map[string]map[string]Result{"racy": {"ksh93": {Stdout: "the-fresh-coin"}}},
	}
	dir := t.TempDir()
	doc, golden := filepath.Join(dir, "measurements.md"), filepath.Join(dir, "golden.json")
	if err := got.Record(prev, cases, doc, golden); err != nil {
		t.Fatalf("Record: %v", err)
	}

	md, err := os.ReadFile(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(md), "the-fresh-coin") {
		t.Error("the document was rendered before the racing row was pinned")
	}
	if !strings.Contains(string(md), "the-kept-sample") {
		t.Error("the document does not carry the pinned value at all")
	}
	back, err := Load(golden)
	if err != nil {
		t.Fatal(err)
	}
	if s := back.Results["racy"]["ksh93"].Stdout; s != "the-kept-sample" {
		t.Errorf("the saved record holds %q, want the value carried forward", s)
	}
}

// TestNormalizeRewritesEveryLineOfAUsageBlock: the usage text names the shell
// on more than one line, and only the first of them is anchored to `Usage:`.
//
// bash-as-sh is the column that shows it — a shell called by its path has both
// lines caught by the path replacement, and only the one that reports the name
// it was invoked under reaches the anchored rule at all. Left as it was, one
// cell of the record read `sh` where every other read `<shell>`, which is a
// difference a later reader has no way to tell from a finding (#667).
func TestNormalizeRewritesEveryLineOfAUsageBlock(t *testing.T) {
	sh := Found{Shell: Shell{Name: "bash-as-sh", Argv0: "sh"}, Path: "/opt/homebrew/bin/bash"}
	// Measured: bash 5.3 invoked as `sh` with an option letter it has no
	// meaning for, trimmed to the first line of what follows the block.
	const refused = "sh: -Z: invalid option\n" +
		"Usage:\tsh [GNU long option] [option] ...\n" +
		"\tsh [GNU long option] [option] script-file ...\n" +
		"GNU long options:\n" +
		"\t--debug\n"

	got := normalize(refused, sh, "/tmp/d")
	for _, line := range strings.Split(got, "~") {
		if strings.Contains(line, "sh [GNU") {
			t.Errorf("a usage line still names the shell: %q", line)
		}
	}
	if n := strings.Count(got, "<shell> [GNU"); n != 2 {
		t.Errorf("%d usage lines were rewritten, want 2: %q", n, got)
	}
}

// TestAUsageBlockEndsWhereTheIndentDoes: the rule is about usage text, not
// about indentation. Matching an indented name anywhere would rewrite any
// output that happens to list the shell's own name under a heading, which is
// a much larger claim than the one being made.
func TestAUsageBlockEndsWhereTheIndentDoes(t *testing.T) {
	sh := Found{Shell: Shell{Name: "bash-as-sh", Argv0: "sh"}, Path: "/opt/homebrew/bin/bash"}
	const refused = "Usage:\tsh [option] ...\n" +
		"\tsh [option] script-file ...\n" +
		"Files:\n" +
		"\tsh is the one being reported on\n"

	got := normalize(refused, sh, "/tmp/d")
	if !strings.Contains(got, "\tsh is the one being reported on") {
		t.Errorf("a line past the end of the block was rewritten: %q", got)
	}
	if strings.Count(got, "<shell> [option]") != 2 {
		t.Errorf("the block itself was not rewritten: %q", got)
	}
}

// TestAUsageBlockIsNotOpenedByAnIndentAlone: without a `Usage:` line there is
// no block, so an indented name is left exactly as the shell printed it.
func TestAUsageBlockIsNotOpenedByAnIndentAlone(t *testing.T) {
	sh := Found{Shell: Shell{Name: "bash-as-sh", Argv0: "sh"}, Path: "/opt/homebrew/bin/bash"}
	const listing = "shells:\n\tsh\n\tksh\n"
	if got := normalize(listing, sh, "/tmp/d"); got != "shells:~\tsh~\tksh" {
		t.Errorf("normalize = %q, want the listing untouched", got)
	}
}

// TestAUsageBlockSurvivesALineThatDoesNotNameTheShell: what ends a block is
// the indent stopping, not the name being absent from a line — the lines
// after such a line are still usage text and still name the shell.
//
// Constructed rather than measured: no panel member lays its usage out this
// way today, and the point is that the rule does not depend on that staying
// true, since a rule that happens to work only on the exact shape in front of
// it is one nobody can reason about when the shape changes.
func TestAUsageBlockSurvivesALineThatDoesNotNameTheShell(t *testing.T) {
	sh := Found{Shell: Shell{Name: "bash-as-sh", Argv0: "sh"}, Path: "/opt/homebrew/bin/bash"}
	const refused = "Usage:\tsh [option] ...\n" +
		"\t   or, with a file:\n" +
		"\tsh [option] script-file ...\n"

	got := normalize(refused, sh, "/tmp/d")
	if n := strings.Count(got, "<shell> [option]"); n != 2 {
		t.Errorf("%d usage lines were rewritten, want both across the line between them: %q", n, got)
	}
}

// TestNormalizeRewritesAFixedSelfName is the third spelling of a shell's own
// name, after the binary's basename and Argv0.
//
// zsh answers with a constant `zsh:` whatever it was invoked as, so a binary
// that is a zsh but is not *called* zsh — which is every build the graders
// produce — printed a name the harness left standing.
func TestNormalizeRewritesAFixedSelfName(t *testing.T) {
	sh := Found{Shell: Shell{Name: "ours", SelfName: "zsh"}, Path: "/build/our-zsh"}
	const said = "zsh:1: command not found: nosuchcmd\n" +
		"+zsh:1> echo a\n"

	got := normalize(said, sh, "/tmp/d")
	want := "<shell>:1: command not found: nosuchcmd~+<shell>:1> echo a"
	if got != want {
		t.Errorf("normalize = %q, want %q", got, want)
	}
}

// TestAFixedSelfNameIsRewrittenOnlyWhereAShellNamesItself. The rewrite is the
// same anchored one the basename gets, and for the same reason: a name
// replaced anywhere corrupts ordinary words. `zsh` is a word a shell's own
// output says — `echo zsh: hi` is a program printing, not a shell
// complaining — so the rule must not reach it.
func TestAFixedSelfNameIsRewrittenOnlyWhereAShellNamesItself(t *testing.T) {
	sh := Found{Shell: Shell{Name: "ours", SelfName: "zsh"}, Path: "/build/our-zsh"}
	for _, tc := range []struct{ said, want string }{
		{"the zsh: shell\n", "the zsh: shell"},
		{"zshrc: not a name\n", "zshrc: not a name"},
		{"  zsh: indented\n", "  zsh: indented"},
		{"zsh1: no colon after the name\n", "zsh1: no colon after the name"},
	} {
		if got := normalize(tc.said, sh, "/tmp/d"); got != tc.want {
			t.Errorf("normalize(%q) = %q, want %q", tc.said, got, tc.want)
		}
	}
}

// TestNoSelfNameRewritesNothingExtra: the field is opt-in, and the shells
// that name themselves by argv[0] — three of the four — must be unaffected.
func TestNoSelfNameRewritesNothingExtra(t *testing.T) {
	sh := Found{Shell: Shell{Name: "ours"}, Path: "/build/our-bash"}
	const said = "zsh:1: command not found: nosuchcmd\n"
	if got := normalize(said, sh, "/tmp/d"); got != "zsh:1: command not found: nosuchcmd" {
		t.Errorf("normalize = %q, want the line untouched", got)
	}
}

// TestTheZshPanelEntryDeclaresItsFixedSelfName pins the measured fact where
// the harness reads it. Losing it costs 22 points of measured zsh conformance
// and nothing looks wrong: the reference column normalizes by accident,
// because real zsh's binary happens to be called zsh.
func TestTheZshPanelEntryDeclaresItsFixedSelfName(t *testing.T) {
	for _, s := range Panel {
		if s.Name == "zsh" && s.SelfName != "zsh" {
			t.Fatalf("the zsh panel entry declares SelfName %q, want zsh", s.SelfName)
		}
		if s.Name != "zsh" && s.SelfName != "" {
			t.Errorf("%s declares a fixed self name %q; only zsh was measured to have one",
				s.Name, s.SelfName)
		}
	}
}

// TestARunIsRecordedTheSameUnderAnyBinaryName is the fix stated as the
// property it buys: where a binary sits is not a fact about the shell, so the
// same shell reached by two names must record identically.
func TestARunIsRecordedTheSameUnderAnyBinaryName(t *testing.T) {
	ref, ok := panelMember(t, "zsh")
	if !ok {
		t.Skip("zsh is not on this machine")
	}
	link := filepath.Join(t.TempDir(), "our-zsh")
	if err := os.Symlink(ref.Path, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	renamed := Found{Shell: Shell{Name: "ours", SelfName: ref.SelfName}, Path: link}
	c := Case{ID: "t", Snippet: "nosuchcmd_zz"}

	want, got := Exec(context.Background(), ref, c), Exec(context.Background(), renamed, c)
	if want.Stderr == "" || !strings.Contains(want.Stderr, "<shell>:") {
		t.Fatalf("the fixture no longer produces a self-named diagnostic: %q", want.Stderr)
	}
	if want != got {
		t.Errorf("the same shell under two names recorded differently:\n  %q\n  %q",
			want.Stderr, got.Stderr)
	}
}

// TestConformanceGradesTheShellAndNotTheBuildPath is the same property one
// level up, through the grader, which is where the 281 rows were lost.
func TestConformanceGradesTheShellAndNotTheBuildPath(t *testing.T) {
	ref, ok := panelMember(t, "zsh")
	if !ok {
		t.Skip("zsh is not on this machine")
	}
	link := filepath.Join(t.TempDir(), "our-zsh")
	if err := os.Symlink(ref.Path, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	cases := []Case{
		{ID: "a", Snippet: "nosuchcmd_zz"},
		{ID: "b", Snippet: "set -x; echo a"},
	}

	rep, err := RunConformance(context.Background(), link, "zsh", nil, cases)
	if err != nil {
		t.Fatalf("RunConformance: %v", err)
	}
	if rep.Passed != rep.Total {
		t.Errorf("zsh graded against itself scored %d/%d; the failures are:\n%s",
			rep.Passed, rep.Total, rep.Summary(true))
	}
}

// TestConformanceStillFailsARowThatDiffersInMoreThanTheName is the other half,
// and the one that decides whether the change is a fix or a loosening.
//
// The rewrite is anchored to one literal word in one position. Everything
// else about the row is still compared byte for byte, so a shell that words
// its diagnostic differently fails exactly as before — here bash, which says
// `line 1: nosuchcmd_zz: command not found` where zsh says `1: command not
// found: nosuchcmd_zz`, graded against zsh.
func TestConformanceStillFailsARowThatDiffersInMoreThanTheName(t *testing.T) {
	_, okz := panelMember(t, "zsh")
	bash, okb := panelMember(t, "bash")
	if !okz || !okb {
		t.Skip("this needs both zsh and bash")
	}
	cases := []Case{{ID: "a", Snippet: "nosuchcmd_zz"}}

	rep, err := RunConformance(context.Background(), bash.Path, "zsh", nil, cases)
	if err != nil {
		t.Fatalf("RunConformance: %v", err)
	}
	if rep.Passed != 0 {
		t.Errorf("bash scored %d/%d against zsh; the self-name rewrite is forgiving "+
			"a difference in the wording, not only in the name", rep.Passed, rep.Total)
	}
}

func panelMember(t *testing.T, name string) (Found, bool) {
	t.Helper()
	found, _ := Resolve(context.Background())
	for _, f := range found {
		if f.Name == name {
			return f, true
		}
	}
	return Found{}, false
}

// TestVersionIdentifiesAShellThatOnlyAnswersHelp pins the probe BusyBox needs.
//
// version tries spellings in order and gives up with "unknown", so a shell it
// cannot name is indistinguishable from a shell with no version at all --
// which is how BusyBox ash read: `--version` is a bad option to it and
// `${.sh.version}` is a bad substitution, both non-zero, and the only place
// it writes its build is the first line of --help. An entry asserting
// MustReport: "busybox" would therefore land in missing on every machine,
// ash present or not. The stand-in refuses the first two probes exactly as
// BusyBox v1.37.0 does and answers the third with the banner it prints, so
// the test fails if the third spelling is dropped or misspelled.
func TestVersionIdentifiesAShellThatOnlyAnswersHelp(t *testing.T) {
	const banner = "BusyBox v1.37.0 (2026-01-10 15:38:28 UTC) multi-call binary."

	path := filepath.Join(t.TempDir(), "sh")
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"--help) echo '" + banner + "'; echo; echo 'Usage: sh [-il] ...'; exit 0 ;;\n" +
		"--version) echo \"sh: bad option '--version'\" >&2; exit 2 ;;\n" +
		"*) echo 'sh: syntax error: bad substitution' >&2; exit 2 ;;\n" +
		"esac\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := version(context.Background(), path); got != banner {
		t.Errorf("version = %q, want %q", got, banner)
	}
	if !strings.Contains(strings.ToLower(version(context.Background(), path)), "busybox") {
		t.Error("the reported version does not carry the word MustReport would match")
	}
}

// TestVersionPrefersTheSpellingAShellAlreadyAnswers keeps the new probe from
// moving a column that was already recorded.
//
// version runs for every panel member, so a spelling appended to the list is
// only additive if no shell ahead of BusyBox reaches it. bash answers both
// --version and --help, with different first lines on the two, and the golden
// record holds the --version one.
func TestVersionPrefersTheSpellingAShellAlreadyAnswers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sh")
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"--version) echo 'GNU bash, version 5.3.15(1)-release'; exit 0 ;;\n" +
		"--help) echo 'Usage: sh [options]'; exit 0 ;;\n" +
		"*) exit 2 ;;\n" +
		"esac\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := version(context.Background(), path); !strings.HasPrefix(got, "GNU bash") {
		t.Errorf("version = %q, want the --version answer", got)
	}
}

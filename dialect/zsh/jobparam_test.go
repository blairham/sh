// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"regexp"
	"strings"
	"testing"
)

// pids replaces every run of digits that is a process id with `P`, so a row
// can assert a shape without asserting a number the kernel chose.
var pids = regexp.MustCompile(`[0-9]{2,}`)

// **The three job parameters are views over the job table**, keyed by the
// number `%N` names.
//
// Measured 2026-09-26 against zsh 5.9.2 (aarch64-apple-darwin25.4.0), run
// `-f` with `env -u FPATH` over a script file, with the commands named by
// absolute path because this harness gives a snippet a PATH of its own. Every
// row here was taken from
// that shell first and then from this one; the pids are masked because they
// are the kernel's, and the shape around them is what the reference fixes.
func TestTheJobParametersReadTheJobTable(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/parameter
print -r -- "have=${+jobstates} ${+jobtexts} ${+jobdirs}"
print -r -- "type=${(t)jobstates}"
/bin/sleep 5 &
print -r -- "keys=${(k)jobstates}"
print -r -- "state=[${jobstates[1]}]"
print -r -- "text=[${jobtexts[1]}]"
print -r -- "dir=[${jobdirs[1]}]"
kill %1
wait`)
	got := pids.ReplaceAllString(out, "P")
	for _, want := range []string{
		"have=1 1 1\n",
		// The type word is the reference's, and the three attributes in it
		// are why each carries MarkReadonly and hideModuleParameter.
		"type=association-readonly-hide-hideval-special\n",
		"keys=1\n",
		// The job's state, the marker `%+` puts on it, and one `pid=state`
		// for the one process it is made of.
		"state=[running:+:P=running]\n",
		"text=[/bin/sleep 5]\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("got %q, want it to contain %q", got, want)
		}
	}
	if !strings.Contains(got, "dir=[/") {
		t.Errorf("got %q, want an absolute directory for the job", got)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// **The marker field is empty for a job carrying neither marker**, and it
// keeps both colons.
//
// Four jobs, because two cannot show it: with one job or two, every job has a
// marker and a value built by joining only the fields that are there would
// pass. Measured against the reference, which answers `running::P=running`
// for the first two of four, `running:-:…` for the third and `running:+:…`
// for the fourth.
func TestTheCurrentAndPreviousMarkersAreTheListingsOwn(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/parameter
/bin/sleep 5 &
/bin/sleep 6 &
/bin/sleep 7 &
/bin/sleep 8 &
print -r -- "1=[${jobstates[1]}] 2=[${jobstates[2]}] 3=[${jobstates[3]}] 4=[${jobstates[4]}]"
kill %1 %2 %3 %4
wait`)
	want := "1=[running::P=running] 2=[running::P=running] " +
		"3=[running:-:P=running] 4=[running:+:P=running]\n"
	if got := pids.ReplaceAllString(out, "P"); !strings.Contains(got, want) {
		t.Errorf("got %q, want it to contain %q", got, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// **A pipeline is one job and several processes**, and each gets a `pid=state`
// of its own.
//
// The row that says the value is built from the job's process list rather
// than from its one settled pid — which is what `$!` answers, and what a
// value built from it would have shown here.
func TestAPipelinesProcessesEachAppearInTheState(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/parameter
/bin/sleep 5 | /bin/cat &
print -r -- "state=[${jobstates[1]}]"
print -r -- "text=[${jobtexts[1]}]"
kill %1
wait`)
	got := pids.ReplaceAllString(out, "P")
	if want := "state=[running:+:P=running:P=running]\n"; !strings.Contains(got, want) {
		t.Errorf("got %q, want it to contain %q", got, want)
	}
	if want := "text=[/bin/sleep 5 | /bin/cat]\n"; !strings.Contains(got, want) {
		t.Errorf("got %q, want it to contain %q", got, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// **The key is a job specification**, so `%+`, `%?text` and a bare command
// prefix all reach the job, and the two kinds of miss are worded differently.
//
// The misses are the half worth measuring: both are the empty string at
// status 0, so the complaint is the only thing that tells them apart and a
// row reading the value alone would pass either way.
func TestAJobParametersKeyIsAJobSpecification(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/parameter
print -r -- "byNumber=[${jobstates[9]}] s=$?"
print -r -- "byMarkedNumber=[${jobstates[%9]}] s=$?"
/bin/sleep 5 &
print -r -- "current=[${jobtexts[%+]}]"
print -r -- "contains=[${jobtexts[%?lee]}]"
print -r -- "prefix=[${jobtexts[/bin/sl]}]"
print -r -- "missing=[${jobtexts[nosuch]}]"
kill %1
wait`)
	for _, want := range []string{
		// A number naming no job says nothing at all, with or without the
		// `%`.
		"byNumber=[] s=0\n",
		"byMarkedNumber=[] s=0\n",
		"current=[/bin/sleep 5]\n",
		"contains=[/bin/sleep 5]\n",
		"prefix=[/bin/sleep 5]\n",
		// Text matching no command is named, and the value is still empty
		// and the status still 0.
		"job not found: nosuch\n",
		"missing=[]\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("got %q, want it to contain %q", out, want)
		}
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// **They are views and not snapshots**, which is the property every produced
// parameter in this module has to have: a table filled in once would be right
// until the first `&` and quietly wrong afterwards.
//
// Read, start a job, read again — three answers where a snapshot gives one.
// Asserted this way round rather than by checking that a value looks right,
// because a snapshot taken late enough passes that.
func TestTheJobParametersAreViewsAndNotSnapshots(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/parameter
print -r -- "before=${#jobstates}"
/bin/sleep 5 &
print -r -- "during=${#jobstates}"
kill %1
wait
print -r -- "after=${#jobstates}"`)
	want := "before=0\nduring=1\nafter=0\n"
	if !strings.Contains(out, want) || st != 0 {
		t.Errorf("got %q (status %d), want it to contain %q", out, st, want)
	}
}

// **And they are frozen, as they are in the shell being modeled**: a produced
// table with no freeze would take an assignment into a stored table that then
// shadows the producer, and nothing afterwards would report a job.
//
// Measured: `jobstates=(a b c)` is `read-only variable: jobstates` at 1 and
// ends the script, and `unset jobstates` is the same sentence — which is the
// row that parts these from the names this module has *not* got, where the
// assignment refuses and the unset is a silent 0.
func TestTheJobParametersAreReadOnly(t *testing.T) {
	for _, src := range []string{
		`jobstates=(a b c)`,
		`jobtexts[1]=q`,
		`unset jobdirs`,
	} {
		out, st := runZsh(t, t.TempDir(), src+"\nprint -r -- unreached")
		if st == 0 || !strings.Contains(out, "read-only variable: ") ||
			strings.Contains(out, "unreached") {
			t.Errorf("%s = %q (status %d), want the read-only refusal and the script ended",
				src, out, st)
		}
	}
}

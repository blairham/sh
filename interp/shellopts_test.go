// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The option record: the long names of the options that are on, under a name
// the dialect chooses.
//
// The variable is named here rather than in the tests' prose because a test in
// this package names a seam and never a shell — the seam is
// Runner.SetShellOptions, and `OPTIONRECORD` is a name no shell uses, which is
// the point. What matters is that the *name* comes from outside the core.
const optionRecord = "OPTIONRECORD"

// withOptionRecord is a shell that has the record, plus the extra option names
// it needs to have anything interesting in it.
func withOptionRecord(r *Runner) {
	withExtras(r)
	r.SetShellOptions(optionRecord)
}

// TestTheOptionRecordIsProducedAndNotStored.
//
// The whole design rests on this. A stored copy would be the options the shell
// started with, so a script that read the record after changing an option
// would be told something untrue — which is the one failure mode this variable
// has that plain absence does not.
func TestTheOptionRecordIsProducedAndNotStored(t *testing.T) {
	out, _ := run(t, `echo "[$`+optionRecord+`]"
set -x
echo "[$`+optionRecord+`]" 2>/dev/null
set +x
echo "[$`+optionRecord+`]"
`, withOptionRecord)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// The trace of the middle line goes to the discarded stream, so what is
	// left is the three values.
	var got []string
	for _, l := range lines {
		if strings.HasPrefix(l, "[") {
			got = append(got, l)
		}
	}
	if len(got) != 3 {
		t.Fatalf("out = %q, want three readings of the record", out)
	}
	if strings.Contains(got[0], "xtrace") {
		t.Errorf("before `set -x` the record was %q, want no xtrace in it", got[0])
	}
	if !strings.Contains(got[1], "xtrace") {
		t.Errorf("after `set -x` the record was %q, want xtrace in it", got[1])
	}
	if strings.Contains(got[2], "xtrace") {
		t.Errorf("after `set +x` the record was %q, want xtrace gone again", got[2])
	}
}

// TestTheOptionRecordIsSortedAndNamedInFull, which is why a script reads it
// for membership rather than comparing the whole string: what comes back out
// is never what went in.
func TestTheOptionRecordIsSortedAndNamedInFull(t *testing.T) {
	out, _ := run(t, `set -u
set -f
echo "$`+optionRecord+`"
`, withOptionRecord)
	value := strings.TrimRight(out, "\n")
	names := strings.Split(value, ":")
	if len(names) < 2 {
		t.Fatalf("record = %q, want a colon-separated list", value)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Errorf("record = %q, want the names sorted", value)
			break
		}
	}
	for _, want := range []string{"noglob", "nounset"} {
		if !strings.Contains(":"+value+":", ":"+want+":") {
			t.Errorf("record = %q, want %q in it by its long name", value, want)
		}
	}
	// And the letters the script used are not what it says: `-u` and `-f`
	// went in, `nounset` and `noglob` come out.
	if strings.Contains(value, ":u:") || strings.Contains(value, ":f:") {
		t.Errorf("record = %q, want long names rather than the letters written", value)
	}
}

// TestTheOptionRecordCarriesTheShellsOwnDefaults. Nothing had to be turned on
// for the record to have something in it, which is the half a value merely
// echoed back from the environment would not have.
func TestTheOptionRecordCarriesTheShellsOwnDefaults(t *testing.T) {
	out, _ := run(t, `echo "[$`+optionRecord+`]"`, withOptionRecord)
	if !strings.Contains(out, "braceexpand") {
		t.Errorf("record = %q, want an option this shell has on by default", out)
	}
}

// TestTheOptionRecordRefusesAssignment. The name is readonly, so a script
// cannot make it lie, and the refusal is the dialect's ordinary readonly one
// rather than a sentence of its own.
func TestTheOptionRecordRefusesAssignment(t *testing.T) {
	out, _ := run(t, optionRecord+`=whatever
echo "after [$`+optionRecord+`]"
`, withOptionRecord)
	if !strings.Contains(out, "readonly variable") {
		t.Errorf("out = %q, want the assignment refused as readonly", out)
	}
	if strings.Contains(out, "whatever") {
		t.Errorf("out = %q, want the assigned value nowhere in the record", out)
	}
}

// TestWithNoNameThereIsNoRecord. Three of the four presets name nothing, and
// for them the variable does not exist and the environment entry is an
// ordinary string.
func TestWithNoNameThereIsNoRecord(t *testing.T) {
	out, _ := run(t, `echo "[${`+optionRecord+`-unset}]"`, withExtras)
	if !strings.Contains(out, "[unset]") {
		t.Errorf("out = %q, want no such variable where the dialect named none", out)
	}
	// And it stays an ordinary string: a shell that named nothing can be
	// assigned to.
	out, _ = run(t, optionRecord+`=mine
echo "[$`+optionRecord+`]"
`, withExtras)
	if !strings.Contains(out, "[mine]") {
		t.Errorf("out = %q, want an ordinary variable where the dialect named none", out)
	}
}

// TestTheEnvironmentSeedsTheOptions is the write direction of the binding, and
// the reason the variable matters at all: what a shell is launched with
// changes what it does.
func TestTheEnvironmentSeedsTheOptions(t *testing.T) {
	out, _ := run(t, `case $- in *u*) echo has-u ;; *) echo no-u ;; esac
case ":$`+optionRecord+`:" in *:nounset:*) echo in-record ;; *) echo not-in-record ;; esac
`, func(r *Runner) {
		withOptionRecord(r)
		r.Env = append(r.Env, optionRecord+"=nounset")
		r.ApplyInheritedShellOptions()
	})
	if !strings.Contains(out, "has-u") {
		t.Errorf("out = %q, want the inherited name to have turned the option on", out)
	}
	if !strings.Contains(out, "in-record") {
		t.Errorf("out = %q, want `$-` and the record to agree", out)
	}
}

// TestSeedingIsSkippedWhereTheDialectNamedNothing. The same environment entry
// does nothing at all without a name, which is what the other three shells do
// with it.
func TestSeedingIsSkippedWhereTheDialectNamedNothing(t *testing.T) {
	out, _ := run(t, `case $- in *u*) echo has-u ;; *) echo no-u ;; esac`, func(r *Runner) {
		withExtras(r)
		r.Env = append(r.Env, optionRecord+"=nounset")
		r.ApplyInheritedShellOptions()
	})
	if !strings.Contains(out, "no-u") {
		t.Errorf("out = %q, want the entry ignored where no name was given", out)
	}
}

// TestAnUnknownNameInTheEnvironmentIsRefusedAndTheRestApplied. Measured: the
// complaint names line 0 and the good names in the same value still take
// effect, so one bad entry does not cost the others.
func TestAnUnknownNameInTheEnvironmentIsRefusedAndTheRestApplied(t *testing.T) {
	out, _ := run(t, `case $- in *u*) echo has-u ;; *) echo no-u ;; esac`, func(r *Runner) {
		withOptionRecord(r)
		r.Env = append(r.Env, optionRecord+"=nosuchoption:nounset")
		r.ApplyInheritedShellOptions()
	})
	if !strings.Contains(out, "invalid option name") {
		t.Errorf("out = %q, want the unknown name refused", out)
	}
	if !strings.Contains(out, "has-u") {
		t.Errorf("out = %q, want the good name applied anyway", out)
	}
	// The plainest of the three refusal shapes: nothing stands where the
	// builtin's name would, which is what separates this from a script's own
	// `set -o` and from an invocation's `-o`.
	if strings.Contains(out, "set: nosuchoption") {
		t.Errorf("out = %q, want no builtin named in a refusal that came from the environment", out)
	}
}

// TestTheEnvironmentIsReadOnceAndOnlyFromWhatWasInherited.
//
// The variable is readonly and produced, so there can be no assigned value to
// read — but the question being asked is what the shell was *launched* with,
// and a second call must not re-apply an option a script has since turned off.
func TestTheEnvironmentIsReadOnceAndOnlyFromWhatWasInherited(t *testing.T) {
	out, _ := run(t, `set +u
case $- in *u*) echo still-u ;; *) echo turned-off ;; esac
`, func(r *Runner) {
		withOptionRecord(r)
		r.Env = append(r.Env, optionRecord+"=nounset")
		r.ApplyInheritedShellOptions()
	})
	if !strings.Contains(out, "turned-off") {
		t.Errorf("out = %q, want the script able to turn off what the environment turned on", out)
	}
}

// TestAChildIsHandedTheRecomputedRecord.
//
// The entry the shell was launched with is the only place a stale value could
// reach a command, and it is where it did: a shell handed the tracing option
// that then turned it off would still have been turning it on in everything it
// started.
func TestAChildIsHandedTheRecomputedRecord(t *testing.T) {
	out, _ := run(t, `set +u
printenv `+optionRecord+`
`, func(r *Runner) {
		withOptionRecord(r)
		r.Env = append(r.Env, optionRecord+"=nounset")
		r.ApplyInheritedShellOptions()
	})
	if strings.Contains(out, "nounset") {
		t.Errorf("the child was handed %q, want the option gone from it after `set +u`", out)
	}
	if !strings.Contains(out, "braceexpand") {
		t.Errorf("the child was handed %q, want this shell's record rather than nothing", out)
	}
}

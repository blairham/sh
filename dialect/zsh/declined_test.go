// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// The staleness checks that make declined.go a ledger rather than a comment.
//
// A recorded decision that cannot go stale is worth very little: the entry
// outlives the reason, somebody implements the thing, and the ledger goes on
// saying it will not be. So each check below is written as a function over a
// ledger rather than over the package's own variables, and each one is run
// twice — once against what is committed, which must be quiet, and once
// against a ledger built to be wrong, which must not be. That second half is
// the whole point: a checker nobody has seen fire is not evidence, and this
// file's failure mode would otherwise be silence.

// declinedModuleFaults is every way a declined *module* entry can have stopped
// being true, as sentences.
//
// One fault and one sentence, rather than a bool, because the interesting
// failure is not "something is wrong" — it is *which*, and the day a module
// starts loading is a different day from the day somebody deletes its reason.
func declinedModuleFaults(ledger map[string]declined, features map[string][]string) []string {
	var faults []string
	for _, module := range sortedKeys(ledger) {
		entry := ledger[module]
		faults = append(faults, entryFaults(module, entry)...)
		if _, loads := features[module]; loads {
			faults = append(faults, fmt.Sprintf(
				"%s is in zmodloadFeatures and is also recorded as declined: "+
					"a module that loads has no decision left to record, so "+
					"take the ledger entry out and say so on #%d",
				module, entry.issue))
		}
	}
	return faults
}

// declinedBuiltinFaults is the same question for a declined *builtin*, and it
// asks the opposite thing of the feature table.
//
// A declined module must be absent from it; a declined builtin must be
// **present** in some module's list, because that is what says the name is a
// thing zsh really has and this shell really declines. A name no module claims
// is not a decision — it is a spelling mistake that would sit here forever
// looking like one.
func declinedBuiltinFaults(ledger map[string]declined, features map[string][]string) []string {
	var faults []string
	for _, name := range sortedKeys(ledger) {
		entry := ledger[name]
		faults = append(faults, entryFaults(name, entry)...)
		var claimed []string
		for _, module := range sortedKeys(features) {
			for _, feature := range features[module] {
				if feature == "b:"+name {
					claimed = append(claimed, module)
				}
			}
		}
		if len(claimed) == 0 {
			faults = append(faults, fmt.Sprintf(
				"no module in zmodloadFeatures names b:%s, so the ledger is "+
					"recording a decision about a builtin this shell has no "+
					"record of zsh having — see #%d", name, entry.issue))
		}
	}
	return faults
}

// entryFaults is what every entry owes whatever it is about.
func entryFaults(name string, entry declined) []string {
	var faults []string
	if entry.issue == 0 {
		faults = append(faults, name+" names no issue, so the measurement it "+
			"was decided on is nowhere a reader can reach")
	}
	if strings.TrimSpace(entry.cost) == "" {
		faults = append(faults, name+" states no cost, and an absence with no "+
			"figure beside it cannot be ranked against anything")
	}
	if strings.TrimSpace(entry.why) == "" {
		faults = append(faults, name+" states no reason, which makes it an "+
			"oversight with a table entry rather than a decision")
	}
	if strings.TrimSpace(entry.changes) == "" {
		faults = append(faults, name+" says nothing that would change the "+
			"answer, so there is no way for it to be wrong")
	}
	return faults
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestTheDeclinedLedgerIsCheckedAgainstTheFeatureTable is the committed half:
// what is in the tree today must be quiet.
func TestTheDeclinedLedgerIsCheckedAgainstTheFeatureTable(t *testing.T) {
	t.Parallel()
	if len(declinedModules) == 0 && len(declinedBuiltins) == 0 &&
		len(declinedFormats) == 0 {
		// An empty ledger is a possible state and it is not this one. Said
		// out loud because a walk over nothing reports exactly what a walk
		// over a clean ledger reports, and the two are not the same result.
		t.Fatal("all three ledgers are empty, so this check looked at nothing")
	}
	for _, fault := range declinedModuleFaults(declinedModules, zmodloadFeatures) {
		t.Errorf("declined module: %s", fault)
	}
	for _, fault := range declinedBuiltinFaults(declinedBuiltins, zmodloadFeatures) {
		t.Errorf("declined builtin: %s", fault)
	}
	// A declined *format* owes the same four fields and has no feature-table
	// question to answer: the builtin that writes it is registered and works,
	// which is what makes it a third kind. Its falsifier is the behavioral
	// test below.
	for _, name := range sortedKeys(declinedFormats) {
		for _, fault := range entryFaults(name, declinedFormats[name]) {
			t.Errorf("declined format: %s", fault)
		}
	}
}

// TestZcompileStillWritesTextRatherThanWordcode is the falsifier for the
// `zcompile wordcode` entry in declinedFormats.
//
// The other two ledgers are checked against a table: a declined module must
// be absent from zmodloadFeatures and a declined builtin must be present in
// it. A declined **format** has no such question — the builtin is registered
// and exits 0 — so the only thing that can go stale is what it *writes*.
//
// So this reads the bytes. It fails the day `zcompile` starts emitting zsh's
// magic, which is the day the decision has been taken back and the ledger
// entry has to come out. The magic is the four bytes the suite file's own
// `%prep` documents and the reference was measured writing; it is quoted here
// as a **negative** expectation, which is the only way this file may hold it.
func TestZcompileStillWritesTextRatherThanWordcode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "victim"), []byte("print victim ran\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The builtin called directly, because this file is the package's own
	// test and cannot reach the external helpers — and because calling it is
	// the point: what is being checked is the bytes it writes.
	var out bytes.Buffer
	sem, diag, d := Semantics(), Diagnostics(), Dialect()
	r := &interp.Runner{
		Stdout: &out, Stderr: &out, Semantics: &sem, Diagnostics: &diag,
		Dir: dir, Name: "zsh", Dialect: &d,
	}
	Apply(r)
	if st := zcompileBuiltin(r, context.Background(), []string{"victim"}); st != 0 {
		t.Fatalf("zcompile exited %d (output %q), so this check looked at nothing",
			st, out.String())
	}
	got, err := os.ReadFile(filepath.Join(dir, "victim.zwc"))
	if err != nil {
		t.Fatalf("no dump written, so this check looked at nothing: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("the dump is empty, so this check looked at nothing")
	}
	magic := []byte{0x07, 0x06, 0x05, 0x04}
	if bytes.HasPrefix(got, magic) {
		t.Errorf("zcompile wrote wordcode magic % x — declinedFormats records "+
			"that this shell does not write that format, so either the entry "+
			"is stale and should be removed on #5141, or the magic arrived by "+
			"accident", got[:4])
	}
	// And the positive half, so the check above is not a test that passes
	// because nothing was written: what is there is the script's own text.
	if want := "print victim ran\n"; string(got) != want {
		t.Errorf("dump = %q, want the input's text %q — declinedFormats' cost "+
			"line quotes those bytes", got, want)
	}
}

// TestTheDeclinedLedgerChecksCanFire is the other half, and it is the half
// that makes the first one mean anything.
//
// Four mutations, one per way an entry stops being true, each of which has to
// be reported. Without this the check above is a function that has never been
// seen to return a non-empty slice, which reads identically to a ledger with
// nothing wrong in it.
func TestTheDeclinedLedgerChecksCanFire(t *testing.T) {
	t.Parallel()
	sound := declined{issue: 1, cost: "c", why: "w", changes: "x"}
	features := map[string][]string{"zsh/zutil": {"b:zregexparse"}}
	for _, row := range []struct {
		what    string
		faults  []string
		wantOne string
	}{
		{
			"a declined module that has started loading",
			declinedModuleFaults(map[string]declined{"zsh/zutil": sound}, features),
			"has no decision left to record",
		},
		{
			"a declined builtin no module names",
			declinedBuiltinFaults(map[string]declined{"nosuchname": sound}, features),
			"no record of zsh having",
		},
		{
			"an entry with no issue",
			entryFaults("x", declined{cost: "c", why: "w", changes: "x"}),
			"names no issue",
		},
		{
			"an entry with no falsifier",
			entryFaults("x", declined{issue: 1, cost: "c", why: "w"}),
			"no way for it to be wrong",
		},
	} {
		found := false
		for _, fault := range row.faults {
			if strings.Contains(fault, row.wantOne) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s reported %q, want a fault mentioning %q",
				row.what, row.faults, row.wantOne)
		}
	}
	// And the control on the other side, so that the four rows above are
	// not simply a checker that reports everything.
	if faults := declinedBuiltinFaults(
		map[string]declined{"zregexparse": sound}, features); len(faults) != 0 {
		t.Errorf("a sound entry reported %q, want nothing", faults)
	}
}

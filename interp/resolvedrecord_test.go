// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// An open whose name reached somewhere else says so in the record, whether it
// was allowed or refused.
//
// It used to say half of it, and a different half each way. An allowed open
// recorded the name the script wrote and nothing said the name had gone
// elsewhere; a refused one recorded the object and nothing said which name
// reached it. "This script read /srv/data/x, and /srv/data is a link to
// /mnt/vol1" is the fact somebody reviewing a run wants, and the shell had
// just learned it and thrown it away.
//
// All fixtures are under t.TempDir(). A symbolic link is a thing that points
// somewhere, and one pointing into a person's home is a test that can damage
// the machine it runs on.

// allowingEverything is a gate that permits and remembers, which is what makes
// the *allowed* consultation observable at all.
func allowingEverything(asked *[]Action) func(*Runner) {
	return func(r *Runner) {
		r.Gate = GateFunc(func(_ context.Context, a Action) Decision {
			*asked = append(*asked, a)
			return Allow
		})
	}
}

// collecting records every access the stream carries.
func collecting(events *[]Event) func(*Runner) {
	return func(r *Runner) {
		r.Events = SinkFunc(func(_ context.Context, e Event) { *events = append(*events, e) })
	}
}

// TestAnAllowedOpenThroughALinkRecordsWhereItWent is #943 itself.
func TestAnAllowedOpenThroughALinkRecordsWhereItWent(t *testing.T) {
	object, link := linkFarm(t)
	physical, err := filepath.EvalSymlinks(object)
	if err != nil {
		t.Fatal(err)
	}
	var asked []Action
	var events []Event
	out, status := run(t, "cat <"+link, func(r *Runner) {
		allowingEverything(&asked)(r)
		collecting(&events)(r)
	})
	if !strings.Contains(out, "classified") || status != 0 {
		t.Fatalf("out = %q status %d, want the allowed read to have happened", out, status)
	}

	var access *Event
	for i, e := range events {
		if e.Kind == EventAccess && e.Action.Kind == ActionOpen && e.Action.Path == link {
			access = &events[i]
		}
	}
	if access == nil {
		t.Fatalf("no access record names the path as written: %+v", events)
	}
	if access.Action.Resolved != physical {
		t.Errorf("the record resolved to %q, want %q — an allowed open that went "+
			"somewhere else has to say so", access.Action.Resolved, physical)
	}

	// And the gate was asked twice about one action: the name, then the
	// object. The second consultation is about the object *alone* — Path is
	// the kernel's name there — so a rule matches what was reached even from a
	// Gate that has never heard of Resolved.
	var opens []Action
	for _, a := range asked {
		if a.Kind == ActionOpen && (a.Path == link || a.Path == physical) {
			opens = append(opens, a)
		}
	}
	if len(opens) != 2 {
		t.Fatalf("the gate saw %d consultations about this open, want the name and the object: %+v",
			len(opens), asked)
	}
	if opens[0].Path != link || opens[0].Resolved != "" {
		t.Errorf("the first consultation is %+v, want the name as written and nothing resolved", opens[0])
	}
	if opens[1].Path != physical {
		t.Errorf("the second consultation asks about %q, want the object at %q — "+
			"a rule must match what was reached", opens[1].Path, physical)
	}
	if opens[1].Resolved == "" {
		t.Error("the second consultation is indistinguishable from the first; a prompting " +
			"gate cannot tell it is being asked again about one open")
	}
	if opens[0].ID != opens[1].ID || opens[0].ID != access.Action.ID {
		t.Errorf("ids %q, %q and %q, want one action", opens[0].ID, opens[1].ID, access.Action.ID)
	}
}

// TestAnOrdinaryOpenResolvesNowhere, which is the other half of a field that
// means anything: it is empty when the name was the object's own name, so a
// consumer can read a non-empty value as news.
func TestAnOrdinaryOpenResolvesNowhere(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, []byte("ordinary\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var asked []Action
	var events []Event
	if out, _ := run(t, "cat <"+plain, func(r *Runner) {
		allowingEverything(&asked)(r)
		collecting(&events)(r)
	}); !strings.Contains(out, "ordinary") {
		t.Fatalf("out = %q", out)
	}
	for _, e := range events {
		if e.Action.Resolved != "" {
			t.Errorf("an ordinary access says it resolved to %q: %+v", e.Action.Resolved, e.Action)
		}
	}
	for _, a := range asked {
		if a.Resolved != "" {
			t.Errorf("an ordinary consultation says it resolved to %q: %+v", a.Resolved, a)
		}
	}
}

// TestAListingThroughALinkRecordsWhereItWent. A directory is the loudest
// oracle the filesystem has, so its record is the one worth being complete.
//
// Its access record is written once the descriptor is in hand rather than
// beside the consultation, unlike its neighbors in fsgate.go — which is what
// lets it say this at all, and is the only reason that file has two shapes.
func TestAListingThroughALinkRecordsWhereItWent(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "entry"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	var asked []Action
	var events []Event
	if out, _ := run(t, "echo "+link+"/*", func(r *Runner) {
		allowingEverything(&asked)(r)
		collecting(&events)(r)
	}); !strings.Contains(out, "entry") {
		t.Fatalf("out = %q, want the glob to have expanded", out)
	}
	for _, e := range events {
		if e.Kind == EventAccess && e.Action.Kind == ActionReadDir && e.Action.Path == link {
			if e.Action.Resolved != physical {
				t.Errorf("the listing record resolved to %q, want %q",
					e.Action.Resolved, physical)
			}
			return
		}
	}
	t.Errorf("no listing of %q reached the stream: %+v", link, events)
}

// TestTheScriptIsToldNothingMore, which is the asymmetry this deliberately
// leaves alone.
//
// The refusal a script sees names the path it wrote and never where the name
// went — reporting the target would hand it the one fact the rule exists to
// withhold, and would let it map a hidden directory one link at a time by
// asking to be refused. That asymmetry is between *parties*: the script is
// told one thing and the operator another. Resolved is entirely inside the
// operator's half, so filling it in makes the two records consistent with each
// other and changes nothing the script can see.
func TestTheScriptIsToldNothingMore(t *testing.T) {
	object, link := linkFarm(t)
	out, _ := run(t, "cat <"+link+" 2>&1", hidesSecrets(nil))
	if strings.Contains(out, "secret") || strings.Contains(out, filepath.Base(object)) {
		t.Errorf("the diagnostic disclosed where the link went: %q", out)
	}
	if !strings.Contains(out, filepath.Base(link)) {
		t.Errorf("the diagnostic must name the path as written, got %q", out)
	}
}

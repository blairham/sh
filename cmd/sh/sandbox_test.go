// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// End to end, through the flags a person types. Everything above this file
// tests a piece; this asks whether `sh -policy p script.sh` is a sandboxed
// shell, which is the thing #494 says has to become true before conformance
// or the wild sweep can exercise the boundary at all.
//
// The invocation is assembled exactly as main() assembles it — readOwnFlags,
// pickDialect, installSeams, driver.MainArgs — because the failure this
// guards against is wiring that exists and is not reached. A shell that was
// never handed its gate looks precisely like a shell that works.

type outcome struct {
	out, errs string
	code      int
}

func sandboxed(t *testing.T, dialect string, args ...string) outcome {
	t.Helper()
	// Through run(), which is main() with the streams passed in — not through
	// a copy of what main does. A helper that reassembled the invocation
	// itself would grade the helper: the mistake this whole file exists to
	// catch is wiring that is present and not reached, and a test that never
	// enters main's own path cannot see it.
	argv := []string{"sh"}
	if dialect != "" {
		argv = append(argv, "-dialect", dialect)
	}
	argv = append(argv, args...)
	var out, errs bytes.Buffer
	code := run(argv, &out, &errs)
	return outcome{out: out.String(), errs: errs.String(), code: code}
}

func writePolicy(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "p.policy")
	src := "version 1\n" + strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestAPolicyRefusesWhatItDoesNotMention is the posture, seen from the outside.
//
// Nothing is allowed, so the exec fails and the probe answers as a missing
// path does. Both shapes are in one script on purpose: they are the two ends
// of the denial spectrum the design settles, and running them together is the
// cheapest way to see that a refusal is loud where a command did not run and
// silent where a question was asked.
func TestAPolicyRefusesWhatItDoesNotMention(t *testing.T) {
	t.Parallel()
	p := writePolicy(t, "default deny")
	got := sandboxed(t, "core", "-policy", p, "-c",
		"if [ -f /etc/hosts ]; then echo visible; else echo hidden; fi\nnosuchprogram\necho status=$?\n")
	if !strings.Contains(got.out, "hidden") {
		t.Errorf("out = %q, want a refused probe to read as a path that is not there", got.out)
	}
	if !strings.Contains(got.out, "status=126") {
		t.Errorf("out = %q, want a refused exec to fail the command", got.out)
	}
	if !strings.Contains(got.errs, "refused") {
		t.Errorf("err = %q, want the refused exec reported", got.errs)
	}
	// And the probe said nothing, which is the half that matters: a refusal
	// that identified itself would be an oracle for what the policy hides.
	if strings.Contains(got.errs, "/etc/hosts") {
		t.Errorf("err = %q, want a refused probe to name nothing", got.errs)
	}
}

// TestAnAllowedScriptRunsNormally is the other half, and without it the test
// above would pass for a shell that simply does not work.
func TestAnAllowedScriptRunsNormally(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data"), []byte("contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := writePolicy(t, "default deny", "allow read "+dir+"/**", "allow write "+dir+"/out/**")
	if err := os.Mkdir(filepath.Join(dir, "out"), 0o700); err != nil {
		t.Fatal(err)
	}
	got := sandboxed(t, "core", "-policy", p, "-c",
		"if [ -f "+dir+"/data ]; then echo visible; else echo hidden; fi\n"+
			"echo written > "+dir+"/out/f\n"+
			"read line < "+dir+"/data\necho got=$line\n")
	if got.code != 0 {
		t.Fatalf("status %d, err = %q", got.code, got.errs)
	}
	if !strings.Contains(got.out, "visible") || !strings.Contains(got.out, "got=contents") {
		t.Errorf("out = %q, want the allowed read to work", got.out)
	}
	body, err := os.ReadFile(filepath.Join(dir, "out", "f"))
	if err != nil || string(body) != "written\n" {
		t.Errorf("the allowed write produced %q, %v", body, err)
	}
}

// TestAReadRuleDoesNotGrantAWrite is the finest distinction in the format,
// asserted where it is easiest to get wrong: through a redirect, which is the
// only way a script destroys a file.
func TestAReadRuleDoesNotGrantAWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := writePolicy(t, "default deny", "allow read "+dir+"/**")
	target := filepath.Join(dir, "f")
	got := sandboxed(t, "core", "-policy", p, "-c", "echo x > "+target+"\necho status=$?\n")
	if _, err := os.Stat(target); err == nil {
		t.Error("a policy allowing only reads created a file")
	}
	if strings.Contains(got.out, "status=0") {
		t.Errorf("out = %q, want the refused redirect to have failed", got.out)
	}
}

// TestTheScriptOperandIsInsideTheBoundary is the consequence a person meets
// first, so it is written down as a test rather than only as prose.
//
// The path came from the invocation, which is where the script's own path
// comes from, and internal/boundary put that access inside the gate
// deliberately. So a default-deny policy has to name the script it is about
// to run — which is correct, since the person who wrote the policy is the
// person who named the script.
func TestTheScriptOperandIsInsideTheBoundary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	script := filepath.Join(dir, "case.sh")
	if err := os.WriteFile(script, []byte("echo ran\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	silent := writePolicy(t, "default deny")
	got := sandboxed(t, "core", "-policy", silent, script)
	if strings.Contains(got.out, "ran") {
		t.Errorf("out = %q, want a script the policy does not mention to be unreadable", got.out)
	}
	if got.code == 0 {
		t.Error("status 0 for a script that was never read")
	}

	naming := writePolicy(t, "default deny", "allow read "+dir+"/**")
	got = sandboxed(t, "core", "-policy", naming, script)
	if got.code != 0 || !strings.Contains(got.out, "ran") {
		t.Errorf("out = %q status = %d err = %q, want the named script to run", got.out, got.code, got.errs)
	}
}

// TestAnExecAllowlistFindsWhatItPermits is the one implication in the engine,
// exercised the way a script reaches it: through a PATH search, which stats
// every candidate before it runs one.
func TestAnExecAllowlistFindsWhatItPermits(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	prog := filepath.Join(dir, "hi")
	if err := os.WriteFile(prog, []byte("#!/bin/sh\necho hello\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	// The interpreter behind the shebang is the kernel's business rather than
	// the gate's, so the allowlist has to name it as well — which is the
	// footgun the design document names, seen from the useful side.
	p := writePolicy(t, "default deny", "allow exec "+dir+"/**", "allow exec /bin/sh")
	got := sandboxed(t, "core", "-policy", p, "-c", "PATH="+dir+"\nhi\necho status=$?\n")
	if !strings.Contains(got.out, "hello") {
		t.Errorf("out = %q err = %q, want the allowlisted program to be found and run", got.out, got.errs)
	}
}

// TestABadPolicyStopsTheShell, and stops it before anything runs. A policy
// that will not parse is not a shell that runs unsandboxed.
func TestABadPolicyStopsTheShell(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "bad.policy")
	if err := os.WriteFile(path, []byte("version 1\nallow read relative/**\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := sandboxed(t, "core", "-policy", path, "-c", "echo ran")
	if strings.Contains(got.out, "ran") {
		t.Error("the script ran with a policy that did not load")
	}
	if got.code != exitFailure {
		t.Errorf("status %d, want %d", got.code, exitFailure)
	}
	for _, want := range []string{path, "line 2", "absolute"} {
		if !strings.Contains(got.errs, want) {
			t.Errorf("err = %q, want it to mention %q", got.errs, want)
		}
	}
	missing := sandboxed(t, "core", "-policy", filepath.Join(t.TempDir(), "absent"), "-c", "echo ran")
	if strings.Contains(missing.out, "ran") {
		t.Error("the script ran with a policy file that is not there")
	}
}

// TestTheAuditStreamRecordsWhatHappened is the Sink half reaching a person,
// in the schema its consumers share.
func TestTheAuditStreamRecordsWhatHappened(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	log := filepath.Join(dir, "audit.jsonl")
	p := writePolicy(t, "default deny")
	got := sandboxed(t, "core", "-policy", p, "-audit", log, "-c", "[ -f /etc/hosts ]\nnosuchprogram\n")
	if got.code == 0 {
		t.Fatalf("expected a failing status, err = %q", got.errs)
	}
	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	var denied []string
	seq := int64(0)
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		var r struct {
			V      int    `json:"v"`
			Seq    int64  `json:"seq"`
			Event  string `json:"event"`
			Action string `json:"action"`
			Path   string `json:"path"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("record %q did not decode: %v", line, err)
		}
		if r.V != 1 {
			t.Errorf("record %q: v = %d, want 1", line, r.V)
		}
		seq++
		if r.Seq != seq {
			t.Errorf("record %q: seq = %d, want %d", line, r.Seq, seq)
		}
		if r.Event == "denied" {
			denied = append(denied, r.Action+" "+r.Path)
		}
	}
	if !slices.Contains(denied, "stat /etc/hosts") {
		t.Errorf("denied = %q, want the refused probe recorded — the script could not see it, so the log must",
			denied)
	}
	if !slices.ContainsFunc(denied, func(s string) bool { return strings.HasPrefix(s, "exec ") }) {
		t.Errorf("denied = %q, want the refused exec recorded", denied)
	}
}

// TestTheAuditStreamAppends, because an audit trail that erases the previous
// run on the next one is not one.
func TestTheAuditStreamAppends(t *testing.T) {
	t.Parallel()
	log := filepath.Join(t.TempDir(), "audit.jsonl")
	p := writePolicy(t, "default deny")
	for range 2 {
		sandboxed(t, "core", "-policy", p, "-audit", log, "-c", "[ -f /etc/hosts ]\n")
	}
	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(strings.TrimSpace(string(body)), "\n") + 1; n < 2 {
		t.Errorf("the log holds %d records after two runs; the second truncated the first", n)
	}
}

// TestAPolicyAndADenyComposeAsAnIntersection.
//
// Any refusal refuses, so adding a `-deny` to an existing policy can only
// narrow it. The opposite — a later flag widening what a policy allowed —
// would make the command line a way around the file.
func TestAPolicyAndADenyComposeAsAnIntersection(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{filepath.Join(dir, "a"), filepath.Join(sub, "b")} {
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p := writePolicy(t, "default deny", "allow read "+dir+"/**")
	script := "if [ -f " + dir + "/a ]; then echo a-visible; else echo a-hidden; fi\n" +
		"if [ -f " + sub + "/b ]; then echo b-visible; else echo b-hidden; fi\n"
	got := sandboxed(t, "core", "-policy", p, "-deny", sub, "-c", script)
	if !strings.Contains(got.out, "a-visible") {
		t.Errorf("out = %q, want what the policy allowed and -deny did not touch", got.out)
	}
	if !strings.Contains(got.out, "b-hidden") {
		t.Errorf("out = %q, want -deny to narrow what the policy allowed", got.out)
	}
}

// TestTheAnswerDoesNotDependOnTheDialect is the claim #494 calls a design
// error to break: the gate is dialect-blind.
//
// The import graph is asserted in internal/policy, which is the structural
// half. This is the behavioral half, and it is the one that would fail if
// somebody reached for the semantics vector inside a rule. The script is
// deliberately one every dialect agrees about, so that a difference in the
// output is a difference in the *policy* and not in the shell.
func TestTheAnswerDoesNotDependOnTheDialect(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	visible := filepath.Join(dir, "seen")
	hidden := filepath.Join(dir, "unseen")
	for _, name := range []string{visible, hidden} {
		if err := os.WriteFile(name, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p := writePolicy(t, "default deny", "allow read "+dir+"/**", "deny read "+hidden)
	script := "if [ -f " + visible + " ]; then echo one; else echo two; fi\n" +
		"if [ -f " + hidden + " ]; then echo three; else echo four; fi\n" +
		"if [ -d " + dir + " ]; then echo five; else echo six; fi\n" +
		"nosuchprogram\necho status=$?\n"

	const wantOut = "one\nfour\nfive\nstatus=126\n"
	for _, dialect := range []string{"core", "posix", "bash", "zsh", "ksh", "dash"} {
		got := sandboxed(t, dialect, "-policy", p, "-c", script)
		if got.out != wantOut {
			t.Errorf("%s: out = %q, want %q — a policy answer must not depend on the dialect",
				dialect, got.out, wantOut)
		}
	}
}

// TestAPolicyIsNeverDiscovered pins the rule about where a policy may come
// from, which is a rule about the *source* and cannot be relaxed by a policy.
//
// A policy nameable by the environment is replaceable by anything that can set
// the environment, and the sandboxed script is one of those — it needs only to
// invoke a nested shell. So there is no variable to read, and this asserts the
// absence by setting the names somebody might reach for and watching nothing
// happen.
func TestAPolicyIsNeverDiscovered(t *testing.T) {
	p := writePolicy(t, "default deny")
	for _, name := range []string{"SH_POLICY", "SHPOLICY", "POLICY"} {
		t.Setenv(name, p)
	}
	got := sandboxed(t, "core", "-c", "echo ran")
	if !strings.Contains(got.out, "ran") {
		t.Errorf("out = %q: a policy was picked up from the environment", got.out)
	}
}

// TestATraceAndAnAuditAreBothFed, because they are different jobs — one is
// watched by eye while a script runs, the other is kept — and a person asking
// for both must not silently get one.
func TestATraceAndAnAuditAreBothFed(t *testing.T) {
	t.Parallel()
	log := filepath.Join(t.TempDir(), "audit.jsonl")
	p := writePolicy(t, "default deny")
	got := sandboxed(t, "core", "-policy", p, "-trace-events", "-audit", log, "-c", "[ -f /etc/hosts ]\n")
	if !strings.Contains(got.errs, "trace: denied stat /etc/hosts") {
		t.Errorf("err = %q, want the human trace on standard error", got.errs)
	}
	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"event":"denied"`) {
		t.Errorf("log = %q, want the same refusal in the audit stream", body)
	}
}

// TestAuditToADashIsStandardError, which is the form that needs no file and is
// what a person reaches for first. A `-` taken as a file name would create one
// called `-` in the working directory, which is the classic version of this
// mistake and the one driver already had once with an operand.
func TestAuditToADashIsStandardError(t *testing.T) {
	t.Parallel()
	p := writePolicy(t, "default deny")
	got := sandboxed(t, "core", "-policy", p, "-audit", "-", "-c", "[ -f /etc/hosts ]\n")
	if !strings.Contains(got.errs, `"event":"denied"`) {
		// A dash read as a path opens a file called `-` instead, and the
		// stream a person was watching stays empty — which is how this
		// mistake is always noticed, and never before it has made a file.
		t.Errorf("err = %q, want the audit stream on standard error", got.errs)
	}
}

// The audit stream says which run it came from and which action each record is
// about, which is what makes it joinable to anything else.
//
// A shipped binary is where this has to be shown. The fields exist in interp
// and are written by internal/event, and until a flag put them in a file
// nothing a person could point at ever carried one — which is the same argument
// that put -policy and -audit here in the first place.
func TestTheAuditStreamCarriesAnIdentity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	log := filepath.Join(dir, "audit.jsonl")
	out := filepath.Join(dir, "written")
	got := sandboxed(t, "core", "-audit", log, "-c",
		"/bin/echo one > "+out+"\n/bin/echo two\n")
	if got.code != 0 {
		t.Fatalf("status %d: %s", got.code, got.errs)
	}
	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	type record struct {
		Seq      int64  `json:"seq"`
		Session  string `json:"session"`
		ActionID string `json:"actionId"`
		Event    string `json:"event"`
		Action   string `json:"action"`
		Path     string `json:"path"`
	}
	var records []record
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		var r record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("record %q did not decode: %v", line, err)
		}
		records = append(records, r)
	}
	if len(records) < 4 {
		t.Fatalf("got %d records, want the two commands and the redirect", len(records))
	}
	// One run, one session, on every line: a file several shells append to is
	// unreadable without it.
	session := records[0].Session
	if session == "" {
		t.Fatal("the stream carries no session, so nothing can be joined to it")
	}
	for _, r := range records {
		if r.Session != session {
			t.Errorf("a %s record says session %q, want %q", r.Event, r.Session, session)
		}
		if r.ActionID == "" {
			t.Errorf("a %s %s record carries no action id", r.Event, r.Action)
		}
	}
	// A command's start and its end share an id, and two commands do not. That
	// is the pairing ordering could not do: a background job and each half of a
	// pipeline write from their own goroutines, so the record after a start is
	// very often another command's.
	byID := map[string][]string{}
	for _, r := range records {
		if r.Action == "exec" {
			byID[r.ActionID] = append(byID[r.ActionID], r.Event)
		}
	}
	if len(byID) != 2 {
		t.Fatalf("two commands ran under %d action ids: %v", len(byID), byID)
	}
	for id, events := range byID {
		if !slices.Contains(events, "command-start") || !slices.Contains(events, "command-end") {
			t.Errorf("action %s has events %v, want a start and an end under one id", id, events)
		}
	}
	// And the id is not the sequence number. They are different questions —
	// which action, and where in the stream — and a consumer joining on seq
	// would pair a start with whatever was emitted next.
	for _, r := range records {
		if r.ActionID == strconv.FormatInt(r.Seq, 10) && r.Event == "command-end" {
			t.Errorf("a command-end has actionId %q and seq %d: the two have collapsed into one number",
				r.ActionID, r.Seq)
		}
	}
}

// Two runs writing to one audit file are two sessions in it.
//
// This is the case the session field exists for. The file is appended rather
// than truncated, so it holds more than one shell's records, and without an
// identity per run a reader has one undifferentiated stream in which two shells'
// action ids collide.
func TestTwoRunsInOneAuditFileAreTwoSessions(t *testing.T) {
	t.Parallel()
	log := filepath.Join(t.TempDir(), "audit.jsonl")
	for range 2 {
		if got := sandboxed(t, "core", "-audit", log, "-c", "/bin/echo hi"); got.code != 0 {
			t.Fatalf("status %d: %s", got.code, got.errs)
		}
	}
	body, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		var r struct {
			Session string `json:"session"`
		}
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("record %q did not decode: %v", line, err)
		}
		seen[r.Session] = true
	}
	if len(seen) != 2 {
		t.Errorf("the file holds %d session(s), want one per run: %v", len(seen), seen)
	}
}

// A policy written against the name a person types protects the place they
// meant, through the binary they run.
//
// This is #538 end to end. On macOS `/tmp` is another name for `/private/tmp`,
// and a shell that has resolved a path — which `cd -P` does, and which is the
// only way `pwd` can answer honestly — presents the physical one to the gate. A
// rule written with the name the person uses walked straight past it, and the
// script saw a refusal that never happened.
func TestAPolicyUnderAPlatformAliasProtectsBothNames(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("no platform aliases here — see internal/policy/alias_other.go")
	}
	dir := t.TempDir()
	secret := filepath.Join("/tmp", "sh-538-"+filepath.Base(dir))
	if err := os.Mkdir(secret, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(secret) })
	if err := os.WriteFile(filepath.Join(secret, "f"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// The rule names the directory the way a person would, and the script
	// reaches it the way a shell that resolved the path does.
	p := writePolicy(t, "default allow\ndeny path "+secret+"/**")
	src := "cd -P " + secret + "\n[ -f f ] && echo found || echo hidden\n" +
		"[ -f " + secret + "/f ] && echo found-lexical || echo hidden-lexical\n"
	got := sandboxed(t, "core", "-policy", p, "-c", src)
	if !strings.Contains(got.out, "hidden\n") {
		t.Errorf("out = %q, want the physical path refused too — the rule named the same place", got.out)
	}
	if !strings.Contains(got.out, "hidden-lexical") {
		t.Errorf("out = %q, want the name as written still refused", got.out)
	}
}

// And what the rule turned into is said out loud, under the flag whose job is
// showing what the boundary is doing.
//
// A rule that quietly covers a second path is a rule whose meaning is not in
// the file it came from, and a policy is reviewed by somebody reading it.
func TestATraceReportsWhatAPolicyNormalized(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("no platform aliases here")
	}
	p := writePolicy(t, "default allow\ndeny path /tmp/nothing-this-test-uses/**")
	got := sandboxed(t, "core", "-policy", p, "-trace-events", "-c", ":")
	want := "policy: deny path /tmp/nothing-this-test-uses/** (also /private/tmp/nothing-this-test-uses/**)"
	if !strings.Contains(got.errs, want) {
		t.Errorf("err =\n%s\nwant %s", got.errs, want)
	}
	// Not on the shell's own output, and not disguised as an event: a consumer
	// reading the trace filters on the prefix, and this is something the policy
	// is rather than something the shell did.
	if strings.Contains(got.out, "policy:") {
		t.Errorf("out = %q, want the report kept off the shell's output stream", got.out)
	}
	if strings.Contains(got.errs, "trace: policy") {
		t.Errorf("err = %q, want the report not to pose as an event", got.errs)
	}
}

// Nothing is reported for a policy that normalized nothing, so the line means
// something when it appears.
func TestATraceReportsNothingWhenNothingNormalized(t *testing.T) {
	p := writePolicy(t, "default allow\ndeny path /srv/nothing-this-test-uses/**\ndeny signal")
	got := sandboxed(t, "core", "-policy", p, "-trace-events", "-c", ":")
	if strings.Contains(got.errs, "policy:") {
		t.Errorf("err = %q, want no report for a policy that named no alias", got.errs)
	}
}

// And without -trace-events there is no report at all: a shell running under a
// policy is not a shell that chatters about it.
func TestAPolicyIsQuietWithoutTheTraceFlag(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("no platform aliases here")
	}
	p := writePolicy(t, "default allow\ndeny path /tmp/nothing-this-test-uses/**")
	got := sandboxed(t, "core", "-policy", p, "-c", "echo ran")
	if !strings.Contains(got.out, "ran") {
		t.Fatalf("out = %q, want the script to have run", got.out)
	}
	if got.errs != "" {
		t.Errorf("err = %q, want silence", got.errs)
	}
}

// A -deny value is normalized and reported exactly as a file's rule is.
//
// The two routes share the rule language, so they have to share this too: a
// flag that quietly covered a second path while the file said so out loud would
// be the two vocabularies coming apart again in a place nobody looks.
func TestATraceReportsWhatADenyValueNormalized(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("no platform aliases here")
	}
	got := sandboxed(t, "core",
		"-deny", "/tmp/nothing-this-test-uses", "-trace-events", "-c", ":")
	want := "policy: deny path /tmp/nothing-this-test-uses/** " +
		"(also /private/tmp/nothing-this-test-uses/**)"
	if !strings.Contains(got.errs, want) {
		t.Errorf("err =\n%s\nwant %s", got.errs, want)
	}
}

// And the flag protects both names, which is the half the report only claims.
func TestADenyValueUnderAPlatformAliasProtectsBothNames(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("no platform aliases here")
	}
	dir := t.TempDir()
	secret := filepath.Join("/tmp", "sh-538-deny-"+filepath.Base(dir))
	if err := os.Mkdir(secret, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(secret) })
	if err := os.WriteFile(filepath.Join(secret, "f"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	src := "cd -P " + secret + "\n[ -f f ] && echo found || echo hidden\n"
	got := sandboxed(t, "core", "-deny", secret, "-c", src)
	if !strings.Contains(got.out, "hidden") {
		t.Errorf("out = %q, want the physical path refused too", got.out)
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// `prompt import`, end to end, at a real terminal.
//
// The only place it can be asked, for the reason promptshowpty_test.go gives:
// the `prompt` function exists only in an interactive session.
//
// What this file is about is the half internal/promptimport's own tests
// cannot reach. Those are handed a parameter namespace; this one hands the
// shell a **program** and asks what came out — which is the spec's decision
// that a configuration of this kind is *evaluated rather than parsed*, and
// the reason the decision was taken.

// importFixture is a configuration that a text reader cannot read, and it is
// written to be exactly that rather than to be realistic.
//
// Two shapes, and each is one the spec names. A **gate** decides whether any
// of the configuration is applied at all, so a reader that matched
// assignments out of the text would carry settings a real shell never
// applied. And **two of the settings have names that are not in the file** —
// they are built and assigned through `eval`, which is what a generated
// configuration does with brace expansion and what no pattern match can see.
const importFixture = `
if [ "${THEME_FIXTURE_VERSION:-}" = "" ]; then
	# The gate. Nothing below is applied, exactly as a real configuration's
	# version check would refuse an old shell.
	POWERLEVEL9K_LEFT_PROMPT_ELEMENTS="gated"
	return 0
fi
POWERLEVEL9K_LEFT_PROMPT_ELEMENTS="dir newline prompt_char"
POWERLEVEL9K_DIR_FOREGROUND=31
for state in OK ERROR; do
	eval "POWERLEVEL9K_PROMPT_CHAR_${state}_VIINS_FOREGROUND=7"
done
`

// TestPromptImportEvaluatesTheConfigurationRatherThanReadingIt drives a
// session, converts a program, and checks what a text reader could not have
// produced.
func TestPromptImportEvaluatesTheConfigurationRatherThanReadingIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	source := filepath.Join(home, "theme.sh")
	if err := os.WriteFile(source, []byte(importFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, "prompt.conf")

	control, tty := terminal(t)
	sh := shell()
	// One axis answered, the way driver's own shell() answers the ones its
	// tests are not about: whether the lines of `eval`'s text continue the
	// caller's. The fixture builds two setting names through `eval`, which
	// is the whole point of it, and a shell with no answer refuses the word
	// rather than running it.
	sh.Semantics.EvalTextContinuesTheCallersLines = interp.Yes
	sh.Stdin, sh.Stdout, sh.Stderr = tty, tty, tty

	drawn := watch(t, control, defaultPrompt)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"testsh"}) }()
	drawn.awaitReadyForInput(t)

	// Gated shut first. The conversion is of what the shell *applied*, so a
	// configuration that declined to apply itself converts to nothing — and
	// the report says which, because an empty import and an empty file look
	// identical afterwards.
	write(t, control, "prompt import p10k "+source+" "+destination+"\r")
	drawn.await(t, "settings carried")
	if got := readFile(t, destination); strings.Contains(got, "dir newline prompt_char") {
		t.Errorf("a gated configuration was converted anyway:\n%s", got)
	}

	// And open. Same file, same command, one variable different.
	write(t, control, "THEME_FIXTURE_VERSION=5\r")
	write(t, control, "prompt import p10k "+source+" "+destination+"; echo done-$((6 * 7))\r")
	drawn.await(t, "done-42")

	got := readFile(t, destination)
	if !strings.Contains(got, "LEFT_ELEMENTS = dir newline prompt_char") {
		t.Errorf("the elements did not survive the conversion:\n%s", got)
	}
	if !strings.Contains(got, "DIR_FOREGROUND = 31") {
		t.Errorf("a plain setting did not survive the conversion:\n%s", got)
	}
	// The two settings whose names are not in the file. This is the whole
	// claim: a reader matching assignments out of the text would find
	// neither, because neither name is written anywhere.
	for _, want := range []string{
		"PROMPT_CHAR_OK_FOREGROUND = 7",
		"PROMPT_CHAR_ERROR_FOREGROUND = 7",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("a name the file never spells did not arrive (%s):\n%s", want, got)
		}
	}

	// And the session is not carrying the configuration afterwards. Sourcing
	// two hundred settings into a person's live shell is not what they asked
	// for when they asked to convert a file.
	write(t, control, "echo leaked-[${POWERLEVEL9K_DIR_FOREGROUND-}]\r")
	drawn.await(t, "leaked-[]")

	endPromptSession(t, control, done, drawn)
}

// TestPromptImportRefusesWithNoDestination is the other half of "named or
// nowhere": inventing a location for somebody is what the block store
// deliberately stopped doing.
func TestPromptImportRefusesWithNoDestination(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	source := filepath.Join(home, "theme.sh")
	if err := os.WriteFile(source, []byte("POWERLEVEL9K_DIR_FOREGROUND=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	control, tty := terminal(t)
	sh := shell()
	sh.Stdin, sh.Stdout, sh.Stderr = tty, tty, tty

	drawn := watch(t, control, defaultPrompt)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"testsh"}) }()
	drawn.awaitReadyForInput(t)

	write(t, control, "prompt import p10k "+source+"; echo status-$?\r")
	drawn.await(t, "status-2")
	if !strings.Contains(drawn.text(), "no destination") {
		t.Errorf("the refusal did not say what was missing:\n%s", drawn.text())
	}

	// And SH_PROMPT_CONFIG is the other way to name one, which is what makes
	// the imported file the one the next prompt reads.
	write(t, control, "SH_PROMPT_CONFIG="+filepath.Join(home, "named.conf")+"\r")
	write(t, control, "prompt import p10k "+source+"\r")
	drawn.await(t, "settings carried")
	if _, err := os.Stat(filepath.Join(home, "named.conf")); err != nil {
		t.Errorf("the configured destination was not written: %v", err)
	}

	endPromptSession(t, control, done, drawn)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading what the import wrote: %v", err)
	}
	return string(b)
}

// TestAPolicyRefusesTheImportsWrite is the gate, and it is a row that can
// fail rather than a claim that it cannot.
//
// Both files a conversion touches are named by a person at a prompt, which
// is exactly what a policy is about: the source is read and the destination
// is written. So a shell under one must refuse both where the policy does
// not permit them — and the check that matters is the side effect and not
// the message, per the rule `make acp`'s rows are written to.
//
// The allowed case runs too. A check that sees a refusal without ever
// having seen the same action succeed cannot tell a working gate from a
// shell that fell over.
func TestAPolicyRefusesTheImportsWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	source := filepath.Join(home, "theme.sh")
	if err := os.WriteFile(source, []byte("POWERLEVEL9K_LEFT_PROMPT_ELEMENTS=dir\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	refused := filepath.Join(home, "refused.conf")
	allowed := filepath.Join(home, "allowed.conf")

	control, tty := terminal(t)
	sh := shell()
	// Refuses exactly one destination, so the other one landing is what
	// says the gate is a gate rather than a shell that writes nothing.
	sh.Gate = &recorder{deny: func(a interp.Action) bool {
		return a.Write && strings.HasSuffix(a.Path, "refused.conf")
	}}
	sh.Stdin, sh.Stdout, sh.Stderr = tty, tty, tty

	// A gated session draws a mark in front of its prompt, so the wait is
	// for the prompt that session actually draws — see driver.AddGate.
	drawn := watch(t, control, "(sandboxed) "+defaultPrompt)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"testsh"}) }()
	drawn.awaitReadyForInput(t)

	write(t, control, "prompt import p10k "+source+" "+refused+"; echo status-$?\r")
	drawn.await(t, "status-1")
	if _, err := os.Stat(refused); err == nil {
		t.Error("the refused destination was written anyway")
	}

	write(t, control, "prompt import p10k "+source+" "+allowed+"\r")
	drawn.await(t, "settings carried")
	if _, err := os.Stat(allowed); err != nil {
		t.Errorf("the permitted destination was not written: %v", err)
	}

	endPromptSession(t, control, done, drawn)
}

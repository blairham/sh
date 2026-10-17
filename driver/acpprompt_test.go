// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// `--acp-prompt`, on the front end every binary is made of: the dialect
// binaries spell it long form only, and it reaches the hook beside the agent's
// command line, which it must not eat (#6937).
func TestTheACPPromptFlagReachesTheHook(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"testsh", "--acp-prompt", "-", "--acp-connect", "npx", "pkg"}, "-"},
		{[]string{"testsh", "--acp-prompt=task.md", "--acp-connect", "npx", "pkg"}, "task.md"},
		{[]string{"testsh", "--acp-connect", "npx", "pkg"}, ""},
	} {
		var out, errs strings.Builder
		sh := gatedShell(&out, &errs)
		var gotPrompt string
		var gotArgv []string
		sh.ConnectACP = func(_ driver.Shell, _ bool, _, prompt string, argv []string) int {
			gotPrompt, gotArgv = prompt, argv
			return 7
		}
		if code := driver.MainArgs(sh, tc.args); code != 7 {
			t.Fatalf("%v: status %d, want the hook's own 7; stderr %q", tc.args, code, errs.String())
		}
		if gotPrompt != tc.want {
			t.Errorf("%v: prompt %q, want %q", tc.args, gotPrompt, tc.want)
		}
		if !slices.Equal(gotArgv, []string{"npx", "pkg"}) {
			t.Errorf("%v: argv %v, want the agent's command line", tc.args, gotArgv)
		}
	}
}

// Refused rather than ignored: with no file there is no prompt, and with no
// agent the line would go on to run a shell that reads the prompt as a script.
func TestTheACPPromptFlagIsRefusedWhereItCannotMeanAnything(t *testing.T) {
	for _, args := range [][]string{
		{"testsh", "--acp-prompt"},
		{"testsh", "--acp-prompt=", "--acp-connect", "npx"},
		{"testsh", "--acp-prompt", "-", "-c", "echo RAN"},
	} {
		var out, errs strings.Builder
		sh := gatedShell(&out, &errs)
		ran := false
		sh.ConnectACP = func(driver.Shell, bool, string, string, []string) int { ran = true; return 0 }
		if code := driver.MainArgs(sh, args); code == 0 {
			t.Errorf("%v exited 0, want a refusal", args)
		}
		if ran || strings.Contains(out.String(), "RAN") {
			t.Errorf("%v ran something anyway: stdout %q", args, out.String())
		}
		if !strings.Contains(errs.String(), "--acp-prompt") {
			t.Errorf("%v: stderr %q does not name the flag", args, errs.String())
		}
	}
}

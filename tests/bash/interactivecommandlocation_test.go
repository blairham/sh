// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// TestAnInteractiveCommandStringIsLocatedAsAPrompt — under `-i -c` bash
// writes its run-time messages with no line, as at a prompt, where plain `-c`
// names `line 1`. Measured 2026-10-04 on bash 5.3.20; see invocation.md.
func TestAnInteractiveCommandStringIsLocatedAsAPrompt(t *testing.T) {
	src := `nosuch; cd /nonexistent-5719`
	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"bash", "-f", "-i", "-c", src}, []string{
			"bash: nosuch: command not found\n", "bash: cd: /nonexistent-5719: ",
		}},
		{[]string{"bash", "-c", src}, []string{
			"bash: line 1: nosuch: command not found\n", "bash: line 1: cd: /nonexistent-5719: ",
		}},
	} {
		var out, errs bytes.Buffer
		driver.MainArgs(bashShell(&out, &errs), c.args)
		for _, w := range c.want {
			if !strings.Contains(errs.String(), w) {
				t.Errorf("%v: said %q, want %q", c.args, errs.String(), w)
			}
		}
	}
}

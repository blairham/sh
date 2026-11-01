// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"testing"
)

// TestTheBuiltinMemoForgetsEveryTableWrite asks lookupBuiltin about a name,
// changes one of the things its answer depends on, and asks again: every
// writer of a table it reads, and both runner words in its key. A memo that
// missed one would give the answer from before the change (#6353).
func TestTheBuiltinMemoForgetsEveryTableWrite(t *testing.T) {
	t.Parallel()
	r := newTestRunner(t, &Runner{})
	own := Builtin(func(*Runner, context.Context, []string) int { return 0 })
	steps := []struct {
		what   string
		change func()
		name   string
		want   bool
	}{
		{"nothing", func() {}, "echo", true},
		{"SetBuiltinEnabled off", func() { r.SetBuiltinEnabled("echo", false) }, "echo", false},
		{"the reserved command", func() { r.reservedCommand = "echo" }, "echo", true},
		{"the reserved command put back", func() { r.reservedCommand = "" }, "echo", false},
		{"SetBuiltinEnabled on", func() { r.SetBuiltinEnabled("echo", true) }, "echo", true},
		{"`enable -n`", func() { biEnable(r, context.Background(), []string{"-n", "echo"}) }, "echo", false},
		{"`enable`", func() { biEnable(r, context.Background(), []string{"echo"}) }, "echo", true},
		{"SetBuiltinWithdrawn", func() { r.SetBuiltinWithdrawn("echo", true) }, "echo", false},
		{"SetBuiltinWithdrawn back", func() { r.SetBuiltinWithdrawn("echo", false) }, "echo", true},
		{"Unregister", func() { r.Unregister("echo") }, "echo", false},
		{"Register", func() { r.Register("echo", own) }, "echo", true},
		{"Register a new name", func() { r.Register("memoed", own) }, "memoed", true},
		{"RegisterPreludeCommand", func() { r.RegisterPreludeCommand("prelude-only", own) }, "prelude-only", false},
		{"a speaker", func() { r.speaker = "sh" }, "prelude-only", true},
		{
			"RegisterPreludeCommand with a speaker",
			func() { r.RegisterPreludeCommand("prelude-too", own) },
			"prelude-too",
			true,
		},
		{"SetPromptEngine", func() { r.SetPromptEngine(own) }, promptEngineCommand, true},
		{"no speaker", func() { r.speaker = "" }, promptEngineCommand, false},
	}
	for _, s := range steps {
		// Asked first, so the memo holds the answer from before the change.
		r.lookupBuiltin(s.name)
		s.change()
		if _, got := r.lookupBuiltin(s.name); got != s.want {
			t.Errorf("after %s, %q is a builtin: %v, want %v", s.what, s.name, got, s.want)
		}
		if _, fresh := r.resolveBuiltin(s.name); fresh != s.want {
			t.Errorf("after %s, the tables themselves answer %v for %q, so this step tests nothing",
				s.what, fresh, s.name)
		}
	}
}

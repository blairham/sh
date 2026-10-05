// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// `$PREBUFFER`, push-line-or-edit and push-line, on a real terminal (#5931).
//
// Measured 2026-10-04 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal
// with this startup file: typing `if true; then`, Return, `echo x`, then ^Xs
// records `[if true; then+|echo x|6]` — the line already entered is in
// `$PREBUFFER`. ^Xp (push-line-or-edit) ends that read and the next one
// starts at the main prompt holding the whole command, so ^Xs records
// `[|if true; then+echo x|20]`, and nothing after `zle push-line-or-edit` in
// ^Xp ran. At the main prompt ^Xp is push-line, and the widget goes on —
// `ran` — and `echo y` put aside comes back on the prompt after the next
// command, `[|echo y|6]`. Before the fix this
// shell had no `$PREBUFFER` at all and `zle push-line-or-edit` was status 1,
// so the rows read `[|echo x|6]`, `ran` and `[|echo x|6]`.
func TestPushLineAndThePrebuffer(t *testing.T) {
	control, screen := widgetSession(t, `PS2='C> '
s() { seen+="[${PREBUFFER//$'\n'/+}|${BUFFER//$'\n'/+}|$CURSOR]" }; zle -N s; bindkey '^Xs' s
p() { zle push-line-or-edit; seen+=ran }; zle -N p; bindkey '^Xp' p
r() { BUFFER="SEEN$seen END" }; zle -N r; bindkey '^Xr' r
c() { BUFFER= }; zle -N c; bindkey '^Xc' c
`)
	send := func(keys string) {
		t.Helper()
		if _, err := control.WriteString(keys); err != nil {
			t.Fatalf("typing %q: %v", keys, err)
		}
	}
	await := func(want string) {
		t.Helper()
		if err := screen.Await(want, widgetBudget); err != nil {
			t.Fatalf("want %q: %v\n%q", want, err, smoke.LastLines(screen.Text(), 6))
		}
	}
	send("if true; then\r")
	await("C> ")
	// The pull-back ends the read and the next starts at the main prompt;
	// the keys after it wait for that prompt.
	send("echo x\x18s\x18p")
	await(widgetMark)
	send("\x18s\x18r")
	await("SEEN[if true; then+|echo x|6][|if true; then+echo x|20] END")
	// And push-line at the main prompt: put aside, a command run, back.
	send("\x18cecho y\x18pecho Z$((1+1))\r")
	await("Z2\r\n")
	await(widgetMark)
	send("\x18s\x18r")
	await("SEEN[if true; then+|echo x|6][|if true; then+echo x|20]ran[|echo y|6] END")
	send("\x18c")
}

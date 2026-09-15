// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"io"
	"strings"
	"testing"
)

// Handing the terminal over is what makes the translation's memory wrong, so
// taking it back is where the memory is dropped.
//
// The failure this pins is #2861, and the shape of it is why the assertion is
// on the bytes written *after* the handover rather than on the whole stream.
// Between the editor's last redraw — which ends by returning the carriage —
// and the newline the loop writes to get off an echoed `^C`, a command ran:
// its output went straight to the terminal and the terminal echoed the keys
// itself, and neither reached this writer. Read back from the buffer those two
// writes are adjacent and the newline looks whole; on the screen they are two
// hundred columns apart and it is not.
//
// So the property is that the newline carries its own return, whatever this
// stream happened to write before the terminal went away.
func TestTakingTheTerminalBackDropsWhatTheStreamsRemember(t *testing.T) {
	_, tty := openTerminal(t)
	var out, errs strings.Builder
	s := Shell{In: tty, Out: translating(&out), Err: translating(&errs)}
	state, err := makeRaw(tty)
	if err != nil {
		t.Skipf("no raw mode: %v", err)
	}
	t.Cleanup(func() { _ = state.restore() })

	// How the editor ends a line: the bracketed-paste sequence finishes by
	// returning the carriage, so this stream's last byte is a `\r`.
	s.write(pasteModeOff)
	s.errf("%s", pasteModeOff)
	drawn, reported := out.Len(), errs.Len()

	s.inLineDiscipline(state, func() {
		// A command's output and the terminal's echo of the ^C that ended it.
		// Written to the terminal and not through the session's streams,
		// which is the whole of the fault: the screen ends two columns in and
		// this writer never saw a byte of it.
		_, _ = io.WriteString(tty, "tick\r\n^C")
	})

	s.write("\n")
	s.errf("\n")
	for _, c := range []struct {
		name, got string
	}{
		{"the session's output", out.String()[drawn:]},
		{"the session's diagnostics", errs.String()[reported:]},
	} {
		if want := "\r\n"; c.got != want {
			t.Errorf("%s wrote %q after the terminal came back, want %q: the prompt "+
				"after it starts where the interrupted command's echo ended", c.name, c.got, want)
		}
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// Shell.In is an io.Reader, so a session can be driven by something that is
// not a descriptor at all. The editor still needs one and still says so: this
// takes runPlain, which is the route a pipe has always taken.
//
// It is here because the widening is this package's half of #787. driver's
// Stdin became an io.Reader so an ACP session's input can be a question put
// over the connection, and handing that front end a nil descriptor for `-i`
// would have been a prompt loop that read nothing — a session that ends at
// once and looks exactly like one that worked.
func TestASessionRunsOnAReaderThatIsNotAFile(t *testing.T) {
	var out, errs strings.Builder
	r := newTestRunner(map[string]string{"PS1": "$ ", "PS2": "> "})
	r.Stdout = &out
	s := Shell{
		Runner: r,
		In:     strings.NewReader("echo one\nfor i in a b\ndo\necho $i\ndone\n"),
		Out:    &out,
		Err:    &errs,
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if want := "one\na\nb\n"; out.String() != want {
		t.Errorf("output %q, want %q", out.String(), want)
	}
	// A prompt for each line, the continuation for the three inside the
	// construct, and one more asking for what never came.
	if want := "$ $ > > > $ "; errs.String() != want {
		t.Errorf("prompts %q, want %q", errs.String(), want)
	}
}

// And the one question this package asks about a descriptor answers no for a
// reader, which is what sends the session to runPlain in the first place.
func TestInFileIsNilForAReaderThatIsNotAFile(t *testing.T) {
	s := Shell{In: strings.NewReader("")}
	if f := s.inFile(); f != nil {
		t.Errorf("inFile() = %v, want nil: a strings.Reader is not an open file", f)
	}
	if IsTerminal(s.inFile()) {
		t.Error("a reader that is not a file was taken for a terminal")
	}
}

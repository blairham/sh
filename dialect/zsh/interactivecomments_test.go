// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A `#` typed at this shell's prompt is a character of the word it stands in
// until `interactivecomments` is set, and that option is **off** by default.
//
// The panel splits on the default rather than on the mechanism, which is what
// makes it an axis and not a correction. Measured 2026-09-12, `printf 'echo a
// #b\n'` into each shell under `-i` on a pipe with a scratch HOME and no
// startup files:
//
//	dash                 a       no option in the matter at all
//	ksh93u+              a       the same
//	bash 5.3.15          a       `shopt interactive_comments`, on by default
//	bash 3.2.57          a       the same
//	bash invoked as sh   a       the same
//	zsh 5.9.2            a #b    `interactivecomments`, off by default
//
// and the switch reaches across in both directions: bash under `shopt -u
// interactive_comments` answers `a #b`, and this shell under `setopt
// interactivecomments` answers `a`.
//
// It is the **prompt** and not the shell's interactivity, which is the half
// that makes `-i -c` a control rather than a second reading of the same
// question: `zsh -f -i -c 'echo a #b'` answers `a`, and so do `eval` and `.`
// typed at that shell's own prompt (#2537).
func TestAHashTypedAtThePromptIsACharacterUntilTheOptionIsSet(t *testing.T) {
	for _, tc := range []struct {
		name, typed, want string
	}{
		{
			name:  "the option is off, so the hash is a word",
			typed: "echo a #b\n",
			want:  "a #b\n",
		},
		{
			// The whole line, which is where it stops looking like a
			// quibble about an argument: the `#` is a command word and
			// the shell goes looking for a command called `#`.
			name:  "a line that is only a comment is a command",
			typed: "# hi\necho after=$?\n",
			want:  "after=127\n",
		},
		{
			name:  "and setting the option makes it a comment again",
			typed: "setopt interactivecomments\necho a #b\n",
			want:  "a\n",
		},
		{
			// Read per line rather than once, so a person who sets it and
			// then unsets it is obeyed both times.
			name:  "and unsetting it takes the comment away again",
			typed: "setopt interactivecomments\necho a #b\nunsetopt interactivecomments\necho c #d\n",
			want:  "a\nc #d\n",
		},
		{
			// A value is not the line somebody typed. Measured: both of
			// these answer `a` in real zsh with the option off.
			name:  "a string handed to eval is read the ordinary way",
			typed: `eval "echo a #b"` + "\n",
			want:  "a\n",
		},
		{
			// And the whole of the typed text is, at every depth. `$( )`
			// and its backquoted spelling carry the rule into the body;
			// `<( )` and `=( )` do not, which is measured and is not a
			// distinction anybody would guess.
			name:  "a command substitution carries the rule into its body",
			typed: "echo M-$(echo a #b; echo AFTER)\n",
			want:  "M-a #b AFTER\n",
		},
		{
			name:  "and so does the backquoted spelling",
			typed: "echo M-`echo a #b`\n",
			want:  "M-a #b\n",
		},
		{
			name:  "and so does a substitution inside a substitution",
			typed: "echo M-$(echo $(echo a #b))\n",
			want:  "M-a #b\n",
		},
		{
			name:  "a process substitution does not",
			typed: "cat <(echo M-a #b; echo AFTER)\n",
			want:  "M-a\n",
		},
		{
			// Where the body *ends* is a separate question and does not
			// move with the option: the `)` is found either way, so the
			// rest of the line still runs. With the option on it is a
			// parse error in this shell and in zsh alike, which is the
			// case the listing below owns rather than this one.
			name:  "and the closing parenthesis is still found",
			typed: "cat <(echo M-a #b) ; echo TAIL\n",
			want:  "M-a\nTAIL\n",
		},
		{
			// Arithmetic is not in this at all: `16#ff` is a base and not
			// a comment, in every shell in the panel and in both states of
			// the option.
			name:  "arithmetic reads a hash as a base either way",
			typed: "echo M-$(( 16#ff )) #b\n",
			want:  "M-255 #b\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// `-f`, because an interactive shell reads startup files and
			// the ones on the machine running the test are whoever's they
			// are. The prompts are drawn on the other stream, so what is
			// asserted here is the output alone.
			if got, errs, code := zshSession(t, tc.typed, "zsh", "-f", "-i"); got != tc.want {
				t.Errorf("typed %q: output %q status %d (stderr %q), want %q",
					tc.typed, got, code, errs, tc.want)
			}
		})
	}
}

// The controls: every route that is not a prompt reads the `#` as a comment,
// whatever the option says and whether or not the shell is interactive.
//
// These are half the measurement rather than a sanity check. A fix that
// turned the comment rule off for this *shell* — rather than for the text its
// front end read — passes every case above and fails every case here, and the
// two that name `-i` are the ones that say the answer is not simply "an
// interactive shell".
func TestEveryRouteButThePromptReadsAHashAsAComment(t *testing.T) {
	const src = "echo a #b"
	for _, tc := range []struct {
		name string
		argv []string
		in   string
	}{
		{name: "a command string", argv: []string{"zsh", "-f", "-c", src}},
		{name: "a command string in an interactive shell", argv: []string{"zsh", "-f", "-i", "-c", src}},
		{name: "standard input without -i", argv: []string{"zsh", "-f"}, in: src + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, errs, code := zshSession(t, tc.in, tc.argv...); got != "a\n" {
				t.Errorf("%v: output %q status %d (stderr %q), want %q", tc.argv, got, code, errs, "a\n")
			}
		})
	}
}

// zshSession runs this dialect's binary over argv with typed on its standard
// input, and returns the two streams and the status.
func zshSession(t *testing.T, typed string, argv ...string) (out, errs string, code int) {
	t.Helper()
	var ran, said bytes.Buffer
	sh := zshWriting(&ran, &said)
	sh.Stdin = strings.NewReader(typed)
	code = driver.MainArgs(sh, argv)
	return ran.String(), said.String(), code
}

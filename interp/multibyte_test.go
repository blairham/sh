// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// runLocale runs src with the multibyte axis answered and the named locale
// variables set, in the environment rather than in the script — the two routes
// a shell learns its encoding by, and the one a test of the *reading* wants.
func runLocale(t *testing.T, answer Answer, env []string, src string) (string, int) {
	t.Helper()
	return run(t, src, func(r *Runner) {
		sem := *r.Semantics
		sem.MultibyteEncodingIsHonored = answer
		r.Semantics = &sem
		r.Env = append(append([]string(nil), r.Env...), env...)
	})
}

// The two halves of the axis on one snippet: a length is bytes under a
// single-byte locale and characters under a UTF-8 one, in the same shell.
//
// The exact number both times, not merely "it ran": an assertion that only
// checked for the absence of a diagnostic would pass against the byte count
// this exists to stop being the only answer.
func TestALengthCountsWhatTheLocaleCallsACharacter(t *testing.T) {
	const src = `s=héllo; t=日本語; printf "[%s][%s]" "${#s}" "${#t}"`
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"no locale at all is single-byte", nil, "[6][9]"},
		{"a C locale is single-byte", []string{"LC_ALL=C"}, "[6][9]"},
		{"a UTF-8 locale counts characters", []string{"LC_ALL=C.UTF-8"}, "[5][3]"},
		{"and a named one does too", []string{"LC_ALL=en_US.UTF-8"}, "[5][3]"},
		{"a single-byte codeset does not", []string{"LC_ALL=en_US.ISO8859-1"}, "[6][9]"},
		{"nor does a locale with no codeset", []string{"LC_ALL=UTF-8"}, "[6][9]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runLocale(t, Yes, tc.env, src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The shell without a multibyte decoder counts bytes under the same locale
// that makes the others count characters. Named as the axis and not as a
// shell, which is what a test in this package may say.
func TestAShellWithoutTheDecoderCountsBytesInEveryLocale(t *testing.T) {
	const src = `s=héllo; printf "[%s]" "${#s}"`
	for _, env := range [][]string{nil, {"LC_ALL=C"}, {"LC_ALL=C.UTF-8"}, {"LC_ALL=ja_JP.UTF-8"}} {
		out, st := runLocale(t, No, env, src)
		if out != "[6]" || st != 0 {
			t.Errorf("with %v: got %q (status %d), want %q at 0", env, out, st, "[6]")
		}
	}
}

// Which variable is consulted, and in what order. Measured across the panel
// rather than taken from the usual rule: an *empty* LC_ALL or LC_CTYPE is
// skipped rather than being an answer of its own, which is the half a
// precedence chain written as "the first name that is set" gets wrong.
func TestTheLocaleVariablesAreConsultedInOrder(t *testing.T) {
	const src = `s=héllo; printf "[%s]" "${#s}"`
	for _, tc := range []struct {
		name string
		env  []string
		want string
	}{
		{"LC_ALL beats LC_CTYPE", []string{"LC_ALL=C", "LC_CTYPE=C.UTF-8"}, "[6]"},
		{"and beats it the other way too", []string{"LC_ALL=C.UTF-8", "LC_CTYPE=C"}, "[5]"},
		{"LC_CTYPE beats LANG", []string{"LC_CTYPE=C", "LANG=C.UTF-8"}, "[6]"},
		{"and beats it the other way too", []string{"LC_CTYPE=C.UTF-8", "LANG=C"}, "[5]"},
		{"LANG answers when neither is set", []string{"LANG=C.UTF-8"}, "[5]"},
		{"an empty LC_ALL is skipped", []string{"LC_ALL=", "LANG=C.UTF-8"}, "[5]"},
		{"an empty LC_CTYPE is skipped", []string{"LC_CTYPE=", "LANG=C.UTF-8"}, "[5]"},
		{"an empty LC_ALL does not skip LC_CTYPE", []string{"LC_ALL=", "LC_CTYPE=C.UTF-8"}, "[5]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runLocale(t, Yes, tc.env, src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// A plain assignment is enough, with nothing exported, and it takes effect on
// the next command. Measured: `LC_ALL=C; s=héllo; echo ${#s}` gives 6 in every
// panel member started under a UTF-8 locale.
func TestTheLocaleIsReadFromTheShellsOwnVariables(t *testing.T) {
	out, st := runLocale(t, Yes, []string{"LC_ALL=C.UTF-8"}, `s=héllo; LC_ALL=C; printf "[%s]" "${#s}"`)
	if out != "[6]" || st != 0 {
		t.Errorf("an assignment did not move it: got %q (status %d), want %q at 0", out, st, "[6]")
	}
	out, st = runLocale(t, Yes, []string{"LC_ALL=C"}, `s=héllo; LC_ALL=C.UTF-8; printf "[%s]" "${#s}"`)
	if out != "[5]" || st != 0 {
		t.Errorf("and not the other way either: got %q (status %d), want %q at 0", out, st, "[5]")
	}
}

// The codeset is compared without case and without the hyphen, and a
// `@modifier` after it is not part of it.
func TestTheCodesetIsSpelledSeveralWays(t *testing.T) {
	const src = `s=héllo; printf "[%s]" "${#s}"`
	for _, name := range []string{
		"en_US.UTF-8", "en_US.utf8", "en_US.UTF8", "en_US.utf-8",
		"de_DE.UTF-8@euro", "C.UTF-8",
	} {
		out, st := runLocale(t, Yes, []string{"LC_ALL=" + name}, src)
		if out != "[5]" || st != 0 {
			t.Errorf("LC_ALL=%s: got %q (status %d), want %q at 0", name, out, st, "[5]")
		}
	}
	for _, name := range []string{"C", "POSIX", "en_US", "en_US.ISO8859-1", "ja_JP.eucJP", "utf8"} {
		out, st := runLocale(t, Yes, []string{"LC_ALL=" + name}, src)
		if out != "[6]" || st != 0 {
			t.Errorf("LC_ALL=%s: got %q (status %d), want %q at 0", name, out, st, "[6]")
		}
	}
}

// A byte that begins no valid sequence is one character wide and is handed
// back as itself. Unanimous across the panel, and the trap is real from both
// ends: folding it into U+FFFD gives the right *length* with a value nothing
// put there, and refusing it gives neither.
func TestAnUndecodableByteIsOneCharacterOfItsOwn(t *testing.T) {
	setVar := func(r *Runner) { r.Vars = map[string]string{"s": "a\x80b"} }
	withVar := func(t *testing.T, src string) (string, int) {
		t.Helper()
		return run(t, src, func(r *Runner) {
			sem := *r.Semantics
			sem.MultibyteEncodingIsHonored = Yes
			r.Semantics = &sem
			r.Env = append(append([]string(nil), r.Env...), "LC_ALL=C.UTF-8")
			setVar(r)
		})
	}
	if out, st := withVar(t, `printf "[%s]" "${#s}"`); out != "[3]" || st != 0 {
		t.Errorf("length: got %q (status %d), want %q at 0", out, st, "[3]")
	}
	// The byte itself, not the replacement character, which is three bytes
	// long and would leave the length right and the value wrong.
	if out, st := withVar(t, `printf "[%s]" "${s:1:1}"`); out != "[\x80]" || st != 0 {
		t.Errorf("substring: got %q (status %d), want %q at 0", out, st, "[\x80]")
	}
	if out, st := withVar(t, `printf "[%s]" "${s:2}"`); out != "[b]" || st != 0 {
		t.Errorf("past it: got %q (status %d), want %q at 0", out, st, "[b]")
	}
	// A truncated sequence at the very end is the same answer: the lead byte
	// alone is one character, so the string is two.
	out, st := run(t, `printf "[%s]" "${#s}"`, func(r *Runner) {
		sem := *r.Semantics
		sem.MultibyteEncodingIsHonored = Yes
		r.Semantics = &sem
		r.Env = append(append([]string(nil), r.Env...), "LC_ALL=C.UTF-8")
		r.Vars = map[string]string{"s": "a\xc3"}
	})
	if out != "[2]" || st != 0 {
		t.Errorf("a truncated lead byte: got %q (status %d), want %q at 0", out, st, "[2]")
	}
}

// A substring's offset and its length count the same unit the length does,
// including from the end. An implementation that converted one end and not the
// other would cut a character in half here rather than fail.
func TestASubstringCountsTheSameUnitAsALength(t *testing.T) {
	for _, tc := range []struct {
		name, src, utf8, single string
	}{
		{"an offset and a length", `printf "[%s]" "${s:1:2}"`, "[él]", "[é]"},
		{"an offset alone", `printf "[%s]" "${s:3}"`, "[lo]", "[llo]"},
		{"a negative offset", `printf "[%s]" "${s: -4}"`, "[éllo]", "[\xa9llo]"},
		{"a negative offset with a length", `printf "[%s]" "${s: -4:2}"`, "[él]", "[\xa9l]"},
		{"a negative length", `printf "[%s]" "${s:1:-1}"`, "[éll]", "[éll]"},
		{"an offset past the end", `printf "[%s]" "${s:9}"`, "[]", "[]"},
		{"a zero length", `printf "[%s]" "${s:1:0}"`, "[]", "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `s=héllo; ` + tc.src
			if out, st := runLocale(t, Yes, []string{"LC_ALL=C.UTF-8"}, src); out != tc.utf8 || st != 0 {
				t.Errorf("in UTF-8: got %q (status %d), want %q at 0", out, st, tc.utf8)
			}
			if out, st := runLocale(t, Yes, []string{"LC_ALL=C"}, src); out != tc.single || st != 0 {
				t.Errorf("in C: got %q (status %d), want %q at 0", out, st, tc.single)
			}
		})
	}
}

// `${#a[@]}` is a count and not a length, so it is the same number in every
// locale; `${#a[N]}` is the width of one element and moves with the codeset.
func TestACountIsNotALength(t *testing.T) {
	const src = `a=(héllo 日本語); printf "[%s][%s][%s]" "${#a[@]}" "${#a[0]}" "${#a[1]}"`
	if out, st := runLocale(t, Yes, []string{"LC_ALL=C.UTF-8"}, src); out != "[2][5][3]" || st != 0 {
		t.Errorf("in UTF-8: got %q (status %d), want %q at 0", out, st, "[2][5][3]")
	}
	if out, st := runLocale(t, Yes, []string{"LC_ALL=C"}, src); out != "[2][6][9]" || st != 0 {
		t.Errorf("in C: got %q (status %d), want %q at 0", out, st, "[2][6][9]")
	}
}

// Asked only where the two readings differ. An all-ASCII value is the same
// length either way, so the axis goes unanswered and nothing is refused — the
// property that keeps `${#x}` working in the core, whose answer to nearly
// every axis is "unanswered".
func TestAnAsciiValueAsksNothing(t *testing.T) {
	out, st := runLocale(t, Unspecified, []string{"LC_ALL=C.UTF-8"}, `x=abcd; printf "[%s][%s]" "${#x}" "${x:1:2}"`)
	if out != "[4][bc]" || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, "[4][bc]")
	}
	// Nor does a non-ASCII value under a locale that names no multibyte
	// encoding: every shell counts its bytes there, so there is nothing to
	// disagree about.
	if out, st := runLocale(t, Unspecified, []string{"LC_ALL=C"}, `s=héllo; printf "[%s]" "${#s}"`); out != "[6]" || st != 0 {
		t.Errorf("in C: got %q (status %d), want %q at 0", out, st, "[6]")
	}
}

// And is refused by name where they do differ, rather than answered by a
// default. The two together are the axis: a value and a locale that make the
// readings disagree is the only shape that needs a dialect.
func TestANonAsciiValueInAUtf8LocaleNeedsAnAnswer(t *testing.T) {
	out, st := runLocale(t, Unspecified, []string{"LC_ALL=C.UTF-8"}, `s=héllo; printf "[%s]" "${#s}"`)
	if !strings.Contains(out, "a character being the locale's rather than a byte") {
		t.Errorf("got %q, want the axis named", out)
	}
	if st != 2 {
		t.Errorf("status %d, want 2", st)
	}
}

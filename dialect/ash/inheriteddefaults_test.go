// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// Three axes this dialect never answered, and one builtin it never had.
//
// #3248 was the sixth axis found in a night where `dialect/ash` set no value
// and the POSIX preset's answer — dash's, for every one of them — was taken
// in silence. This file holds what a sweep for the rest of that shape found.
// The sweep's own method is in the pull request; what matters here is that
// each row below is a *measurement* against BusyBox v1.37.0 in the
// digest-pinned Alpine image internal/oracle reaches, on 2026-09-16, under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`.
//
// Every row is paired with a control that the wrong answer also passes, so
// the pair as a whole can only be produced by the shell this dialect claims
// to be.

// `$((x+1))` where x holds a name, which dash alone refuses.
func TestArithmeticRereadsANameShapedValue(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			// The discriminator. dash answers `Illegal number: y` at 2 and
			// prints nothing.
			"a value that names another parameter",
			`y=5; x=y; echo $((x+1))`,
			"6\n",
		},
		{
			// And it follows the values as far as they go, which says the
			// lookup recurses rather than being one extra step.
			"a value that names a parameter that names a third",
			`y=z; z=7; x=y; echo $((x+1))`,
			"8\n",
		},
		{
			// The control: a plain numeral, which every column answers the
			// same way. A shell that refused the two rows above would still
			// pass this one.
			"a value that is a numeral",
			`n=5; echo $((n+1))`,
			"6\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runIn(t, tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// `.` with an operand PATH has not got, which bash falls back on and dash,
// zsh and ksh93 do not.
func TestDotFallsBackToTheCurrentDirectory(t *testing.T) {
	for _, tc := range []struct {
		name          string
		onPath, inCwd bool
		want          string
	}{
		{
			// The discriminator. With the name nowhere on PATH, the copy
			// beside the script is what runs; dash reports `not found` and
			// ends the script.
			"only in the current directory",
			false, true, "the current directory copy ran\n",
		},
		{
			// The control the whole panel agrees on, and the one that says
			// the fallback is a fallback: with the name in both places, the
			// PATH copy wins.
			"in both, PATH first",
			true, true, "the PATH copy ran\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			pathDir := filepath.Join(dir, "elsewhere")
			if err := os.MkdirAll(pathDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if tc.inCwd {
				write(t, filepath.Join(dir, "lib.sh"),
					"printf 'the current directory copy ran\\n'\n")
			}
			if tc.onPath {
				write(t, filepath.Join(pathDir, "lib.sh"),
					"printf 'the PATH copy ran\\n'\n")
			}
			out, st, err := preset.Combined(t, dialecttest.Base{
				Name: "ash", Dir: dir,
				Env: []string{"PATH=" + pathDir + ":/usr/bin:/bin"},
			}, ". lib.sh\n")
			if err != nil {
				t.Fatalf("unsupported: %v", err)
			}
			if st != 0 {
				t.Errorf("status %d, want 0; output %q", st, out)
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// `[[ x =~ "" ]]`, which this shell matches with and bash and zsh refuse.
func TestAnEmptyRegexOperandMatches(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      int
	}{
		// The discriminator: 0 here, 2 in bash under all three builds and 1
		// in zsh, each with a sentence of its own on standard error.
		{"an empty regex", `[[ abc =~ "" ]]`, 0},
		// The two controls, which say the operator works at all — a shell
		// that refused every regex would answer the first row's 0 by
		// accident only if it also broke these.
		{"a regex that matches", `[[ abc =~ b ]]`, 0},
		{"a regex that does not match", `[[ abc =~ x ]]`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runIn(t, tc.src)
			if st != tc.want {
				t.Errorf("status %d, want %d; output %q", st, tc.want, out)
			}
		})
	}
}

// `source`, which this shell has as the second spelling of `.` and dash has
// not got at all. `cmd/ash` answered `source: not found` at 127.
func TestSourceIsTheOtherSpellingOfDot(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "lib.sh"), `printf 'n=%s [%s]\n' "$#" "$1"`+"\n")
	src := "set -- outer1 outer2\n" +
		"printf 'v=[%s]\\n' \"$(command -v source)\"\n" +
		"source ./lib.sh word\n" +
		"printf 'after n=%s [%s]\\n' \"$#\" \"$1\"\n"
	want := "v=[source]\nn=1 [word]\nafter n=2 [outer1]\n"
	out, st, err := preset.Combined(t, dialecttest.Base{
		Name: "ash", Dir: dir, Env: []string{"PATH=/usr/bin:/bin"},
	}, src)
	if err != nil {
		t.Fatalf("unsupported: %v", err)
	}
	if st != 0 {
		t.Errorf("status %d, want 0; output %q", st, out)
	}
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

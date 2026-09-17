// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A script's history list and the file it is kept in, measured 2026-09-16 on
// bash 5.3.20 from script files with no terminal and no startup files. Every
// expected string is what bash wrote for the same script.

// historyFileRun runs src with $F naming a history file seeded with lines,
// and answers standard output and what the file holds afterwards.
func historyFileRun(t *testing.T, src string, lines ...string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	f := filepath.Join(dir, "hf")
	if len(lines) > 0 {
		if err := os.WriteFile(f, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	out, errs, _ := historyRun(t, strings.ReplaceAll(src, "$F", f))
	if errs != "" {
		t.Errorf("stderr %q", errs)
	}
	data, _ := os.ReadFile(f)
	return out, string(data)
}

func TestTurningTheListOnReadsTheHistoryFile(t *testing.T) {
	out, file := historyFileRun(t, "HISTFILE=$F\nset -o history\nhistory\n", "alpha one", "beta two")
	if want := "    1  alpha one\n    2  beta two\n    3  history\n"; out != want {
		t.Errorf("listing %q, want %q", out, want)
	}
	if want := "alpha one\nbeta two\nhistory\n"; file != want {
		t.Errorf("file %q, want %q", file, want)
	}
}

func TestTheFileIsReadOnceAndOnlyAtTheFirstTurningOn(t *testing.T) {
	out, _ := historyFileRun(t, "HISTFILE=$F\nset -o history\nset +o history\nset -o history\nhistory\n", "alpha one")
	if want := "    1  alpha one\n    2  set +o history\n    3  history\n"; out != want {
		t.Errorf("a second turning on: listing %q, want %q", out, want)
	}
	// A first turning on with no file to read is still the first.
	out, _ = historyFileRun(t, "set -o history\nHISTFILE=$F\nset +o history\nset -o history\nhistory\n", "alpha one")
	if strings.Contains(out, "alpha") {
		t.Errorf("a file named after the first turning on was read: %q", out)
	}
}

func TestTheSizesDefaultWhenTheListIsTurnedOn(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"echo \"[${HISTSIZE-u}][${HISTFILESIZE-u}]\"\nset -o history\necho \"[$HISTSIZE][$HISTFILESIZE]\"\n", "[u][u]\n[500][500]\n"},
		{"HISTSIZE=7\nset -o history\necho \"[$HISTSIZE][$HISTFILESIZE]\"\n", "[7][7]\n"},
		{"HISTFILESIZE=7\nset -o history\necho \"[$HISTSIZE][$HISTFILESIZE]\"\n", "[500][7]\n"},
		{"set -o history\nunset HISTSIZE HISTFILESIZE\nset +o history\nset -o history\necho \"[${HISTSIZE-u}][${HISTFILESIZE-u}]\"\n", "[u][u]\n"},
	} {
		if out, _ := historyFileRun(t, c.src); out != c.want {
			t.Errorf("%q: out %q, want %q", c.src, out, c.want)
		}
	}
}

func TestHistfilesizeKeepsTheNewestLinesItReads(t *testing.T) {
	out, _ := historyFileRun(t, "HISTFILE=$F\nHISTFILESIZE=2\nset -o history\nhistory\n", "alpha one", "beta two", "gamma three")
	if want := "    1  beta two\n    2  gamma three\n    3  history\n"; out != want {
		t.Errorf("listing %q, want %q", out, want)
	}
}

// What the ending appends is the count `-a` appends: the entries this session
// made and nothing has written, counted from the end of the list.
func TestTheEndingAppendsWhatThisSessionAdded(t *testing.T) {
	seed := []string{"alpha one", "beta two", "gamma three"}
	for _, c := range []struct{ name, src, want string }{{
		name: "a read entry is not written back",
		src:  "HISTFILE=$F\nset -o history\nhistory -r\nhistory -c\n",
		want: "alpha one\nbeta two\ngamma three\n",
	}, {
		name: "an -a in between writes each entry once",
		src:  "HISTFILE=$F\nset -o history\nhistory -a\necho b\n",
		want: "alpha one\nbeta two\ngamma three\nhistory -a\necho b\n",
	}, {
		name: "a -w does not reset the count",
		src:  "HISTFILE=$F\nset -o history\nhistory -w\necho b\n",
		want: "alpha one\nbeta two\ngamma three\nhistory -w\nhistory -w\necho b\n",
	}, {
		name: "every -d takes one off",
		src:  "HISTFILE=$F\nset -o history\necho q\nhistory -d 1\nhistory -d 1\nhistory -d 1\n",
		want: "history -d 1\n",
	}, {
		name: "a builtin's own dropped line takes one off",
		src:  "HISTFILE=$F\nset -o history\necho q\nhistory -p x\n",
		want: "alpha one\nbeta two\ngamma three\necho q\n",
	}, {
		name: "-s entries count",
		src:  "HISTFILE=$F\nset -o history\nhistory -s z\nhistory -s y\n",
		want: "alpha one\nbeta two\ngamma three\nz\ny\n",
	}, {
		name: "-c puts the count back to nothing",
		src:  "HISTFILE=$F\nset -o history\necho 1\nhistory -c\necho 2\n",
		want: "alpha one\nbeta two\ngamma three\necho 2\n",
	}, {
		name: "turned off before the end writes nothing",
		src:  "HISTFILE=$F\nset -o history\necho x\nset +o history\n",
		want: "alpha one\nbeta two\ngamma three\n",
	}, {
		name: "unset before the end writes nothing",
		src:  "HISTFILE=$F\nset -o history\nunset HISTFILE\necho x\n",
		want: "alpha one\nbeta two\ngamma three\n",
	}, {
		name: "a command substitution's ending writes nothing",
		src:  "HISTFILE=$F\nset -o history\nx=$(echo sub; exit 3)\nhistory -c\n",
		want: "alpha one\nbeta two\ngamma three\n",
	}} {
		t.Run(c.name, func(t *testing.T) {
			_, file := historyFileRun(t, c.src, seed...)
			if c.name == "every -d takes one off" {
				// The three deletions took the seeded lines off the list,
				// which is not the file: the file keeps them and gains one.
				c.want = strings.Join(seed, "\n") + "\n" + c.want
			}
			if file != c.want {
				t.Errorf("file %q, want %q", file, c.want)
			}
		})
	}
}

// HISTCONTROL and HISTIGNORE keep a line out of a script's list as they keep
// one out of a prompt's.
func TestTheHistoryKnobsReachAScriptsList(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{{
		name: "ignorespace",
		src:  "HISTCONTROL=ignorespace\nset -o history\n echo a\necho b\nhistory\n",
		want: "a\nb\n    1  echo b\n    2  history\n",
	}, {
		name: "ignoredups is the line before only",
		src:  "HISTCONTROL=ignoredups\nset -o history\necho a\necho a\necho b\necho a\nhistory\n",
		want: "a\na\nb\na\n    1  echo a\n    2  echo b\n    3  echo a\n    4  history\n",
	}, {
		name: "erasedups keeps the newest copy",
		src:  "HISTCONTROL=erasedups\nset -o history\necho a\necho b\necho a\nhistory\n",
		want: "a\nb\na\n    1  echo b\n    2  echo a\n    3  history\n",
	}, {
		name: "a pattern and the line before",
		src:  "HISTIGNORE='true*:&'\nset -o history\necho a\necho a\ntrue\necho a\nhistory\n",
		want: "a\na\na\n    1  echo a\n    2  history\n",
	}, {
		name: "a quoted colon is part of a pattern",
		src:  "HISTIGNORE='a\\:b*'\nset -o history\na:b 2>/dev/null\nab 2>/dev/null\nhistory\n",
		want: "    1  ab 2>/dev/null\n    2  history\n",
	}, {
		name: "an ignored builtin has no line of its own to drop",
		src:  "set -o history\nHISTIGNORE='history*'\nhistory -p x\necho y\nhistory -s z\nhistory\n",
		want: "x\ny\n    1  HISTIGNORE='history*'\n    2  echo y\n    3  z\n",
	}} {
		t.Run(c.name, func(t *testing.T) {
			out, _, _ := historyRun(t, c.src)
			if out != c.want {
				t.Errorf("out %q, want %q", out, c.want)
			}
		})
	}
}

// HISTSIZE bounds the list, and the numbers go on from what it dropped.
func TestHistsizeBoundsTheListAndTheNumbersGoOn(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"set -o history\necho a\necho b\necho c\nHISTSIZE=2\nhistory\n", "a\nb\nc\n    3  HISTSIZE=2\n    4  history\n"},
		{"set -o history\necho a\necho b\necho c\nHISTSIZE=3\necho d\necho e\nhistory\n", "a\nb\nc\nd\ne\n    4  echo d\n    5  echo e\n    6  history\n"},
		{"set -o history\necho a\necho b\necho c\nHISTSIZE=3\nhistory -p x\nhistory\n", "a\nb\nc\nx\n    2  echo c\n    3  HISTSIZE=3\n    4  history\n"},
		{"set -o history\necho a\necho b\necho c\nHISTSIZE=3\nhistory -d 3\nhistory\n", "a\nb\nc\n    2  echo c\n    3  history -d 3\n    4  history\n"},
		{"set -o history\nHISTSIZE=2\nhistory -s a\nhistory -s b\nhistory -s c\nhistory\n", "    4  c\n    5  history\n"},
		{"set -o history\nHISTSIZE=0\necho a\nhistory\n", "a\n"},
		{"set -o history\nHISTSIZE=-1\necho a\nhistory\n", "a\n    1  HISTSIZE=-1\n    2  echo a\n    3  history\n"},
		{"set -o history\nset -H\necho a\necho b\necho c\nHISTSIZE=3\necho d\necho !5\nhistory\n", "a\nb\nc\nd\necho d\n    5  echo d\n    6  echo echo d\n    7  history\n"},
	} {
		if out, _, _ := historyRun(t, c.src); out != c.want {
			t.Errorf("%q:\n out %q\nwant %q", c.src, out, c.want)
		}
	}
}

// In POSIX mode a double-quoted reference is left alone.
func TestPosixModeSparesADoubleQuotedReference(t *testing.T) {
	out, errs, _ := historyRun(t, "set -o history\necho a\nset -o histexpand\nset -o posix\necho \"!!\" !!\n")
	if wantOut, wantErr := "a\n!! set -o posix\n", "echo \"!!\" set -o posix\n"; out != wantOut || errs != wantErr {
		t.Errorf("out %q err %q, want %q and %q", out, errs, wantOut, wantErr)
	}
}

// A here-document inside a command substitution is not expanded, and the
// entry for it ends with the substitution rather than with a blank line.
func TestAHereDocumentInsideASubstitutionIsNotExpanded(t *testing.T) {
	out, _, _ := historyRun(t, "set -o history\nset -o histexpand\necho a\necho $(cat <<EOF\necho !!\nEOF\n)\nhistory\n")
	want := "a\necho !!\n    1  set -o histexpand\n    2  echo a\n    3  echo $(cat <<EOF\necho !!\nEOF\n)\n    4  history\n"
	if out != want {
		t.Errorf("out %q, want %q", out, want)
	}
}

// A word beginning with `#` ends expansion for the line.
func TestACommentIsLeftAsWritten(t *testing.T) {
	out, errs, _ := historyRun(t, "set -o history\nset -H\necho a\necho ab c # !nosuch\necho a;#!!\n")
	if wantOut := "a\nab c\na\n"; out != wantOut || errs != "" {
		t.Errorf("out %q err %q, want %q and nothing", out, errs, wantOut)
	}
}

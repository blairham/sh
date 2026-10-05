// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// zmv, zargs and run-help against rows measured from zsh 5.9.2 with its own
// copies (the header of each testdata file says how). Nothing here reads
// zsh's function files; the probe called them.

// contribRows reads a tab-separated testdata file, skipping its comment
// lines.
func contribRows(t *testing.T, name string) [][]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var rows [][]string
	for _, line := range strings.Split(strings.TrimSuffix(string(b), "\n"), "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			rows = append(rows, strings.Split(line, "\t"))
		}
	}
	if len(rows) == 0 {
		t.Fatalf("%s holds no rows", name)
	}
	return rows
}

// shellWords splits the files column, where a space inside a name is
// written `\ `.
func shellWords(s string) []string {
	var words []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
		case s[i] == ' ':
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(s[i])
		}
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	return words
}

// listing is what a directory holds, in the testdata's spelling: path:contents
// for a file, path/ for a directory, path -> target for a link.
func listing(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || p == dir {
			return err
		}
		rel := "./" + strings.TrimPrefix(p, dir+"/")
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, _ := os.Readlink(p)
			out = append(out, rel+" -> "+target)
		case info.IsDir():
			out = append(out, rel+"/")
		default:
			b, _ := os.ReadFile(p)
			out = append(out, rel+":"+strings.TrimRight(string(b), "\n"))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

// listingEntry splits the testdata's listing column at each entry's `./`.
var listingEntry = regexp.MustCompile(`(?:^| )\./`)

func TestZmvAnswersWhatZshAnswers(t *testing.T) {
	for _, row := range contribRows(t, "zmv.tsv") {
		t.Run(row[1], func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range shellWords(row[0]) {
				p := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(name+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			out, _ := runZshOnPath(t, dir, "fpath=("+shippedFunctionDir(t)+")\nautoload -Uz zmv\n"+row[1]+"\nprint -r -- st=$?\n")
			if got := strings.ReplaceAll(out, "\n", "~"); got != row[2] {
				t.Errorf("output %q, want %q", got, row[2])
			}
			var want []string
			for _, e := range listingEntry.Split(row[3], -1) {
				if e != "" {
					want = append(want, "./"+e)
				}
			}
			slices.Sort(want)
			if got := listing(t, dir); !slices.Equal(got, want) {
				t.Errorf("left %q, want %q", got, want)
			}
		})
	}
}

func TestZargsAnswersWhatZshAnswers(t *testing.T) {
	for _, row := range contribRows(t, "zargs.tsv") {
		t.Run(row[0], func(t *testing.T) {
			out, _ := runZshOnPath(t, t.TempDir(), "fpath=("+shippedFunctionDir(t)+")\nautoload -Uz zargs\n"+row[0]+"\nprint -r -- st=$?\n")
			if got := strings.ReplaceAll(out, "\n", "~"); got != row[1] {
				t.Errorf("output %q, want %q", got, row[1])
			}
		})
	}
}

// run-help with a help directory holding `cd` and `if`, a pager that says
// where the help file is, and a `man` that says what it was asked for.
// Measured 2026-10-04 against zsh 5.9.2 with its own run-help, the alias
// removed and TERM=xterm, from a script file with standard input on the null
// device — so the question between two meanings finds no terminal to ask on.
func TestRunHelpAnswersWhatZshAnswers(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	help := filepath.Join(dir, "help")
	for _, d := range []string{bin, help} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{
		"man":    "#!/bin/sh\necho \"MAN:$*\"\n",
		"pg":     "#!/bin/sh\necho \"PAGER:$(basename \"$1\")\"\n",
		"mytool": "#!/bin/sh\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, topic := range []string{"cd", "if"} {
		if err := os.WriteFile(filepath.Join(help, topic), []byte(topic+" help\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tool := filepath.Join(bin, "mytool")
	const reverse, plain = "\x1b[7m", "\x1b[27m"
	const ask = reverse + "Press any key for more help or q to quit" + plain + "not interactive and can't open terminal\n\n"
	// Where echo is depends on the machine: /bin/echo on a Mac, and on a
	// Linux runner both /usr/bin/echo and /bin/echo, /bin being a link to
	// /usr/bin. Each one on the path is a meaning of its own, with the
	// question before it, so the expectation is built the way the shell
	// will look.
	echoes := ""
	for _, d := range []string{"/usr/bin", "/bin"} {
		if _, err := os.Stat(filepath.Join(d, "echo")); err == nil {
			echoes += ask + "echo is " + filepath.Join(d, "echo") + "\nMAN:echo\n"
		}
	}
	for _, tc := range []struct{ call, want string }{
		{`\run-help cd`, "PAGER:cd\n"},
		{`\run-help if`, "PAGER:if\n"},
		{`\run-help mytool`, "mytool is " + tool + "\nMAN:mytool\n"},
		{`\run-help ll`, "ll is an alias for mytool -l\nmytool is " + tool + "\nMAN:mytool\n"},
		{`\run-help nosuch`, "nosuch not found\nMAN:nosuch\n"},
		{`\run-help mytool sub -x`, "mytool is " + tool + "\nMAN:mytool sub -x\n"},
		{`\run-help echo`, "echo is a shell builtin\nMAN:zshbuiltins\n" + echoes},
		{`\run-help typeset`, "typeset is a reserved word\nMAN:zshmisc\n" + ask + "typeset is a shell builtin\nMAN:zshbuiltins\n"},
		{`\run-help`, "Here is a list of topics for which special help is available:\n\ncd  if\n"},
	} {
		t.Run(tc.call, func(t *testing.T) {
			out, _ := runZshOnPath(t, dir, "fpath=("+shippedFunctionDir(t)+")\n"+
				"path=("+bin+" $path); PAGER=pg; HELPDIR="+help+"; COLUMNS=80; TERM=xterm\n"+
				"autoload -Uz run-help\nalias ll='mytool -l'\n"+tc.call+"\nprint -r -- st=$?\n")
			if want := tc.want + "st=0\n"; out != want {
				t.Errorf("got %q, want %q", out, want)
			}
		})
	}
}

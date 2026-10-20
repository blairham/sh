// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// vcsFixtures builds the repositories of testdata/vcs_info-fixtures.sh in a
// directory of the test's own and answers its root, with every link in the
// path resolved: git reports the top of a repository that way, and the rows
// write the root as @R.
func vcsFixtures(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git here")
	}
	script, err := filepath.Abs(filepath.Join("testdata", "vcs_info-fixtures.sh"))
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "r")
	cmd := exec.Command("/bin/sh", script, root)
	cmd.Env = append(os.Environ(), "HOME="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building the fixtures: %v\n%s", err, out)
	}
	return root
}

// vcsRun runs src after autoload in the fixture directory named, under the
// environment the rows were measured in.
func vcsRun(t *testing.T, root, where, src string) string {
	t.Helper()
	// Three lines before src, as the measurement had, so that a diagnostic
	// naming a line names the same one.
	out, _ := runZshOnPath(t, root, "HOME=/nonexistent/home TERM=dumb LC_ALL=C; "+
		"export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null; fpath=("+shippedFunctionDir(t)+")\n"+
		"autoload -Uz vcs_info\ncd "+filepath.Join(root, where)+" || exit 9\n"+src+"\n")
	return out
}

func TestVcsInfoAnswersWhatZshAnswers(t *testing.T) {
	root := vcsFixtures(t)
	for _, row := range contribRows(t, "vcs_info.tsv") {
		t.Run(row[0]+" "+row[1], func(t *testing.T) {
			got := strings.ReplaceAll(strings.ReplaceAll(vcsRun(t, root, row[0], row[1]), root, "@R"), "\n", "~")
			if got != row[2] {
				t.Errorf("output %q, want %q", got, row[2])
			}
		})
	}
}

// Where this copy answers in its own words, or differs from zsh's on
// purpose; docs/spec/functions.md gives each reason.
func TestVcsInfoWhereItIsItsOwn(t *testing.T) {
	root := vcsFixtures(t)
	const show = `print -r -- "0=${(q)vcs_info_msg_0_}" "1=${(q)vcs_info_msg_1_}" "2=${(q)vcs_info_msg_2_}" "${+vcs_info_msg_2_}"`
	for _, tc := range []struct{ name, where, src, want string }{
		// The usage is this file's: the stream, standard output, and the
		// status are zsh's.
		{"hookadd with too little", "clean", "vcs_info; { vcs_info_hookadd } >/dev/null 2>&1; print st=$?; { vcs_info_hookadd } 2>/dev/null | wc -l | tr -d ' '", "st=1\n1\n"},
		{"hookdel with too little", "clean", "vcs_info; { vcs_info_hookdel -a x } >/dev/null 2>&1; print st=$?; { vcs_info_hookdel } 2>/dev/null | wc -l | tr -d ' '", "st=1\n1\n"},
		// The one system this copy reads, marked when it is not looked for.
		{"printsys", "clean", "vcs_info; vcs_info_printsys | grep -v '^# '", "git\n"},
		{"printsys disabled", "clean", "vcs_info; zstyle ':vcs_info:*' disable git; vcs_info_printsys | grep -v '^# '", "#git\n"},
		{"printsys not enabled", "clean", "vcs_info; zstyle ':vcs_info:*' enable hg; vcs_info_printsys | grep -v '^# '", "#git\n"},
		// zsh's copy fails on a typeset of vcs_info_msg_-1_ for both of these.
		{"enable none, any case", "clean", "zstyle ':vcs_info:*' enable none; zstyle ':vcs_info:*' nvcsformats NV; vcs_info; " + show, "0='' 1='' 2='' 0\n"},
		{"enable NONE with nvcsformats", "clean", "zstyle ':vcs_info:*' enable NONE; zstyle ':vcs_info:*' nvcsformats NV; vcs_info; " + show, "0='' 1='' 2='' 0\n"},
		// zsh's copy sets one fewer nvcsformats than max-exports when there
		// are more of them than that.
		{"nvcsformats past max-exports", "none", "zstyle ':vcs_info:*' max-exports 3; zstyle ':vcs_info:*' nvcsformats a b c d; vcs_info; " + show, "0=a 1=b 2=c 1\n"},
		// zsh's own vcs_info_lastmsg, autoloaded before vcs_info has run,
		// stops on a helper it does not have and then names vcs_info_msg_-1_.
		{
			"lastmsg on its own", "clean", "autoload -Uz vcs_info_lastmsg; vcs_info_msg_0_=hello; vcs_info_msg_1_=; vcs_info_lastmsg",
			"$vcs_info_msg_0_: \"hello\"\n$vcs_info_msg_1_: \"\"\n",
		},
		// A max-exports that is not a number of at least 1 is 2, with a
		// warning on standard output, as in zsh; the warning is worded here.
		{"max-exports 0", "clean", "zstyle ':vcs_info:*' max-exports 0; zstyle ':vcs_info:*' formats a b c; vcs_info >/dev/null; " + show, "0=a 1=b 2='' 0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := vcsRun(t, root, tc.where, tc.src); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

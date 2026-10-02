// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"regexp"
	"testing"
)

// **A forked body that is not parentheses keeps the parent's markers as
// numbers, and numbers its jobs by whether the parent holds a job one**
// (#5321). Measured 2026-10-02 on zsh 5.9.2 under `-f -c`, byte for byte but
// for the process ids, which are masked on both sides. See
// interp.Runner.runAsAForkedBody.
func TestAForkedBodyNumbersByTheParentsJobOne(t *testing.T) {
	const j = "/bin/sleep 0.3 & /bin/sleep 0.3 & print ${(kv)jobstates}"
	const one = "/bin/sleep 1 & "
	for _, c := range []struct{ name, src, want string }{
		{"a brace group element, no parent job", "{ " + j + " } | cat; :", "2 running::P=running 3 running::P=running\n"},
		{"a brace group element, a parent job", one + "{ " + j + " } | cat; :", "1 running:+:P=running 2 running::P=running\n"},
		{"two parent jobs", one + one + "{ " + j + " } | cat; :", "1 running:-:P=running 2 running:+:P=running\n"},
		{"three parent jobs", one + one + one + "{ " + j + " } | cat; :", "1 running::P=running 2 running:-:P=running\n"},
		{"a & body, no parent job", "{ " + j + " } & wait $!; :", "2 running::P=running 3 running::P=running\n"},
		{"a & body, a parent job", one + "{ " + j + " } & wait $!; :", "1 running:+:P=running 2 running::P=running\n"},
		{"a function element", "f() { " + j + " }; f | cat; :", "2 running::P=running 3 running::P=running\n"},
		{"an if element", "if true; then " + j + "; fi | cat; :", "2 running::P=running 3 running::P=running\n"},
		{"an if element, a parent job", one + "if true; then " + j + "; fi | cat; :", "2 running::P=running 3 running::P=running\n"},
		{"an if in a brace group", "{ if true; then " + j + "; fi } | cat; :", "3 running::P=running 4 running::P=running\n"},
		{
			"job one ended and noticed",
			"/bin/sleep 0 & /bin/sleep 1 & /bin/sleep 0.2; jobs >/dev/null; { " + j + " } | cat; :",
			"2 running:+:P=running 3 running::P=running\n",
		},
		{
			"job two ended and noticed",
			"/bin/sleep 1 & /bin/sleep 0 & /bin/sleep 0.2; jobs >/dev/null; { " + j + " } | cat; :",
			"1 running:+:P=running 2 running::P=running\n",
		},
		{"control: parentheses", one + "( " + j + " ) | cat; :", "2 running::P=running 3 running::P=running\n"},
		{
			"the body is the slot it holds",
			"{ jobs %1; echo s=$?; wait %1; echo w=$?; kill -0 %1; echo k=$? } 2>&1 | cat; :",
			"s=0\nw=0\nzsh:kill:1: %1: no such job\nk=1\n",
		},
		{"a builtin element holds nothing", "jobs %1 | cat; echo $pipestatus", "zsh:jobs:1: %1: no such job\n127 0\n"},
		{"a function element is the slot it holds", "f() { jobs %1; echo $? }; f | cat", "0\n"},
		{"an eval element", `eval '/bin/sleep 0.3 & print ${(k)jobstates}' | cat`, "2\n"},
		{
			"a + left on a number nobody holds, read by a builtin element",
			"f() { /bin/sleep 0 & wait }; f; wait %% 2>&1 | /bin/cat",
			"zsh:wait:1: no current job\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := statePIDs.ReplaceAllString(runZshC(c.src), ":P="); got != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, got, c.want)
			}
		})
	}
}

// statePIDs masks the process id a `$jobstates` value carries.
var statePIDs = regexp.MustCompile(`:[0-9]+=`)

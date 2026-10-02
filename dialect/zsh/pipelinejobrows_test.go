// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"regexp"
	"testing"
)

// **A pipeline is listed an element at a time, each in its own state**, and
// the forked elements of one whose last element the shell runs itself are a
// job while that element runs (#5322). Measured 2026-10-02 on zsh 5.9.2 under
// `-f -c`, byte for byte but for process ids. See interp/jobelements.go.
func TestAPipelineIsListedAnElementAtATime(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a backgrounded pipeline",
			"/bin/sleep 0.3 | cat & jobs",
			"[1]  + running    /bin/sleep 0.3 | \n       running    cat\n",
		},
		{
			"jobs -l names each element's process",
			"/bin/sleep 0.3 | cat & jobs -l",
			"[1]  + P running    /bin/sleep 0.3 | \n       P running    cat\n",
		},
		{
			"jobs -p names the group once",
			"/bin/sleep 0.3 | cat & jobs -p",
			"[1]  + P running    /bin/sleep 0.3 | \n             running    cat\n",
		},
		{
			"an element that exited",
			"exit 3 | /bin/sleep 0.3 & /bin/sleep 0.1; jobs",
			"[1]  + exit 3     exit 3 | \n       running    /bin/sleep 0.3\n",
		},
		{
			"a signal word widens the column",
			`/bin/sh -c "kill -TERM \$\$" | /bin/sleep 0.3 & /bin/sleep 0.1; jobs`,
			"[1]  + terminated  /bin/sh -c \"kill -TERM \\$\\$\" | \n       running     /bin/sleep 0.3\n",
		},
		{
			"forking a later element notices an earlier one's end",
			"true | true | /bin/sleep 0.3 & jobs",
			"[1]  + done       true | \n       done       true | \n       running    /bin/sleep 0.3\n",
		},
		{
			"nothing is forked after the last",
			"/bin/sleep 0.3 | cat | cat & jobs",
			"[1]  + running    /bin/sleep 0.3 | \n       running    cat | \n       running    cat\n",
		},
		{"a pipeline inside a group is one row", "{ /bin/sleep 0.3 | cat } & jobs", "[1]  + running    { /bin/sleep 0.3 | cat; }\n"},
		{
			"the parameter keeps the raw status",
			"exit 3 | /bin/sleep 0.3 & /bin/sleep 0.1; print ${(kv)jobstates}",
			"1 running:+:P=exit 768:P=running\n",
		},
		{
			"the parameter shows only a reaped end",
			"true | /bin/sleep 0.3 & print ${(kv)jobstates}",
			"1 running:+:P=running:P=running\n",
		},
		{"the forked elements are a job", "true | { jobs }", "[1]    running    true\n"},
		{"a builtin last element is not", "true | jobs", ""},
		{"a function last element is", "f() { jobs }; true | f", "[1]    running    true\n"},
		{"even inside a held slot", "f() { true | { jobs } }; f", "[2]    running    true\n"},
		{
			"a job started there takes the +",
			"true | { /bin/sleep 0.3 & jobs }",
			"[1]  - done       true\n[2]  + running    /bin/sleep 0.3\n",
		},
		{"a wait notices the end", "true | { /bin/sleep 0.1; jobs }", "[1]    done       true\n"},
		{
			"several forked elements",
			"true | { : } | { : } | { jobs }",
			"[1]    done       true | \n       done       { :; } | \n       running    { :; }\n",
		},
		{
			"named, not current",
			"true | { jobs %1; echo s=$?; jobs %% 2>&1; echo t=$? }",
			"[1]    running    true\ns=0\nzsh:jobs:1: no current job\nt=127\n",
		},
		{
			"listed done once, then forgotten",
			"true | { /bin/sleep 0.1; jobs; jobs; wait %1 2>&1; echo w=$? }",
			"[1]    done       true\nzsh:wait:1: %1: no such job\nw=127\n",
		},
		{"a wait for it waits for its processes", "/bin/sleep 0.2 | { wait %1; echo w=$? }", "w=0\n"},
		{"a pipeline inside notices the end", "true | { true | true; jobs }", "[1]    done       true\n"},
		{"its own status is not the pipeline's", "exit 4 | { /bin/sleep 0.1; wait %1; echo w=$? }", "w=0\n"},
		{
			"gone once the pipeline is",
			"true | { /bin/sleep 1 & }; jobs; jobs %- 2>&1; echo $?",
			"[2]  + running    /bin/sleep 1\nzsh:jobs:1: no previous job\n127\n",
		},
		{"the parameter lists it", "true | { /bin/sleep 0.1; print ${(kv)jobstates} }", "1 done::P=done\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := pidsInRows.ReplaceAllString(runZshC(c.src), "${1}P$2")
			if got != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, got, c.want)
			}
		})
	}
}

// pidsInRows masks a process id where a row or a `$jobstates` value has one.
var pidsInRows = regexp.MustCompile(`(\]  [+-] |       |:)[0-9]{3,}( |=)`)

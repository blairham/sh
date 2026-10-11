// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package startupcost

import "strconv"

// The construct table: one loop per piece of a simple command, so that a cost
// creeping into one of them is seen the week it starts (#6353).
//
// The gate's workload is one program, and a program averages. The step at
// v0.0.3 and the creep that followed it were both invisible to it for weeks,
// because every row moved a little and no row moved alone: `[` became six
// times what it cost at v0.0.2 while the workload read "a bit slower". Each
// row here isolates one construct over the same `(( ))` loop, so subtracting
// the first row from any other is what that construct costs per iteration,
// and a row that moves while its neighbors do not names its own cause.
//
// Every program prints what it computed, for the reason workloadProgram does:
// a shell that refused the construct would post the best time in the table.

// Construct is one row: a loop that exercises one construct.
type Construct struct {
	// Name is what the row is reported under.
	Name string
	// Program is the text handed to `-c`.
	Program string
	// Answer is what the program must print for its time to count.
	Answer string
}

// ConstructIterations is how many times each row's loop runs. The issue's
// table was read at 100,000; a tenth of that keeps a `make startup` pass in
// seconds per row while leaving the construct, not the process start, as most
// of what is timed.
const ConstructIterations = 10000

// Constructs are the rows, in the order #6353's table reads them. Every one is
// in the language bash and zsh share, so the same text runs under both
// dialects and both references.
func Constructs() []Construct {
	n := strconv.Itoa(ConstructIterations)
	loop := func(body string) string {
		return "i=0; while (( i<" + n + " )); do (( i++ )); " + body + "done; "
	}
	return []Construct{
		{"arith", loop("") + "echo $i", n},
		{"arith-dollar", "i=0; while (( i<" + n + " )); do i=$((i+1)); done; echo $i", n},
		{"test", "i=0; while [ $i -lt " + n + " ]; do (( i++ )); done; echo $i", n},
		{"dbracket", "i=0; while [[ $i -lt " + n + " ]]; do (( i++ )); done; echo $i", n},
		{"assign", loop("x=$i; ") + "echo $x", n},
		{"builtin", loop(": $i; ") + "echo $i", n},
		{"function", "f() { :; }; " + loop("f; ") + "echo $i", n},
		{"trim", "s=a/b/c; " + loop("x=${s#*/}; ") + "echo $x", "b/c"},
		{"arith-expand", loop("x=$((i*2+1)); ") + "echo $x", strconv.Itoa(2*ConstructIterations + 1)},
	}
}

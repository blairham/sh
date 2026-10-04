// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"io"
	"testing"
)

// TestAnEmptyOnlyIsRefused: -only given with nothing in it is a mistake to
// refuse, not a request for the whole corpus; left off, it is (#5707).
func TestAnEmptyOnlyIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{"flag left off", nil, false},
		{"empty", []string{"-only", ""}, true},
		{"only separators", []string{"-only", " , ,"}, true},
		{"a case", []string{"-only", "cd/two-operands"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("oracle", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			only := fs.String("only", "", "")
			if err := fs.Parse(tc.args); err != nil {
				t.Fatal(err)
			}
			if got := onlyGivenEmpty(fs, *only); got != tc.want {
				t.Errorf("onlyGivenEmpty(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

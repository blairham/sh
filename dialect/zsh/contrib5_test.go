// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strconv"
	"strings"
	"testing"
)

// catch, throw, zmathfunc and zstyle+ against rows measured from zsh 5.9.2
// with its own copies; testdata/contrib5.tsv's header says how. The prelude
// is one line, as the measurement's was, so a diagnostic names the same line.
func TestCatchThrowZmathfuncZstylePlusAnswerWhatZshAnswers(t *testing.T) {
	dir := t.TempDir()
	for _, row := range contribRows(t, "contrib5.tsv") {
		t.Run(row[0], func(t *testing.T) {
			out, st := runZshOnPath(t, dir, "fpath=("+shippedFunctionDir(t)+"); autoload -Uz catch throw zmathfunc zstyle+\n"+row[0]+"\n")
			if got := strings.ReplaceAll(out, "\n", "~"); got != row[1] {
				t.Errorf("output %q, want %q", got, row[1])
			}
			if got := strconv.Itoa(st); got != row[2] {
				t.Errorf("status %s, want %s", got, row[2])
			}
		})
	}
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A subscript in a read-modify-write is evaluated once here too: measured
// 2026-10-02 on bash 5.3.20 (and ksh93u+ the same), `array=(1); x=0; ((
// array[++x]++ ))` leaves x at 1 and the array two elements long, and `((
// a[i++] += 10 ))` steps i once (#5145).
func TestAReadModifyWriteSubscriptIsEvaluatedOnce(t *testing.T) {
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	driver.MainArgs(sh, []string{"bash", "-c", `array=(1); x=0; (( array[++x]++ )); echo $x ${#array[@]} ${array[@]}; a=(5 6 7); i=0; (( a[i++] += 10 )); echo $i ${a[@]}; b=(1 2 3); (( b[-1]++ )); echo ${b[@]}`})
	if got, want := out.String(), "1 2 1 1\n1 15 6 7\n1 2 4\n"; got != want || errs.Len() != 0 {
		t.Errorf("got %q, %q, want %q", got, errs.String(), want)
	}
}

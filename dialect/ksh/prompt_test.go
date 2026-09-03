// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
)

// // Measured: with PS1='<$LOGNAME>@ ' exported, real ksh93 draws <bhamilton>@.
//
// Its other two prompt habits — a bare ! becoming the history number, and a
// backslash being dropped from whatever follows — are separate questions and
// are not answered by this field.

func TestPromptStyle(t *testing.T) {
	if got := ksh.PromptStyle().Expand; got != true {
		t.Errorf("Expand = %v, want %v", got, true)
	}
}

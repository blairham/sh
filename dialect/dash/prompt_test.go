// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
)

// // Measured: with PS1='<$LOGNAME>@ ' exported, real dash draws <bhamilton>@,
// and with PS1='<$((1+1))>@ ' it draws <2>@.

func TestPromptStyle(t *testing.T) {
	if got := dash.PromptStyle().Expand; got != true {
		t.Errorf("Expand = %v, want %v", got, true)
	}
}

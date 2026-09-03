// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
)

// // Measured: with PS1='<$LOGNAME>@ ' exported, real bash draws <bhamilton>@,
// and with PS1='<$(echo sub)>@ ' it draws <sub>@ — expanded, and expanded
// again at every prompt.

func TestPromptStyle(t *testing.T) {
	if got := bash.PromptStyle().Expand; got != true {
		t.Errorf("Expand = %v, want %v", got, true)
	}
}

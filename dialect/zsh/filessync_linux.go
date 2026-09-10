// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "syscall"

// The flush `zf_sync` performs, on the family of Unix whose call cannot fail
// and reports nothing. See filessync_darwin.go for the constant.
const fileSyncSupported = true

func fileSync() error {
	syscall.Sync()
	return nil
}

// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "syscall"

// The flush `zf_sync` performs, on the family of Unix whose call reports
// whether it worked.
//
// fileSyncSupported gates the *registration* of `zf_sync` rather than its
// behavior: a platform that is not here does not get the name at all, so
// `zmodload -F zsh/files b:zf_sync` refuses by it rather than accepting the
// command and failing inside it.
const fileSyncSupported = true

func fileSync() error { return syscall.Sync() }

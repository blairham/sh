// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package files

import "errors"

// A platform whose flush-the-buffers call this package has not been taught.
// `zf_sync` is not registered — see dialect/zsh/modules/files/sync_darwin.go for why that is the
// honest shape — so nothing reaches this.

const fileSyncSupported = false

func fileSync() error { return errors.ErrUnsupported }

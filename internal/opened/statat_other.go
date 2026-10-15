// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package opened

import "io/fs"

const statAtSupported = false

// traverseChecksEveryLookup: nothing here is asked through a hold.
const traverseChecksEveryLookup = false

func statAtOnce(int, string, bool) (fs.FileInfo, error) { return nil, errStatAtUnsupported }

func accessAtOnce(int, string, uint32) error { return errStatAtUnsupported }

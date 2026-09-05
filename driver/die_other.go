// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package driver

import "syscall"

// A platform whose signal-disposition call was not measured says so rather
// than guessing at a structure layout, exactly as rlimit_other.go declines to
// invent a resource number. The raise still happens, and dieBySignal's grace
// period turns what would have been a wedged shell into an exit status of 128
// plus the number — wrong, and reported, which is the pair of properties a
// hang does not have.
func restoreDefaultDisposition(syscall.Signal) error { return nil }

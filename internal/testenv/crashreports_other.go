// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin

package testenv

// keepCrashReportsOut has nothing to do off darwin. On Linux a shell half that
// dies of a core-dumping signal is kept from being dumped by the prctl its
// suite already makes, which covers a crash reporter piped from core_pattern
// too. See crashreports_darwin.go.
func keepCrashReportsOut() {}

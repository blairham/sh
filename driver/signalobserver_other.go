// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin && !linux

package driver

import "os"

// A platform nothing here was measured on observes nothing: what darwin and
// Linux do for the signals os/signal is never told about is in
// signalobserver_darwin.go and signalobserver_linux.go.

func observeTheUnforwarded(chan os.Signal) (stop func()) { return nil }

func heldByTheObserver(int) bool { return false }

func profToItsDefault() {}

func profToTheRuntime() {}

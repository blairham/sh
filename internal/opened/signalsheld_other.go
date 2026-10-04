// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !darwin

package opened

// WithSignalsHeld is do, on a platform whose FIFO open does not lose its
// peer to a signal. See the Darwin implementation for the one that does.
func WithSignalsHeld[T any](do func() (T, error)) (T, error) { return do() }

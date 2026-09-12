// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package tty

import "os"

// modeState is nothing on a platform with no termios, which is what makes
// every call in mode.go answer ErrUnsupported rather than pretend.
type modeState = struct{}

func setMode(*os.File, bool) (*Mode, error) { return nil, ErrUnsupported }

func putMode(*os.File, modeState) error { return ErrUnsupported }

func postProcessesOutput(*os.File) bool { return false }

func clearOutputPostProcessing(*os.File) error { return ErrUnsupported }

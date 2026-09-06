// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package repl

import (
	"errors"
	"io"
	"os"
)

// A platform with no pseudo-terminal keeps no output unless it was asked to.
//
// The default is "capture where a terminal can be put behind it", so a
// platform that cannot put one behind it captures nothing — which is the
// behaviour every platform had before the conduit existed, kept here rather
// than degraded into the pipe-wrapping capture that costs a child its
// terminal. A session that wants that anyway still asks for it by name.
type ptyConduit struct{}

func newPtyConduit(*os.File, io.Writer, func(io.Writer) io.Writer) (*ptyConduit, error) {
	return nil, errors.New("repl: no pseudo-terminal on this platform")
}

func (c *ptyConduit) Stream() *os.File { return nil }
func (c *ptyConduit) drain()           {}
func (c *ptyConduit) close()           {}

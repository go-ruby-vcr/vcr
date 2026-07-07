// Copyright (c) 2026, the go-ruby-vcr/vcr authors
// All rights reserved. Use of this source code is governed by a
// BSD-3-Clause license that can be found in the LICENSE file.

package vcr

import "fmt"

// UnhandledRequestError is returned when an incoming request matches no recorded
// interaction and the active record mode does not permit recording it. It
// corresponds to VCR's Errors::UnhandledHTTPRequestError.
type UnhandledRequestError struct {
	CassetteName string
	Request      Request
	RecordMode   RecordMode
}

func (e *UnhandledRequestError) Error() string {
	return fmt.Sprintf(
		"vcr: an HTTP request has been made that VCR does not know how to handle: %s %s (cassette %q, record mode :%s)",
		e.Request.Method, e.Request.URI, e.CassetteName, e.RecordMode,
	)
}

// FormatError indicates that cassette bytes could not be parsed as a valid VCR
// cassette.
type FormatError struct {
	Msg string
}

func (e *FormatError) Error() string {
	return "vcr: invalid cassette: " + e.Msg
}

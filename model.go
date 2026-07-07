// Copyright (c) 2026, the go-ruby-vcr/vcr authors
// All rights reserved. Use of this source code is governed by a
// BSD-3-Clause license that can be found in the LICENSE file.

package vcr

import (
	"fmt"
	"time"
)

// Body models the {encoding, string} pair VCR stores for both request and
// response bodies.
type Body struct {
	Encoding string
	String   string
}

// Request is the recorded/incoming HTTP request as VCR models it. Headers maps a
// header name to its ordered list of values, matching the YAML schema.
type Request struct {
	Method  string
	URI     string
	Body    Body
	Headers map[string][]string
}

// Status is the response status line: numeric code plus reason phrase.
type Status struct {
	Code    int
	Message string
}

// Response is the recorded HTTP response.
type Response struct {
	Status      Status
	Headers     map[string][]string
	Body        Body
	HTTPVersion string
}

// Interaction is one recorded request/response pair with the time it was
// recorded.
type Interaction struct {
	Request    Request
	Response   Response
	RecordedAt time.Time
}

// RecordMode selects how a cassette reconciles incoming requests with what it
// already holds. It mirrors VCR's :once/:none/:new_episodes/:all symbols.
type RecordMode int

const (
	// RecordOnce replays recorded interactions; it records new ones only when the
	// cassette did not already exist on disk.
	RecordOnce RecordMode = iota
	// RecordNone replays only; an unmatched request is an error.
	RecordNone
	// RecordNewEpisodes replays known interactions and records new ones.
	RecordNewEpisodes
	// RecordAll never replays and always records, re-writing the cassette.
	RecordAll
)

// String returns the VCR symbol name (without the leading colon).
func (m RecordMode) String() string {
	switch m {
	case RecordOnce:
		return "once"
	case RecordNone:
		return "none"
	case RecordNewEpisodes:
		return "new_episodes"
	case RecordAll:
		return "all"
	default:
		return "unknown"
	}
}

// ParseRecordMode maps a VCR record-mode name to a RecordMode. It accepts both
// the bare name ("once") and the Ruby symbol spelling (":once").
func ParseRecordMode(s string) (RecordMode, error) {
	switch s {
	case "once", ":once":
		return RecordOnce, nil
	case "none", ":none":
		return RecordNone, nil
	case "new_episodes", ":new_episodes":
		return RecordNewEpisodes, nil
	case "all", ":all":
		return RecordAll, nil
	default:
		return RecordOnce, fmt.Errorf("vcr: unknown record mode %q", s)
	}
}

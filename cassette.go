// Copyright (c) 2026, the go-ruby-vcr/vcr authors
// All rights reserved. Use of this source code is governed by a
// BSD-3-Clause license that can be found in the LICENSE file.

package vcr

import "time"

// Cassette is an ordered list of recorded interactions plus the record/replay
// state machine for a single VCR.use_cassette scope. It is created by
// [VCR.UseCassette]; the block receives it and drives replay/record through
// [Cassette.Interact] (or the lower-level [Cassette.Play]/[Cassette.Record]).
type Cassette struct {
	// Name is the cassette name as passed to UseCassette (without the .yml
	// extension added for the on-disk path).
	Name string

	interactions  []Interaction
	played        []bool
	matchers      []RequestMatcher
	mode          RecordMode
	existedOnDisk bool
	dirty         bool
	allowRepeats  bool
	now           func() time.Time
}

// Mode returns the cassette's record mode.
func (c *Cassette) Mode() RecordMode { return c.mode }

// Interactions returns a copy of the interactions the cassette currently holds
// (loaded from disk plus any recorded during this scope).
func (c *Cassette) Interactions() []Interaction {
	out := make([]Interaction, len(c.interactions))
	copy(out, c.interactions)
	return out
}

// CanRecord reports whether the active record mode permits recording a new
// interaction into this cassette.
func (c *Cassette) CanRecord() bool {
	switch c.mode {
	case RecordAll, RecordNewEpisodes:
		return true
	case RecordOnce:
		return !c.existedOnDisk
	default: // RecordNone (and any invalid mode)
		return false
	}
}

// Play returns the response of the first unplayed recorded interaction whose
// request matches req under the cassette's matchers, and true. If no interaction
// matches it returns the zero Response and false. Unless playback repeats are
// allowed, each recorded interaction is played at most once.
func (c *Cassette) Play(req Request) (Response, bool) {
	for i := range c.interactions {
		if !c.allowRepeats && c.played[i] {
			continue
		}
		if matchAll(c.matchers, c.interactions[i].Request, req) {
			if !c.allowRepeats {
				c.played[i] = true
			}
			return c.interactions[i].Response, true
		}
	}
	return Response{}, false
}

// Record appends a new interaction (timestamped by the clock seam) and marks the
// cassette dirty so it is written on eject. Newly recorded interactions are
// flagged as played so they are not replayed within the same scope.
func (c *Cassette) Record(req Request, resp Response) {
	c.interactions = append(c.interactions, Interaction{
		Request:    req,
		Response:   resp,
		RecordedAt: c.now(),
	})
	c.played = append(c.played, true)
	c.dirty = true
}

// Interact is the full record/replay decision for one request. It models what
// the rbgo binding does around a bound Net::HTTP call:
//
//   - Unless the mode is RecordAll, it tries to replay a matching interaction.
//   - Otherwise, if the mode permits recording, it invokes doer to perform the
//     real request and records the result.
//   - Otherwise it returns an *UnhandledRequestError.
//
// doer is the HTTP seam: the caller performs the real request. It is only called
// when the cassette must record.
func (c *Cassette) Interact(req Request, doer func(Request) (Response, error)) (Response, error) {
	if c.mode != RecordAll {
		if resp, ok := c.Play(req); ok {
			return resp, nil
		}
	}
	if !c.CanRecord() {
		return Response{}, &UnhandledRequestError{
			CassetteName: c.Name,
			Request:      req,
			RecordMode:   c.mode,
		}
	}
	resp, err := doer(req)
	if err != nil {
		return Response{}, err
	}
	c.Record(req, resp)
	return resp, nil
}

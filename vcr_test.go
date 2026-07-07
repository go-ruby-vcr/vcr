// Copyright (c) 2026, the go-ruby-vcr/vcr authors
// All rights reserved. Use of this source code is governed by a
// BSD-3-Clause license that can be found in the LICENSE file.

package vcr

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// memFS is an in-memory filesystem seam used to drive every branch
// deterministically.
type memFS struct {
	files    map[string][]byte
	dirs     map[string]bool
	readErr  error // if non-nil, ReadFile returns it (unless the file exists)
	writeErr error
	mkdirErr error
}

func newMemFS() *memFS {
	return &memFS{files: map[string][]byte{}, dirs: map[string]bool{}}
}

func (m *memFS) FS() FS {
	return FS{
		ReadFile: func(name string) ([]byte, error) {
			if data, ok := m.files[name]; ok {
				return data, nil
			}
			if m.readErr != nil {
				return nil, m.readErr
			}
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
		},
		WriteFile: func(name string, data []byte, _ os.FileMode) error {
			if m.writeErr != nil {
				return m.writeErr
			}
			m.files[name] = append([]byte(nil), data...)
			return nil
		},
		MkdirAll: func(path string, _ os.FileMode) error {
			if m.mkdirErr != nil {
				return m.mkdirErr
			}
			m.dirs[path] = true
			return nil
		},
	}
}

func fixedClock() func() time.Time {
	t := time.Date(2011, time.November, 1, 4, 58, 44, 0, time.UTC)
	return func() time.Time { return t }
}

func newVCR(m *memFS) *VCR {
	return &VCR{
		CassetteDir: "cassettes",
		RecordMode:  RecordOnce,
		Matchers:    DefaultMatchers(),
		FS:          m.FS(),
		Now:         fixedClock(),
	}
}

func sampleRequest() Request {
	return Request{
		Method:  "get",
		URI:     "http://example.com/foo",
		Body:    Body{Encoding: "UTF-8", String: ""},
		Headers: map[string][]string{"Accept": {"*/*"}},
	}
}

func sampleResponse() Response {
	return Response{
		Status:      Status{Code: 200, Message: "OK"},
		Headers:     map[string][]string{"Content-Type": {"text/plain"}},
		Body:        Body{Encoding: "UTF-8", String: "hello"},
		HTTPVersion: "1.1",
	}
}

func okDoer(resp Response) func(Request) (Response, error) {
	return func(Request) (Response, error) { return resp, nil }
}

func TestNew(t *testing.T) {
	v := New()
	if v.CassetteDir != "fixtures/vcr_cassettes" {
		t.Fatalf("dir = %q", v.CassetteDir)
	}
	if v.RecordMode != RecordOnce {
		t.Fatalf("mode = %v", v.RecordMode)
	}
	if len(v.Matchers) != 2 {
		t.Fatalf("matchers = %d", len(v.Matchers))
	}
	if v.FS.ReadFile == nil || v.FS.WriteFile == nil || v.FS.MkdirAll == nil {
		t.Fatal("FS not populated")
	}
}

func TestRecordThenReplayOnce(t *testing.T) {
	m := newMemFS()
	v := newVCR(m)
	req := sampleRequest()
	resp := sampleResponse()

	// First scope: cassette does not exist -> records.
	var calls int
	err := v.UseCassette("greet", CassetteOptions{}, func(c *Cassette) error {
		got, err := c.Interact(req, func(r Request) (Response, error) {
			calls++
			return resp, nil
		})
		if err != nil {
			return err
		}
		if got.Body.String != "hello" {
			t.Fatalf("body = %q", got.Body.String)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("record scope: %v", err)
	}
	if calls != 1 {
		t.Fatalf("doer calls = %d, want 1", calls)
	}
	path := filepath.Join("cassettes", "greet.yml")
	if _, ok := m.files[path]; !ok {
		t.Fatalf("cassette not written; files=%v", m.files)
	}

	// Second scope: cassette exists -> replays, doer not called.
	calls = 0
	err = v.UseCassette("greet", CassetteOptions{}, func(c *Cassette) error {
		got, err := c.Interact(req, func(r Request) (Response, error) {
			calls++
			return Response{}, nil
		})
		if err != nil {
			return err
		}
		if got.Body.String != "hello" {
			t.Fatalf("replay body = %q", got.Body.String)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("replay scope: %v", err)
	}
	if calls != 0 {
		t.Fatalf("doer called on replay: %d", calls)
	}
}

func TestOnceExistingUnknownRequestErrors(t *testing.T) {
	m := newMemFS()
	v := newVCR(m)
	// Pre-populate a cassette so :once treats it as replay-only.
	m.files[filepath.Join("cassettes", "greet.yml")] = MarshalCassette([]Interaction{
		{Request: sampleRequest(), Response: sampleResponse(), RecordedAt: fixedClock()()},
	})

	unknown := sampleRequest()
	unknown.URI = "http://example.com/other"
	err := v.UseCassette("greet", CassetteOptions{}, func(c *Cassette) error {
		_, err := c.Interact(unknown, okDoer(sampleResponse()))
		return err
	})
	var ue *UnhandledRequestError
	if !errors.As(err, &ue) {
		t.Fatalf("want UnhandledRequestError, got %v", err)
	}
	if ue.RecordMode != RecordOnce {
		t.Fatalf("mode = %v", ue.RecordMode)
	}
	if msg := ue.Error(); msg == "" || !strings.Contains(msg, "example.com/other") {
		t.Fatalf("error message = %q", msg)
	}
}

func TestNoneReplayOnly(t *testing.T) {
	m := newMemFS()
	v := newVCR(m)
	m.files[filepath.Join("cassettes", "c.yml")] = MarshalCassette([]Interaction{
		{Request: sampleRequest(), Response: sampleResponse(), RecordedAt: fixedClock()()},
	})
	mode := RecordNone
	// Known request replays.
	err := v.UseCassette("c", CassetteOptions{RecordMode: &mode}, func(c *Cassette) error {
		got, err := c.Interact(sampleRequest(), okDoer(Response{}))
		if err != nil {
			return err
		}
		if got.Status.Code != 200 {
			t.Fatalf("code = %d", got.Status.Code)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("none replay: %v", err)
	}
	// Unknown request errors.
	unknown := sampleRequest()
	unknown.Method = "post"
	err = v.UseCassette("c", CassetteOptions{RecordMode: &mode}, func(c *Cassette) error {
		_, err := c.Interact(unknown, okDoer(Response{}))
		return err
	})
	var ue *UnhandledRequestError
	if !errors.As(err, &ue) {
		t.Fatalf("want UnhandledRequestError, got %v", err)
	}
}

func TestNoneMissingCassetteNotWritten(t *testing.T) {
	m := newMemFS()
	v := newVCR(m)
	mode := RecordNone
	err := v.UseCassette("missing", CassetteOptions{RecordMode: &mode}, func(c *Cassette) error {
		_, err := c.Interact(sampleRequest(), okDoer(sampleResponse()))
		return err
	})
	var ue *UnhandledRequestError
	if !errors.As(err, &ue) {
		t.Fatalf("want UnhandledRequestError, got %v", err)
	}
	if len(m.files) != 0 {
		t.Fatalf(":none must not write, files=%v", m.files)
	}
}

func TestNewEpisodesReplaysKnownRecordsNew(t *testing.T) {
	m := newMemFS()
	v := newVCR(m)
	m.files[filepath.Join("cassettes", "c.yml")] = MarshalCassette([]Interaction{
		{Request: sampleRequest(), Response: sampleResponse(), RecordedAt: fixedClock()()},
	})
	mode := RecordNewEpisodes
	newReq := sampleRequest()
	newReq.URI = "http://example.com/bar"
	newResp := sampleResponse()
	newResp.Body.String = "world"

	var calls int
	err := v.UseCassette("c", CassetteOptions{RecordMode: &mode}, func(c *Cassette) error {
		// known -> replay (no doer)
		if _, err := c.Interact(sampleRequest(), func(Request) (Response, error) {
			calls++
			return Response{}, nil
		}); err != nil {
			return err
		}
		// new -> record (doer)
		got, err := c.Interact(newReq, func(Request) (Response, error) {
			calls++
			return newResp, nil
		})
		if err != nil {
			return err
		}
		if got.Body.String != "world" {
			t.Fatalf("body = %q", got.Body.String)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("new_episodes: %v", err)
	}
	if calls != 1 {
		t.Fatalf("doer calls = %d, want 1 (only the new request)", calls)
	}
	its, err := ParseCassette(m.files[filepath.Join("cassettes", "c.yml")])
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if len(its) != 2 {
		t.Fatalf("interactions = %d, want 2", len(its))
	}
}

func TestRecordAllAlwaysRecords(t *testing.T) {
	m := newMemFS()
	v := newVCR(m)
	// Existing cassette with an interaction that would otherwise replay.
	m.files[filepath.Join("cassettes", "c.yml")] = MarshalCassette([]Interaction{
		{Request: sampleRequest(), Response: sampleResponse(), RecordedAt: fixedClock()()},
	})
	mode := RecordAll
	newResp := sampleResponse()
	newResp.Body.String = "fresh"
	var calls int
	err := v.UseCassette("c", CassetteOptions{RecordMode: &mode}, func(c *Cassette) error {
		got, err := c.Interact(sampleRequest(), func(Request) (Response, error) {
			calls++
			return newResp, nil
		})
		if err != nil {
			return err
		}
		if got.Body.String != "fresh" {
			t.Fatalf("body = %q", got.Body.String)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("all: %v", err)
	}
	if calls != 1 {
		t.Fatalf("doer calls = %d, want 1 (never replays)", calls)
	}
	its, _ := ParseCassette(m.files[filepath.Join("cassettes", "c.yml")])
	if len(its) != 1 || its[0].Response.Body.String != "fresh" {
		t.Fatalf("cassette not rewritten: %+v", its)
	}
}

func TestRecordAllEmptyWritesEmptyCassette(t *testing.T) {
	m := newMemFS()
	v := newVCR(m)
	m.files[filepath.Join("cassettes", "c.yml")] = MarshalCassette([]Interaction{
		{Request: sampleRequest(), Response: sampleResponse(), RecordedAt: fixedClock()()},
	})
	mode := RecordAll
	err := v.UseCassette("c", CassetteOptions{RecordMode: &mode}, func(c *Cassette) error {
		return nil // no requests
	})
	if err != nil {
		t.Fatalf("all-empty: %v", err)
	}
	its, err := ParseCassette(m.files[filepath.Join("cassettes", "c.yml")])
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if len(its) != 0 {
		t.Fatalf("want empty cassette, got %d", len(its))
	}
}

func TestUseCassetteReadError(t *testing.T) {
	m := newMemFS()
	m.readErr = errors.New("disk on fire")
	v := newVCR(m)
	err := v.UseCassette("c", CassetteOptions{}, func(c *Cassette) error { return nil })
	if err == nil || err.Error() != "disk on fire" {
		t.Fatalf("want read error, got %v", err)
	}
}

func TestUseCassetteParseError(t *testing.T) {
	m := newMemFS()
	m.files[filepath.Join("cassettes", "c.yml")] = []byte("http_interactions: not-a-sequence\n")
	v := newVCR(m)
	err := v.UseCassette("c", CassetteOptions{}, func(c *Cassette) error { return nil })
	var fe *FormatError
	if !errors.As(err, &fe) {
		t.Fatalf("want FormatError, got %v", err)
	}
}

func TestUseCassetteWriteError(t *testing.T) {
	m := newMemFS()
	m.writeErr = errors.New("read-only fs")
	v := newVCR(m)
	err := v.UseCassette("c", CassetteOptions{}, func(c *Cassette) error {
		c.Record(sampleRequest(), sampleResponse())
		return nil
	})
	if err == nil || err.Error() != "read-only fs" {
		t.Fatalf("want write error, got %v", err)
	}
}

func TestUseCassetteMkdirError(t *testing.T) {
	m := newMemFS()
	m.mkdirErr = errors.New("no mkdir")
	v := newVCR(m)
	err := v.UseCassette("c", CassetteOptions{}, func(c *Cassette) error {
		c.Record(sampleRequest(), sampleResponse())
		return nil
	})
	if err == nil || err.Error() != "no mkdir" {
		t.Fatalf("want mkdir error, got %v", err)
	}
}

func TestUseCassetteBlockErrorTakesPrecedence(t *testing.T) {
	m := newMemFS()
	m.writeErr = errors.New("write fail")
	v := newVCR(m)
	blockErr := errors.New("block boom")
	err := v.UseCassette("c", CassetteOptions{}, func(c *Cassette) error {
		c.Record(sampleRequest(), sampleResponse())
		return blockErr
	})
	if !errors.Is(err, blockErr) {
		t.Fatalf("want block error precedence, got %v", err)
	}
}

func TestInteractDoerError(t *testing.T) {
	m := newMemFS()
	v := newVCR(m)
	doerErr := errors.New("network down")
	err := v.UseCassette("c", CassetteOptions{}, func(c *Cassette) error {
		_, err := c.Interact(sampleRequest(), func(Request) (Response, error) {
			return Response{}, doerErr
		})
		return err
	})
	if !errors.Is(err, doerErr) {
		t.Fatalf("want doer error, got %v", err)
	}
}

func TestOptionOverrides(t *testing.T) {
	m := newMemFS()
	v := newVCR(m)
	v.Matchers = nil // force UseCassette to fall back to DefaultMatchers
	mode := RecordAll
	repeats := true
	err := v.UseCassette("c", CassetteOptions{
		RecordMode:           &mode,
		Matchers:             []RequestMatcher{MatchMethod},
		AllowPlaybackRepeats: &repeats,
	}, func(c *Cassette) error {
		if c.mode != RecordAll || !c.allowRepeats || len(c.matchers) != 1 {
			t.Fatalf("options not applied: %+v", c)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("overrides: %v", err)
	}
}

func TestDefaultMatchersFallbackWhenAllEmpty(t *testing.T) {
	m := newMemFS()
	v := newVCR(m)
	v.Matchers = nil
	err := v.UseCassette("c", CassetteOptions{}, func(c *Cassette) error {
		if len(c.matchers) != 2 {
			t.Fatalf("want default matchers, got %d", len(c.matchers))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("fallback: %v", err)
	}
}

func TestCassettePathExtensions(t *testing.T) {
	v := &VCR{CassetteDir: "d"}
	if got := v.cassettePath("a"); got != filepath.Join("d", "a.yml") {
		t.Fatalf("got %q", got)
	}
	if got := v.cassettePath("a.yml"); got != filepath.Join("d", "a.yml") {
		t.Fatalf("got %q", got)
	}
	if got := v.cassettePath("a.yaml"); got != filepath.Join("d", "a.yaml") {
		t.Fatalf("got %q", got)
	}
}

func TestAllowPlaybackRepeats(t *testing.T) {
	m := newMemFS()
	v := newVCR(m)
	v.AllowPlaybackRepeats = true
	m.files[filepath.Join("cassettes", "c.yml")] = MarshalCassette([]Interaction{
		{Request: sampleRequest(), Response: sampleResponse(), RecordedAt: fixedClock()()},
	})
	mode := RecordNone
	err := v.UseCassette("c", CassetteOptions{RecordMode: &mode}, func(c *Cassette) error {
		for i := 0; i < 3; i++ {
			if _, err := c.Interact(sampleRequest(), okDoer(Response{})); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("repeats: %v", err)
	}
}

func TestOSFSRoundTrip(t *testing.T) {
	dir := t.TempDir()
	v := &VCR{
		CassetteDir: filepath.Join(dir, "nested", "cassettes"),
		RecordMode:  RecordOnce,
		Matchers:    DefaultMatchers(),
		FS:          OSFS(),
		// Now left nil to exercise the default (time.Now) clock seam.
	}
	err := v.UseCassette("real", CassetteOptions{}, func(c *Cassette) error {
		_, err := c.Interact(sampleRequest(), okDoer(sampleResponse()))
		return err
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	path := filepath.Join(dir, "nested", "cassettes", "real.yml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cassette not on disk: %v", err)
	}
	// Second run replays from the real file (covers the os.ReadFile hit path).
	err = v.UseCassette("real", CassetteOptions{}, func(c *Cassette) error {
		got, err := c.Interact(sampleRequest(), func(Request) (Response, error) {
			t.Fatal("doer must not be called on replay")
			return Response{}, nil
		})
		if err != nil {
			return err
		}
		if got.Status.Code != 200 {
			t.Fatalf("code = %d", got.Status.Code)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
}

func TestInteractionsCopy(t *testing.T) {
	m := newMemFS()
	v := newVCR(m)
	var snap []Interaction
	_ = v.UseCassette("c", CassetteOptions{}, func(c *Cassette) error {
		c.Record(sampleRequest(), sampleResponse())
		snap = c.Interactions()
		return nil
	})
	if len(snap) != 1 {
		t.Fatalf("interactions = %d", len(snap))
	}
}

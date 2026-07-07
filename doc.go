// Copyright (c) 2026, the go-ruby-vcr/vcr authors
// All rights reserved. Use of this source code is governed by a
// BSD-3-Clause license that can be found in the LICENSE file.

// Package vcr is a pure-Go (CGO=0, stdlib-only) reimplementation of the Ruby
// gem "vcr" (https://github.com/vcr/vcr). It records HTTP interactions to
// "cassettes" and replays them on subsequent runs, so tests become
// deterministic and offline.
//
// The library is the cassette store, request matcher, and record/replay state
// machine. It deliberately does NOT perform HTTP or read a wall clock itself:
// those are seams supplied by the caller (in particular the rbgo binding, which
// intercepts its bound Net::HTTP). This keeps the whole package deterministic
// and testable with an in-memory filesystem.
//
// # Seams
//
//   - Filesystem: [FS] is a struct of func fields (ReadFile/WriteFile/MkdirAll).
//     [OSFS] returns the os-backed implementation; tests inject an in-memory one.
//   - Clock: [VCR.Now] supplies the timestamp stored as recorded_at.
//   - HTTP: [Cassette.Interact] takes a doer func(Request) (Response, error) that
//     performs the real request only when the cassette must record. The rbgo
//     binding supplies a doer wired to Net::HTTP.
//
// # Record modes
//
//   - [RecordOnce]         replay if the cassette exists, otherwise record.
//   - [RecordNone]         replay only; an unknown request is an error.
//   - [RecordNewEpisodes]  replay known requests, record new ones.
//   - [RecordAll]          never replay; always (re-)record.
//
// # Cassette format
//
// Cassettes are serialized to VCR's default YAML schema: a top-level
// http_interactions sequence of {request, response, recorded_at} maps. The
// serializer is a small purpose-built emitter/parser (no third-party YAML
// dependency) that round-trips the schema byte-for-byte.
//
// The intended Ruby surface, provided by the rbgo binding on top of this
// package, is:
//
//	VCR.configure { |c| c.cassette_library_dir = "..."; c.default_record_mode = :once }
//	VCR.use_cassette("name", record: :new_episodes) { ... }
package vcr

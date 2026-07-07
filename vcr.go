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
	"time"
)

// FS is the filesystem seam. Its func fields mirror the os package so tests can
// inject an in-memory implementation and drive every error branch
// deterministically. Use [OSFS] for the os-backed implementation.
type FS struct {
	ReadFile  func(name string) ([]byte, error)
	WriteFile func(name string, data []byte, perm os.FileMode) error
	MkdirAll  func(path string, perm os.FileMode) error
}

// OSFS returns an FS backed by the os package.
func OSFS() FS {
	return FS{
		ReadFile:  os.ReadFile,
		WriteFile: os.WriteFile,
		MkdirAll:  os.MkdirAll,
	}
}

// VCR is the top-level configuration and cassette manager. It corresponds to
// Ruby's VCR module configured via VCR.configure.
type VCR struct {
	// CassetteDir is the directory cassettes are read from and written to
	// (Ruby: cassette_library_dir).
	CassetteDir string
	// RecordMode is the default record mode for cassettes that do not override it
	// (Ruby: default_record_mode).
	RecordMode RecordMode
	// Matchers is the default matcher set; empty means DefaultMatchers.
	Matchers []RequestMatcher
	// AllowPlaybackRepeats lets a single recorded interaction be replayed more
	// than once (Ruby: allow_playback_repeats).
	AllowPlaybackRepeats bool
	// FS is the filesystem seam. Zero value is unusable; use OSFS or an injected
	// implementation.
	FS FS
	// Now is the clock seam used for recorded_at. Nil means time.Now.
	Now func() time.Time
}

// New returns a VCR with the conventional defaults: cassette directory
// "fixtures/vcr_cassettes", record mode :once, the default matchers, and the
// os-backed filesystem.
func New() *VCR {
	return &VCR{
		CassetteDir: "fixtures/vcr_cassettes",
		RecordMode:  RecordOnce,
		Matchers:    DefaultMatchers(),
		FS:          OSFS(),
	}
}

// CassetteOptions overrides per-cassette settings for a single UseCassette call.
// Nil pointers/empty slices mean "inherit the VCR default".
type CassetteOptions struct {
	RecordMode           *RecordMode
	Matchers             []RequestMatcher
	AllowPlaybackRepeats *bool
}

// clock returns the effective clock seam.
func (v *VCR) clock() func() time.Time {
	if v.Now != nil {
		return v.Now
	}
	return time.Now
}

// cassettePath builds the on-disk path for a cassette name, appending .yml when
// the name has no YAML extension.
func (v *VCR) cassettePath(name string) string {
	if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
		name += ".yml"
	}
	return filepath.Join(v.CassetteDir, name)
}

// UseCassette inserts the named cassette, runs fn with it, then ejects it,
// writing the cassette back to disk when the scope recorded anything (or when
// the mode is RecordAll). It mirrors VCR.use_cassette(name, options) { ... }.
//
// A missing cassette file is not an error (it means "no prior recordings"); any
// other filesystem error, and any cassette parse error, is returned. When fn
// returns an error the cassette is still ejected, and fn's error takes
// precedence over an eject error.
func (v *VCR) UseCassette(name string, opts CassetteOptions, fn func(*Cassette) error) error {
	path := v.cassettePath(name)

	mode := v.RecordMode
	if opts.RecordMode != nil {
		mode = *opts.RecordMode
	}
	matchers := opts.Matchers
	if len(matchers) == 0 {
		matchers = v.Matchers
	}
	if len(matchers) == 0 {
		matchers = DefaultMatchers()
	}
	repeats := v.AllowPlaybackRepeats
	if opts.AllowPlaybackRepeats != nil {
		repeats = *opts.AllowPlaybackRepeats
	}

	var loaded []Interaction
	existed := false
	data, err := v.FS.ReadFile(path)
	if err == nil {
		existed = true
		loaded, err = ParseCassette(data)
		if err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	c := &Cassette{
		Name:          name,
		matchers:      matchers,
		mode:          mode,
		existedOnDisk: existed,
		allowRepeats:  repeats,
		now:           v.clock(),
	}
	// RecordAll re-records from scratch: prior interactions are neither replayed
	// nor carried into the rewritten cassette.
	if mode != RecordAll {
		c.interactions = append(c.interactions, loaded...)
		c.played = make([]bool, len(loaded))
	}

	ferr := fn(c)
	werr := v.eject(c, path)
	if ferr != nil {
		return ferr
	}
	return werr
}

// eject writes the cassette back when it recorded anything, or unconditionally
// for RecordAll (which rewrites the file even when empty).
func (v *VCR) eject(c *Cassette, path string) error {
	if !c.dirty && c.mode != RecordAll {
		return nil
	}
	data := MarshalCassette(c.interactions)
	if err := v.FS.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return v.FS.WriteFile(path, data, 0o644)
}

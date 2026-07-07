// Copyright (c) 2026, the go-ruby-vcr/vcr authors
// All rights reserved. Use of this source code is governed by a
// BSD-3-Clause license that can be found in the LICENSE file.

package vcr

import "testing"

func TestRecordModeString(t *testing.T) {
	cases := map[RecordMode]string{
		RecordOnce:        "once",
		RecordNone:        "none",
		RecordNewEpisodes: "new_episodes",
		RecordAll:         "all",
		RecordMode(99):    "unknown",
	}
	for m, want := range cases {
		if got := m.String(); got != want {
			t.Fatalf("RecordMode(%d).String() = %q, want %q", m, got, want)
		}
	}
}

func TestParseRecordMode(t *testing.T) {
	ok := map[string]RecordMode{
		"once":          RecordOnce,
		":once":         RecordOnce,
		"none":          RecordNone,
		":none":         RecordNone,
		"new_episodes":  RecordNewEpisodes,
		":new_episodes": RecordNewEpisodes,
		"all":           RecordAll,
		":all":          RecordAll,
	}
	for s, want := range ok {
		got, err := ParseRecordMode(s)
		if err != nil || got != want {
			t.Fatalf("ParseRecordMode(%q) = %v, %v", s, got, err)
		}
	}
	if _, err := ParseRecordMode("bogus"); err == nil {
		t.Fatal("want error for bogus mode")
	}
}

func TestCanRecord(t *testing.T) {
	cases := []struct {
		mode    RecordMode
		existed bool
		want    bool
	}{
		{RecordAll, true, true},
		{RecordNewEpisodes, true, true},
		{RecordNone, false, false},
		{RecordOnce, false, true},
		{RecordOnce, true, false},
		{RecordMode(99), false, false},
	}
	for _, tc := range cases {
		c := &Cassette{mode: tc.mode, existedOnDisk: tc.existed}
		if got := c.CanRecord(); got != tc.want {
			t.Fatalf("CanRecord(mode=%v existed=%v) = %v, want %v", tc.mode, tc.existed, got, tc.want)
		}
	}
}

func TestCassetteMode(t *testing.T) {
	c := &Cassette{mode: RecordNewEpisodes}
	if c.Mode() != RecordNewEpisodes {
		t.Fatalf("Mode() = %v", c.Mode())
	}
}

// Copyright (c) 2026, the go-ruby-vcr/vcr authors
// All rights reserved. Use of this source code is governed by a
// BSD-3-Clause license that can be found in the LICENSE file.

package vcr

import "testing"

func TestMatchMethodURIBody(t *testing.T) {
	a := Request{Method: "get", URI: "http://x/", Body: Body{String: "b"}}
	b := a

	if !MatchMethod(a, b) || !MatchURI(a, b) || !MatchBody(a, b) {
		t.Fatal("identical requests should match")
	}
	b.Method = "post"
	if MatchMethod(a, b) {
		t.Fatal("methods differ")
	}
	b = a
	b.URI = "http://y/"
	if MatchURI(a, b) {
		t.Fatal("URIs differ")
	}
	b = a
	b.Body.String = "c"
	if MatchBody(a, b) {
		t.Fatal("bodies differ")
	}
}

func TestMatchHeaders(t *testing.T) {
	a := Request{Headers: map[string][]string{"A": {"1", "2"}}}
	b := Request{Headers: map[string][]string{"A": {"1", "2"}}}
	if !MatchHeaders(a, b) {
		t.Fatal("equal headers should match")
	}
	// different length
	if MatchHeaders(a, Request{Headers: map[string][]string{"A": {"1", "2"}, "B": {"x"}}}) {
		t.Fatal("different sizes must not match")
	}
	// missing key
	if MatchHeaders(a, Request{Headers: map[string][]string{"C": {"1", "2"}}}) {
		t.Fatal("missing key must not match")
	}
	// different value length
	if MatchHeaders(a, Request{Headers: map[string][]string{"A": {"1"}}}) {
		t.Fatal("different value length must not match")
	}
	// different value
	if MatchHeaders(a, Request{Headers: map[string][]string{"A": {"1", "3"}}}) {
		t.Fatal("different value must not match")
	}
}

func TestMatchAll(t *testing.T) {
	a := Request{Method: "get", URI: "http://x/"}
	b := a
	if !matchAll(DefaultMatchers(), a, b) {
		t.Fatal("default matchers should accept identical requests")
	}
	b.URI = "http://y/"
	if matchAll(DefaultMatchers(), a, b) {
		t.Fatal("default matchers should reject differing URI")
	}
}

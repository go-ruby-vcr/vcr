// Copyright (c) 2026, the go-ruby-vcr/vcr authors
// All rights reserved. Use of this source code is governed by a
// BSD-3-Clause license that can be found in the LICENSE file.

package vcr

// RequestMatcher reports whether an incoming request should be considered a
// match for a previously recorded request. The first argument is the recorded
// request, the second is the incoming one, mirroring VCR's request matchers.
type RequestMatcher func(recorded, incoming Request) bool

// MatchMethod matches on the HTTP method.
func MatchMethod(recorded, incoming Request) bool {
	return recorded.Method == incoming.Method
}

// MatchURI matches on the full request URI.
func MatchURI(recorded, incoming Request) bool {
	return recorded.URI == incoming.URI
}

// MatchBody matches on the request body string.
func MatchBody(recorded, incoming Request) bool {
	return recorded.Body.String == incoming.Body.String
}

// MatchHeaders matches on the full set of request headers (names and their
// ordered values).
func MatchHeaders(recorded, incoming Request) bool {
	return headersEqual(recorded.Headers, incoming.Headers)
}

// DefaultMatchers returns VCR's default matcher set: method and URI.
func DefaultMatchers() []RequestMatcher {
	return []RequestMatcher{MatchMethod, MatchURI}
}

// matchAll reports whether every matcher accepts the pair.
func matchAll(ms []RequestMatcher, recorded, incoming Request) bool {
	for _, m := range ms {
		if !m(recorded, incoming) {
			return false
		}
	}
	return true
}

// headersEqual compares two header maps for exact equality.
func headersEqual(x, y map[string][]string) bool {
	if len(x) != len(y) {
		return false
	}
	for k, xv := range x {
		yv, ok := y[k]
		if !ok {
			return false
		}
		if len(xv) != len(yv) {
			return false
		}
		for i := range xv {
			if xv[i] != yv[i] {
				return false
			}
		}
	}
	return true
}

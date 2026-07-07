// Copyright (c) 2026, the go-ruby-vcr/vcr authors
// All rights reserved. Use of this source code is governed by a
// BSD-3-Clause license that can be found in the LICENSE file.

package vcr

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

const goldenCassette = `---
http_interactions:
- request:
    method: get
    uri: http://example.com/foo
    body:
      encoding: UTF-8
      string: ''
    headers:
      Accept:
      - '*/*'
  response:
    status:
      code: 200
      message: OK
    headers:
      Content-Type:
      - text/plain
    body:
      encoding: UTF-8
      string: hello
    http_version: '1.1'
  recorded_at: Tue, 01 Nov 2011 04:58:44 GMT
recorded_with: go-ruby-vcr 0.1.0
`

func goldenInteraction() Interaction {
	return Interaction{
		Request:    sampleRequest(),
		Response:   sampleResponse(),
		RecordedAt: time.Date(2011, time.November, 1, 4, 58, 44, 0, time.UTC),
	}
}

func TestMarshalGoldenBytes(t *testing.T) {
	got := string(MarshalCassette([]Interaction{goldenInteraction()}))
	if got != goldenCassette {
		t.Fatalf("golden mismatch\n--- got ---\n%s\n--- want ---\n%s", got, goldenCassette)
	}
}

func TestMarshalEmpty(t *testing.T) {
	got := string(MarshalCassette(nil))
	want := "---\nhttp_interactions: []\nrecorded_with: go-ruby-vcr 0.1.0\n"
	if got != want {
		t.Fatalf("empty marshal = %q, want %q", got, want)
	}
	its, err := ParseCassette([]byte(got))
	if err != nil {
		t.Fatalf("parse empty: %v", err)
	}
	if len(its) != 0 {
		t.Fatalf("want 0 interactions, got %d", len(its))
	}
}

func TestRoundTripGolden(t *testing.T) {
	it := goldenInteraction()
	data := MarshalCassette([]Interaction{it})
	got, err := ParseCassette(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("interactions = %d", len(got))
	}
	if !reflect.DeepEqual(got[0], it) {
		t.Fatalf("round-trip mismatch:\n got=%+v\nwant=%+v", got[0], it)
	}
	// Re-marshal must reproduce identical bytes.
	if string(MarshalCassette(got)) != string(data) {
		t.Fatal("re-marshal not byte-identical")
	}
}

func TestRoundTripSpecialBodies(t *testing.T) {
	// Body exercising every doubleQuote/unescapeDouble branch plus an empty
	// response HTTPVersion (omitted) and empty headers ({}).
	it := Interaction{
		Request: Request{
			Method:  "post",
			URI:     "https://api.example.com/v1/things?x=1&y=2",
			Body:    Body{Encoding: "UTF-8", String: "a\"b\\c\nd\te\rf\x01g"},
			Headers: map[string][]string{},
		},
		Response: Response{
			Status:  Status{Code: 404, Message: "Not Found"},
			Body:    Body{Encoding: "UTF-8", String: "line1\nline2"},
			Headers: map[string][]string{},
		},
		RecordedAt: time.Date(2020, 2, 3, 10, 20, 30, 0, time.UTC),
	}
	data := MarshalCassette([]Interaction{it})
	got, err := ParseCassette(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !reflect.DeepEqual(got[0], it) {
		t.Fatalf("special round-trip mismatch:\n got=%+v\nwant=%+v", got[0], it)
	}
	if string(MarshalCassette(got)) != string(data) {
		t.Fatal("re-marshal not byte-identical")
	}
}

func TestEncodeScalarVariants(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "''"},
		{"get", "get"},
		{"text/plain", "text/plain"},
		{"http://x/y", "http://x/y"},
		{"9606", "'9606'"},    // numeric-looking -> quoted
		{"true", "'true'"},    // bool-looking -> quoted
		{"~", "'~'"},          // null-looking -> quoted
		{"*/*", "'*/*'"},      // indicator start -> quoted
		{"ab ", "'ab '"},      // trailing space -> quoted
		{"a: b", "'a: b'"},    // colon-space -> quoted
		{"a:", "'a:'"},        // trailing colon -> quoted
		{"a #b", "'a #b'"},    // space-hash -> quoted
		{"@it's", "'@it''s'"}, // single-quote escaping
	}
	for _, tc := range cases {
		if got := encodeScalar(tc.in); got != tc.want {
			t.Fatalf("encodeScalar(%q) = %q, want %q", tc.in, got, tc.want)
		}
		// Every encoded scalar must decode back to the input.
		if dec := decodeScalar(tc.want); dec != tc.in {
			t.Fatalf("decodeScalar(%q) = %q, want %q", tc.want, dec, tc.in)
		}
	}
}

func TestDecodeScalarEdgeEscapes(t *testing.T) {
	cases := []struct{ in, want string }{
		{`"\z"`, "z"},       // unknown escape -> literal
		{`"\x"`, "x"},       // truncated \x
		{`"\xZZ"`, "xZZ"},   // invalid hex digits
		{`"a\` + `"`, `a\`}, // trailing backslash
		{"plain", "plain"},  // bare plain scalar
	}
	for _, tc := range cases {
		if got := decodeScalar(tc.in); got != tc.want {
			t.Fatalf("decodeScalar(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- parser edge shapes ------------------------------------------------------

func parseYAML(s string) any {
	p := &yparser{lines: splitLines([]byte(s))}
	if len(p.lines) == 0 {
		return nil
	}
	return p.parse(p.lines[0].indent)
}

func TestParseAtEnd(t *testing.T) {
	// parse called with an exhausted cursor returns nil.
	p := &yparser{}
	if got := p.parse(0); got != nil {
		t.Fatalf("parse at end = %#v", got)
	}
}

func TestParserShapes(t *testing.T) {
	if got := parseYAML("- a\n- b"); !reflect.DeepEqual(got, []any{"a", "b"}) {
		t.Fatalf("scalar seq = %#v", got)
	}
	if got := parseYAML("-\n  a: b"); !reflect.DeepEqual(got, []any{map[string]any{"a": "b"}}) {
		t.Fatalf("dash-then-map = %#v", got)
	}
	if got := parseYAML("-"); !reflect.DeepEqual(got, []any{nil}) {
		t.Fatalf("lone dash = %#v", got)
	}
	if got := parseYAML("k:\n  x: 1\nk2: v"); !reflect.DeepEqual(got, map[string]any{"k": map[string]any{"x": "1"}, "k2": "v"}) {
		t.Fatalf("nested map = %#v", got)
	}
	if got := parseYAML("k:"); !reflect.DeepEqual(got, map[string]any{"k": nil}) {
		t.Fatalf("empty at EOF = %#v", got)
	}
	if got := parseYAML("k:\nk2: v"); !reflect.DeepEqual(got, map[string]any{"k": nil, "k2": "v"}) {
		t.Fatalf("empty then sibling = %#v", got)
	}
	if got := parseYAML("k:\n- a"); !reflect.DeepEqual(got, map[string]any{"k": []any{"a"}}) {
		t.Fatalf("seq at same indent = %#v", got)
	}
	if got := parseYAML("k: {}"); !reflect.DeepEqual(got, map[string]any{"k": map[string]any{}}) {
		t.Fatalf("flow empty map = %#v", got)
	}
	if got := parseYAML("k: []"); !reflect.DeepEqual(got, map[string]any{"k": []any{}}) {
		t.Fatalf("flow empty seq = %#v", got)
	}
	if got := parseYAML("plain scalar text"); got != "plain scalar text" {
		t.Fatalf("top scalar = %#v", got)
	}
	if got := parseYAML("k: v\nscalarline"); !reflect.DeepEqual(got, map[string]any{"k": "v"}) {
		t.Fatalf("map then stray scalar = %#v", got)
	}
	if got := parseYAML("k: v\n- a"); !reflect.DeepEqual(got, map[string]any{"k": "v"}) {
		t.Fatalf("map then dash = %#v", got)
	}
}

func TestSplitKeyQuoted(t *testing.T) {
	if got := parseYAML(`"a b": v`); !reflect.DeepEqual(got, map[string]any{"a b": "v"}) {
		t.Fatalf("quoted key = %#v", got)
	}
	if got := parseYAML(`'x''y': v`); !reflect.DeepEqual(got, map[string]any{"x'y": "v"}) {
		t.Fatalf("single-quoted key = %#v", got)
	}
	if got := parseYAML(`"abc"`); got != "abc" {
		t.Fatalf("quoted scalar (no colon) = %#v", got)
	}
	if got := parseYAML(`"a" b`); got != `"a" b` {
		t.Fatalf("quoted then non-colon = %#v", got)
	}
	if got := parseYAML(`"abc`); got != `"abc` {
		t.Fatalf("unterminated quote = %#v", got)
	}
}

// --- decode error branches ---------------------------------------------------

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"top not mapping":              "- a\n",
		"interactions not seq":         "http_interactions: scalar\n",
		"interaction not mapping":      "http_interactions:\n- justscalar\n",
		"missing request":              "http_interactions:\n- response:\n    status:\n      code: 200\n",
		"missing response":             "http_interactions:\n- request:\n    method: get\n",
		"bad recorded_at":              "http_interactions:\n- request:\n    method: get\n  response:\n    status:\n      code: 200\n  recorded_at: not-a-date\n",
		"status not mapping":           "http_interactions:\n- request:\n    method: get\n  response:\n    status: oops\n",
		"bad code":                     "http_interactions:\n- request:\n    method: get\n  response:\n    status:\n      code: abc\n",
		"body not mapping":             "http_interactions:\n- request:\n    method: get\n    body: oops\n  response:\n    status:\n      code: 200\n",
		"headers not mapping":          "http_interactions:\n- request:\n    method: get\n    headers: oops\n  response:\n    status:\n      code: 200\n",
		"header value not seq":         "http_interactions:\n- request:\n    method: get\n    headers:\n      Accept: notaseq\n  response:\n    status:\n      code: 200\n",
		"header entry not scalar":      "http_interactions:\n- request:\n    method: get\n    headers:\n      Accept:\n      - k: v\n  response:\n    status:\n      code: 200\n",
		"response headers not mapping": "http_interactions:\n- request:\n    method: get\n  response:\n    status:\n      code: 200\n    headers: oops\n",
		"response body not mapping":    "http_interactions:\n- request:\n    method: get\n  response:\n    status:\n      code: 200\n    body: oops\n",
	}
	for name, doc := range cases {
		_, err := ParseCassette([]byte(doc))
		var fe *FormatError
		if !errors.As(err, &fe) {
			t.Fatalf("%s: want FormatError, got %v", name, err)
		}
		if fe.Error() == "" {
			t.Fatalf("%s: empty error message", name)
		}
	}
}

func TestParseNoInteractionsKey(t *testing.T) {
	// http_interactions absent -> nil, nil.
	its, err := ParseCassette([]byte("recorded_with: x\n"))
	if err != nil || its != nil {
		t.Fatalf("absent key: %v %v", its, err)
	}
	// http_interactions present but empty (nil value) -> nil, nil.
	its, err = ParseCassette([]byte("http_interactions:\nrecorded_with: x\n"))
	if err != nil || its != nil {
		t.Fatalf("empty value: %v %v", its, err)
	}
	// Completely empty input.
	its, err = ParseCassette(nil)
	if err != nil || its != nil {
		t.Fatalf("empty input: %v %v", its, err)
	}
}

func TestParseMinimalInteraction(t *testing.T) {
	// A valid interaction that omits body/headers/message/http_version and
	// recorded_at, exercising the nil branches of the decoders and asString.
	doc := "" +
		"http_interactions:\n" +
		"- request:\n" +
		"    method: get\n" +
		"    uri: http://x\n" +
		"    headers:\n" +
		"      Empty:\n" +
		"  response:\n" +
		"    status:\n" +
		"      code: 204\n" +
		"recorded_with: x\n"
	its, err := ParseCassette([]byte(doc))
	if err != nil {
		t.Fatalf("minimal: %v", err)
	}
	if len(its) != 1 {
		t.Fatalf("interactions = %d", len(its))
	}
	it := its[0]
	if it.Request.Method != "get" || it.Request.URI != "http://x" {
		t.Fatalf("request = %+v", it.Request)
	}
	if it.Response.Status.Code != 204 || it.Response.Status.Message != "" {
		t.Fatalf("status = %+v", it.Response.Status)
	}
	if it.Response.HTTPVersion != "" {
		t.Fatalf("http_version = %q", it.Response.HTTPVersion)
	}
	if v, ok := it.Request.Headers["Empty"]; !ok || v != nil {
		t.Fatalf("empty header = %#v (ok=%v)", v, ok)
	}
	if !it.RecordedAt.IsZero() {
		t.Fatalf("recorded_at = %v, want zero", it.RecordedAt)
	}
}

func TestRoundTripEmptyHeaders(t *testing.T) {
	// nil headers marshal to `headers: {}` and parse back to an empty map,
	// exercising decodeHeaders' len==0 branch.
	it := Interaction{
		Request:    Request{Method: "get", URI: "http://x", Body: Body{Encoding: "UTF-8"}},
		Response:   Response{Status: Status{Code: 200, Message: "OK"}, Body: Body{Encoding: "UTF-8"}},
		RecordedAt: time.Date(2021, 5, 6, 7, 8, 9, 0, time.UTC),
	}
	data := MarshalCassette([]Interaction{it})
	got, err := ParseCassette(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got[0].Request.Headers == nil || len(got[0].Request.Headers) != 0 {
		t.Fatalf("headers = %#v, want empty map", got[0].Request.Headers)
	}
	if string(MarshalCassette(got)) != string(data) {
		t.Fatal("re-marshal not byte-identical")
	}
}

func TestSplitLinesSkipsMarkersAndBlanks(t *testing.T) {
	lines := splitLines([]byte("---\n\n  \nkey: value\r\n"))
	if len(lines) != 1 || lines[0].text != "key: value" || lines[0].indent != 0 {
		t.Fatalf("splitLines = %#v", lines)
	}
}

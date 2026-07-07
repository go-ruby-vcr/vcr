// Copyright (c) 2026, the go-ruby-vcr/vcr authors
// All rights reserved. Use of this source code is governed by a
// BSD-3-Clause license that can be found in the LICENSE file.

package vcr

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// recordedWith is the tool tag written as the cassette's recorded_with field,
// matching VCR's "recorded_with: VCR <version>" convention.
const recordedWith = "go-ruby-vcr 0.1.0"

// MarshalCassette serializes interactions to VCR's default cassette YAML schema.
// The output round-trips through ParseCassette byte-for-byte.
func MarshalCassette(interactions []Interaction) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	if len(interactions) == 0 {
		b.WriteString("http_interactions: []\n")
	} else {
		b.WriteString("http_interactions:\n")
		for _, it := range interactions {
			writeInteraction(&b, it)
		}
	}
	b.WriteString("recorded_with: ")
	b.WriteString(recordedWith)
	b.WriteByte('\n')
	return []byte(b.String())
}

func writeInteraction(b *strings.Builder, it Interaction) {
	// Request block. The leading "- " puts request at column 2; its children are
	// at column 4, matching VCR/Psych output.
	b.WriteString("- request:\n")
	writeScalar(b, "    ", "method", it.Request.Method)
	writeScalar(b, "    ", "uri", it.Request.URI)
	writeBody(b, "    ", it.Request.Body)
	writeHeaders(b, "    ", it.Request.Headers)

	// Response block.
	b.WriteString("  response:\n")
	b.WriteString("    status:\n")
	b.WriteString("      code: ")
	b.WriteString(strconv.Itoa(it.Response.Status.Code))
	b.WriteByte('\n')
	writeScalar(b, "      ", "message", it.Response.Status.Message)
	writeHeaders(b, "    ", it.Response.Headers)
	writeBody(b, "    ", it.Response.Body)
	if it.Response.HTTPVersion != "" {
		writeScalar(b, "    ", "http_version", it.Response.HTTPVersion)
	}

	// recorded_at.
	b.WriteString("  recorded_at: ")
	b.WriteString(it.RecordedAt.UTC().Format(http.TimeFormat))
	b.WriteByte('\n')
}

func writeScalar(b *strings.Builder, indent, key, val string) {
	b.WriteString(indent)
	b.WriteString(encodeScalar(key))
	b.WriteString(": ")
	b.WriteString(encodeScalar(val))
	b.WriteByte('\n')
}

func writeBody(b *strings.Builder, indent string, body Body) {
	b.WriteString(indent)
	b.WriteString("body:\n")
	writeScalar(b, indent+"  ", "encoding", body.Encoding)
	writeScalar(b, indent+"  ", "string", body.String)
}

func writeHeaders(b *strings.Builder, indent string, headers map[string][]string) {
	if len(headers) == 0 {
		b.WriteString(indent)
		b.WriteString("headers: {}\n")
		return
	}
	b.WriteString(indent)
	b.WriteString("headers:\n")
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		b.WriteString(indent)
		b.WriteString("  ")
		b.WriteString(encodeScalar(name))
		b.WriteString(":\n")
		for _, v := range headers[name] {
			b.WriteString(indent)
			b.WriteString("  - ")
			b.WriteString(encodeScalar(v))
			b.WriteByte('\n')
		}
	}
}

// --- scalar encoding ---------------------------------------------------------

// indicatorStart lists the characters that carry special meaning at the start of
// a YAML scalar; a value beginning with one of them must be quoted.
const indicatorStart = "!&*?|>%@`\"'#,[]{}:-" + " "

var numberRe = regexp.MustCompile(`^[-+]?(0x[0-9a-fA-F]+|[0-9]+\.?[0-9]*([eE][-+]?[0-9]+)?|\.[0-9]+([eE][-+]?[0-9]+)?)$`)

// encodeScalar renders a Go string as a YAML scalar, choosing plain, single, or
// double quoting so that decodeScalar recovers the exact bytes.
func encodeScalar(s string) string {
	if s == "" {
		return "''"
	}
	if isPlainScalar(s) {
		return s
	}
	if hasControl(s) {
		return doubleQuote(s)
	}
	return singleQuote(s)
}

// isPlainScalar reports whether s can be emitted as an unquoted YAML scalar and
// read back unchanged.
func isPlainScalar(s string) bool {
	if looksLikeNonString(s) {
		return false
	}
	if strings.IndexByte(indicatorStart, s[0]) >= 0 {
		return false
	}
	if s[len(s)-1] == ' ' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c == 0x7f {
			return false
		}
		if c == ':' && (i+1 == len(s) || s[i+1] == ' ') {
			return false
		}
		if c == '#' && s[i-1] == ' ' {
			return false
		}
	}
	return true
}

// looksLikeNonString reports whether a plain rendering of s would be read back
// by a YAML parser as a non-string (number, bool, null), forcing quoting to
// preserve the string type.
func looksLikeNonString(s string) bool {
	switch strings.ToLower(s) {
	case "", "~", "null", "true", "false", "yes", "no", "on", "off":
		return true
	}
	return numberRe.MatchString(s)
}

func hasControl(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}

func singleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func doubleQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			b.WriteString(`\\`)
		case c == '"':
			b.WriteString(`\"`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '\r':
			b.WriteString(`\r`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\x%02X`, c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// --- parsing -----------------------------------------------------------------

// ParseCassette parses cassette bytes in VCR's YAML schema into interactions.
func ParseCassette(data []byte) ([]Interaction, error) {
	lines := splitLines(data)
	if len(lines) == 0 {
		return nil, nil
	}
	p := &yparser{lines: lines}
	root := p.parse(lines[0].indent)
	m, ok := root.(map[string]any)
	if !ok {
		return nil, &FormatError{"top-level document is not a mapping"}
	}
	raw, present := m["http_interactions"]
	if !present || raw == nil {
		return nil, nil
	}
	seq, ok := raw.([]any)
	if !ok {
		return nil, &FormatError{"http_interactions is not a sequence"}
	}
	out := make([]Interaction, 0, len(seq))
	for _, item := range seq {
		im, ok := item.(map[string]any)
		if !ok {
			return nil, &FormatError{"interaction is not a mapping"}
		}
		it, err := decodeInteraction(im)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, nil
}

func decodeInteraction(m map[string]any) (Interaction, error) {
	var it Interaction
	reqRaw, ok := m["request"].(map[string]any)
	if !ok {
		return it, &FormatError{"interaction is missing request"}
	}
	req, err := decodeRequest(reqRaw)
	if err != nil {
		return it, err
	}
	respRaw, ok := m["response"].(map[string]any)
	if !ok {
		return it, &FormatError{"interaction is missing response"}
	}
	resp, err := decodeResponse(respRaw)
	if err != nil {
		return it, err
	}
	it.Request = req
	it.Response = resp
	if ra, ok := m["recorded_at"].(string); ok && ra != "" {
		t, err := time.Parse(http.TimeFormat, ra)
		if err != nil {
			return it, &FormatError{"invalid recorded_at: " + ra}
		}
		it.RecordedAt = t
	}
	return it, nil
}

func decodeRequest(m map[string]any) (Request, error) {
	var r Request
	r.Method = asString(m["method"])
	r.URI = asString(m["uri"])
	body, err := decodeBody(m["body"])
	if err != nil {
		return r, err
	}
	r.Body = body
	headers, err := decodeHeaders(m["headers"])
	if err != nil {
		return r, err
	}
	r.Headers = headers
	return r, nil
}

func decodeResponse(m map[string]any) (Response, error) {
	var r Response
	status, err := decodeStatus(m["status"])
	if err != nil {
		return r, err
	}
	r.Status = status
	headers, err := decodeHeaders(m["headers"])
	if err != nil {
		return r, err
	}
	r.Headers = headers
	body, err := decodeBody(m["body"])
	if err != nil {
		return r, err
	}
	r.Body = body
	r.HTTPVersion = asString(m["http_version"])
	return r, nil
}

func decodeStatus(v any) (Status, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return Status{}, &FormatError{"status is not a mapping"}
	}
	codeStr := asString(m["code"])
	code, err := strconv.Atoi(codeStr)
	if err != nil {
		return Status{}, &FormatError{"invalid status code: " + codeStr}
	}
	return Status{Code: code, Message: asString(m["message"])}, nil
}

func decodeBody(v any) (Body, error) {
	if v == nil {
		return Body{}, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return Body{}, &FormatError{"body is not a mapping"}
	}
	return Body{Encoding: asString(m["encoding"]), String: asString(m["string"])}, nil
}

func decodeHeaders(v any) (map[string][]string, error) {
	if v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, &FormatError{"headers is not a mapping"}
	}
	if len(m) == 0 {
		return map[string][]string{}, nil
	}
	out := make(map[string][]string, len(m))
	for k, raw := range m {
		vals, err := decodeStringSeq(raw)
		if err != nil {
			return nil, err
		}
		out[k] = vals
	}
	return out, nil
}

func decodeStringSeq(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	seq, ok := v.([]any)
	if !ok {
		return nil, &FormatError{"header value is not a sequence"}
	}
	out := make([]string, 0, len(seq))
	for _, e := range seq {
		s, ok := e.(string)
		if !ok {
			return nil, &FormatError{"header value entry is not a scalar"}
		}
		out = append(out, s)
	}
	return out, nil
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// --- minimal block-YAML parser ----------------------------------------------

type yline struct {
	indent int
	text   string
}

// splitLines turns raw bytes into non-blank logical lines with their
// indentation, dropping document markers and blank lines.
func splitLines(data []byte) []yline {
	var out []yline
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		t := strings.TrimLeft(line, " ")
		if t == "" || t == "---" {
			continue
		}
		out = append(out, yline{indent: len(line) - len(t), text: t})
	}
	return out
}

type yparser struct {
	lines []yline
	pos   int
}

// parse parses a single node (mapping, sequence, or scalar) beginning at the
// current position, whose content sits at the given indent.
func (p *yparser) parse(indent int) any {
	if p.pos >= len(p.lines) {
		return nil
	}
	line := p.lines[p.pos]
	if isDash(line.text) {
		return p.parseSeq(indent)
	}
	if _, _, ok := splitKey(line.text); ok {
		return p.parseMap(indent)
	}
	p.pos++
	return decodeNode(line.text)
}

func (p *yparser) parseMap(indent int) any {
	m := map[string]any{}
	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		if line.indent != indent || isDash(line.text) {
			break
		}
		key, rest, ok := splitKey(line.text)
		if !ok {
			break
		}
		p.pos++
		if rest != "" {
			m[key] = decodeNode(rest)
			continue
		}
		if p.pos >= len(p.lines) {
			m[key] = nil
			continue
		}
		next := p.lines[p.pos]
		switch {
		case next.indent > indent:
			m[key] = p.parse(next.indent)
		case next.indent == indent && isDash(next.text):
			m[key] = p.parseSeq(indent)
		default:
			m[key] = nil
		}
	}
	return m
}

func (p *yparser) parseSeq(indent int) any {
	var items []any
	for p.pos < len(p.lines) {
		line := p.lines[p.pos]
		if line.indent != indent || !isDash(line.text) {
			break
		}
		inner := strings.TrimPrefix(line.text, "-")
		inner = strings.TrimPrefix(inner, " ")
		var sub []yline
		if inner != "" {
			sub = append(sub, yline{indent: indent + 2, text: inner})
		}
		p.pos++
		for p.pos < len(p.lines) && p.lines[p.pos].indent > indent {
			sub = append(sub, p.lines[p.pos])
			p.pos++
		}
		if len(sub) == 0 {
			items = append(items, nil)
			continue
		}
		sp := &yparser{lines: sub}
		items = append(items, sp.parse(sub[0].indent))
	}
	return items
}

func isDash(text string) bool {
	return text == "-" || strings.HasPrefix(text, "- ")
}

// splitKey splits a "key: value" (or "key:") line into its decoded key and the
// raw value text. ok is false when the line is not a mapping entry (i.e. a
// scalar).
func splitKey(text string) (key, rest string, ok bool) {
	if text[0] == '\'' || text[0] == '"' {
		q := text[0]
		i := 1
		for i < len(text) {
			if text[i] == q {
				if q == '\'' && i+1 < len(text) && text[i+1] == '\'' {
					i += 2
					continue
				}
				break
			}
			i++
		}
		if i >= len(text) {
			return "", "", false
		}
		after := text[i+1:]
		if after == "" || after[0] != ':' {
			return "", "", false
		}
		rest = strings.TrimPrefix(after[1:], " ")
		return decodeScalar(text[:i+1]), rest, true
	}
	for i := 0; i < len(text); i++ {
		if text[i] != ':' {
			continue
		}
		if i == len(text)-1 {
			return text[:i], "", true
		}
		if text[i+1] == ' ' {
			return text[:i], text[i+2:], true
		}
	}
	return "", "", false
}

// decodeNode decodes a raw value token that may be a flow-empty collection or a
// scalar.
func decodeNode(text string) any {
	switch text {
	case "{}":
		return map[string]any{}
	case "[]":
		return []any{}
	default:
		return decodeScalar(text)
	}
}

// decodeScalar decodes a single YAML scalar token (plain, single-quoted, or
// double-quoted) into its Go string value.
func decodeScalar(text string) string {
	if len(text) >= 2 {
		if text[0] == '\'' && text[len(text)-1] == '\'' {
			return strings.ReplaceAll(text[1:len(text)-1], "''", "'")
		}
		if text[0] == '"' && text[len(text)-1] == '"' {
			return unescapeDouble(text[1 : len(text)-1])
		}
	}
	return text
}

func unescapeDouble(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(s) {
			b.WriteByte('\\')
			break
		}
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case '"':
			b.WriteByte('"')
		case '\\':
			b.WriteByte('\\')
		case 'x':
			if i+2 < len(s) {
				if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
					b.WriteByte(byte(v))
					i += 2
					continue
				}
			}
			b.WriteByte('x')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

<p align="center"><img src="https://go-ruby-vcr.github.io/logo.png" alt="go-ruby-vcr/vcr" width="720"></p>

# vcr — go-ruby-vcr

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-vcr.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of the deterministic core of Ruby's
[`vcr`](https://github.com/vcr/vcr) gem** — record HTTP interactions to
*cassettes* and replay them on later runs so tests are deterministic and
offline. It reproduces the cassette store, the record/replay state machine, the
four record modes, the request matchers, and VCR's default YAML cassette format
— **without any Ruby runtime**.

It is the VCR library for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby) (`require "vcr"`),
but a **standalone, reusable** module, a sibling of the other `go-ruby-*`
gem ports.

> **What it is — and isn't.** Everything VCR does *around* the wire is
> deterministic and needs **no interpreter**, so it lives here as pure Go: the
> ordered list of recorded interactions, matching an incoming request against
> them, deciding (per record mode) whether to replay or record, and serializing
> the cassette to VCR's YAML schema byte-for-byte. The **HTTP round-trip, the
> filesystem, and the wall clock are host seams**: [`FS`](vcr.go) supplies
> `ReadFile`/`WriteFile`/`MkdirAll` (`OSFS` in production, an in-memory map in
> tests), `VCR.Now` supplies the `recorded_at` timestamp, and
> `Cassette.Interact` takes a `doer func(Request) (Response, error)` that
> performs the real request only when the cassette must record. **The core opens
> no socket and touches no real disk of its own.** The rbgo binding wires the
> seams to Ruby's `Net::HTTP`, `File`, and `Time`.

## Features

Faithful port of the `vcr` gem's core:

- **Cassette store** — an ordered list of `Interaction{Request, Response,
  RecordedAt}` with a record/replay state machine per `use_cassette` scope.
- **Record modes** — `RecordOnce` (`:once`), `RecordNone` (`:none`),
  `RecordNewEpisodes` (`:new_episodes`), `RecordAll` (`:all`) with VCR's exact
  semantics, including `:once` refusing to record against an existing cassette
  and `:all` re-recording from scratch.
- **Request matching** — `MatchMethod`, `MatchURI`, `MatchBody`, `MatchHeaders`;
  the default matcher set is method + URI, and matchers are composable per
  cassette. Each recorded interaction is replayed once unless
  `AllowPlaybackRepeats` is set.
- **Cassette YAML** — a tiny, dependency-free (de)serializer matching VCR's
  default schema (`http_interactions:` → `request:{method,uri,body,headers}` +
  `response:{status:{code,message},headers,body:{encoding,string}}` +
  `recorded_at`), with byte-faithful round-trip.
- **Unhandled requests** — an `*UnhandledRequestError` (VCR's
  `UnhandledHTTPRequestError`) when a request matches nothing and the mode
  forbids recording.
- **Seams** — `FS` (filesystem), `VCR.Now` (clock), and the `Interact` doer
  (HTTP); every branch is reproducible with an in-memory FS and a fixed clock.

CGO-free, dependency-free (stdlib only), **100% test coverage**, `gofmt` +
`go vet` clean, and green across the six 64-bit Go targets (amd64, arm64,
riscv64, loong64, ppc64le, **s390x** — big-endian) plus `js/wasm` and
`wasip1/wasm`.

## Install

```sh
go get github.com/go-ruby-vcr/vcr
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/go-ruby-vcr/vcr"
)

func main() {
	v := vcr.New() // cassette dir "fixtures/vcr_cassettes", mode :once, os FS

	err := v.UseCassette("example", vcr.CassetteOptions{}, func(c *vcr.Cassette) error {
		req := vcr.Request{Method: "get", URI: "http://example.com/"}

		// Replays a matching recorded interaction, or records a new one by
		// calling the doer (the HTTP seam) and storing its response.
		resp, err := c.Interact(req, func(r vcr.Request) (vcr.Response, error) {
			// The rbgo binding performs the real Net::HTTP request here.
			return vcr.Response{
				Status: vcr.Status{Code: 200, Message: "OK"},
				Body:   vcr.Body{Encoding: "UTF-8", String: "hello"},
			}, nil
		})
		if err != nil {
			return err
		}
		fmt.Println(resp.Body.String)
		return nil
	})
	if err != nil {
		panic(err)
	}
}
```

### Selecting a record mode and matchers

```go
mode := vcr.RecordNewEpisodes
err := v.UseCassette("api", vcr.CassetteOptions{
	RecordMode: &mode,
	Matchers:   []vcr.RequestMatcher{vcr.MatchMethod, vcr.MatchURI, vcr.MatchBody},
}, func(c *vcr.Cassette) error {
	// ...
	return nil
})
```

### Injecting the filesystem and clock (tests / hosts)

```go
v := &vcr.VCR{
	CassetteDir: "cassettes",
	RecordMode:  vcr.RecordOnce,
	Matchers:    vcr.DefaultMatchers(),
	FS: vcr.FS{ // in-memory seam
		ReadFile:  memRead,
		WriteFile: memWrite,
		MkdirAll:  memMkdir,
	},
	Now: func() time.Time { return fixedTime },
}
```

## Value model

| gem                                             | this package                                          |
| ----------------------------------------------- | ----------------------------------------------------- |
| `VCR.configure { \|c\| c.cassette_library_dir = … }` | `&VCR{CassetteDir: …, RecordMode: …, Matchers: …}` |
| `VCR.use_cassette(name, record:) { … }`         | `(*VCR).UseCassette(name, CassetteOptions{…}, fn)`    |
| `:once / :none / :new_episodes / :all`          | `RecordOnce / RecordNone / RecordNewEpisodes / RecordAll` |
| request matchers `[:method, :uri, :body, …]`    | `MatchMethod / MatchURI / MatchBody / MatchHeaders`   |
| the cassette (`http_interactions:` YAML)        | `MarshalCassette` / `ParseCassette` (`[]Interaction`) |
| an `HTTPInteraction` (request/response/time)    | `Interaction{Request, Response, RecordedAt}`          |
| `UnhandledHTTPRequestError`                      | `*UnhandledRequestError`                              |
| the intercepted `Net::HTTP` request             | the `Interact` doer (host seam)                       |
| the cassette file on disk                        | the `FS` seam (`OSFS` in production)                  |

## Tests & coverage

The suite is deterministic: the cassette store is driven through an in-memory
`FS` seam and a fixed clock, so every branch — all four record modes, every
matcher, the unknown-request-in-`:none` error, cassette-not-found, and each
filesystem-error path — is reproducible. **No test opens a socket or touches a
real disk**, so the cross-arch qemu lanes and the Windows lane all hold coverage
at **100%**.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%
```

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-vcr/vcr authors.

> **Private maintenance repository (Issue 35, checked 2026-09-15)**: [lilyzhaun/softvowifi-sipgo](https://github.com/lilyzhaun/softvowifi-sipgo) exists and is PRIVATE. `lilyzhaun` owns the repository and final integration; `newdamm` contributes reviewed maintenance PRs. The write invitation is **PENDINGINVITATION** at this check, not confirmed access; only `newdamm` can accept it. No admin permission was granted.
> Module path remains `github.com/emiago/sipgo`. SoftVoWiFi's verified migration candidate uses `replace github.com/emiago/sipgo v1.4.3 => github.com/lilyzhaun/softvowifi-sipgo v0.0.0-20260915075721-59e490add69c`, pinning commit `59e490add69cf7ab779ee4fe4a5d90e390469806`, not the latest maintenance `main` HEAD. This documentation-only update advances `main` without changing that pin, production code, module files or licenses; the main-repository migration still awaits final acceptance.
> The initial snapshot was imported from SoftVoWiFi commit `8ca6114de2c915473165bf0698a09ced0f9108e3`, path `engine/third_party/sipgo`. This maintenance README replaces, rather than reproduces, the upstream README; [LICENSE](LICENSE), attribution and source metadata below are preserved. PRIVATE is not legal or public-release clearance. This is a private snapshot import, not a GitHub fork-network claim.
> Canonical policy: `engine/third_party/manifest.json` in SoftVoWiFi; handoff: `docs/第三方Go依赖维护与导出.md`. The initial-import text below, including "maintained local copy" and `./third_party/sipgo`, is historical, not current integration guidance. Upstream metadata does not identify the patched tree. Future code changes require review and a separately verified pin update.

# sipgo (maintained local copy)

This directory is a maintained source copy of [emiago/sipgo](https://github.com/emiago/sipgo) for the SoftVoWiFi engine.

- Module path: `github.com/emiago/sipgo`
- Import baseline version: `v1.4.3` (not an unmodified description of this maintained tree)
- Upstream VCS: `https://github.com/emiago/sipgo`
- Exact commit: `769e4bc958376e2363566c8f7042202b410becdc`
- License: BSD-2-Clause (see [LICENSE](LICENSE))
- Original module zip SHA-256: `6fd9c6d280f2b7d2a93de21b73f06f1e35b3064cae6cdd5226a5beec4b8ebe76`

## Retained scope

Kept from upstream v1.4.3:

- Root production Go (`ua.go`, `client.go`, `server.go`, `dialog*.go`)
- `sip/` production Go, including platform-specific files
- `go.mod`, `go.sum`, `LICENSE`
- Selected parser tests and helpers: `sip/parser_test.go`, `sip/parser_stream_test.go`, `sip/parser_fuzz_test.go`, `sip/testdata/fuzz`

Not retained: examples, CI, icons, dialog/server/client integration tests, `siptest/`, `fakes/`, torture corpora, and unrelated transport tests.

Engine wiring uses a version-scoped replace only:

```
replace github.com/emiago/sipgo v1.4.3 => ./third_party/sipgo
```

`engine/go.mod` versions are otherwise unchanged; `go mod tidy` is not part of this copy.

## Local diff purpose

The copy exists to keep stream parsing correct on TCP fragment boundaries without changing the public complete-buffer parser.

1. Stream-only: a non-empty logical header that ends exactly at the current chunk must return `io.ErrUnexpectedEOF` with consumed `0` before `ParseHeader`. The whole logical header, including continuations, stays in the buffer. A terminator blank line is still consumed immediately. `Parse` / `ParseHeaders` complete-buffer semantics are unchanged; there is no `n < len(data)` deletion.
2. Partial-message growth is checked against `MaxMessageLength` before awaiting more data or allocating a body. The budget is overflow-safe and per message, not for a whole TCP batch. On limit, transports stop further reads and release the parser under the existing error contract. Fully consumed oversized messages still recover as upstream did.
3. TCP keepalive CRLF detection only applies at start-line with an empty parser buffer, so a header-section `\r` / `\n` split is not dropped. Non-partial parse errors stop the TCP read loop and close the parser.
4. Opt-in `WithTransportLayerRequireExistingConnection` (default false) makes `ClientRequestConnection` and `serverRequestConnection` fail on UDP cache miss/closed/reuse-key mismatch before `CreateConnection`. Default remains the upstream create-on-miss behavior. `WriteMsg` of a Response still only looks up an existing connection. Existing `WithTransportLayerReadFilter` can drop datagrams before parse without stopping the read loop.

This is not a new framer and does not upgrade other dependencies.

## Source-readiness inventory (2026-09-15)

The locally cached `github.com/emiago/sipgo/@v/v1.4.3.info` records commit
`769e4bc958376e2363566c8f7042202b410becdc` and `refs/tags/v1.4.3`.
The matching local module zip is the comparison source; this is not fresh
remote-history or availability verification.

Five retained production files differ from that zip:

| File | Maintained difference |
|---|---|
| `sip/parser.go` | Stream-aware logical-header boundary; complete-buffer callers pass false |
| `sip/parser_stream.go` | Partial-message length checks, overflow-safe body budget, stream-header call and simplified buffer write |
| `sip/transport_layer.go` | Opt-in existing-connection requirement for client and server request paths |
| `sip/transport_tcp.go` | Keepalive boundary, parser close, stop on non-partial errors, omit raw packet data from parse-error logs |
| `sip/transport_udp.go` | Atomic `CreateConnection` attempt counter used by regression tests |

Four added tests, all under `sip/`: `parser_public_test.go`,
`parser_stream_boundary_test.go`, `transport_require_existing_test.go`, and
`transport_require_existing_server_test.go`. `WithTransportLayerReadFilter`
was already present in the cached baseline, not introduced by these patches.
README replacement is an additional documentation difference, not a sixth
production-code patch. The upstream test suite is only partially retained.

The cached upstream README's Credits section states:

> This project was influenced by [gosip](https://github.com/ghettovoice/gosip), project by @ghetovoice, but started as new project to achieve best/better performance and to improve API.
> This unfortunately required many design changes, therefore this libraries are not compatible.

That is the upstream author's statement, not independent proof that no code
was copied. The upstream README also thanks [pion](https://github.com/pion/webrtc)
for sharing the [SIPgo/WebRTC example](https://github.com/pion/example-webrtc-applications/tree/master/sip-to-webrtc);
examples are outside this retained subset. These source notes do not clear
all provenance or distribution risks. The cached version's BSD-2-Clause
LICENSE retains `Copyright (c) 2022, Emir Aganovic.` and `All rights reserved.`;
do not generalize that license to other versions or dependencies.

Checksums of the original unmodified v1.4.3 files used as the import baseline:

- `LICENSE`: `81e1a2bcb702a0103d3ae3c4fc6c3866d227d9effaaa4b7b801f7ffdaf8cee64`
- `go.mod`: `a6b48c779ce33d7f4805a85295f66d5459504854c716e64174131f791544987c`
- `go.sum`: `bb8ee93953e451b8036dde71e7b93cc1e932d9ec5570b6dcdc32a98ea0730d59`
- `sip/parser.go`: `2d1cbcbc9af77daee730783baca3483179106eb0e70f9fdf283f55f753ae9e81`
- `sip/parser_stream.go`: `e15bcb08bf35701a580ca762d2663142d1d81f879646d150dfe7ef5e791f330e`
- `sip/transport_tcp.go`: `39b4a772da6f73e8b660426b052b6d1883c675584c388ab89f223e49ee081379`
- `sip/transport_layer.go`: `b47a1457ce90c73a83551ebd3123e9a7cd8ea9edf4ecce7afe26a31797c8ec7c`
- `sip/transport_udp.go`: `d67e76fdbde305b10ce2946c3ec1b33be345bb776aa578abe2aa777f61c2908e`

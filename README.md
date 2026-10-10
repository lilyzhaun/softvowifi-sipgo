# softvowifi-sipgo

[![CI](https://github.com/lilyzhaun/softvowifi-sipgo/actions/workflows/ci.yml/badge.svg)](https://github.com/lilyzhaun/softvowifi-sipgo/actions/workflows/ci.yml)

**English** · [简体中文](README.zh-CN.md)

A public maintained snapshot of [emiago/sipgo](https://github.com/emiago/sipgo),
focused on SIP stream parsing, connection reuse and transaction lifecycle behavior.
Imports remain `github.com/emiago/sipgo` and `github.com/emiago/sipgo/sip`.

This is an independent snapshot, not an official upstream release or complete
mirror. It supplies SIP signaling primitives—not a full IMS client, SIM/AKA
provider, IPsec tunnel, RTP codec stack or certified VoWiFi application.

## Maintained behavior

- Stream parsing preserves logical headers and continuations across TCP chunk
  boundaries without changing complete-buffer parsing semantics.
- Partial-message limits are overflow-safe and apply per message rather than to
  an entire TCP batch. Keepalive detection does not discard split header bytes.
- `sip.WithTransportLayerRequireExistingConnection()` opts into rejecting request
  connection-cache misses instead of silently creating a new connection. The
  default create-on-miss behavior is unchanged.
- Terminal transaction errors are published before `Done` closes. Termination
  callbacks can read `Err` without taking the active FSM lock. Explicit client
  termination reports `ErrTransactionCanceled`; server termination remains
  `ErrTransactionTerminated`.

Retained root and `sip/` production sources include client, server, dialog and
transport helpers. Only selected upstream tests and additional focused regressions
are retained; this is **not** the full upstream integration or torture suite.
See [PROVENANCE.md](PROVENANCE.md) for the baseline and patch inventory.

## Requirements

`go.mod` declares Go 1.23.0 and requests toolchain Go 1.24.2. CI uses Go 1.26 on
Linux; other platforms should be checked for the APIs your integration uses.
The parser example below has no network or privilege requirements.

## Use the maintained snapshot

In an existing Go module:

```sh
go mod edit -replace=github.com/emiago/sipgo@v1.4.3=github.com/lilyzhaun/softvowifi-sipgo@v0.0.0-20260916002955-f30e2a8e5daa
go get github.com/emiago/sipgo@v1.4.3
go list -m -json github.com/emiago/sipgo
```

This example pins the reviewed maintained transaction/parser code, not moving
`main`. The replacement is **version-scoped**: if module version selection moves
the original module away from `v1.4.3`, this replacement will not apply to that
other version. Inspect `go list`'s `Replace` entry rather than assuming the fork
is active. Keep original imports; the repository URL is not a new module path.

## Offline parsing example

The message uses synthetic identities and a reserved `.invalid` domain. It does
not listen on a port, send SIP traffic or authenticate a real subscriber.

```go
package main

import (
	"fmt"
	"log"

	"github.com/emiago/sipgo/sip"
)

func main() {
	wire := "OPTIONS sip:service@example.invalid SIP/2.0\r\n" +
		"Via: SIP/2.0/UDP 127.0.0.1:5060;branch=z9hG4bK-docs\r\n" +
		"From: <sip:alice@example.invalid>;tag=docs\r\n" +
		"To: <sip:service@example.invalid>\r\n" +
		"Call-ID: docs@example.invalid\r\n" +
		"CSeq: 1 OPTIONS\r\n" +
		"Max-Forwards: 70\r\n" +
		"Content-Length: 0\r\n\r\n"
	message, err := sip.ParseMessage([]byte(wire))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Parsed %T\n", message)
}
```

## Develop and test

```sh
git clone https://github.com/lilyzhaun/softvowifi-sipgo.git
cd softvowifi-sipgo
go test -p 1 -race -shuffle=on -count=1 -timeout=120s ./...
go vet -p 1 -composites=false ./...
```

Full vet has a recorded legacy warning for unkeyed `sip.HeaderKV` literals in
`client.go`. CI disables the **whole composites analyzer** while running the
other default analyzers; it does not claim a clean full-vet result. Retained
source formatting is reported, not automatically rewritten. Synthetic/loopback
tests do not prove interoperability with an IMS network or a SIP carrier.

## Contribute, report and stay safe

Read [CONTRIBUTING.md](CONTRIBUTING.md), the [code of conduct](CODE_OF_CONDUCT.md)
and [security policy](SECURITY.md). Use the
[issue templates](https://github.com/lilyzhaun/softvowifi-sipgo/issues/new/choose)
for non-security defects and suggestions. Real SIP may contain credentials,
phone numbers, message bodies and authentication material: provide synthetic
reproductions rather than full logs, configuration or packet captures.

## License and credits

The original [BSD-2-Clause LICENSE](LICENSE), including Emir Aganovic's copyright,
is preserved. Credit for the upstream SIP implementation belongs to
[emiago/sipgo](https://github.com/emiago/sipgo). Its documented influences and
example acknowledgments include [gosip](https://github.com/ghettovoice/gosip)
and [Pion](https://github.com/pion/webrtc); these statements are attribution,
not independent proof about every historical code origin.

[PROVENANCE.md](PROVENANCE.md) and the [archived README](docs/README-imported.md)
retain the exact v1.4.3 source record, checksums, patch notes and limitations.
Old private-migration instructions in that archive are not current setup steps.

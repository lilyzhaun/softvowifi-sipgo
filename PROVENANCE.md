# Provenance and maintained scope

## Current repository

`lilyzhaun/softvowifi-sipgo` is public. It is an independently imported maintained
snapshot, not a GitHub fork-network mirror. The original module path is
`github.com/emiago/sipgo`; README refreshes do not change consumer pins or source.

## Recorded upstream baseline

- Project: [emiago/sipgo](https://github.com/emiago/sipgo).
- Version: `v1.4.3`.
- Recorded original commit: `769e4bc958376e2363566c8f7042202b410becdc`.
- Original module archive SHA-256:
  `6fd9c6d280f2b7d2a93de21b73f06f1e35b3064cae6cdd5226a5beec4b8ebe76`.
- License: the unchanged BSD-2-Clause text in [LICENSE](LICENSE), retaining
  `Copyright (c) 2022, Emir Aganovic.` and `All rights reserved.`

The original record was based on the retained module archive and metadata. It is
not a claim that this maintenance task re-audited all upstream history.

## Retained and modified material

Root client/server/dialog sources and `sip/` production sources are retained.
Only selected parser tests, fuzz inputs and additional maintenance regressions
are present, not all upstream examples, test helpers or integration/torture suites.

Maintenance covers stream header boundaries and message-size budgets,
keepalive framing, opt-in existing-connection requirements and terminal-error
publication/callback ordering. It does not describe this tree as pure v1.4.3.

The [archived README](docs/README-imported.md) preserves the full original source
inventory, individual checksums, contributor/third-party credits and dated
terminal-error test notes. Earlier private status and migration instructions are
historical. Existing upstream references to gosip and Pion are retained there;
upstream credit is not independent proof of the absence of copied material.

Root LICENSE, original copyright, production code and module files are unchanged
by this public documentation/GitHub maintenance. New protocol changes still need
separate regressions, review and explicit consumer adoption.

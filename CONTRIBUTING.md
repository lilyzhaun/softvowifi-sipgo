# Contributing

English and 简体中文 contributions are welcome. Read the [README](README.md),
[provenance record](PROVENANCE.md) and [code of conduct](CODE_OF_CONDUCT.md) first.

## Reports

Use the issue templates and identify the exact maintained commit and effective
module replacement. Include Go/OS/architecture, a minimal synthetic SIP message
or loopback test, and expected/actual results. Never submit real credentials,
subscriber identifiers, message bodies, full configuration or raw captures.
Vulnerabilities use the [private security channel](SECURITY.md).

## Changes

- Branch from current `main`; keep one concern per PR.
- Add a failing regression before changing parser/transport/transaction behavior.
  Do not weaken existing tests to obtain green results.
- Preserve complete-buffer semantics when fixing stream-only behavior. Retain
  transaction errors, callback ordering, connection ownership and cleanup checks.
- Preserve the original module path, dependencies unless explicitly reviewed,
  copyright, license and source attribution.
- Format changed Go files and run:

  ```sh
  go test -p 1 -race -shuffle=on -count=1 -timeout=120s ./...
  go vet -p 1 -composites=false ./...
  git diff --check
  ```

The composites analyzer is disabled because of the recorded legacy HeaderKV
warnings; other default vet analyzers run. Full-vet failures and retained-source
formatting differences must be disclosed, not called passing checks.

Explain head/base, compatibility, test commands, results and skipped paths in the
PR. The retained suite is not the full upstream suite and loopback SIP is not a
real IMS/carrier test. Use Conventional Commits and your existing legitimate Git
identity; do not add tool banners, fabricated authors or generated attribution
trailers. Do not rewrite shared history or force-push over others' work.

Documentation-only commits do not require a downstream pin upgrade. Protocol or
dependency changes need a separately reviewed, fixed revision and consumer tests.

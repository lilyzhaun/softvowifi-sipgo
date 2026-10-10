# Security policy

Security maintenance targets current `main` and explicitly reviewed fixes; no
published LTS or response-time SLA is promised. Consumers of old pinned snapshots
must separately review and adopt fixes. This SIP library is not a certification
of a complete IMS/VoWiFi product or every upstream transport/integration path.

## Private reporting

Use GitHub's **Security → Report a vulnerability**:

https://github.com/lilyzhaun/softvowifi-sipgo/security/advisories/new

Do not publish an unpatched vulnerability in an issue or PR. If this entry is
temporarily unavailable, use GitHub's support/reporting tools rather than a
public disclosure containing sensitive material. Coordinate disclosure timing
with the maintainer before publishing exploit details.

Include an affected commit, impact, privilege/transport assumptions and a minimal
synthetic reproduction. Do not upload real SIP Authorization headers, passwords,
tokens, subscriber identities, message bodies, full config, private keys or raw
packet captures. Use reserved domains and synthetic test values.

Keep diagnostic logs metadata-only when possible. Parser/transport failures must
not become a reason to dump complete messages or to bypass authentication and
connection ownership in a consuming application. Source and test coverage limits
are documented in [PROVENANCE.md](PROVENANCE.md).

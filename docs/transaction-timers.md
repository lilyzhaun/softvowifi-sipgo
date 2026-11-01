# Instance transaction timers

## Maintenance scope

Owner: lilyzhaun. Branch: `feat/instance-transaction-timers-20261102`.
Base: `50b4d0c278723d9fd7075617359f2bcd2ce5cc43`.

This change is limited to transaction timer capture, existing client/server
state-machine consumers, synthetic regression tests and this API's documentation.
It preserves module/dependency/license identity and existing transport ownership.

Four loopback regressions demonstrated that non-INVITE transactions selected
A/B/D instead of E/F/K and stopped retransmitting after a provisional response.
The per-layer immutable timer input lets independent user agents use their own
clocks without changing process-wide variables.

References are protocol semantics only:
- [RFC3261 sections 17.1/17.2](https://www.rfc-editor.org/rfc/rfc3261#section-17)
- [RFC6026 accepted-state L/M timers](https://www.rfc-editor.org/rfc/rfc6026)

Proxy Timer C, registration expiry/refresh/backoff, socket-write budgets and
INVITE-success retransmission at the transaction-user layer have separate owners.
They must not be represented by an unused transaction-layer field.

## API and capture boundary

`sip.TransactionTimers` supplies the complete T1/T2/T4/B/D/F/H/J policy.
`Validate` rejects non-positive values, T2 below T1 and an overflowing 64*T1.
The caller remains responsible for protocol/profile constraints, including the
RFC3261 unreliable INVITE Timer D minimum. Tests use explicit synthetic clocks;
they are not proposed production profiles.

```go
policy := sip.TransactionTimers{
    T1: 500 * time.Millisecond, T2: 4 * time.Second, T4: 5 * time.Second,
    B: 32 * time.Second, D: 32 * time.Second, F: 32 * time.Second,
    H: 32 * time.Second, J: 32 * time.Second,
}
option, err := sip.WithTransactionLayerTimers(policy)
if err != nil {
    return err
}
ua, err := sipgo.NewUA(sipgo.WithUserAgentTransactionLayerOptions(option))
```

The option validates once and captures its value immediately. A layer owns an
immutable copy; both client and server transactions copy that layer's clocks.
Changing the caller's struct or another layer cannot alter an existing exchange.
The option may be reused. The existing 200ms automatic Trying delay is retained
for explicitly configured layers.

Callers owning a transport/registry can use `NewServerTxWithTimers` directly.
It validates and captures the same value before construction and returns an
error for invalid input; `Init`, transport ownership and callbacks are unchanged.
This additive entry is maintained by lilyzhaun on
`feat/direct-server-transaction-timers-20261102`, based on `951da7d9cac6`.

Without this option, a layer captures **all legacy timer variables** once at
layer construction. Direct `NewClientTx`/`NewServerTx` callers capture them at
transaction construction. `SetTimers` and direct variable writes are still
startup-only, not concurrent runtime configuration APIs. Earlier code re-read
globals in subsequent transactions/state transitions; that behavior is replaced
by capture and is an intentional compatibility change.

## Actual consumers

| Consumer | Captured clocks |
|---|---|
| INVITE client Calling | A from T1; B deadline |
| INVITE non-2xx client Completed | D retention |
| non-INVITE client Trying/Proceeding | E from T1, exponential cap at T2 in Trying, T2 after firing in Proceeding; F deadline |
| non-INVITE final response | K from T4 |
| INVITE server non-2xx response/ACK | G from T1 capped at T2; H ACK deadline; I from T4 |
| non-INVITE server final response | J retention |
| INVITE Accepted states | L/M from 64*T1 |

Reliable transports suppress A/E/G and use zero D/I/J/K retention. B/F/H and
L/M still apply to their respective states. Provisional non-INVITE responses
keep E/F alive. B/F/H timeout causes are published before `Done`; completed
retention expiry remains ordinary termination. Existing cancellation, error and
connection-ownership contracts remain in force.

Verification covers real loopback UDP request/reply/retransmission and retirement,
independent instances, legacy capture, TCP retransmission suppression and B/F
deadlines, plus the retained terminal-ordering suite. These are library tests,
not an IMS registration, device-data-plane or carrier interoperability result.

## Validation record

The initial focused run preserved four pre-existing behavioral failures as
regressions and passed their corrected consumers. It also exposed a fifth issue:
Timer H expired but published ordinary termination instead of its timeout cause.
The fix uses the existing error-bearing FSM entry; the affected server, T2 and
TCP cases subsequently passed together.

The frozen retained suite passed `go test -p 1 -race -shuffle=on -count=1
-timeout=120s ./...`: 52 parent and 704 child tests passed, with the existing
`TestSIPTortuous` skip retained. `go vet -p 1 -composites=false ./...` passed;
this is the previously documented composites exception, not a clean full-vet
claim. No production timer default, dependency, module path or license changed.

package sip

import (
	"fmt"
	"math"
	"time"
)

// TransactionTimers is a complete per-layer duration policy. A/E/G use T1,
// I/K use T4, and RFC6026 L/M use 64*T1. B/D/F/H/J remain independent.
// Reliable transports suppress A/E/G and use zero D/I/J/K as required by
// their state machines. Durations must meet the caller's protocol profile;
// Validate checks positivity, retransmission bounds and duration overflow.
type TransactionTimers struct {
	T1, T2, T4    time.Duration
	B, D, F, H, J time.Duration
}

func (p TransactionTimers) Validate() error {
	for _, field := range []struct {
		name  string
		value time.Duration
	}{
		{"T1", p.T1}, {"T2", p.T2}, {"T4", p.T4}, {"B", p.B}, {"D", p.D}, {"F", p.F}, {"H", p.H}, {"J", p.J},
	} {
		if field.value <= 0 {
			return fmt.Errorf("transaction timer %s must be positive", field.name)
		}
	}
	if p.T2 < p.T1 {
		return fmt.Errorf("transaction timer T2 must be at least T1")
	}
	if p.T1 > time.Duration(math.MaxInt64/64) {
		return fmt.Errorf("transaction timer 64*T1 overflows")
	}
	return nil
}

// WithTransactionLayerTimers validates and captures the supplied value before
// any layer is created. The returned option can be reused across independent
// layers. Existing transactions never consult mutable caller/global timers.
func WithTransactionLayerTimers(p TransactionTimers) (TransactionLayerOption, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	captured := p.snapshot()
	return func(layer *TransactionLayer) {
		value := captured
		layer.timers = &value
	}, nil
}

func (p TransactionTimers) snapshot() transactionTimerSnapshot {
	return transactionTimerSnapshot{T2: p.T2, A: p.T1, B: p.B, D: p.D, E: p.T1, F: p.F,
		G: p.T1, H: p.H, I: p.T4, J: p.J, K: p.T4, L: 64 * p.T1, M: 64 * p.T1, Trying: 200 * time.Millisecond}
}

type transactionTimerSnapshot struct {
	T2, A, B, D, E, F, G, H, I, J, K, L, M, Trying time.Duration
}

// Legacy variables are startup-only. Direct constructors capture them once;
// a layer without an explicit policy captures them when the layer is created.
func captureLegacyTransactionTimers() transactionTimerSnapshot {
	return transactionTimerSnapshot{T2: T2, A: Timer_A, B: Timer_B, D: Timer_D, E: Timer_E, F: Timer_F,
		G: Timer_G, H: Timer_H, I: Timer_I, J: Timer_J, K: Timer_K, L: Timer_L, M: Timer_M, Trying: Timer_1xx}
}

func doubledTimerInterval(value, limit time.Duration) time.Duration {
	if value >= limit-value {
		return limit
	}
	return 2 * value
}

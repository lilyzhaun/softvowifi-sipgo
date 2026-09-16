package sip

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
)

func terminalPair(t *testing.T) (*ClientTx, *ServerTx) {
	t.Helper()
	socket, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := socket.Close(); err != nil {
			t.Error(err)
		}
	})
	req := testUDPServerRequest("127.0.0.1:5061", "127.0.0.1", 5060)
	conn := &UDPConnection{PacketConn: socket, Listener: true, refcount: 2}
	client := NewClientTx("terminal-client", req, conn, slog.Default())
	server := NewServerTx("terminal-server", req, conn, slog.Default())
	t.Cleanup(client.Terminate)
	t.Cleanup(server.Terminate)
	return client, server
}

func TestTerminalErrorPublishedBeforeDone(t *testing.T) {
	client, server := terminalPair(t)
	for _, test := range []struct {
		name string
		tx   Transaction
		want error
	}{
		{"client", client, ErrTransactionCanceled},
		{"server", server, ErrTransactionTerminated},
	} {
		t.Run(test.name, func(t *testing.T) {
			entered := make(chan error, 1)
			release := make(chan struct{})
			finished := make(chan struct{})
			var calls atomic.Int32
			if !test.tx.OnTerminate(func(_ string, err error) {
				calls.Add(1)
				entered <- err
				<-release
			}) {
				t.Fatal("callback registration failed")
			}
			go func() { test.tx.Terminate(); close(finished) }()
			<-test.tx.Done()
			callbackErr := <-entered
			published := test.tx.Err()
			var workers sync.WaitGroup
			for range 8 {
				workers.Add(1)
				go func() { defer workers.Done(); test.tx.Terminate() }()
			}
			workers.Wait()
			close(release)
			<-finished
			if !errors.Is(published, test.want) {
				t.Errorf("Done observed before terminal error: got %v, want %v", published, test.want)
			}
			if !errors.Is(callbackErr, test.want) || !errors.Is(test.tx.Err(), test.want) || calls.Load() != 1 {
				t.Errorf("unstable terminal result: callback=%v final=%v calls=%d", callbackErr, test.tx.Err(), calls.Load())
			}
		})
	}
	select {
	case _, open := <-client.Responses():
		t.Fatalf("response channel changed on termination: open=%v", open)
	default:
	}
}

func TestTerminalErrorPreservesFSMCause(t *testing.T) {
	for _, cause := range []error{nil, fmt.Errorf("synthetic timeout: %w", ErrTransactionTimeout), fmt.Errorf("synthetic transport: %w", ErrTransactionTransport)} {
		t.Run(fmt.Sprint(cause), func(t *testing.T) {
			client, server := terminalPair(t)
			for _, test := range []struct {
				name   string
				tx     Transaction
				base   *baseTx
				remove func() fsmInput
			}{
				{"client", client, &client.baseTx, client.actDelete},
				{"server", server, &server.baseTx, server.actDelete},
			} {
				t.Run(test.name, func(t *testing.T) {
					want := cause
					if want == nil {
						want = ErrTransactionTerminated
					}
					observed := make(chan error, 1)
					test.tx.OnTerminate(func(_ string, err error) {
						if test.tx.Err() != err {
							t.Error("callback error differs from published error")
						}
						observed <- err
					})
					test.base.fsmMu.Lock()
					test.base.fsmErr = cause
					test.remove()
					test.base.fsmMu.Unlock()
					test.tx.Terminate()
					if !errors.Is(<-observed, want) || !errors.Is(test.tx.Err(), want) {
						t.Fatalf("termination replaced FSM cause: %v, want %v", test.tx.Err(), want)
					}
				})
			}
		})
	}
}

func TestTerminalUnblocksFSMPassUpWithoutTakingFSMLock(t *testing.T) {
	client, _ := terminalPair(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	terminated := make(chan struct{})
	client.OnTerminate(func(string, error) {
		if !errors.Is(client.Err(), ErrTransactionCanceled) {
			t.Error("callback cannot read the published cancellation")
		}
	})
	go func() {
		client.fsmMu.Lock()
		client.fsmResp = NewResponse(200, "OK")
		close(entered)
		client.fsmPassUp()
		<-release
		client.fsmMu.Unlock()
		close(finished)
	}()
	<-entered
	go func() { client.Terminate(); close(terminated) }()
	<-terminated
	close(release)
	<-finished
	if !errors.Is(client.Err(), ErrTransactionCanceled) {
		t.Fatal("blocked delivery lost cancellation")
	}
}

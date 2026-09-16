package sip_test

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

type terminalRequester struct {
	layer   *sip.TransactionLayer
	started chan *sip.ClientTx
	barrier <-chan struct{}
	calls   *atomic.Int32
}

func (requester terminalRequester) Request(ctx context.Context, req *sip.Request) (sip.ClientTransaction, error) {
	tx, err := requester.layer.Request(ctx, req)
	if err != nil {
		return nil, err
	}
	if requester.barrier != nil {
		tx.OnTerminate(func(string, error) {
			requester.calls.Add(1)
			<-requester.barrier
		})
	}
	requester.started <- tx
	return tx, nil
}

func TestTerminalUDPClientContract(t *testing.T) {
	for _, scenario := range []string{"shutdown", "terminate", "cancel", "success"} {
		t.Run(scenario, func(t *testing.T) {
			peer, err := net.ListenPacket("udp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer peer.Close()
			ua, err := sipgo.NewUA()
			if err != nil {
				t.Fatal(err)
			}
			defer ua.Close()
			client, err := sipgo.NewClient(ua, sipgo.WithClientHostname("127.0.0.1"))
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			started := make(chan *sip.ClientTx, 1)
			var barrier chan struct{}
			var calls atomic.Int32
			if scenario == "shutdown" || scenario == "terminate" {
				barrier = make(chan struct{})
			}
			var workers sync.WaitGroup
			defer func() {
				cancel()
				if barrier != nil {
					close(barrier)
				}
				workers.Wait()
				if barrier != nil && calls.Load() != 1 {
					t.Errorf("termination callback count=%d, want 1", calls.Load())
				}
			}()
			client.TxRequester = terminalRequester{ua.TransactionLayer(), started, barrier, &calls}
			req := sip.NewRequest(sip.MESSAGE, sip.Uri{Scheme: "sip", User: "synthetic", Host: "127.0.0.1"})
			req.SetTransport("UDP")
			req.SetDestination(peer.LocalAddr().String())
			type result struct {
				response *sip.Response
				err      error
			}
			completed := make(chan result, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				response, err := client.Do(ctx, req)
				completed <- result{response, err}
			}()
			if err := peer.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Fatal(err)
			}
			buffer := make([]byte, 4096)
			size, address, err := peer.ReadFrom(buffer)
			if err != nil {
				t.Fatal(err)
			}
			message, err := sip.ParseMessage(buffer[:size])
			if err != nil {
				t.Fatal(err)
			}
			wireRequest, ok := message.(*sip.Request)
			if !ok {
				t.Fatal("peer did not receive a request")
			}
			tx := <-started
			switch scenario {
			case "shutdown", "terminate":
				workers.Add(1)
				go func() {
					defer workers.Done()
					if scenario == "shutdown" {
						if err := ua.Close(); err != nil {
							t.Error(err)
						}
					} else {
						tx.Terminate()
					}
				}()
				<-tx.Done()
				workers.Add(1)
				go func() { defer workers.Done(); tx.Terminate() }()
			case "cancel":
				cancel()
			case "success":
				response := sip.NewResponseFromRequest(wireRequest, 200, "OK", nil)
				if _, err := peer.WriteTo([]byte(response.String()), address); err != nil {
					t.Fatal(err)
				}
			}
			outcome := <-completed
			switch scenario {
			case "shutdown", "terminate":
				if outcome.response != nil || !errors.Is(outcome.err, sip.ErrTransactionCanceled) {
					t.Errorf("terminal contract: response=%v err=%v", outcome.response, outcome.err)
				}
				if !errors.Is(tx.Err(), sip.ErrTransactionCanceled) {
					t.Error("Done has no cancellation error")
				}
			case "cancel":
				if outcome.response != nil || !errors.Is(outcome.err, context.Canceled) {
					t.Errorf("context contract: response=%v err=%v", outcome.response, outcome.err)
				}
			case "success":
				if outcome.response == nil || outcome.err != nil || outcome.response.StatusCode != 200 {
					t.Errorf("200 contract: response=%v err=%v", outcome.response, outcome.err)
				}
			}
		})
	}
}

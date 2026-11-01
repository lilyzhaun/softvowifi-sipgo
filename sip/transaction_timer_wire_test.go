package sip_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

// Explicit synthetic clocks distinguish E/F/K from A/B/D without changing
// production defaults. Tests using legacy globals deliberately run serially.
func scopedNonInviteTimerValues(t *testing.T) {
	t.Helper()
	a, b, d, e, f, k := sip.Timer_A, sip.Timer_B, sip.Timer_D, sip.Timer_E, sip.Timer_F, sip.Timer_K
	sip.Timer_A, sip.Timer_B, sip.Timer_D = time.Hour, time.Hour, time.Hour
	sip.Timer_E, sip.Timer_F, sip.Timer_K = 100*time.Millisecond, time.Hour, 100*time.Millisecond
	t.Cleanup(func() {
		sip.Timer_A, sip.Timer_B, sip.Timer_D, sip.Timer_E, sip.Timer_F, sip.Timer_K = a, b, d, e, f, k
	})
}

func timerWireClient(t *testing.T, options ...sip.TransactionLayerOption) (*net.UDPConn, *sipgo.Client) {
	t.Helper()
	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { peer.Close() })
	ua, err := sipgo.NewUA(sipgo.WithUserAgentTransactionLayerOptions(options...))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ua.Close() })
	client, err := sipgo.NewClient(ua, sipgo.WithClientHostname("127.0.0.1"))
	if err != nil {
		t.Fatal(err)
	}
	return peer, client
}

func startTimerWireTransaction(t *testing.T, peer *net.UDPConn, client *sipgo.Client) (sip.ClientTransaction, *sip.Request, *net.UDPAddr) {
	return startMethodTimerTransaction(t, peer, client, sip.MESSAGE)
}

func startMethodTimerTransaction(t *testing.T, peer *net.UDPConn, client *sipgo.Client, method sip.RequestMethod) (sip.ClientTransaction, *sip.Request, *net.UDPAddr) {
	t.Helper()
	req := sip.NewRequest(method, sip.Uri{Scheme: "sip", User: "synthetic", Host: "127.0.0.1"})
	req.SetTransport("UDP")
	req.SetDestination(peer.LocalAddr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	tx, err := client.TransactionRequest(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Terminate)
	peer.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 4096)
	n, address, err := peer.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	message, err := sip.ParseMessage(buf[:n])
	if err != nil {
		t.Fatal(err)
	}
	wire, ok := message.(*sip.Request)
	if !ok || wire.Method != method {
		t.Fatal("peer did not receive the actual request")
	}
	return tx, wire, address
}

func TestNonInviteWireRetransmissionUsesE(t *testing.T) {
	scopedNonInviteTimerValues(t)
	peer, client := timerWireClient(t)
	_, first, _ := startTimerWireTransaction(t, peer, client)
	peer.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
	buf := make([]byte, 4096)
	n, _, err := peer.ReadFromUDP(buf)
	if err != nil {
		t.Fatal("Timer E did not retransmit the actual non-INVITE request")
	}
	if string(buf[:n]) != first.String() {
		t.Fatal("non-INVITE retransmission changed the original transaction")
	}
}

func TestNonInviteWireTimeoutUsesF(t *testing.T) {
	scopedNonInviteTimerValues(t)
	sip.Timer_E, sip.Timer_F = time.Hour, 100*time.Millisecond
	peer, client := timerWireClient(t)
	tx, _, _ := startTimerWireTransaction(t, peer, client)
	select {
	case <-tx.Done():
		if !errors.Is(tx.Err(), sip.ErrTransactionTimeout) {
			t.Fatal("Timer F lost its terminal timeout error")
		}
	case <-time.After(600 * time.Millisecond):
		t.Fatal("non-INVITE remained alive past Timer F")
	}
}

func TestNonInviteFinalResponseRetentionUsesK(t *testing.T) {
	scopedNonInviteTimerValues(t)
	sip.Timer_E = time.Hour
	peer, client := timerWireClient(t)
	tx, request, address := startTimerWireTransaction(t, peer, client)
	response := sip.NewResponseFromRequest(request, 200, "OK", nil)
	if _, err := peer.WriteToUDP([]byte(response.String()), address); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-tx.Responses():
		if got.StatusCode != 200 {
			t.Fatal("wrong final response")
		}
	case <-time.After(time.Second):
		t.Fatal("final response did not reach its real transaction")
	}
	// Client.Do terminates immediately and would conceal the retention timer.
	select {
	case <-tx.Done():
	case <-time.After(600 * time.Millisecond):
		t.Fatal("non-INVITE retained the transaction past Timer K")
	}
}

func TestNonInviteProvisionalKeepsRetransmission(t *testing.T) {
	scopedNonInviteTimerValues(t)
	sip.Timer_A, sip.Timer_E = 200*time.Millisecond, 200*time.Millisecond
	peer, client := timerWireClient(t)
	started := time.Now()
	tx, request, address := startTimerWireTransaction(t, peer, client)
	response := sip.NewResponseFromRequest(request, 180, "Ringing", nil)
	if _, err := peer.WriteToUDP([]byte(response.String()), address); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-tx.Responses():
		if got.StatusCode != 180 {
			t.Fatal("wrong provisional response")
		}
	case <-time.After(time.Second):
		t.Fatal("provisional response did not reach its real transaction")
	}
	if time.Since(started) >= 200*time.Millisecond {
		t.Fatal("precondition: provisional response was not observed before the initial retransmission")
	}
	peer.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
	buf := make([]byte, 4096)
	n, _, err := peer.ReadFromUDP(buf)
	if err != nil {
		t.Fatal("non-INVITE provisional response stopped Timer E instead of continuing retransmission")
	}
	if string(buf[:n]) != request.String() {
		t.Fatal("proceeding retransmission changed the original request")
	}
}

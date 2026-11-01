package sip_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

func syntheticTimers() sip.TransactionTimers {
	return sip.TransactionTimers{T1: time.Hour, T2: time.Hour, T4: 100 * time.Millisecond,
		B: time.Hour, D: 100 * time.Millisecond, F: time.Hour, H: time.Hour, J: 150 * time.Millisecond}
}

func checkedTimerOption(t *testing.T, policy sip.TransactionTimers) sip.TransactionLayerOption {
	t.Helper()
	option, err := sip.WithTransactionLayerTimers(policy)
	if err != nil {
		t.Fatal(err)
	}
	return option
}

func TestTransactionTimerPolicyRejectsInvalidInput(t *testing.T) {
	for _, change := range []func(*sip.TransactionTimers){
		func(p *sip.TransactionTimers) { p.T1 = 0 },
		func(p *sip.TransactionTimers) { p.T1 = time.Duration(math.MaxInt64/64 + 1) },
		func(p *sip.TransactionTimers) { p.T2 = p.T1 - 1 },
		func(p *sip.TransactionTimers) { p.T4 = -1 },
		func(p *sip.TransactionTimers) { p.B = 0 },
		func(p *sip.TransactionTimers) { p.D = 0 },
		func(p *sip.TransactionTimers) { p.F = 0 },
		func(p *sip.TransactionTimers) { p.H = 0 },
		func(p *sip.TransactionTimers) { p.J = 0 },
	} {
		p := syntheticTimers()
		change(&p)
		if p.Validate() == nil {
			t.Fatal("invalid duration policy passed validation")
		}
		if option, err := sip.WithTransactionLayerTimers(p); err == nil || option != nil {
			t.Fatal("invalid policy produced a usable option")
		}
	}
}

func TestTransactionTimerLayersCaptureCallerValuesAndRemainIsolated(t *testing.T) {
	scopedNonInviteTimerValues(t)
	p := syntheticTimers()
	p.F = 150 * time.Millisecond
	option := checkedTimerOption(t, p)
	p.F = time.Hour // Mutating the caller's value cannot change its earlier option.
	peerA, clientA := timerWireClient(t, option)
	peerB, clientB := timerWireClient(t, checkedTimerOption(t, p))
	a, _, _ := startTimerWireTransaction(t, peerA, clientA)
	b, _, _ := startTimerWireTransaction(t, peerB, clientB)
	// Explicitly configured layers never re-read these legacy variables.
	sip.Timer_F = time.Nanosecond
	select {
	case <-a.Done():
		if !errors.Is(a.Err(), sip.ErrTransactionTimeout) {
			t.Fatal("captured F lost terminal timeout")
		}
	case <-time.After(700 * time.Millisecond):
		t.Fatal("short instance did not consume its captured F")
	}
	select {
	case <-b.Done():
		t.Fatal("one instance changed another instance's timeout")
	default:
	}
}

func TestLegacyLayerCapturesTimersBeforeLaterTransactions(t *testing.T) {
	scopedNonInviteTimerValues(t)
	peer, client := timerWireClient(t)
	sip.Timer_E, sip.Timer_F = time.Hour, time.Nanosecond
	tx, first, _ := startTimerWireTransaction(t, peer, client)
	peer.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
	buf := make([]byte, 4096)
	n, _, err := peer.ReadFromUDP(buf)
	if err != nil || string(buf[:n]) != first.String() {
		t.Fatal("existing layer re-read changed global E/F when creating a transaction")
	}
	select {
	case <-tx.Done():
		t.Fatal("global change terminated the existing layer's transaction")
	default:
	}
}

func TestCapturedClientTimersReachActualWireTerminals(t *testing.T) {
	for _, name := range []string{"B", "D", "K", "M"} {
		t.Run(name, func(t *testing.T) {
			p := syntheticTimers()
			method, code := sip.INVITE, 486
			switch name {
			case "B":
				p.B, code = 100*time.Millisecond, 0
			case "K":
				method, code = sip.MESSAGE, 200
			case "M":
				p.T1, p.T2, code = 2*time.Millisecond, 20*time.Millisecond, 200
			}
			peer, client := timerWireClient(t, checkedTimerOption(t, p))
			tx, request, address := startMethodTimerTransaction(t, peer, client, method)
			if code != 0 {
				response := sip.NewResponseFromRequest(request, code, "Synthetic", nil)
				if _, err := peer.WriteToUDP([]byte(response.String()), address); err != nil {
					t.Fatal(err)
				}
				select {
				case <-tx.Responses():
				case <-time.After(time.Second):
					t.Fatal("actual final response missing")
				}
			}
			select {
			case <-tx.Done():
				if name == "B" && !errors.Is(tx.Err(), sip.ErrTransactionTimeout) {
					t.Fatal("B lost timeout cause")
				}
			case <-time.After(700 * time.Millisecond):
				t.Fatalf("captured Timer %s did not retire its wire transaction", name)
			}
		})
	}
}

type timerServerRequest struct {
	request *sip.Request
	tx      *sip.ServerTx
}

func timerWireServer(t *testing.T, policy sip.TransactionTimers, method sip.RequestMethod, code int) (*net.UDPConn, *net.UDPAddr, timerServerRequest) {
	t.Helper()
	ua, err := sipgo.NewUA(sipgo.WithUserAgentTransactionLayerOptions(checkedTimerOption(t, policy)))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		ua.Close()
		t.Fatal(err)
	}
	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		listener.Close()
		ua.Close()
		t.Fatal(err)
	}
	requests := make(chan timerServerRequest, 1)
	errors := make(chan error, 1)
	ua.TransactionLayer().OnRequest(func(req *sip.Request, tx *sip.ServerTx) {
		requests <- timerServerRequest{req, tx}
		errors <- tx.Respond(sip.NewResponseFromRequest(req, code, "Synthetic", nil))
	})
	done := make(chan error, 1)
	go func() { done <- ua.TransportLayer().ServeUDP(listener) }()
	t.Cleanup(func() {
		listener.Close()
		peer.Close()
		ua.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("UDP server did not stop")
		}
	})
	address := listener.LocalAddr().(*net.UDPAddr)
	wire := fmt.Sprintf("%s sip:service@127.0.0.1 SIP/2.0\r\nVia: SIP/2.0/UDP %s;branch=%s\r\nFrom: <sip:sender@example.invalid>;tag=synthetic\r\nTo: <sip:service@example.invalid>\r\nCall-ID: timer@example.invalid\r\nCSeq: 1 %s\r\nContent-Length: 0\r\n\r\n", method, peer.LocalAddr(), sip.GenerateBranchN(16), method)
	if _, err := peer.WriteToUDP([]byte(wire), address); err != nil {
		t.Fatal(err)
	}
	peer.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 4096)
	if _, _, err := peer.ReadFromUDP(buf); err != nil {
		t.Fatal("server did not send its first actual response")
	}
	var received timerServerRequest
	select {
	case received = <-requests:
	case <-time.After(time.Second):
		t.Fatal("actual server transaction missing")
	}
	select {
	case err := <-errors:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("server response did not complete")
	}
	return peer, address, received
}

func TestCapturedServerTimersReachActualWireTerminals(t *testing.T) {
	for _, name := range []string{"G-H", "I", "J", "L"} {
		t.Run(name, func(t *testing.T) {
			p := syntheticTimers()
			method, code := sip.INVITE, 486
			switch name {
			case "G-H":
				p.T1, p.T2, p.H = 50*time.Millisecond, 80*time.Millisecond, 250*time.Millisecond
			case "J":
				method, code = sip.MESSAGE, 200
			case "L":
				p.T1, p.T2, code = 2*time.Millisecond, 20*time.Millisecond, 200
			}
			peer, address, received := timerWireServer(t, p, method, code)
			if name == "G-H" {
				peer.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
				buf := make([]byte, 4096)
				n, _, err := peer.ReadFromUDP(buf)
				if err != nil {
					t.Fatal("captured G did not retransmit the response")
				}
				message, err := sip.ParseMessage(buf[:n])
				if err != nil {
					t.Fatal(err)
				}
				response, ok := message.(*sip.Response)
				if !ok || response.StatusCode != 486 {
					t.Fatal("G retransmitted the wrong response")
				}
			}
			if name == "I" {
				ack := received.request.Clone()
				ack.Method = sip.ACK
				ack.CSeq().MethodName = sip.ACK
				if _, err := peer.WriteToUDP([]byte(ack.String()), address); err != nil {
					t.Fatal(err)
				}
				select {
				case <-received.tx.Acks():
				case <-time.After(time.Second):
					t.Fatal("server did not consume actual ACK")
				}
			}
			select {
			case <-received.tx.Done():
				if name == "G-H" && !errors.Is(received.tx.Err(), sip.ErrTransactionTimeout) {
					t.Fatal("H lost timeout cause")
				}
			case <-time.After(700 * time.Millisecond):
				t.Fatalf("captured Timer %s did not retire actual server transaction", name)
			}
		})
	}
}

func TestNonInviteProceedingRetransmitsAtCapturedT2(t *testing.T) {
	p := syntheticTimers()
	p.T1, p.T2 = 100*time.Millisecond, 500*time.Millisecond
	peer, client := timerWireClient(t, checkedTimerOption(t, p))
	tx, request, address := startTimerWireTransaction(t, peer, client)
	response := sip.NewResponseFromRequest(request, 180, "Ringing", nil)
	if _, err := peer.WriteToUDP([]byte(response.String()), address); err != nil {
		t.Fatal(err)
	}
	select {
	case <-tx.Responses():
	case <-time.After(time.Second):
		t.Fatal("provisional response missing")
	}
	peer.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 4096)
	if _, _, err := peer.ReadFromUDP(buf); err != nil {
		t.Fatal("first E retransmission missing")
	}
	peer.SetReadDeadline(time.Now().Add(350 * time.Millisecond))
	if _, _, err := peer.ReadFromUDP(buf); err == nil {
		t.Fatal("Proceeding doubled E instead of waiting captured T2")
	} else if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
		t.Fatal(err)
	}
	peer.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, _, err := peer.ReadFromUDP(buf)
	if err != nil || string(buf[:n]) != request.String() {
		t.Fatal("Proceeding did not retransmit original request at T2")
	}
}

func TestReliableClientSuppressesRetransmitAndRetentionButKeepsTimeout(t *testing.T) {
	for _, scenario := range []struct {
		method sip.RequestMethod
		final  bool
	}{{sip.MESSAGE, true}, {sip.INVITE, true}, {sip.MESSAGE, false}, {sip.INVITE, false}} {
		t.Run(fmt.Sprintf("%s-final-%t", scenario.method, scenario.final), func(t *testing.T) {
			method := scenario.method
			listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { listener.Close() })
			p := syntheticTimers()
			p.T1, p.T2, p.T4, p.D = 30*time.Millisecond, 50*time.Millisecond, time.Hour, time.Hour
			if !scenario.final {
				p.B, p.F = 100*time.Millisecond, 100*time.Millisecond
			}
			ua, err := sipgo.NewUA(sipgo.WithUserAgentTransactionLayerOptions(checkedTimerOption(t, p)))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { ua.Close() })
			client, err := sipgo.NewClient(ua, sipgo.WithClientHostname("127.0.0.1"))
			if err != nil {
				t.Fatal(err)
			}
			req := sip.NewRequest(method, sip.Uri{Scheme: "sip", Host: "127.0.0.1"})
			req.SetTransport("TCP")
			req.SetDestination(listener.Addr().String())
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			tx, err := client.TransactionRequest(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tx.Terminate)
			listener.SetDeadline(time.Now().Add(time.Second))
			conn, err := listener.AcceptTCP()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { conn.Close() })
			conn.SetReadDeadline(time.Now().Add(time.Second))
			reader := bufio.NewReader(conn)
			var wire strings.Builder
			for n := 0; n < 32; n++ {
				line, err := reader.ReadString('\n')
				if err != nil {
					t.Fatal(err)
				}
				wire.WriteString(line)
				if line == "\r\n" {
					break
				}
			}
			message, err := sip.ParseMessage([]byte(wire.String()))
			if err != nil {
				t.Fatal(err)
			}
			incoming, ok := message.(*sip.Request)
			if !ok {
				t.Fatal("missing TCP request")
			}
			if !scenario.final {
				select {
				case <-tx.Done():
					if !errors.Is(tx.Err(), sip.ErrTransactionTimeout) {
						t.Fatal("TCP deadline lost B/F timeout")
					}
				case <-time.After(700 * time.Millisecond):
					t.Fatal("TCP disabled B/F with retransmission")
				}
				return
			}
			conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
			if _, err := reader.ReadByte(); err == nil {
				t.Fatal("reliable transport retransmitted request")
			} else if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
				t.Fatal(err)
			}
			code := 200
			if method == sip.INVITE {
				code = 486
			}
			response := sip.NewResponseFromRequest(incoming, code, "Synthetic", nil)
			conn.SetWriteDeadline(time.Now().Add(time.Second))
			if _, err := conn.Write([]byte(response.String())); err != nil {
				t.Fatal(err)
			}
			select {
			case <-tx.Responses():
			case <-time.After(time.Second):
				t.Fatal("TCP response missing")
			}
			select {
			case <-tx.Done():
			case <-time.After(400 * time.Millisecond):
				t.Fatal("reliable transport used nonzero D/K retention")
			}
		})
	}
}

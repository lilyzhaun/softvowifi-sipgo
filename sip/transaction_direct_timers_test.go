package sip

import (
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"
)

func TestDirectServerTimerPolicyDrivesActualResponseAndDeadline(t *testing.T) {
	peer, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { peer.Close() })
	sender, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sender.Close() })
	address := peer.LocalAddr().(*net.UDPAddr)
	request := testUDPServerRequest(address.String(), "127.0.0.1", address.Port)
	request.Method = INVITE
	request.AppendHeader(&CSeqHeader{SeqNo: 1, MethodName: INVITE})
	connection := &UDPConnection{PacketConn: sender, Listener: true, refcount: 2}
	policy := TransactionTimers{T1: time.Hour, T2: time.Hour, T4: time.Second, B: time.Hour, D: time.Second, F: time.Hour, H: 100 * time.Millisecond, J: time.Second}
	bad := policy
	bad.H = 0
	if tx, err := NewServerTxWithTimers("invalid", request, connection, slog.Default(), bad); err == nil || tx != nil {
		t.Fatal("invalid direct policy constructed a transaction")
	}
	tx, err := NewServerTxWithTimers("direct", request, connection, slog.Default(), policy)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Terminate)
	policy.H = time.Hour
	if err := tx.Init(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Respond(NewResponseFromRequest(request, 486, "Busy Here", nil)); err != nil {
		t.Fatal(err)
	}
	peer.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 4096)
	n, _, err := peer.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	message, err := ParseMessage(buf[:n])
	if err != nil {
		t.Fatal(err)
	}
	response, ok := message.(*Response)
	if !ok || response.StatusCode != 486 {
		t.Fatal("direct transaction did not send actual final response")
	}
	select {
	case <-tx.Done():
		if !errors.Is(tx.Err(), ErrTransactionTimeout) {
			t.Fatal("direct H lost timeout cause")
		}
	case <-time.After(600 * time.Millisecond):
		t.Fatal("direct transaction ignored captured H")
	}
}

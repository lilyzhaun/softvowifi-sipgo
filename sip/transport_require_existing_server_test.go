package sip

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func testUDPServerRequest(source string, viaHost string, viaPort int) *Request {
	req := NewRequest(OPTIONS, Uri{Scheme: "sip", Host: "127.0.0.1"})
	req.SetTransport("UDP")
	req.SetSource(source)
	viaParams := NewParams()
	viaParams.Add("branch", GenerateBranchN(16))
	req.PrependHeader(&ViaHeader{
		ProtocolName:    "SIP",
		ProtocolVersion: "2.0",
		Transport:       "UDP",
		Host:            viaHost,
		Port:            viaPort,
		Params:          viaParams,
	})
	return req
}

func Test_serverRequestConnection_default_creates(t *testing.T) {
	layer := NewTransportLayer(net.DefaultResolver, NewParser(), nil)
	t.Cleanup(func() { _ = layer.Close() })
	req := testUDPServerRequest("127.0.0.1:5061", "127.0.0.1", 5060)
	conn, err := layer.serverRequestConnection(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateConnection: %v", err)
	}
	if conn == nil {
		t.Fatal("nil connection")
	}
	if got := layer.udp.createCount.Load(); got != 1 {
		t.Fatalf("createCount=%d want 1", got)
	}
}

func Test_serverRequestConnection_requireExisting_miss_doesNotCreate(t *testing.T) {
	layer := NewTransportLayer(net.DefaultResolver, NewParser(), nil, WithTransportLayerRequireExistingConnection())
	t.Cleanup(func() { _ = layer.Close() })
	req := testUDPServerRequest("127.0.0.1:5061", "127.0.0.1", 5060)
	conn, err := layer.serverRequestConnection(context.Background(), req)
	if conn != nil {
		t.Fatal("created fallback connection")
	}
	if !errors.Is(err, ErrRequireExistingConnection) {
		t.Fatalf("err=%v", err)
	}
	if got := layer.udp.createCount.Load(); got != 0 {
		t.Fatalf("createCount=%d want 0", got)
	}
}

func Test_serverRequestConnection_requireExisting_afterClose_doesNotCreate(t *testing.T) {
	ln, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	layer := NewTransportLayer(net.DefaultResolver, NewParser(), nil, WithTransportLayerRequireExistingConnection())
	t.Cleanup(func() { _ = layer.Close() })
	go func() { _ = layer.ServeUDP(ln) }()
	waitUDPPool(t, layer, ln.LocalAddr().String())
	_ = ln.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && layer.udp.GetConnection(ln.LocalAddr().String()) != nil {
		time.Sleep(time.Millisecond)
	}
	req := testUDPServerRequest("127.0.0.1:5061", "127.0.0.1", 5060)
	conn, err := layer.serverRequestConnection(context.Background(), req)
	if conn != nil {
		t.Fatal("created after close")
	}
	if !errors.Is(err, ErrRequireExistingConnection) {
		t.Fatalf("err=%v", err)
	}
	if got := layer.udp.createCount.Load(); got != 0 {
		t.Fatalf("createCount=%d want 0", got)
	}
}

func Test_serverRequestConnection_requireExisting_concurrentIncoming(t *testing.T) {
	ln, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	layer := NewTransportLayer(net.DefaultResolver, NewParser(), nil, WithTransportLayerRequireExistingConnection())
	t.Cleanup(func() { _ = layer.Close() })
	go func() { _ = layer.ServeUDP(ln) }()
	waitUDPPool(t, layer, ln.LocalAddr().String())
	req := testUDPServerRequest("127.0.0.1:5061", "127.0.0.1", 5060)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = layer.serverRequestConnection(context.Background(), req)
	}()
	go func() {
		defer wg.Done()
		_ = ln.Close()
		_ = layer.Close()
	}()
	wg.Wait()
	conn, err := layer.serverRequestConnection(context.Background(), req)
	if conn != nil || !errors.Is(err, ErrRequireExistingConnection) {
		t.Fatalf("after concurrent close conn=%v err=%v", conn, err)
	}
	if got := layer.udp.createCount.Load(); got != 0 {
		t.Fatalf("createCount=%d want 0", got)
	}
}

func Test_serverRequestConnection_requireExisting_reusesBoundConn(t *testing.T) {
	ln, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	layer := NewTransportLayer(net.DefaultResolver, NewParser(), nil, WithTransportLayerRequireExistingConnection())
	t.Cleanup(func() { _ = layer.Close() })
	go func() { _ = layer.ServeUDP(ln) }()
	waitUDPPool(t, layer, ln.LocalAddr().String())
	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("peer: %v", err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	if _, err := peer.WriteToUDP([]byte("\r\n"), ln.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatalf("poke: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && layer.udp.GetConnection(peer.LocalAddr().String()) == nil {
		time.Sleep(time.Millisecond)
	}
	source := peer.LocalAddr().String()
	_, sourcePort, err := ParseAddr(source)
	if err != nil {
		t.Fatalf("ParseAddr: %v", err)
	}
	req := testUDPServerRequest(source, "127.0.0.1", sourcePort)
	conn, err := layer.serverRequestConnection(context.Background(), req)
	if err != nil {
		t.Fatalf("reuse: %v", err)
	}
	if conn.LocalAddr().String() != ln.LocalAddr().String() {
		t.Fatalf("conn=%s listener=%s", conn.LocalAddr(), ln.LocalAddr())
	}
	if got := layer.udp.createCount.Load(); got != 0 {
		t.Fatalf("createCount=%d want 0", got)
	}
	resp := NewResponseFromRequest(req, StatusOK, "OK", nil)
	resp.SetTransport("UDP")
	resp.SetDestination(source)
	if err := conn.WriteMsg(resp); err != nil {
		t.Fatalf("WriteMsg: %v", err)
	}
	_ = peer.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 2048)
	n, _, err := peer.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read 200: %v", err)
	}
	msg, err := ParseMessage(buf[:n])
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, ok := msg.(*Response)
	if !ok || got.StatusCode != StatusOK {
		t.Fatalf("resp=%v", msg)
	}
}

func Test_serverRequestConnection_requireExisting_inviteTryingUsesBoundConn(t *testing.T) {
	ln, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	layer := NewTransportLayer(net.DefaultResolver, NewParser(), nil, WithTransportLayerRequireExistingConnection())
	t.Cleanup(func() { _ = layer.Close() })
	_ = NewTransactionLayer(layer)
	go func() { _ = layer.ServeUDP(ln) }()
	waitUDPPool(t, layer, ln.LocalAddr().String())
	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("peer: %v", err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	_, peerPort, err := ParseAddr(peer.LocalAddr().String())
	if err != nil {
		t.Fatalf("ParseAddr: %v", err)
	}
	invite := NewRequest(INVITE, Uri{Scheme: "sip", User: "bob", Host: "127.0.0.1"})
	invite.SetTransport("UDP")
	fromParams := NewParams()
	fromParams.Add("tag", "peer")
	invite.AppendHeader(&FromHeader{Address: Uri{Scheme: "sip", User: "alice", Host: "127.0.0.1"}, Params: fromParams})
	invite.AppendHeader(&ToHeader{Address: Uri{Scheme: "sip", User: "bob", Host: "127.0.0.1"}})
	invite.AppendHeader(NewHeader("Max-Forwards", "70"))
	id := CallIDHeader("invite-bound")
	invite.AppendHeader(&id)
	invite.AppendHeader(&CSeqHeader{SeqNo: 1, MethodName: INVITE})
	viaParams := NewParams()
	viaParams.Add("branch", GenerateBranchN(16))
	invite.PrependHeader(&ViaHeader{ProtocolName: "SIP", ProtocolVersion: "2.0", Transport: "UDP", Host: "127.0.0.1", Port: peerPort, Params: viaParams})
	invite.SetBody(nil)
	if _, err := peer.WriteToUDP([]byte(invite.String()), ln.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatalf("INVITE: %v", err)
	}
	_ = peer.SetReadDeadline(time.Now().Add(Timer_1xx + 500*time.Millisecond))
	buf := make([]byte, 4096)
	n, from, err := peer.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("100 Trying: %v", err)
	}
	if from.String() != ln.LocalAddr().String() {
		t.Fatalf("trying from %s want %s", from, ln.LocalAddr())
	}
	msg, err := ParseMessage(buf[:n])
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	resp, ok := msg.(*Response)
	if !ok || resp.StatusCode != StatusTrying {
		t.Fatalf("resp=%v", msg)
	}
	if got := layer.udp.createCount.Load(); got != 0 {
		t.Fatalf("createCount=%d want 0", got)
	}
}

func Test_serverRequestConnection_requireExisting_inviteMissDoesNotCreate(t *testing.T) {
	layer := NewTransportLayer(net.DefaultResolver, NewParser(), nil, WithTransportLayerRequireExistingConnection())
	t.Cleanup(func() { _ = layer.Close() })
	txl := NewTransactionLayer(layer)
	invite := NewRequest(INVITE, Uri{Scheme: "sip", User: "bob", Host: "127.0.0.1"})
	invite.SetTransport("UDP")
	invite.SetSource("127.0.0.1:5061")
	fromParams := NewParams()
	fromParams.Add("tag", "peer")
	invite.AppendHeader(&FromHeader{Address: Uri{Scheme: "sip", User: "alice", Host: "127.0.0.1"}, Params: fromParams})
	invite.AppendHeader(&ToHeader{Address: Uri{Scheme: "sip", User: "bob", Host: "127.0.0.1"}})
	invite.AppendHeader(NewHeader("Max-Forwards", "70"))
	id := CallIDHeader("invite-miss")
	invite.AppendHeader(&id)
	invite.AppendHeader(&CSeqHeader{SeqNo: 1, MethodName: INVITE})
	viaParams := NewParams()
	viaParams.Add("branch", GenerateBranchN(16))
	invite.PrependHeader(&ViaHeader{ProtocolName: "SIP", ProtocolVersion: "2.0", Transport: "UDP", Host: "127.0.0.1", Port: 5061, Params: viaParams})
	invite.SetBody(nil)
	if err := txl.handleRequest(invite); err == nil {
		t.Fatal("expected typed miss error")
	} else if !errors.Is(err, ErrRequireExistingConnection) {
		t.Fatalf("err=%v", err)
	}
	if got := layer.udp.createCount.Load(); got != 0 {
		t.Fatalf("createCount=%d want 0", got)
	}
}

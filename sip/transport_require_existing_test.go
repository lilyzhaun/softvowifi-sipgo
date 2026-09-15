package sip

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func testUDPClientRequest(laddr Addr, dest string) *Request {
	req := NewRequest(REGISTER, Uri{Scheme: "sip", Host: "example.com"})
	req.SetTransport("UDP")
	req.SetDestination(dest)
	req.Laddr = laddr
	viaParams := NewParams()
	viaParams.Add("branch", GenerateBranchN(16))
	req.PrependHeader(&ViaHeader{
		ProtocolName:    "SIP",
		ProtocolVersion: "2.0",
		Transport:       "UDP",
		Host:            "127.0.0.1",
		Port:            5060,
		Params:          viaParams,
	})
	return req
}

func waitUDPPool(t *testing.T, layer *TransportLayer, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if layer.udp.GetConnection(addr) != nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("listener not pooled: %s", addr)
}

func Test_ClientRequestConnection_default_creates(t *testing.T) {
	layer := NewTransportLayer(net.DefaultResolver, NewParser(), nil)
	t.Cleanup(func() { _ = layer.Close() })
	req := testUDPClientRequest(Addr{}, "127.0.0.1:5060")
	conn, err := layer.ClientRequestConnection(context.Background(), req)
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

func Test_ClientRequestConnection_requireExisting_miss_doesNotCreate(t *testing.T) {
	layer := NewTransportLayer(net.DefaultResolver, NewParser(), nil, WithTransportLayerRequireExistingConnection())
	t.Cleanup(func() { _ = layer.Close() })
	req := testUDPClientRequest(Addr{IP: net.IPv4(127, 0, 0, 1), Port: 5060}, "127.0.0.1:5060")
	conn, err := layer.ClientRequestConnection(context.Background(), req)
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

func Test_ClientRequestConnection_requireExisting_reusesListener(t *testing.T) {
	ln, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	layer := NewTransportLayer(net.DefaultResolver, NewParser(), nil, WithTransportLayerRequireExistingConnection())
	t.Cleanup(func() { _ = layer.Close() })
	go func() { _ = layer.ServeUDP(ln) }()
	laddr := ln.LocalAddr().String()
	waitUDPPool(t, layer, laddr)
	host, port, err := ParseAddr(laddr)
	if err != nil {
		t.Fatalf("ParseAddr: %v", err)
	}
	req := testUDPClientRequest(Addr{IP: net.ParseIP(host), Port: port}, "127.0.0.1:5060")
	conn, err := layer.ClientRequestConnection(context.Background(), req)
	if err != nil {
		t.Fatalf("reuse: %v", err)
	}
	if conn == nil {
		t.Fatal("nil reused connection")
	}
	if got := layer.udp.createCount.Load(); got != 0 {
		t.Fatalf("createCount=%d want 0", got)
	}
}

func Test_ClientRequestConnection_requireExisting_afterClose_doesNotCreate(t *testing.T) {
	ln, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	layer := NewTransportLayer(net.DefaultResolver, NewParser(), nil, WithTransportLayerRequireExistingConnection())
	t.Cleanup(func() { _ = layer.Close() })
	go func() { _ = layer.ServeUDP(ln) }()
	laddr := ln.LocalAddr().String()
	waitUDPPool(t, layer, laddr)
	_ = ln.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && layer.udp.GetConnection(laddr) != nil {
		time.Sleep(time.Millisecond)
	}
	host, port, err := ParseAddr(laddr)
	if err != nil {
		t.Fatalf("ParseAddr: %v", err)
	}
	req := testUDPClientRequest(Addr{IP: net.ParseIP(host), Port: port}, "127.0.0.1:5060")
	conn, err := layer.ClientRequestConnection(context.Background(), req)
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

func Test_ClientRequestConnection_requireExisting_concurrentClose(t *testing.T) {
	ln, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	layer := NewTransportLayer(net.DefaultResolver, NewParser(), nil, WithTransportLayerRequireExistingConnection())
	t.Cleanup(func() { _ = layer.Close() })
	go func() { _ = layer.ServeUDP(ln) }()
	laddr := ln.LocalAddr().String()
	waitUDPPool(t, layer, laddr)
	host, port, err := ParseAddr(laddr)
	if err != nil {
		t.Fatalf("ParseAddr: %v", err)
	}
	req := testUDPClientRequest(Addr{IP: net.ParseIP(host), Port: port}, "127.0.0.1:5060")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = layer.ClientRequestConnection(context.Background(), req)
	}()
	go func() {
		defer wg.Done()
		_ = ln.Close()
		_ = layer.Close()
	}()
	wg.Wait()
	req2 := testUDPClientRequest(Addr{IP: net.ParseIP(host), Port: port}, "127.0.0.1:5060")
	conn, err := layer.ClientRequestConnection(context.Background(), req2)
	if conn != nil || !errors.Is(err, ErrRequireExistingConnection) {
		t.Fatalf("after concurrent close conn=%v err=%v", conn, err)
	}
	if got := layer.udp.createCount.Load(); got != 0 {
		t.Fatalf("createCount=%d want 0", got)
	}
}

func Test_UDP_readFilter_drops_before_parser(t *testing.T) {
	ln, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	parsed := make(chan struct{}, 1)
	layer := NewTransportLayer(net.DefaultResolver, NewParser(), nil, WithTransportLayerReadFilter(func(info TransportReadProps, data []byte) ([]byte, error) {
		if info.RemoteAddr == nil || net.ParseIP(pcscfHost(info.RemoteAddr.String())).Equal(net.IPv4(127, 0, 0, 1)) {
			return nil, nil
		}
		return data, nil
	}))
	t.Cleanup(func() { _ = layer.Close() })
	layer.OnMessage(func(Message) { parsed <- struct{}{} })
	go func() { _ = layer.ServeUDP(ln) }()
	waitUDPPool(t, layer, ln.LocalAddr().String())
	src, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("src: %v", err)
	}
	t.Cleanup(func() { _ = src.Close() })
	pkt := []byte("OPTIONS sip:x SIP/2.0\r\nVia: SIP/2.0/UDP 127.0.0.1:9;branch=z9hG4bK.x\r\nFrom: <sip:a@x>;tag=a\r\nTo: <sip:b@x>\r\nCall-ID: drop\r\nCSeq: 1 OPTIONS\r\nContent-Length: 0\r\n\r\n")
	if _, err := src.WriteToUDP(pkt, ln.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case <-parsed:
		t.Fatal("non-matching source reached parser")
	case <-time.After(200 * time.Millisecond):
	}
}

func pcscfHost(raw string) string {
	host, _, err := net.SplitHostPort(raw)
	if err != nil {
		return raw
	}
	return host
}

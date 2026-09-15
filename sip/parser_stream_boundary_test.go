package sip

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestStreamLogicalHeaders_when_Fragmented(t *testing.T) {
	body := "Security-Server: body\r\nContent-Length: 0\r\n\r\nSIP/2.0 200 fake"
	for _, lengthName := range []string{"Content-Length", "l"} {
		wire := "SIP/2.0 401 Unauthorized\r\nCSeq: 2 REGISTER\r\n" +
			"Security-Server: public; \t\r\n\tfirst=one;\r\n second=sip:public.invalid;\r\n\tthird=three\r\n" +
			"SECURITY-SERVER: duplicate\r\nSecurity-Server: \t\r\n" +
			fmt.Sprintf("%s: %d\r\n\r\n", lengthName, len(body)) + body +
			"SIP/2.0 423 Interval Too Brief\r\nCSeq: 3 REGISTER\r\nl: 0\r\n\r\n"
		check := func(t *testing.T, chunks []string) {
			t.Helper()
			parser := NewParser().NewSIPStream()
			defer parser.Close()
			var messages []Message
			for _, chunk := range chunks {
				err := parser.ParseSIPStream([]byte(chunk), func(message Message) { messages = append(messages, message) })
				if err != nil && !errors.Is(err, ErrParseSipPartial) {
					t.Fatalf("parse: %v", err)
				}
			}
			if len(messages) != 2 {
				t.Fatalf("callbacks=%d, want 2", len(messages))
			}
			first, ok := messages[0].(*Response)
			if !ok || first.StatusCode != 401 || first.CSeq().SeqNo != 2 || first.CSeq().MethodName != REGISTER {
				t.Fatalf("first response: %v", messages[0])
			}
			var values []string
			for _, header := range first.GetHeaders("Security-Server") {
				values = append(values, header.Value())
			}
			want := []string{"public; first=one; second=sip:public.invalid; third=three", "duplicate", ""}
			if !reflect.DeepEqual(values, want) || string(first.Body()) != body {
				t.Fatalf("headers=%q body=%q", values, first.Body())
			}
			second, ok := messages[1].(*Response)
			if !ok || second.StatusCode != 423 || second.CSeq().SeqNo != 3 || len(second.Body()) != 0 || len(second.GetHeaders("Security-Server")) != 0 {
				t.Fatalf("second response: %v", messages[1])
			}
			if parser.Buffer().Len() != 0 {
				t.Fatal("unconsumed bytes")
			}
		}
		for cut := 0; cut <= len(wire); cut++ {
			t.Run(fmt.Sprintf("%s/cut-%d", lengthName, cut), func(t *testing.T) {
				check(t, []string{wire[:cut], wire[cut:]})
			})
		}
		t.Run(lengthName+"/bytewise", func(t *testing.T) {
			chunks := make([]string, len(wire))
			for offset := range wire {
				chunks[offset] = wire[offset : offset+1]
			}
			check(t, chunks)
		})
		t.Run(lengthName+"/physical-lines", func(t *testing.T) {
			check(t, strings.SplitAfter(wire, "\r\n"))
		})
	}
}

func TestStreamPendingHeader_when_LogicalLineEndsAtChunkBoundary(t *testing.T) {
	for _, header := range []string{"X-Public: one\r\n", "X-Public: one\r\n two\r\n\tthree\r\n"} {
		parser := NewParser().NewSIPStream()
		defer parser.Close()
		prefix := "SIP/2.0 200 OK\r\n"
		if _, err := parser.Write([]byte(prefix + header)); err != nil {
			t.Fatal(err)
		}
		message, consumed, err := parser.ParseNext()
		if !errors.Is(err, io.ErrUnexpectedEOF) || consumed != len(prefix) || parser.Buffer().String() != header || len(message.GetHeaders("X-Public")) != 0 {
			t.Fatalf("premature header consumption: n=%d err=%v buffer=%q", consumed, err, parser.Buffer().String())
		}
	}
}

func TestCompleteParser_when_FinalHeaderHasNoTerminator(t *testing.T) {
	parser := NewParser()
	message, consumed, err := parser.ParseHeaders([]byte("SIP/2.0 200 OK\r\nX-Public: one\r\n"), false)
	if !errors.Is(err, io.ErrUnexpectedEOF) || consumed != 31 || len(message.GetHeaders("X-Public")) != 1 {
		t.Fatalf("complete-buffer semantics changed: n=%d err=%v message=%v", consumed, err, message)
	}
}

func TestStreamLimits_when_PartialMessageKeepsGrowing(t *testing.T) {
	for _, testCase := range []struct {
		name, prefix, repeat string
	}{
		{"start-line", "SIP/2.0 200 ", "x"},
		{"header", "SIP/2.0 200 OK\r\nX-Public: ", "x"},
		{"continuation", "SIP/2.0 200 OK\r\nX-Public: one\r\n", " two\r\n"},
		{"many-headers", "SIP/2.0 200 OK\r\n", "X-Public: one\r\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			base := NewParser()
			base.MaxMessageLength = 128
			parser := base.NewSIPStream()
			defer parser.Close()
			callback := func(Message) { t.Fatal("oversized message delivered") }
			err := parser.ParseSIPStream([]byte(testCase.prefix), callback)
			for count := 0; count < 256 && errors.Is(err, ErrParseSipPartial); count++ {
				err = parser.ParseSIPStream([]byte(testCase.repeat), callback)
			}
			if !errors.Is(err, ErrMessageTooLarge) {
				t.Fatalf("growth was not bounded: %v", err)
			}
			if parser.totalRead+parser.Buffer().Len() > 128+len(testCase.repeat) {
				t.Fatal("accepted excess input after reporting limit")
			}
		})
	}
}

func TestStreamLimits_when_DeclaredBodyExceedsBudget(t *testing.T) {
	for _, length := range []string{"4294967295", "18446744073709551615", "99999999999999999999999999999999"} {
		parser := NewParser().NewSIPStream()
		defer parser.Close()
		err := parser.ParseSIPStream([]byte("SIP/2.0 200 OK\r\nl: "+length+"\r\n\r\n"), func(Message) { t.Fatal("delivered oversized body") })
		if err == nil || errors.Is(err, ErrParseSipPartial) {
			t.Fatalf("length=%s err=%v", length, err)
		}
		if parser.msg != nil && len(parser.msg.Body()) != 0 {
			t.Fatal("allocated oversized body")
		}
	}
}

func TestStreamLimits_when_BatchExceedsIndividualBudget(t *testing.T) {
	wire := "SIP/2.0 200 OK\r\nl: 0\r\n\r\n"
	base := NewParser()
	base.MaxMessageLength = len(wire)
	parser := base.NewSIPStream()
	defer parser.Close()
	count := 0
	err := parser.ParseSIPStream([]byte(strings.Repeat(wire, 10)), func(Message) { count++ })
	if err != nil || count != 10 {
		t.Fatalf("per-message budget rejected batch: count=%d err=%v", count, err)
	}
}

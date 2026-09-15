package sip_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/emiago/sipgo/sip"
)

func Test_HeadersPreserved_when_PublicBytesSegmented(t *testing.T) {
	cases := []struct {
		name     string
		headers  string
		lines    int
		nonempty int
	}{
		{"absent", "", 0, 0},
		{"mixed_case", "sEcUrItY-SeRvEr: public\r\n", 1, 1},
		{"duplicates", "Security-Server: public\r\nSECURITY-SERVER: other\r\n", 2, 2},
		{"empty", "Security-Server: \t\r\n", 1, 0},
		{"fold", "Security-Server: public;\r\n\tparameter=value\r\n", 1, 1},
		{"fold_colon", "Security-Server: public;\r\n\tparameter=sip:public.invalid\r\n", 1, 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			body := "Security-Server: body-only\r\n\r\nSIP/2.0 401 body"
			wire := []byte("SIP/2.0 401 Unauthorized\r\nCSeq: 2 REGISTER\r\nWWW-Authenticate: public\r\n" + testCase.headers + fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body)) + body + "SIP/2.0 423 Interval Too Brief\r\nCSeq: 3 REGISTER\r\nl: 0\r\n\r\n")
			failures := 0
			for cut := 0; cut <= len(wire); cut++ {
				parser := sip.NewParser().NewSIPStream()
				var messages []sip.Message
				valid := true
				for _, segment := range [][]byte{wire[:cut], wire[cut:]} {
					err := parser.ParseSIPStream(segment, func(message sip.Message) { messages = append(messages, message) })
					if err != nil && !errors.Is(err, sip.ErrParseSipPartial) {
						t.Logf("parserAccepted=false index=%d", cut)
						valid = false
						break
					}
				}
				parser.Close()
				if !valid || len(messages) != 2 {
					failures++
					continue
				}
				response, ok := messages[0].(*sip.Response)
				if !ok {
					failures++
					continue
				}
				headers := response.GetHeaders("Security-Server")
				nonempty := 0
				for _, header := range headers {
					if strings.TrimSpace(header.Value()) != "" {
						nonempty++
					}
				}
				if response.StatusCode != 401 || response.CSeq().MethodName != sip.REGISTER || len(headers) != testCase.lines || nonempty != testCase.nonempty || len(response.GetHeaders("WWW-Authenticate")) != 1 || string(response.Body()) != body {
					failures++
				}
				if testCase.name == "fold_colon" && len(headers) == 1 && headers[0].Value() != "public; parameter=sip:public.invalid" {
					failures++
				}
			}
			if failures != 0 {
				t.Errorf("matrixFailures=%d", failures)
			}
		})
	}
}

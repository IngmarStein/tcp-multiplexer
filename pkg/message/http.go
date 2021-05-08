package message

import (
	"bufio"
	"bytes"
	"github.com/davecgh/go-spew/spew"
	"github.com/sirupsen/logrus"
	"io"
	"net/textproto"
	"strconv"
)

// https://tools.ietf.org/html/rfc2616
// refer /usr/local/Cellar/go/1.16.3/libexec/src/net/http/request.go:1021 readRequest

type HTTPMessageReader struct {
}

const (
	contentLength = "Content-Length"
	CRLF          = "\r\n"
)

// support HTTP1 plaintext
// DO NOT Support:
// 1. https
// 2. websocket

func (H HTTPMessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	tp := textproto.NewReader(bufio.NewReader(conn))
	startLine, err := tp.ReadLine()
	if err != nil {
		return nil, err
	}

	headers, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}

	var body []byte
	if vv, ok := headers[contentLength]; ok {
		size, err := strconv.Atoi(vv[0])
		if err != nil {
			return nil, err
		}

		body = make([]byte, size)
		_, err = tp.R.Read(body)
		if err != nil {
			return nil, err
		}
	}

	msg := dumpHTTPMessage(startLine, headers, body)

	if logrus.GetLevel() == logrus.DebugLevel {
		spew.Dump(msg)
	}

	return msg, err
}

func dumpHTTPMessage(startLine string, headers textproto.MIMEHeader, body []byte) []byte {
	var b bytes.Buffer
	b.WriteString(startLine)
	b.WriteString(CRLF)
	for k, vv := range headers {
		for _, v := range vv {
			b.WriteString(k)
			b.WriteString(": ")
			b.WriteString(v)
			b.WriteString(CRLF)
		}
	}
	b.WriteString(CRLF)
	if len(body) != 0 {
		b.Write(body)
	}
	return b.Bytes()
}

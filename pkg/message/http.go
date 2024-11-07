package message

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/davecgh/go-spew/spew"
)

// https://tools.ietf.org/html/rfc2616
// refer /usr/local/Cellar/go/1.16.3/libexec/src/net/http/request.go:1021 readRequest

type HTTPMessageReader struct {
}

func (H HTTPMessageReader) Name() string {
	return "http"
}

const (
	headerKeyContentLength = "Content-Length"
	headerKeyContentType   = "Content-Type"
	headerFormContentType  = "multipart/form-data"
	CRLF                   = "\r\n"
	boundaryPrefix         = "boundary="
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
	slog.Debug(startLine)

	headers, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	slog.Debug(fmt.Sprintf("%v", headers))

	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Messages#body
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Messages#body_2
	// 1. without body
	var body []byte

	// first check form type
	isFormContentType := false
	if vv, ok := headers[headerKeyContentType]; ok {
		slog.Debug(vv[0])
		parts := strings.Split(vv[0], ";")
		contentType := strings.TrimSpace(parts[0])
		// 3. Multiple-resource bodies
		if contentType == headerFormContentType {
			isFormContentType = true
			if len(parts) < 2 {
				return nil, errors.New("expect boundary= part in " + headerKeyContentType)
			}
			boundaryPart := strings.TrimSpace(parts[1])
			if !strings.HasPrefix(boundaryPart, boundaryPrefix) {
				return nil, errors.New("expect boundary= part in " + headerKeyContentType)
			}

			lastBoundary := "--" + strings.TrimPrefix(boundaryPart, boundaryPrefix) + "--"
			scanner := bufio.NewScanner(tp.R)
			for scanner.Scan() {
				line := scanner.Bytes()
				body = append(body, line...)
				body = append(body, []byte(CRLF)...)
				if string(line) == lastBoundary {
					break
				}
			}
			if err := scanner.Err(); err != nil {
				return nil, err
			}
		}
	}

	// 2. Single-resource bodies: use Content-Length as size
	if !isFormContentType {
		if vv, ok := headers[headerKeyContentLength]; ok {
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
	}

	// TODO: 4. Transfer-Encoding
	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Transfer-Encoding

	msg := dumpHTTPMessage(startLine, headers, body)

	slog.Debug(spew.Sdump(msg))

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

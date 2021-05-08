package message

import (
	"bufio"
	"io"
)

// https://tools.ietf.org/html/rfc862

type EchoMessageReader struct {
}

// ReadMessage message is expected \n terminated
func (e EchoMessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	return bufio.NewReader(conn).ReadBytes('\n')
}

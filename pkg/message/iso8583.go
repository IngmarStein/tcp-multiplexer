package message

import (
	"bytes"
	"encoding/binary"
	"io"
)

type ISO8583MessageReader struct {
}

// ReadMessage assume including a header with the length of the 8583 message
// http://j8583.sourceforge.net/desc8583en.html
// otherwise, we have to parse iso8583 message
func (I ISO8583MessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	header := make([]byte, 2)
	_, err := conn.Read(header)
	if err != nil {
		return nil, err
	}

	var length uint16
	err = binary.Read(bytes.NewReader(header), binary.BigEndian, &length)
	if err != nil {
		return nil, err
	}

	isoMsg := make([]byte, length)
	_, err = conn.Read(isoMsg)
	if err != nil {
		return nil, err
	}

	return append(header, isoMsg...), nil
}

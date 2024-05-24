package message

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	maxTCPFrameLength = 260
	mbapHeaderLength  = 6
)

type ModbusMessageReader struct {
}

func (m ModbusMessageReader) Name() string {
	return "modbus"
}

func (m ModbusMessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	header := make([]byte, mbapHeaderLength)
	_, err := io.ReadFull(conn, header)
	if err != nil {
		return nil, err
	}

	// determine how many more bytes we need to read
	bytesNeeded := int(binary.BigEndian.Uint16(header[4:6]))
	// never read more than the max allowed frame length
	if bytesNeeded+mbapHeaderLength > maxTCPFrameLength {
		return nil, fmt.Errorf("protocol error: %d larger than max allowed frame length (%d)", bytesNeeded+mbapHeaderLength, maxTCPFrameLength)
	}

	// an MBAP length of 0 is illegal
	if bytesNeeded <= 0 {
		return nil, fmt.Errorf("protocol error: illegal MBAP length (%d)", bytesNeeded)
	}

	// read the PDU
	rxbuf := make([]byte, bytesNeeded)
	_, err = io.ReadFull(conn, rxbuf)
	if err != nil {
		return nil, err
	}

	return append(header, rxbuf...), nil
}

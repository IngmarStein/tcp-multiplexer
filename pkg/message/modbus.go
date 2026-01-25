package message

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	mbapHeaderLength        = 6
	modbusTCPMaxFrameLength = 260
	modbusRTUMaxFrameLength = 262
)

type ModbusMessageReader struct {
}

func (m ModbusMessageReader) Name() string {
	return "modbus"
}

func (m ModbusMessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	return readModbusMessage(conn, modbusTCPMaxFrameLength, false)
}

type ModbusRTUMessageReader struct {
}

func (m ModbusRTUMessageReader) Name() string {
	return "modbus-rtu"
}

func (m ModbusRTUMessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	return readModbusMessage(conn, modbusRTUMaxFrameLength, true)
}

func readModbusMessage(conn io.Reader, maxFrameLength int, verifyCRC bool) ([]byte, error) {
	header := make([]byte, mbapHeaderLength)
	_, err := io.ReadFull(conn, header)
	if err != nil {
		return nil, err
	}

	// The Protocol field is zero to indicate Modbus protocol.
	if binary.BigEndian.Uint16(header[2:4]) != 0 {
		return nil, fmt.Errorf("protocol error: non-zero protocol id")
	}

	// determine how many more bytes we need to read
	bytesNeeded := int(binary.BigEndian.Uint16(header[4:6]))
	// never read more than the max allowed frame length
	if bytesNeeded+mbapHeaderLength > maxFrameLength {
		return nil, fmt.Errorf("protocol error: %d larger than max allowed frame length (%d)", bytesNeeded+mbapHeaderLength, maxFrameLength)
	}

	// an MBAP length of 0 is illegal
	if bytesNeeded <= 0 {
		return nil, fmt.Errorf("protocol error: illegal MBAP length (%d)", bytesNeeded)
	}

	// read the PDU (and CRC if present)
	rxbuf := make([]byte, bytesNeeded)
	_, err = io.ReadFull(conn, rxbuf)
	if err != nil {
		return nil, err
	}

	fullMsg := append(header, rxbuf...)

	if verifyCRC {
		if len(rxbuf) < 3 { // Must have at least Unit ID (1) and CRC (2)
			return nil, fmt.Errorf("protocol error: frame too short for CRC")
		}
		// CRC is calculated over Unit Address and Message (PDU)
		// These are all bytes after the 6-byte MBAP header, excluding the last 2 bytes (the CRC itself)
		actualCRC := binary.LittleEndian.Uint16(rxbuf[len(rxbuf)-2:])
		expectedCRC := crc16(rxbuf[:len(rxbuf)-2])
		if actualCRC != expectedCRC {
			return nil, fmt.Errorf("protocol error: CRC mismatch (got %04x, want %04x)", actualCRC, expectedCRC)
		}
	}

	return fullMsg, nil
}

// crc16 calculates the Modbus CRC-16
func crc16(data []byte) uint16 {
	var crc uint16 = 0xFFFF
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&0x0001 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

package message

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	mbapHeaderLength        = 6
	modbusTCPMaxFrameLength = 260
	// Modbus RTU over TCP adds a 2-byte CRC to the Modbus TCP frame.
	modbusRTUMaxFrameLength = modbusTCPMaxFrameLength + 2

	modbusFuncReadCoils              = 1
	modbusFuncReadDiscreteInputs     = 2
	modbusFuncReadHoldingRegisters   = 3
	modbusFuncReadInputRegisters     = 4
	modbusFuncWriteSingleCoil        = 5
	modbusFuncWriteSingleRegister    = 6
	modbusFuncWriteMultipleCoils     = 15
	modbusFuncWriteMultipleRegisters = 16

	modbusExceptionBit = 0x80
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

type ModbusSerialMessageReader struct {
}

func (m ModbusSerialMessageReader) Name() string {
	return "modbus-serial"
}

func (m ModbusSerialMessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	return readModbusSerialMessage(conn)
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
		if len(rxbuf) < 4 { // Must have at least Unit ID (1), Function Code (1), and CRC (2)
			return nil, fmt.Errorf("protocol error: frame too short for CRC, got %d bytes, expected at least 4", len(rxbuf))
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

func readModbusSerialMessage(conn io.Reader) ([]byte, error) {
	// Read Address and Function Code
	head := make([]byte, 2)
	if _, err := io.ReadFull(conn, head); err != nil {
		return nil, err
	}

	funcCode := head[1]
	// Exception response
	if funcCode&modbusExceptionBit != 0 {
		// Fixed length: Addr(1) + Func(1) + Code(1) + CRC(2) = 5 bytes
		rest := make([]byte, 3)
		if _, err := io.ReadFull(conn, rest); err != nil {
			return nil, err
		}
		fullMsg := append(head, rest...)
		if !checkCRC(fullMsg) {
			return nil, fmt.Errorf("protocol error: CRC mismatch for exception frame")
		}
		return fullMsg, nil
	}

	// Read rest based on function code
	var fullMsg []byte

	switch funcCode {
	case modbusFuncReadCoils, modbusFuncReadDiscreteInputs, modbusFuncReadHoldingRegisters, modbusFuncReadInputRegisters:
		// Ambiguous: Request (8 bytes) or Response (3 + ByteCount + 2)
		// Read 3rd byte (ByteCount for Response, or StartAddrHi for Request)
		b3 := make([]byte, 1)
		if _, err := io.ReadFull(conn, b3); err != nil {
			return nil, err
		}

		fullMsg = append(head, b3...)
		byteCount := int(b3[0])
		responseLen := 5 + byteCount
		requestLen := 8

		// Determine which length is shorter to read first
		readTarget := min(responseLen, requestLen)

		// We have 3 bytes. Read until readTarget.
		needed := readTarget - 3
		if needed > 0 {
			buf := make([]byte, needed)
			if _, err := io.ReadFull(conn, buf); err != nil {
				return nil, err
			}
			fullMsg = append(fullMsg, buf...)
		}

		// At this point, len(fullMsg) == min(responseLen, requestLen).
		// If the CRC is valid for this shorter frame, we're done.
		if checkCRC(fullMsg) {
			return fullMsg, nil
		}

		// If we haven't satisfied the longer frame, read the rest
		finalTarget := max(responseLen, requestLen)

		needed = finalTarget - len(fullMsg)
		if needed > 0 {
			buf := make([]byte, needed)
			if _, err := io.ReadFull(conn, buf); err != nil {
				return nil, err
			}
			fullMsg = append(fullMsg, buf...)
		}

		// Check CRC again for the longer frame
		if checkCRC(fullMsg) {
			return fullMsg, nil
		}

		return nil, fmt.Errorf("protocol error: CRC mismatch for ambiguous frame")

	case modbusFuncWriteSingleCoil, modbusFuncWriteSingleRegister:
		// Fixed 8 bytes
		rest := make([]byte, 6)
		if _, err := io.ReadFull(conn, rest); err != nil {
			return nil, err
		}
		fullMsg = append(head, rest...)
		if !checkCRC(fullMsg) {
			return nil, fmt.Errorf("protocol error: CRC mismatch")
		}
		return fullMsg, nil

	case modbusFuncWriteMultipleCoils, modbusFuncWriteMultipleRegisters:
		// Response is Fixed 8. Request is Variable.
		// Read up to 8 bytes.
		rest := make([]byte, 6)
		if _, err := io.ReadFull(conn, rest); err != nil {
			return nil, err
		}
		fullMsg = append(head, rest...)

		// Check CRC (assuming Response)
		if checkCRC(fullMsg) {
			return fullMsg, nil
		}

		// Must be Request.
		// [Addr][Func][StartHi][StartLo][CountHi][CountLo][ByteCount][Data...][CRC]
		// Offset 6 is ByteCount.
		byteCount := int(fullMsg[6])
		// We have 8 bytes: Addr, Func, S, C, BC, D0.
		// Wait, if 8 bytes read:
		// 0: Addr
		// 1: Func
		// 2-3: Start
		// 4-5: Count
		// 6: ByteCount
		// 7: Data[0]

		// Total Length = 1(Addr)+1(Func)+2(Start)+2(Count)+1(BC)+BC(Data)+2(CRC) = 9 + BC.
		// We have 8 bytes.
		// Need 9+BC - 8 = 1 + BC bytes.

		tail := make([]byte, 1+byteCount)
		if _, err := io.ReadFull(conn, tail); err != nil {
			return nil, err
		}
		fullMsg = append(fullMsg, tail...)

		if !checkCRC(fullMsg) {
			return nil, fmt.Errorf("protocol error: CRC mismatch")
		}
		return fullMsg, nil

	default:
		return nil, fmt.Errorf("protocol error: unsupported function code %d", funcCode)
	}
}

func checkCRC(msg []byte) bool {
	if len(msg) < 2 {
		return false
	}
	actual := binary.LittleEndian.Uint16(msg[len(msg)-2:])
	expected := crc16(msg[:len(msg)-2])
	return actual == expected
}

// crc16 calculates the Modbus CRC-16.
func crc16(data []byte) uint16 {
	var crc uint16 = 0xFFFF
	for _, b := range data {
		crc ^= uint16(b)
		for range 8 {
			if crc&0x0001 != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

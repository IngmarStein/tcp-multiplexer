package message

import (
	"io"
	"strconv"
)

// MPUMessageReader for reading MPU Switch format iso8583
type MPUMessageReader struct {
}

func (M MPUMessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	// message header is 4-byte ASCII
	header := make([]byte, 4)
	_, err := conn.Read(header)
	if err != nil {
		return nil, err
	}

	headerStr := string(header)
	length, err:= strconv.Atoi(headerStr)
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

func (M MPUMessageReader) Name() string {
	return "mpu"
}

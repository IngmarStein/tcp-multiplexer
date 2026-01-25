package message

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestModbusSerialMessageReader_ReadMessage(t *testing.T) {
	generateFrame := func(payload []byte) []byte {
		crc := crc16(payload)
		buf := make([]byte, len(payload)+2)
		copy(buf, payload)
		binary.LittleEndian.PutUint16(buf[len(payload):], crc)
		return buf
	}

	reader := &ModbusSerialMessageReader{}

	tests := []struct {
		name    string
		payload []byte
		wantErr bool
	}{
		{
			name: "FC03 Request (Fixed 8)",
			// Addr(1), Func(3), Start(0,0), Count(0,1)
			payload: generateFrame([]byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x01}),
			wantErr: false,
		},
		{
			name: "FC03 Response (Variable, ByteCount=2)",
			// Addr(1), Func(3), ByteCount(2), Data(0,0)
			payload: generateFrame([]byte{0x01, 0x03, 0x02, 0x00, 0x00}),
			wantErr: false,
		},
		{
			name: "FC03 Response (Variable, ByteCount=4)",
			// Addr(1), Func(3), ByteCount(4), Data(0,0,0,0)
			payload: generateFrame([]byte{0x01, 0x03, 0x04, 0x00, 0x00, 0x00, 0x00}),
			wantErr: false,
		},
		{
			name: "FC16 Request (Variable)",
			// Addr(1), Func(16), Start(0,0), Count(0,1), ByteCount(2), Data(0,0)
			payload: generateFrame([]byte{0x01, 0x10, 0x00, 0x00, 0x00, 0x01, 0x02, 0x00, 0x00}),
			wantErr: false,
		},
		{
			name: "FC16 Response (Fixed 8)",
			// Addr(1), Func(16), Start(0,0), Count(0,1)
			payload: generateFrame([]byte{0x01, 0x10, 0x00, 0x00, 0x00, 0x01}),
			wantErr: false,
		},
		{
			name: "Exception Response",
			// Addr(1), Func(0x83), Code(2)
			payload: generateFrame([]byte{0x01, 0x83, 0x02}),
			wantErr: false,
		},
		{
			name: "CRC Error",
			payload: func() []byte {
				f := generateFrame([]byte{0x01, 0x03, 0x00, 0x00, 0x00, 0x01})
				f[len(f)-1] ^= 0xFF // Corrupt CRC
				return f
			}(),
			wantErr: true,
		},
		{
			name:    "Unsupported Function Code",
			payload: generateFrame([]byte{0x01, 0x17, 0x00}), // FC 23 not implemented
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := bytes.NewBuffer(tt.payload)
			got, err := reader.ReadMessage(conn)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if !bytes.Equal(got, tt.payload) {
					t.Errorf("ReadMessage() got = %x, want %x", got, tt.payload)
				}
			}
		})
	}
}

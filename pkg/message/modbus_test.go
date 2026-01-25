package message

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestModbusMessageReader_ReadMessage(t *testing.T) {
	tests := []struct {
		name    string
		reader  Reader
		payload []byte
		wantErr bool
	}{
		{
			name:   "Modbus TCP Normal",
			reader: &ModbusMessageReader{},
			// Trans(2)+Proto(2)+Len(2)+Unit(1)+PDU(1)
			payload: []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x02, 0x01, 0x02},
			wantErr: false,
		},
		{
			name:   "Modbus TCP Non-zero Protocol ID",
			reader: &ModbusMessageReader{},
			// Protocol ID 1 instead of 0
			payload: []byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x02, 0x01, 0x02},
			wantErr: false,
		},
		{
			name:   "Modbus RTU over TCP Valid CRC",
			reader: &ModbusRTUMessageReader{},
			// Payload for CRC: Unit(0x01), PDU(0x03, 0x00, 0x00, 0x00, 0x01)
			// Modbus CRC16 for [0x01, 0x03, 0x00, 0x00, 0x00, 0x01] is 0x0A84
			payload: []byte{
				0x00, 0x01, // Trans
				0x00, 0x00, // Proto
				0x00, 0x08, // Len (1 Unit + 5 PDU + 2 CRC)
				0x01,                         // Unit
				0x03, 0x00, 0x00, 0x00, 0x01, // PDU
				0x84, 0x0A, // CRC (Little Endian: 0x0A84 -> 0x84, 0x0A)
			},
			wantErr: false,
		},
		{
			name:   "Modbus RTU over TCP Invalid CRC",
			reader: &ModbusRTUMessageReader{},
			payload: []byte{
				0x00, 0x01, 0x00, 0x00, 0x00, 0x08,
				0x01, 0x03, 0x00, 0x00, 0x00, 0x01,
				0xFF, 0xFF, // Invalid CRC
			},
			wantErr: true,
		},
		{
			name:   "Modbus TCP Max Length (260)",
			reader: &ModbusMessageReader{},
			payload: func() []byte {
				buf := make([]byte, modbusTCPMaxFrameLength)
				binary.BigEndian.PutUint16(buf[4:6], modbusTCPMaxFrameLength-mbapHeaderLength)
				return buf
			}(),
			wantErr: false,
		},
		{
			name:   "Modbus RTU over TCP Max Length (262)",
			reader: &ModbusRTUMessageReader{},
			payload: func() []byte {
				// Unit(1) + PDU(253) + CRC(2)
				data := make([]byte, modbusRTUMaxFrameLength-mbapHeaderLength-2)
				for i := range data {
					data[i] = byte(i)
				}
				crc := crc16(data)
				buf := make([]byte, modbusRTUMaxFrameLength)
				binary.BigEndian.PutUint16(buf[4:6], modbusRTUMaxFrameLength-mbapHeaderLength)
				copy(buf[6:], data)
				binary.LittleEndian.PutUint16(buf[modbusRTUMaxFrameLength-2:], crc)
				return buf
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := bytes.NewBuffer(tt.payload)
			got, err := tt.reader.ReadMessage(conn)
			if (err != nil) != tt.wantErr {
				t.Errorf("%s: ReadMessage() error = %v, wantErr %v", tt.name, err, tt.wantErr)
				return
			}
			if err == nil {
				if len(got) != len(tt.payload) {
					t.Errorf("%s: ReadMessage() got length = %v, want %v", tt.name, len(got), len(tt.payload))
				}
			}
		})
	}
}

package message

import (
	"bytes"
	"fmt"
	"testing"
)

func TestMPUMessageReader_ReadMessage(t *testing.T) {
	buf := &bytes.Buffer{}
	msgLen := 124
	header := []byte(fmt.Sprintf("%04d", msgLen))
	buf.Write(header)
	for i := 0; i < msgLen; i++ {
		buf.WriteByte('a')
	}
	buf.WriteString("another message")
	fmt.Printf("%x\n", buf)

	iso, err := MPUMessageReader{}.ReadMessage(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal("Expected no error, but got:", err)
	}
	fmt.Printf("%x\n", iso)
}

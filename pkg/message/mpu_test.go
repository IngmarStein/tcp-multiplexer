package message

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
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
	spew.Dump(buf.Bytes())

	iso, err := MPUMessageReader{}.ReadMessage(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal("Expected no error, but got:", err)
	}
	spew.Dump(iso)
}

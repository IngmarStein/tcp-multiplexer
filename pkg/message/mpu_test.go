package message

import (
	"bytes"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
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
	spew.Dump(buf.Bytes())

	iso, err := MPUMessageReader{}.ReadMessage(bytes.NewReader(buf.Bytes()))
	assert.Equal(t, nil, err)
	spew.Dump(iso)
}

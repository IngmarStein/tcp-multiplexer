package message

import "io"

// Reader read message for specified application protocol from client and target server
type Reader interface {
	ReadMessage(conn io.Reader) ([]byte, error)
	Name() string
}

var Readers map[string]Reader

func init() {
	Readers = make(map[string]Reader)
	for _, msgReader := range []Reader{
		&EchoMessageReader{},
		&HTTPMessageReader{},
		&ISO8583MessageReader{},
		&MPUMessageReader{},
	} {
		Readers[msgReader.Name()] = msgReader
	}
}

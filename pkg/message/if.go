package message

import "io"

// Reader read message for specified application protocol from client and target server
type Reader interface {
	ReadMessage(conn io.Reader) ([]byte, error)
}

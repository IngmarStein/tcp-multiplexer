package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/xujiahua/tcp-multiplexer/pkg/message"
	"net"
	"os"
)

func main() {
	targetServer := "127.0.0.1:1234"
	if len(os.Args) > 1 {
		targetServer = os.Args[1]
	}

	conn, err := net.Dial("tcp", targetServer)
	if err != nil {
		fmt.Println(err)
		os.Exit(2)
	}

	for {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print(">> ")
		inputData, _ := reader.ReadBytes('\n')
		buf := new(bytes.Buffer)
		err := binary.Write(buf, binary.BigEndian, uint16(len(inputData)))
		handleErr(err)

		_, err = conn.Write(append(buf.Bytes(), inputData...))
		handleErr(err)

		msg, err := message.ISO8583MessageReader{}.ReadMessage(conn)
		handleErr(err)

		spew.Dump(msg)
	}
}

func handleErr(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(2)
	}
}

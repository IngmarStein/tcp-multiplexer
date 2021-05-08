package main

import (
	"fmt"
	"github.com/xujiahua/tcp-multiplexer/pkg/message"
	"io"
	"net"
	"os"
)

func handleConnection(conn net.Conn) {
	defer func(c net.Conn) {
		err := c.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(conn)

	for {
		data, err := message.ISO8583MessageReader{}.ReadMessage(conn)
		if err == io.EOF {
			fmt.Println("connection is closed")
			break
		}
		if err != nil {
			fmt.Println(err)
			break
		}

		_, err = conn.Write(data)
		if err != nil {
			fmt.Println(err)
			break
		}
	}
}

func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "1234"
	}
	return port
}

func main() {
	PORT := ":" + getPort()
	l, err := net.Listen("tcp4", PORT)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer l.Close()

	count := 0
	for {
		conn, err := l.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}
		count++
		fmt.Printf("%d: %v <-> %v\n", count, conn.LocalAddr(), conn.RemoteAddr())
		go handleConnection(conn)
	}
}

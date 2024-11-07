package multiplexer

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ingmarstein/tcp-multiplexer/pkg/message"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelInfo)
}

func handleErr(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(2)
	}
}

func client(t *testing.T, server string, clientIndex int) {
	conn, err := net.Dial("tcp", server)
	handleErr(err)
	defer conn.Close()

	for i := 0; i < 10; i++ {
		echo := []byte(fmt.Sprintf("client %d counter %d\n", clientIndex, i))
		_, err = conn.Write(echo)
		handleErr(err)

		echoReply, err := message.EchoMessageReader{}.ReadMessage(conn)
		handleErr(err)

		if bytes.Compare(echo, echoReply) != 0 {
			t.Fatalf("Expected %s, but got %s", echo, echoReply)
		}
	}

	fmt.Println("client connection closed")
}

func handleConnection(conn net.Conn) {
	defer func(c net.Conn) {
		err := c.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(conn)

	for {
		data, err := bufio.NewReader(conn).ReadBytes('\n')
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

func TestMultiplexer_Start(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	go func() {
		defer l.Close()
		for {
			conn, err := l.Accept()
			if err != nil {
				fmt.Println(err)
				break
			}
			go handleConnection(conn)
		}
	}()
	const muxServer = "127.0.0.1:1235"

	mux := New(l.Addr().String(), "1235", message.EchoMessageReader{})

	errChan := make(chan error, 1)
	go func() {
		errChan <- mux.Start()
	}()

	time.Sleep(time.Second)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		client(t, muxServer, 1)
		wg.Done()
	}()
	go func() {
		client(t, muxServer, 2)
		wg.Done()
	}()

	wg.Wait()
	time.Sleep(time.Second)

	select {
	case err := <-errChan:
		if err != nil {
			t.Fatal("Expected no error, but got:", err)
		}
	default:
	}
	err = mux.Close()
	if err != nil {
		t.Fatal("Expected no error, but got:", err)
	}
}

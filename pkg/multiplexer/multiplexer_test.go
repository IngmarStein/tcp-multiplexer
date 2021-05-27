package multiplexer

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/xujiahua/tcp-multiplexer/pkg/message"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

func init() {
	logrus.SetLevel(logrus.InfoLevel)
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

		assert.Equal(t, echo, echoReply)
	}

	fmt.Println("client connection closed")
}

// target server
// cd example/echo-server
// go run main.go
func TestMultiplexer_Start(t *testing.T) {
	const targetServer = "127.0.0.1:1234"
	const muxServer = "127.0.0.1:1235"
	mux := New(targetServer, "1235", message.EchoMessageReader{})

	go func() {
		err := mux.Start()
		assert.Equal(t, nil, err)
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

	err := mux.Close()
	assert.Equal(t, nil, err)
}

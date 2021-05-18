package multiplexer

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/sirupsen/logrus"
	"github.com/xujiahua/tcp-multiplexer/pkg/message"
	"io"
	"net"
)

type responseWrapper struct {
	message []byte
	err     error
}

type Multiplexer struct {
	targetServer  string
	port          string
	requestQueue  chan []byte
	responseQueue chan *responseWrapper
	messageReader message.Reader
	l             net.Listener
}

func New(targetServer, port string, messageReader message.Reader) Multiplexer {
	return Multiplexer{
		targetServer:  targetServer,
		port:          port,
		messageReader: messageReader,
		requestQueue:  make(chan []byte),
		responseQueue: make(chan *responseWrapper),
	}
}

func (mux *Multiplexer) Start() error {
	var err error
	mux.l, err = net.Listen("tcp", ":"+mux.port)
	if err != nil {
		return err
	}

	go mux.targetConnLoop()

	count := 0
	for {
		conn, err := mux.l.Accept()
		if err != nil {
			logrus.Error(err)
			continue
		}
		count++
		logrus.Infof("#%d: %v <-> %v", count, conn.RemoteAddr(), conn.LocalAddr())

		go mux.handleConnection(conn)
	}
}

func (mux Multiplexer) handleConnection(conn net.Conn) {
	defer func(c net.Conn) {
		err := c.Close()
		if err != nil {
			logrus.Errorf("%v", err)
		}
	}(conn)

	for {
		msg, err := mux.messageReader.ReadMessage(conn)
		if err == io.EOF {
			logrus.Infof("closed: %v <-> %v", conn.RemoteAddr(), conn.LocalAddr())
			break
		}
		if err != nil {
			logrus.Errorf("%v", err)
			break
		}

		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			fmt.Println("Message from Client...")
			spew.Dump(msg)
		}

		// enqueue request msg
		mux.requestQueue <- msg

		// dequeue response msg
		responseWrapper, ok := <-mux.responseQueue
		if !ok {
			// channel is closed, server is shutting down
			logrus.Warn("response queue is closed")
			break
		}

		if responseWrapper.err != nil {
			logrus.Errorf("failed to forward message, %v", err)
			break
		}

		// write back
		_, err = conn.Write(responseWrapper.message)
		if err != nil {
			logrus.Errorf("%v", err)
			break
		}
	}
}

func (mux Multiplexer) createTargetConn() net.Conn {
	for {
		logrus.Info("creating target connection")
		conn, err := net.Dial("tcp", mux.targetServer)
		if err != nil {
			logrus.Errorf("failed to connect to target server %s, %v", mux.targetServer, err)
			// TODO: sleep for a while?
			continue
		}

		logrus.Infof("new target connection: %v <-> %v", conn.LocalAddr(), conn.RemoteAddr())
		return conn
	}
}

func (mux Multiplexer) targetConnLoop() {
	conn := mux.createTargetConn()

	for request := range mux.requestQueue {
		_, err := conn.Write(request)
		if err != nil {
			// NOTE: receive 1 request, must send 1 response, in case of mismatch
			mux.responseQueue <- &responseWrapper{
				err: err,
			}

			logrus.Errorf("target connection: %v", err)
			// renew conn
			conn = mux.createTargetConn()
			continue
		}

		msg, err := mux.messageReader.ReadMessage(conn)
		mux.responseQueue <- &responseWrapper{
			message: msg,
			err:     err,
		}

		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			fmt.Println("Message from Target Server...")
			spew.Dump(msg)
		}

		if err != nil {
			logrus.Errorf("target connection: %v", err)
			// renew conn
			conn = mux.createTargetConn()
			continue
		}
	}
}

func (mux Multiplexer) Close() error {
	logrus.Info("closing server...")
	close(mux.requestQueue)
	close(mux.responseQueue)
	return mux.l.Close()
}

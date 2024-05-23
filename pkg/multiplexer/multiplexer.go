package multiplexer

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/ingmarstein/tcp-multiplexer/pkg/message"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"sync"
)

type Multiplexer struct {
	targetServer  string
	port          string
	messageReader message.Reader
	l             net.Listener
	quit          chan struct{}
	wg            *sync.WaitGroup
	requestQueue  chan *reqContainer
}

func New(targetServer, port string, messageReader message.Reader) Multiplexer {
	return Multiplexer{
		targetServer:  targetServer,
		port:          port,
		messageReader: messageReader,
		quit:          make(chan struct{}),
	}
}

func (mux *Multiplexer) Start() error {
	var err error
	mux.l, err = net.Listen("tcp", ":"+mux.port)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	mux.wg = &wg

	sender, receiver, requestQueue := mpscChannel(1)
	mux.requestQueue = requestQueue

	// target connection loop
	go func() {
		mux.targetConnLoop(receiver)
	}()

	count := 0
L:
	for {
		conn, err := mux.l.Accept()
		if err != nil {
			logrus.Error(err)
			select {
			case <-mux.quit:
				logrus.Info("no more connections will be accepted")
				return nil
			default:
				goto L
			}
		}
		count++
		logrus.Infof("#%d: %v <-> %v", count, conn.RemoteAddr(), conn.LocalAddr())

		wg.Add(1)
		go func() {
			mux.handleConnection(conn, sender)
			wg.Done()
		}()
	}
}

func (mux *Multiplexer) handleConnection(conn net.Conn, sender chan<- *reqContainer) {
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
			logrus.Debug("Message from Client...")
			spew.Dump(msg)
		}

		callbackSender, callback := oneshotChannel()

		// enqueue request msg to target conn loop
		sender <- &reqContainer{
			message: msg,
			sender:  callbackSender,
		}

		// get response from target conn loop
		resp := <-callback
		if resp.err != nil {
			logrus.Errorf("failed to forward message, %v", err)
			break
		}

		// write back
		_, err = conn.Write(resp.message)
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

func (mux *Multiplexer) targetConnLoop(requestQueue <-chan *reqContainer) {
	conn := mux.createTargetConn()

	for container := range requestQueue {
		request := container.message
		_, err := conn.Write(request)
		if err != nil {
			container.sender <- &respContainer{
				err: err,
			}

			logrus.Errorf("target connection: %v", err)
			// renew conn
			conn = mux.createTargetConn()
			continue
		}

		msg, err := mux.messageReader.ReadMessage(conn)
		container.sender <- &respContainer{
			message: msg,
			err:     err,
		}

		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			logrus.Debug("Message from Target Server...")
			spew.Dump(msg)
		}

		if err != nil {
			logrus.Errorf("target connection: %v", err)
			// renew conn
			conn = mux.createTargetConn()
			continue
		}
	}

	logrus.Info("target connection write/read loop stopped gracefully")
}

// Close graceful shutdown
func (mux *Multiplexer) Close() error {
	close(mux.quit)
	logrus.Info("closing server...")
	err := mux.l.Close()
	if err != nil {
		return err
	}

	logrus.Debug("wait all incoming connections closed")
	mux.wg.Wait()
	logrus.Info("incoming connections closed")

	// stop target conn loop
	close(mux.requestQueue)

	logrus.Info("multiplexer server stopped gracefully")
	logrus.Info("server is closed gracefully")
	return nil
}

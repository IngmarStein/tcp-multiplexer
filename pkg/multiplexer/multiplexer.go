package multiplexer

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/ingmarstein/tcp-multiplexer/pkg/message"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"sync"
	"time"
)

type (
	messageType int

	reqContainer struct {
		typ     messageType
		message []byte
		sender  chan<- *respContainer
	}

	respContainer struct {
		message []byte
		err     error
	}

	Multiplexer struct {
		targetServer  string
		port          string
		messageReader message.Reader
		l             net.Listener
		quit          chan struct{}
		wg            *sync.WaitGroup
		requestQueue  chan *reqContainer
	}
)

const (
	Connection messageType = iota
	Disconnection
	Packet
)

func deadline() time.Time {
	return time.Now().Add(60 * time.Second)
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

	requestQueue := make(chan *reqContainer, 32)
	mux.requestQueue = requestQueue

	// target connection loop
	go func() {
		mux.targetConnLoop(requestQueue)
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
			mux.handleConnection(conn, requestQueue)
			wg.Done()
		}()
	}
}

func (mux *Multiplexer) handleConnection(conn net.Conn, sender chan<- *reqContainer) {
	defer func(c net.Conn) {
		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			logrus.Debugf("Closing client connection: %v", c.RemoteAddr())
		}
		err := c.Close()
		sender <- &reqContainer{typ: Disconnection}
		if err != nil {
			logrus.Error(err)
		}
	}(conn)

	sender <- &reqContainer{typ: Connection}
	callback := make(chan *respContainer)

	for {
		err := conn.SetReadDeadline(deadline())
		if err != nil {
			logrus.Errorf("error setting read deadline: %v", err)
		}
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
			logrus.Debug("Message from client...")
			spew.Dump(msg)
		}

		// enqueue request msg to target conn loop
		sender <- &reqContainer{
			typ:     Packet,
			message: msg,
			sender:  callback,
		}

		// get response from target conn loop
		resp := <-callback
		if resp.err != nil {
			logrus.Errorf("failed to forward message, %v", err)
			break
		}

		// write back
		err = conn.SetWriteDeadline(deadline())
		if err != nil {
			logrus.Errorf("error setting write deadline: %v", err)
		}
		_, err = conn.Write(resp.message)
		if err != nil {
			logrus.Errorf("%v", err)
			break
		}
	}
}

func (mux *Multiplexer) createTargetConn() net.Conn {
	for {
		logrus.Info("creating target connection")
		conn, err := net.DialTimeout("tcp", mux.targetServer, 30*time.Second)
		if err != nil {
			logrus.Errorf("failed to connect to target server %s, %v", mux.targetServer, err)
			// TODO: make sleep time configurable
			time.Sleep(1 * time.Second)
			continue
		}

		logrus.Infof("new target connection: %v <-> %v", conn.LocalAddr(), conn.RemoteAddr())
		return conn
	}
}

func (mux *Multiplexer) targetConnLoop(requestQueue <-chan *reqContainer) {
	var conn net.Conn
	clients := 0

	for container := range requestQueue {
		switch container.typ {
		case Connection:
			clients++
			if logrus.IsLevelEnabled(logrus.DebugLevel) {
				logrus.Debugf("Connected clients: %d", clients)
			}
			continue
		case Disconnection:
			clients--
			if logrus.IsLevelEnabled(logrus.DebugLevel) {
				logrus.Debugf("Connected clients: %d", clients)
			}
			if clients == 0 && conn != nil {
				logrus.Info("closing target connection")
				err := conn.Close()
				if err != nil {
					logrus.Error(err)
				}
			}
			continue
		case Packet:
			break
		}

		if conn == nil {
			conn = mux.createTargetConn()
		}

		err := conn.SetWriteDeadline(deadline())
		if err != nil {
			logrus.Errorf("error setting write deadline: %v", err)
		}

		_, err = conn.Write(container.message)
		if err != nil {
			container.sender <- &respContainer{
				err: err,
			}

			logrus.Errorf("target connection: %v", err)
			// renew conn
			err = conn.Close()
			if err != nil {
				logrus.Error(err)
			}
			conn = nil
			continue
		}

		err = conn.SetReadDeadline(deadline())
		if err != nil {
			logrus.Errorf("error setting read deadline: %v", err)
		}

		msg, err := mux.messageReader.ReadMessage(conn)
		container.sender <- &respContainer{
			message: msg,
			err:     err,
		}

		if logrus.IsLevelEnabled(logrus.DebugLevel) {
			logrus.Debug("Message from target server...")
			spew.Dump(msg)
		}

		if err != nil {
			logrus.Errorf("target connection: %v", err)
			// renew conn
			err = conn.Close()
			if err != nil {
				logrus.Error(err)
			}
			conn = nil
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

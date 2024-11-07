package multiplexer

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/ingmarstein/tcp-multiplexer/pkg/message"
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
			select {
			case <-mux.quit:
				slog.Info("no more connections will be accepted")
				return nil
			default:
				slog.Error(err.Error())
				goto L
			}
		}
		count++
		slog.Info(fmt.Sprintf("#%d: %v <-> %v", count, conn.RemoteAddr(), conn.LocalAddr()))

		wg.Add(1)
		go func() {
			mux.handleConnection(conn, requestQueue)
			wg.Done()
		}()
	}
}

func (mux *Multiplexer) handleConnection(conn net.Conn, sender chan<- *reqContainer) {
	defer func(c net.Conn) {
		slog.Debug(fmt.Sprintf("Closing client connection: %v", c.RemoteAddr()))
		err := c.Close()
		sender <- &reqContainer{typ: Disconnection}
		if err != nil {
			slog.Error(err.Error())
		}
	}(conn)

	sender <- &reqContainer{typ: Connection}
	callback := make(chan *respContainer)

	for {
		err := conn.SetReadDeadline(deadline())
		if err != nil {
			slog.Error(fmt.Sprintf("error setting read deadline: %v", err))
		}
		msg, err := mux.messageReader.ReadMessage(conn)
		if err == io.EOF {
			slog.Info(fmt.Sprintf("closed: %v <-> %v", conn.RemoteAddr(), conn.LocalAddr()))
			break
		}
		if err != nil {
			slog.Error(err.Error())
			break
		}

		slog.Debug(fmt.Sprintf("Message from client...\n%s", spew.Sdump(msg)))

		// enqueue request msg to target conn loop
		sender <- &reqContainer{
			typ:     Packet,
			message: msg,
			sender:  callback,
		}

		// get response from target conn loop
		resp := <-callback
		if resp.err != nil {
			slog.Error(fmt.Sprintf("failed to forward message, %v", resp.err))
			break
		}

		// write back
		err = conn.SetWriteDeadline(deadline())
		if err != nil {
			slog.Error(fmt.Sprintf("error setting write deadline: %v", err))
		}
		_, err = conn.Write(resp.message)
		if err != nil {
			slog.Error(err.Error())
			break
		}
	}
}

func (mux *Multiplexer) createTargetConn() net.Conn {
	for {
		slog.Info("creating target connection")
		conn, err := net.DialTimeout("tcp", mux.targetServer, 30*time.Second)
		if err != nil {
			slog.Error(fmt.Sprintf("failed to connect to target server %s, %v", mux.targetServer, err))
			// TODO: make sleep time configurable
			time.Sleep(1 * time.Second)
			continue
		}

		slog.Info(fmt.Sprintf("new target connection: %v <-> %v", conn.LocalAddr(), conn.RemoteAddr()))
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
			slog.Info(fmt.Sprintf("Connected clients: %d", clients))
			continue
		case Disconnection:
			clients--
			slog.Info(fmt.Sprintf("Connected clients: %d", clients))
			if clients == 0 && conn != nil {
				slog.Info("closing target connection")
				err := conn.Close()
				if err != nil {
					slog.Error(err.Error())
				}
				conn = nil
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
			slog.Error(fmt.Sprintf("error setting write deadline: %v", err))
		}

		_, err = conn.Write(container.message)
		if err != nil {
			container.sender <- &respContainer{
				err: err,
			}

			slog.Error(fmt.Sprintf("target connection: %v", err))
			// renew conn
			err = conn.Close()
			if err != nil {
				slog.Error(fmt.Sprintf("error while closing connection: %v", err))
			}
			conn = nil
			continue
		}

		err = conn.SetReadDeadline(deadline())
		if err != nil {
			slog.Error(fmt.Sprintf("error setting read deadline: %v", err))
		}

		msg, err := mux.messageReader.ReadMessage(conn)
		container.sender <- &respContainer{
			message: msg,
			err:     err,
		}

		slog.Debug(fmt.Sprintf("Message from target server...\n%s", spew.Sdump(msg)))

		if err != nil {
			slog.Error(fmt.Sprintf("target connection: %v", err))
			// renew conn
			err = conn.Close()
			if err != nil {
				slog.Error(fmt.Sprintf("error while closing connection: %v", err))
			}
			conn = nil
			continue
		}
	}

	slog.Info("target connection write/read loop stopped gracefully")
}

// Close graceful shutdown
func (mux *Multiplexer) Close() error {
	close(mux.quit)
	slog.Info("closing server...")
	err := mux.l.Close()
	if err != nil {
		return err
	}

	slog.Debug("wait all incoming connections closed")
	mux.wg.Wait()
	slog.Info("incoming connections closed")

	// stop target conn loop
	close(mux.requestQueue)

	slog.Info("multiplexer server stopped gracefully")
	slog.Info("server is closed gracefully")
	return nil
}

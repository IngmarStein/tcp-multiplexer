package multiplexer

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

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
		timeout       time.Duration
		delay         time.Duration
		retryDelay    time.Duration
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

func New(targetServer, port string, messageReader message.Reader, delay time.Duration, timeout time.Duration, retryDelay time.Duration) Multiplexer {
	return Multiplexer{
		targetServer:  targetServer,
		port:          port,
		messageReader: messageReader,
		quit:          make(chan struct{}),
		delay:         delay,
		timeout:       timeout,
		retryDelay:    retryDelay,
	}
}

func (mux *Multiplexer) deadline() time.Time {
	return time.Now().Add(mux.timeout)
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
				slog.Error("accept error", "error", err)
				goto L
			}
		}
		count++
		slog.Info("new connection", "id", count, "remote", conn.RemoteAddr(), "local", conn.LocalAddr())

		wg.Add(1)
		go func() {
			mux.handleConnection(conn, requestQueue)
			wg.Done()
		}()
	}
}

func (mux *Multiplexer) handleConnection(conn net.Conn, sender chan<- *reqContainer) {
	defer func(c net.Conn) {
		slog.Debug("closing client connection", "remote", c.RemoteAddr())
		err := c.Close()
		sender <- &reqContainer{typ: Disconnection}
		if err != nil {
			slog.Error("error closing client connection", "error", err)
		}
	}(conn)

	sender <- &reqContainer{typ: Connection}
	callback := make(chan *respContainer)

	for {
		err := conn.SetReadDeadline(mux.deadline())
		if err != nil {
			slog.Error("error setting read deadline", "error", err)
		}
		msg, err := mux.messageReader.ReadMessage(conn)
		if err == io.EOF {
			slog.Info("closed connection", "remote", conn.RemoteAddr(), "local", conn.LocalAddr())
			break
		}
		if err != nil {
			slog.Error("error reading from client", "error", err)
			break
		}

		slog.Debug("message from client", "hex", fmt.Sprintf("%x", msg))

		// enqueue request msg to target conn loop
		sender <- &reqContainer{
			typ:     Packet,
			message: msg,
			sender:  callback,
		}

		// get response from target conn loop
		resp := <-callback
		if resp.err != nil {
			slog.Error("failed to forward message", "error", resp.err)
			break
		}

		// write back
		err = conn.SetWriteDeadline(mux.deadline())
		if err != nil {
			slog.Error("error setting write deadline", "error", err)
		}
		_, err = conn.Write(resp.message)
		if err != nil {
			slog.Error("error writing to client", "error", err)
			break
		}
	}
}

func (mux *Multiplexer) createTargetConn() (net.Conn, error) {
	slog.Info("creating target connection")
	conn, err := net.DialTimeout("tcp", mux.targetServer, mux.timeout)
	if err != nil {
		slog.Error("failed to connect to target server", "server", mux.targetServer, "error", err)
		return nil, err
	}

	slog.Info("new target connection", "local", conn.LocalAddr(), "remote", conn.RemoteAddr())

	if mux.delay > 0 {
		slog.Info("waiting before using new target connection", "delay", mux.delay)
		time.Sleep(mux.delay)
	}

	return conn, nil
}

func (mux *Multiplexer) targetConnLoop(requestQueue <-chan *reqContainer) {
	var conn net.Conn
	clients := 0
	nextRetry := time.Now()
	var lastErr error

	replyTimer := time.NewTimer(time.Second)
	if !replyTimer.Stop() {
		<-replyTimer.C
	}
	defer replyTimer.Stop()

	reply := func(c *reqContainer, r *respContainer) {
		replyTimer.Reset(time.Second)
		select {
		case c.sender <- r:
			if !replyTimer.Stop() {
				select {
				case <-replyTimer.C:
				default:
				}
			}
		case <-replyTimer.C:
			slog.Warn("failed to send response to client", "error", "timeout")
		}
	}

	for container := range requestQueue {
		switch container.typ {
		case Connection:
			clients++
			slog.Info("connected clients", "count", clients)
			continue
		case Disconnection:
			clients--
			slog.Info("connected clients", "count", clients)
			if clients == 0 && conn != nil {
				slog.Info("closing target connection")
				err := conn.Close()
				if err != nil {
					slog.Error("error closing target connection", "error", err)
				}
				conn = nil
			}
			continue
		case Packet:
			break
		}

		if conn == nil {
			if time.Now().Before(nextRetry) {
				reply(container, &respContainer{
					err: lastErr,
				})
				continue
			}

			c, err := mux.createTargetConn()
			if err != nil {
				lastErr = fmt.Errorf("failed to connect to target, entering backoff: %w", err)
				if mux.retryDelay > 0 {
					nextRetry = time.Now().Add(mux.retryDelay)
				}
				reply(container, &respContainer{
					err: lastErr,
				})
				continue
			}
			conn = c
		}

		err := conn.SetWriteDeadline(mux.deadline())
		if err != nil {
			slog.Error("error setting write deadline", "error", err)
		}

		_, err = conn.Write(container.message)
		if err != nil {
			reply(container, &respContainer{
				err: err,
			})

			slog.Error("target connection error during write", "error", err)
			// renew conn
			err = conn.Close()
			if err != nil {
				slog.Error("error while closing connection", "error", err)
			}
			conn = nil
			continue
		}

		err = conn.SetReadDeadline(mux.deadline())
		if err != nil {
			slog.Error("error setting read deadline", "error", err)
		}

		msg, err := mux.messageReader.ReadMessage(conn)
		reply(container, &respContainer{
			message: msg,
			err:     err,
		})

		slog.Debug("message from target server", "hex", fmt.Sprintf("%x", msg))

		if err != nil {
			slog.Error("target connection error during read", "error", err)
			// renew conn
			err = conn.Close()
			if err != nil {
				slog.Error("error while closing connection", "error", err)
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
	slog.Info("closing server")
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

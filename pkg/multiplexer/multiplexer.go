package multiplexer

import (
	"bufio"
	"crypto/tls"
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
		TLSConfig     *tls.Config
		DropTLS       bool
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

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func isTLSClientHello(reader *bufio.Reader) (bool, error) {
	prefix, err := reader.Peek(3)
	if err != nil {
		return false, err
	}
	// Valid Modbus TCP has protocol ID 0, so its third byte cannot match.
	if prefix[0] != 0x16 || prefix[1] != 0x03 || prefix[2] < 0x01 || prefix[2] > 0x04 {
		return false, nil
	}
	header, err := reader.Peek(9)
	if err != nil {
		return false, err
	}
	recordLength := int(header[3])<<8 | int(header[4])
	helloLength := int(header[6])<<16 | int(header[7])<<8 | int(header[8])
	return recordLength >= 4 && recordLength <= 16384 && header[5] == 1 && helloLength >= 41, nil
}

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
		id := count

		wg.Go(func() {
			mux.handleConnection(conn, requestQueue, id)
		})
	}
}

func (mux *Multiplexer) handleConnection(conn net.Conn, sender chan<- *reqContainer, id int) {
	registered := false
	defer func(c net.Conn) {
		slog.Debug("closing client connection", "remote", c.RemoteAddr())
		err := c.Close()
		if registered {
			sender <- &reqContainer{typ: Disconnection}
		}
		if err != nil {
			slog.Error("error closing client connection", "error", err)
		}
	}(conn)

	clientTLS := false
	if mux.TLSConfig != nil || mux.DropTLS {
		if err := conn.SetDeadline(mux.deadline()); err != nil {
			slog.Error("error setting client deadline", "error", err)
			return
		}
		reader := bufio.NewReader(conn)
		tlsHello, err := isTLSClientHello(reader)
		if err != nil {
			if err == io.EOF && reader.Buffered() == 0 {
				slog.Info("closed connection", "remote", conn.RemoteAddr(), "local", conn.LocalAddr())
				return
			}
			slog.Error("error detecting client protocol", "remote", conn.RemoteAddr(), "error", err)
			return
		}
		conn = &bufferedConn{Conn: conn, reader: reader}
		if tlsHello {
			if mux.DropTLS {
				slog.Debug("TLS client connection dropped", "remote", conn.RemoteAddr(), "reason", "TLS drop mode")
				return
			}
			secureConn := tls.Server(conn, mux.TLSConfig)
			if err := secureConn.Handshake(); err != nil {
				slog.Error("TLS handshake failed", "remote", conn.RemoteAddr(), "error", err)
				return
			}
			state := secureConn.ConnectionState()
			slog.Info("TLS handshake completed", "remote", conn.RemoteAddr(),
				"version", fmt.Sprintf("0x%04x", state.Version),
				"cipher", tls.CipherSuiteName(state.CipherSuite),
				"alpn", state.NegotiatedProtocol)
			conn = secureConn
			clientTLS = true
		}
	}

	slog.Info("new connection", "id", id, "remote", conn.RemoteAddr(), "local", conn.LocalAddr())
	sender <- &reqContainer{typ: Connection}
	registered = true
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
		if clientTLS && (len(msg) < 8 || msg[2] != 0 || msg[3] != 0) {
			slog.Error("TLS client sent an invalid Modbus TCP frame", "remote", conn.RemoteAddr())
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

// Close graceful shutdown.
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

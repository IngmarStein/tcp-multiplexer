package multiplexer

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/ingmarstein/tcp-multiplexer/pkg/message"
)

func testTLSCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, roots
}

func TestTLSAndPlaintextModbus(t *testing.T) {
	cert, roots := testTLSCertificate(t)
	tests := []struct {
		name        string
		tls         bool
		untrusted   bool
		invalid     bool
		transaction [2]byte
		drop        bool
		write       bool
	}{
		{name: "plaintext", transaction: [2]byte{0, 1}},
		{name: "plaintext TLS-like transaction ID", transaction: [2]byte{0x16, 0x03}},
		{name: "drop mode plaintext", drop: true, transaction: [2]byte{0, 1}},
		{name: "drop mode TLS-like transaction ID", drop: true, transaction: [2]byte{0x16, 0x03}},
		{name: "drop mode maximum transaction ID", drop: true, transaction: [2]byte{0xff, 0xff}},
		{name: "drop mode write single register", drop: true, write: true, transaction: [2]byte{0x16, 0x03}},
		{name: "TLS", tls: true, transaction: [2]byte{0, 2}},
		{name: "untrusted certificate", tls: true, untrusted: true},
		{name: "TLS non-Modbus protocol ID", tls: true, invalid: true},
		{name: "TLS frame without function code", tls: true, invalid: true, transaction: [2]byte{0, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()

			request := []byte{tt.transaction[0], tt.transaction[1], 0, 0, 0, 6, 4, 3, 0, 1, 0, 1}
			if tt.invalid && tt.transaction[1] == 0 {
				request[3] = 1
			} else if tt.invalid {
				request = []byte{0, 3, 0, 0, 0, 1, 4}
			}
			response := []byte{tt.transaction[0], tt.transaction[1], 0, 0, 0, 5, 4, 3, 2, 0, 42}
			if tt.write {
				request[7] = 6
				response = bytes.Clone(request)
			}
			targetResult := make(chan error, 1)
			go func() {
				c, err := target.Accept()
				if err != nil {
					targetResult <- err
					return
				}
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(5 * time.Second))
				got := make([]byte, len(request))
				if _, err := io.ReadFull(c, got); err != nil {
					targetResult <- err
					return
				}
				if !bytes.Equal(got, request) {
					targetResult <- errors.New("target received unexpected Modbus request")
					return
				}
				_, err = c.Write(response)
				targetResult <- err
			}()

			mux := New(target.Addr().String(), "", message.ModbusMessageReader{}, 0, 5*time.Second, time.Second)
			mux.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
			if tt.drop {
				mux.TLSConfig = nil
				mux.DropTLS = true
			}
			queue := make(chan *reqContainer, 32)
			loopDone := make(chan struct{})
			go func() {
				mux.targetConnLoop(queue)
				close(loopDone)
			}()
			client, server := net.Pipe()
			handlerDone := make(chan struct{})
			go func() {
				mux.handleConnection(server, queue, 1)
				close(handlerDone)
			}()
			defer func() {
				client.Close()
				<-handlerDone
				close(queue)
				<-loopDone
			}()
			_ = client.SetDeadline(time.Now().Add(5 * time.Second))

			var conn net.Conn = client
			if tt.tls {
				trusted := roots
				if tt.untrusted {
					trusted = x509.NewCertPool()
				}
				secure := tls.Client(client, &tls.Config{RootCAs: trusted, ServerName: "localhost", MinVersion: tls.VersionTLS12})
				err := secure.Handshake()
				if tt.untrusted {
					var verifyErr *tls.CertificateVerificationError
					if !errors.As(err, &verifyErr) {
						t.Fatalf("expected a certificate verification error, got %v", err)
					}
					_ = target.Close()
					if err := <-targetResult; err == nil {
						t.Fatal("untrusted client unexpectedly reached the target")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				conn = secure
			}

			if _, err := conn.Write(request[:1]); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Write(request[1:]); err != nil {
				t.Fatal(err)
			}
			if tt.invalid {
				var b [1]byte
				if _, err := conn.Read(b[:]); !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("expected the invalid TLS Modbus frame to be rejected, got %v", err)
				}
				_ = target.Close()
				if err := <-targetResult; err == nil {
					t.Fatal("invalid TLS frame unexpectedly reached the target")
				}
				return
			}
			got := make([]byte, len(response))
			if _, err := io.ReadFull(conn, got); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, response) {
				t.Fatalf("response = %x, want %x", got, response)
			}
			if err := <-targetResult; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTLSAndPlaintextShareTarget(t *testing.T) {
	cert, roots := testTLSCertificate(t)
	for _, tt := range []struct {
		name string
		drop bool
	}{
		{"TLS termination", false},
		{"TLS drop", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()
			targetDone := make(chan error, 1)
			go func() {
				conn, err := target.Accept()
				if err != nil {
					targetDone <- err
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				for i := range 3 {
					request := make([]byte, 12)
					if _, err := io.ReadFull(conn, request); err != nil {
						targetDone <- err
						return
					}
					want := []byte{0x16, 3, 0, 0, 0, 6, 4, 3, 0, byte(i), 0, 1}
					if !bytes.Equal(request, want) {
						targetDone <- errors.New("target received unexpected Modbus request")
						return
					}
					response := []byte{0x16, 3, 0, 0, 0, 5, 4, 3, 2, 0, byte(42 + i)}
					if _, err := conn.Write(response); err != nil {
						targetDone <- err
						return
					}
				}
				targetDone <- nil
			}()

			mux := New(target.Addr().String(), "", message.ModbusMessageReader{}, 0, 5*time.Second, time.Second)
			mux.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
			clientCount := 2
			if tt.drop {
				mux.TLSConfig = nil
				mux.DropTLS = true
				clientCount++
			}
			queue := make(chan *reqContainer, 32)
			loopDone := make(chan struct{})
			go func() {
				mux.targetConnLoop(queue)
				close(loopDone)
			}()
			var clients []net.Conn
			var handlers []chan struct{}
			for i := range clientCount {
				client, server := net.Pipe()
				clients = append(clients, client)
				done := make(chan struct{})
				handlers = append(handlers, done)
				go func() {
					mux.handleConnection(server, queue, i+1)
					close(done)
				}()
				_ = client.SetDeadline(time.Now().Add(5 * time.Second))
			}
			defer func() {
				for _, client := range clients {
					client.Close()
				}
				for _, done := range handlers {
					<-done
				}
				close(queue)
				<-loopDone
			}()
			for i, client := range []net.Conn{clients[0], clients[1], clients[0]} {
				if i == 1 {
					secure := tls.Client(clients[clientCount-1], &tls.Config{
						RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS12,
					})
					err := secure.Handshake()
					if tt.drop {
						if err == nil {
							t.Fatal("TLS handshake unexpectedly succeeded in drop mode")
						}
						<-handlers[clientCount-1]
					} else {
						if err != nil {
							t.Fatal(err)
						}
						client = secure
					}
				}
				request := []byte{0x16, 3, 0, 0, 0, 6, 4, 3, 0, byte(i), 0, 1}
				if _, err := client.Write(request); err != nil {
					t.Fatal(err)
				}
				response := make([]byte, 11)
				if _, err := io.ReadFull(client, response); err != nil {
					t.Fatal(err)
				}
				want := []byte{0x16, 3, 0, 0, 0, 5, 4, 3, 2, 0, byte(42 + i)}
				if !bytes.Equal(response, want) {
					t.Fatalf("response = %x, want %x", response, want)
				}
			}
			if err := <-targetDone; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTLSClientHelloDetection(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"captured Huawei header", []byte{0x16, 3, 1, 1, 0x1c, 1, 0, 1, 0x18}, true},
		{"fragmented handshake record", []byte{0x16, 3, 3, 0, 4, 1, 0, 1, 0x18}, true},
		{"plaintext TLS-like transaction ID", []byte{0x16, 3, 0, 0, 0, 6, 4, 3, 0}, false},
		{"not a ClientHello", []byte{0x16, 3, 3, 0, 45, 2, 0, 0, 41}, false},
		{"oversized record", []byte{0x16, 3, 3, 0x40, 1, 1, 0, 0, 41}, false},
		{"record without handshake header", []byte{0x16, 3, 3, 0, 3, 1, 0, 0, 41}, false},
		{"impossible ClientHello length", []byte{0x16, 3, 3, 0, 45, 1, 0, 0, 40}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(bytes.NewReader(tt.data))
			got, err := isTLSClientHello(reader)
			if err != nil || got != tt.want {
				t.Fatalf("detection = %v, %v; want %v, nil", got, err, tt.want)
			}
			remaining, err := io.ReadAll(reader)
			if err != nil || !bytes.Equal(remaining, tt.data) {
				t.Fatalf("detection consumed or changed bytes: %x, %v", remaining, err)
			}
		})
	}
}

func TestTLSDropFragmentedClientHello(t *testing.T) {
	for _, tt := range []struct {
		name  string
		level slog.Level
	}{
		{"INFO", slog.LevelInfo},
		{"DEBUG", slog.LevelDebug},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: tt.level})))
			t.Cleanup(func() { slog.SetDefault(previous) })
			for _, header := range [][]byte{
				{0x16, 3, 1, 1, 0x1c, 1, 0, 1, 0x18},
				{0x16, 3, 3, 0, 45, 1, 0, 0, 41},
			} {
				mux := New("", "", message.ModbusMessageReader{}, 0, time.Second, time.Second)
				mux.DropTLS = true
				queue := make(chan *reqContainer, 32)
				client, server := net.Pipe()
				done := make(chan struct{})
				go func() {
					mux.handleConnection(server, queue, 1)
					close(done)
				}()
				if err := client.SetDeadline(time.Now().Add(time.Second)); err != nil {
					t.Fatal(err)
				}
				for _, b := range header {
					if _, err := client.Write([]byte{b}); err != nil {
						t.Fatal(err)
					}
				}
				var b [1]byte
				if _, err := client.Read(b[:]); !errors.Is(err, io.EOF) {
					t.Fatalf("expected dropped connection, got %v", err)
				}
				client.Close()
				<-done
				close(queue)
				for event := range queue {
					t.Fatalf("dropped TLS client produced a target event: %v", event.typ)
				}
			}
			if tt.level == slog.LevelInfo && logs.Len() != 0 {
				t.Fatalf("TLS drops produced INFO or higher logs: %s", logs.String())
			}
			if tt.level == slog.LevelDebug && bytes.Count(logs.Bytes(), []byte("TLS client connection dropped")) != 2 {
				t.Fatalf("expected two DEBUG drop records, got: %s", logs.String())
			}
		})
	}
}

func TestTLSDetectionFailureDoesNotRegisterClient(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	mux := New("", "", message.ModbusMessageReader{}, 0, time.Second, time.Second)
	mux.DropTLS = true
	queue := make(chan *reqContainer, 32)
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		mux.handleConnection(server, queue, 1)
		close(done)
	}()
	_ = client.SetDeadline(time.Now().Add(time.Second))
	if _, err := client.Write([]byte{0x16, 3, 1}); err != nil {
		t.Fatal(err)
	}
	client.Close()
	<-done
	close(queue)
	for event := range queue {
		t.Fatalf("unclassified client produced a target event: %v", event.typ)
	}
	if !bytes.Contains(logs.Bytes(), []byte("level=ERROR")) ||
		!bytes.Contains(logs.Bytes(), []byte("error detecting client protocol")) {
		t.Fatalf("missing protocol detection error: %s", logs.String())
	}
}

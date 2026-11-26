/*
Copyright © 2021 xujiahua <littleguner@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/

package cmd

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ingmarstein/tcp-multiplexer/pkg/message"
	"github.com/ingmarstein/tcp-multiplexer/pkg/multiplexer"
	"github.com/spf13/cobra"
)

var (
	port                string
	targetServer        string
	applicationProtocol string
	timeout             int
	delay               time.Duration
	retryDelay          time.Duration
	tlsCertFile         string
	tlsKeyFile          string
	tlsDrop             bool
)

// serverCmd represents the server command.
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "start multiplexer proxy server",
	Run: func(cmd *cobra.Command, args []string) {
		slog.SetLogLoggerLevel(slog.LevelWarn)
		if verbose {
			slog.SetLogLoggerLevel(slog.LevelInfo)
		}
		if debug {
			slog.SetLogLoggerLevel(slog.LevelDebug)
		}

		slog.Info("starting multiplexer",
			"version", version,
			"port", port,
			"targetServer", targetServer,
			"applicationProtocol", applicationProtocol)

		msgReader, ok := message.Readers[applicationProtocol]
		if !ok {
			slog.Error("application protocol is not supported", "protocol", applicationProtocol)
			os.Exit(2)
		}

		mux := multiplexer.New(targetServer, port, msgReader, delay, time.Duration(timeout)*time.Second, retryDelay)
		if applicationProtocol != "modbus" && (tlsDrop || tlsCertFile != "" || tlsKeyFile != "") {
			slog.Error("TLS handling is supported only with --applicationProtocol modbus")
			os.Exit(2)
		}
		if tlsDrop {
			if tlsCertFile != "" || tlsKeyFile != "" {
				slog.Error("--tls-drop cannot be combined with --tls-cert or --tls-key")
				os.Exit(2)
			}
			mux.DropTLS = true
			slog.Info("TLS drop mode enabled")
		}
		if tlsCertFile != "" || tlsKeyFile != "" {
			if tlsCertFile == "" || tlsKeyFile == "" {
				slog.Error("both --tls-cert and --tls-key are required")
				os.Exit(2)
			}
			cert, err := tls.LoadX509KeyPair(tlsCertFile, tlsKeyFile)
			if err != nil {
				slog.Error("failed to load TLS certificate and key", "error", err)
				os.Exit(2)
			}
			mux.TLSConfig = &tls.Config{
				MinVersion:   tls.VersionTLS12,
				Certificates: []tls.Certificate{cert},
				GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
					slog.Info("TLS client hello",
						"remote", hello.Conn.RemoteAddr(),
						"server_name", hello.ServerName,
						"versions", fmt.Sprintf("%#x", hello.SupportedVersions),
						"cipher_suites", fmt.Sprintf("%#x", hello.CipherSuites),
						"signature_schemes", fmt.Sprintf("%v", hello.SignatureSchemes),
						"alpn", hello.SupportedProtos)
					return nil, nil
				},
			}
		}
		go func() {
			err := mux.Start()
			if err != nil {
				slog.Error(err.Error())
				os.Exit(2)
			}
		}()

		signalChan := make(chan os.Signal, 1)
		signal.Notify(
			signalChan,
			syscall.SIGHUP,  // kill -SIGHUP XXXX
			syscall.SIGINT,  // kill -SIGINT XXXX or Ctrl+c
			syscall.SIGQUIT, // kill -SIGQUIT XXXX
		)
		<-signalChan

		err := mux.Close()
		if err != nil {
			slog.Error(err.Error())
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().StringVarP(&port, "listen", "l", "8000", "multiplexer will listen on")
	serverCmd.Flags().StringVarP(&targetServer, "targetServer", "t", "127.0.0.1:1234", "multiplexer will forward message to")
	serverCmd.Flags().StringVarP(&applicationProtocol, "applicationProtocol", "p", "echo", "multiplexer will parse to message echo/http/iso8583/modbus")
	serverCmd.Flags().IntVar(&timeout, "timeout", 60, "timeout in seconds")
	serverCmd.Flags().DurationVar(&delay, "delay", 0, "delay after connect")
	serverCmd.Flags().DurationVar(&retryDelay, "retryDelay", 1*time.Second, "delay before retrying target connection")
	serverCmd.Flags().StringVar(&tlsCertFile, "tls-cert", "", "PEM server certificate for optional TLS clients (modbus only)")
	serverCmd.Flags().StringVar(&tlsKeyFile, "tls-key", "", "PEM server private key for optional TLS clients (modbus only)")
	serverCmd.Flags().BoolVar(&tlsDrop, "tls-drop", false, "close TLS ClientHello connections without certificates; preserve plaintext Modbus TCP")
}

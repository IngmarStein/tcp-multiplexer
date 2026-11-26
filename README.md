# tcp-multiplexer

Use it in front of a target server and let your client programs connect to it, if target server **only allows you to
create a limited number of TCP connections concurrently**. While it has its limitation: increased latency as incoming
request will block each other.

A common use case for tcp-multiplexer is to allow multiple modbus/TCP clients connect to solar inverters which often
only support a single TCP connection.

## Architecture

```
┌──────────┐
│ ┌────────┴──┐      ┌─────────────────┐      ┌───────────────┐
│ │           ├─────►│                 │      │               │
│ │ client(s) │      │ tcp-multiplexer ├─────►│ target server │
└─┤           ├─────►│                 │      │               │
  └───────────┘      └─────────────────┘      └───────────────┘


─────► TCP connection

drawn by https://asciiflow.com/
```

Unlike with a reverse proxy, the TCP connection between `tcp-multiplexer` and the target server will be reused for all
clients' TCP connections.

Multiplexer is simple. For every TCP connection from clients, the handling logic:

```
for {
get lock...
data pipe:
	1. get request message from client
	2. forward request message to target server
	3. get response message from target server
	4. forward response message to client
release lock...
}
```

The lock makes sure that at any time, the TCP connection to the target server will be used in exactly one
request-response loop.
This way, all connections from clients share one TCP connection to the target server.

Next key point is how to detect message (e.g., HTTP) from the TCP data stream.

## Supported application protocols

Every application protocol (request–response message exchange pattern) has its own message format. The following formats
are supported currently:

1. echo: \n terminated
2. http1 (not including https, websocket): not fully supported
3. iso8583: with 2 bytes header of the length of iso8583 message
4. modbus-tcp
5. modbus-rtu: Modbus RTU over TCP (includes CRC)
6. modbus-serial: Raw Modbus RTU (serial) over TCP

```
$ ./tcp-multiplexer list
* iso8583
* echo
* http
* modbus
* modbus-rtu
* modbus-serial

usage for example: ./tcp-multiplexer server -p echo
```

See detailed: https://github.com/ingmarstein/tcp-multiplexer/tree/master/example

## Usage

```
$ ./tcp-multiplexer server -h
start multiplexer proxy server

Usage:
  tcp-multiplexer server [flags]

Flags:
  -p, --applicationProtocol string   multiplexer will parse to message echo/http/iso8583 (default "echo")
      --delay duration               delay after connect
      --retryDelay duration          delay before retrying target connection (default "1s")
  -h, --help                         help for server
  -l, --listen string                multiplexer will listen on (default "8000")
  -t, --targetServer string          multiplexer will forward message to (default "127.0.0.1:1234")
      --timeout int                  timeout in seconds (default 60)

Global Flags:
  -v, --verbose   verbose log
```

#### In a container

```
docker run ghcr.io/ingmarstein/tcp-multiplexer server -t 127.0.0.1:1234 -l 8000 -p modbus
```

Alternatively, use the included `compose.yml` file as a template if you prefer to use Docker Compose.

## TLS handling for Modbus TCP

### Drop unwanted TLS connections without certificates

Enable `--tls-drop` with `-p modbus` to close incoming TLS ClientHello
connections without performing a handshake or forwarding TLS data to the
target. No certificate or private key is needed. A drop logs
`TLS client connection dropped` only at DEBUG level (`-d` / `--debug`), not
at INFO (`-v`). The startup message `TLS drop mode enabled` remains at INFO.
Dropped clients do not produce `new connection` or `connected clients`
messages: registration happens after protocol detection, or after a successful
handshake in termination mode. Only registered clients are unregistered.
Other protocol errors and timeouts remain errors.

Detection peeks at the TLS record and ClientHello headers with the configured
client timeout; it does not consume bytes from plaintext connections. Valid
Modbus TCP uses protocol ID zero, which cannot match the TLS record-version
bytes, even when its transaction ID is `0x1603`. Existing plaintext clients
continue to share the target connection when a TLS client is dropped.
The client may reconnect repeatedly; dropping does not make its TLS service
functional. An incomplete or malformed TLS header is not silently ignored.

`--tls-drop` is opt-in, Modbus-only, and cannot be combined with `--tls-cert`
or `--tls-key`. Without these options, the original plaintext behavior remains.

### Optional TLS termination

The multiplexer can accept plaintext Modbus TCP and TLS-wrapped Modbus TCP on the same
listener. Enable it with both `--tls-cert` and `--tls-key`, pointing to a PEM
server certificate and its private key. TLS is terminated per client; decrypted
Modbus requests use the existing shared plaintext connection to the target.
The minimum TLS version is 1.2. The listener does not authenticate clients with
client certificates, and plaintext connections are still accepted.

At INFO level (`-v`), a parsed ClientHello logs the source, SNI, offered TLS
versions, cipher suites, signature schemes and ALPN protocols. A successful
handshake logs the negotiated version and cipher. Handshake failures and invalid
decrypted Modbus frames are errors. These diagnostics cannot establish which
certificate authorities the client trusts, whether it will accept the chosen
certificate, or which application protocol it sends after the handshake. The
captured Huawei ClientHello does not establish that its application protocol is
Modbus TCP; use this feature only as a test of that hypothesis. If decrypted
data is not Modbus TCP, TLS support will not provide the service the client
expects. Do not send private keys, passwords or full packet captures in logs.

Missing or invalid certificate files cause an explicit startup failure. If the
Wallbox rejects the certificate, inspect its TLS error and the configured trust
on the Wallbox; do not disable certificate validation to force a connection.
Even a successful handshake does not prove that the Wallbox sent Modbus or
that charging, metering or energy-management functionality works.

## Testing

Start echo server (listen on port 1234)

```
$ go run example/echo-server/main.go
1: 127.0.0.1:1234 <-> 127.0.0.1:58088
```

Start TCP multiplexing (listen on port 8000)

```
$ ./tcp-multiplexer server -p echo -t 127.0.0.1:1234 -l 8000
INFO[2021-05-09T02:06:40+08:00] creating target connection
INFO[2021-05-09T02:06:40+08:00] new target connection: 127.0.0.1:58088 <-> 127.0.0.1:1234
INFO[2021-05-09T02:07:57+08:00] #1: 127.0.0.1:58342 <-> 127.0.0.1:8000
INFO[2021-05-09T02:08:16+08:00] closed: 127.0.0.1:58342 <-> 127.0.0.1:8000
INFO[2021-05-09T02:08:19+08:00] #2: 127.0.0.1:58402 <-> 127.0.0.1:8000
```

client test

```
$ nc 127.0.0.1 8000
kkk
kkk
^C
$ nc 127.0.0.1 8000
mmm
mmm
```

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

```
$ ./tcp-multiplexer list                                    
* iso8583
* echo
* http
* modbus

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
      --retry-delay duration         delay before retrying target connection
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

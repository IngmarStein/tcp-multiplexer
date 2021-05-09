
# tcp-multiplexer

Use it in front of target server and let your client programs connect it, if target server **only allows you to create limited tcp connections concurrently**. While it has its limitation: increased latency as incoming request will block each other. Suitable for testing purpose.

## arch

```
┌──────────┐
│ ┌────────┴──┐      ┌─────────────────┐      ┌───────────────┐
│ │           ├─────►│                 │      │               │
│ │ client(s) │      │ tcp-multiplexer ├─────►│ target server │
└─┤           ├─────►│                 │      │               │
  └───────────┘      └─────────────────┘      └───────────────┘


─────► tcp connection

drawn by https://asciiflow.com/
```

Unlike reverse proxy, tcp connection between tcp-multiplexer and target server will be reused for all clients' tcp connections.


Multiplexer is simple. For every tcp connection from clients, the handling logic:

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

lock make sure that at the same time, tcp connection to target server will be used in exactly one request-response loop. That's key point for every connection from clients share one tcp connection to target server.

Next key point is how to detect message (e.g., HTTP Message format) from tcp data stream.

## supported application protocol

Every application protocol has it's own message format. For now, support:

1. echo: \n terminated
2. http1 (not include https, websocket)
3. iso8583: with 2 bytes header of the length of iso8583 message

```
$ ./tcp-multiplexer list                                    
* iso8583
* echo
* http

usage for example: ./tcp-multiplexer server -p echo

```

See detailed: https://github.com/XUJiahua/tcp-multiplexer/tree/master/example

## usage

```
$ ./tcp-multiplexer server -h
start multiplexer proxy server

Usage:
  tcp-multiplexer server [flags]

Flags:
  -p, --applicationProtocol string   multiplexer will parse to message echo/http/iso8583 (default "echo")
  -h, --help                         help for server
  -l, --listen string                multiplexer will listen on (default "8000")
  -t, --targetServer string          multiplexer will forward message to (default "127.0.0.1:1234")

Global Flags:
  -v, --verbose   verbose log
```

### multiplexing echo server

start echo server (listen on port 1234)

```
$ go run example/echo-server/main.go
1: 127.0.0.1:1234 <-> 127.0.0.1:58088
```

start tcp multiplexing (listen on port 8000)

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





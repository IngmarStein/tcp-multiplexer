
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

Unlike reverse proxy, tcp connection between tcp-multiplexer and target server will be reused.

## supported application protocol

1. echo
2. http1 (not include https, websocket)
3. iso8583 (with 2 bytes header of the length of iso8583 message)

See detailed: https://github.com/XUJiahua/tcp-multiplexer/tree/master/example

## usage

```
```

### multiplexing echo server

```

```

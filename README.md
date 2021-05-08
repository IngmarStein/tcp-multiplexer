
# tcp-multiplexer

Useful if target server only allows you to create limited tcp connection concurrently.

While it has its limitation: increased latency as incoming request will block each other.

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

## usage

```
```

### multiplexing echo server

```

```

## Build & Development Commands

- `make build` — build binary (CGO_ENABLED=0)
- `make test` — run all tests (`go test ./...`)
- `make vet` — run `go vet ./...`
- `make fmt` — format code (`go fmt ./...`)
- `make run` — run with HTTP protocol (verbose)
- `make run-8583` — run with ISO 8583 protocol
- `make run-modbus` — run with Modbus protocol
- Run a single test: `go test ./pkg/message/ -run TestName`

## Architecture

TCP multiplexer sits between multiple clients and a single target server, funneling all client connections through one shared TCP connection to the target. A mutex-like request queue serializes request-response pairs so the single target connection is never used concurrently.

### Key packages

- **`cmd/`** — CLI entry points using cobra. Subcommands: `server` (main proxy), `list` (supported protocols), `version`.
- **`pkg/multiplexer/`** — Core proxy logic. `Multiplexer` accepts client connections, runs a single `targetConnLoop` goroutine that owns the target connection, and serializes all forwarding through a `requestQueue` channel. Client connections are handled in separate goroutines that enqueue requests and wait for responses via per-request callback channels.
- **`pkg/message/`** — Protocol-specific message readers. Each implements the `Reader` interface (`ReadMessage(io.Reader) ([]byte, error)` + `Name() string`). Registered in `if.go` init(). Supported protocols: echo, http, iso8583, mpu, modbus (TCP, RTU, serial).

### Adding a new protocol

1. Create a new file in `pkg/message/` implementing the `Reader` interface.
2. Register it in `pkg/message/if.go`'s `init()` function.

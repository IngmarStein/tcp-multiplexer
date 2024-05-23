.phony: fmt run run-8583 run-modbus echo-client http-client-nobody http-client-body http-client-form build

fmt:
	go fmt ./...
run: fmt
	go run main.go server -v -p http
run-8583: fmt
	go run main.go server -v -p iso8583
run-modbus: fmt
	go run main.go server -v -p modbus
echo-client:
	nc 127.0.0.1 8000
http-client-nobody:
	curl -v http://127.0.0.1:8000
http-client-body:
	curl -v -X POST -d '{"name":"bob"}' -H 'Content-Type: application/json' http://127.0.0.1:8000
# TODO: to support
http-client-form:
	curl -v -X POST -F key1=value1 http://127.0.0.1:8000

build: fmt
	go build

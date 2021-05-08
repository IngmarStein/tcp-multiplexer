fmt:
	go fmt ./...
run:fmt
	go run main.go server -v
nc-client:
	nc 127.0.0.1 8000

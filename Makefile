fmt:
	go fmt ./...
run:fmt
	go run main.go server -v -p http
echo-client:
	nc 127.0.0.1 8000
http-client:
	curl http://127.0.0.1:8000
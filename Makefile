fmt:
	go fmt ./...
run:fmt
	go run main.go server -v -p http
echo-client:
	nc 127.0.0.1 8000
http-client-nobody:
	curl -v http://127.0.0.1:8000
http-client-body:
	curl -v -X POST -d '{"name":"bob"}' -H 'Content-Type: application/json' http://127.0.0.1:8000
# TODO: to support
http-client-form:
	curl -v -X POST -F key1=value1 http://127.0.0.1:8000

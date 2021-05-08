
Below is the list of test target servers for each supported application protocol.

## echo

Message pattern: \n terminated.

### echo-server

```
go run main.go
```

### client

```
nc 127.0.0.1 1234
```

## http

Message pattern: HTTP 1.1 PlainText, refer RFC.
![](https://developer.mozilla.org/en-US/docs/Web/HTTP/Messages/httpmsgstructure2.png)

### http-server

```
go run main.go
```

### client

```
curl http://127.0.0.1:1234
```
## iso8583

Message pattern: 2 bytes header with the length of iso8583 message.

| 2 bytes            | M bytes            |
| ------------------ | ------------------ |
| Message Length = M | ISO–8583 Message   |

reference:
https://github.com/kpavlov/jreactive-8583/blob/master/README.md

### dummy-iso8583-server

```
go run main.go
```

### dummy-iso8583-client

```
go run main.go
```

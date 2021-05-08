package main

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"net/http"
	"net/http/httputil"
	"os"
)

func getPort() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "1234"
	}
	return port
}

func headers(w http.ResponseWriter, req *http.Request) {
	dump, err := httputil.DumpRequest(req, true)
	if err != nil {
		fmt.Println(err)
	}
	spew.Dump(dump)

	for name, headers := range req.Header {
		for _, h := range headers {
			fmt.Fprintf(w, "%v: %v\n", name, h)
		}
	}
}

func main() {
	http.HandleFunc("/", headers)

	http.ListenAndServe(":"+getPort(), nil)
}

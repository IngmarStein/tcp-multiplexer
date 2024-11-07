package main

import (
	"fmt"
	"html"
	"net/http"
	"net/http/httputil"
	"os"

	"github.com/davecgh/go-spew/spew"
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html.EscapeString(string(dump))))
}

func main() {
	http.HandleFunc("/", headers)

	port := getPort()
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Printf("Error listening on port %s: %v\n", port, err)
	}
}

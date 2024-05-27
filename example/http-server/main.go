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

			_, err := fmt.Fprintf(w, "%v: %v\n", name, h)
			if err != nil {
				fmt.Printf("Error writing header %s: %v\n", name, err)
				return
			}
		}
	}
}

func main() {
	http.HandleFunc("/", headers)

	port := getPort()
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		fmt.Printf("Error listening on port %s: %v\n", port, err)
	}
}

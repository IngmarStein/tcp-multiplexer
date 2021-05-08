package message

import (
	"bytes"
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"
	"testing"
)

func TestHTTPMessageReader_ReadMessage(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dump, err := httputil.DumpRequest(r, true)
		assert.Equal(t, nil, err)
		fmt.Println(string(dump))

		dump2, err := HTTPMessageReader{}.ReadMessage(bytes.NewReader(dump))
		assert.Equal(t, nil, err)
		// headers may not in same orders
		fmt.Println(string(dump2))
	}))
	defer ts.Close()

	const body = "Go is a general-purpose language designed with systems programming in mind."
	req, err := http.NewRequest("POST", ts.URL, strings.NewReader(body))
	if err != nil {
		log.Fatal(err)
	}
	req.Host = "www.example.org"

	_, err = http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
}

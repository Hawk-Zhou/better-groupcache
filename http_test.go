package geecache

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

var client = &http.Client{
	Timeout: time.Millisecond * 1000,
}

const addr = "http://0.0.0.0:8001/geecache"

func testGet(url string) (bodyText string, code int, err error) {
	rep, err := client.Get(url)
	// log.Println("requesting:", url)
	if err != nil {
		return "", 0, err
	}
	defer rep.Body.Close()

	bytes, err := io.ReadAll(rep.Body)

	bodyText = string(bytes)

	return bodyText, rep.StatusCode, err
}

func TestServer(t *testing.T) {
	println("the following groups exist")

	for key := range groups {
		println(key)
	}

	callbackCount := 0

	NewGroup("g1", 10, GetterFunc(func(key string) ([]byte, error) {
		callbackCount++
		return []byte(key), nil
	}))

	p := NewHTTPPool(8001)

	server := p.NewServer()
	go server.ListenAndServe()
	time.Sleep(time.Millisecond * 50)

	data := []struct {
		params        string
		shouldSuccess bool
		expectCount   int
	}{
		{"", false, 0},
		{"/", false, 0},
		{"//", false, 0},
		{"////", false, 0},
		{"/g1/", false, 0},
		{"//asd", false, 0},
		{"/g2/x", false, 0},
		{"/g1/asdf", true, 1},
		{"/g1/asdf", true, 1},
		// size = maxBytes, evict asdf
		{"/g1/12345", true, 2},
		{"/g1/asdf", true, 3},
		// can't write, exceed maxBytes
		{"/g1/123456", false, 3},
	}

	for _, d := range data {
		t.Run(d.params, func(t *testing.T) {
			var (
				retStr string
				code   int
				err    error
			)

			retStr, code, err = testGet(addr + d.params)

			if err != nil {
				t.Fatalf("Unexpected error %+v", err)
			}

			if d.expectCount != callbackCount {
				fmt.Printf("expecting callback count %d, got %d", callbackCount, d.expectCount)
			}

			if !d.shouldSuccess {
				if code == 200 {
					t.Error("should fail")
				}
			} else {
				if code != 200 {
					t.Error("should success, got:", code)
				}

				if retStr != strings.Split(d.params, "/")[2] {
					t.Error("wrong return body text, got:", retStr)
				}
			}

		})
	}
}

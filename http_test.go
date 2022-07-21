package geecache

import (
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

var testingClient = &http.Client{
	Timeout: time.Millisecond * 1000,
}

const addr = "http://0.0.0.0:8001/geecache"

func testGet(url string) (bodyText string, code int, err error) {
	rep, err := testingClient.Get(url)
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

	wg := sync.WaitGroup{}
	wg.Add(1)
	server := p.NewServer()
	go func() {
		defer wg.Done()
		server.ListenAndServe()
	}()
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
		// callback is called, but result can't be cached, exceed maxBytes
		{"/g1/123456", false, 4},
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
				t.Errorf("expecting callback count %d, got %d", d.expectCount, callbackCount)
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
	if err := server.Shutdown(context.TODO()); err != nil {
		t.Error("Fail to shutdown sever")
	}

	wg.Wait()
}

func TestHTTPGetter_Get(t *testing.T) {

	p := NewHTTPPool(8001)
	getter := &HTTPGetter{
		baseURL: "http://" + p.host + p.basePath,
	}
	if getter.baseURL != "http://0.0.0.0:8001/geecache/" {
		t.Error("url is wrong:", getter.baseURL)
	}

	groupName := "test_httpGetter"
	callbackFlag := false

	NewGroup("test_httpGetter", 10, GetterFunc(func(key string) ([]byte, error) {
		callbackFlag = true
		return []byte(key), nil
	}))

	server := p.NewServer()
	go server.ListenAndServe()
	time.Sleep(time.Millisecond * 50)

	for key := range groups {
		println(key)
	}

	data := []struct {
		group        string
		key          string
		wantErr      bool
		wantCallback bool
	}{
		{"notexist", "123", true, false},
		{groupName, "1234", false, true},
		{groupName, "1234", false, false},
		{groupName, "19191", false, true},
		{groupName, "114", false, true},
		{groupName, "1145141919", true, true},
	}
	for _, d := range data {
		t.Run(d.key, func(t *testing.T) {
			defer func() {
				callbackFlag = false
			}()
			got, err := getter.Get(d.group, d.key)
			if (err != nil) != d.wantErr {
				t.Errorf("HTTPGetter.Get() error = %v, wantErr %v", err, d.wantErr)
				return
			}
			if d.wantCallback != callbackFlag {
				t.Errorf("HTTPGetter.Get() callbackFlag = %v, want %v", callbackFlag, d.wantCallback)
				return
			}
			if !d.wantErr && !reflect.DeepEqual(got, []byte(d.key)) {
				t.Errorf("HTTPGetter.Get() = %v, want %v", string(got), d.key)
			}
		})
	}
}

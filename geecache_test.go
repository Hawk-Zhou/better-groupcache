package geecache

import (
	"fmt"
	"log"
	"os/exec"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestGetterCallback(t *testing.T) {
	var callback Getter = GetterFunc(func(key string) ([]byte, error) {
		return []byte(key), nil
	})

	quote := "random word"
	expectRet := []byte(quote)
	if v, _ := callback.Get(quote); !reflect.DeepEqual(v, expectRet) {
		t.Error("getter is a bad callback")
	}
}

func TestGroupWR(t *testing.T) {
	dbStub := map[string][]byte{
		"hello": []byte("world"),
		"my":    []byte("pace"),
	}

	refCount := new(int)
	getGenerator := func() (ret Getter) {
		ret = GetterFunc(func(key string) ([]byte, error) {
			(*refCount)++
			fmt.Printf("[Getter Called] Loading %v from slow storage, count %v\n", key, *refCount)
			return []byte(dbStub[key]), nil
		})
		return ret
	}

	getter := getGenerator()

	g := NewGroup("G1", len("hello"+"world"), getter)
	g.RegisterPeers(NewHTTPPool(19623))

	testdata := []struct {
		name          string
		key           string
		output        []byte
		expectedCount int
		err           error
	}{
		{"populate hello", "hello", dbStub["hello"], 1, nil},
		{"hit hello", "hello", dbStub["hello"], 1, nil},
		{"hit hello", "hello", dbStub["hello"], 1, nil},
		{"populate my", "my", dbStub["my"], 2, nil},
		{"hit my", "my", dbStub["my"], 2, nil},
		{"repopulate hello", "hello", dbStub["hello"], 3, nil},
	}

	for _, d := range testdata {
		t.Run(d.name, func(t *testing.T) {
			ret, err := g.Get(d.key)
			if err != nil {
				t.Error("unexpected error")
			}
			if *refCount != d.expectedCount || !reflect.DeepEqual(ret.Get(), d.output) {
				t.Errorf("expecting incr'ed = %d / ret val = %s, got %d/%s", d.expectedCount, d.output, *refCount, string(ret.Get()))
			}
		})
	}

}

func TestGetFromRemotePool(t *testing.T) {
	localGroup := NewGroup("getRemoteGroup", 10, nil)
	localPool := NewHTTPPool(4970)
	localGroup.RegisterPeers(localPool)

	go func() {
		cmd := exec.Command("./script.sh")
		stdout, err := cmd.Output()

		if err != nil {
			fmt.Println(err.Error())
			return
		}

		// Print the output
		fmt.Println(string(stdout))
	}()

	// server can't setup in less than 800ms
	time.Sleep(1000 * time.Millisecond)

	localGroup.AddPeers("http://0.0.0.0:4971/geecache/")
	localPool.RemovePeers("http://" + localPool.host + localPool.basePath)

	log.Println("getting from remote")
	ret, err := localGroup.Get("114")

	if err != nil {
		t.Errorf("can't get from remote: +%v", err)
	}

	if !reflect.DeepEqual(ret.b, []byte("remote")) {
		t.Errorf("wrong ret: %v", string(ret.b))
	}

}

func TestIntegrationSingleFlight(t *testing.T) {

	refCount := 0
	getGenerator := func() (ret Getter) {
		ret = GetterFunc(func(key string) ([]byte, error) {
			refCount++
			time.Sleep(500 * time.Millisecond)
			fmt.Printf("[Getter Called] Loading %v from slow storage, count %v\n", key, refCount)
			return []byte(key), nil
		})
		return ret
	}

	getter := getGenerator()

	g := NewGroup("G1", len("hello"+"world"), getter)
	g.RegisterPeers(NewHTTPPool(19623))

	testdata := []struct {
		name          string
		key           string
		expectedCount int
		err           error
	}{
		{"hello0", "hello", 1, nil},
		{"hello1", "hello", 1, nil},
		{"hello2", "hello", 1, nil},
		{"hello3", "hello", 1, nil},
		{"hello4", "hello", 1, nil},
	}

	wg := sync.WaitGroup{}
	for _, d := range testdata {
		d := d
		wg.Add(1)
		go func() {
			log.Printf("test %v start", d.name)
			ret, err := g.load(d.key)
			if err != nil {
				t.Error("unexpected error")
			}
			if refCount != d.expectedCount || !reflect.DeepEqual(ret.Get(), []byte(d.key)) {
				t.Errorf("expecting incr'ed = %d / ret val = %s, got %d/%s", d.expectedCount, d.key, refCount, string(ret.Get()))
			}
			wg.Done()
		}()
	}

	wg.Wait()
}

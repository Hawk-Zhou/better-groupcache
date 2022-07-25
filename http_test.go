package geecache

import (
	"context"
	"io"
	"log"
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

func TestServerAbnormalRequest(t *testing.T) {
	println("the following groups exist")

	for key := range groups {
		println(key)
	}

	callbackCount := 0

	g := NewGroup("g1", 10, GetterFunc(func(key string) ([]byte, error) {
		callbackCount++
		return []byte(key), nil
	}))

	p := NewHTTPPool(8001)
	g.RegisterPeers(p)

	wg := sync.WaitGroup{}
	wg.Add(1)
	server := p.NewServer()
	go func() {
		defer wg.Done()
		log.Println("trying to init server at", server.Addr)
		server.ListenAndServe()
	}()
	time.Sleep(time.Millisecond * 200)

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

	g := NewGroup("test_httpGetter", 10, GetterFunc(func(key string) ([]byte, error) {
		callbackFlag = true
		return []byte(key), nil
	}))
	g.RegisterPeers(p)

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
		{"notExist", "123", true, false},
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

func TestHTTPPool_PeerOp(t *testing.T) {
	count := 0
	g := NewGroup("peerOp", 10, nil)
	localPool := NewHTTPPool(8001)
	g.RegisterPeers(localPool)

	localPool.AddPeers() // actually already implicitly added itself when registering peers
	_, ok := localPool.PickPeer("114")
	if ok {
		t.Error("should omit itself")
	}
	err := localPool.RemovePeers("http://" + localPool.host + localPool.basePath)
	if err != nil {
		t.Error("itself should exist and be removable")
	}

	err = localPool.RemovePeers("http://" + localPool.host + localPool.basePath)
	if err == nil {
		t.Error("itself doesn't exist and should err")
	}

	remotePool := NewHTTPPool(8002)
	remoteSever := remotePool.NewServer()
	// We have to use a new group instead of use the above "peerOp"
	// The localPool is registered to peerOp. To make sure it request from remote,
	// itself is removed from its peers. So any query it got will be directed to remote.
	// If remote use localPool, then the query will be given to localPool to choose a peer to answer.
	// Boom bang, now it loops forever.
	remoteGroup := NewGroup("remoteG", 10, GetterFunc(func(key string) ([]byte, error) {
		count++
		return []byte(key), nil
	}))
	remoteGroup.RegisterPeers(remotePool)
	localPool.AddPeers("http://" + remotePool.host + remotePool.basePath)
	localPool.RemovePeers("http://" + localPool.host + localPool.basePath)
	go func() {
		log.Print("init server", remoteSever.Addr)
		err := remoteSever.ListenAndServe()
		if err != nil {
			log.Printf("can't init server,err:%+v", err)
		}

	}()

	time.Sleep(200 * time.Millisecond)

	pGetter, _ := localPool.PickPeer("114")

	ret, err := pGetter.Get("remoteG", "114")

	if !reflect.DeepEqual(ret, []byte("114")) ||
		err != nil ||
		count != 1 {
		t.Errorf("can't get key properly from remote, ret,err,count are %v,%v,%v", string(ret), err, count)
	}
}

func Test_remoteManagePeers(t *testing.T) {
	g := NewGroup("remoteMgtPurgePeers", 10, nil)
	localPool := NewHTTPPool(4584)
	g.RegisterPeers(localPool)

	server := localPool.NewServer()
	go server.ListenAndServe()
	time.Sleep(time.Millisecond * 50)

	peerAddr := "http://0.0.0.0:11451/geecache" // to be added and then removed
	// add peer
	localPool.AddPeers(peerAddr)

	// check peer is added
	_, ok := localPool.httpGetters[peerAddr]
	if !ok {
		t.Error("unexpected error, the node isn't added as peer")
	}

	// remotely instruct removal of the peer
	err := localPool.RemovePeerRemote("http://"+localPool.host+localPool.basePath, peerAddr)
	if err != nil {
		t.Errorf("%v", err)
	}

	// check remote removal success
	_, ok = localPool.httpGetters[peerAddr]
	if ok {
		t.Error("unexpected error, the node isn't removed")
	}

	// check error (remotely remove nonexistent node)
	err = localPool.RemovePeerRemote("http://"+localPool.host+localPool.basePath, peerAddr)
	if err == nil {
		t.Errorf("should err cuz removing nonexistent node")
	}

	// check peer is not there before remotely add
	_, ok = localPool.httpGetters[peerAddr]
	if ok {
		t.Error("unexpected error, the node shouldn't be there")
	}

	// remotely add
	err = localPool.AddPeerRemote("http://"+localPool.host+localPool.basePath, peerAddr)
	if err != nil {
		t.Error("unexpected error, remotely add peer failed")
		println(err.Error())
	}

	// check peer is there after remotely add
	_, ok = localPool.httpGetters[peerAddr]
	if !ok {
		t.Error("unexpected error, the node should be added")
	}

	// check wrong parameter refusal
	if localPool.RemovePeerRemote("addr") == nil ||
		localPool.AddPeerRemote("addr") == nil {
		t.Error("should deny empty peer list")
	}
}

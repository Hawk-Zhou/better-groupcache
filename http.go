package geecache

import (
	"errors"
	"fmt"
	"geecache/consistentHash"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultBasePath = "/geecache/"

// sharedClient is http.Client that has timeout set properly
var sharedClient = &http.Client{
	Timeout: time.Millisecond * 200,
}

type HTTPGetter struct {
	baseURL string // "http://0.0.0.0:8000/geecache/"
}

func (hg *HTTPGetter) Get(group string, key string) ([]byte, error) {
	url := fmt.Sprintf(hg.baseURL+"%v/%v", group, key)

	resp, err := sharedClient.Get(url)
	if err != nil {
		return nil, err
	}
	// otherwise memory will leak
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		if err != nil {
			return nil, fmt.Errorf("another error happened when handling statusCode(%v) from response:%w",
				resp.StatusCode,
				err)
		}
		return nil, errors.New(resp.Status + ": " + string(body))
	}

	return body, nil
}

// trick to validate a struct implements an interface properly
var _ PeerGetter = (*HTTPGetter)(nil)

type HTTPPool struct {
	host        string // "ip:port"
	basePath    string // "/pathname/"
	mu          sync.Mutex
	peers       *consistentHash.CHash
	httpGetters map[string]*HTTPGetter
}

func NewHTTPPool(port int) *HTTPPool {
	return &HTTPPool{
		host:        "0.0.0.0:" + fmt.Sprint(port),
		basePath:    defaultBasePath,
		peers:       consistentHash.NewCHash(nil),
		httpGetters: make(map[string]*HTTPGetter),
	}
}

func (p *HTTPPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if !strings.HasPrefix(path, p.basePath) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf("bad Pathname: %v \n", path)))
		return
	}
	params := strings.SplitN(path[len(p.basePath):], "/", 3)
	if len(params) != 2 {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("wrong number of parameter (group/key)\n"))
		return
	} else if params[0] == "" || params[1] == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("group name / key should be not null\n"))
		return
	}
	// log.Printf("[HTTPPool/ServeHttp] trying to access %s:%s", params[0], params[1])
	g, ok := GetGroup(params[0])
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("group name doesn't exist\n"))
		return
	}
	ret, err := g.Get(params[1])
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error() + "\n"))
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(ret.Get())
}

func (p *HTTPPool) NewServer() *http.Server {
	server := newHttpServer(p.host, p)
	return server
}

// newHttpServer returns a http.Server that handles queries
// run Server.ListenAndServe in a goroutine, or it blocks
func newHttpServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         addr,
		ReadTimeout:  20 * time.Second,
		IdleTimeout:  120 * time.Second,
		WriteTimeout: 20 * time.Second,
		Handler:      handler,
	}
}

// AddPeers set peers of this format:
//  "http://0.0.0.0:8000/geecache/"
// * also register itself automatically
// * idempotent operation
func (p *HTTPPool) AddPeers(peers ...string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	peers = append(peers, "http://"+p.host+p.basePath)

	for _, peer := range peers {
		if _, ok := p.peers.NameToSalt[peer]; ok {
			continue
		}

		err := p.peers.AddNode(peer)

		if err != nil {
			return fmt.Errorf("can't add the peer %s: %w", peer, err)
		}

		p.httpGetters[peer] = &HTTPGetter{
			baseURL: peer,
		}
	}

	return nil
}

// RemovePeers remove a set of peers:
// format:"http://0.0.0.0:8000/geecache/"
// * Not idempotent
// * Errs if not exist
func (p *HTTPPool) RemovePeers(peers ...string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, peer := range peers {
		err := p.peers.RemoveNode(peer)

		if err != nil {
			return fmt.Errorf("can't remove peers: +%w", err)
		}

		delete(p.httpGetters, peer)
	}

	return nil
}

// PickPeer return a peer if peer is valid (not "")
// and is not the caller itself.
// * Return false is no peer exists.
func (p *HTTPPool) PickPeer(query string) (PeerGetter, bool) {

	peer := p.peers.FindNode(query)

	if "http://"+p.host+p.basePath == peer {
		return nil, false
	}

	pGetter, valid := p.httpGetters[peer]
	return pGetter, valid
}

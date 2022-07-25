package geecache

import (
	"bytes"
	"errors"
	"fmt"
	"geecache/consistentHash"
	pb "geecache/geecachepb"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
)

type httpCtxKey string

const defaultBasePath = "/geecache/"

// sharedClient is http.Client that has timeout set properly
var sharedClient = &http.Client{
	Timeout: time.Millisecond * 200,
}

type HTTPGetter struct {
	baseURL string // "http://0.0.0.0:8000/geecache/"
}

func (hg *HTTPGetter) Get(group string, key string) ([]byte, error) {

	requestPb := &pb.Request{}
	requestPb.Type = pb.Request_ISQUERY
	queryPb := &pb.Request_Query{Group: group, Key: key}
	requestPb.Body = &pb.Request_Query_{Query: queryPb}

	// url := fmt.Sprintf(hg.baseURL+"%v/%v", group, key)

	marshalledReq, err := proto.Marshal(requestPb)

	if err != nil {
		return nil, fmt.Errorf("http.Get can't marshal: %w", err)
	}

	resp, err := sharedClient.Post(hg.baseURL,
		"application/octet-stream",
		bytes.NewReader(marshalledReq))
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

// NewHTTPPool should be initialized with AddPeers
func NewHTTPPool(port int) *HTTPPool {
	return &HTTPPool{
		host:        "0.0.0.0:" + fmt.Sprint(port),
		basePath:    defaultBasePath,
		peers:       consistentHash.NewCHash(nil),
		httpGetters: make(map[string]*HTTPGetter),
	}
}

// signal a remote peer to remove its peers
func (p *HTTPPool) removePeerRemote(remoteURL string, peers ...string) error {
	requestPb := &pb.Request{}
	requestPb.Type = pb.Request_ISMANAGE
	managePb := &pb.Request_Manage{Op: pb.Request_Manage_PURGE, Node: peers}
	requestPb.Body = &pb.Request_Manage_{Manage: managePb}

	// url := fmt.Sprintf(hg.baseURL+"%v/%v", group, key)

	marshalledReq, err := proto.Marshal(requestPb)

	if err != nil {
		return fmt.Errorf("http.Get can't marshal: %w", err)
	}

	resp, err := sharedClient.Post(remoteURL,
		"application/octet-stream",
		bytes.NewReader(marshalledReq))
	if err != nil {
		return err
	}
	// otherwise memory will leak
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		if err != nil {
			return fmt.Errorf("another error happened when handling statusCode(%v) from response:%w",
				resp.StatusCode,
				err)
		}
		return errors.New(resp.Status + ": " + string(body))
	}

	return nil
}

func (p *HTTPPool) answerQuery(group string, key string, w http.ResponseWriter, r *http.Request) {

	if group == "" || key == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("group name / key should be not null\n"))
		return
	}

	g, ok := GetGroup(group)
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("group name doesn't exist\n"))
		return
	}
	ret, err := g.Get(key)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error() + "\n"))
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(ret.Get())
}

// answerMgtPurgePeers remove peers on request
// It returns 200 if all removal are successful
// If any fails, it returns InternalSeverError and a list of nodes failed to remove in the body
func (p *HTTPPool) answerMgtPurgePeers(peers []string, w http.ResponseWriter, r *http.Request) {
	var (
		total      = len(peers)
		success    = 0
		fail       = 0
		failedtoRm = make([]string, 0, total)
		err        error
	)

	for _, peer := range peers {
		err = p.RemovePeers(peer)
		if err != nil {
			failedtoRm = append(failedtoRm, peer+":"+err.Error())
			fail++
		} else {
			success++
		}
	}

	if fail > 0 {
		w.WriteHeader(http.StatusInternalServerError)
		builder := strings.Builder{}
		builder.WriteString(fmt.Sprintf("%d/%d nodes deleted, the rest failed\n", success, total))
		builder.WriteString(strings.Join(failedtoRm, "\n"))
		builder.WriteRune('\n')
		w.Write([]byte(builder.String()))
		return
	}
	w.WriteHeader(200)
}

func (p *HTTPPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if !strings.HasPrefix(path, p.basePath) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(fmt.Sprintf("bad Pathname: %v \n", path)))
		return
	}

	reqBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("[http.ServeHTTP] unexpected error: %v\n", err)
		return
	}
	requestPb := &pb.Request{}
	err = proto.Unmarshal(reqBytes, requestPb)
	if err != nil {
		log.Printf("[http.ServeHTTP] can't unmarshal: %v\n", err)
		return
	}

	reqTypePb := requestPb.GetType()
	if reqTypePb == pb.Request_ISQUERY {
		query := requestPb.GetQuery()
		if query == nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(fmt.Sprintf("bad request.query (got nil after unmarshal): %v \n", path)))
			return
		}
		log.Printf("got query %+v,%+v\n", query.Group, query.Key)
		p.answerQuery(query.Group, query.Key, w, r)
		return
	}

	if reqTypePb == pb.Request_ISMANAGE {
		manage := requestPb.GetManage()
		if manage == nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(fmt.Sprintf("bad request.manage (got nil after unmarshal): %v \n", path)))
			return
		}
		log.Printf("got manage %+v,%+v\n", manage.Op, manage.Node)
		if manage.Op == pb.Request_Manage_PURGE {
			p.answerMgtPurgePeers(manage.Node, w, r)
		}
	}
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
		// errs if not exist
		err := p.peers.RemoveNode(peer)

		if err != nil {
			return fmt.Errorf("can't remove peers: +%w", err)
		}

		delete(p.httpGetters, peer)
	}

	return nil
}

// PickPeer returns a peer if peer is valid (not "")
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

var _ PeerPicker = (*HTTPPool)(nil)

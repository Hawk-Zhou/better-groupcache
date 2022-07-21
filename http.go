package geecache

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const defaultBasePath = "/geecache/"

type HTTPPool struct {
	host     string // "ip:port"
	basePath string // "/pathname/"
}

func NewHTTPPool(port int) *HTTPPool {
	return &HTTPPool{
		host:     "0.0.0.0:" + fmt.Sprint(port),
		basePath: defaultBasePath,
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
	server := newHttpServer(p)
	return server
}

// newHttpServer returns a http.Server that handles queries
// run Server.ListenAndServe in a goroutine, or it blocks
func newHttpServer(handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         "0.0.0.0:8001",
		ReadTimeout:  20 * time.Second,
		IdleTimeout:  120 * time.Second,
		WriteTimeout: 20 * time.Second,
		Handler:      handler,
	}
}

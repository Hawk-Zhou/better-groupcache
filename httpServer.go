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
	r.Body.Close()
	if !strings.HasPrefix(path, p.basePath) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad Pathname\n"))
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

func (p *HTTPPool) RunServer() (*http.Server, error) {
	server := newHttpServer(p)
	err := server.ListenAndServe()
	return server, err
}

// As no timeOut (using default)
// is very dangerous as per the book.
// It's necessary to provide one
func newHttpServer(handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         "0.0.0.0:8001",
		ReadTimeout:  20 * time.Second,
		IdleTimeout:  120 * time.Second,
		WriteTimeout: 20 * time.Second,
		Handler:      handler,
	}
}

// func main() {
// 	_ = NewGroup("g1", 10, GetterFunc(func(key string) ([]byte, error) { return []byte(key), nil }))
// 	p := NewHTTPPool(8001)
// 	_, err := p.RunServer()
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	//mux := http.NewServeMux()
// 	// mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
// 	// 	r.Body.Close()
// 	// 	log.Println(r.URL.Path)
// 	// 	w.WriteHeader(200)
// 	// 	w.Write([]byte("ACK\n"))
// 	// })

// 	// server := NewHttpServer(mux)
// 	// err := server.ListenAndServe()

// }

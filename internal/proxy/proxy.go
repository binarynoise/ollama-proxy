package proxy

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"ollama-proxy/internal/proxy/interceptor"
	"ollama-proxy/internal/tracker"
)

// Proxy represents an HTTP reverse proxy that can intercept and track specific requests
type Proxy struct {
	target      *url.URL
	proxy       *httputil.ReverseProxy
	interceptor *interceptor.Interceptor
}

// NewProxy creates a new Proxy instance
func NewProxy(target string, tracker *tracker.CallTracker) (*Proxy, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	p := &Proxy{
		target:      targetURL,
		interceptor: interceptor.NewInterceptor(tracker),
	}

	// Initialize the reverse proxy
	p.proxy = &httputil.ReverseProxy{
		Director:       p.director,
		ModifyResponse: p.modifyResponse,
		ErrorHandler:   p.errorHandler,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}

	return p, nil
}

// ServeHTTP handles incoming HTTP requests
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if p.interceptor.ShouldIntercept(r) {
		fw, req, callID := p.interceptor.InterceptRequest(w, r)
		p.proxy.ServeHTTP(fw, req)
		p.interceptor.CompleteCall(callID)
		return
	}

	// Proxy the request without interception
	p.proxy.ServeHTTP(w, r)
}

// director modifies the request to be sent to the target
func (p *Proxy) director(req *http.Request) {
	targetQuery := p.target.RawQuery
	req.URL.Scheme = p.target.Scheme
	req.URL.Host = p.target.Host
	req.URL.Path = singleJoiningSlash(p.target.Path, req.URL.Path)

	switch {
	case targetQuery == "" || req.URL.RawQuery == "":
		req.URL.RawQuery = targetQuery + req.URL.RawQuery
	default:
		req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
	}

	if _, ok := req.Header["User-Agent"]; !ok {
		req.Header.Set("User-Agent", "")
	}
}

// modifyResponse can be used to modify the response before it's sent to the client
func (p *Proxy) modifyResponse(resp *http.Response) error {
	// Response modification logic would go here
	return nil
}

// errorHandler handles proxy errors
func (p *Proxy) errorHandler(w http.ResponseWriter, r *http.Request, err error) {
	log.Printf("http: proxy error: %v", err)
	w.WriteHeader(http.StatusBadGateway)
}

// singleJoiningSlash joins two URL paths with a single slash
func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

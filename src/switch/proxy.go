package _switch

import (
	"encoding/base64"
	"github.com/armon/go-socks5"
	"github.com/danieldin95/openlan-go/src/cli/config"
	"github.com/danieldin95/openlan-go/src/libol"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type HttpProxy struct {
	Listen string
	Users  map[string]string
	Logger *libol.SubLogger
}

var (
	connectOkay = []byte("HTTP/1.1 200 Connection established\r\n\r\n")
)

func parseBasicAuth(auth string) (username, password string, ok bool) {
	const prefix = "Basic "
	// Case insensitive prefix match. See Issue 22736.
	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return
	}
	c, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return
	}
	cs := string(c)
	s := strings.IndexByte(cs, ':')
	if s < 0 {
		return
	}
	return cs[:s], cs[s+1:], true
}

func NewHttpProxy(addr string) *HttpProxy {
	return &HttpProxy{
		Listen: addr,
		Logger: libol.NewSubLogger(addr),
	}
}

func (t *HttpProxy) isAuth(username, password string) bool {
	if p, ok := t.Users[username]; ok {
		return p == password
	}
	return false
}

func (t *HttpProxy) CheckAuth(w http.ResponseWriter, r *http.Request) bool {
	if len(t.Users) == 0 {
		return true
	}
	auth := r.Header.Get("Proxy-Authorization")
	user, password, ok := parseBasicAuth(auth)
	if !ok || !t.isAuth(user, password) {
		w.Header().Set("Proxy-Authenticate", "Basic")
		http.Error(w, "Proxy Authentication Required", http.StatusProxyAuthRequired)
		return false
	}
	return true
}

func (t *HttpProxy) route(w http.ResponseWriter, p *http.Response) {
	defer p.Body.Close()
	for key, value := range p.Header {
		if key == "Proxy-Authorization" {
			if len(value) > 0 { // Pop first value for next proxy.
				value = value[1:]
			}
		}
		for _, v := range value {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(p.StatusCode)
	_, _ = io.Copy(w, p.Body)
}

func (t *HttpProxy) tunnel(w http.ResponseWriter, conn net.Conn) {
	src, bio, err := w.(http.Hijacker).Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer src.Close()
	t.Logger.Info("HttpProxy.tunnel %s -> %s", src.RemoteAddr(), conn.RemoteAddr())
	wait := libol.NewWaitOne(2)
	libol.Go(func() {
		defer wait.Done()
		// The returned bufio.Reader may contain unprocessed buffered data from the client.
		// Copy them to dst so we can use src directly.
		if n := bio.Reader.Buffered(); n > 0 {
			n64, err := io.CopyN(conn, bio, int64(n))
			if n64 != int64(n) || err != nil {
				t.Logger.Warn("HttpProxy.tunnel io.CopyN:", n64, err)
				return
			}
		}
		if _, err := io.Copy(conn, src); err != nil {
			t.Logger.Debug("HttpProxy.tunnel from ws %s", err)
		}
	})
	libol.Go(func() {
		defer wait.Done()
		if _, err := io.Copy(src, conn); err != nil {
			t.Logger.Debug("HttpProxy.tunnel from target %s", err)
		}
	})
	wait.Wait()
	t.Logger.Debug("HttpProxy.tunnel %s exit", conn.RemoteAddr())
}

func (t *HttpProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.Logger.Debug("HttpProxy.ServeHTTP %v", r)
	t.Logger.Debug("HttpProxy.ServeHTTP %v", r.URL.Host)
	if !t.CheckAuth(w, r) {
		t.Logger.Info("HttpProxy.ServeHTTP Required %v Authentication", r.URL.Host)
		return
	}
	if r.Method == "CONNECT" { //RFC-7231 Tunneling TCP based protocols through Web Proxy servers
		conn, err := net.Dial("tcp", r.URL.Host)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		_, _ = w.Write(connectOkay)
		t.tunnel(w, conn)
	} else { //RFC 7230 - HTTP/1.1: Message Syntax and Routing
		transport := &http.Transport{}
		p, err := transport.RoundTrip(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer transport.CloseIdleConnections()
		t.route(w, p)
	}
}

type TcpProxy struct {
	Listen   string
	Target   string
	Listener net.Listener
	Logger   *libol.SubLogger
}

func NewTcpProxy(listen, target string) *TcpProxy {
	return &TcpProxy{
		Listen: listen,
		Target: target,
		Logger: libol.NewSubLogger(listen),
	}
}

func (t *TcpProxy) tunnel(src net.Conn, dst net.Conn) {
	defer dst.Close()
	defer src.Close()
	t.Logger.Info("TcpProxy.tunnel %s -> %s", src.RemoteAddr(), dst.RemoteAddr())
	wait := libol.NewWaitOne(2)
	libol.Go(func() {
		defer wait.Done()
		if _, err := io.Copy(dst, src); err != nil {
			t.Logger.Debug("TcpProxy.tunnel from ws %s", err)
		}
	})
	libol.Go(func() {
		defer wait.Done()
		if _, err := io.Copy(src, dst); err != nil {
			t.Logger.Debug("TcpProxy.tunnel from target %s", err)
		}
	})
	wait.Wait()
	t.Logger.Debug("TcpProxy.tunnel %s exit", dst.RemoteAddr())
}

func (t *TcpProxy) Start() {
	var listen net.Listener
	promise := &libol.Promise{
		First:  time.Second * 2,
		MaxInt: time.Minute,
		MinInt: time.Second * 10,
	}
	promise.Done(func() error {
		var err error
		listen, err = net.Listen("tcp", t.Listen)
		if err != nil {
			t.Logger.Warn("TcpProxy.Start %s", err)
		}
		return err
	})
	t.Listener = listen
	t.Logger.Info("TcpProxy.Start: %s", t.Target)
	libol.Go(func() {
		defer listen.Close()
		for {
			conn, err := listen.Accept()
			if err != nil {
				t.Logger.Error("TcpServer.Accept: %s", err)
				break
			}
			// connect target and pipe it.
			target, err := net.Dial("tcp", t.Target)
			if err != nil {
				t.Logger.Error("TcpProxy.Accept %s", err)
				continue
			}
			libol.Go(func() {
				t.tunnel(conn, target)
			})
		}
	})
	return
}

func (t *TcpProxy) Stop() {
	if t.Listener != nil {
		t.Listener.Close()
	}
	t.Logger.Info("TcpProxy.Stop")
	t.Listener = nil
}

type Proxy struct {
	cfg   *config.Proxy
	tcp   []*TcpProxy
	socks *socks5.Server
	http  *http.Server
}

func NewProxy(cfg *config.Proxy) *Proxy {
	return &Proxy{
		cfg: cfg,
	}
}

func (p *Proxy) initSocks() {
	if p.cfg.Socks == nil || p.cfg.Socks.Listen == "" {
		return
	}
	// Create a SOCKS5 server
	auth := p.cfg.Socks.Auth
	authMethods := make([]socks5.Authenticator, 0, 2)
	if len(auth.Username) > 0 {
		author := socks5.UserPassAuthenticator{
			Credentials: socks5.StaticCredentials{
				auth.Username: auth.Password,
			},
		}
		authMethods = append(authMethods, author)
	}
	conf := &socks5.Config{
		AuthMethods: authMethods,
	}
	server, err := socks5.New(conf)
	if err != nil {
		libol.Error("Proxy.initSocks %s", err)
		return
	}
	p.socks = server
}

func (p *Proxy) initHttp() {
	if p.cfg.Http == nil || p.cfg.Http.Listen == "" {
		return
	}
	addr := p.cfg.Http.Listen
	auth := p.cfg.Http.Auth
	hp := NewHttpProxy(addr)
	if len(auth.Username) > 0 {
		hp.Users = make(map[string]string, 1)
		hp.Users[auth.Username] = auth.Password
	}
	p.http = &http.Server{
		Addr:    addr,
		Handler: hp,
	}
}

func (p *Proxy) initTcp() {
	if len(p.cfg.Tcp) == 0 {
		return
	}
	p.tcp = make([]*TcpProxy, 0, 32)
	for _, c := range p.cfg.Tcp {
		p.tcp = append(p.tcp, NewTcpProxy(c.Listen, c.Target))
	}
}

func (p *Proxy) startSocks() {
	if p.cfg.Socks == nil || p.socks == nil {
		return
	}
	addr := p.cfg.Socks.Listen
	libol.Info("Proxy.startSocks %s", addr)
	libol.Go(func() {
		promise := &libol.Promise{
			First:  time.Second * 2,
			MaxInt: time.Minute,
			MinInt: time.Second * 10,
		}
		promise.Done(func() error {
			if err := p.socks.ListenAndServe("tcp", addr); err != nil {
				libol.Warn("Proxy.startSocks %s", err)
				return err
			}
			return nil
		})
	})
}

func (p *Proxy) startTcp() {
	if len(p.tcp) == 0 {
		return
	}
	for _, t := range p.tcp {
		t.Start()
	}
}

func (p *Proxy) Initialize() {
	if p.cfg == nil {
		return
	}
	p.initSocks()
	p.initHttp()
	p.initTcp()
}

func (p *Proxy) startHttp() {
	if p.http == nil {
		return
	}
	libol.Info("Proxy.startHttp %s", p.http.Addr)
	libol.Go(func() {
		defer p.http.Shutdown(nil)
		promise := &libol.Promise{
			First:  time.Second * 2,
			MaxInt: time.Minute,
			MinInt: time.Second * 10,
		}
		promise.Done(func() error {
			if err := p.http.ListenAndServe(); err != nil {
				libol.Warn("Proxy.startHttp %s", err)
				return err
			}
			return nil
		})
	})
}

func (p *Proxy) Start() {
	if p.cfg == nil {
		return
	}
	libol.Info("Proxy.Start")
	libol.Go(p.startTcp)
	libol.Go(p.startSocks)
	libol.Go(p.startHttp)
}

func (p *Proxy) stopTcp() {
	if len(p.tcp) == 0 {
		return
	}
	for _, t := range p.tcp {
		t.Stop()
	}
}

func (p *Proxy) Stop() {
	if p.cfg == nil {
		return
	}
	libol.Info("Proxy.Stop")
	p.stopTcp()
}

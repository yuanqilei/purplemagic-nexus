// playergateway exposes only player account APIs and release files. Database,
// SSH and management credentials never enter the player distribution.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"dnf90.local/gateway/internal/management"
	"dnf90.local/gateway/internal/modules/dnf/adminapi"
	"dnf90.local/gateway/internal/modules/dnf/logincodes"
	"dnf90.local/gateway/internal/services/launcherhub"
)

type config struct {
	QQBotListen     string              `json:"qq_bot_listen,omitempty"`
	QQBotToken      string              `json:"qq_bot_token,omitempty"`
	Management      *management.Config  `json:"management,omitempty"`
	Listen          string              `json:"listen"`
	Certificate     string              `json:"certificate"`
	Key             string              `json:"key"`
	Upstream        string              `json:"upstream"`
	ProxySecret     string              `json:"proxy_secret"`
	ReleaseDir      string              `json:"release_dir"`
	BootstrapListen string              `json:"bootstrap_listen"`
	BootstrapTarget string              `json:"bootstrap_target"`
	Hub             *launcherhub.Config `json:"launcher_hub,omitempty"`
}

var releaseFile = regexp.MustCompile(`^/launcher/(settings\.json|release\.json|ServerS5A3-[0-9.]+\.zip|PurpleMagic\.exe(?:\.sha256)?|DNF90-Portable-[0-9]+\.[0-9]+\.[0-9]+\.exe(?:\.sha256)?)$`)

func handler(c config) http.Handler {
	return handlerWithHub(c, nil)
}

func handlerWithHub(c config, hub *launcherhub.Hub) http.Handler {
	return handlerWithCodes(c, hub, nil)
}

type registrationReservation struct { id uint64; account, ip string; store *logincodes.Store }
type reservationContextKey struct{}

func handlerWithCodes(c config, hub *launcherhub.Hub, codes *logincodes.Store) http.Handler {
	u, err := url.Parse(c.Upstream)
	if err != nil {
		panic(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	original := proxy.Director
	proxy.Director = func(r *http.Request) {
		original(r)
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		r.Header.Del("X-Forwarded-For")
		r.Header.Del("Forwarded")
		r.Header.Del("Cookie")
		r.Header.Del("Authorization")
		r.Header.Set("X-Player-IP", ip)
		r.Header.Set("X-Player-Proxy-Key", c.ProxySecret)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		reservation, _ := resp.Request.Context().Value(reservationContextKey{}).(*registrationReservation)
		if reservation == nil { return nil }
		// The account API reports business failures in HTTP 200 JSON, so only
		// consume when its explicit {"ok":true} acknowledgement is present.
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10+1))
		resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), resp.Body))
		var result struct { OK bool `json:"ok"` }
		acknowledged := readErr == nil && len(body) <= 64<<10 && json.Unmarshal(body, &result) == nil && result.OK
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && acknowledged {
			return reservation.store.Finish(resp.Request.Context(), reservation.id, reservation.account, "player", reservation.ip, true)
		}
		if resp.StatusCode >= 400 && resp.StatusCode < 500 || (resp.StatusCode >= 200 && resp.StatusCode < 300 && readErr == nil && len(body) <= 64<<10 && json.Unmarshal(body, &result) == nil && !result.OK) {
			return reservation.store.Finish(resp.Request.Context(), reservation.id, reservation.account, "player", reservation.ip, false)
		}
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "service unavailable", http.StatusBadGateway)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.RawQuery != "" {
			http.NotFound(w, r)
			return
		}
		switch {
		case hub != nil && (strings.HasPrefix(r.URL.Path, "/launcher/v2/") || strings.HasPrefix(r.URL.Path, "/blobs/")):
			hub.PublicHandler().ServeHTTP(w, r)
		case r.Method == "POST" && r.URL.Path == "/api/account/register":
			if codes == nil { playerError(w,http.StatusServiceUnavailable,"registration_unavailable","注册码服务暂不可用"); return }
			r.Body = http.MaxBytesReader(w,r.Body,4096)
			body,err:=io.ReadAll(r.Body); if err!=nil {playerError(w,http.StatusBadRequest,"invalid_request","注册请求无效");return}
			var payload map[string]json.RawMessage
			if json.Unmarshal(body,&payload)!=nil {playerError(w,http.StatusBadRequest,"invalid_request","注册请求无效");return}
			var code,account string
			if json.Unmarshal(payload["code"],&code)!=nil || json.Unmarshal(payload["account"],&account)!=nil {playerError(w,http.StatusBadRequest,"invalid_code","请输入有效注册码");return}
			ip,_,_:=net.SplitHostPort(r.RemoteAddr)
			id,err:=codes.Reserve(r.Context(),code,account,"player",ip)
			if err!=nil {
				if strings.Contains(err.Error(),"注册码") || err.Error()=="账号无效" { playerError(w,http.StatusForbidden,"invalid_code",err.Error()) } else { playerError(w,http.StatusServiceUnavailable,"registration_unavailable","注册码服务暂不可用") }
				return
			}
			delete(payload,"code"); forwarded,err:=json.Marshal(payload); if err!=nil {playerError(w,http.StatusBadRequest,"invalid_request","注册请求无效");return}
			r.Body=io.NopCloser(bytes.NewReader(forwarded)); r.ContentLength=int64(len(forwarded)); r.Header.Set("Content-Length",strconv.Itoa(len(forwarded)))
			r=r.WithContext(context.WithValue(r.Context(),reservationContextKey{},&registrationReservation{id:id,account:account,ip:ip,store:codes}))
			proxy.ServeHTTP(w,r)
		case r.Method == "POST" && (r.URL.Path == "/api/account/login" || r.URL.Path == "/api/account/password" || r.URL.Path == "/api/account/binding-code" || r.URL.Path == "/api/diagnostics/context"):
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			proxy.ServeHTTP(w, r)
		case r.Method == "GET" && r.URL.Path == "/api/status":
			proxy.ServeHTTP(w, r)
		case (r.Method == "GET" || r.Method == "HEAD") && releaseFile.MatchString(r.URL.Path):
			path := filepath.Join(c.ReleaseDir, filepath.Base(r.URL.Path))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() {
				http.NotFound(w, r)
				return
			}
			if strings.HasSuffix(r.URL.Path, ".exe") {
				w.Header().Set("Content-Disposition", `attachment; filename="PurpleMagic.exe"`)
			}
			http.ServeFile(w, r, path)
		default:
			http.NotFound(w, r)
		}
	})
}

func playerError(w http.ResponseWriter,status int,code,message string) { w.Header().Set("Content-Type","application/json; charset=utf-8"); w.WriteHeader(status); _=json.NewEncoder(w).Encode(map[string]any{"ok":false,"error":code,"message":message}) }

func bootstrap(ctx context.Context, listen, target string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(listen)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsPrivate() {
		return nil, errors.New("bootstrap must bind a private address")
	}
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return nil, err
	}
	context.AfterFunc(ctx, func() { ln.Close() })
	slots := make(chan struct{}, 64)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			select {
			case slots <- struct{}{}:
				go func() {
					defer func() { <-slots }()
					defer conn.Close()
					stop := context.AfterFunc(ctx, func() { conn.Close() })
					defer stop()
					conn.SetDeadline(time.Now().Add(5 * time.Minute))
					remote, err := net.DialTimeout("tcp", target, 5*time.Second)
					if err != nil {
						return
					}
					defer remote.Close()
					stopRemote := context.AfterFunc(ctx, func() { remote.Close() })
					defer stopRemote()
					done := make(chan struct{})
					go func() { io.Copy(remote, conn); remote.(*net.TCPConn).CloseWrite(); close(done) }()
					io.Copy(conn, remote)
					conn.Close()
					<-done
				}()
			default:
				conn.Close()
			}
		}
	}()
	return ln, nil
}

func main() {
	path := flag.String("config", "gateway.json", "private deployment configuration")
	initSchema := flag.Bool("init-management-schema", false, "initialize additive management tables and exit")
	flag.Parse()
	raw, err := os.ReadFile(*path)
	if err != nil {
		log.Fatal(err)
	}
	var c config
	if err = json.Unmarshal(raw, &c); err != nil {
		log.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *initSchema {
		if c.Management == nil {
			log.Fatal("management configuration required")
		}
		if err = management.InitializeSchema(ctx, *c.Management); err != nil {
			log.Fatal(err)
		}
		log.Print("management schema ready")
		return
	}
	if err = runGateway(ctx, c); err != nil {
		log.Fatal(err)
	}
}

func privateHandler(hub http.Handler, module http.Handler, gmHub ...http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/internal/hub/") || strings.HasPrefix(r.URL.Path, "/api/v1/hub/") {
			if len(gmHub) > 0 && gmHub[0] != nil {
				gmHub[0].ServeHTTP(w, r)
			} else {
				http.NotFound(w, r)
			}
			return
		}
		if module != nil && (r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/console/") || strings.HasPrefix(r.URL.Path, "/api/")) {
			module.ServeHTTP(w, r)
			return
		}
		if hub != nil {
			hub.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}

func runGateway(ctx context.Context, c config) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if c.Listen == "" && c.Management == nil {
		return errors.New("configure a player or management listener")
	}
	if c.Listen != "" && len(c.ProxySecret) < 32 {
		return errors.New("missing player proxy secret")
	}
	if c.Management != nil {
		if err := c.Management.Validate(); err != nil {
			return err
		}
		if c.Management.Listen == c.Listen {
			return errors.New("player and management listeners must differ")
		}
		if c.Hub != nil && c.Management.Listen != c.Hub.AdminListen {
			return errors.New("Hub and management must share the private listener")
		}
	}
	certificate, err := tls.LoadX509KeyPair(c.Certificate, c.Key)
	if err != nil {
		return errors.New("gateway TLS certificate unavailable")
	}
	var hub *launcherhub.Hub
	if c.Hub != nil {
		hub, err = launcherhub.New(*c.Hub)
		if err != nil {
			return errors.New("launcher Hub configuration invalid")
		}
	}
	var module *management.Module
	if c.Management != nil {
		module, err = management.Open(ctx, *c.Management, func(msg string, args ...any) { log.Println(append([]any{msg}, args...)...) })
		if err != nil {
			return err
		}
		defer module.Close()
	}
	if c.BootstrapListen != "" {
		listener, e := bootstrap(ctx, c.BootstrapListen, c.BootstrapTarget)
		if e != nil {
			return e
		}
		defer listener.Close()
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{certificate}}
	var servers []*http.Server
	if c.Listen != "" {
		var codes *logincodes.Store; if module!=nil { codes=module.LoginCodes }
		servers = append(servers, &http.Server{Addr: c.Listen, Handler: handlerWithCodes(c, hub, codes), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 6 * time.Hour, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192, TLSConfig: tlsConfig})
	}
	if c.QQBotListen != "" {
		host, _, e := net.SplitHostPort(c.QQBotListen)
		ip := net.ParseIP(host)
		if e != nil || ip == nil || !ip.IsLoopback() || c.Management == nil || len(c.QQBotToken) < 32 {
			return errors.New("QQ bot listener requires loopback, management and a separate token")
		}
		client, e := adminapi.NewClient(c.Management.GameAPI, c.Management.GameToken)
		if e != nil {
			return e
		}
		servers = append(servers, &http.Server{Addr: c.QQBotListen, Handler: withQQBot(http.NotFoundHandler(), client, c.QQBotToken), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, MaxHeaderBytes: 4096})
	}
	var gmHub http.Handler
	if hub != nil && c.Management != nil && c.Management.Catalog != nil {
		gmHub = hub.GMHandler(c.Management.ServiceToken, c.Management.PVFSHA256)
	} else if hub != nil && module != nil {
		gmHub = hub.ConsoleHandler(module.HubSessionGuard)
	}
	var admin http.Handler
	var api http.Handler
	var listen string
	if hub != nil {
		admin = hub.AdminHandler()
		listen = c.Hub.AdminListen
	}
	if module != nil {
		api = module.Handler
		if c.QQBotToken != "" {
			if len(c.QQBotToken) < 32 {
				return errors.New("QQ bot token requires at least 32 characters")
			}
			client, e := adminapi.NewClient(c.Management.GameAPI, c.Management.GameToken)
			if e != nil {
				return e
			}
			api = withQQBot(api, client, c.QQBotToken)
		}
		listen = c.Management.Listen
	}
	if listen != "" {
		servers = append(servers, &http.Server{Addr: listen, Handler: privateHandler(admin, api, gmHub), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Minute, WriteTimeout: 2 * time.Minute, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192, TLSConfig: tlsConfig})
	}
	errorsCh := make(chan error, len(servers))
	defer func() {
		for _, server := range servers {
			shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if server.Shutdown(shutdown) != nil {
				server.Close()
			}
			cancel()
		}
	}()
	for _, server := range servers {
		go func() {
			if server.TLSConfig == nil {
				errorsCh <- server.ListenAndServe()
			} else {
				errorsCh <- server.ListenAndServeTLS("", "")
			}
		}()
	}
	select {
	case <-ctx.Done():
		return nil
	case err = <-errorsCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("gateway listener stopped")
	}
}

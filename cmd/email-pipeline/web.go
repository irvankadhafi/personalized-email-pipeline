package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

const (
	defaultWebListen = "127.0.0.1:8080"
	maxWebBody       = 4 << 10
	webShutdownLimit = 5 * time.Second
)

//go:embed web/*
var webFiles embed.FS

func webCommand(args []string, _ io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("web", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	listen := flags.String("listen", defaultWebListen, "loopback listen address")
	if flags.Parse(args) != nil || flags.NArg() != 0 {
		return staticError(stderr, "invalid_configuration")
	}
	address, err := loopbackListenAddress(*listen)
	if err != nil {
		return staticError(stderr, "guard_refused")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return staticError(stderr, "web_unavailable")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := serveWeb(ctx, listener, newWebHandler()); err != nil {
		return staticError(stderr, "web_unavailable")
	}
	return 0
}

func serveWeb(ctx context.Context, listener net.Listener, handler http.Handler) error {
	boundAuthority, err := listenerAuthority(listener.Addr())
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler: hostAuthorityHandler{authority: boundAuthority, next: handler}, ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8 << 10,
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		if lifecycle, ok := server.Handler.(interface{ shutdown() }); ok {
			lifecycle.shutdown()
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), webShutdownLimit)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if err := <-result; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

type boundAuthority struct {
	ip   net.IP
	port uint16
}

type hostAuthorityHandler struct {
	authority boundAuthority
	next      http.Handler
}

func (h hostAuthorityHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	host, port, err := net.SplitHostPort(request.Host)
	portNumber, portErr := strconv.ParseUint(port, 10, 16)
	ip := net.ParseIP(host)
	if err != nil || portErr != nil || ip == nil || uint16(portNumber) != h.authority.port || !equalLiteralIP(ip, h.authority.ip) {
		http.Error(writer, "forbidden", http.StatusForbidden)
		return
	}
	h.next.ServeHTTP(writer, request)
}

func equalLiteralIP(left, right net.IP) bool {
	if left4, right4 := left.To4(), right.To4(); left4 != nil && right4 != nil {
		return left4.Equal(right4)
	}
	return left.To16().Equal(right.To16())
}

func (h hostAuthorityHandler) shutdown() {
	if lifecycle, ok := h.next.(interface{ shutdown() }); ok {
		lifecycle.shutdown()
	}
}

func listenerAuthorityFromString(address string) (boundAuthority, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return boundAuthority{}, errors.New("invalid listener authority")
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return boundAuthority{}, errors.New("invalid listener port")
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return boundAuthority{}, errors.New("listener authority must be a literal IP")
	}
	return boundAuthority{ip: ip, port: uint16(portNumber)}, nil
}

func listenerAuthority(address net.Addr) (boundAuthority, error) {
	return listenerAuthorityFromString(address.String())
}

func loopbackListenAddress(address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", errors.New("invalid listen address")
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return "", errors.New("invalid listen port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", errors.New("listen address must be a literal loopback IP")
	}
	return address, nil
}

package quic

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
)

type eventCollector struct {
	mu     sync.Mutex
	events []Event
}

func (c *eventCollector) Log(e Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *eventCollector) Has(component, status string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.events {
		if e.Component == component && e.Status == status {
			return true
		}
	}
	return false
}

func TestIntegrationHTTP3WithSelfSignedCert(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")
	if err := writeSelfSignedPair(certFile, keyFile); err != nil {
		t.Fatalf("failed to generate cert pair: %v", err)
	}

	addr := freeUDPAddr(t)
	collector := &eventCollector{}

	s := New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}), Config{
		Addr:     addr,
		CertFile: certFile,
		KeyFile:  keyFile,
		Logger:   collector.Log,
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.StartHTTP3()
	}()

	tr := &http3.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h3"}},
	}
	defer tr.Close()
	client := &http.Client{Transport: tr, Timeout: 2 * time.Second}

	var lastErr error
	url := "https://" + addr + "/healthz"
	for i := 0; i < 40; i++ {
		resp, err := client.Get(url)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if string(body) != "ok" {
				t.Fatalf("unexpected body: %q", string(body))
			}
			lastErr = nil
			break
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("failed HTTP/3 request: %v", lastErr)
	}

	if err := s.ShutdownGraceful(2 * time.Second); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("start returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server exit")
	}

	if !collector.Has("h3", "started") {
		t.Fatal("expected h3 started event")
	}
	if !collector.Has("h3", "stopped") {
		t.Fatal("expected h3 stopped event")
	}
}

func TestDualStackStartupShutdown(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")
	if err := writeSelfSignedPair(certFile, keyFile); err != nil {
		t.Fatalf("failed to generate cert pair: %v", err)
	}

	udpAddr := freeUDPAddr(t)
	tcpAddr := freeTCPAddr(t)
	collector := &eventCollector{}

	s := New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}), Config{
		Addr:      udpAddr,
		HTTP1Addr: tcpAddr,
		CertFile:  certFile,
		KeyFile:   keyFile,
		Logger:    collector.Log,
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.StartDualAndServe()
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	url := "http://" + tcpAddr + "/healthz"
	for i := 0; i < 40; i++ {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			lastErr = nil
			break
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("http/1 endpoint not reachable: %v", lastErr)
	}

	if err := s.ShutdownGraceful(2 * time.Second); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("dual-stack returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for dual-stack exit")
	}

	if !collector.Has("server", "started") {
		t.Fatal("expected server started event")
	}
	if !collector.Has("server", "stopped") {
		t.Fatal("expected server stopped event")
	}
}

func TestErrorPathPortInUse(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "server.crt")
	keyFile := filepath.Join(dir, "server.key")
	if err := writeSelfSignedPair(certFile, keyFile); err != nil {
		t.Fatalf("failed to generate cert pair: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve TCP port: %v", err)
	}
	defer ln.Close()

	s := New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), Config{
		Addr:      freeUDPAddr(t),
		HTTP1Addr: ln.Addr().String(),
		CertFile:  certFile,
		KeyFile:   keyFile,
	})

	err = s.StartDualAndServe()
	if err == nil {
		t.Fatal("expected port-in-use error")
	}
	if !strings.Contains(err.Error(), "h1 server error") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestErrorPathBadCert(t *testing.T) {
	dir := t.TempDir()
	certFile := filepath.Join(dir, "bad.crt")
	keyFile := filepath.Join(dir, "bad.key")
	if err := os.WriteFile(certFile, []byte("not a cert"), 0o600); err != nil {
		t.Fatalf("failed writing cert file: %v", err)
	}
	if err := os.WriteFile(keyFile, []byte("not a key"), 0o600); err != nil {
		t.Fatalf("failed writing key file: %v", err)
	}

	s := New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), Config{
		Addr:     freeUDPAddr(t),
		CertFile: certFile,
		KeyFile:  keyFile,
	})

	if err := s.StartHTTP3(); err == nil {
		t.Fatal("expected bad cert error")
	}
}

func freeTCPAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate free TCP addr: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func freeUDPAddr(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate free UDP addr: %v", err)
	}
	addr := pc.LocalAddr().String()
	_ = pc.Close()
	return addr
}

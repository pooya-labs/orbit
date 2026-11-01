package postgres

import (
	"context"
	"io"
	"net"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"
)

type testDBTunnel struct {
	client   net.Conn
	upstream net.Conn
	once     sync.Once
}

func (t *testDBTunnel) close() {
	t.once.Do(func() {
		_ = t.client.Close()
		_ = t.upstream.Close()
	})
}

type testDBProxy struct {
	listener    net.Listener
	upstream    string
	mu          sync.Mutex
	online      bool
	tunnels     map[*testDBTunnel]struct{}
	workers     sync.WaitGroup
	closeOnce   sync.Once
}

func newTestDBProxy(t *testing.T, upstream string) *testDBProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for PostgreSQL test proxy: %v", err)
	}
	proxy := &testDBProxy{
		listener: listener,
		upstream: upstream,
		online:   true,
		tunnels:  make(map[*testDBTunnel]struct{}),
	}
	proxy.workers.Add(1)
	go proxy.accept()
	t.Cleanup(proxy.close)
	return proxy
}

func (p *testDBProxy) address() string {
	return p.listener.Addr().String()
}

func (p *testDBProxy) accept() {
	defer p.workers.Done()
	for {
		client, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.workers.Add(1)
		go func() {
			defer p.workers.Done()
			p.handle(client)
		}()
	}
}

func (p *testDBProxy) handle(client net.Conn) {
	p.mu.Lock()
	online := p.online
	p.mu.Unlock()
	if !online {
		_ = client.Close()
		return
	}

	upstream, err := net.DialTimeout("tcp", p.upstream, 2*time.Second)
	if err != nil {
		_ = client.Close()
		return
	}
	tunnel := &testDBTunnel{client: client, upstream: upstream}
	p.mu.Lock()
	if !p.online {
		p.mu.Unlock()
		tunnel.close()
		return
	}
	p.tunnels[tunnel] = struct{}{}
	p.mu.Unlock()

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, client)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		done <- struct{}{}
	}()
	<-done
	tunnel.close()

	p.mu.Lock()
	delete(p.tunnels, tunnel)
	p.mu.Unlock()
}

func (p *testDBProxy) setOnline(online bool) {
	p.mu.Lock()
	p.online = online
	tunnels := make([]*testDBTunnel, 0, len(p.tunnels))
	if !online {
		for tunnel := range p.tunnels {
			tunnels = append(tunnels, tunnel)
		}
	}
	p.mu.Unlock()
	for _, tunnel := range tunnels {
		tunnel.close()
	}
}

func (p *testDBProxy) activeTunnels() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.tunnels)
}

func (p *testDBProxy) close() {
	p.closeOnce.Do(func() {
		p.setOnline(false)
		_ = p.listener.Close()
		p.workers.Wait()
	})
}

func TestRepositoryRecoversAfterPostgresNetworkDisconnect(t *testing.T) {
	databaseURL := os.Getenv("ORBIT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("ORBIT_TEST_DATABASE_URL is not set")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	if parsed.Host == "" {
		t.Skip("test database URL does not use a TCP host")
	}
	proxy := newTestDBProxy(t, parsed.Host)
	proxiedURL := *parsed
	proxiedURL.Host = proxy.address()

	openCtx, openCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer openCancel()
	repository, err := Open(openCtx, proxiedURL.String())
	if err != nil {
		t.Fatalf("open repository through TCP proxy: %v", err)
	}
	t.Cleanup(repository.Close)

	// Warm and return a real pooled connection so the outage closes an
	// established PostgreSQL session, not only a not-yet-used dial attempt.
	warmCtx, warmCancel := context.WithTimeout(context.Background(), 5*time.Second)
	conn, err := repository.pool.Acquire(warmCtx)
	if err != nil {
		warmCancel()
		t.Fatalf("acquire PostgreSQL pool connection: %v", err)
	}
	if err := conn.Ping(warmCtx); err != nil {
		conn.Release()
		warmCancel()
		t.Fatalf("warm PostgreSQL pool connection: %v", err)
	}
	conn.Release()
	warmCancel()
	if proxy.activeTunnels() == 0 {
		t.Fatal("expected at least one established PostgreSQL TCP tunnel")
	}

	// Close established TCP sessions and reject new dials while the simulated
	// network path is offline. The PostgreSQL process itself remains running.
	proxy.setOnline(false)
	failureCtx, failureCancel := context.WithTimeout(context.Background(), 3*time.Second)
	err = repository.Ping(failureCtx)
	failureCancel()
	if err == nil {
		t.Fatal("repository Ping unexpectedly succeeded while PostgreSQL network path was offline")
	}

	// Restore the same TCP proxy listener. The original pgxpool must discard
	// broken connections, establish a new session, and become healthy again.
	proxy.setOnline(true)
	recoveryDeadline := time.Now().Add(10 * time.Second)
	var recovered bool
	var lastErr error
	for time.Now().Before(recoveryDeadline) {
		pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
		lastErr = repository.Ping(pingCtx)
		pingCancel()
		if lastErr == nil {
			recovered = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !recovered {
		t.Fatalf("existing PostgreSQL pool did not recover after network path restoration: %v", lastErr)
	}
}

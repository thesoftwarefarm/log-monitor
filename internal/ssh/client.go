package ssh

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"log-monitor/internal/config"
	"log-monitor/internal/logger"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// Pool manages SSH connections to multiple servers, reusing existing connections.
type Pool struct {
	mu      sync.Mutex
	clients map[string]*ssh.Client
}

func NewPool() *Pool {
	return &Pool{
		clients: make(map[string]*ssh.Client),
	}
}

// GetClient returns a cached or new SSH connection for the given server config.
// The context allows callers to cancel/timeout the connection attempt.
func (p *Pool) GetClient(ctx context.Context, srv config.ServerConfig) (*ssh.Client, error) {
	key := fmt.Sprintf("%s@%s:%d", srv.User, srv.Host, srv.Port)
	logger.Log("ssh", "GetClient start: %s", key)

	p.mu.Lock()
	if c, ok := p.clients[key]; ok {
		p.mu.Unlock()
		logger.Log("ssh", "found cached client for %s, sending keepalive", key)
		// Check if connection is still alive — outside the lock so a slow
		// SendRequest doesn't block the entire pool.
		done := make(chan error, 1)
		go func() {
			_, _, err := c.SendRequest("keepalive@openssh.com", true, nil)
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil {
				logger.Log("ssh", "keepalive OK for %s", key)
				return c, nil
			}
			logger.Log("ssh", "keepalive failed for %s: %v", key, err)
		case <-ctx.Done():
			logger.Log("ssh", "keepalive cancelled for %s", key)
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			logger.Log("ssh", "keepalive timed out for %s", key)
		}
		// Connection is dead, remove it
		c.Close()
		p.mu.Lock()
		if p.clients[key] == c {
			delete(p.clients, key)
		}
		p.mu.Unlock()
	} else {
		p.mu.Unlock()
		logger.Log("ssh", "no cached client for %s, dialing", key)
	}

	client, err := dial(ctx, srv)
	if err != nil {
		logger.Log("ssh", "dial failed for %s: %v", key, err)
		return nil, err
	}

	logger.Log("ssh", "dial succeeded for %s", key)
	p.mu.Lock()
	p.clients[key] = client
	p.mu.Unlock()

	return client, nil
}

func dial(ctx context.Context, srv config.ServerConfig) (*ssh.Client, error) {
	logger.Log("ssh", "buildAuth method=%s", srv.Auth.Method)
	authMethods, agentConn, err := buildAuth(srv.Auth)
	if err != nil {
		logger.Log("ssh", "buildAuth failed: %v", err)
		return nil, fmt.Errorf("auth setup for %s: %w", srv.Host, err)
	}
	logger.Log("ssh", "buildAuth succeeded")

	cfg := &ssh.ClientConfig{
		User:            srv.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := fmt.Sprintf("%s:%d", srv.Host, srv.Port)

	// Dial TCP with the context so callers can cancel/timeout the attempt.
	logger.Log("ssh", "TCP dialing %s ...", addr)
	d := net.Dialer{Timeout: 10 * time.Second}
	tcpConn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		if agentConn != nil {
			agentConn.Close()
		}
		logger.Log("ssh", "TCP dial failed %s: %v", addr, err)
		return nil, fmt.Errorf("TCP dial %s: %w", addr, err)
	}
	logger.Log("ssh", "TCP connected to %s", addr)

	// Close the TCP connection if the context is cancelled during the SSH
	// handshake. ssh.NewClientConn does not accept a context, so this is the
	// only way to interrupt it.
	handshakeDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			logger.Log("ssh", "context cancelled during handshake, closing TCP to %s", addr)
			tcpConn.Close()
		case <-handshakeDone:
		}
	}()

	logger.Log("ssh", "SSH handshake starting with %s ...", addr)
	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, cfg)
	close(handshakeDone)
	if err != nil {
		tcpConn.Close()
		if agentConn != nil {
			agentConn.Close()
		}
		logger.Log("ssh", "SSH handshake failed %s: %v", addr, err)
		return nil, fmt.Errorf("SSH handshake %s: %w", addr, err)
	}
	logger.Log("ssh", "SSH handshake succeeded with %s", addr)

	// Verify the context wasn't cancelled after handshake completed but
	// before we closed handshakeDone.
	if ctx.Err() != nil {
		sshConn.Close()
		if agentConn != nil {
			agentConn.Close()
		}
		logger.Log("ssh", "context expired after handshake for %s", addr)
		return nil, fmt.Errorf("SSH connect %s: %w", addr, ctx.Err())
	}

	return ssh.NewClient(sshConn, chans, reqs), nil
}

// buildAuth returns auth methods and, if agent auth is used, the agent socket
// connection (caller must close it on dial failure).
func buildAuth(auth config.AuthConfig) ([]ssh.AuthMethod, net.Conn, error) {
	switch auth.Method {
	case "key":
		keyData, err := os.ReadFile(auth.KeyPath)
		if err != nil {
			return nil, nil, fmt.Errorf("reading key %s: %w", auth.KeyPath, err)
		}
		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			return nil, nil, fmt.Errorf("parsing key %s: %w", auth.KeyPath, err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil, nil

	case "agent":
		sock := os.Getenv("SSH_AUTH_SOCK")
		if sock == "" {
			return nil, nil, fmt.Errorf("SSH_AUTH_SOCK not set")
		}
		logger.Log("ssh", "dialing agent socket %s ...", sock)
		conn, err := net.DialTimeout("unix", sock, 5*time.Second)
		if err != nil {
			return nil, nil, fmt.Errorf("connecting to SSH agent: %w", err)
		}
		logger.Log("ssh", "agent socket connected")
		agentClient := agent.NewClient(conn)
		return []ssh.AuthMethod{ssh.PublicKeysCallback(agentClient.Signers)}, conn, nil

	case "password":
		return nil, nil, fmt.Errorf("password auth requires interactive input; use key or agent instead")

	default:
		return nil, nil, fmt.Errorf("unknown auth method: %s", auth.Method)
	}
}

// CloseAll closes all cached SSH connections.
func (p *Pool) CloseAll() {
	logger.Log("ssh", "CloseAll start")
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, c := range p.clients {
		c.Close()
		delete(p.clients, key)
	}
	logger.Log("ssh", "CloseAll done")
}

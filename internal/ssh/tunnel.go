// Package ssh implements optional SSH port forwarding for database URIs (swl2 uri_maybe_open_tunnel).
//
// Append @@[user[:pass]@]jump-host to a connection URI to forward the remote DB
// host through an SSH jump box, e.g. postgres://u:p@db@@jump/dbname.
package ssh

import (
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"sync"

	"github.com/ceymard/swl-go/internal/errs"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// tunnelPattern matches host[:port]@@[user[:pass]@]ssh-host[:port] inside a URI.
var tunnelPattern = regexp.MustCompile(`([^@:/]+)(?::(\d+))?@@(?:([^@:]+)(?::([^@]+))?@)?([^:/]+)(?::(\d+))?`)

// OpenResult is returned by MaybeOpenTunnel.
type OpenResult struct {
	URI    string
	Tunnel *Tunnel
}

// Close shuts down an opened tunnel, if any.
func (r *OpenResult) Close() error {
	if r == nil || r.Tunnel == nil {
		return nil
	}
	return r.Tunnel.Close()
}

// Tunnel forwards a local TCP port to a remote host:port through SSH.
type Tunnel struct {
	listener net.Listener
	client   *ssh.Client
	wg       sync.WaitGroup
	once     sync.Once
}

// Close stops accepting connections and closes the SSH client.
func (t *Tunnel) Close() error {
	var err error
	t.once.Do(func() {
		if t.listener != nil {
			err = t.listener.Close()
		}
		if t.client != nil {
			_ = t.client.Close()
		}
		t.wg.Wait()
	})
	return err
}

// MaybeOpenTunnel parses uri for @@ forwarding syntax and opens a tunnel when present.
func MaybeOpenTunnel(uri string, defaultPort int) (*OpenResult, error) {
	match := tunnelPattern.FindStringSubmatch(uri)
	if match == nil {
		return &OpenResult{URI: uri}, nil
	}
	loc := tunnelPattern.FindStringIndex(uri)

	remoteHost := match[1]
	remotePort := defaultPort
	if match[2] != "" {
		p, err := strconv.Atoi(match[2])
		if err != nil {
			return nil, errs.Wrap(err, "parse remote port in ssh tunnel uri")
		}
		remotePort = p
	}

	cfg := jumpConfig{
		username: match[3],
		password: match[4],
		host:     match[5],
		port:     22,
	}
	if match[6] != "" {
		p, err := strconv.Atoi(match[6])
		if err != nil {
			return nil, errs.Wrap(err, "parse ssh port in tunnel uri")
		}
		cfg.port = p
	}

	mergeSSHConfig(&cfg)

	tun, localPort, err := openTunnel(cfg, remoteHost, remotePort)
	if err != nil {
		return nil, err
	}

	newURI := uri[:loc[0]] + fmt.Sprintf("127.0.0.1:%d", localPort) + uri[loc[1]:]
	return &OpenResult{URI: newURI, Tunnel: tun}, nil
}

type jumpConfig struct {
	username string
	password string
	host     string
	port     int
}

func openTunnel(cfg jumpConfig, dstHost string, dstPort int) (*Tunnel, int, error) {
	auths, err := authMethods(cfg.password)
	if err != nil {
		return nil, 0, err
	}
	if len(auths) == 0 {
		return nil, 0, errs.New("no ssh authentication method available (set SSH_AUTH_SOCK or provide password in uri)")
	}

	clientConfig := &ssh.ClientConfig{
		User:            cfg.username,
		Auth:            auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // swl2 node-ssh default; use known_hosts in production
	}
	if clientConfig.User == "" {
		clientConfig.User = os.Getenv("USER")
	}

	addr := net.JoinHostPort(cfg.host, strconv.Itoa(cfg.port))
	client, err := ssh.Dial("tcp", addr, clientConfig)
	if err != nil {
		return nil, 0, errs.Wrap(err, "ssh connect", "host", cfg.host)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = client.Close()
		return nil, 0, errs.Wrap(err, "listen local ssh forward port")
	}

	t := &Tunnel{listener: ln, client: client}
	t.wg.Add(1)
	go t.serve(ln, client, dstHost, dstPort)

	return t, ln.Addr().(*net.TCPAddr).Port, nil
}

func (t *Tunnel) serve(ln net.Listener, client *ssh.Client, dstHost string, dstPort int) {
	defer t.wg.Done()
	for {
		local, err := ln.Accept()
		if err != nil {
			return
		}
		t.wg.Add(1)
		go func(local net.Conn) {
			defer t.wg.Done()
			remote, err := client.Dial("tcp", net.JoinHostPort(dstHost, strconv.Itoa(dstPort)))
			if err != nil {
				_ = local.Close()
				return
			}
			go func() { _, _ = io.Copy(remote, local) }()
			go func() { _, _ = io.Copy(local, remote) }()
		}(local)
	}
}

func authMethods(password string) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if password != "" {
		methods = append(methods, ssh.Password(password))
	}
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		conn, err := net.Dial("unix", sock)
		if err != nil {
			return methods, errs.Wrap(err, "dial ssh agent")
		}
		methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
	}
	return methods, nil
}

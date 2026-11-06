// 9r-relay is the client side of the 9router relay.
//
//	9r-relay install <pairing-code> [--name N] [--listen 127.0.0.1:20130] [--proxy URL]
//	                                 Windows: pair + install/start the "9router Relay" service
//	9r-relay pair <pairing-code>     pair only (writes the config)
//	9r-relay run                     run in the foreground (Linux/macOS, or testing)
//	9r-relay status | test           state / end-to-end check
//	9r-relay start | stop | uninstall [--purge]
//
// Clients (Codex, SDKs) use http://127.0.0.1:20130/v1 with their 9router API key.
package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/flynn/noise"
	"github.com/lampt24/9router/relay/internal/wire"
)

var version = "dev"

const defaultListen = "127.0.0.1:20130"

// Config is stored in an admin-only directory (Windows: %ProgramData%\9r-relay,
// others: ~/.config/9r-relay, mode 0700/0600). It holds the device private key.
type Config struct {
	URL        string `json:"url"`
	ServerKey  string `json:"server_key"`
	DevicePriv string `json:"device_priv"`
	DevicePub  string `json:"device_pub"`
	DeviceID   string `json:"device_id"`
	Name       string `json:"name"`
	CFID       string `json:"cf_access_client_id,omitempty"`
	CFSecret   string `json:"cf_access_client_secret,omitempty"`
	Listen     string `json:"listen"`
	Proxy      string `json:"proxy,omitempty"`
}

func configPath() string { return filepath.Join(configDir(), "config.json") }

func loadConfig() (*Config, error) {
	b, err := os.ReadFile(configPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.New("not paired yet: run `9r-relay install <pairing-code>`")
		}
		if errors.Is(err, os.ErrPermission) {
			return nil, errors.New("cannot read config: run from an elevated (Administrator) prompt")
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.Listen == "" {
		c.Listen = defaultListen
	}
	return &c, nil
}

func saveConfig(c *Config) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := protectDir(dir); err != nil {
		return fmt.Errorf("restrict %s: %w", dir, err)
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	tmp := configPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, configPath())
}

func (c *Config) device() (noise.DHKey, error) {
	priv, e1 := base64.RawURLEncoding.DecodeString(c.DevicePriv)
	pub, e2 := base64.RawURLEncoding.DecodeString(c.DevicePub)
	if e1 != nil || e2 != nil || len(priv) != 32 || len(pub) != 32 {
		return noise.DHKey{}, errors.New("config: invalid device key")
	}
	return noise.DHKey{Private: priv, Public: pub}, nil
}

func (c *Config) httpClient() (*http.Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if c.Proxy != "" {
		u, err := url.Parse(c.Proxy)
		if err != nil {
			return nil, fmt.Errorf("proxy: %w", err)
		}
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Transport: tr}, nil
}

func (c *Config) dialConfig(hello wire.Hello) (wire.DialConfig, error) {
	dev, err := c.device()
	if err != nil {
		return wire.DialConfig{}, err
	}
	key, err := base64.RawURLEncoding.DecodeString(c.ServerKey)
	if err != nil || len(key) != 32 {
		return wire.DialConfig{}, errors.New("config: invalid server key")
	}
	hc, err := c.httpClient()
	if err != nil {
		return wire.DialConfig{}, err
	}
	h := http.Header{}
	h.Set("User-Agent", "9r-relay/"+version)
	if c.CFID != "" {
		h.Set("CF-Access-Client-Id", c.CFID)
		h.Set("CF-Access-Client-Secret", c.CFSecret)
	}
	hello.Client = version
	return wire.DialConfig{URL: c.URL, ServerKey: key, Device: dev, Header: h, HTTPClient: hc, Hello: hello}, nil
}

func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("listen address %s is not loopback; refusing to expose the relay to the network", addr)
}

// pair redeems a pairing code with a fresh device key and saves the config.
func pair(code, name, listen, proxy string) (*Config, error) {
	pc, err := wire.DecodePairCode(code, time.Now())
	if err != nil {
		return nil, err
	}
	if err := checkLoopback(listen); err != nil {
		return nil, err
	}
	if name == "" {
		name, _ = os.Hostname()
	}
	dev, err := wire.GenerateKey()
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		URL: pc.URL, ServerKey: pc.ServerKey,
		DevicePriv: base64.RawURLEncoding.EncodeToString(dev.Private),
		DevicePub:  base64.RawURLEncoding.EncodeToString(dev.Public),
		CFID:       pc.CFID, CFSecret: pc.CFSecret, Listen: listen, Proxy: proxy,
	}
	dc, err := cfg.dialConfig(wire.Hello{Invite: pc.Invite, Name: name})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sess, w, err := wire.Dial(ctx, dc)
	if err != nil {
		return nil, fmt.Errorf("pairing failed: %w", err)
	}
	_ = sess.Close()
	cfg.DeviceID, cfg.Name = w.DeviceID, w.Name
	if err := saveConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// runRelay accepts local connections and tunnels each one.
func runRelay(ctx context.Context, cfg *Config, log *slog.Logger) error {
	if err := checkLoopback(cfg.Listen); err != nil {
		return err
	}
	dc, err := cfg.dialConfig(wire.Hello{})
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	go func() { <-ctx.Done(); ln.Close() }()
	log.Info("relay listening", "listen", cfg.Listen, "base_url", "http://"+cfg.Listen+"/v1", "relay", cfg.URL, "device", cfg.DeviceID, "version", version)
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		go handleConn(ctx, c, dc, log)
	}
}

func handleConn(ctx context.Context, c net.Conn, dc wire.DialConfig, log *slog.Logger) {
	defer c.Close()
	start := time.Now()
	sess, _, err := wire.Dial(ctx, dc)
	if err != nil {
		log.Error("tunnel failed", "err", err)
		return
	}
	defer sess.Close()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(sess, c); done <- struct{}{} }()
	go func() { _, _ = io.Copy(c, sess); done <- struct{}{} }()
	select {
	case <-done:
	case <-sess.Done():
	case <-ctx.Done():
	}
	log.Debug("tunnel closed", "dur", time.Since(start).Round(time.Millisecond))
}

// selfTest sends an unauthenticated request through a tunnel: a 401 from
// 9router proves the whole path (Cloudflare, Access, Noise, Caddy, 9router).
func selfTest(cfg *Config) error {
	dc, err := cfg.dialConfig(wire.Hello{})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	t0 := time.Now()
	sess, w, err := wire.Dial(ctx, dc)
	if err != nil {
		return err
	}
	defer sess.Close()
	hs := time.Since(t0)
	if _, err := io.WriteString(sess, "GET /v1/models HTTP/1.1\r\nHost: 9r-relay\r\nConnection: close\r\n\r\n"); err != nil {
		return err
	}
	line, err := bufio.NewReader(sess).ReadString('\n')
	if err != nil {
		return fmt.Errorf("no response through tunnel: %w", err)
	}
	fmt.Printf("handshake OK as device %s (%s) in %s\n", w.DeviceID, w.Name, hs.Round(time.Millisecond))
	fmt.Printf("GET /v1/models without API key -> %s", line)
	if !strings.Contains(line, " 401") {
		return errors.New("expected HTTP 401 from 9router")
	}
	fmt.Println("end-to-end path OK")
	return nil
}

func printStatus(cfg *Config) {
	fmt.Printf("device   : %s (%s)\n", cfg.DeviceID, cfg.Name)
	fmt.Printf("relay    : %s\n", cfg.URL)
	fmt.Printf("server fp: %s\n", fingerprint(cfg.ServerKey))
	fmt.Printf("base url : http://%s/v1\n", cfg.Listen)
	if cfg.Proxy != "" {
		fmt.Printf("proxy    : %s\n", cfg.Proxy)
	}
	state := "CLOSED"
	if c, err := net.DialTimeout("tcp", cfg.Listen, 2*time.Second); err == nil {
		c.Close()
		state = "OPEN"
	}
	fmt.Printf("listener : %s\n", state)
	fmt.Printf("service  : %s\n", serviceState())
}

func fingerprint(b64 string) string {
	raw, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil || len(raw) < 8 {
		return "?"
	}
	return base64.RawURLEncoding.EncodeToString(raw[:8])
}

func pairFlags(name string, args []string) (code, devName, listen, proxy string, err error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	n := fs.String("name", "", "device name (default: hostname)")
	l := fs.String("listen", defaultListen, "local listen address (loopback only)")
	p := fs.String("proxy", "", "HTTP(S) proxy URL for outbound connections, e.g. http://proxy:8080")
	// Accept the code before or after flags.
	var rest []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], wire.PairCodePrefix) {
			code = args[i]
			continue
		}
		rest = append(rest, args[i])
	}
	if err = fs.Parse(rest); err != nil {
		return
	}
	if code == "" && fs.NArg() > 0 {
		code = fs.Arg(0)
	}
	return code, *n, *l, *p, nil
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	if err := dispatch(os.Args[1], os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func dispatch(cmd string, args []string) error {
	switch cmd {
	case "pair":
		if !isElevated() {
			return errElevation
		}
		code, name, listen, proxy, err := pairFlags(cmd, args)
		if err != nil {
			return err
		}
		if code == "" {
			return errors.New("usage: 9r-relay pair <pairing-code>")
		}
		cfg, err := pair(code, name, listen, proxy)
		if err != nil {
			return err
		}
		fmt.Printf("paired as %s (%s); config: %s\n", cfg.DeviceID, cfg.Name, configPath())
		return nil
	case "install":
		return install(args)
	case "uninstall":
		purge := len(args) > 0 && args[0] == "--purge"
		return uninstall(purge)
	case "start":
		return startService()
	case "stop":
		return stopService()
	case "run":
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return runRelay(ctx, cfg, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	case "service":
		return runAsService()
	case "status":
		cfg, err := loadConfig()
		if err != nil {
			fmt.Printf("service  : %s\n", serviceState())
			return err
		}
		printStatus(cfg)
		return nil
	case "test":
		cfg, err := loadConfig()
		if err != nil {
			return err
		}
		return selfTest(cfg)
	case "version", "--version", "-v":
		fmt.Println(version)
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `9r-relay `+version+` - 9router relay client

  install <pairing-code> [--name N] [--listen 127.0.0.1:20130] [--proxy URL]
                 pair and install/start the background service (Windows, run as Administrator)
  pair <code>    pair only
  status         show device, listener and service state
  test           end-to-end check (expects HTTP 401 from 9router without an API key)
  start | stop   control the service
  uninstall [--purge]   remove the service (--purge also deletes the device key/config)
  run            run in the foreground

Clients use http://127.0.0.1:20130/v1 with a 9router API key.
`)
}

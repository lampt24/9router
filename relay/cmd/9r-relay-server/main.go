// 9r-relay-server terminates 9r-relay tunnels on the VPS.
//
//	9r-relay-server serve      [--listen 127.0.0.1:20140] [--upstream 127.0.0.1:20129]
//	9r-relay-server pair-new   --name company-pc [--ttl 10m]      prints a pairing code
//	9r-relay-server devices                                       list paired devices
//	9r-relay-server revoke     <id|name>
//	9r-relay-server pubkey
//
// Environment: RELAY_STATE (default /data/state.json), RELAY_PUBLIC_URL
// (wss://gw.lampt.works/v1/tunnel), CF_ACCESS_CLIENT_ID / CF_ACCESS_CLIENT_SECRET
// (embedded in pairing codes for the Cloudflare Access outer layer).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/lampt24/9router/relay/internal/server"
	"github.com/lampt24/9router/relay/internal/wire"
)

var version = "dev"

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	store := &server.Store{Path: env("RELAY_STATE", "/data/state.json")}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(store, os.Args[2:])
	case "pair-new":
		err = pairNew(store, os.Args[2:])
	case "devices":
		err = devices(store, len(os.Args) > 2 && os.Args[2] == "--json")
	case "revoke":
		if len(os.Args) != 3 {
			err = errors.New("usage: revoke <id|name>")
		} else {
			var d server.Device
			d, err = store.Revoke(os.Args[2], time.Now())
			if err == nil {
				fmt.Printf("revoked %s (%s); live tunnels close within 30s\n", d.ID, d.Name)
			}
		}
	case "pubkey":
		var st *server.State
		if st, err = store.Load(); err == nil {
			fmt.Println(st.ServerPub)
		}
	case "version", "--version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: 9r-relay-server serve|pair-new|devices|revoke|pubkey|version")
}

func serve(store *server.Store, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	listen := fs.String("listen", env("RELAY_LISTEN", "127.0.0.1:20140"), "listen address (loopback; cloudflared connects here)")
	upstream := fs.String("upstream", env("RELAY_UPSTREAM", "127.0.0.1:20129"), "upstream TCP address (Caddy /v1 origin)")
	_ = fs.Parse(args)

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	st, err := store.Load()
	if err != nil {
		return err
	}
	srv := server.New(store, *upstream, log)
	hs := &http.Server{
		Addr:              *listen,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	log.Info("9r-relay-server starting", "version", version, "listen", *listen, "upstream", *upstream,
		"server_key_fp", server.KeyFingerprint(st.ServerPub), "devices", len(st.Devices))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(sctx)
	}()
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func pairNew(store *server.Store, args []string) error {
	fs := flag.NewFlagSet("pair-new", flag.ExitOnError)
	name := fs.String("name", "", "device name shown in the device list")
	ttl := fs.Duration("ttl", 10*time.Minute, "invite lifetime (max 10m)")
	url := fs.String("url", env("RELAY_PUBLIC_URL", "wss://gw.lampt.works"+wire.TunnelPath), "public relay URL")
	jsonOut := fs.Bool("json", false, "machine readable output")
	_ = fs.Parse(args)
	if *name == "" {
		return errors.New("--name is required")
	}
	now := time.Now()
	secret, st, err := store.NewInvite(*name, *ttl, now)
	if err != nil {
		return err
	}
	exp := now.Add(*ttl)
	code := wire.PairCode{
		URL: *url, ServerKey: st.ServerPub, Invite: secret, Exp: exp.Unix(),
		CFID: os.Getenv("CF_ACCESS_CLIENT_ID"), CFSecret: os.Getenv("CF_ACCESS_CLIENT_SECRET"),
	}.Encode()
	if *jsonOut {
		fmt.Printf("{\"code\":%q,\"expires\":%q,\"name\":%q}\n", code, exp.UTC().Format(time.RFC3339), *name)
		return nil
	}
	fmt.Printf("Pairing code for %q (single use, expires %s):\n\n%s\n\nOn the device: 9r-relay install <code>\n", *name, exp.Local().Format("15:04:05"), code)
	return nil
}

func devices(store *server.Store, asJSON bool) error {
	st, err := store.Load()
	if err != nil {
		return err
	}
	if asJSON {
		type row struct {
			server.Device
			Fingerprint string `json:"fp"`
		}
		out := struct {
			Devices []row `json:"devices"`
			Pending int   `json:"pending_invites"`
		}{Pending: len(st.Invites), Devices: []row{}}
		for _, d := range st.Devices {
			d.Pub = ""
			out.Devices = append(out.Devices, row{Device: d, Fingerprint: server.KeyFingerprint(st.FindDevicePub(d.ID))})
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tPAIRED\tSTATUS\tKEY")
	for _, d := range st.Devices {
		status := "active"
		if d.Revoked != nil {
			status = "revoked " + d.Revoked.Local().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", d.ID, d.Name, d.Created.Local().Format("2006-01-02 15:04"), status, server.KeyFingerprint(d.Pub))
	}
	fmt.Fprintf(tw, "\n%d pending invite(s)\n", len(st.Invites))
	return tw.Flush()
}

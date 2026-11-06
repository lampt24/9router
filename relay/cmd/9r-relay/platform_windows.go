//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	svcName    = "9rRelay"
	svcDisplay = "9router Relay"
)

var errElevation = errors.New("run this from an elevated prompt (right-click PowerShell -> Run as Administrator)")

func configDir() string { return filepath.Join(os.Getenv("ProgramData"), "9r-relay") }
func installDir() string {
	return filepath.Join(os.Getenv("ProgramFiles"), "9r-relay")
}

func isElevated() bool { return windows.GetCurrentProcessToken().IsElevated() }

// protectDir limits the directory to SYSTEM and Administrators (SIDs, so it
// works on localized Windows). Files created inside inherit the ACL.
func protectDir(dir string) error {
	out, err := exec.Command("icacls", dir, "/inheritance:r", "/grant:r",
		"*S-1-5-18:(OI)(CI)F", "*S-1-5-32-544:(OI)(CI)F").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

func install(args []string) error {
	if !isElevated() {
		return errElevation
	}
	code, name, listen, proxy, err := pairFlags("install", args)
	if err != nil {
		return err
	}
	if code != "" {
		cfg, err := pair(code, name, listen, proxy)
		if err != nil {
			return err
		}
		fmt.Printf("paired as %s (%s)\n", cfg.DeviceID, cfg.Name)
	} else if _, err := loadConfig(); err != nil {
		return err
	}

	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	// Stop an existing service so the binary can be replaced.
	if s, err := m.OpenService(svcName); err == nil {
		_ = stopAndWait(s)
		s.Close()
	}

	// Run a private copy from Program Files (admin-only), never from a
	// user-writable download folder: the service runs as LocalSystem.
	self, err := os.Executable()
	if err != nil {
		return err
	}
	dst := filepath.Join(installDir(), "9r-relay.exe")
	if err := os.MkdirAll(installDir(), 0o755); err != nil {
		return err
	}
	if !samePath(self, dst) {
		if err := copyFile(self, dst); err != nil {
			return fmt.Errorf("copy binary: %w", err)
		}
	}

	conf := mgr.Config{
		DisplayName:      svcDisplay,
		Description:      "Encrypted relay to 9router (http://127.0.0.1:20130/v1). https://manage.lampt.works/relay/",
		StartType:        mgr.StartAutomatic,
		ServiceStartName: "LocalSystem",
	}
	s, err := m.OpenService(svcName)
	if err == nil {
		conf.BinaryPathName = fmt.Sprintf("\"%s\" service", dst)
		if err := s.UpdateConfig(conf); err != nil {
			s.Close()
			return err
		}
	} else {
		s, err = m.CreateService(svcName, dst, conf, "service")
		if err != nil {
			return err
		}
	}
	defer s.Close()
	_ = s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 15 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}, 86400)
	if err := s.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	fmt.Printf("installed and started service %q (%s)\n", svcDisplay, svcName)
	time.Sleep(2 * time.Second)
	if cfg, err := loadConfig(); err == nil {
		printStatus(cfg)
	}
	return nil
}

func uninstall(purge bool) error {
	if !isElevated() {
		return errElevation
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	if s, err := m.OpenService(svcName); err == nil {
		_ = stopAndWait(s)
		if err := s.Delete(); err != nil {
			s.Close()
			return err
		}
		s.Close()
		fmt.Println("service removed")
	}
	_ = os.RemoveAll(installDir())
	if purge {
		if err := os.RemoveAll(configDir()); err != nil {
			return err
		}
		fmt.Println("config and device key deleted (revoke the device on the server too)")
	} else {
		fmt.Printf("config kept in %s (use --purge to delete)\n", configDir())
	}
	return nil
}

func withService(fn func(*mgr.Service) error) error {
	if !isElevated() {
		return errElevation
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(svcName)
	if err != nil {
		return errors.New("service not installed")
	}
	defer s.Close()
	return fn(s)
}

func startService() error {
	return withService(func(s *mgr.Service) error { return s.Start() })
}

func stopService() error { return withService(stopAndWait) }

func stopAndWait(s *mgr.Service) error {
	st, err := s.Query()
	if err != nil || st.State == svc.Stopped {
		return err
	}
	if _, err := s.Control(svc.Stop); err != nil {
		return err
	}
	for i := 0; i < 40; i++ {
		time.Sleep(250 * time.Millisecond)
		if st, err = s.Query(); err != nil || st.State == svc.Stopped {
			return err
		}
	}
	return errors.New("timeout waiting for service to stop")
}

func serviceState() string {
	m, err := mgr.Connect()
	if err != nil {
		// Non-admin users may not open the SCM with full rights; try read-only.
		h, err2 := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
		if err2 != nil {
			return "unknown (" + err.Error() + ")"
		}
		m = &mgr.Mgr{Handle: h}
	}
	defer m.Disconnect()
	name, _ := windows.UTF16PtrFromString(svcName)
	h, err := windows.OpenService(m.Handle, name, windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return "not installed"
	}
	s := &mgr.Service{Name: svcName, Handle: h}
	defer s.Close()
	st, err := s.Query()
	if err != nil {
		return "unknown"
	}
	switch st.State {
	case svc.Running:
		return "running"
	case svc.Stopped:
		return "stopped"
	case svc.StartPending:
		return "starting"
	case svc.StopPending:
		return "stopping"
	}
	return fmt.Sprintf("state %d", st.State)
}

// ---- service runtime

type handler struct{}

func (handler) Execute(_ []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}
	log, closeLog := serviceLogger()
	defer closeLog()
	cfg, err := loadConfig()
	if err != nil {
		log.Error("config", "err", err)
		return true, 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan error, 1)
	go func() { errc <- runRelay(ctx, cfg, log) }()
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-errc:
				case <-time.After(5 * time.Second):
				}
				log.Info("service stopped")
				return false, 0
			}
		case err := <-errc:
			// Listener died (e.g. port in use): exit non-zero so SCM recovery restarts us.
			log.Error("relay stopped", "err", err)
			return true, 2
		}
	}
}

func runAsService() error {
	ok, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("`service` is used by the Windows service manager; use `run` to test in a console")
	}
	return svc.Run(svcName, handler{})
}

func serviceLogger() (*slog.Logger, func()) {
	path := filepath.Join(configDir(), "relay.log")
	if fi, err := os.Stat(path); err == nil && fi.Size() > 5<<20 {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return slog.New(slog.NewTextHandler(io.Discard, nil)), func() {}
	}
	return slog.New(slog.NewTextHandler(f, nil)), func() { f.Close() }
}

func samePath(a, b string) bool {
	pa, _ := filepath.Abs(a)
	pb, _ := filepath.Abs(b)
	return equalFold(pa, pb)
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

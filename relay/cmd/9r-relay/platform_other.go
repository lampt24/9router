//go:build !windows

package main

import (
	"errors"
	"os"
	"path/filepath"
)

var errElevation = errors.New("unreachable")

func configDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		d = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(d, "9r-relay")
}

func isElevated() bool { return true }

func protectDir(dir string) error { return os.Chmod(dir, 0o700) }

var errNoService = errors.New("background service install is Windows-only; on Linux/macOS use `9r-relay run` (see README for a systemd unit)")

func install(args []string) error {
	code, name, listen, proxy, err := pairFlags("install", args)
	if err != nil {
		return err
	}
	if code == "" {
		return errNoService
	}
	if _, err := pair(code, name, listen, proxy); err != nil {
		return err
	}
	println("paired; start the relay with: 9r-relay run")
	return nil
}

func uninstall(purge bool) error {
	if purge {
		return os.RemoveAll(configDir())
	}
	return errNoService
}

func startService() error  { return errNoService }
func stopService() error   { return errNoService }
func serviceState() string { return "n/a (use `9r-relay run`)" }
func runAsService() error  { return errNoService }

// Copyright (c) 2026 Federico Pereira <lord.basex@gmail.com>

package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// A Linux copy is either the user's own (the .tar.gz's install.sh, in
// ~/.local/bin), updated by running the new install.sh, or a package
// (/usr/bin), updated by its package manager through pkexec: the desktop
// asks for the administrator's password.
type userPlan struct{ exe string }

type pkgPlan struct {
	exe  string
	kind string // deb, rpm or pacman
}

func (u *Updater) plan() (plan, error) {
	exe, err := u.exe()
	if err != nil {
		return nil, err
	}
	if home, _ := os.UserHomeDir(); home != "" && exe == filepath.Join(home, ".local", "bin", "go-link-desk") {
		return &userPlan{exe: exe}, nil
	}
	if exe != "/usr/bin/go-link-desk" {
		return nil, ErrManual
	}
	if _, err := exec.LookPath("pkexec"); err != nil {
		return nil, ErrManual
	}
	for _, k := range []struct{ kind, cmd, arg string }{{"deb", "dpkg", "-S"}, {"rpm", "rpm", "-qf"}, {"pacman", "pacman", "-Qo"}} {
		if _, err := exec.LookPath(k.cmd); err == nil && exec.Command(k.cmd, k.arg, exe).Run() == nil {
			return &pkgPlan{exe: exe, kind: k.kind}, nil
		}
	}
	return nil, ErrManual
}

func (p *userPlan) asset(tag string) string {
	return "go-link-desk-" + tag + "-linux-" + arch() + ".tar.gz"
}

func (p *userPlan) install(ctx context.Context, archive string) (func() error, error) {
	dir, err := os.MkdirTemp("", "go-link-desk-new-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := untar(archive, dir); err != nil {
		return nil, err
	}
	scripts, _ := filepath.Glob(filepath.Join(dir, "*", "install.sh"))
	if len(scripts) != 1 {
		return nil, errors.New("updater: the archive has no install.sh")
	}
	if out, err := exec.CommandContext(ctx, "/bin/sh", scripts[0]).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("updater: install.sh: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return afterExit(os.Getpid(), p.exe), nil
}

func (p *pkgPlan) asset(tag string) string {
	v := bare(tag)
	rpmArch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[arch()]
	switch p.kind {
	case "deb":
		return "go-link-desk_" + v + "_" + arch() + ".deb"
	case "rpm":
		return "go-link-desk-" + v + "-1." + rpmArch + ".rpm"
	default:
		return "go-link-desk-" + v + "-1-" + rpmArch + ".pkg.tar.zst"
	}
}

func (p *pkgPlan) install(ctx context.Context, pkg string) (func() error, error) {
	// The package goes where root's tools can read it.
	_ = os.Chmod(filepath.Dir(pkg), 0o755)
	_ = os.Chmod(pkg, 0o644)
	var args []string
	switch p.kind {
	case "deb":
		args = []string{"dpkg", "-i", pkg}
	case "rpm":
		args = []string{"rpm", "-U", "--force", pkg}
	default:
		args = []string{"pacman", "-U", "--noconfirm", pkg}
	}
	if out, err := exec.CommandContext(ctx, "pkexec", args...).CombinedOutput(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && (ee.ExitCode() == 126 || ee.ExitCode() == 127) {
			return nil, ErrCancelled
		}
		return nil, fmt.Errorf("updater: %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return afterExit(os.Getpid(), p.exe), nil
}

// untar unpacks a .tar.gz into dir, refusing paths that leave it.
func untar(archive, dir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dir, h.Name)
		if !strings.HasPrefix(target, filepath.Clean(dir)+string(os.PathSeparator)) {
			return fmt.Errorf("updater: %q leaves the archive", h.Name)
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode)&0o755)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, io.LimitReader(tr, maxAsset))
			out.Close()
			if err != nil {
				return err
			}
		}
	}
}

func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

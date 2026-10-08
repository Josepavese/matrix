package fusefs

import (
	"context"
	"crypto/rand"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Josepavese/matrix/internal/logic/childenv"
	"github.com/Josepavese/matrix/internal/logic/providerdiag"
)

// Mount uses the same read-only semantic protocol on Linux, macOS and Windows;
// the installed rclone/OS filesystem driver provides the native mount boundary.
func (p *Provider) Mount(dir string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mounted {
		return fmt.Errorf("semantic filesystem already mounted")
	}
	if p.view == nil {
		return fmt.Errorf("semantic filesystem view is required")
	}
	if err := checkMountpoint(dir); err != nil {
		return err
	}
	path := p.driverPath
	if path == "" {
		path = "rclone"
	}
	binary, err := exec.LookPath(path)
	if err != nil {
		return fmt.Errorf("semantic mount driver unavailable: install rclone and the native FUSE/WinFsp prerequisite")
	}
	mount, err := startMount(binary, dir, p.view)
	if err != nil {
		return err
	}
	p.server = mount
	p.mounted = true
	return nil
}

func (p *Provider) Unmount() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.server == nil {
		return nil
	}
	if err := p.server.Unmount(); err != nil {
		return err
	}
	p.server = nil
	p.mounted = false
	return nil
}

func checkMountpoint(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("semantic mountpoint must be empty; existing data is never hidden")
	}
	return nil
}

type mountRuntime struct {
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	server    *http.Server
	listener  net.Listener
	directory string
	scratch   string
	done      chan error
}

func startMount(binary, dir string, view fs.FS) (*mountRuntime, error) {
	scratch, err := os.MkdirTemp("", "matrix-semantic-mount-")
	if err != nil {
		return nil, err
	}
	config := filepath.Join(scratch, "rclone.conf")
	if err := os.WriteFile(config, nil, 0600); err != nil {
		_ = os.RemoveAll(scratch)
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = os.RemoveAll(scratch)
		return nil, err
	}
	token := rand.Text()
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: projectionHandler(view, token)}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, mountArgs(dir, config)...)
	cmd.Env = append(childenv.Environment(), "RCLONE_HTTP_URL=http://"+listener.Addr().String()+"/", "RCLONE_HTTP_HEADERS=X-Matrix-FS-Key,"+token)
	cmd.Stderr = providerdiag.NewStderrCapture(8192, binary)
	cmd.WaitDelay = 2 * time.Second
	configureMountProcess(cmd)
	mount := &mountRuntime{cmd: cmd, cancel: cancel, server: server, listener: listener, directory: dir, scratch: scratch, done: make(chan error, 1)}
	go func() { _ = server.Serve(listener) }()
	if err := cmd.Start(); err != nil {
		cancel()
		_ = server.Close()
		_ = os.RemoveAll(scratch)
		return nil, fmt.Errorf("semantic mount process failed to start")
	}
	go func() { mount.done <- cmd.Wait(); close(mount.done) }()
	if err := mount.waitReady(); err != nil {
		_ = mount.Unmount()
		return nil, err
	}
	return mount, nil
}

func mountArgs(dir, config string) []string {
	args := []string{"mount", ":http:", dir, "--config", config, "--read-only", "--file-perms", "0400", "--dir-perms", "0500", "--attr-timeout", "0s", "--dir-cache-time", "0s", "--vfs-cache-mode", "off", "--http-no-head", "--vfs-read-chunk-size", "0", "--buffer-size", "0", "--log-level", "ERROR"}
	return append(args, platformMountArgs()...)
}

func (m *mountRuntime) waitReady() error {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	poll := time.NewTicker(25 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-m.done:
			return fmt.Errorf("semantic mount driver exited before mounting; check native driver prerequisites")
		case <-timer.C:
			return fmt.Errorf("semantic mount readiness not observed")
		case <-poll.C:
			entries, err := os.ReadDir(m.directory)
			if err == nil && len(entries) > 0 {
				return nil
			}
		}
	}
}

func (m *mountRuntime) Unmount() error {
	m.cancel()
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	select {
	case <-m.done:
	case <-timer.C:
		_ = m.cmd.Process.Kill()
		return fmt.Errorf("semantic mount process did not stop within its cleanup deadline")
	}
	_ = m.server.Close()
	_ = m.listener.Close()
	if err := os.RemoveAll(m.scratch); err != nil {
		return err
	}
	return verifyUnmounted(m.directory)
}

func verifyUnmounted(directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("semantic unmount not observed")
	}
	if len(entries) > 0 {
		return fmt.Errorf("semantic mount remains visible after process exit")
	}
	return nil
}

// Done lets command owners react to driver failure instead of waiting forever
// for a keyboard interrupt while the native mount has already disappeared.
func (p *Provider) Done() <-chan error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if mount, ok := p.server.(*mountRuntime); ok {
		return mount.done
	}
	return nil
}

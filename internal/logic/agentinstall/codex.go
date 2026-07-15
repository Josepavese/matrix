package agentinstall

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/agentcfg"
	"github.com/Josepavese/matrix/internal/middleware"
)

type Config struct {
	FS      middleware.FS
	Process middleware.Process
	Target  string
	Package string
	Env     []string
}

type npmInstall struct {
	process middleware.Process
	npmPath string
	staging string
	pkg     string
}

func InstallCanonicalCodex(ctx context.Context, cfg Config) (agentcfg.Config, error) {
	nodePath, npmPath, err := runtimes(cfg.Process)
	if err != nil {
		return agentcfg.Config{}, err
	}
	staging := cfg.Target + ".installing"
	if err := prepareStaging(cfg.FS, staging); err != nil {
		return agentcfg.Config{}, err
	}
	defer func() { _ = cfg.FS.RemoveAll(staging) }()

	fmt.Printf("Installing canonical Codex ACP provider '%s' into %s\n", cfg.Package, cfg.Target)
	if err := installPackage(ctx, npmInstall{process: cfg.Process, npmPath: npmPath, staging: staging, pkg: cfg.Package}); err != nil {
		return agentcfg.Config{}, err
	}
	relativeScript := filepath.Join("node_modules", "@agentclientprotocol", "codex-acp", "dist", "index.js")
	if _, err := cfg.FS.Stat(filepath.Join(staging, relativeScript)); err != nil {
		return agentcfg.Config{}, fmt.Errorf("canonical Codex ACP entrypoint missing after install: %w", err)
	}
	if err := swapDir(cfg.FS, cfg.Target, staging); err != nil {
		return agentcfg.Config{}, err
	}
	return agentcfg.Config{
		Command: nodePath, Args: []string{filepath.Join(cfg.Target, relativeScript)},
		Env: cfg.Env, Kind: "acp", Transport: "stdio",
	}, nil
}

func runtimes(proc middleware.Process) (string, string, error) {
	if proc == nil {
		return "", "", fmt.Errorf("canonical Codex ACP install requires a process provider")
	}
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return "", "", fmt.Errorf("canonical Codex ACP install requires node: %w", err)
	}
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return "", "", fmt.Errorf("canonical Codex ACP install requires npm: %w", err)
	}
	return nodePath, npmPath, nil
}

func prepareStaging(fs middleware.FS, staging string) error {
	if err := fs.RemoveAll(staging); err != nil {
		return err
	}
	return fs.MkdirAll(staging, 0o755)
}

func installPackage(ctx context.Context, install npmInstall) error {
	result, err := install.process.ExecSeparate(ctx, middleware.CommandSpec{
		Runner: install.npmPath, Args: []string{"install", "--prefix", install.staging, "--no-audit", "--no-fund", install.pkg},
	})
	if err != nil {
		return fmt.Errorf("canonical Codex ACP npm install failed: %w", err)
	}
	if result != nil && result.ExitCode == 0 {
		return nil
	}
	detail := "npm returned no result"
	if result != nil {
		detail = trimOutput(string(append(result.Stderr, result.Stdout...)))
	}
	return fmt.Errorf("canonical Codex ACP npm install failed with exit code %d: %s", exitCode(result), detail)
}

func swapDir(fs middleware.FS, target, staging string) error {
	backup := target + ".previous"
	_ = fs.RemoveAll(backup)
	_, statErr := fs.Stat(target)
	if statErr == nil {
		if err := fs.Rename(target, backup); err != nil {
			return fmt.Errorf("failed to stage existing agent install: %w", err)
		}
	}
	if err := fs.Rename(staging, target); err != nil {
		if statErr == nil {
			_ = fs.Rename(backup, target)
		}
		return fmt.Errorf("failed to activate agent install: %w", err)
	}
	_ = fs.RemoveAll(backup)
	return nil
}

func exitCode(result *middleware.ExecResult) int {
	if result == nil {
		return -1
	}
	return result.ExitCode
}

func trimOutput(raw string) string {
	raw = strings.Join(strings.Fields(strings.ReplaceAll(raw, "\x00", "\\0")), " ")
	if len(raw) > 400 {
		raw = raw[:400]
	}
	return raw
}

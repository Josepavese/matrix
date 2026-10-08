package containersandbox

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Josepavese/matrix/internal/logic/childenv"
	"github.com/Josepavese/matrix/internal/providers/osfs"
)

func mountPaths(workspace, state string) (string, string, error) {
	var err error
	workspace, err = canonicalMount(workspace)
	if err != nil {
		return "", "", err
	}
	state, err = canonicalMount(state)
	if err != nil {
		return "", "", err
	}
	if inside(workspace, state) || inside(state, workspace) {
		return "", "", fmt.Errorf("sandbox state and workspace must be separate directories")
	}
	if home, err := osfs.NewFSProvider().UserHomeDir(); err == nil && (inside(state, home) || inside(workspace, home)) {
		return "", "", fmt.Errorf("sandbox cannot expose the user home or its ancestors")
	}
	return workspace, state, nil
}

func inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func mountArgument(source, target string, readOnly bool) string {
	fields := []string{"type=bind", "src=" + source, "dst=" + target, "bind-recursive=disabled"}
	if readOnly {
		fields = append(fields, "readonly")
	}
	var buffer strings.Builder
	writer := csv.NewWriter(&buffer)
	_ = writer.Write(fields)
	writer.Flush()
	return strings.TrimSuffix(buffer.String(), "\n")
}

type boundaryPaths struct{ name, image, workspace, state string }

func createArguments(launch Launch, paths boundaryPaths) ([]string, []string, error) {
	name, image, workspace, state := paths.name, paths.image, paths.workspace, paths.state
	policy := launch.Policy
	args := []string{"create", "--name", name, "--pull", "never", "--interactive", "--init", "--read-only", "--log-driver", "none", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true", "--network", policy.Network, "--user", policy.User,
		"--memory", strconv.FormatUint(policy.MemoryBytes, 10), "--memory-swap", strconv.FormatUint(policy.MemoryBytes, 10), "--cpus", strconv.FormatUint(uint64(policy.CPUs), 10), "--pids-limit", strconv.FormatUint(uint64(policy.Pids), 10),
		"--mount", mountArgument(workspace, GuestWorkspace, policy.WorkspaceAccess == "read-only"), "--mount", mountArgument(state, "/home/matrix", false),
		"--workdir", GuestWorkspace, "--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=64m", "--env", "HOME=/home/matrix"}
	env := childenv.Environment()
	seen := map[string]bool{"HOME": true}
	for _, entry := range launch.Env {
		key, _, ok := strings.Cut(entry, "=")
		if key == "MATRIX_SANDBOX" {
			continue
		}
		if !ok || key == "" || strings.ContainsAny(key, "\x00\r\n") || seen[key] {
			return nil, nil, fmt.Errorf("invalid or duplicate container environment name")
		}
		if strings.HasPrefix(key, "DOCKER_") {
			return nil, nil, fmt.Errorf("container environment conflicts with engine control")
		}
		seen[key] = true
		args = append(args, "--env", key)
		env = append(env, entry)
	}
	if launch.Command == "" {
		return nil, nil, fmt.Errorf("container agent command required")
	}
	args = append(args, "--entrypoint", launch.Command, image)
	args = append(args, launch.Args...)
	return args, env, nil
}

func canonicalMount(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("sandbox mount directory is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("sandbox mount directory unavailable")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("sandbox mounts require existing directories")
	}
	if filepath.Dir(resolved) == resolved {
		return "", fmt.Errorf("sandbox cannot mount a filesystem root")
	}
	return resolved, nil
}

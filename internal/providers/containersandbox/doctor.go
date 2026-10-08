package containersandbox

import (
	"context"
	"runtime"

	"os/exec"
)

type Prerequisites struct {
	Platform       string `json:"platform"`
	Driver         string `json:"driver"`
	Available      bool   `json:"available"`
	Status         string `json:"status"`
	ImageID        string `json:"cached_image_id,omitempty"`
	StateDirectory string `json:"persistent_state_directory,omitempty"`
}

// Probe does not create a container, a state directory or a provider session.
// Available means prerequisites passed; physical enforcement is tested on launch.
func Probe(ctx context.Context, launch Launch) (Prerequisites, error) {
	result := Prerequisites{Platform: runtime.GOOS, Driver: "local-docker-linux-containers", Status: "prerequisites_unavailable"}
	engine, err := exec.LookPath(launch.Policy.Engine)
	if err != nil {
		return result, err
	}
	image, err := inspectLocalImage(ctx, engine, launch.Policy.Image)
	if err != nil {
		return result, err
	}
	workspace, state, err := mountPaths(launch.Workspace, launch.Policy.StateDir)
	if err != nil {
		return result, err
	}
	result.Available = true
	result.Status = "prerequisites_present_not_launched"
	result.ImageID = image
	result.StateDirectory = StateDirectory(state, workspace, launch.Identity, launch.Command)
	return result, nil
}

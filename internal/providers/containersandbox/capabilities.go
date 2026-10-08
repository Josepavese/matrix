package containersandbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/childenv"
)

func verifyEngineLimits(ctx context.Context, engine string) error {
	format := `{"os":{{json .OSType}},"memory":{{json .MemoryLimit}},"swap":{{json .SwapLimit}},"cpu":{{json .CPUCfsQuota}},"pids":{{json .PidsLimit}}}`
	output, err := engineCommand(ctx, engine, childenv.Environment(), "info", "--format", format)
	if err != nil {
		return err
	}
	var support struct {
		OS                      string
		Memory, Swap, CPU, Pids bool
	}
	if json.Unmarshal(output, &support) != nil {
		return fmt.Errorf("sandbox engine limit capabilities unavailable")
	}
	if support.OS != "linux" {
		return fmt.Errorf("sandbox engine is not running Linux containers")
	}
	for _, available := range []bool{support.Memory, support.Swap, support.CPU, support.Pids} {
		if !available {
			return fmt.Errorf("sandbox engine cannot enforce requested memory/swap/CPU/PID limits")
		}
	}
	return nil
}

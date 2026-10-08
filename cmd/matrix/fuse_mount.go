package main

import (
	"errors"
	"fmt"

	"github.com/Josepavese/matrix/internal/logic/filesystem"
	"github.com/Josepavese/matrix/internal/providers/fusefs"
	signalprovider "github.com/Josepavese/matrix/internal/providers/signal"
	"github.com/spf13/cobra"
)

var fuseMountCmd = &cobra.Command{
	Use:   "mount [dir]",
	Short: "Mount the Matrix virtual filesystem to a directory",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		view, closeFn, err := openSemanticFS(semanticSummaries)
		if err != nil {
			return fmt.Errorf("semantic view unavailable: %w", err)
		}
		defer closeFn()
		provider := fusefs.NewProvider().WithView(view).WithDriverPath(semanticMountDriver)
		mgr := filesystem.NewManager(provider)
		dir, err := resolveInvocationPath(args[0])
		if err != nil {
			return err
		}
		if err := mgr.MountVirtualFS(dir); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Matrix semantic FS mounted at %s\nPress Ctrl+C to unmount and exit.\n", dir)
		stopped := make(chan struct{})
		go func() { signalprovider.NewProvider().Wait(); close(stopped) }()
		select {
		case <-stopped:
		case <-cmd.Context().Done():
			err = cmd.Context().Err()
		case <-provider.Done():
			err = fmt.Errorf("semantic mount driver exited unexpectedly")
		}
		return errors.Join(err, mgr.UnmountVirtualFS())
	},
}

func init() {
	fuseMountCmd.Flags().StringVar(&semanticMountDriver, "driver-path", "rclone", "path to the installed PAL mount helper")
	fuseMountCmd.Flags().BoolVar(&semanticSummaries, "include-summaries", false, "expose terminal summaries; may contain private conversation content")
	fuseCmd.AddCommand(fuseMountCmd)
}

var semanticMountDriver string

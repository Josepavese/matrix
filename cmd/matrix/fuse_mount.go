package main

import (
	"fmt"
	"os"

	"github.com/Josepavese/matrix/internal/logic/filesystem"
	"github.com/Josepavese/matrix/internal/providers/fusefs"
	signalprovider "github.com/Josepavese/matrix/internal/providers/signal"
	"github.com/spf13/cobra"
)

var fuseMountCmd = &cobra.Command{
	Use:   "mount [dir]",
	Short: "Mount the Matrix virtual filesystem to a directory",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		provider := fusefs.NewProvider()
		mgr := filesystem.NewManager(provider)

		dir, err := resolveInvocationPath(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Mount error: invalid mount directory: %v\n", err)
			os.Exit(1)
		}
		if err := mgr.MountVirtualFS(dir); err != nil {
			fmt.Fprintf(os.Stderr, "Mount error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Matrix Virtual FS mounted at %s\n", dir)
		fmt.Println("Press Ctrl+C to unmount and exit.")

		// Block and wait for interruption via PAL abstraction
		sig := signalprovider.NewProvider()
		sig.Wait()

		fmt.Println("\nUnmounting...")
		if err := mgr.UnmountVirtualFS(); err != nil {
			fmt.Fprintf(os.Stderr, "Unmount error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	fuseCmd.AddCommand(fuseMountCmd)
}

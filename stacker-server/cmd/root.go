package cmd

import (
	"fmt"
	"os"

	"stacker/internal/config"
	"stacker/internal/server"

	"github.com/spf13/cobra"
)

var addr string

// Build metadata, overridden at release time via -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the stacker version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("stacker %s (commit %s, built %s)\n", version, commit, date)
	},
}

var rootCmd = &cobra.Command{
	Use:   "stacker",
	Short: "Stacker manages application deployments via docker stack",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if addr != "" {
			cfg.Addr = addr
		}
		return server.Run(cfg)
	},
	SilenceUsage: true,
}

func init() {
	rootCmd.Flags().StringVar(&addr, "addr", "", "address to listen on (default $STACKER_ADDR or :8080)")
	rootCmd.Version = version
	rootCmd.AddCommand(versionCmd)
}

// Execute runs the root command, exiting non-zero on failure.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

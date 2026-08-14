package cmd

import (
	"fmt"
	"os"

	"stacker/internal/config"
	"stacker/internal/server"

	"github.com/spf13/cobra"
)

var addr string

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
}

// Execute runs the root command, exiting non-zero on failure.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

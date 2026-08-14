package cmd

import (
	"os"

	"stacker/internal/bootstrap"
	"stacker/internal/config"

	"github.com/spf13/cobra"
)

var (
	setupYes   bool
	setupCheck bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Check and install the tools stacker needs",
	Long: "Checks for ssh, ssh-keygen, ssh-copy-id, sshpass and docker, and offers\n" +
		"to install anything missing using brew, apt, dnf, yum, pacman, zypper or apk.\n" +
		"This runs automatically the first time stacker starts.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		return bootstrap.Run(cmd.Context(), cfg.DataDir, bootstrap.Options{
			AssumeYes: setupYes,
			CheckOnly: setupCheck,
			Out:       os.Stdout,
		})
	},
	SilenceUsage: true,
}

func init() {
	setupCmd.Flags().BoolVarP(&setupYes, "yes", "y", false, "install missing dependencies without asking")
	setupCmd.Flags().BoolVar(&setupCheck, "check", false, "only report status, never install")
	rootCmd.AddCommand(setupCmd)
}

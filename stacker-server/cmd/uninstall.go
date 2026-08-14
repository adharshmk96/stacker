package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"stacker/internal/config"

	"github.com/spf13/cobra"
)

var uninstallYes bool

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the stacker data directory",
	Long: "Deletes the stacker application data directory, including the sqlite\n" +
		"database and every generated SSH keypair. This cannot be undone.\n" +
		"The stacker binary itself is left in place.",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := config.DataDir()
		if err != nil {
			return err
		}

		if _, err := os.Stat(dir); os.IsNotExist(err) {
			fmt.Fprintf(cmd.OutOrStdout(), "nothing to remove: %s does not exist\n", dir)
			return nil
		} else if err != nil {
			return err
		}

		if !uninstallYes {
			fmt.Fprintf(cmd.OutOrStdout(), "This will permanently delete %s, including the database and all SSH keys.\n", dir)
			fmt.Fprint(cmd.OutOrStdout(), "Type 'yes' to continue: ")
			answer, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
			if err != nil {
				return err
			}
			if strings.TrimSpace(answer) != "yes" {
				fmt.Fprintln(cmd.OutOrStdout(), "aborted")
				return nil
			}
		}

		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "removed %s\n", dir)
		return nil
	},
	SilenceUsage: true,
}

func init() {
	uninstallCmd.Flags().BoolVarP(&uninstallYes, "yes", "y", false, "delete without asking for confirmation")
	rootCmd.AddCommand(uninstallCmd)
}

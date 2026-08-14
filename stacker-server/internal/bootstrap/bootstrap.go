package bootstrap

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// stateFile records that a successful check already happened, so normal starts
// stay silent. Bump the suffix when Deps changes in a way that must re-run.
const stateFile = ".bootstrap-v1"

// Status is the result of looking one dependency up on PATH.
type Status struct {
	Dep
	Path string // empty when the binary was not found
}

// Found reports whether the binary is on PATH.
func (s Status) Found() bool { return s.Path != "" }

// Check looks up every dependency. It never installs anything.
func Check() []Status {
	out := make([]Status, 0, len(Deps))
	for _, d := range Deps {
		path, _ := exec.LookPath(d.Bin) //nolint:errcheck // absence is the signal
		out = append(out, Status{Dep: d, Path: path})
	}
	return out
}

// MissingRequired returns the required deps that are not installed.
func MissingRequired(sts []Status) []Status {
	var missing []Status
	for _, s := range sts {
		if s.Required && !s.Found() {
			missing = append(missing, s)
		}
	}
	return missing
}

// Options controls one bootstrap run.
type Options struct {
	// AssumeYes skips the confirmation prompt.
	AssumeYes bool
	// CheckOnly reports status and never installs.
	CheckOnly bool
	// In is where the confirmation is read from; defaults to os.Stdin.
	In io.Reader
	// Out is where progress is written; defaults to os.Stderr.
	Out io.Writer
}

// Run reports dependency status and, when something required is missing and
// the user agrees, installs it. It returns an error only when a required dep
// is still missing afterwards.
func Run(ctx context.Context, dataDir string, opts Options) error {
	in := opts.In
	if in == nil {
		in = os.Stdin
	}
	out := opts.Out
	if out == nil {
		out = os.Stderr
	}

	sts := Check()
	report(out, sts)

	missing := MissingRequired(sts)
	if len(missing) == 0 {
		markDone(dataDir)
		return nil
	}
	if opts.CheckOnly {
		return fmt.Errorf("%s", missingSummary(missing))
	}

	mgr := detectManager()
	installable, manual := partition(mgr, missing)

	for _, s := range manual {
		fmt.Fprintf(out, "\n  %s cannot be installed automatically — %s\n", s.Bin, manualHint(s.Dep))
	}
	if len(installable) == 0 {
		return fmt.Errorf("%s", missingSummary(missing))
	}

	if !opts.AssumeYes {
		ok, err := confirm(in, out, *mgr, installable)
		if err != nil || !ok {
			if err != nil {
				return err
			}
			return fmt.Errorf("%s (run `stacker setup` when ready)", missingSummary(missing))
		}
	}

	if err := install(ctx, out, *mgr, installable); err != nil {
		return err
	}

	// Re-check rather than trusting the installer's exit code: a package can
	// land somewhere that is not on this process's PATH.
	if still := MissingRequired(Check()); len(still) > 0 {
		return fmt.Errorf("%s after install — you may need to open a new shell", missingSummary(still))
	}
	fmt.Fprintln(out, "\nAll dependencies are ready.")
	markDone(dataDir)
	return nil
}

// EnsureFirstRun runs the bootstrap the first time stacker starts. It is
// advisory: a failure is printed but never stops the server, since most of the
// app works without ssh-copy-id and the API reports the missing tool itself.
func EnsureFirstRun(ctx context.Context, dataDir string, out io.Writer) {
	if done(dataDir) {
		return
	}
	fmt.Fprintln(out, "First run — checking dependencies (this happens once).")
	opts := Options{Out: out, CheckOnly: !interactive()}
	if err := Run(ctx, dataDir, opts); err != nil {
		fmt.Fprintf(out, "\nwarning: %v\n", err)
		fmt.Fprintln(out, "Run `stacker setup` to finish setting up. Starting anyway.")
	}
}

func report(out io.Writer, sts []Status) {
	fmt.Fprintln(out, "\nDependencies:")
	for _, s := range sts {
		switch {
		case s.Found():
			fmt.Fprintf(out, "  [ok]      %-12s %s\n", s.Bin, s.Path)
		case s.Required:
			fmt.Fprintf(out, "  [missing] %-12s %s\n", s.Bin, s.Why)
		default:
			fmt.Fprintf(out, "  [skipped] %-12s optional — %s\n", s.Bin, s.Why)
		}
	}
}

// partition splits missing deps into those the detected manager has a recipe
// for and those the user must handle. A nil manager makes everything manual.
func partition(mgr *manager, missing []Status) (installable, manual []Status) {
	for _, s := range missing {
		if mgr == nil || s.Packages[mgr.Name] == "" {
			manual = append(manual, s)
			continue
		}
		installable = append(installable, s)
	}
	return installable, manual
}

func confirm(in io.Reader, out io.Writer, mgr manager, sts []Status) (bool, error) {
	if !interactive() {
		return false, nil
	}
	fmt.Fprintf(out, "\nStacker can install these with %s:\n", mgr.Bin)
	for _, cmd := range installCommands(mgr, sts) {
		fmt.Fprintf(out, "  $ %s\n", strings.Join(cmd, " "))
	}
	fmt.Fprint(out, "\nProceed? [y/N] ")

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return false, nil
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

// installCommands is the exact argv list shown in the prompt and then run, so
// what the user approves is what executes.
func installCommands(mgr manager, sts []Status) [][]string {
	var cmds [][]string
	if refresh := mgr.refreshCommand(); refresh != nil {
		cmds = append(cmds, refresh)
	}

	// One command per package keeps a single failure from blocking the rest,
	// and dedupes the openssh package shared by ssh/ssh-keygen/ssh-copy-id.
	seen := map[string]bool{}
	for _, s := range sts {
		pkg := s.Packages[mgr.Name]
		if seen[pkg] {
			continue
		}
		seen[pkg] = true
		if mgr.Name == "brew" && s.Bin == "sshpass" {
			// sshpass is no longer in homebrew/core; this tap is the
			// commonly used replacement formula.
			pkg = "hudochenkov/sshpass/sshpass"
		}
		cmds = append(cmds, mgr.command(pkg))
	}
	return cmds
}

func install(ctx context.Context, out io.Writer, mgr manager, sts []Status) error {
	for _, argv := range installCommands(mgr, sts) {
		fmt.Fprintf(out, "\n$ %s\n", strings.Join(argv, " "))
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Stdin = os.Stdin // sudo may need to prompt for a password
		cmd.Stdout = out
		cmd.Stderr = out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("`%s` failed: %w", strings.Join(argv, " "), err)
		}
	}
	return nil
}

func missingSummary(missing []Status) string {
	names := make([]string, 0, len(missing))
	for _, s := range missing {
		names = append(names, s.Bin)
	}
	return "missing required dependencies: " + strings.Join(names, ", ")
}

// interactive reports whether stdin is a terminal we can prompt on.
func interactive() bool {
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func statePath(dataDir string) string { return filepath.Join(dataDir, stateFile) }

func done(dataDir string) bool {
	_, err := os.Stat(statePath(dataDir))
	return err == nil
}

// markDone records success. A write failure only costs an extra check next
// start, so it is deliberately ignored.
func markDone(dataDir string) {
	_ = os.WriteFile(statePath(dataDir), []byte("ok\n"), 0o600)
}

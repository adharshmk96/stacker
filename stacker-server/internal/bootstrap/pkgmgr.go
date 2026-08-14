package bootstrap

import (
	"os"
	"os/exec"
	"runtime"
)

// manager is a detected package manager and how to drive it non-interactively.
type manager struct {
	// Name keys into Dep.Packages.
	Name string
	// Bin is the executable, e.g. "apt-get".
	Bin string
	// Args are the arguments preceding the package names.
	Args []string
	// NeedsRoot means the command must be prefixed with sudo when we are not
	// already root. Homebrew explicitly refuses to run under sudo.
	NeedsRoot bool
	// Refresh is an optional index-update command run before installing.
	Refresh []string
}

// candidates is ordered: the first manager found on PATH wins. Homebrew comes
// first because a mac (or linuxbrew) user expects their own prefix to be used
// rather than a system package manager.
var candidates = []manager{
	{Name: "brew", Bin: "brew", Args: []string{"install"}},
	{Name: "apt", Bin: "apt-get", Args: []string{"install", "-y"}, NeedsRoot: true, Refresh: []string{"update"}},
	{Name: "dnf", Bin: "dnf", Args: []string{"install", "-y"}, NeedsRoot: true},
	{Name: "yum", Bin: "yum", Args: []string{"install", "-y"}, NeedsRoot: true},
	{Name: "pacman", Bin: "pacman", Args: []string{"-S", "--needed", "--noconfirm"}, NeedsRoot: true},
	{Name: "zypper", Bin: "zypper", Args: []string{"install", "-y"}, NeedsRoot: true},
	{Name: "apk", Bin: "apk", Args: []string{"add"}, NeedsRoot: true},
}

// detectManager returns the package manager to use, or nil when none is found.
func detectManager() *manager {
	for _, m := range candidates {
		if _, err := exec.LookPath(m.Bin); err == nil {
			found := m
			return &found
		}
	}
	return nil
}

// command builds the argv that installs pkgs, including sudo when required.
func (m manager) command(pkgs ...string) []string {
	argv := append([]string{m.Bin}, m.Args...)
	argv = append(argv, pkgs...)
	if m.NeedsRoot && os.Geteuid() != 0 {
		argv = append([]string{"sudo"}, argv...)
	}
	return argv
}

// refreshCommand builds the index-update argv, or nil when there is none.
func (m manager) refreshCommand() []string {
	if len(m.Refresh) == 0 {
		return nil
	}
	argv := append([]string{m.Bin}, m.Refresh...)
	if m.NeedsRoot && os.Geteuid() != 0 {
		argv = append([]string{"sudo"}, argv...)
	}
	return argv
}

// manualHint is the best advice we can give when there is no usable manager.
func manualHint(d Dep) string {
	if url, ok := docsURL[d.Bin]; ok {
		return "see " + url
	}
	if runtime.GOOS == "darwin" {
		return "install Homebrew (https://brew.sh) then re-run `stacker setup`"
	}
	return "install " + d.Bin + " with your system package manager"
}

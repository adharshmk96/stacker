// Package bootstrap checks — and, with consent, installs — the external
// command line tools stacker shells out to. It runs once on first start and
// on demand via `stacker setup`.
package bootstrap

// Dep is one external binary stacker needs on PATH.
type Dep struct {
	// Bin is the executable looked up on PATH.
	Bin string
	// Why is shown to the user so a prompt explains itself.
	Why string
	// Required marks a dep stacker cannot work without. Optional deps are
	// reported but never block and are never auto-installed.
	Required bool
	// Packages maps a package manager to the package providing Bin. A manager
	// missing from the map means stacker has no recipe and will only advise.
	Packages map[string]string
}

// Deps is the full dependency set, in the order they are reported.
var Deps = []Dep{
	{
		Bin:      "ssh",
		Why:      "connects to your nodes",
		Required: true,
		Packages: map[string]string{
			"brew": "openssh", "apt": "openssh-client", "dnf": "openssh-clients",
			"yum": "openssh-clients", "pacman": "openssh", "zypper": "openssh-clients",
			"apk": "openssh-client",
		},
	},
	{
		Bin:      "ssh-keygen",
		Why:      "generates the keypairs stacker stores for you",
		Required: true,
		Packages: map[string]string{
			"brew": "openssh", "apt": "openssh-client", "dnf": "openssh-clients",
			"yum": "openssh-clients", "pacman": "openssh", "zypper": "openssh-clients",
			"apk": "openssh-keygen",
		},
	},
	{
		Bin:      "ssh-copy-id",
		Why:      "installs a stacker key onto a node",
		Required: true,
		Packages: map[string]string{
			"brew": "openssh", "apt": "openssh-client", "dnf": "openssh-clients",
			"yum": "openssh-clients", "pacman": "openssh", "zypper": "openssh-clients",
			"apk": "openssh-client",
		},
	},
	{
		Bin:      "sshpass",
		Why:      "feeds the password to ssh-copy-id so it does not need a tty",
		Required: true,
		Packages: map[string]string{
			// sshpass was dropped from homebrew/core, so macOS needs a tap.
			// installBrewSshpass() handles that; the value here is only a label.
			"brew": "sshpass", "apt": "sshpass", "dnf": "sshpass", "yum": "sshpass",
			"pacman": "sshpass", "zypper": "sshpass", "apk": "sshpass",
		},
	},
	{
		Bin:      "docker",
		Why:      "runs deployments locally; remote hosts need their own docker",
		Required: false,
		Packages: map[string]string{
			// Deliberately no recipes: Docker installs are interactive
			// (Desktop on macOS, a repo + group change on Linux), so stacker
			// points at the docs instead of guessing.
		},
	},
}

// docsURL is printed for deps stacker has no install recipe for.
var docsURL = map[string]string{
	"docker": "https://docs.docker.com/get-docker/",
}

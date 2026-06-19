package cli

import (
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

type ToolStatus struct {
	Name        string
	Found       bool
	Version     string
	MinMajor    int
	MajorOK     bool
	InstallHint string
}

var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)

func probeTool(name, bin string, minMajor int, args ...string) ToolStatus {
	st := ToolStatus{Name: name, MinMajor: minMajor, MajorOK: minMajor == 0, InstallHint: installHintFor(name)}

	path, err := exec.LookPath(bin)
	if err != nil || path == "" {
		return st
	}

	out, err := exec.Command(bin, args...).CombinedOutput()
	if err != nil && len(out) == 0 {
		return st
	}

	m := versionPattern.FindStringSubmatch(string(out))
	if m == nil {
		return st
	}

	st.Found = true
	st.Version = m[0]
	if minMajor > 0 {
		if major, err := strconv.Atoi(m[1]); err == nil && major >= minMajor {
			st.MajorOK = true
		}
	}

	return st
}

func probeGo() ToolStatus { return probeTool("go", "go", 0, "version") }

func probeNode() ToolStatus { return probeTool("node", "node", 18, "--version") }

func probeNPM() ToolStatus { return probeTool("npm", "npm", 0, "--version") }

func probeGit() ToolStatus { return probeTool("git", "git", 0, "--version") }

func installHintFor(tool string) string {
	switch tool {
	case "go":
		switch runtime.GOOS {
		case "darwin":
			return "brew install go"
		case "windows":
			return "winget install -e --id GoLang.Go"
		default:
			return "see https://go.dev/dl/ - download the linux tarball and extract to /usr/local/go, then add /usr/local/go/bin to PATH"
		}

	case "node":
		switch runtime.GOOS {
		case "darwin":
			return "brew install node"
		case "windows":
			return "winget install -e --id OpenJS.NodeJS.LTS"
		default:
			return "curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash - && sudo apt-get install -y nodejs   # debian/ubuntu; for other distros see https://nodejs.org/en/download"
		}

	case "npm":
		return "ships with Node - install Node and npm comes along"
	case "git":
		switch runtime.GOOS {
		case "darwin":
			return "brew install git    # (or `xcode-select --install` for the Apple-bundled variant)"
		case "windows":
			return "winget install -e --id Git.Git"
		default:
			return "use your distro's package manager (e.g. `sudo apt install git` / `sudo dnf install git`)"
		}
	}

	return ""
}

func (s ToolStatus) summary() string {
	if !s.Found {
		return "missing"
	}

	if !s.MajorOK {
		return s.Version + " (need >=" + strconv.Itoa(s.MinMajor) + ")"
	}

	return s.Version
}

func (s ToolStatus) ok() bool { return s.Found && s.MajorOK }

func (s ToolStatus) versionInts() (int, int, int) {
	if !s.Found {
		return 0, 0, 0
	}

	m := versionPattern.FindStringSubmatch(s.Version)
	if m == nil {
		return 0, 0, 0
	}

	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch := 0
	if len(m) >= 4 && m[3] != "" {
		patch, _ = strconv.Atoi(m[3])
	}

	return major, minor, patch
}

func trimSpace(s string) string { return strings.TrimSpace(s) }

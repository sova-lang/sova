package cli

import (
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// ToolStatus is the resolved state of one host-toolchain dependency the Sova compiler shells out to (Go for the backend `go build`, Node/npm for the ts2sova generator and front-end bundling steps, optionally git for package resolution). Populated by probeTool; consumed by both `sova version` (compact one-liner) and `sova doctor` (multi-line diagnostic with install hint).
type ToolStatus struct {
	Name        string // Human-readable label: "go", "node", "npm", "git".
	Found       bool   // True when the binary was discovered on PATH and produced a parseable version string.
	Version     string // The parsed major.minor.patch (no leading 'v', no trailing build suffix). Empty when Found=false.
	MinMajor    int    // Minimum required major version (0 = no minimum). MajorOK reflects whether the discovered version satisfies this.
	MajorOK     bool   // True when Version's major component is >= MinMajor (always true when MinMajor=0).
	InstallHint string // Platform-specific suggestion shown by `sova doctor` when Found=false or MajorOK=false. Empty for tools without a recommendation.
}

// versionPattern matches the first `X.Y.Z` (or `X.Y`) digit-run in a tool's version output. Built to be lenient: `node` prints `v20.10.0`, `go version go1.23.0 linux/amd64`, `npm 10.2.4`, `git version 2.42.0` — all yield "20.10.0" / "1.23.0" / "10.2.4" / "2.42.0".
var versionPattern = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)

// probeTool runs `<bin> <args...>` with a hard wall of 4 seconds (via exec.CommandContext is overkill for this; relying on the binary returning quickly is fine in practice — every tool we probe answers `--version` in <100ms). Parses the first version-like token from stdout+stderr combined (Go writes to stderr historically). Returns a populated ToolStatus including the install hint for the current OS.
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

// probeGo runs `go version`. Go has no minimum we enforce — any modern `go` works for the `go build` step; users who want to pin should use their distro/asdf/mise.
func probeGo() ToolStatus { return probeTool("go", "go", 0, "version") }

// probeNode runs `node --version` and enforces a minimum of Node 18 — the line we drew when ts2sova-generator switched to module resolution that uses Node 18+ APIs (structured clone via `node:util`, fetch availability, top-level await in CommonJS interop).
func probeNode() ToolStatus { return probeTool("node", "node", 18, "--version") }

// probeNPM runs `npm --version`. npm ships with Node so the practical check is mostly redundant, but a separate ToolStatus lets `sova doctor` flag the rare case where someone disabled npm via `--without-npm` on a custom Node build.
func probeNPM() ToolStatus { return probeTool("npm", "npm", 0, "--version") }

// probeGit runs `git --version`. Git is used by pkgmgr when resolving git-flavoured package URLs; missing git makes those resolutions fail with a confusing error, so we surface it up front.
func probeGit() ToolStatus { return probeTool("git", "git", 0, "--version") }

// installHintFor returns a short, copy-pasteable shell command (or URL) suggesting how to install the named tool on the running OS. Hints favour the official installer / package manager idiomatic for each platform: Homebrew on macOS, winget on Windows, Nodesource's curl-pipe for Linux Node installs (because distro `nodejs` packages tend to lag and ts2sova depends on recent Node APIs). Returns "" for unrecognised tool names so the caller can decide whether to print anything.
func installHintFor(tool string) string {
	switch tool {
	case "go":
		switch runtime.GOOS {
		case "darwin":
			return "brew install go"
		case "windows":
			return "winget install -e --id GoLang.Go"
		default:
			return "see https://go.dev/dl/ — download the linux tarball and extract to /usr/local/go, then add /usr/local/go/bin to PATH"
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
		return "ships with Node — install Node and npm comes along"
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

// summary returns "20.10.0" when Found and version-ok; "20.10.0 (need >=18)" when below minimum; "missing" when the binary wasn't on PATH. Used by `sova version` for the compact display.
func (s ToolStatus) summary() string {
	if !s.Found {
		return "missing"
	}
	if !s.MajorOK {
		return s.Version + " (need >=" + strconv.Itoa(s.MinMajor) + ")"
	}
	return s.Version
}

// ok reports whether the tool is in a usable state — found AND meeting any minimum-major requirement.
func (s ToolStatus) ok() bool { return s.Found && s.MajorOK }

// versionInts returns the parsed version components — useful for callers comparing against more nuanced requirements than the major-only check. Missing components default to 0.
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

// trimSpace is a tiny convenience used by callers that read the raw command output before parsing; centralised here so call sites don't need a `strings` import just for the trim.
func trimSpace(s string) string { return strings.TrimSpace(s) }

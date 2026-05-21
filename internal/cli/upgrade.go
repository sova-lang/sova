package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"sova/internal/termui"

	"github.com/spf13/cobra"
)

// GithubReleasesLatestURL is the API endpoint queried for the most recent
// Sova release. Defined as a `var` so the upgrade command stays easy to
// repoint at a fork during development; the canonical repo URL is filled in
// once the public repo exists.
var GithubReleasesLatestURL = "https://api.github.com/repos/sova-lang/sova/releases/latest"

const (
	upgradeUserAgent     = "sova-cli"
	upgradeNoticeTimeout = 2 * time.Second
	upgradeCacheTTL      = 24 * time.Hour
)

// newUpgradeCmd registers `sova upgrade` - checks GitHub for a newer release,
// downloads the matching prebuilt archive (binary plus bundled stdlib), and
// installs it over the running executable. `--check` short-circuits before
// any download and just reports whether an upgrade is available. `--force`
// reinstalls the latest release even when the running binary already matches.
func newUpgradeCmd() *cobra.Command {
	var checkOnly, force bool
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Install the latest Sova release",
		Long:  "Download and install the latest Sova release from GitHub. The release archive bundles the compiler binary together with the stdlib, both unpacked side-by-side next to the running executable.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if checkOnly {
				return runUpgradeCheck()
			}
			return runUpgrade(force)
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "only check whether a newer version is available, without downloading")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "reinstall the latest release even if the current binary already matches")
	return cmd
}

func runUpgradeCheck() error {
	current := strings.TrimPrefix(Version, "v")
	termui.Header("sova upgrade --check")
	termui.Step(fmt.Sprintf("current version: %s", termui.Bold(current)))

	rel, err := fetchLatestRelease(context.Background())
	if err != nil {
		termui.Failure(fmt.Sprintf("failed to query GitHub releases: %v", err))
		return err
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	writeUpgradeCache(latest)
	termui.Step(fmt.Sprintf("latest release : %s", termui.Bold(latest)))

	if compareSemver(current, latest) >= 0 {
		termui.Success("sova is up to date")
		return nil
	}
	fmt.Fprintln(os.Stderr)
	termui.Info(fmt.Sprintf("A newer version is available: %s → %s", current, termui.Cyan(latest)))
	termui.Info("Run `sova upgrade` to install it.")
	return nil
}

func runUpgrade(force bool) error {
	current := strings.TrimPrefix(Version, "v")
	termui.Header("sova upgrade")

	sp := termui.StartSpinner("checking GitHub for the latest release")
	rel, err := fetchLatestRelease(context.Background())
	sp.Stop()
	if err != nil {
		termui.Failure(fmt.Sprintf("failed to query GitHub releases: %v", err))
		return err
	}
	if rel.TagName == "" {
		termui.Failure("GitHub returned no releases")
		return errors.New("no releases available")
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	writeUpgradeCache(latest)

	if !force && compareSemver(current, latest) >= 0 {
		termui.Success(fmt.Sprintf("sova is already on the latest version (%s)", current))
		return nil
	}

	assetName, err := pickAssetForPlatform()
	if err != nil {
		termui.Failure(err.Error())
		return err
	}
	var asset *githubAsset
	for i := range rel.Assets {
		if rel.Assets[i].Name == assetName {
			asset = &rel.Assets[i]
			break
		}
	}
	if asset == nil {
		err := fmt.Errorf("release %s has no asset named %s", rel.TagName, assetName)
		termui.Failure(err.Error())
		return err
	}

	termui.Step(fmt.Sprintf("current: %s", current))
	termui.Step(fmt.Sprintf("latest : %s", termui.Bold(latest)))

	tempArchive := filepath.Join(os.TempDir(), fmt.Sprintf("sova-upgrade-%d-%s", os.Getpid(), assetName))
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("sova-upgrade-%d", os.Getpid()))
	defer os.Remove(tempArchive)
	defer os.RemoveAll(tempDir)

	if err := downloadWithProgress(asset.BrowserDownloadURL, tempArchive, assetName); err != nil {
		termui.Failure(fmt.Sprintf("download failed: %v", err))
		return err
	}

	termui.Step("extracting")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return err
	}
	if err := extractArchive(tempArchive, tempDir); err != nil {
		termui.Failure(fmt.Sprintf("extract failed: %v", err))
		return err
	}

	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate running executable: %w", err)
	}
	binaryName := filepath.Base(currentPath)
	newBinary := findBinaryByName(tempDir, binaryName)
	if newBinary == "" {
		err := fmt.Errorf("extracted archive does not contain a %s binary", binaryName)
		termui.Failure(err.Error())
		return err
	}

	// Sova ships the stdlib bundled into the release archive directly
	// alongside the binary. Place it next to the executable so loadStdPackage
	// finds it on its `<binary>/std` probe.
	stdSource := filepath.Join(filepath.Dir(newBinary), "std")
	stdTarget := filepath.Join(filepath.Dir(currentPath), "std")
	termui.Step(fmt.Sprintf("installing → %s", termui.Dim(currentPath)))
	if info, err := os.Stat(stdSource); err == nil && info.IsDir() {
		if err := replaceTree(stdSource, stdTarget); err != nil {
			termui.WarnMsg(fmt.Sprintf("stdlib refresh failed: %v (binary still installed)", err))
		}
	}

	if err := replaceBinary(currentPath, newBinary); err != nil {
		termui.Failure(fmt.Sprintf("install failed: %v", err))
		return err
	}

	termui.Success(fmt.Sprintf("updated sova: %s → %s", current, latest))
	return nil
}

// downloadWithProgress streams `url` into `destPath`, drawing a progress bar
// when stderr is a TTY. The total length is taken from the Content-Length
// header; when missing we only show bytes downloaded so far.
func downloadWithProgress(url, destPath, label string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", upgradeUserAgent)
	httpClient := &http.Client{Timeout: 5 * time.Minute}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	total := resp.ContentLength
	var written int64
	buf := make([]byte, 32*1024)
	lastDraw := time.Now().Add(-time.Second)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if time.Since(lastDraw) > 100*time.Millisecond {
				termui.Progress(fmt.Sprintf("  downloading %s", label), written, total)
				lastDraw = time.Now()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	termui.Progress(fmt.Sprintf("  downloading %s", label), written, total)
	termui.EndProgress()
	return nil
}

// pickAssetForPlatform returns the release asset name expected for the
// current OS/arch. Matches the convention used by the release pipeline
// (`sova-<os>-<arch>.{tar.gz|zip}`). Returns an error when the platform isn't
// supported by the prebuilt-release matrix.
func pickAssetForPlatform() (string, error) {
	var arch string
	switch runtime.GOARCH {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("no prebuilt release for arch %s; build from source", runtime.GOARCH)
	}
	switch runtime.GOOS {
	case "linux":
		return fmt.Sprintf("sova-linux-%s.tar.gz", arch), nil
	case "darwin":
		if arch != "arm64" {
			return "", fmt.Errorf("no prebuilt release for darwin/%s", runtime.GOARCH)
		}
		return "sova-osx-arm64.tar.gz", nil
	case "windows":
		return fmt.Sprintf("sova-win-%s.zip", arch), nil
	}
	return "", fmt.Errorf("no prebuilt release for %s/%s", runtime.GOOS, runtime.GOARCH)
}

// extractArchive unpacks `path` into `destDir`. Handles `.zip` and
// `.tar.gz` formats - the two we ship for binary releases.
func extractArchive(path, destDir string) error {
	switch {
	case strings.HasSuffix(strings.ToLower(path), ".zip"):
		return extractZip(path, destDir)
	case strings.HasSuffix(strings.ToLower(path), ".tar.gz"):
		return extractTarGz(path, destDir)
	}
	return fmt.Errorf("unsupported archive format: %s", path)
}

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if err := extractZipEntry(f, dest); err != nil {
			return err
		}
	}
	return nil
}

func extractZipEntry(f *zip.File, dest string) error {
	target := filepath.Join(dest, f.Name)
	if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && target != filepath.Clean(dest) {
		return fmt.Errorf("zip entry escapes destination: %s", f.Name)
	}
	if f.FileInfo().IsDir() {
		return os.MkdirAll(target, f.Mode())
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func extractTarGz(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, hdr.Name)
		if !strings.HasPrefix(target, filepath.Clean(dest)+string(os.PathSeparator)) && target != filepath.Clean(dest) {
			return fmt.Errorf("tar entry escapes destination: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
}

// findBinaryByName locates a file named `name` (or `name.exe`) under `root`.
// Used after extracting the release archive to discover the new binary
// regardless of how the asset is structured internally.
func findBinaryByName(root, name string) string {
	candidate := filepath.Join(root, name)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(name, ".exe") {
		alt := filepath.Join(root, name+".exe")
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	var found string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, _ error) error {
		if info == nil || info.IsDir() {
			return nil
		}
		base := info.Name()
		if base == name || (runtime.GOOS == "windows" && base == name+".exe") {
			found = p
			return filepath.SkipDir
		}
		return nil
	})
	return found
}

// replaceBinary swaps `target` with the contents of `newBinary` atomically
// where the OS permits. On Windows we rename the old executable aside first
// because the file is locked while running; on Unix we stage as a sibling
// (same filesystem) and rename(2) over the target, which is safe with the
// kernel's text-segment caching of the running process.
func replaceBinary(target, newBinary string) error {
	dir := filepath.Dir(target)
	if runtime.GOOS == "windows" {
		oldPath := target + ".old"
		_ = os.Remove(oldPath)
		if err := os.Rename(target, oldPath); err != nil {
			return err
		}
		if err := copyFile(newBinary, target, 0o755); err != nil {
			_ = os.Rename(oldPath, target)
			return err
		}
		return nil
	}
	staged := filepath.Join(dir, filepath.Base(target)+".new")
	if err := copyFile(newBinary, staged, 0o755); err != nil {
		return err
	}
	return os.Rename(staged, target)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

// replaceTree refreshes the `std` directory next to the binary so the
// stdlib bundled with the new release replaces the old one cleanly. Backup +
// rename gives us rollback if the new copy fails mid-write.
func replaceTree(src, dst string) error {
	backup := dst + ".old"
	if _, err := os.Stat(dst); err == nil {
		_ = os.RemoveAll(backup)
		if err := os.Rename(dst, backup); err != nil {
			return err
		}
	}
	if err := copyTree(src, dst); err != nil {
		// roll back on failure
		_ = os.RemoveAll(dst)
		if _, err2 := os.Stat(backup); err2 == nil {
			_ = os.Rename(backup, dst)
		}
		return err
	}
	_ = os.RemoveAll(backup)
	return nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(p, target, info.Mode())
	})
}

// fetchLatestRelease queries GitHub's "latest release" endpoint and returns
// the deserialised payload. Bounded with `ctx` so the ambient `maybeShowUpdateNotice` flow can give up quickly when offline.
func fetchLatestRelease(ctx context.Context) (*githubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, GithubReleasesLatestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", upgradeUserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from GitHub releases API", resp.StatusCode)
	}
	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// compareSemver returns -1/0/+1 for a < b / a == b / a > b. Strips an
// optional `-prerelease` suffix because we don't want pre-release builds
// preferred over stable ones during the "up to date?" check.
func compareSemver(a, b string) int {
	stripPre := func(v string) string {
		if i := strings.IndexByte(v, '-'); i >= 0 {
			return v[:i]
		}
		return v
	}
	aParts := strings.Split(stripPre(a), ".")
	bParts := strings.Split(stripPre(b), ".")
	for i := 0; i < 3; i++ {
		ax, bx := 0, 0
		if i < len(aParts) {
			ax, _ = strconv.Atoi(aParts[i])
		}
		if i < len(bParts) {
			bx, _ = strconv.Atoi(bParts[i])
		}
		if ax < bx {
			return -1
		}
		if ax > bx {
			return 1
		}
	}
	return 0
}

// upgradeCacheFile returns the path to the cached latest-version record. We
// persist the most recently observed release in `$XDG_CACHE_HOME/sova/upgrade.json`
// (or the OS equivalent) so the ambient update notice can fire without
// hitting GitHub more than once per 24h.
func upgradeCacheFile() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sova", "upgrade.json"), nil
}

type upgradeCache struct {
	Latest    string    `json:"latest"`
	CheckedAt time.Time `json:"checked_at"`
}

func writeUpgradeCache(latest string) {
	path, err := upgradeCacheFile()
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	data, _ := json.Marshal(upgradeCache{Latest: latest, CheckedAt: time.Now().UTC()})
	_ = os.WriteFile(path, data, 0o644)
}

func readUpgradeCache() (*upgradeCache, bool) {
	path, err := upgradeCacheFile()
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var c upgradeCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, false
	}
	return &c, true
}

// MaybeShowUpdateNotice prints a one-line "newer version available" hint
// when the cached latest version is ahead of the running binary. Cheap and
// silent on every other run - the cache TTL determines how often we hit
// GitHub. Designed to be called near the end of long-running commands
// (`build`, `dev`) so it doesn't slow them down.
func MaybeShowUpdateNotice() {
	defer func() { _ = recover() }()
	current := strings.TrimPrefix(Version, "v")
	if current == "dev" || current == "" {
		return
	}
	cache, ok := readUpgradeCache()
	if ok && time.Since(cache.CheckedAt) < upgradeCacheTTL {
		emitUpgradeNotice(current, cache.Latest)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), upgradeNoticeTimeout)
	defer cancel()
	rel, err := fetchLatestRelease(ctx)
	if err != nil || rel == nil {
		return
	}
	latest := strings.TrimPrefix(rel.TagName, "v")
	writeUpgradeCache(latest)
	emitUpgradeNotice(current, latest)
}

func emitUpgradeNotice(current, latest string) {
	if latest == "" || compareSemver(current, latest) >= 0 {
		return
	}
	fmt.Fprintln(os.Stderr)
	termui.Info(fmt.Sprintf("A new version of sova is available: %s → %s", current, termui.Cyan(latest)))
	termui.Info("Run `sova upgrade` to install it.")
}

// ensure the cobra binding compiles in non-Unix builds where `exec` isn't
// otherwise used by this file
var _ = exec.Command

package cli

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Version is the toolchain version printed by `sova version`. The default value `"dev"` covers builds straight from a developer checkout; release builds override it via `-ldflags "-X 'sova/internal/cli.Version=v1.2.3'"`, typically driven by the Git tag in the publish workflow. Keep this as a `var` (not `const`) so ldflags can rewrite it.
var Version = "dev"

// Commit is the Git commit SHA the binary was built from. Same ldflags-override pattern as `Version`. Optional in dev - falls back to the Go-runtime `vcs.revision` build info when the binary was built with module support and the workspace was a Git repo, so even unstamped `go build` reports the actual SHA.
var Commit = ""

// BuildDate is when the release binary was produced (ISO-8601 UTC). Set by CI alongside `Version`; left empty on dev builds.
var BuildDate = ""

// newVersionCmd registers `sova version` - prints the toolchain version and (in long-form) the Git commit, build date, and Go runtime version. Designed to be stable enough for CI consumers (`sova version --short`) to grep against without ambiguity.
func newVersionCmd() *cobra.Command {
	var short bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the Sova toolchain version",
		RunE: func(cmd *cobra.Command, args []string) error {
			v, commit, date := resolveVersionInfo()
			if short {
				fmt.Println(v)
				return nil
			}
			fmt.Printf("sova %s\n", v)
			if commit != "" {
				fmt.Printf("  commit: %s\n", commit)
			}
			if date != "" {
				fmt.Printf("  built:  %s\n", date)
			}
			fmt.Printf("  go:     %s\n", runtime.Version())
			fmt.Printf("  os:     %s/%s\n", runtime.GOOS, runtime.GOARCH)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&short, "short", "s", false, "print only the version string (CI-friendly)")
	return cmd
}

// resolveVersionInfo returns the effective version, commit, and build date. When `Commit` was not stamped at link time, falls back to `debug.ReadBuildInfo()`'s `vcs.revision` setting so locally-built binaries still report the right SHA when the build was done from a Git checkout.
func resolveVersionInfo() (string, string, string) {
	commit := Commit
	if commit == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" {
					commit = s.Value
					break
				}
			}
		}
	}
	return Version, commit, BuildDate
}

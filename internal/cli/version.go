package cli

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/spf13/cobra"
)

var Version = "dev"

var Commit = ""

var BuildDate = ""

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
			fmt.Println("\nhost toolchain:")
			for _, st := range []ToolStatus{probeGo(), probeNode(), probeNPM()} {
				fmt.Printf("  %-7s %s\n", st.Name+":", st.summary())
			}

			fmt.Println("\nrun `sova doctor` for diagnostics and install hints")
			return nil
		},
	}

	cmd.Flags().BoolVarP(&short, "short", "s", false, "print only the version string (CI-friendly)")
	return cmd
}

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

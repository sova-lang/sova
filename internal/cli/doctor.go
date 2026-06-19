package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose the host toolchain (Go, Node, npm, Git)",
		Long:  "Probes every binary the Sova compiler shells out to and reports its version and status. Prints per-OS install hints for anything missing. Exits non-zero when a required tool is missing or below minimum - wire into CI to fail fast.",
		RunE: func(cmd *cobra.Command, args []string) error {
			tools := []ToolStatus{
				probeGo(),
				probeNode(),
				probeNPM(),
				probeGit(),
			}

			fmt.Printf("sova doctor - host: %s/%s\n\n", runtime.GOOS, runtime.GOARCH)
			problems := 0
			for _, st := range tools {
				mark := "OK"
				if !st.ok() {
					mark = "FAIL"
					problems++
				}

				fmt.Printf("  [%s] %s - %s\n", mark, st.Name, st.summary())
				if !st.ok() && st.InstallHint != "" {
					fmt.Printf("         install: %s\n", st.InstallHint)
				}
			}

			fmt.Println()
			if problems == 0 {
				fmt.Println("everything looks good - host toolchain is ready")
				return nil
			}

			fmt.Printf("%d problem(s) - install the tools above and re-run `sova doctor` to verify\n", problems)
			return fmt.Errorf("doctor: %d unmet requirement(s)", problems)
		},
	}
}

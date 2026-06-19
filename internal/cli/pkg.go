package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sova/internal/services/pkgmgr"

	"github.com/spf13/cobra"
)

func findProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, pkgmgr.ManifestFilename)); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd, nil
		}

		dir = parent
	}
}

func newInstallCmd() *cobra.Command {
	var frozen, offline, includeDev bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install dependencies from sova.toml into .sova/deps and write sova.lock",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := findProjectRoot()
			if err != nil {
				return err
			}

			svc := pkgmgr.NewService(root)
			res, err := svc.Install(pkgmgr.InstallOptions{Frozen: frozen, Offline: offline, IncludeDev: includeDev})
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "[install] resolved %d package(s)\n", len(res.Packages))
			return nil
		},
	}

	cmd.Flags().BoolVar(&frozen, "frozen", false, "fail if the lockfile would change (CI mode)")
	cmd.Flags().BoolVar(&offline, "offline", false, "skip all network operations; resolve only against the local cache")
	cmd.Flags().BoolVar(&includeDev, "include-dev", false, "also resolve [dev-dependencies] of the root package(s)")
	return cmd
}

func newAddCmd() *cobra.Command {
	var dev bool
	cmd := &cobra.Command{
		Use:   "add <name> <spec>",
		Short: "Add a dependency to sova.toml and install",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := findProjectRoot()
			if err != nil {
				return err
			}

			svc := pkgmgr.NewService(root)
			res, err := svc.Add(pkgmgr.AddOptions{Name: args[0], Spec: args[1], Dev: dev})
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "[add] %s; resolved %d package(s)\n", args[0], len(res.Packages))
			return nil
		},
	}

	cmd.Flags().BoolVar(&dev, "dev", false, "add to [dev-dependencies] instead of [dependencies]")
	return cmd
}

func newRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a dependency from sova.toml and re-install",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := findProjectRoot()
			if err != nil {
				return err
			}

			svc := pkgmgr.NewService(root)
			res, err := svc.Remove(args[0])
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "[remove] %s; resolved %d package(s)\n", args[0], len(res.Packages))
			return nil
		},
	}

	return cmd
}

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [name...]",
		Short: "Re-resolve dependencies; with no args, update everything",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := findProjectRoot()
			if err != nil {
				return err
			}

			svc := pkgmgr.NewService(root)
			res, err := svc.Update(args)
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "[update] resolved %d package(s)\n", len(res.Packages))
			return nil
		},
	}

	return cmd
}

func newLinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "link <path>",
		Short: "Register a local-path override for development",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := findProjectRoot()
			if err != nil {
				return err
			}

			svc := pkgmgr.NewService(root)
			name, err := svc.Link(args[0])
			if err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "[link] %s -> %s; run `sova install` to apply\n", name, args[0])
			return nil
		},
	}

	return cmd
}

func newUnlinkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlink <name>",
		Short: "Remove a local-path override",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := findProjectRoot()
			if err != nil {
				return err
			}

			svc := pkgmgr.NewService(root)
			ok, err := svc.Unlink(args[0])
			if err != nil {
				return err
			}

			if !ok {
				fmt.Fprintf(os.Stderr, "[unlink] no link for %s\n", args[0])
				return nil
			}

			fmt.Fprintf(os.Stderr, "[unlink] %s; run `sova install` to apply\n", args[0])
			return nil
		},
	}

	return cmd
}

func newOutdatedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "outdated",
		Short: "Show dependencies with newer versions available upstream",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := findProjectRoot()
			if err != nil {
				return err
			}

			svc := pkgmgr.NewService(root)
			rep, err := svc.Outdated()
			if err != nil {
				return err
			}

			if len(rep.Entries) == 0 {
				fmt.Println("No git-source packages to check.")
				return nil
			}

			drift := 0
			for _, e := range rep.Entries {
				marker := "  "
				if e.Latest != "" && !strings.HasPrefix(e.Latest, "error:") && e.Latest != e.Current && "v"+e.Current != e.Latest && e.Current != strings.TrimPrefix(e.Latest, "v") {
					marker = "* "
					drift++
				}

				fmt.Printf("%s%-30s current=%s latest=%s\n", marker, e.Name, e.Current, e.Latest)
			}

			fmt.Fprintf(os.Stderr, "[outdated] %d package(s) with newer versions available\n", drift)
			return nil
		},
	}

	return cmd
}

func newIndexCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Manage the package indexes",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Force-refresh every configured index repo",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := findProjectRoot()
			if err != nil {
				return err
			}

			svc := pkgmgr.NewService(root)
			if err := svc.RefreshIndex(); err != nil {
				return err
			}

			fmt.Fprintln(os.Stderr, "[index] refreshed")
			return nil
		},
	})
	return cmd
}

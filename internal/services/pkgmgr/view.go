package pkgmgr

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const DepsDirname = ".sova/deps"

type LinkMode int

const (
	LinkModeAuto LinkMode = iota
	LinkModeHardlink
	LinkModeCopy
	LinkModeSymlink
)

type Materialiser struct {
	Mode LinkMode
}

func (m *Materialiser) Apply(projectRoot string, res *Resolution) error {
	depsRoot := filepath.Join(projectRoot, DepsDirname)
	if err := os.RemoveAll(depsRoot); err != nil {
		return err
	}

	if err := os.MkdirAll(depsRoot, 0o755); err != nil {
		return err
	}

	pkgs := make([]*ResolvedPackage, 0, len(res.Packages))
	for _, pkg := range res.Packages {
		pkgs = append(pkgs, pkg)
	}

	parentDeps := map[string]bool{}

	for _, a := range pkgs {
		for _, b := range pkgs {
			if a.Name != b.Name && strings.HasPrefix(b.Name, a.Name+"/") {
				parentDeps[a.Name] = true
			}
		}
	}

	sort.SliceStable(pkgs, func(i, j int) bool {
		ai, aj := strings.Count(pkgs[i].Name, "/"), strings.Count(pkgs[j].Name, "/")
		if ai != aj {
			return ai < aj
		}

		return pkgs[i].Name < pkgs[j].Name
	})

	for _, pkg := range pkgs {
		dest := filepath.Join(depsRoot, pkg.Name)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}

		if strings.HasPrefix(pkg.Source, "path+") || pkg.Source == "workspace" {
			mode := m.Mode
			if parentDeps[pkg.Name] {
				mode = LinkModeCopy
			}

			if err := m.stageDirectWithMode(pkg.Dir, dest, mode); err != nil {
				return err
			}

			continue
		}

		if err := m.stageFromCache(pkg.Dir, dest); err != nil {
			return err
		}
	}

	return nil
}

func (m *Materialiser) stageDirect(srcDir, dest string) error {
	return m.stageDirectWithMode(srcDir, dest, m.Mode)
}

func (m *Materialiser) stageDirectWithMode(srcDir, dest string, mode LinkMode) error {
	if mode == LinkModeAuto {
		if runtime.GOOS == "windows" {
			mode = LinkModeCopy
		} else {
			mode = LinkModeSymlink
		}
	}

	switch mode {
	case LinkModeSymlink:
		return os.Symlink(srcDir, dest)
	default:
		return copyTree(srcDir, dest)
	}
}

func (m *Materialiser) stageFromCache(srcDir, dest string) error {
	mode := m.Mode
	if mode == LinkModeAuto {
		if runtime.GOOS == "windows" {
			mode = LinkModeCopy
		} else {
			mode = LinkModeHardlink
		}
	}

	return walkAndStage(srcDir, dest, mode)
}

func walkAndStage(srcDir, dest string, mode LinkMode) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(srcDir, path)
		if rel == "." {
			return os.MkdirAll(dest, 0o755)
		}

		first := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		if first == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		out := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}

		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}

		switch mode {
		case LinkModeHardlink:
			if err := os.Link(path, out); err == nil {
				return nil
			}

			return copyFile(path, out)
		case LinkModeSymlink:
			abs, _ := filepath.Abs(path)
			return os.Symlink(abs, out)
		default:
			return copyFile(path, out)
		}
	})
}

func copyTree(srcDir, dest string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, _ := filepath.Rel(srcDir, path)
		if rel == "." {
			return os.MkdirAll(dest, 0o755)
		}

		first := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		if first == ".git" || first == ".sova" {
			if d.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		out := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}

		return copyFile(path, out)
	})
}

func copyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}

	defer sf.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	df, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(df, sf)
	closeErr := df.Close()
	if copyErr != nil {
		return copyErr
	}

	return closeErr
}

func IsNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }

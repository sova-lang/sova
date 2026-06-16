package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// dotenvAutoloadFiles is the load order used when the user enables `[env].autoload` without supplying a custom `files` list. Mirrors the Vite / Next.js convention so newcomers don't need to learn a new scheme: base + per-profile + per-machine overlays, later entries winning.
var dotenvAutoloadFiles = []string{
	".env",
	".env.${profile}",
	".env.local",
	".env.${profile}.local",
}

// defaultDotenvProfile is the profile name used when neither the manifest pins one nor `SOVA_PROFILE` is set in the environment that runs the build. Matches the convention of treating `development` as the safe default — production-only file paths stay unloaded unless the operator explicitly asks for them.
const defaultDotenvProfile = "development"

// loadProjectDotenv reads the project's dotenv files per the manifest's `[env]` table and produces the merged key/value map plus the public-prefix to use when later filtering for the frontend. Returns a nil map (and empty prefix) when autoload is off, when the manifest has no `[env]` section, or when no configured file was found on disk — all of which mean "nothing to inject".
//
// Resolution order:
//
//  1. `manifest.Env.Files` (when set) wins over the built-in default list.
//  2. Each entry is substituted for `${profile}`. Profile comes from `manifest.Env.Profile`, falling back to the `SOVA_PROFILE` env at build time, falling back to `defaultDotenvProfile`.
//  3. Each substituted path is joined against `projectRoot` and read. Missing files skip silently; parse errors on present files skip silently too — dotenv loading is best-effort by design, the user's real env always wins at runtime anyway.
//  4. Vars are merged left-to-right; later files overwrite earlier ones.
//
// The public-prefix returned is whatever the manifest declares (default `""`), which the codegen layer uses to filter what crosses into the JS bundle.
func loadProjectDotenv(projectRoot string, m manifest) (map[string]string, string) {
	if !m.Env.Autoload {
		return nil, ""
	}

	files := m.Env.Files
	if len(files) == 0 {
		files = dotenvAutoloadFiles
	}

	profile := strings.TrimSpace(m.Env.Profile)
	if profile == "" {
		profile = strings.TrimSpace(os.Getenv("SOVA_PROFILE"))
	}
	if profile == "" {
		profile = defaultDotenvProfile
	}

	merged := map[string]string{}
	for _, raw := range files {
		path := strings.ReplaceAll(raw, "${profile}", profile)
		abs := path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(projectRoot, filepath.FromSlash(path))
		}
		mergeDotenv(merged, parseDotenvFile(abs))
	}
	if len(merged) == 0 {
		return nil, m.Env.PublicPrefix
	}
	return merged, m.Env.PublicPrefix
}

// mergeDotenv copies every entry from `src` into `dst`, letting `src` win on conflicts. Keeps mutation behind a single helper so the loader's intent (later files override earlier) is documented at the only call site.
func mergeDotenv(dst, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}

// parseDotenvFile reads one dotenv file from disk and returns the parsed key/value map. A missing file produces an empty map (not an error) — autoload is best-effort. The parser handles the common subset of the de-facto dotenv format:
//
//   - `#` introduces a line comment (only at column 0 or after blanks; in-value `#` is preserved).
//   - Blank lines are skipped.
//   - `KEY=value` with `KEY` matching the conservative `[A-Za-z_][A-Za-z0-9_]*` shape.
//   - `KEY=` produces an empty-string value (legal, distinct from "unset" — but the runtime conflates them on the backend side).
//   - Values may be single- or double-quoted; quotes are stripped. Double-quoted values support `\n`, `\r`, `\t`, `\\`, and `\"` escapes (no full JSON-style interpolation — keep it simple).
//   - Surrounding whitespace on both key and value is trimmed.
//   - `export KEY=value` is accepted (the `export` keyword is dropped) so shell-sourceable .env files work as-is.
//
// Variable interpolation (`${OTHER_KEY}`) is intentionally not supported; combine values at the call site instead. The user can keep more elaborate templating outside this loader.
func parseDotenvFile(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// .env files can have long URLs; bump the buffer past the default 64 KiB cap.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if !isDotenvKey(key) {
			continue
		}
		value := strings.TrimSpace(line[eq+1:])
		value = unquoteDotenvValue(value)
		out[key] = value
	}
	return out
}

// isDotenvKey enforces the conservative `[A-Za-z_][A-Za-z0-9_]*` shape on dotenv keys. Rejects exotic forms (spaces, dashes, dots) that would round-trip ambiguously through `os.Setenv` and the JS bundle — better to ignore the line than to bake in a key the runtime can't reliably look up.
func isDotenvKey(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// unquoteDotenvValue strips matching single or double quotes around a value and resolves the small `\n` / `\r` / `\t` / `\\` / `\"` escape set in the double-quoted case. Unquoted values come back exactly as written (after the call site's `TrimSpace`). The intentionally tiny escape set keeps the parser predictable and avoids re-inventing a JSON-like surface inside dotenv files.
func unquoteDotenvValue(v string) string {
	if len(v) >= 2 {
		first, last := v[0], v[len(v)-1]
		if first == '\'' && last == '\'' {
			return v[1 : len(v)-1]
		}
		if first == '"' && last == '"' {
			inner := v[1 : len(v)-1]
			var b strings.Builder
			for i := 0; i < len(inner); i++ {
				c := inner[i]
				if c == '\\' && i+1 < len(inner) {
					next := inner[i+1]
					switch next {
					case 'n':
						b.WriteByte('\n')
					case 'r':
						b.WriteByte('\r')
					case 't':
						b.WriteByte('\t')
					case '\\':
						b.WriteByte('\\')
					case '"':
						b.WriteByte('"')
					default:
						b.WriteByte('\\')
						b.WriteByte(next)
					}
					i++
					continue
				}
				b.WriteByte(c)
			}
			return b.String()
		}
	}
	return v
}

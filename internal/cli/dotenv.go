package cli

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

var dotenvAutoloadFiles = []string{
	".env",
	".env.${profile}",
	".env.local",
	".env.${profile}.local",
}

const defaultDotenvProfile = "development"

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

func mergeDotenv(dst, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}

func parseDotenvFile(path string) map[string]string {
	out := map[string]string{}

	f, err := os.Open(path)
	if err != nil {
		return out
	}

	defer f.Close()

	scanner := bufio.NewScanner(f)

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

package cli

// BuildConfig is the resolved configuration for a single compiler invocation, merged from manifest, CLI flags, and defaults.
type BuildConfig struct {
	Entry                   string
	SourceDir               string
	OutputDir               string
	OutputName              string
	WirePort                int
	WireHost                string
	WireSessionSecret       string
	WireSessionGraceSeconds int
	ServePort               int
	ServeHost               string
	ServeStrictPort         bool
	ServeFrontend           bool
	ServeWebDir             string
	ProdMode                bool
	TestMode                bool
	SCSSCommand             string // SCSSCommand pins the `sass` / `dart-sass` binary the embed resolver uses for `.scss`/`.sass` files. Empty enables auto-discovery on PATH (looks for `sass`, then `dart-sass`); set explicitly via `[build.scss] command = "..."` in sova.toml when the binary lives outside PATH or has a non-standard name.
	SCSSDisabled            bool   // SCSSDisabled short-circuits SCSS preprocessing entirely. Set via `[build.scss] enabled = false`; defaults to false so SCSS works as long as a binary is discoverable. `@embed` on `.scss` with SCSS disabled produces a clear diagnostic.
}

// DefaultBuildConfig returns a BuildConfig populated with the compiler's built-in defaults.
func DefaultBuildConfig() BuildConfig {
	return BuildConfig{
		Entry:         "",
		SourceDir:     ".",
		OutputDir:     ".output",
		OutputName:    "output",
		WirePort:      8080,
		ServePort:     5173,
		ServeFrontend: true,
		ServeWebDir:   "web",
	}
}

// OutputDirectory returns the resolved output directory. Implements the buildConfigGetter contract consumed by the codegen passes.
func (c BuildConfig) OutputDirectory() string { return c.OutputDir }

// SourceDirectory returns the resolved source directory (project root). Implements the buildConfigGetter contract consumed by the codegen passes; the Go emitter uses it to anchor build-artefact persistence (like the carried-over `go.sum` for extern Go-module dependencies) at the project root rather than inside the wipeable output directory.
func (c BuildConfig) SourceDirectory() string { return c.SourceDir }

// OutputBaseName returns the resolved output basename without extension. Implements the buildConfigGetter contract consumed by the codegen passes.
func (c BuildConfig) OutputBaseName() string { return c.OutputName }

// WirePortValue returns the configured wire server port (manifest-driven default; env can still override at runtime).
func (c BuildConfig) WirePortValue() int { return c.WirePort }

// WireHostValue returns the configured wire server host. An empty string means listen on all interfaces.
func (c BuildConfig) WireHostValue() string { return c.WireHost }

// WireSessionSecretValue returns the configured HMAC secret used to sign session-id cookies. Empty when not set in manifest; codegen falls back to the WIRE_SESSION_SECRET env var at runtime, and a generated dev fallback as last resort.
func (c BuildConfig) WireSessionSecretValue() string { return c.WireSessionSecret }

// WireSessionGraceSecondsValue returns the manifest-configured reconnect grace window for the WebSocket-backed session manager. Zero means "use compiler default (5 seconds)".
func (c BuildConfig) WireSessionGraceSecondsValue() int { return c.WireSessionGraceSeconds }

// ProdModeValue returns whether this build targets a production binary (embedded assets, no dev helpers).
func (c BuildConfig) ProdModeValue() bool { return c.ProdMode }

// TestModeValue returns whether this build is a `sova test` run. When true, the codegen pipeline emits a test driver `main()` that walks the discovered TestRegistry instead of the regular wire/dev main, and `on test` files participate in the backend Go output.
func (c BuildConfig) TestModeValue() bool { return c.TestMode }

// SCSSCommandValue returns the configured `sass` / `dart-sass` command for the embed resolver to invoke when an @embed targets a `.scss` / `.sass` file. Empty means auto-discovery; the embed resolver wraps this in `scss.New` which performs the actual `exec.LookPath` lookup.
func (c BuildConfig) SCSSCommandValue() string { return c.SCSSCommand }

// SCSSDisabledValue returns true when SCSS preprocessing is explicitly disabled in the manifest. The embed resolver treats `.scss`/`.sass` paths as errors when this is true, even if a binary would otherwise be discoverable.
func (c BuildConfig) SCSSDisabledValue() bool { return c.SCSSDisabled }

// CacheKey is the key under which the resolved BuildConfig is stored in the pass-manager cache.
const CacheKey = "build_config"

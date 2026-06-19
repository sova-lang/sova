package cli

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
	SCSSCommand             string
	SCSSDisabled            bool
	Codegen                 []CodegenStep
	LoadedEnv               map[string]string
	LoadedEnvPublicPrefix   string
}

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

func (c BuildConfig) OutputDirectory() string { return c.OutputDir }

func (c BuildConfig) SourceDirectory() string { return c.SourceDir }

func (c BuildConfig) OutputBaseName() string { return c.OutputName }

func (c BuildConfig) WirePortValue() int { return c.WirePort }

func (c BuildConfig) WireHostValue() string { return c.WireHost }

func (c BuildConfig) WireSessionSecretValue() string { return c.WireSessionSecret }

func (c BuildConfig) WireSessionGraceSecondsValue() int { return c.WireSessionGraceSeconds }

func (c BuildConfig) ProdModeValue() bool { return c.ProdMode }

func (c BuildConfig) TestModeValue() bool { return c.TestMode }

func (c BuildConfig) SCSSCommandValue() string { return c.SCSSCommand }

func (c BuildConfig) SCSSDisabledValue() bool { return c.SCSSDisabled }

func (c BuildConfig) LoadedEnvValue() map[string]string { return c.LoadedEnv }

func (c BuildConfig) LoadedEnvPublicPrefixValue() string { return c.LoadedEnvPublicPrefix }

const CacheKey = "build_config"

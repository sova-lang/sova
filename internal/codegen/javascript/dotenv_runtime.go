package javascript

import (
	"encoding/json"
	"sort"
	"strings"

	"sova/internal/codegen"
)

type dotenvLoadedEnvGetter interface {
	LoadedEnvValue() map[string]string
	LoadedEnvPublicPrefixValue() string
}

func dotenvSovaEnvJS(ctx *codegen.EmitContext) string {
	if ctx == nil || ctx.Cache == nil {
		return ""
	}

	cfg, ok := ctx.Cache["build_config"].(dotenvLoadedEnvGetter)
	if !ok {
		return ""
	}

	loaded := cfg.LoadedEnvValue()
	if len(loaded) == 0 {
		return ""
	}

	prefix := cfg.LoadedEnvPublicPrefixValue()
	if prefix == "" {

		return ""
	}

	publicVars := map[string]string{}

	for k, v := range loaded {
		if strings.HasPrefix(k, prefix) {
			publicVars[k] = v
		}
	}

	if len(publicVars) == 0 {
		return ""
	}

	keys := make([]string, 0, len(publicVars))
	for k := range publicVars {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	ordered := make(map[string]string, len(keys))
	for _, k := range keys {
		ordered[k] = publicVars[k]
	}

	encoded, err := json.Marshal(ordered)
	if err != nil {

		return ""
	}

	return "(function(){\n" +
		"  var defaults = " + string(encoded) + ";\n" +
		"  var existing = (typeof globalThis !== 'undefined' && globalThis.__SOVA_ENV) ? globalThis.__SOVA_ENV : {};\n" +
		"  var merged = {};\n" +
		"  for (var k in defaults) { merged[k] = defaults[k]; }\n" +
		"  for (var k in existing) { merged[k] = existing[k]; }\n" +
		"  (typeof globalThis !== 'undefined' ? globalThis : window).__SOVA_ENV = merged;\n" +
		"})();"
}

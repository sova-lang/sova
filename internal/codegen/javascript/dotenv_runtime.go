package javascript

import (
	"encoding/json"
	"sort"
	"strings"

	"sova/internal/codegen"
)

// dotenvLoadedEnvGetter is the duck-typed view of the build config the dotenv-injection helper needs. Lives here (not in `codegen.EmitContext`) because `codegen` can't import `cli` — putting the interface inside the consumer keeps the dependency direction clean. Mirrors the same shape used by the Go emitter's dotenv helper.
type dotenvLoadedEnvGetter interface {
	LoadedEnvValue() map[string]string
	LoadedEnvPublicPrefixValue() string
}

// dotenvSovaEnvJS returns the JavaScript snippet that seeds `globalThis.__SOVA_ENV` from the project's `[env].autoload` map, filtered to keys matching the manifest's `public_prefix`. Returns the empty string when there's nothing to inject — autoload off, no files found, or the prefix excluded every key.
//
// The snippet merges INTO any pre-existing `__SOVA_ENV` rather than replacing it, so a page that ships a server-side-rendered runtime override (`<script>window.__SOVA_ENV = {...}</script>` ahead of the bundle) still wins. The baked values are *defaults*, the page's overlay is authoritative — same precedence model as the backend side (process env > .env file).
//
// Keys are emitted in sorted order so the rendered bundle is reproducible across builds; otherwise Go map iteration order would churn the JS literal and invalidate downstream cache layers (esbuild output hash, browser HTTP cache).
//
// Security note: the prefix gate is the only thing preventing SECRET_KEY-shaped vars from leaking into the client bundle. An empty prefix means "expose nothing" — a deliberate fail-closed default. The user has to opt in explicitly via `public_prefix = "PUBLIC_"` (or whatever convention they pick) for any var to cross the boundary.
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
		// Empty prefix: explicit fail-closed. Refuse to ship anything to the frontend without a deliberate `public_prefix` opt-in.
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
		// json.Marshal on a string-string map can't realistically fail; if it does, drop the injection rather than emit broken JS.
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

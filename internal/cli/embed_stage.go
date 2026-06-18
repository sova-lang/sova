package cli

import (
	"io"
	"os"
	"path/filepath"
	"sova/internal/passes"
	"sova/internal/services/compiler"
)

// stageEmbedAssets copies every file referenced by a `@embed`-decorated const into `<outputDir>/__embeds/<hash>-<basename>` so the `//go:embed` directives produced by the Go emitter resolve at `go build` time. The naming convention (sha256[:16] of contents + original basename) keeps the staged tree stable across rebuilds (same content → same staged filename) and avoids collisions when two different source files share a basename. Files staged with the wrong content are silently overwritten because the hash includes the content; same hash means same bytes.
//
// Reads the build-wide registry populated by `passes.PassResolveEmbeds` under `EmbedAssetsCacheKey`. Silently no-ops when the registry is empty so projects without `@embed` declarations don't pay any I/O cost.
func stageEmbedAssets(c *compiler.CompilerContext, outputDir string) error {
	raw, ok := c.Cache[passes.EmbedAssetsCacheKey]
	if !ok {
		return nil
	}
	records, ok := raw.([]*passes.EmbedRecord)
	if !ok || len(records) == 0 {
		return nil
	}
	stageDir := filepath.Join(outputDir, "__embeds")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return err
	}
	for _, rec := range records {
		if rec == nil || rec.Info == nil {
			continue
		}
		dest := filepath.Join(stageDir, rec.Info.ContentHash+"-"+filepath.Base(rec.Info.SourcePath))
		if err := copyEmbedFile(rec.Info.SourcePath, dest); err != nil {
			return err
		}
	}
	return nil
}

// stageStaticAssets copies every file referenced by an `@asset`-decorated const into `<outputDir>/assets/<StagedName>`, where the prod embed.FS picks them up (the existing `//go:embed assets` directive in prod_helpers.go.tpl covers the whole directory) and where the dev-mode server serves them via the `/__sova/<file>` route. Each asset's StagedName already encodes the content hash, so files are stable across rebuilds and safe to cache indefinitely client-side.
//
// Reads the build-wide registry populated by `passes.PassResolveAssets` under `AssetsCacheKey`. No-ops silently when the registry is empty.
func stageStaticAssets(c *compiler.CompilerContext, outputDir string) error {
	raw, ok := c.Cache[passes.AssetsCacheKey]
	if !ok {
		return nil
	}
	records, ok := raw.([]*passes.AssetRecord)
	if !ok || len(records) == 0 {
		return nil
	}
	stageDir := filepath.Join(outputDir, "assets")
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return err
	}
	for _, rec := range records {
		if rec == nil || rec.Info == nil {
			continue
		}
		dest := filepath.Join(stageDir, rec.Info.StagedName)
		if rec.TransformedContent != nil {
			if err := os.WriteFile(dest, rec.TransformedContent, 0o644); err != nil {
				return err
			}
			continue
		}
		if err := copyEmbedFile(rec.Info.SourcePath, dest); err != nil {
			return err
		}
	}
	return nil
}

func copyEmbedFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

package cli

import (
	"io"
	"os"
	"path/filepath"
	"sova/internal/passes"
	"sova/internal/services/compiler"
)

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

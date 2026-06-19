package imagepipe

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"strings"

	nativewebp "github.com/HugoSmits86/nativewebp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

type Format string

const (
	FormatAuto Format = ""
	FormatPNG  Format = "png"
	FormatJPEG Format = "jpeg"
	FormatGIF  Format = "gif"
	FormatWebP Format = "webp"
)

type Options struct {
	To        Format
	Quality   int
	MaxWidth  int
	MaxHeight int
}

func (o Options) NeedsTransform() bool {
	return o.To != FormatAuto || o.MaxWidth > 0 || o.MaxHeight > 0 || o.Quality > 0
}

func NormalizeFormat(raw string) (Format, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return FormatAuto, true
	case "png":
		return FormatPNG, true
	case "jpg", "jpeg":
		return FormatJPEG, true
	case "gif":
		return FormatGIF, true
	case "webp":
		return FormatWebP, true
	}

	return "", false
}

func ExtensionFor(f Format) string {
	switch f {
	case FormatPNG:
		return ".png"
	case FormatJPEG:
		return ".jpg"
	case FormatGIF:
		return ".gif"
	case FormatWebP:
		return ".webp"
	}

	return ""
}

func Transform(src []byte, srcExt string, opts Options) ([]byte, string, error) {
	if !opts.NeedsTransform() {
		return src, srcExt, nil
	}

	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, "", fmt.Errorf("decode: %w", err)
	}

	img = resizeIfNeeded(img, opts.MaxWidth, opts.MaxHeight)
	target := opts.To
	if target == FormatAuto {
		target = formatFromExt(srcExt)
		if target == FormatAuto {
			return nil, "", fmt.Errorf("source extension %q is not a recognised image format and `to:` is unset", srcExt)
		}
	}

	var buf bytes.Buffer
	if err := encode(&buf, img, target, opts.Quality); err != nil {
		return nil, "", err
	}

	ext := ExtensionFor(target)
	if ext == "" {
		ext = srcExt
	}

	return buf.Bytes(), ext, nil
}

func resizeIfNeeded(src image.Image, maxW, maxH int) image.Image {
	if maxW <= 0 && maxH <= 0 {
		return src
	}

	b := src.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()
	dstW := srcW
	dstH := srcH
	if maxW > 0 && dstW > maxW {
		dstH = dstH * maxW / dstW
		dstW = maxW
	}

	if maxH > 0 && dstH > maxH {
		dstW = dstW * maxH / dstH
		dstH = maxH
	}

	if dstW == srcW && dstH == srcH {
		return src
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, xdraw.Over, nil)
	return dst
}

func encode(w io.Writer, img image.Image, f Format, quality int) error {
	switch f {
	case FormatPNG:
		enc := png.Encoder{CompressionLevel: png.BestCompression}

		return enc.Encode(w, img)
	case FormatJPEG:
		q := quality
		if q <= 0 {
			q = 85
		}

		if q > 100 {
			q = 100
		}

		return jpeg.Encode(w, img, &jpeg.Options{Quality: q})
	case FormatGIF:
		return gif.Encode(w, img, nil)
	case FormatWebP:
		return nativewebp.Encode(w, img, &nativewebp.Options{
			CompressionLevel: nativewebp.BestCompression,
		})
	}

	return fmt.Errorf("unsupported format %q", string(f))
}

func formatFromExt(ext string) Format {
	switch strings.ToLower(ext) {
	case ".png":
		return FormatPNG
	case ".jpg", ".jpeg":
		return FormatJPEG
	case ".gif":
		return FormatGIF
	case ".webp":
		return FormatWebP
	}

	return FormatAuto
}

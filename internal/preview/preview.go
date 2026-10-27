package preview

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"

	_ "image/gif"
	_ "image/png"

	_ "golang.org/x/image/webp"

	"github.com/rwcarlsen/goexif/exif"
	xdraw "golang.org/x/image/draw"
)

const (
	MaxSide   = 320
	Quality   = 80
	maxPixels = 50_000_000
)

var (
	ErrUnsupported = errors.New("preview: unsupported format")
	ErrTooLarge    = errors.New("preview: image too large")
)

var supportedTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

func Supported(mimeType string) bool {
	return supportedTypes[mimeType]
}

func Make(src io.ReadSeeker, dst io.Writer) error {
	cfg, _, err := image.DecodeConfig(src)
	if err != nil {
		if errors.Is(err, image.ErrFormat) {
			return ErrUnsupported
		}
		return fmt.Errorf("decode config: %w", err)
	}

	if int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return fmt.Errorf("%dx%d: %w", cfg.Width, cfg.Height, ErrTooLarge)
	}

	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek: %w", err)
	}
	orientation := readOrientation(src)

	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek: %w", err)
	}

	img, _, err := image.Decode(src)
	if err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	small := resize(img, MaxSide)
	rotated := orient(small, orientation)

	if err := jpeg.Encode(dst, rotated, &jpeg.Options{Quality: Quality}); err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	return nil
}

func readOrientation(r io.Reader) int {
	exifData, err := exif.Decode(r)
	if err != nil {
		return 1
	}

	tag, err := exifData.Get(exif.Orientation)
	if err != nil {
		return 1
	}

	orientation, err := tag.Int(0)
	if err != nil || orientation < 1 || orientation > 8 {
		return 1
	}

	return orientation
}

func resize(src image.Image, maxSide int) *image.RGBA {
	bounds := src.Bounds()
	width, height := bounds.Dx(), bounds.Dy()

	if width > maxSide || height > maxSide {
		if width >= height {
			height = max(1, height*maxSide/width)
			width = maxSide
		} else {
			width = max(1, width*maxSide/height)
			height = maxSide
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	return dst
}

func orient(src *image.RGBA, orientation int) *image.RGBA {
	if orientation == 1 {
		return src
	}

	width, height := src.Bounds().Dx(), src.Bounds().Dy()

	dstWidth, dstHeight := width, height
	if orientation >= 5 {
		dstWidth, dstHeight = height, width
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))

	for y := 0; y < dstHeight; y++ {
		for x := 0; x < dstWidth; x++ {
			var srcX, srcY int
			switch orientation {
			case 2: // зеркало по горизонтали
				srcX, srcY = width-1-x, y
			case 3: // 180°
				srcX, srcY = width-1-x, height-1-y
			case 4: // зеркало по вертикали
				srcX, srcY = x, height-1-y
			case 5: // транспонирование
				srcX, srcY = y, x
			case 6: // 90° по часовой
				srcX, srcY = y, height-1-x
			case 7: // транспонирование через другую диагональ
				srcX, srcY = width-1-y, height-1-x
			case 8: // 90° против часовой
				srcX, srcY = width-1-y, x
			}
			dst.SetRGBA(x, y, src.RGBAAt(srcX, srcY))
		}
	}

	return dst
}

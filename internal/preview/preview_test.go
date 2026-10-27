package preview

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestMakeSize(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		wantW, wantH  int
	}{
		{name: "горизонтальная уменьшается по ширине", width: 1000, height: 500, wantW: 320, wantH: 160},
		{name: "вертикальная уменьшается по высоте", width: 500, height: 1000, wantW: 160, wantH: 320},
		{name: "маленькая не увеличивается", width: 100, height: 50, wantW: 100, wantH: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := encodeJPEG(t, solid(tt.width, tt.height, color.RGBA{R: 200, A: 255}))

			var dst bytes.Buffer
			if err := Make(bytes.NewReader(src), &dst); err != nil {
				t.Fatal(err)
			}

			got := decodeJPEG(t, dst.Bytes()).Bounds()
			if got.Dx() != tt.wantW || got.Dy() != tt.wantH {
				t.Errorf("размер = %dx%d, ожидался %dx%d", got.Dx(), got.Dy(), tt.wantW, tt.wantH)
			}
		})
	}
}

// Настоящий EXIF-тег в настоящем JPEG: проверяет связку с goexif,
// а не только нашу функцию поворота
func TestMakeAppliesExifOrientation(t *testing.T) {
	src := withOrientation(t, encodeJPEG(t, solid(400, 200, color.RGBA{G: 200, A: 255})), 6)

	var dst bytes.Buffer
	if err := Make(bytes.NewReader(src), &dst); err != nil {
		t.Fatal(err)
	}

	got := decodeJPEG(t, dst.Bytes()).Bounds()
	if got.Dx() != 160 || got.Dy() != 320 {
		t.Errorf("размер = %dx%d, ожидался 160x320 (повёрнута на 90°)", got.Dx(), got.Dy())
	}
}

// 2×2 с разными углами: каждый вариант Orientation раскладывает их по-своему
//
//	topLeft    topRight
//	bottomLeft bottomRight
func TestOrient(t *testing.T) {
	topLeft := color.RGBA{R: 255, A: 255}
	topRight := color.RGBA{G: 255, A: 255}
	bottomLeft := color.RGBA{B: 255, A: 255}
	bottomRight := color.RGBA{R: 255, G: 255, A: 255}

	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src.SetRGBA(0, 0, topLeft)
	src.SetRGBA(1, 0, topRight)
	src.SetRGBA(0, 1, bottomLeft)
	src.SetRGBA(1, 1, bottomRight)

	tests := []struct {
		orientation int
		want        [4]color.RGBA // верхний ряд, потом нижний
	}{
		{1, [4]color.RGBA{topLeft, topRight, bottomLeft, bottomRight}},
		{2, [4]color.RGBA{topRight, topLeft, bottomRight, bottomLeft}},
		{3, [4]color.RGBA{bottomRight, bottomLeft, topRight, topLeft}},
		{4, [4]color.RGBA{bottomLeft, bottomRight, topLeft, topRight}},
		{5, [4]color.RGBA{topLeft, bottomLeft, topRight, bottomRight}},
		{6, [4]color.RGBA{bottomLeft, topLeft, bottomRight, topRight}},
		{7, [4]color.RGBA{bottomRight, topRight, bottomLeft, topLeft}},
		{8, [4]color.RGBA{topRight, bottomRight, topLeft, bottomLeft}},
	}

	for _, tt := range tests {
		got := orient(src, tt.orientation)
		gotPixels := [4]color.RGBA{
			got.RGBAAt(0, 0), got.RGBAAt(1, 0),
			got.RGBAAt(0, 1), got.RGBAAt(1, 1),
		}
		if gotPixels != tt.want {
			t.Errorf("orientation %d: got %v, want %v", tt.orientation, gotPixels, tt.want)
		}
	}
}

func TestMakeTransparentBecomesWhite(t *testing.T) {
	var src bytes.Buffer
	if err := png.Encode(&src, image.NewRGBA(image.Rect(0, 0, 10, 10))); err != nil {
		t.Fatal(err)
	}

	var dst bytes.Buffer
	if err := Make(bytes.NewReader(src.Bytes()), &dst); err != nil {
		t.Fatal(err)
	}

	red, green, blue, _ := decodeJPEG(t, dst.Bytes()).At(5, 5).RGBA()
	// JPEG с потерями: ровно 255 не обещан, но до чёрного далеко
	if red>>8 < 240 || green>>8 < 240 || blue>>8 < 240 {
		t.Errorf("пиксель = (%d,%d,%d), ожидался белый", red>>8, green>>8, blue>>8)
	}
}

func TestMakeUnsupported(t *testing.T) {
	var dst bytes.Buffer
	err := Make(bytes.NewReader([]byte("это не картинка")), &dst)
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("err = %v, ожидалась %v", err, ErrUnsupported)
	}
}

// Размер в заголовке GIF — 65535×65535: декодировать такое нетбук не потянет.
// Проверка должна сработать по заголовку, до чтения пикселей
func TestMakeTooLarge(t *testing.T) {
	var src bytes.Buffer
	if err := gif.Encode(&src, solid(1, 1, color.RGBA{A: 255}), nil); err != nil {
		t.Fatal(err)
	}
	raw := src.Bytes()
	// Logical Screen Descriptor: ширина и высота сразу за "GIF89a", little-endian
	copy(raw[6:10], []byte{0xFF, 0xFF, 0xFF, 0xFF})

	var dst bytes.Buffer
	err := Make(bytes.NewReader(raw), &dst)
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, ожидалась %v", err, ErrTooLarge)
	}
}

func TestSupported(t *testing.T) {
	for _, mimeType := range []string{"image/jpeg", "image/png", "image/gif", "image/webp"} {
		if !Supported(mimeType) {
			t.Errorf("Supported(%q) = false", mimeType)
		}
	}
	for _, mimeType := range []string{"video/mp4", "application/pdf", "image/heic", ""} {
		if Supported(mimeType) {
			t.Errorf("Supported(%q) = true", mimeType)
		}
	}
}

func solid(width, height int, fill color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, fill)
		}
	}
	return img
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodeJPEG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("превью — не JPEG: %v", err)
	}
	return img
}

// withOrientation вставляет сразу за SOI минимальный APP1-сегмент:
// заголовок Exif, TIFF big-endian и IFD0 с одним тегом Orientation (0x0112)
func withOrientation(t *testing.T, jpg []byte, orientation byte) []byte {
	t.Helper()
	if len(jpg) < 2 || jpg[0] != 0xFF || jpg[1] != 0xD8 {
		t.Fatal("не JPEG: нет SOI")
	}

	payload := []byte{
		'E', 'x', 'i', 'f', 0, 0,
		'M', 'M', 0x00, 0x2A, // TIFF, big-endian
		0x00, 0x00, 0x00, 0x08, // смещение IFD0
		0x00, 0x01, // в IFD0 один тег
		0x01, 0x12, // Orientation
		0x00, 0x03, // тип SHORT
		0x00, 0x00, 0x00, 0x01, // одно значение
		0x00, orientation, 0x00, 0x00, // само значение, дополненное до 4 байт
		0x00, 0x00, 0x00, 0x00, // следующего IFD нет
	}
	length := len(payload) + 2 // длина сегмента включает сами два байта длины

	var out bytes.Buffer
	out.Write(jpg[:2])
	out.Write([]byte{0xFF, 0xE1, byte(length >> 8), byte(length)})
	out.Write(payload)
	out.Write(jpg[2:])
	return out.Bytes()
}

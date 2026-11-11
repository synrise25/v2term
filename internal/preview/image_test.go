package preview

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			im.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 4), G: uint8(y * 5), B: 80, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func kittyPixels(t *testing.T, native string) []byte {
	t.Helper()
	var payload strings.Builder
	for _, part := range regexp.MustCompile("\\x1b_G([^;]*);([^\\x1b]*)\\x1b\\\\").FindAllStringSubmatch(native, -1) {
		if strings.Contains(part[1], "a=p") {
			continue
		}
		if len(part[2]) > 4096 {
			t.Fatal("Kitty payload exceeded the protocol chunk limit")
		}
		payload.WriteString(part[2])
	}
	decoded, err := base64.StdEncoding.DecodeString(payload.String())
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestDecodePreparesNativeAndFallbackRows(t *testing.T) {
	data := testPNG(t)
	for _, protocol := range []Protocol{ITerm2, Kitty, Blocks} {
		t.Run(string(protocol), func(t *testing.T) {
			im, err := Decode(data, "fixture", 5, 2, protocol)
			if err != nil {
				t.Fatal(err)
			}
			if im.Columns > 5 || len(im.Rows) > 2 || len(im.Rows) == 0 {
				t.Fatal("image escaped preview bounds")
			}
			for _, row := range im.Rows {
				if ansi.StringWidth(row.Text) != im.Columns {
					t.Fatal("fallback row has wrong cell width")
				}
				switch protocol {
				case ITerm2:
					if !strings.Contains(row.Native, "File=") || !strings.Contains(row.Native, "inline=1") || !strings.Contains(row.Native, "height=1") {
						t.Fatal("invalid native image command")
					}
					payload := strings.TrimSuffix(strings.SplitN(row.Native, ":", 2)[1], "\a")
					decoded, err := base64.StdEncoding.DecodeString(payload)
					if err != nil {
						t.Fatal(err)
					}
					tile, _, err := image.Decode(bytes.NewReader(decoded))
					if err != nil || tile.Bounds().Dy() != 16 {
						t.Fatal("native image row is not independently decodable")
					}
				case Kitty:
					if !strings.Contains(row.Native, "a=p") || !strings.Contains(row.Native, "r=1") || !strings.Contains(row.Native, "z=-1") || strings.Contains(row.Native, "a=T") {
						t.Fatal("invalid Kitty placement")
					}
					// Verify the actual wire payload, not only placement metadata.
					// EncodeGraphics requires an explicit direct transmission mode;
					// its zero value otherwise silently emits an empty upload.
					tile, err := png.Decode(bytes.NewReader(kittyPixels(t, row.Native)))
					if err != nil || tile.Bounds().Dx() != im.Columns*8 || tile.Bounds().Dy() != 16 {
						t.Fatalf("Kitty upload has no valid image pixels: %v", err)
					}
				case Blocks:
					if row.Native != "" {
						t.Fatal("fallback emitted unsupported image protocol")
					}
				}
			}
			resized, err := im.Resize(2, 1, protocol)
			if err != nil || resized.Columns > 2 || len(resized.Rows) > 1 {
				t.Fatal("resize did not respect bounds")
			}
		})
	}
}

func TestKittyChunkedUploadRetainsAllPixels(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 576, 32))
	seed := uint32(17)
	for i := range src.Pix {
		seed = seed*1664525 + 1013904223
		src.Pix[i] = byte(seed >> 24)
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, src); err != nil {
		t.Fatal(err)
	}
	im, err := Decode(encoded.Bytes(), "chunked", 72, 2, Kitty)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range im.Rows {
		if !strings.Contains(row.Native, "m=1") || !strings.Contains(row.Native, "m=0") {
			t.Fatal("fixture did not exercise a chunked transfer")
		}
		tile, err := png.Decode(bytes.NewReader(kittyPixels(t, row.Native)))
		if err != nil || tile.Bounds().Dx() != 576 || tile.Bounds().Dy() != 16 {
			t.Fatalf("chunked image upload was truncated: %v", err)
		}
	}
}

func TestDecodeRejectsOversizedOrActiveContent(t *testing.T) {
	if _, err := Decode([]byte("<svg><script>bad()</script></svg>"), "x", 10, 10, Blocks); err == nil {
		t.Fatal("unsupported active content accepted")
	}
	data := append([]byte(nil), testPNG(t)...)
	binary.BigEndian.PutUint32(data[16:20], 6000)
	binary.BigEndian.PutUint32(data[20:24], 4000)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	if _, err := Decode(data, "huge", 10, 10, Blocks); err == nil || !strings.Contains(err.Error(), "尺寸") {
		t.Fatalf("oversized image not rejected before decode: %v", err)
	}
}

func TestLoaderDoesNotSendV2EXCredentials(t *testing.T) {
	data := testPNG(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("credentials sent to an image host")
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(data)
	}))
	defer srv.Close()
	l := NewLoader()
	if _, err := l.Load(context.Background(), srv.URL, "instance", 10, 4, Blocks); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Load(context.Background(), "file:///etc/passwd", "instance", 10, 4, Blocks); err == nil {
		t.Fatal("non-HTTP URL accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.Load(ctx, srv.URL, "instance", 10, 4, Blocks); err == nil {
		t.Fatal("cancelled load continued")
	}
}

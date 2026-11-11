// Package preview loads bounded images and prepares terminal-native row tiles.
package preview

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/iterm2"
	"github.com/charmbracelet/x/ansi/kitty"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

type Protocol string

const (
	Auto   Protocol = "auto"
	ITerm2 Protocol = "iterm2"
	Kitty  Protocol = "kitty"
	Blocks Protocol = "blocks"
)

func ValidProtocol(s string) bool {
	return s == string(Auto) || s == string(ITerm2) || s == string(Kitty) || s == string(Blocks)
}

type Row struct {
	Text, Native string
	ID           int
}
type Image struct {
	Columns       int
	Rows          []Row
	Width, Height int
	source        image.Image
	key           string
}

type Loader struct {
	HTTP  *http.Client
	slots chan struct{}
}

func NewLoader() *Loader {
	return &Loader{slots: make(chan struct{}, 2), HTTP: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !validURL(r.URL) {
			return errors.New("图片重定向无效")
		}
		return nil
	}}}
}

func validURL(u *url.URL) bool {
	return (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil
}

// Load deliberately uses a separate HTTP client without V2EX credentials.
func (l *Loader) Load(ctx context.Context, target, key string, columns, rows int, protocol Protocol) (*Image, error) {
	if l.slots != nil {
		select {
		case l.slots <- struct{}{}:
			defer func() { <-l.slots }()
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	u, err := url.Parse(target)
	if err != nil || !validURL(u) {
		return nil, errors.New("无效图片地址")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "v2term/0.3")
	req.Header.Set("Referer", "https://www.v2ex.com/")
	req.Header.Set("Accept", "image/*")
	resp, err := l.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("图片加载失败，请检查网络后重试")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("图片服务器返回 HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (12<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 12<<20 {
		return nil, errors.New("图片超过 12 MiB，未加载")
	}
	return Decode(data, key, columns, rows, protocol)
}

func Decode(data []byte, key string, columns, rows int, protocol Protocol) (*Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("不支持的图片格式（支持 PNG、JPEG、GIF、WebP）")
	}
	if cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 20_000_000 {
		return nil, errors.New("图片尺寸过大，未解码")
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("图片数据无法解码")
	}
	// Retain only a bounded thumbnail for terminal resizes.
	w, h := fit(cfg.Width, cfg.Height, 1024, 1024)
	thumb := image.NewNRGBA(image.Rect(0, 0, w, h))
	xdraw.ApproxBiLinear.Scale(thumb, thumb.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	im := &Image{source: thumb, key: key, Width: cfg.Width, Height: cfg.Height}
	return im.Resize(columns, rows, protocol)
}

func fit(w, h, maxW, maxH int) (int, int) {
	if w > maxW {
		h = max(1, h*maxW/w)
		w = maxW
	}
	if h > maxH {
		w = max(1, w*maxH/h)
		h = maxH
	}
	return w, h
}

func (im *Image) Resize(columns, rows int, protocol Protocol) (*Image, error) {
	columns, rows = max(1, min(columns, 72)), max(1, min(rows, 16))
	w, h := fit(im.source.Bounds().Dx(), im.source.Bounds().Dy(), columns*8, rows*16)
	cols, rowCount := max(1, (w+7)/8), max(1, (h+15)/16)
	thumb := image.NewNRGBA(image.Rect(0, 0, cols*8, rowCount*16))
	xdraw.ApproxBiLinear.Scale(thumb, thumb.Bounds(), im.source, im.source.Bounds(), xdraw.Src, nil)
	pixels := image.NewNRGBA(image.Rect(0, 0, cols, rowCount*2))
	xdraw.ApproxBiLinear.Scale(pixels, pixels.Bounds(), thumb, thumb.Bounds(), xdraw.Src, nil)
	result := &Image{Columns: cols, Width: im.Width, Height: im.Height, source: im.source, key: im.key}
	for y := 0; y < rowCount; y++ {
		row := Row{ID: int(crc32.ChecksumIEEE([]byte(fmt.Sprintf("%s:%d:%d:%d", im.key, cols, rowCount, y)))&0x7fffffff) + 1}
		var blocks strings.Builder
		for x := 0; x < cols; x++ {
			top := color.NRGBAModel.Convert(pixels.At(x, y*2)).(color.NRGBA)
			bottom := color.NRGBAModel.Convert(pixels.At(x, y*2+1)).(color.NRGBA)
			fmt.Fprintf(&blocks, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", top.R, top.G, top.B, bottom.R, bottom.G, bottom.B)
		}
		blocks.WriteString("\x1b[0m")
		row.Text = blocks.String()
		if protocol == ITerm2 || protocol == Kitty {
			tile := thumb.SubImage(image.Rect(0, y*16, cols*8, (y+1)*16))
			var encoded bytes.Buffer
			if protocol == ITerm2 {
				var err error
				if tile.(*image.NRGBA).Opaque() {
					err = jpeg.Encode(&encoded, tile, &jpeg.Options{Quality: 75})
				} else {
					err = png.Encode(&encoded, tile)
				}
				if err != nil {
					return nil, err
				}
				row.Native = ansi.ITerm2(iterm2.File{Size: int64(encoded.Len()), Width: iterm2.Cells(cols), Height: iterm2.Cells(1), Inline: true, IgnoreAspectRatio: true, DoNotMoveCursor: true, Content: []byte(base64.StdEncoding.EncodeToString(encoded.Bytes()))})
			} else {
				// The library handles the protocol's 4096-byte chunk limit.
				// Upload separately: x/ansi's Options omits negative Z values, so the
				// placement command below explicitly chooses the background layer.
				if err := kitty.EncodeGraphics(&encoded, tile, &kitty.Options{Action: kitty.Transmit, Transmission: kitty.Direct, ID: row.ID, Format: kitty.PNG, Quiet: 2, Chunk: true}); err != nil {
					return nil, err
				}
				row.Native = encoded.String() + kittyPlacement(row.ID, cols)
			}
		}
		result.Rows = append(result.Rows, row)
	}
	return result, nil
}

func kittyPlacement(id, columns int) string {
	return fmt.Sprintf("\x1b_Ga=p,i=%d,c=%d,r=1,z=-1,C=1,q=2;\x1b\\", id, columns)
}

package main

import (
	_ "embed"
	"log"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed blur.kage
var blurSrc []byte

var (
	blurShaderOnce sync.Once
	blurShader     *ebiten.Shader
)

// loadBlurShader compiles the blur shader once. On failure it logs and returns
// nil, so the app simply runs without glow rather than crashing.
func loadBlurShader() *ebiten.Shader {
	blurShaderOnce.Do(func() {
		s, err := ebiten.NewShader(blurSrc)
		if err != nil {
			log.Printf("bloom: blur shader failed to compile: %v", err)
			return
		}
		blurShader = s
	})
	return blurShader
}

// bloom adds an additive Gaussian halo around a bright source layer. It owns two
// ping-pong buffers sized to the layer; reuse one instance per region so the
// buffers stay a stable size.
type bloom struct {
	w, h int
	a, b *ebiten.Image

	// uniforms and dir persist across passes so a blur costs no allocation;
	// only the direction values change per pass.
	uniforms map[string]any
	dir      []float32
}

func (bl *bloom) ensure(w, h int) {
	if bl.a != nil && bl.w == w && bl.h == h {
		return
	}
	bl.w, bl.h = w, h
	bl.a = ebiten.NewImage(w, h)
	bl.b = ebiten.NewImage(w, h)
}

// apply blurs src separably (iters passes at the given pixel spread) and adds
// the result to dst with the given brightness gain. src is treated as emissive:
// it should be bright marks on a transparent/black background. It is a no-op if
// the shader is unavailable.
func (bl *bloom) apply(dst, src *ebiten.Image, spread float64, iters int, gain float64) {
	shader := loadBlurShader()
	if shader == nil {
		return
	}
	b := src.Bounds()
	bl.ensure(b.Dx(), b.Dy())

	cur := src
	for range iters {
		bl.blur(bl.a, cur, shader, spread, 0)
		bl.blur(bl.b, bl.a, shader, 0, spread)
		cur = bl.b
	}
	op := &ebiten.DrawImageOptions{Blend: ebiten.BlendLighter}
	op.ColorScale.Scale(float32(gain), float32(gain), float32(gain), float32(gain))
	dst.DrawImage(cur, op)
}

// blur runs one separable Gaussian pass from src into dst along (dx, dy) pixels.
func (bl *bloom) blur(dst, src *ebiten.Image, shader *ebiten.Shader, dx, dy float64) {
	dst.Clear()
	if bl.uniforms == nil {
		bl.dir = make([]float32, 2)
		bl.uniforms = map[string]any{"Direction": bl.dir}
	}
	bl.dir[0], bl.dir[1] = float32(dx), float32(dy)
	op := &ebiten.DrawRectShaderOptions{}
	op.Images[0] = src
	op.Uniforms = bl.uniforms
	dst.DrawRectShader(bl.w, bl.h, shader, op)
}

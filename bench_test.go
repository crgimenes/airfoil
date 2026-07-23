package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// benchGame is simGame plus the render-side resources the draw benchmarks
// need; NewGame is not used because it wires the native menu.
func benchGame() *Game {
	g := simGame()
	g.fieldImg = ebiten.NewImage(gridW, gridH)
	g.trailImg = ebiten.NewImage(simW, simH)
	g.dotImg = ebiten.NewImage(2, 2)
	g.fadeImg = ebiten.NewImage(1, 1)
	g.pixbuf = make([]byte, gridW*gridH*4)
	g.stepSim(3)
	return g
}

func benchSceneGame(b *testing.B) *Game {
	g := benchGame()
	g.setScene(nekoScene(), "neko")
	g.animPlaying = true
	if b != nil {
		b.ReportAllocs()
	}
	return g
}

func BenchmarkPaintFieldSpeed(b *testing.B) {
	g := benchGame()
	b.ReportAllocs()
	for b.Loop() {
		g.paintField()
	}
}

func BenchmarkPaintFieldVorticity(b *testing.B) {
	g := benchGame()
	g.mode = modeVorticity
	b.ReportAllocs()
	for b.Loop() {
		g.paintField()
	}
}

func BenchmarkSmokeStep(b *testing.B) {
	g := benchGame()
	b.ReportAllocs()
	for b.Loop() {
		g.smoke.Step(g.sim, tracerSpeed)
	}
}

func BenchmarkDrawSmoke(b *testing.B) {
	g := benchGame()
	dst := ebiten.NewImage(simW, simH)
	b.ReportAllocs()
	for b.Loop() {
		g.drawSmoke(dst)
	}
}

func BenchmarkDrawStreamlines(b *testing.B) {
	g := benchGame()
	dst := ebiten.NewImage(simW, simH)
	b.ReportAllocs()
	for b.Loop() {
		g.drawStreamlines(dst)
	}
}

// BenchmarkSceneMask is the per-frame cost of an animated scene: rebuilding the
// solid mask at the current timeline position.
func BenchmarkSceneMask(b *testing.B) {
	g := benchSceneGame(b)
	for b.Loop() {
		g.sceneMask(g.scn.LoopTime(g.animTime))
		g.animTime += 1.0 / 60
	}
}

func BenchmarkMenuSignature(b *testing.B) {
	g := benchGame()
	b.ReportAllocs()
	for b.Loop() {
		g.menuSignature()
	}
}

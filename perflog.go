package main

import (
	"log"
	"runtime"
	"slices"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// perfLog gathers per-stage frame timings and, once a second, prints one
// machine-parseable line to the terminal. The output is key=value on purpose:
// it reads fine over SSH on an exhibit machine, lands in the browser console
// on the wasm build, and an AI agent can grep it without screen scraping.
//
// Percentiles are reported alongside the average because the occasional 25 ms
// frame is what the eye notices, not the 12 ms mean.
type perfLog struct {
	enabled bool

	upd    []float64 // whole Update, ms
	solver []float64 // stepSim inside Update, ms
	draw   []float64 // whole Draw, ms
	field  []float64 // paintField inside Draw, ms
	smoke  []float64 // drawSmoke inside Draw, ms

	windowStart time.Time
}

// now returns the stage start time, or zero when disabled so the caller pays
// only a nil-check on the fast path.
func (p *perfLog) now() time.Time {
	if !p.enabled {
		return time.Time{}
	}
	return time.Now()
}

// add records the elapsed milliseconds since t0 into series.
func (p *perfLog) add(series *[]float64, t0 time.Time) {
	if !p.enabled || t0.IsZero() {
		return
	}
	*series = append(*series, float64(time.Since(t0))/1e6)
}

// tick prints and resets the window once per second. Call it once per Update.
func (p *perfLog) tick() {
	if !p.enabled {
		return
	}
	if p.windowStart.IsZero() {
		p.windowStart = time.Now()
		return
	}
	if time.Since(p.windowStart) < time.Second {
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Printf("kutta perf: fps=%.1f tps=%.1f upd_p50=%.2fms upd_p95=%.2fms upd_max=%.2fms solver=%.2fms draw_p50=%.2fms draw_p95=%.2fms field=%.2fms smoke=%.2fms heap=%dMB sys=%dMB gc=%d",
		ebiten.ActualFPS(), ebiten.ActualTPS(),
		percentile(p.upd, 50), percentile(p.upd, 95), percentile(p.upd, 100),
		percentile(p.solver, 50),
		percentile(p.draw, 50), percentile(p.draw, 95),
		percentile(p.field, 50), percentile(p.smoke, 50),
		m.HeapAlloc/(1<<20), m.Sys/(1<<20), m.NumGC)

	p.upd = p.upd[:0]
	p.solver = p.solver[:0]
	p.draw = p.draw[:0]
	p.field = p.field[:0]
	p.smoke = p.smoke[:0]
	p.windowStart = time.Now()
}

// eventf logs a discrete event line (scene load, instability reset, kiosk
// toggle) so a slow frame in the log can be matched to its cause.
func (p *perfLog) eventf(format string, args ...any) {
	if !p.enabled {
		return
	}
	log.Printf("kutta event: "+format, args...)
}

// percentile returns the q-th percentile of samples (q=100 is the maximum).
// samples is reordered in place; the per-second window makes that harmless.
func percentile(samples []float64, q int) float64 {
	if len(samples) == 0 {
		return 0
	}
	slices.Sort(samples)
	if q >= 100 {
		return samples[len(samples)-1]
	}
	idx := (len(samples) - 1) * q / 100
	return samples[idx]
}

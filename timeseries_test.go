package tgraph

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func baseTime() time.Time {
	return time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
}

func renderLines(out string) []string {
	return strings.Split(strings.TrimRight(out, "\n"), "\n")
}

// A spike inside the window must survive column aggregation: it appears
// on the top canvas row and sets the max value label.
func TestTimeChartSpikePreserved(t *testing.T) {
	c := NewTimeChart(40, 8, 5*time.Minute)
	s := c.AddSeries("bitrate", "")
	end := baseTime()
	start := end.Add(-5 * time.Minute)
	for i := 0; i < 300; i++ {
		v := 10.0
		if i == 150 {
			v = 100.0 // spike
		}
		c.Add(s, start.Add(time.Duration(i)*time.Second), v)
	}
	lines := renderLines(c.Render())
	// legend + blank + 8 canvas rows + axis + ticks
	assert.Equal(t, 12, len(lines))
	canvas := lines[2:10]
	assert.Contains(t, canvas[0], "100")
	assert.Contains(t, canvas[0], "●")
	// Every canvas/axis/tick line fills exactly the viewport width.
	for _, l := range append(canvas, lines[10], lines[11]) {
		assert.Equal(t, 40, len([]rune(l)))
	}
}

// A gap in the samples must show as blank columns, not interpolation.
func TestTimeChartGapLeftBlank(t *testing.T) {
	c := NewTimeChart(40, 6, 5*time.Minute)
	s := c.AddSeries("s", "")
	end := baseTime()
	start := end.Add(-5 * time.Minute)
	// Samples only in the first and last 30 seconds of the window.
	for i := 0; i <= 30; i++ {
		c.Add(s, start.Add(time.Duration(i)*time.Second), 5)
		c.Add(s, end.Add(-time.Duration(i)*time.Second), 5)
	}
	lines := renderLines(c.Render())
	canvas := lines[2:8]
	sep := strings.IndexRune(canvas[0], '│')
	midCol := sep + 1 + (40-sep-1)/2
	for _, l := range canvas {
		assert.Equal(t, ' ', []rune(l)[midCol])
	}
	// But the edges of the window are drawn.
	assert.Contains(t, canvas[len(canvas)/2], "●")
}

// Samples older than the window must not affect the visible range.
func TestTimeChartOldSamplesOutsideWindow(t *testing.T) {
	c := NewTimeChart(40, 6, 5*time.Minute)
	s := c.AddSeries("s", "")
	end := baseTime()
	c.Add(s, end.Add(-time.Hour), 100000) // ancient spike
	for i := 0; i < 60; i++ {
		c.Add(s, end.Add(-time.Duration(i)*time.Second), 10)
	}
	out := c.Render()
	assert.NotContains(t, out, "100000")
	assert.Contains(t, out, "10")
}

// Out-of-order samples are sorted in; several values at the same
// timestamp are all kept, so the column spans valley to peak.
func TestTimeChartOutOfOrderAndSameTimestamp(t *testing.T) {
	c := NewTimeChart(40, 6, 5*time.Minute)
	s := c.AddSeries("s", "")
	end := baseTime()
	c.Add(s, end, 20)
	c.Add(s, end.Add(-time.Minute), 5)  // arrives after a newer sample
	c.Add(s, end.Add(-time.Minute), 15) // same timestamp, second value
	lines := renderLines(c.Render())
	canvas := lines[2:8]
	sep := strings.IndexRune(canvas[0], '│')
	cw := 40 - sep - 1
	col := sep + 1 + int(float64(cw-1)*4.0/5.0) // t = end-1min
	filled := 0
	for _, l := range canvas {
		if []rune(l)[col] == '●' {
			filled++
		}
	}
	// min 5 / max 20 over the column must fill a vertical span.
	assert.Greater(t, filled, 1)
}

// NaN and ±Inf samples are dropped, counted, and never drawn.
func TestTimeChartNonFiniteDropped(t *testing.T) {
	c := NewTimeChart(40, 6, 5*time.Minute)
	s := c.AddSeries("s", "")
	end := baseTime()
	c.Add(s, end, math.NaN())
	c.Add(s, end, math.Inf(1))
	c.Add(s, end, math.Inf(-1))
	assert.Equal(t, 3, c.Dropped)
	c.Add(s, end, 1)
	assert.Contains(t, c.Render(), "●")
}

// Memory stays bounded: old samples are evicted once MaxSamples is hit.
func TestTimeChartMemoryCap(t *testing.T) {
	c := NewTimeChart(40, 6, 5*time.Minute)
	c.MaxSamples = 100
	s := c.AddSeries("s", "")
	end := baseTime()
	for i := 0; i < 1000; i++ {
		c.Add(s, end.Add(time.Duration(i)*time.Second), float64(i))
	}
	assert.Equal(t, 100, len(c.series[0].samples))
	assert.Equal(t, 900, c.Dropped)
	// The kept samples are the newest ones.
	assert.Equal(t, 999.0, c.series[0].samples[99].V)
}

// A shrinking terminal is clamped to the minimum viewport; layout holds.
func TestTimeChartNarrowTerminal(t *testing.T) {
	c := NewTimeChart(2, 1, time.Minute)
	s := c.AddSeries("s", "")
	c.Add(s, baseTime(), 1)
	lines := renderLines(c.Render())
	// legend + blank + 3 canvas rows + axis + ticks
	assert.Equal(t, 7, len(lines))
	for _, l := range lines[2:] {
		assert.Equal(t, minTimeChartWidth, len([]rune(l)))
	}
}

// Multiple series get distinct glyphs and legend entries.
func TestTimeChartMultiSeriesLegend(t *testing.T) {
	c := NewTimeChart(50, 6, 5*time.Minute)
	a := c.AddSeries("in", "")
	b := c.AddSeries("out", "")
	end := baseTime()
	for i := 0; i < 60; i++ {
		c.Add(a, end.Add(-time.Duration(i)*time.Second), float64(i))
		c.Add(b, end.Add(-time.Duration(i)*time.Second), float64(60-i))
	}
	out := c.Render()
	assert.Contains(t, out, "● in")
	assert.Contains(t, out, "■ out")
	lines := renderLines(out)
	assert.Equal(t, 2+1+6+2, len(lines))
}

// Time ticks are drawn along the axis and fit the viewport.
func TestTimeChartTimeTicks(t *testing.T) {
	c := NewTimeChart(60, 6, 5*time.Minute)
	s := c.AddSeries("s", "")
	end := baseTime()
	c.Add(s, end, 1)
	lines := renderLines(c.Render())
	ticks := lines[len(lines)-1]
	assert.Contains(t, ticks, "12:00:00") // newest sample at the right edge
	assert.Contains(t, ticks, "11:55:00") // window start at the left edge
	assert.Equal(t, 60, len([]rune(ticks)))
}

// An empty chart still renders a well-formed grid.
func TestTimeChartEmpty(t *testing.T) {
	c := NewTimeChart(30, 5, time.Minute)
	lines := renderLines(c.Render())
	assert.Equal(t, 5+2, len(lines)) // no legend lines
	for _, l := range lines {
		assert.Equal(t, 30, len([]rune(l)))
	}
}

// Unknown series indexes are ignored instead of panicking.
func TestTimeChartAddUnknownSeries(t *testing.T) {
	c := NewTimeChart(30, 5, time.Minute)
	c.Add(0, baseTime(), 1)
	c.Add(-1, baseTime(), 1)
	assert.Equal(t, 0, c.Dropped)
}

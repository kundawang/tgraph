package tgraph

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
)

const (
	defaultTimeChartWidth  = 60
	defaultTimeChartHeight = 10
	minTimeChartWidth      = 10
	minTimeChartHeight     = 3

	// DefaultMaxSamples bounds how many samples are kept per series, so
	// memory stays flat no matter how long the chart has been running.
	DefaultMaxSamples = 10000

	// DefaultWindow is the default sliding time window (5 minutes).
	DefaultWindow = 5 * time.Minute
)

// Glyphs used to draw each series, in series order.
var seriesGlyphs = []rune{'●', '■', '▲', '◆', '○', '□', '△', '◇'}

// Sample is a single timestamped measurement.
type Sample struct {
	T time.Time
	V float64
}

type timeSeries struct {
	name    string
	color   string
	samples []Sample // kept sorted by T
}

// TimeChart draws a scrolling time-series chart into a fixed-width,
// fixed-height terminal viewport. Only the most recent Window of samples
// is visible; as time advances the window slides forward. Each terminal
// column aggregates every sample that falls into it, keeping both the
// peak and the valley, so spikes are never smoothed away. Columns with
// no samples are left blank, making gaps in the data visible.
type TimeChart struct {
	// Width and Height of the viewport in terminal cells. Values below
	// the minimum (or zero) are clamped so a shrinking terminal never
	// breaks the layout.
	Width  int
	Height int
	// Window is the visible time span. Defaults to DefaultWindow.
	Window time.Duration
	// MaxSamples caps the samples kept per series. Oldest samples are
	// evicted first. Defaults to DefaultMaxSamples.
	MaxSamples int
	// Dropped counts samples that were not stored: non-finite values
	// (NaN, ±Inf) and samples evicted by MaxSamples.
	Dropped int

	series []*timeSeries
}

// NewTimeChart returns a TimeChart with the given viewport size and
// sliding window.
func NewTimeChart(width, height int, window time.Duration) *TimeChart {
	return &TimeChart{
		Width:      width,
		Height:     height,
		Window:     window,
		MaxSamples: DefaultMaxSamples,
	}
}

// AddSeries registers a new series and returns its index, which is
// passed to Add. colorName may be one of the names in colorMap or empty.
func (c *TimeChart) AddSeries(name, colorName string) int {
	c.series = append(c.series, &timeSeries{name: name, color: colorName})
	return len(c.series) - 1
}

// Add records a sample for the series idx. Samples may arrive out of
// order: they are inserted sorted by timestamp, and several values at
// the same timestamp are all kept. Non-finite values are dropped.
// Unknown series indexes are ignored.
func (c *TimeChart) Add(idx int, t time.Time, v float64) {
	if idx < 0 || idx >= len(c.series) {
		return
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		c.Dropped++
		return
	}
	s := c.series[idx]
	// Insert sorted by time; equal timestamps land after existing ones.
	i := sort.Search(len(s.samples), func(i int) bool { return s.samples[i].T.After(t) })
	s.samples = append(s.samples, Sample{})
	copy(s.samples[i+1:], s.samples[i:])
	s.samples[i] = Sample{T: t, V: v}

	maxSamples := c.MaxSamples
	if maxSamples <= 0 {
		maxSamples = DefaultMaxSamples
	}
	if len(s.samples) > maxSamples {
		excess := len(s.samples) - maxSamples
		// Copy so the evicted prefix's backing array can be collected.
		s.samples = append([]Sample(nil), s.samples[excess:]...)
		c.Dropped += excess
	}
}

// windowEnd returns the right edge of the sliding window: the newest
// sample timestamp across all series, or the zero time when empty.
func (c *TimeChart) windowEnd() time.Time {
	var end time.Time
	for _, s := range c.series {
		if n := len(s.samples); n > 0 && s.samples[n-1].T.After(end) {
			end = s.samples[n-1].T
		}
	}
	return end
}

// Render draws the chart and returns it as a string. The layout is:
// one legend line per series, a blank line, Height canvas rows with
// min/max value labels, an axis row and a row of time tick labels.
// Every canvas, axis and tick line is exactly Width cells wide.
func (c *TimeChart) Render() string {
	width := c.Width
	if width <= 0 {
		width = defaultTimeChartWidth
	}
	if width < minTimeChartWidth {
		width = minTimeChartWidth
	}
	height := c.Height
	if height <= 0 {
		height = defaultTimeChartHeight
	}
	if height < minTimeChartHeight {
		height = minTimeChartHeight
	}
	window := c.Window
	if window <= 0 {
		window = DefaultWindow
	}

	end := c.windowEnd()
	start := end.Add(-window)

	// Value range over the visible samples only.
	vmin, vmax := math.Inf(1), math.Inf(-1)
	for _, s := range c.series {
		for _, sm := range s.samples {
			if sm.T.Before(start) || sm.T.After(end) {
				continue
			}
			if sm.V < vmin {
				vmin = sm.V
			}
			if sm.V > vmax {
				vmax = sm.V
			}
		}
	}
	if vmin > vmax { // no visible samples
		vmin, vmax = 0, 1
	}
	if vmin == vmax {
		vmin -= 0.5
		vmax += 0.5
	}

	maxLabel := formatAxisValue(vmax)
	minLabel := formatAxisValue(vmin)
	yl := len(maxLabel)
	if len(minLabel) > yl {
		yl = len(minLabel)
	}
	cw := width - yl - 1 // y labels + '│' separator
	if cw < 1 {
		cw = 1
	}

	// Aggregate samples into columns, keeping peak and valley per column.
	type colAgg struct {
		min, max float64
		ok       bool
	}
	aggs := make([][]colAgg, len(c.series))
	for si, s := range c.series {
		a := make([]colAgg, cw)
		for _, sm := range s.samples {
			if sm.T.Before(start) || sm.T.After(end) {
				continue
			}
			col := int(float64(cw-1) * float64(sm.T.Sub(start)) / float64(window))
			if col < 0 {
				col = 0
			}
			if col >= cw {
				col = cw - 1
			}
			if !a[col].ok {
				a[col].min, a[col].max, a[col].ok = sm.V, sm.V, true
			} else {
				if sm.V < a[col].min {
					a[col].min = sm.V
				}
				if sm.V > a[col].max {
					a[col].max = sm.V
				}
			}
		}
		aggs[si] = a
	}

	rowOf := func(v float64) int {
		r := int(math.Round(float64(height-1) * (vmax - v) / (vmax - vmin)))
		if r < 0 {
			r = 0
		}
		if r >= height {
			r = height - 1
		}
		return r
	}

	grid := make([][]rune, height)
	for r := range grid {
		grid[r] = make([]rune, cw)
		for i := range grid[r] {
			grid[r][i] = ' '
		}
	}
	// Fill each occupied column from its valley to its peak. Columns
	// without samples stay blank, so gaps in the data show as gaps in
	// the curve. Later series draw over earlier ones on overlap.
	for si := range c.series {
		g := seriesGlyphs[si%len(seriesGlyphs)]
		for col, a := range aggs[si] {
			if !a.ok {
				continue
			}
			for r := rowOf(a.max); r <= rowOf(a.min); r++ {
				grid[r][col] = g
			}
		}
	}

	var b strings.Builder
	// Legend, one line per series.
	for si, s := range c.series {
		name := s.name
		if name == "" {
			name = "series" + strconv.Itoa(si)
		}
		g := seriesGlyphs[si%len(seriesGlyphs)]
		if attr, ok := colorMap[s.color]; ok {
			color.New(attr).Fprintf(&b, "%c", g)
		} else {
			fmt.Fprintf(&b, "%c", g)
		}
		fmt.Fprintf(&b, " %s\n", name)
	}
	if len(c.series) > 0 {
		b.WriteByte('\n')
	}
	// Canvas rows, with max/min value labels on the first/last row.
	for r := 0; r < height; r++ {
		var lbl string
		if r == 0 {
			lbl = maxLabel
		} else if r == height-1 {
			lbl = minLabel
		}
		fmt.Fprintf(&b, "%*s│%s\n", yl, lbl, string(grid[r]))
	}
	// Axis and time tick labels.
	b.WriteString(strings.Repeat(" ", yl))
	b.WriteString("└")
	b.WriteString(strings.Repeat("─", cw))
	b.WriteByte('\n')
	b.WriteString(strings.Repeat(" ", yl+1))
	b.WriteString(tickLabels(start, window, cw))
	b.WriteByte('\n')
	return b.String()
}

// Draw prints the chart to stdout.
func (c *TimeChart) Draw() {
	fmt.Print(c.Render())
}

// tickLabels lays out "HH:MM:SS" labels along the axis, skipping any
// that would overlap a previous label. Returns a blank row when the
// canvas is too narrow for labels.
func tickLabels(start time.Time, window time.Duration, cw int) string {
	row := make([]rune, cw)
	for i := range row {
		row[i] = ' '
	}
	const labelLen = 8 // len("15:04:05")
	if cw < labelLen+2 {
		return string(row)
	}
	n := cw / (labelLen + 2)
	if n < 2 {
		n = 2
	}
	prevEnd := -1
	for i := 0; i < n; i++ {
		col := 0
		if n > 1 {
			col = i * (cw - 1) / (n - 1)
		}
		span := int64(window)
		if cw > 1 {
			span = int64(window) * int64(col) / int64(cw-1)
		}
		lbl := start.Add(time.Duration(span)).Format("15:04:05")
		pos := col - labelLen/2
		if pos < 0 {
			pos = 0
		}
		if pos+labelLen > cw {
			pos = cw - labelLen
		}
		if pos <= prevEnd {
			continue
		}
		for j, ch := range lbl {
			row[pos+j] = ch
		}
		prevEnd = pos + labelLen
	}
	return string(row)
}

// formatAxisValue formats a value label compactly, e.g. 100, 0.5, -3.14.
func formatAxisValue(v float64) string {
	return strconv.FormatFloat(v, 'g', 5, 64)
}

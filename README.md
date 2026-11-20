# Tgraph

[![GoDoc](https://img.shields.io/static/v1?label=godoc&message=reference&color=blue)](https://pkg.go.dev/mod/github.com/daoleno/tgraph)
![Build](https://github.com/daoleno/tgraph/workflows/Build/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/daoleno/tgraph)](https://goreportcard.com/report/github.com/daoleno/tgraph)

Tgraph is a Go package for drawing basic graphics in a terminal. It was inspired by [termgraph](https://github.com/mkaz/termgraph).

Graph types supported:

- Bar Graphs
- Multi-variable
- Stacked charts
- Emoji
- Scrolling time-series charts

## Examples

[\_examples](./_examples/README.md) have a cli tool that shows how to use the package.

## Time-series charts

`TimeChart` draws a scrolling curve of timestamped samples into a
fixed-size terminal viewport — e.g. a live bitrate monitor showing the
last 5 minutes:

```go
c := tgraph.NewTimeChart(70, 10, 5*time.Minute)
in := c.AddSeries("bitrate-in", "green")
for sample := range samples {
	c.Add(in, sample.T, sample.V)
	c.Draw()
}
```

- Multiple series with distinct glyphs, colors and a legend.
- Each terminal column keeps the peak and valley of the samples that
  fall into it, so spikes are never smoothed away; columns without
  samples stay blank, making dropouts visible.
- Out-of-order samples are sorted in, several values at the same
  timestamp are all kept, and non-finite values (NaN, ±Inf) are dropped.
- Memory is bounded by `MaxSamples` (default 10000 per series); the
  oldest samples are evicted first and counted in `Dropped`.

## License

[MIT](LICENSE) License

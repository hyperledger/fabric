/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package gendoc

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/hyperledger/fabric/common/metrics"
	"github.com/hyperledger/fabric/common/metrics/internal/namer"
)

// A Field represents data that is included in the reference table for metrics.
type Field uint8

const (
	Name        Field = iota // Name is the meter name.
	Type                     // Type is the type of meter option.
	Description              // Description is the help text from the meter option.
	Labels                   // Labels is the meter's label information.
	Bucket                   // Bucket is the statsd bucket format
)

// A Column represents a column of data in the reference table.
type Column struct {
	Field Field
	Name  string
	Width int
}

// NewPrometheusTable creates a table that can be used to document Prometheus
// metrics maintained by Fabric.
func NewPrometheusTable(cells Cells) Table {
	return Table{
		Cells: cells,
		Columns: []Column{
			{Field: Name, Name: "Name", Width: max(cells.MaxLen(Name)+2, 20)},
			{Field: Type, Name: "Type", Width: 11},
			{Field: Description, Name: "Description", Width: 60},
			{Field: Labels, Name: "Labels", Width: 20},
		},
	}
}

// NewStatsdTable creates a table that can be used to document StatsD metrics
// maintained by Fabric.
func NewStatsdTable(cells Cells) Table {
	return Table{
		Cells: cells,
		Columns: []Column{
			{Field: Bucket, Name: "Bucket", Width: max(cells.MaxLen(Bucket)+2, 20)},
			{Field: Type, Name: "Type", Width: 11},
			{Field: Description, Name: "Description", Width: 60},
		},
	}
}

// A Table maintains the cells and columns used to generate the restructured text
// formatted reference documentation.
type Table struct {
	Columns []Column
	Cells   Cells
}

// Generate generates a restructured text formatted table from the cells and
// columns contained in the table.
func (t Table) Generate(w io.Writer) {
	fmt.Fprint(w, t.header())
	for _, c := range t.Cells {
		fmt.Fprint(w, t.formatCell(c))
		fmt.Fprint(w, t.rowSeparator())
	}
}

func (t Table) rowSeparator() string    { return t.separator("-") }
func (t Table) headerSeparator() string { return t.separator("=") }

func (t Table) separator(delim string) string {
	var s string
	for _, c := range t.Columns {
		s += "+" + strings.Repeat(delim, c.Width)
	}
	return s + "+\n"
}

func (t Table) header() string {
	var h string
	h += t.rowSeparator()
	for _, c := range t.Columns {
		h += "| " + printWidth(c.Name, c.Width-2) + " "
	}
	h += "|\n"
	h += t.headerSeparator()
	return h
}

func (t Table) formatCell(cell Cell) string {
	contents := map[Field][]string{}
	lineCount := 0

	// wrap lines
	for _, c := range t.Columns {
		lines := wrapWidths(cell.Field(c.Field), c.Width-2)
		if l := len(lines); l > lineCount {
			lineCount = l
		}
		contents[c.Field] = lines
	}

	// add extra lines
	for _, col := range t.Columns {
		lines := contents[col.Field]
		contents[col.Field] = padLines(lines, col.Width-2, lineCount)
	}

	var c string
	for i := 0; i < lineCount; i++ {
		for _, col := range t.Columns {
			c += "| " + contents[col.Field][i] + " "
		}
		c += "|\n"
	}

	return c
}

func wrapWidths(s string, width int) []string {
	var result []string
	for _, s := range strings.Split(s, "\n") {
		result = append(result, wrapWidth(s, width)...)
	}
	return result
}

func wrapWidth(s string, width int) []string {
	words := strings.Fields(strings.TrimSpace(s))
	if len(words) == 0 { // only white space
		return []string{s}
	}

	result := words[0]
	remaining := width - len(words[0])
	for _, w := range words[1:] {
		if len(w)+1 > remaining {
			result += "\n" + w
			remaining = width - len(w) - 1
		} else {
			result += " " + w
			remaining -= len(w) + 1
		}
	}

	return strings.Split(result, "\n")
}

func padLines(lines []string, w, h int) []string {
	for len(lines) < h {
		lines = append(lines, "")
	}
	for idx, line := range lines {
		lines[idx] = printWidth(line, w)
	}

	return lines
}

func printWidth(s string, w int) string {
	s = strings.TrimSpace(s)
	if len(s) < w {
		return s + strings.Repeat(" ", w-len(s))
	}
	return s
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

type Cell struct {
	meterType   string
	namer       *namer.Namer
	description string
	labels      []string
}

func (c Cell) Field(f Field) string {
	switch f {
	case Name:
		return c.Name()
	case Type:
		return c.Type()
	case Description:
		return c.Description()
	case Labels:
		return c.Labels()
	case Bucket:
		return c.BucketFormat()
	default:
		panic(fmt.Sprintf("unknown field type: %d", f))
	}
}

func (c Cell) Name() string        { return strings.Replace(c.namer.FullyQualifiedName(), ".", "_", -1) }
func (c Cell) Type() string        { return c.meterType }
func (c Cell) Description() string { return c.description }
func (c Cell) Labels() string      { return strings.Join(c.labels, "\n") }

func (c Cell) BucketFormat() string {
	var lvs []string
	for _, label := range c.labels {
		lvs = append(lvs, label, asBucketVar(label))
	}
	return c.namer.Format(lvs...)
}

func asBucketVar(s string) string { return "%{" + s + "}" }

type Cells []Cell

func (c Cells) Len() int           { return len(c) }
func (c Cells) Less(i, j int) bool { return c[i].Name() < c[j].Name() }
func (c Cells) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }

func (c Cells) MaxLen(f Field) int {
	var maxlen int
	for _, c := range c {
		if l := len(c.Field(f)); l > maxlen {
			maxlen = l
		}
	}
	return maxlen
}

// NewCells transforms metrics options to cells that can be used for doc
// generation.
func NewCells(options []interface{}) (Cells, error) {
	var cells Cells
	for _, o := range options {
		switch m := o.(type) {
		case metrics.CounterOpts:
			cells = append(cells, counterCell(m))
		case metrics.GaugeOpts:
			cells = append(cells, gaugeCell(m))
		case metrics.HistogramOpts:
			cells = append(cells, histogramCell(m))
		default:
			return nil, fmt.Errorf("unknown option type: %t", o)
		}
	}
	sort.Sort(cells)
	return cells, nil
}

func counterCell(c metrics.CounterOpts) Cell {
	if c.StatsdFormat == "" {
		c.StatsdFormat = "%{#fqname}"
	}
	return Cell{
		namer:       namer.NewCounterNamer(c),
		meterType:   "counter",
		description: c.Help,
		labels:      c.LabelNames,
	}
}

func gaugeCell(g metrics.GaugeOpts) Cell {
	if g.StatsdFormat == "" {
		g.StatsdFormat = "%{#fqname}"
	}
	return Cell{
		namer:       namer.NewGaugeNamer(g),
		meterType:   "gauge",
		description: g.Help,
		labels:      g.LabelNames,
	}
}

func histogramCell(h metrics.HistogramOpts) Cell {
	if h.StatsdFormat == "" {
		h.StatsdFormat = "%{#fqname}"
	}
	return Cell{
		namer:       namer.NewHistogramNamer(h),
		meterType:   "histogram",
		description: h.Help,
		labels:      h.LabelNames,
	}
}

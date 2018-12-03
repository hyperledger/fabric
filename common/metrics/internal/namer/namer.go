/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package namer

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hyperledger/fabric/common/metrics"
)

type Namer struct {
	namespace  string
	subsystem  string
	name       string
	nameFormat string
	labelNames map[string]struct{}
}

func NewCounterNamer(c metrics.CounterOpts) *Namer {
	return &Namer{
		namespace:  c.Namespace,
		subsystem:  c.Subsystem,
		name:       c.Name,
		nameFormat: c.StatsdFormat,
		labelNames: sliceToSet(c.LabelNames),
	}
}

func NewGaugeNamer(g metrics.GaugeOpts) *Namer {
	return &Namer{
		namespace:  g.Namespace,
		subsystem:  g.Subsystem,
		name:       g.Name,
		nameFormat: g.StatsdFormat,
		labelNames: sliceToSet(g.LabelNames),
	}
}

func NewHistogramNamer(h metrics.HistogramOpts) *Namer {
	return &Namer{
		namespace:  h.Namespace,
		subsystem:  h.Subsystem,
		name:       h.Name,
		nameFormat: h.StatsdFormat,
		labelNames: sliceToSet(h.LabelNames),
	}
}

func (n *Namer) validateKey(name string) {
	if _, ok := n.labelNames[name]; !ok {
		panic("invalid label name: " + name)
	}
}

func (n *Namer) FullyQualifiedName() string {
	switch {
	case n.namespace != "" && n.subsystem != "":
		return strings.Join([]string{n.namespace, n.subsystem, n.name}, ".")
	case n.namespace != "":
		return strings.Join([]string{n.namespace, n.name}, ".")
	case n.subsystem != "":
		return strings.Join([]string{n.subsystem, n.name}, ".")
	default:
		return n.name
	}
}

func (n *Namer) labelsToMap(labelValues []string) map[string]string {
	labels := map[string]string{}
	for i := 0; i < len(labelValues); i += 2 {
		key := labelValues[i]
		n.validateKey(key)
		if i == len(labelValues)-1 {
			labels[key] = "unknown"
		} else {
			labels[key] = labelValues[i+1]
		}
	}
	return labels
}

var formatRegexp = regexp.MustCompile(`%{([#?[:alnum:]_]+)}`)
var invalidLabelValueRegexp = regexp.MustCompile(`[.|:\s]`)

func (n *Namer) Format(labelValues ...string) string {
	labels := n.labelsToMap(labelValues)

	cursor := 0
	var segments []string
	// iterate over the regex groups and convert to formatters
	matches := formatRegexp.FindAllStringSubmatchIndex(n.nameFormat, -1)
	for _, m := range matches {
		start, end := m[0], m[1]
		labelStart, labelEnd := m[2], m[3]

		if start > cursor {
			segments = append(segments, n.nameFormat[cursor:start])
		}

		key := n.nameFormat[labelStart:labelEnd]
		var value string
		switch key {
		case "#namespace":
			value = n.namespace
		case "#subsystem":
			value = n.subsystem
		case "#name":
			value = n.name
		case "#fqname":
			value = n.FullyQualifiedName()
		default:
			var ok bool
			value, ok = labels[key]
			if !ok {
				panic(fmt.Sprintf("invalid label in name format: %s", key))
			}
			value = invalidLabelValueRegexp.ReplaceAllString(value, "_")
		}
		segments = append(segments, value)

		cursor = end
	}

	// handle any trailing suffix
	if cursor != len(n.nameFormat) {
		segments = append(segments, n.nameFormat[cursor:])
	}

	return strings.Join(segments, "")
}

func sliceToSet(set []string) map[string]struct{} {
	labelSet := map[string]struct{}{}
	for _, s := range set {
		labelSet[s] = struct{}{}
	}
	return labelSet
}

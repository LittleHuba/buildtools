/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"encoding/json"
	"path/filepath"
)

const (
	sarifSchema       = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion      = "2.1.0"
	sarifToolURI      = "https://github.com/bazel-contrib/buildtools/tree/main/buildifier"
	sarifLevelWarning = "warning"
	sarifLevelError   = "error"
	sarifFormatRule   = "format"
	sarifParseRule    = "parse-error"
)

// Minimal subset of SARIF 2.1.0 needed to report buildifier diagnostics.

type sarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool    sarifTool     `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifTool struct {
	Driver sarifDriver `json:"driver"`
}

type sarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Rules          []sarifRule `json:"rules"`
}

type sarifRule struct {
	ID               string        `json:"id"`
	ShortDescription *sarifMessage `json:"shortDescription,omitempty"`
	HelpURI          string        `json:"helpUri,omitempty"`
}

type sarifResult struct {
	RuleID    string          `json:"ruleId"`
	RuleIndex int             `json:"ruleIndex"`
	Level     string          `json:"level"`
	Message   sarifMessage    `json:"message"`
	Locations []sarifLocation `json:"locations"`
}

type sarifMessage struct {
	Text string `json:"text"`
}

type sarifLocation struct {
	PhysicalLocation sarifPhysicalLocation `json:"physicalLocation"`
}

type sarifPhysicalLocation struct {
	ArtifactLocation sarifArtifactLocation `json:"artifactLocation"`
	Region           *sarifRegion          `json:"region,omitempty"`
}

type sarifArtifactLocation struct {
	URI string `json:"uri"`
}

type sarifRegion struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn,omitempty"`
	EndLine     int `json:"endLine,omitempty"`
	EndColumn   int `json:"endColumn,omitempty"`
}

// sarifBuilder collects rules and results, deduplicating rules by id.
type sarifBuilder struct {
	rules   []sarifRule
	indexes map[string]int
	results []sarifResult
}

func (b *sarifBuilder) ruleIndex(id, description, helpURI string) int {
	if i, ok := b.indexes[id]; ok {
		return i
	}
	rule := sarifRule{ID: id, HelpURI: helpURI}
	if description != "" {
		rule.ShortDescription = &sarifMessage{Text: description}
	}
	b.rules = append(b.rules, rule)
	b.indexes[id] = len(b.rules) - 1
	return b.indexes[id]
}

func (b *sarifBuilder) add(ruleID, description, helpURI, level, message, filename string, region *sarifRegion) {
	b.results = append(b.results, sarifResult{
		RuleID:    ruleID,
		RuleIndex: b.ruleIndex(ruleID, description, helpURI),
		Level:     level,
		Message:   sarifMessage{Text: message},
		Locations: []sarifLocation{{
			PhysicalLocation: sarifPhysicalLocation{
				ArtifactLocation: sarifArtifactLocation{URI: filepath.ToSlash(filename)},
				Region:           region,
			},
		}},
	})
}

// formatSarif renders the diagnostics as a SARIF 2.1.0 log.
func (d *Diagnostics) formatSarif(verbose bool) string {
	b := &sarifBuilder{indexes: map[string]int{}}

	for _, f := range d.Files {
		for _, w := range f.Warnings {
			// Without a start line the result is reported at file level.
			var region *sarifRegion
			if w.Start.Line > 0 {
				region = &sarifRegion{
					StartLine:   w.Start.Line,
					StartColumn: w.Start.Column,
					EndLine:     w.End.Line,
					EndColumn:   w.End.Column,
				}
			}
			b.add(w.Category, "", w.URL, sarifLevelWarning, w.Message, f.Filename, region)
		}
		switch {
		case !f.Valid:
			b.add(sarifParseRule, "The file is not a valid Starlark file.", "", sarifLevelError,
				"The file could not be parsed.", f.Filename, nil)
		case !f.Formatted:
			b.add(sarifFormatRule, "The file is not formatted according to buildifier's rules.", "", sarifLevelWarning,
				"The file needs to be reformatted.", f.Filename, nil)
		}
	}

	if b.rules == nil {
		b.rules = []sarifRule{}
	}
	if b.results == nil {
		b.results = []sarifResult{}
	}

	log := sarifLog{
		Schema:  sarifSchema,
		Version: sarifVersion,
		Runs: []sarifRun{{
			Tool: sarifTool{Driver: sarifDriver{
				Name:           "buildifier",
				InformationURI: sarifToolURI,
				Rules:          b.rules,
			}},
			Results: b.results,
		}},
	}

	var out []byte
	if verbose {
		out, _ = json.MarshalIndent(log, "", "    ")
	} else {
		out, _ = json.Marshal(log)
	}
	return string(out) + "\n"
}

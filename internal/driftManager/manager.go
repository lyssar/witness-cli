package driftmanager

import "strings"

type DriftFlag uint32

const (
	NewInSourceMissingInTarget DriftFlag = 1 << iota
	TargetRemovedButSourceExists
	SourceRemovedButTargetExists
	SourceChanged
	TargetChangedManually
)

func (df DriftFlag) String() string {
	var parts []string
	if df&NewInSourceMissingInTarget != 0 {
		parts = append(parts, "NewInSourceMissingInTarget")
	}
	if df&TargetRemovedButSourceExists != 0 {
		parts = append(parts, "TargetRemovedButSourceExists")
	}
	if df&SourceRemovedButTargetExists != 0 {
		parts = append(parts, "SourceRemovedButTargetExists")
	}
	if df&SourceChanged != 0 {
		parts = append(parts, "SourceChanged")
	}
	if df&TargetChangedManually != 0 {
		parts = append(parts, "TargetChangedManually")
	}
	return strings.Join(parts, "|")
}

type ResourceDrift struct {
	Path       string
	Flags      DriftFlag
	SourceSha  string
	TargetSha  string
	SourceBase string
	TargetBase string
}

type Snapshot struct {
	Version   int                         `json:"version"`
	Resources map[string]ResourceSnapshot `json:"resources"`
}

type ResourceSnapshot struct {
	SourceSha string `json:"source_sha"`
	TargetSha string `json:"target_sha"`
}

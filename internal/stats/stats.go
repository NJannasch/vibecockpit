package stats

import (
	"math"
	"sort"
	"time"

	"vibecockpit/internal/inventory"
	"vibecockpit/internal/provider"
)

type TimelineEvent struct {
	Date     string `json:"date"`
	Type     string `json:"type"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Detail   string `json:"detail,omitempty"`
	Provider string `json:"provider,omitempty"`
	Project  string `json:"project,omitempty"`
}

type ToolStats struct {
	Name          string `json:"name"`
	Provider      string `json:"provider"`
	FirstSession  string `json:"firstSession"`
	LastSession   string `json:"lastSession"`
	TotalSessions int    `json:"totalSessions"`
	ActivityDays  int    `json:"activityDays"`
}

type ArtifactEntry struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Project  string `json:"project,omitempty"`
	Source   string `json:"source,omitempty"`
	Date     string `json:"date"`
	Category string `json:"category"`
}

type Summary struct {
	FirstActivityDate   string `json:"firstActivityDate"`
	DaysSinceFirstUse   int    `json:"daysSinceFirstUse"`
	TotalSessions       int    `json:"totalSessions"`
	MostActiveTool      string `json:"mostActiveTool"`
	MostActiveProject   string `json:"mostActiveProject"`
	TotalProjects       int    `json:"totalProjects"`
	ProjectsWithConfigs int    `json:"projectsWithConfigs"`
	TotalTools          int    `json:"totalTools"`
	TotalExtensions     int    `json:"totalExtensions"`
	TotalMemories       int    `json:"totalMemories"`
	TotalInstructions   int    `json:"totalInstructions"`
}

type AdoptionStats struct {
	Timeline  []TimelineEvent `json:"timeline"`
	Tools     []ToolStats     `json:"tools"`
	Artifacts []ArtifactEntry `json:"artifacts"`
	Summary   Summary         `json:"summary"`
}

func Compute(sessions []provider.Session, inv *inventory.Inventory) *AdoptionStats {
	stats := &AdoptionStats{}

	toolMap := buildToolStats(sessions)
	stats.Tools = sortedToolStats(toolMap)
	stats.Timeline = buildTimeline(sessions, inv, toolMap)
	stats.Artifacts = buildArtifacts(inv)
	stats.Summary = buildSummary(sessions, inv, toolMap)

	return stats
}

type toolAgg struct {
	first    time.Time
	last     time.Time
	count    int
	daySet   map[string]bool
}

func buildToolStats(sessions []provider.Session) map[string]*toolAgg {
	m := map[string]*toolAgg{}
	for _, s := range sessions {
		if s.Provider == "" {
			continue
		}
		a := m[s.Provider]
		if a == nil {
			a = &toolAgg{daySet: map[string]bool{}}
			m[s.Provider] = a
		}
		a.count++

		ts := s.Created
		if ts.IsZero() {
			ts = s.Modified
		}
		if ts.IsZero() {
			continue
		}

		if a.first.IsZero() || ts.Before(a.first) {
			a.first = ts
		}
		if ts.After(a.last) {
			a.last = ts
		}
		a.daySet[ts.Format("2006-01-02")] = true

		if !s.Modified.IsZero() {
			if s.Modified.After(a.last) {
				a.last = s.Modified
			}
			a.daySet[s.Modified.Format("2006-01-02")] = true
		}
	}
	return m
}

func sortedToolStats(m map[string]*toolAgg) []ToolStats {
	var out []ToolStats
	for name, a := range m {
		ts := ToolStats{
			Name:          name,
			Provider:      name,
			TotalSessions: a.count,
			ActivityDays:  len(a.daySet),
		}
		if !a.first.IsZero() {
			ts.FirstSession = a.first.Format(time.RFC3339)
		}
		if !a.last.IsZero() {
			ts.LastSession = a.last.Format(time.RFC3339)
		}
		out = append(out, ts)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].TotalSessions > out[j].TotalSessions
	})
	return out
}

func buildTimeline(_ []provider.Session, inv *inventory.Inventory, toolMap map[string]*toolAgg) []TimelineEvent {
	var events []TimelineEvent

	for name, a := range toolMap {
		if a.first.IsZero() {
			continue
		}
		events = append(events, TimelineEvent{
			Date:     a.first.Format(time.RFC3339),
			Type:     "first-session",
			Category: "tool",
			Title:    "First " + name + " session",
			Provider: name,
		})
	}

	if inv != nil {
		for _, f := range inv.InstructionFiles {
			if f.Modified == "" {
				continue
			}
			events = append(events, TimelineEvent{
				Date:     f.Modified,
				Type:     "instruction-file",
				Category: "config",
				Title:    f.Type + " created",
				Detail:   f.Path,
				Project:  f.ProjectName,
			})
		}

		for _, e := range inv.IDEExtensions {
			if e.Installed == "" {
				continue
			}
			events = append(events, TimelineEvent{
				Date:     e.Installed,
				Type:     "extension-install",
				Category: "extension",
				Title:    e.Name + " installed",
				Detail:   e.IDE + " — " + e.ID,
			})
		}

		for _, m := range inv.Memories {
			if m.Modified == "" {
				continue
			}
			events = append(events, TimelineEvent{
				Date:     m.Modified,
				Type:     "memory-created",
				Category: "memory",
				Title:    "Memory: " + m.Name,
				Detail:   m.Description,
				Project:  m.ProjectName,
			})
		}
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Date < events[j].Date
	})

	return events
}

func buildArtifacts(inv *inventory.Inventory) []ArtifactEntry {
	if inv == nil {
		return nil
	}
	var out []ArtifactEntry

	for _, f := range inv.InstructionFiles {
		out = append(out, ArtifactEntry{
			Name:     f.Type,
			Type:     "instruction",
			Project:  f.ProjectName,
			Source:   f.Path,
			Date:     f.Modified,
			Category: "config",
		})
	}

	for _, e := range inv.IDEExtensions {
		out = append(out, ArtifactEntry{
			Name:     e.Name,
			Type:     "extension",
			Source:   e.IDE,
			Date:     e.Installed,
			Category: "extension",
		})
	}

	for _, m := range inv.Memories {
		out = append(out, ArtifactEntry{
			Name:     m.Name,
			Type:     "memory",
			Project:  m.ProjectName,
			Source:   m.MemoryType,
			Date:     m.Modified,
			Category: "memory",
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Date < out[j].Date
	})

	return out
}

func buildSummary(sessions []provider.Session, inv *inventory.Inventory, toolMap map[string]*toolAgg) Summary {
	s := Summary{
		TotalSessions: len(sessions),
	}

	if inv != nil {
		s.TotalTools = len(inv.Tools)
		s.TotalExtensions = len(inv.IDEExtensions)
		s.TotalMemories = len(inv.Memories)
		s.TotalInstructions = len(inv.InstructionFiles)
		s.TotalProjects = len(inv.ProjectPaths)

		projectsWithConfig := map[string]bool{}
		for _, f := range inv.InstructionFiles {
			if f.ProjectName != "" {
				projectsWithConfig[f.ProjectName] = true
			}
		}
		s.ProjectsWithConfigs = len(projectsWithConfig)
	}

	var earliest time.Time
	var maxCount int
	projectCounts := map[string]int{}

	for name, a := range toolMap {
		if !a.first.IsZero() && (earliest.IsZero() || a.first.Before(earliest)) {
			earliest = a.first
		}
		if a.count > maxCount {
			maxCount = a.count
			s.MostActiveTool = name
		}
	}

	for _, sess := range sessions {
		if sess.ProjectName != "" {
			projectCounts[sess.ProjectName]++
		}
	}
	var maxProjCount int
	for name, cnt := range projectCounts {
		if cnt > maxProjCount {
			maxProjCount = cnt
			s.MostActiveProject = name
		}
	}

	if !earliest.IsZero() {
		s.FirstActivityDate = earliest.Format(time.RFC3339)
		s.DaysSinceFirstUse = int(math.Ceil(time.Since(earliest).Hours() / 24))
	}

	return s
}

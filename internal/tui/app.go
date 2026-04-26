package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"vibecockpit/internal/config"
	"vibecockpit/internal/provider"
	"vibecockpit/internal/search"
)

type viewState int

const (
	viewList viewState = iota
	viewNewProject
	viewSettings
	viewConfirmDelete
	viewModelPicker
)

type sortMode int

const (
	sortModified sortMode = iota
	sortCreated
	sortName
	sortMessages
	sortModeCount
)

type groupMode int

const (
	groupNone groupMode = iota
	groupProvider
	groupProject
	groupDate
	groupModeCount
)

func (g groupMode) String() string {
	switch g {
	case groupProvider:
		return "Tool"
	case groupProject:
		return "Project"
	case groupDate:
		return "Date"
	}
	return "None"
}

func (s sortMode) String() string {
	switch s {
	case sortModified:
		return "Modified"
	case sortCreated:
		return "Created"
	case sortName:
		return "Name"
	case sortMessages:
		return "Messages"
	}
	return ""
}

type sessionsLoaded struct {
	sessions []provider.Session
	err      error
}

type Action struct {
	Kind    string
	Session *provider.Session
	Dir     string
}

type Model struct {
	state     viewState
	sessions  []provider.Session
	filtered  []provider.Session
	cursor    int
	offset    int
	width     int
	height    int
	config    *config.Config
	providers []provider.Provider
	action    Action
	quitting  bool
	sortBy    sortMode
	groupBy   groupMode

	filterInput textinput.Model
	filtering   bool
	newDirInput textinput.Model
	termCursor  int
	modelCursor int
	loading     bool
}

func New(cfg *config.Config, providers []provider.Provider) Model {
	fi := textinput.New()
	fi.Prompt = "  Filter: "
	fi.PromptStyle = filterPrompt
	fi.CharLimit = 100

	home, _ := os.UserHomeDir()
	nd := textinput.New()
	nd.Prompt = "  Path: "
	nd.PromptStyle = filterPrompt
	nd.SetValue(filepath.Join(home, "Documents", "Workspace") + "/")
	nd.CharLimit = 256

	termIdx := 0
	for i, t := range config.AvailableTerminals() {
		if t == cfg.Terminal {
			termIdx = i
			break
		}
	}

	return Model{
		config:      cfg,
		providers:   providers,
		filterInput: fi,
		newDirInput: nd,
		termCursor:  termIdx,
		loading:     true,
	}
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		var all []provider.Session
		for _, p := range m.providers {
			sessions, err := p.ScanSessions(context.Background())
			if err != nil {
				return sessionsLoaded{err: err}
			}
			all = append(all, sessions...)
		}
		return sessionsLoaded{sessions: all}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionsLoaded:
		m.loading = false
		m.sessions = msg.sessions
		m.applyFilter()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch m.state {
		case viewList:
			return m.updateList(msg)
		case viewNewProject:
			return m.updateNewProject(msg)
		case viewSettings:
			return m.updateSettings(msg)
		case viewConfirmDelete:
			return m.updateConfirmDelete(msg)
		case viewModelPicker:
			return m.updateModelPicker(msg)
		}
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.filtering {
		switch msg.String() {
		case "enter":
			m.filtering = false
			return m, nil
		case "esc":
			m.filtering = false
			m.filterInput.SetValue("")
			m.applyFilter()
			return m, nil
		default:
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.applyFilter()
			return m, cmd
		}
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "j", "down":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
			m.ensureVisible()
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
			m.ensureVisible()
		}
	case "g", "home":
		m.cursor = 0
		m.offset = 0
	case "G", "end":
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
			m.ensureVisible()
		}
	case "/":
		m.filtering = true
		m.filterInput.Focus()
		return m, textinput.Blink
	case "enter":
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			m.action = Action{Kind: "resume", Session: &s}
			return m, tea.Quit
		}
	case "n":
		m.state = viewNewProject
		m.newDirInput.Focus()
		return m, textinput.Blink
	case "t":
		m.state = viewSettings
	case "s":
		m.sortBy = (m.sortBy + 1) % sortModeCount
		m.applyFilter()
	case "tab":
		m.groupBy = (m.groupBy + 1) % groupModeCount
	case "m":
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			m.modelCursor = 0
			models := m.modelOptions(s.Model)
			for i, mo := range models {
				if mo == s.Model {
					m.modelCursor = i
					break
				}
			}
			m.state = viewModelPicker
		}
	case "d", "x":
		if len(m.filtered) > 0 {
			s := m.filtered[m.cursor]
			if !s.IsActive {
				m.state = viewConfirmDelete
			}
		}
	}
	return m, nil
}

func (m Model) modelOptions(current string) []string {
	seen := make(map[string]bool)
	var models []string
	if current != "" {
		models = append(models, current)
		seen[current] = true
	}
	for _, mo := range config.Models {
		if !seen[mo] {
			models = append(models, mo)
			seen[mo] = true
		}
	}
	return models
}

func (m Model) updateNewProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = viewList
		return m, nil
	case "enter":
		dir := m.newDirInput.Value()
		if dir != "" {
			m.action = Action{Kind: "new", Dir: dir}
			return m, tea.Quit
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.newDirInput, cmd = m.newDirInput.Update(msg)
		return m, cmd
	}
}

func (m Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.state = viewList
	case "j", "down":
		if m.termCursor < len(config.AvailableTerminals())-1 {
			m.termCursor++
		}
	case "k", "up":
		if m.termCursor > 0 {
			m.termCursor--
		}
	case "enter":
		m.config.Terminal = config.AvailableTerminals()[m.termCursor]
		_ = m.config.Save()
		m.state = viewList
	}
	return m, nil
}

func (m Model) updateModelPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		m.state = viewList
		return m, nil
	}
	models := m.modelOptions(m.filtered[m.cursor].Model)
	switch msg.String() {
	case "esc", "q":
		m.state = viewList
	case "j", "down":
		if m.modelCursor < len(models)-1 {
			m.modelCursor++
		}
	case "k", "up":
		if m.modelCursor > 0 {
			m.modelCursor--
		}
	case "enter":
		s := m.filtered[m.cursor]
		s.Model = models[m.modelCursor]
		m.action = Action{Kind: "resume", Session: &s}
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		if m.cursor >= 0 && m.cursor < len(m.filtered) {
			s := m.filtered[m.cursor]
			for _, p := range m.providers {
				if p.Name() == s.Provider {
					_ = p.DeleteSession(s.ID)
					break
				}
			}
			// Reload sessions
			m.state = viewList
			m.loading = true
			return m, m.Init()
		}
		m.state = viewList
	case "n", "N", "esc":
		m.state = viewList
	}
	return m, nil
}

type scored struct {
	session provider.Session
	score   int
}

func (m *Model) applyFilter() {
	q := search.ParseQuery(m.filterInput.Value())

	var results []scored
	for _, s := range m.sessions {
		if q.ActiveOnly && !s.IsActive {
			continue
		}

		filterOk := true
		for key, val := range q.Filters {
			switch key {
			case "model":
				if !search.FieldContains(s.Model, val) {
					filterOk = false
				}
			case "branch":
				if !search.FieldContains(s.GitBranch, val) {
					filterOk = false
				}
			case "project":
				if !search.FieldContains(s.ProjectName, val) {
					filterOk = false
				}
			case "provider":
				if !search.FieldContains(s.Provider, val) {
					filterOk = false
				}
			}
			if !filterOk {
				break
			}
		}
		if !filterOk {
			continue
		}

		if len(q.FuzzyTerms) == 0 {
			results = append(results, scored{s, 0})
			continue
		}

		summary := s.Summary
		if summary == "" {
			summary = s.FirstPrompt
		}

		ok, score := search.FuzzyMatchMulti(q.FuzzyTerms,
			s.ProjectName, summary, s.GitBranch, s.Model, s.ProjectPath)
		if ok {
			results = append(results, scored{s, score})
		}
	}

	hasFuzzy := len(q.FuzzyTerms) > 0
	sort.Slice(results, func(i, j int) bool {
		if hasFuzzy && results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return m.sortLess(results[i].session, results[j].session)
	})

	m.filtered = make([]provider.Session, len(results))
	for i, r := range results {
		m.filtered[i] = r.session
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	m.offset = 0
}

func (m Model) sortLess(a, b provider.Session) bool {
	switch m.sortBy {
	case sortCreated:
		return a.Created.After(b.Created)
	case sortName:
		return strings.ToLower(a.ProjectName) < strings.ToLower(b.ProjectName)
	case sortMessages:
		return a.MessageCount > b.MessageCount
	default:
		return a.Modified.After(b.Modified)
	}
}

func (m *Model) ensureVisible() {
	vh := m.viewportHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+vh {
		m.offset = m.cursor - vh + 1
	}
}

func (m Model) viewportHeight() int {
	used := 9
	return max(3, m.height-used)
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	switch m.state {
	case viewList:
		b.WriteString(m.renderFilter())
		b.WriteString("\n\n")
		if m.loading {
			b.WriteString(dimText.Render("  Loading sessions..."))
		} else if len(m.sessions) == 0 {
			b.WriteString(m.renderOnboarding())
		} else if len(m.filtered) == 0 {
			b.WriteString(dimText.Render("  No matching sessions. Press esc to clear filter."))
		} else {
			b.WriteString(m.renderTable())
		}
		b.WriteString("\n")
		b.WriteString(m.renderFooter())

	case viewNewProject:
		b.WriteString(m.renderNewProject())

	case viewSettings:
		b.WriteString(m.renderSettings())

	case viewConfirmDelete:
		b.WriteString(m.renderFilter())
		b.WriteString("\n\n")
		b.WriteString(m.renderTable())
		b.WriteString("\n")
		b.WriteString(m.renderDeleteConfirm())

	case viewModelPicker:
		b.WriteString(m.renderFilter())
		b.WriteString("\n\n")
		b.WriteString(m.renderTable())
		b.WriteString("\n")
		b.WriteString(m.renderModelPicker())
	}

	return b.String()
}

func (m Model) renderHeader() string {
	title := logoStyle.Render("◆  VibeCockpit")
	sub := subtitleStyle.Render("The cockpit for your copilots")
	content := lipgloss.JoinVertical(lipgloss.Center, title, sub)
	box := headerBorder.Width(max(40, m.width-4)).Render(content)
	return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, box)
}

func (m Model) renderFilter() string {
	left := m.filterInput.View()
	right := helpKeyStyle.Render("n") + helpStyle.Render(" new  ") +
		helpKeyStyle.Render("s") + helpStyle.Render(" sort  ") +
		helpKeyStyle.Render("t") + helpStyle.Render(" terminal  ") +
		helpKeyStyle.Render("q") + helpStyle.Render(" quit")
	gap := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right)-4)
	return "  " + left + strings.Repeat(" ", gap) + right
}

func (m Model) renderOnboarding() string {
	var b strings.Builder
	b.WriteString(logoStyle.Render("  Welcome!") + "\n\n")
	b.WriteString("  No coding sessions found yet. Here's how to get started:\n\n")
	b.WriteString(helpKeyStyle.Render("  1. ") + "Install Claude Code: " + dimText.Render("npm i -g @anthropic-ai/claude-code") + "\n")
	b.WriteString(helpKeyStyle.Render("  2. ") + "Run " + helpKeyStyle.Render("claude") + " in any project directory\n")
	b.WriteString(helpKeyStyle.Render("  3. ") + "Come back here — sessions appear automatically\n\n")
	b.WriteString("  Or press " + helpKeyStyle.Render("n") + " to create a new project right now.\n")
	return b.String()
}

func (m Model) renderTable() string {
	var b strings.Builder

	cols := m.columnWidths()
	stW, projW, sumW, modW, brW, msgW, modifW := cols[0], cols[1], cols[2], cols[3], cols[4], cols[5], cols[6]

	labels := dimText.Render(" [" + m.sortBy.String() + "]")
	if m.groupBy != groupNone {
		labels += dimText.Render(" [Group: " + m.groupBy.String() + "]")
	}
	header := fmt.Sprintf("  %-*s %-*s %-*s %-*s %-*s %*s  %-*s",
		stW, "ST", projW, "PROJECT", sumW, "SUMMARY", modW, "MODEL", brW, "BRANCH", msgW, "#", modifW, "MODIFIED")
	b.WriteString(columnHeaderStyle.Render(header) + labels + "\n")
	b.WriteString(dimText.Render("  "+strings.Repeat("─", max(10, m.width-4))) + "\n")

	vh := m.viewportHeight()
	end := min(m.offset+vh, len(m.filtered))

	lastGroup := ""
	for i := m.offset; i < end; i++ {
		s := m.filtered[i]

		if m.groupBy != groupNone {
			gk := m.groupKeyFor(s)
			if gk != lastGroup {
				lastGroup = gk
				groupLine := logoStyle.Render("  "+gk) + " " + dimText.Render(fmt.Sprintf("(%d)", m.groupCount(gk)))
				b.WriteString(groupLine + "\n")
			}
		}

		isSelected := i == m.cursor

		st := "  "
		if s.IsActive {
			st = activeIndicator + " "
		}
		if isSelected {
			st = cursorIndicator + " "
			if s.IsActive {
				st = activeIndicator + cursorIndicator[1:]
			}
		}

		summary := s.Summary
		if summary == "" {
			summary = s.FirstPrompt
		}

		row := fmt.Sprintf("%-*s %-*s %-*s %-*s %-*s %*d  %-*s",
			stW, st,
			projW, truncate(s.ProjectName, projW),
			sumW, truncate(summary, sumW),
			modW, truncate(shortModel(s.Model), modW),
			brW, truncate(s.GitBranch, brW),
			msgW, s.MessageCount,
			modifW, relativeTime(s.Modified))

		if isSelected {
			row = selectedRowStyle.Width(m.width - 2).Render(row)
		} else {
			row = normalRowStyle.Render(row)
		}
		b.WriteString("  " + row + "\n")
	}

	return b.String()
}

func (m Model) columnWidths() [7]int {
	w := max(80, m.width-6)
	stW := 2
	msgW := 4
	modifW := 10
	modW := 12
	brW := 14

	if w < 100 {
		brW = 0
		modW = 0
	} else if w < 120 {
		brW = 10
	}

	remaining := w - stW - msgW - modifW - modW - brW - 7
	projW := max(10, remaining*30/100)
	sumW := max(10, remaining-projW)

	return [7]int{stW, projW, sumW, modW, brW, msgW, modifW}
}

func (m Model) groupKeyFor(s provider.Session) string {
	switch m.groupBy {
	case groupProvider:
		return s.Provider
	case groupProject:
		return s.ProjectName
	case groupDate:
		d := time.Since(s.Modified)
		switch {
		case d < 24*time.Hour:
			return "Today"
		case d < 48*time.Hour:
			return "Yesterday"
		case d < 7*24*time.Hour:
			return "This week"
		case d < 30*24*time.Hour:
			return "This month"
		default:
			return "Older"
		}
	}
	return ""
}

func (m Model) groupCount(key string) int {
	n := 0
	for _, s := range m.filtered {
		if m.groupKeyFor(s) == key {
			n++
		}
	}
	return n
}

func (m Model) renderFooter() string {
	var pathLine string
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		s := m.filtered[m.cursor]
		home, _ := os.UserHomeDir()
		path := s.ProjectPath
		if home != "" && strings.HasPrefix(path, home) {
			path = "~" + path[len(home):]
		}
		pathLine = dimText.Render("  "+path) + "\n"
	}

	left := dimText.Render(fmt.Sprintf("  %d of %d sessions", min(m.cursor+1, len(m.filtered)), len(m.filtered)))

	var right string
	if m.filtering {
		right = helpStyle.Render("fuzzy search  ") +
			helpKeyStyle.Render("model:") + helpStyle.Render("opus  ") +
			helpKeyStyle.Render("branch:") + helpStyle.Render("main  ") +
			helpKeyStyle.Render("active") + helpStyle.Render("  ") +
			helpKeyStyle.Render("esc") + helpStyle.Render(" clear")
	} else {
		right = helpKeyStyle.Render("↑↓") + helpStyle.Render(" navigate  ") +
			helpKeyStyle.Render("/") + helpStyle.Render(" filter  ") +
			helpKeyStyle.Render("enter") + helpStyle.Render(" resume  ") +
			helpKeyStyle.Render("m") + helpStyle.Render(" model  ") +
			helpKeyStyle.Render("s") + helpStyle.Render(" sort  ") +
			helpKeyStyle.Render("tab") + helpStyle.Render(" group")
	}

	gap := max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	return pathLine + left + strings.Repeat(" ", gap) + right
}

func (m Model) renderDeleteConfirm() string {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return ""
	}
	s := m.filtered[m.cursor]
	msg := fmt.Sprintf("  Delete session \"%s\" from %s?  ", truncate(s.Summary, 40), s.ProjectName)
	prompt := warningStyle.Render(msg) +
		helpKeyStyle.Render("y") + helpStyle.Render(" yes  ") +
		helpKeyStyle.Render("n") + helpStyle.Render(" no")
	return prompt
}

func (m Model) renderModelPicker() string {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return ""
	}
	s := m.filtered[m.cursor]
	models := m.modelOptions(s.Model)

	var rows strings.Builder
	for i, mo := range models {
		cursor := "  "
		if i == m.modelCursor {
			cursor = cursorIndicator + " "
		}
		label := mo
		if mo == s.Model {
			label += dimText.Render(" (current)")
		}
		style := normalRowStyle
		if i == m.modelCursor {
			style = selectedRowStyle
		}
		rows.WriteString("  " + style.Render(cursor+label) + "\n")
	}

	content := logoStyle.Render("Resume with Model") + "\n" +
		dimText.Render("Session: "+truncate(s.ProjectName, 30)) + "\n\n" +
		rows.String() + "\n" +
		helpKeyStyle.Render("enter") + helpStyle.Render(" launch  ") +
		helpKeyStyle.Render("esc") + helpStyle.Render(" cancel")
	box := settingsStyle.Width(50).Render(content)
	return lipgloss.Place(m.width, m.height-6, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderNewProject() string {
	content := logoStyle.Render("New Project") + "\n\n" +
		m.newDirInput.View() + "\n\n" +
		helpKeyStyle.Render("enter") + helpStyle.Render(" create & launch  ") +
		helpKeyStyle.Render("esc") + helpStyle.Render(" cancel")
	box := newProjectStyle.Render(content)
	return lipgloss.Place(m.width, m.height-6, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderSettings() string {
	var rows strings.Builder
	for i, t := range config.AvailableTerminals() {
		cursor := "  "
		if i == m.termCursor {
			cursor = cursorIndicator + " "
		}
		label := t
		if t == "default" {
			label = "default (replace current process)"
		}
		style := normalRowStyle
		if i == m.termCursor {
			style = selectedRowStyle
		}
		rows.WriteString(style.Render(cursor+label) + "\n")
	}
	current := dimText.Render("Current: " + m.config.Terminal)
	content := logoStyle.Render("Terminal") + "\n" + current + "\n\n" +
		rows.String() + "\n" +
		helpKeyStyle.Render("enter") + helpStyle.Render(" select  ") +
		helpKeyStyle.Render("esc") + helpStyle.Render(" cancel")
	box := settingsStyle.Width(45).Render(content)
	return lipgloss.Place(m.width, m.height-6, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) GetAction() Action {
	return m.action
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

func shortModel(model string) string {
	if model == "" {
		return "-"
	}
	s := strings.TrimPrefix(model, "claude-")
	parts := strings.Split(s, "-")
	if len(parts) >= 3 {
		name := parts[0]
		major := parts[1]
		minor := parts[2]
		if len(minor) > 4 {
			minor = minor[:1]
		}
		return name + " " + major + "." + minor
	}
	return s
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 02")
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

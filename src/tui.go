package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// View modes
type viewMode int

const (
	modeDashboard viewMode = iota
	modeThreadChain
	modeViewAnswer
	modeCreateWorkspace
	modeCreateThread
	modeAttachRepo
	modeSettings
	modeFollowUp
)

// Focus panel in dashboard
type dashPanel int

const (
	panelWorkspaces dashPanel = iota
	panelThreads
)

// Messages
type tickMsg time.Time

// Model is the main bubbletea model.
type Model struct {
	mode        viewMode
	width       int
	height      int
	store       *WorkspaceStore
	threads     *ThreadManager
	workspaces  []*Workspace
	wsIndex     int
	threadIndex int
	panel       dashPanel

	// Answer viewer
	viewport     viewport.Model
	viewedThread *Thread

	// Chain viewer
	chain      []*Thread // current conversation chain
	chainIndex int       // selected item in the chain

	// Input field
	input       textinput.Model
	errMsg      string
	suggestions []string
	sugIndex    int

	// Settings
	settingsIndex  int // which setting row is selected
	settingsEditing bool // currently editing a text setting

}

func NewModel(store *WorkspaceStore, threads *ThreadManager) Model {
	ti := textinput.New()
	ti.CharLimit = 512

	return Model{
		mode:    modeDashboard,
		store:   store,
		threads: threads,
		input:   ti,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.loadWorkspaces,
		tickEvery(),
	)
}

func tickEvery() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) loadWorkspaces() tea.Msg {
	ws, _ := m.store.List()
	return workspacesLoadedMsg(ws)
}

type workspacesLoadedMsg []*Workspace

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = msg.Height - 6
		return m, nil

	case workspacesLoadedMsg:
		m.workspaces = msg
		return m, nil

	case tickMsg:
		return m, tickEvery()
	}

	switch m.mode {
	case modeDashboard:
		return m.updateDashboard(msg)
	case modeThreadChain:
		return m.updateThreadChain(msg)
	case modeViewAnswer:
		return m.updateViewAnswer(msg)
	case modeSettings:
		return m.updateSettings(msg)
	case modeCreateWorkspace, modeCreateThread, modeAttachRepo, modeFollowUp:
		return m.updateInput(msg)
	}

	return m, nil
}

func (m Model) updateDashboard(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		m.errMsg = ""
		switch msg.String() {
		case "q", "ctrl+c":
			m.threads.StopAll()
			return m, tea.Quit

		case "tab":
			if m.panel == panelWorkspaces {
				m.panel = panelThreads
			} else {
				m.panel = panelWorkspaces
			}

		case "j", "down":
			if m.panel == panelWorkspaces {
				if m.wsIndex < len(m.workspaces)-1 {
					m.wsIndex++
					m.threadIndex = 0
				}
			} else {
				threads := m.currentThreads()
				if m.threadIndex < len(threads)-1 {
					m.threadIndex++
				}
			}

		case "k", "up":
			if m.panel == panelWorkspaces {
				if m.wsIndex > 0 {
					m.wsIndex--
					m.threadIndex = 0
				}
			} else {
				if m.threadIndex > 0 {
					m.threadIndex--
				}
			}

		case "w":
			m.mode = modeCreateWorkspace
			m.input.SetValue("")
			m.input.Placeholder = "workspace name"
			m.input.Focus()
			return m, textinput.Blink

		case "n":
			if len(m.workspaces) == 0 {
				m.errMsg = "create a workspace first (w)"
				return m, nil
			}
			ws := m.workspaces[m.wsIndex]
			if len(ws.Repos) == 0 {
				m.errMsg = "add sources first (a)"
				return m, nil
			}
			m.mode = modeCreateThread
			m.input.SetValue("")
			m.input.Placeholder = "ask a question or describe a task..."
			m.input.Focus()
			return m, textinput.Blink

		case "s":
			if len(m.workspaces) == 0 {
				m.errMsg = "create a workspace first (w)"
				return m, nil
			}
			m.mode = modeSettings
			m.settingsIndex = 0
			m.settingsEditing = false
			return m, nil

		case "a":
			if len(m.workspaces) == 0 {
				m.errMsg = "create a workspace first (w)"
				return m, nil
			}
			m.mode = modeAttachRepo
			m.input.SetValue("")
			m.input.Placeholder = "path to repo, GitHub URL, or user/repo"
			m.input.Focus()
			return m, textinput.Blink

		case "enter":
			if m.panel == panelWorkspaces && len(m.workspaces) > 0 {
				m.panel = panelThreads
				m.threadIndex = 0
			} else if m.panel == panelThreads {
				threads := m.currentThreads()
				if len(threads) > 0 && m.threadIndex < len(threads) {
					root := threads[m.threadIndex]
					m.chain = m.threads.GetChain(root.ID)
					m.chainIndex = len(m.chain) - 1 // start at most recent
					m.mode = modeThreadChain
					return m, nil
				}
			}

		case "d":
			if m.panel == panelThreads {
				threads := m.currentThreads()
				if len(threads) > 0 && m.threadIndex < len(threads) {
					t := threads[m.threadIndex]
					m.threads.Remove(t.ID)
					if m.threadIndex > 0 {
						m.threadIndex--
					}
				}
			} else if m.panel == panelWorkspaces && len(m.workspaces) > 0 {
				ws := m.workspaces[m.wsIndex]
				m.store.Delete(ws.ID)
				m.workspaces, _ = m.store.List()
				if m.wsIndex >= len(m.workspaces) && m.wsIndex > 0 {
					m.wsIndex--
				}
			}
		}
	}
	return m, nil
}

func (m Model) updateThreadChain(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Refresh chain (threads may have finished since last render)
	if len(m.chain) > 0 {
		m.chain = m.threads.GetChain(m.chain[0].ID)
		if m.chainIndex >= len(m.chain) {
			m.chainIndex = len(m.chain) - 1
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			m.mode = modeDashboard
			m.chain = nil
			return m, nil

		case "j", "down":
			if m.chainIndex < len(m.chain)-1 {
				m.chainIndex++
			}

		case "k", "up":
			if m.chainIndex > 0 {
				m.chainIndex--
			}

		case "enter":
			if m.chainIndex < len(m.chain) {
				t := m.chain[m.chainIndex]
				if t.Status == ThreadExited {
					m.viewedThread = t
					m.mode = modeViewAnswer
					m.viewport = viewport.New(m.width-4, m.height-10)
					m.viewport.SetContent(wordWrap(t.Answer(), m.width-8))
					return m, nil
				}
			}

		case "f":
			// Ask follow-up from the end of the chain
			if len(m.chain) > 0 {
				last := m.chain[len(m.chain)-1]
				if last.Status == ThreadExited {
					m.mode = modeFollowUp
					m.input.SetValue("")
					m.input.Placeholder = "ask a follow-up question..."
					m.input.Focus()
					return m, textinput.Blink
				}
			}

		case "d":
			// Delete selected thread from chain (and its descendants)
			if m.chainIndex < len(m.chain) {
				t := m.chain[m.chainIndex]
				// Remove this thread and all descendants
				for i := len(m.chain) - 1; i >= m.chainIndex; i-- {
					m.threads.Remove(m.chain[i].ID)
				}
				_ = t // suppress unused
				// Refresh
				if m.chainIndex == 0 {
					// Deleted the root, go back to dashboard
					m.mode = modeDashboard
					m.chain = nil
					return m, nil
				}
				m.chain = m.threads.GetChain(m.chain[0].ID)
				if m.chainIndex >= len(m.chain) {
					m.chainIndex = len(m.chain) - 1
				}
			}
		}
	}
	return m, nil
}

func (m Model) viewThreadChain() string {
	if len(m.chain) == 0 {
		return ""
	}

	root := m.chain[0]
	header := titleStyle.Render("  Thread") + mutedStyle.Render("  "+truncateStr(root.Question, 50))

	// Source tags
	var tags []string
	for _, repo := range root.Repos {
		tags = append(tags, tagStyle.Render(filepath.Base(repo)))
	}
	meta := metaLabelStyle.Render("  sources ") + strings.Join(tags, " ")

	sep := mutedStyle.Render(strings.Repeat("─", m.width-4))

	// Chain items
	var items []string
	for i, t := range m.chain {
		cursor := "  "
		style := listItemStyle
		if i == m.chainIndex {
			cursor = "▸ "
			style = selectedItemStyle
		}

		// Depth indicator
		depthPrefix := ""
		if i > 0 {
			depthPrefix = "↳ "
		}

		q := truncateStr(t.Question, m.width-20)
		statusStr := threadStatusStyle(t.Status).Render(t.Status.String())
		duration := mutedStyle.Render(t.Duration().String())

		line := style.Render(fmt.Sprintf("%s%s%s", cursor, depthPrefix, q))
		line += "\n    " + statusStr + "  " + duration

		// Show preview if done
		if t.Status == ThreadExited {
			preview := t.Preview(m.width - 12)
			if preview != "" {
				line += "\n    " + mutedStyle.Render(preview)
			}
		}

		items = append(items, line)
	}

	content := strings.Join(items, "\n\n")

	panel := activePanelStyle.Width(m.width - 4).Render(content)

	help := helpStyle.Render("↑/↓: navigate  enter: view answer  f: follow-up  d: delete  esc: back")

	return header + "\n" + meta + "\n" + sep + "\n" + panel + "\n\n" + help
}

func truncateStr(s string, maxLen int) string {
	if maxLen < 4 {
		maxLen = 4
	}
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

func (m Model) updateViewAnswer(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.mode = modeThreadChain
			m.viewedThread = nil
			return m, nil
		case "f":
			// Follow-up from the viewed thread
			m.mode = modeFollowUp
			m.input.SetValue("")
			m.input.Placeholder = "ask a follow-up question..."
			m.input.Focus()
			return m, textinput.Blink
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) updateInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			value := m.input.Value()
			prevMode := m.mode
			m.mode = modeDashboard
			m.input.Blur()
			m.suggestions = nil
			m.sugIndex = 0

			if value != "" {
				switch prevMode {
				case modeCreateWorkspace:
					_, err := m.store.Create(value, nil)
					if err != nil {
						m.errMsg = err.Error()
					} else {
						m.workspaces, _ = m.store.List()
					}

				case modeCreateThread:
					if len(m.workspaces) == 0 {
						m.errMsg = "no workspace selected"
					} else {
						ws := m.workspaces[m.wsIndex]
						_, err := m.threads.Spawn(value, ws.ID)
						if err != nil {
							m.errMsg = err.Error()
						}
					}

				case modeAttachRepo:
					if len(m.workspaces) > 0 {
						ws := m.workspaces[m.wsIndex]
						_, err := m.store.AddRepo(ws.ID, value)
						if err != nil {
							m.errMsg = err.Error()
						} else {
							m.workspaces, _ = m.store.List()
						}
					}

				case modeFollowUp:
					// Spawn follow-up from the last thread in the chain
					if len(m.chain) > 0 {
						last := m.chain[len(m.chain)-1]
						_, err := m.threads.SpawnWithParent(value, last.WorkspaceID, last.ID)
						if err != nil {
							m.errMsg = err.Error()
						} else {
							// Refresh chain and jump to the new thread
							m.chain = m.threads.GetChain(m.chain[0].ID)
							m.chainIndex = len(m.chain) - 1
						}
					}
					m.viewedThread = nil
					m.mode = modeThreadChain
				}
			}
			return m, nil

		case "esc":
			if prevMode := m.mode; prevMode == modeFollowUp {
				m.mode = modeThreadChain
			} else {
				m.mode = modeDashboard
			}
			m.input.Blur()
			m.suggestions = nil
			m.sugIndex = 0
			return m, nil

		case "tab":
			if m.mode == modeAttachRepo && len(m.suggestions) > 0 {
				m.input.SetValue(m.suggestions[m.sugIndex])
				m.input.CursorEnd()
				m.suggestions = CompletePath(m.input.Value(), 8)
				m.sugIndex = 0
				return m, nil
			}

		case "shift+tab":
			if m.mode == modeAttachRepo && len(m.suggestions) > 0 {
				m.sugIndex--
				if m.sugIndex < 0 {
					m.sugIndex = len(m.suggestions) - 1
				}
				return m, nil
			}

		case "down":
			if m.mode == modeAttachRepo && len(m.suggestions) > 0 {
				m.sugIndex++
				if m.sugIndex >= len(m.suggestions) {
					m.sugIndex = 0
				}
				return m, nil
			}

		case "up":
			if m.mode == modeAttachRepo && len(m.suggestions) > 0 {
				m.sugIndex--
				if m.sugIndex < 0 {
					m.sugIndex = len(m.suggestions) - 1
				}
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)

	// Update path suggestions in repo mode
	if m.mode == modeAttachRepo {
		val := m.input.Value()
		if isLocalPath(val) {
			m.suggestions = CompletePath(val, 8)
		} else {
			m.suggestions = nil
		}
		m.sugIndex = 0
	}

	return m, cmd
}

func isLocalPath(s string) bool {
	if s == "" {
		return true
	}
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "~/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	if strings.Contains(s, "://") || strings.HasPrefix(s, "git@") {
		return false
	}
	return true
}

func (m Model) currentThreads() []*Thread {
	if len(m.workspaces) == 0 {
		return nil
	}
	ws := m.workspaces[m.wsIndex]
	return m.threads.RootThreadsForWorkspace(ws.ID)
}

// View renders the TUI.
func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	switch m.mode {
	case modeThreadChain:
		return m.viewThreadChain()
	case modeViewAnswer:
		return m.viewAnswer()
	case modeSettings:
		return m.viewSettings()
	case modeCreateWorkspace, modeCreateThread, modeAttachRepo, modeFollowUp:
		return m.viewInput()
	default:
		return m.viewDashboard()
	}
}

func (m Model) viewDashboard() string {
	header := titleStyle.Render("  xue.") + mutedStyle.Render("  study your repos")

	leftWidth := m.width/3 - 2
	rightWidth := m.width - leftWidth - 4

	// Left panel: workspaces with repos
	leftTitle := headerStyle.Render("Workspaces")
	var leftItems []string
	for i, ws := range m.workspaces {
		cursor := "  "
		style := listItemStyle
		if i == m.wsIndex {
			cursor = "▸ "
			if m.panel == panelWorkspaces {
				style = selectedItemStyle
			}
		}
		line := style.Render(cursor + ws.Name)
		if len(ws.Repos) == 0 {
			line += "\n" + mutedStyle.Render("    (no sources — press a)")
		} else {
			for _, repo := range ws.Repos {
				line += "\n" + mutedStyle.Render("    "+filepath.Base(repo))
			}
		}
		leftItems = append(leftItems, line)
	}
	if len(leftItems) == 0 {
		leftItems = append(leftItems, mutedStyle.Render("  No workspaces — press w"))
	}

	leftContent := leftTitle + "\n" + strings.Join(leftItems, "\n")
	leftPanel := panelStyle.Width(leftWidth)
	if m.panel == panelWorkspaces {
		leftPanel = activePanelStyle.Width(leftWidth)
	}
	left := leftPanel.Render(leftContent)

	// Right panel: threads
	rightTitle := headerStyle.Render("Questions")
	threads := m.currentThreads()
	var rightItems []string
	for i, t := range threads {
		cursor := "  "
		style := listItemStyle
		if i == m.threadIndex {
			cursor = "▸ "
			if m.panel == panelThreads {
				style = selectedItemStyle
			}
		}

		// Truncate question for display
		q := t.Question
		maxQ := rightWidth - 20
		if maxQ < 10 {
			maxQ = 10
		}
		if len(q) > maxQ {
			q = q[:maxQ] + "..."
		}

		// Compact: question + status + follow-up count on one line
		chain := m.threads.GetChain(t.ID)
		latest := chain[len(chain)-1]
		statusStr := threadStatusStyle(latest.Status).Render(latest.Status.String())
		line := style.Render(fmt.Sprintf("%s%s", cursor, q))
		line += "  " + statusStr
		if followUps := len(chain) - 1; followUps > 0 {
			line += "  " + mutedStyle.Render(fmt.Sprintf("+%d", followUps))
		}
		rightItems = append(rightItems, line)
	}
	if len(rightItems) == 0 {
		if len(m.workspaces) > 0 {
			ws := m.workspaces[m.wsIndex]
			if len(ws.Repos) > 0 {
				rightItems = append(rightItems, mutedStyle.Render("  No questions yet — press n"))
			} else {
				rightItems = append(rightItems, mutedStyle.Render("  Add sources first (a)"))
			}
		} else {
			rightItems = append(rightItems, mutedStyle.Render("  Create a workspace first (w)"))
		}
	}

	rightContent := rightTitle + "\n" + strings.Join(rightItems, "\n")
	rightPanel := panelStyle.Width(rightWidth)
	if m.panel == panelThreads {
		rightPanel = activePanelStyle.Width(rightWidth)
	}
	right := rightPanel.Render(rightContent)

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	errLine := ""
	if m.errMsg != "" {
		errLine = "\n" + errorStyle.Render("  "+m.errMsg)
	}

	help := helpStyle.Render("w: workspace  a: add source  n: ask question  s: settings  enter: view  d: delete  tab: switch  q: quit")

	return header + "\n\n" + body + errLine + "\n\n" + help
}

func (m Model) viewAnswer() string {
	t := m.viewedThread
	if t == nil {
		return ""
	}

	// Header: question
	q := t.Question
	if len(q) > m.width-10 {
		q = q[:m.width-10] + "..."
	}
	header := titleStyle.Render("  Q: ") + lipgloss.NewStyle().Foreground(colorText).Render(q)

	// Metadata line: sources + duration
	var tags []string
	for _, repo := range t.Repos {
		tags = append(tags, tagStyle.Render(filepath.Base(repo)))
	}
	meta := metaLabelStyle.Render("  sources ") + strings.Join(tags, " ")
	meta += "    " + metaLabelStyle.Render("time ") + mutedStyle.Render(t.Duration().String())
	if depth := m.threads.ChainDepth(t); depth > 0 {
		meta += "    " + metaLabelStyle.Render("depth ") + mutedStyle.Render(fmt.Sprintf("%d", depth))
	}

	// Separator
	sep := mutedStyle.Render(strings.Repeat("─", m.width-4))

	// Answer body in viewport
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Width(m.width - 4).
		Height(m.height - 8)

	help := helpStyle.Render("scroll: up/down/pgup/pgdn  f: follow-up  q/esc: back")

	return header + "\n" + meta + "\n" + sep + "\n" + border.Render(m.viewport.View()) + "\n" + help
}

func (m Model) viewInput() string {
	var title string
	var hint string
	switch m.mode {
	case modeCreateWorkspace:
		title = "Create Workspace"
		hint = "enter: confirm  esc: cancel"
	case modeCreateThread:
		title = "Ask a Question"
		hint = "enter: submit  esc: cancel"
	case modeAttachRepo:
		title = "Add Source"
		hint = "tab: complete  enter: add  esc: cancel"
	case modeFollowUp:
		title = "Follow-up Question"
		hint = "enter: submit  esc: back"
	}

	content := inputLabelStyle.Render(title) + "\n\n" +
		m.input.View()

	// Show path completions
	if m.mode == modeAttachRepo && len(m.suggestions) > 0 {
		content += "\n"
		for i, s := range m.suggestions {
			if i == m.sugIndex {
				content += "\n" + selectedItemStyle.Render("▸ "+s)
			} else {
				content += "\n" + mutedStyle.Render("  "+s)
			}
		}
	}

	if m.mode == modeAttachRepo && m.input.Value() == "" {
		content += "\n\n" + mutedStyle.Render("local path · GitHub URL · git URI · user/repo")
	}

	content += "\n\n" + mutedStyle.Render(hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 2).
		Width(60).
		Render(content)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		box,
	)
}

// Settings rows: each setting is a named row with its own interaction.
// Row 0: Model (cycle through options)
// Row 1: Max Tokens (text input)
// Row 2: System Prompt (text input)
const (
	settingModel        = 0
	settingMaxTokens    = 1
	settingSystemPrompt = 2
	settingCount        = 3
)

func (m Model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	ws := m.workspaces[m.wsIndex]

	if m.settingsEditing {
		// Text editing mode for max_tokens or system_prompt
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				value := m.input.Value()
				m.settingsEditing = false
				m.input.Blur()

				switch m.settingsIndex {
				case settingMaxTokens:
					var tokens int
					fmt.Sscanf(value, "%d", &tokens)
					ws.Settings.MaxTokens = tokens
				case settingSystemPrompt:
					ws.Settings.SystemPrompt = value
				}
				m.store.UpdateSettings(ws.ID, ws.Settings)
				m.workspaces, _ = m.store.List()
				return m, nil
			case "esc":
				m.settingsEditing = false
				m.input.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc":
			m.mode = modeDashboard
			return m, nil

		case "j", "down":
			if m.settingsIndex < settingCount-1 {
				m.settingsIndex++
			}

		case "k", "up":
			if m.settingsIndex > 0 {
				m.settingsIndex--
			}

		case "enter", " ", "l", "right":
			switch m.settingsIndex {
			case settingModel:
				// Cycle to next model
				current := ws.Settings.Model
				idx := 0
				for i, model := range AvailableModels {
					if model == current {
						idx = i
						break
					}
				}
				idx = (idx + 1) % len(AvailableModels)
				ws.Settings.Model = AvailableModels[idx]
				m.store.UpdateSettings(ws.ID, ws.Settings)
				m.workspaces, _ = m.store.List()

			case settingMaxTokens:
				m.settingsEditing = true
				m.input.SetValue("")
				if ws.Settings.MaxTokens > 0 {
					m.input.SetValue(fmt.Sprintf("%d", ws.Settings.MaxTokens))
				}
				m.input.Placeholder = "max output tokens (0 = default)"
				m.input.Focus()
				return m, textinput.Blink

			case settingSystemPrompt:
				m.settingsEditing = true
				m.input.SetValue(ws.Settings.SystemPrompt)
				m.input.Placeholder = "custom system prompt"
				m.input.Focus()
				return m, textinput.Blink
			}

		case "h", "left":
			if m.settingsIndex == settingModel {
				// Cycle backwards
				current := ws.Settings.Model
				idx := 0
				for i, model := range AvailableModels {
					if model == current {
						idx = i
						break
					}
				}
				idx--
				if idx < 0 {
					idx = len(AvailableModels) - 1
				}
				ws.Settings.Model = AvailableModels[idx]
				m.store.UpdateSettings(ws.ID, ws.Settings)
				m.workspaces, _ = m.store.List()
			}
		}
	}
	return m, nil
}

func (m Model) viewSettings() string {
	if len(m.workspaces) == 0 {
		return ""
	}
	ws := m.workspaces[m.wsIndex]

	header := titleStyle.Render("  Settings") + mutedStyle.Render("  "+ws.Name)

	type settingRow struct {
		label string
		value string
		hint  string
	}

	rows := []settingRow{
		{
			label: "Model",
			value: ModelDisplayName(ws.Settings.Model),
			hint:  "enter/←/→ to cycle",
		},
		{
			label: "Max Tokens",
			value: func() string {
				if ws.Settings.MaxTokens == 0 {
					return "default"
				}
				return fmt.Sprintf("%d", ws.Settings.MaxTokens)
			}(),
			hint: "enter to edit",
		},
		{
			label: "System Prompt",
			value: func() string {
				if ws.Settings.SystemPrompt == "" {
					return "none"
				}
				s := ws.Settings.SystemPrompt
				if len(s) > 40 {
					s = s[:40] + "..."
				}
				return s
			}(),
			hint: "enter to edit",
		},
	}

	var lines []string
	for i, row := range rows {
		cursor := "  "
		style := listItemStyle
		if i == m.settingsIndex {
			cursor = "▸ "
			style = selectedItemStyle
		}

		label := metaLabelStyle.Render(row.label)
		value := lipgloss.NewStyle().Foreground(colorText).Render(row.value)
		hint := mutedStyle.Render("  " + row.hint)

		line := style.Render(cursor) + label + "  " + value
		if i == m.settingsIndex {
			line += hint
		}
		lines = append(lines, line)
	}

	content := headerStyle.Render("Workspace Settings") + "\n" + strings.Join(lines, "\n\n")

	// If editing, show the input below
	if m.settingsEditing {
		content += "\n\n" + m.input.View()
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorPrimary).
		Padding(1, 2).
		Width(60).
		Render(content)

	help := helpStyle.Render("↑/↓: navigate  enter: change  esc: back")

	return header + "\n\n" + lipgloss.Place(m.width, m.height-4,
		lipgloss.Center, lipgloss.Center,
		box,
	) + "\n" + help
}

// wordWrap wraps text to fit within maxWidth columns.
func wordWrap(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}
	var result strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if len(line) <= maxWidth {
			result.WriteString(line)
			result.WriteByte('\n')
			continue
		}
		for len(line) > 0 {
			if len(line) <= maxWidth {
				result.WriteString(line)
				result.WriteByte('\n')
				break
			}
			// Find last space within maxWidth
			cut := maxWidth
			for cut > 0 && line[cut] != ' ' {
				cut--
			}
			if cut == 0 {
				cut = maxWidth // no space found, hard break
			}
			result.WriteString(line[:cut])
			result.WriteByte('\n')
			line = strings.TrimLeft(line[cut:], " ")
		}
	}
	return result.String()
}

func runTUI(store *WorkspaceStore, threads *ThreadManager) error {
	m := NewModel(store, threads)
	p := tea.NewProgram(m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err := p.Run()
	return err
}

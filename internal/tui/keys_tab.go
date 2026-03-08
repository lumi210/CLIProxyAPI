package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// keysTabModel displays and manages API keys.
type keysTabModel struct {
	client   *Client
	viewport viewport.Model
	keys     []accessAPIKeyEntry
	gemini   []map[string]any
	claude   []map[string]any
	codex    []map[string]any
	vertex   []map[string]any
	openai   []map[string]any
	err      error
	width    int
	height   int
	ready    bool
	cursor   int
	confirm  int // -1 = no deletion pending
	status   string

	// Editing / Adding
	editing      bool
	adding       bool
	editIdx      int
	editField    int
	editInputs   []textinput.Model
	editingError string
}

type keysDataMsg struct {
	apiKeys []accessAPIKeyEntry
	gemini  []map[string]any
	claude  []map[string]any
	codex   []map[string]any
	vertex  []map[string]any
	openai  []map[string]any
	err     error
}

type keyActionMsg struct {
	action string
	err    error
}

func newKeysTabModel(client *Client) keysTabModel {
	inputs := make([]textinput.Model, 3)
	prompts := []string{"  Key: ", "  Expires At: ", "  Token Limit: "}
	limits := []int{512, 64, 32}
	for i := range inputs {
		inputs[i] = textinput.New()
		inputs[i].Prompt = prompts[i]
		inputs[i].CharLimit = limits[i]
	}
	return keysTabModel{
		client:     client,
		confirm:    -1,
		editInputs: inputs,
	}
}

func (m keysTabModel) Init() tea.Cmd {
	return m.fetchKeys
}

func (m keysTabModel) fetchKeys() tea.Msg {
	result := keysDataMsg{}
	apiKeys, err := m.client.GetAPIKeys()
	if err != nil {
		result.err = err
		return result
	}
	result.apiKeys = apiKeys
	result.gemini, _ = m.client.GetGeminiKeys()
	result.claude, _ = m.client.GetClaudeKeys()
	result.codex, _ = m.client.GetCodexKeys()
	result.vertex, _ = m.client.GetVertexKeys()
	result.openai, _ = m.client.GetOpenAICompat()
	return result
}

func (m keysTabModel) Update(msg tea.Msg) (keysTabModel, tea.Cmd) {
	switch msg := msg.(type) {
	case localeChangedMsg:
		m.viewport.SetContent(m.renderContent())
		return m, nil
	case keysDataMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			m.keys = msg.apiKeys
			m.gemini = msg.gemini
			m.claude = msg.claude
			m.codex = msg.codex
			m.vertex = msg.vertex
			m.openai = msg.openai
			if m.cursor >= len(m.keys) {
				m.cursor = max(0, len(m.keys)-1)
			}
		}
		m.viewport.SetContent(m.renderContent())
		return m, nil

	case keyActionMsg:
		if msg.err != nil {
			m.status = errorStyle.Render("✗ " + msg.err.Error())
		} else {
			m.status = successStyle.Render("✓ " + msg.action)
		}
		m.confirm = -1
		m.viewport.SetContent(m.renderContent())
		return m, m.fetchKeys

	case tea.KeyMsg:
		// ---- Editing / Adding mode ----
		if m.editing || m.adding {
			switch msg.String() {
			case "enter":
				if m.editField < len(m.editInputs)-1 {
					m.focusEditField(m.editField + 1)
					m.viewport.SetContent(m.renderContent())
					return m, textinput.Blink
				}
				entry, err := m.currentEditEntry()
				if err != nil {
					m.editingError = err.Error()
					m.viewport.SetContent(m.renderContent())
					return m, nil
				}
				isAdding := m.adding
				editIdx := m.editIdx
				m.clearEditingState()
				if isAdding {
					return m, func() tea.Msg {
						err := m.client.AddAPIKey(entry)
						if err != nil {
							return keyActionMsg{err: err}
						}
						return keyActionMsg{action: T("key_added")}
					}
				}
				return m, func() tea.Msg {
					err := m.client.EditAPIKey(editIdx, entry)
					if err != nil {
						return keyActionMsg{err: err}
					}
					return keyActionMsg{action: T("key_updated")}
				}
			case "esc":
				m.clearEditingState()
				m.viewport.SetContent(m.renderContent())
				return m, nil
			case "tab", "shift+tab", "up", "down":
				delta := 1
				if msg.String() == "shift+tab" || msg.String() == "up" {
					delta = -1
				}
				next := m.editField + delta
				if next < 0 {
					next = len(m.editInputs) - 1
				}
				if next >= len(m.editInputs) {
					next = 0
				}
				m.focusEditField(next)
				m.viewport.SetContent(m.renderContent())
				return m, textinput.Blink
			default:
				var cmd tea.Cmd
				m.editInputs[m.editField], cmd = m.editInputs[m.editField].Update(msg)
				m.editingError = ""
				m.viewport.SetContent(m.renderContent())
				return m, cmd
			}
		}

		// ---- Delete confirmation ----
		if m.confirm >= 0 {
			switch msg.String() {
			case "y", "Y":
				idx := m.confirm
				m.confirm = -1
				return m, func() tea.Msg {
					err := m.client.DeleteAPIKey(idx)
					if err != nil {
						return keyActionMsg{err: err}
					}
					return keyActionMsg{action: T("key_deleted")}
				}
			case "n", "N", "esc":
				m.confirm = -1
				m.viewport.SetContent(m.renderContent())
				return m, nil
			}
			return m, nil
		}

		// ---- Normal mode ----
		switch msg.String() {
		case "j", "down":
			if len(m.keys) > 0 {
				m.cursor = (m.cursor + 1) % len(m.keys)
				m.viewport.SetContent(m.renderContent())
			}
			return m, nil
		case "k", "up":
			if len(m.keys) > 0 {
				m.cursor = (m.cursor - 1 + len(m.keys)) % len(m.keys)
				m.viewport.SetContent(m.renderContent())
			}
			return m, nil
		case "a":
			// Add new key
			m.adding = true
			m.editing = false
			m.setEditInputs(accessAPIKeyEntry{})
			m.focusEditField(0)
			m.viewport.SetContent(m.renderContent())
			return m, textinput.Blink
		case "e":
			// Edit selected key
			if m.cursor < len(m.keys) {
				m.editing = true
				m.adding = false
				m.editIdx = m.cursor
				m.setEditInputs(m.keys[m.cursor])
				m.focusEditField(0)
				m.viewport.SetContent(m.renderContent())
				return m, textinput.Blink
			}
			return m, nil
		case "d":
			// Delete selected key
			if m.cursor < len(m.keys) {
				m.confirm = m.cursor
				m.viewport.SetContent(m.renderContent())
			}
			return m, nil
		case "c":
			// Copy selected key to clipboard
			if m.cursor < len(m.keys) {
				key := m.keys[m.cursor].APIKey
				if err := clipboard.WriteAll(key); err != nil {
					m.status = errorStyle.Render(T("copy_failed") + ": " + err.Error())
				} else {
					m.status = successStyle.Render(T("copied"))
				}
				m.viewport.SetContent(m.renderContent())
			}
			return m, nil
		case "r":
			m.status = ""
			return m, m.fetchKeys
		default:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *keysTabModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	for i := range m.editInputs {
		m.editInputs[i].Width = max(20, w-20)
	}
	if !m.ready {
		m.viewport = viewport.New(w, h)
		m.viewport.SetContent(m.renderContent())
		m.ready = true
	} else {
		m.viewport.Width = w
		m.viewport.Height = h
	}
}

func (m keysTabModel) View() string {
	if !m.ready {
		return T("loading")
	}
	return m.viewport.View()
}

func (m keysTabModel) renderContent() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render(T("keys_title")))
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render(T("keys_help")))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", m.width))
	sb.WriteString("\n")

	if m.err != nil {
		sb.WriteString(errorStyle.Render(T("error_prefix") + m.err.Error()))
		sb.WriteString("\n")
		return sb.String()
	}

	// ━━━ Access API Keys (interactive) ━━━
	sb.WriteString(tableHeaderStyle.Render(fmt.Sprintf("  %s (%d)", T("access_keys"), len(m.keys))))
	sb.WriteString("\n")

	if len(m.keys) == 0 {
		sb.WriteString(subtitleStyle.Render(T("no_keys")))
		sb.WriteString("\n")
	}

	for i, key := range m.keys {
		cursor := "  "
		rowStyle := lipgloss.NewStyle()
		if i == m.cursor {
			cursor = "▸ "
			rowStyle = lipgloss.NewStyle().Bold(true)
		}

		row := fmt.Sprintf("%s%d. %s", cursor, i+1, formatAccessKeyEntry(key))
		sb.WriteString(rowStyle.Render(row))
		sb.WriteString("\n")

		// Delete confirmation
		if m.confirm == i {
			sb.WriteString(warningStyle.Render(fmt.Sprintf("    "+T("confirm_delete_key"), maskKey(key.APIKey))))
			sb.WriteString("\n")
		}

		// Edit input
		if m.editing && m.editIdx == i {
			m.renderEditForm(&sb)
		}
	}

	// Add input
	if m.adding {
		sb.WriteString("\n")
		m.renderEditForm(&sb)
	}

	sb.WriteString("\n")

	// ━━━ Provider Keys (read-only display) ━━━
	renderProviderKeys(&sb, "Gemini API Keys", m.gemini)
	renderProviderKeys(&sb, "Claude API Keys", m.claude)
	renderProviderKeys(&sb, "Codex API Keys", m.codex)
	renderProviderKeys(&sb, "Vertex API Keys", m.vertex)

	if len(m.openai) > 0 {
		renderSection(&sb, "OpenAI Compatibility", len(m.openai))
		for i, entry := range m.openai {
			name := getString(entry, "name")
			baseURL := getString(entry, "base-url")
			prefix := getString(entry, "prefix")
			info := name
			if prefix != "" {
				info += " (prefix: " + prefix + ")"
			}
			if baseURL != "" {
				info += " → " + baseURL
			}
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, info))
		}
		sb.WriteString("\n")
	}

	if m.status != "" {
		sb.WriteString(m.status)
		sb.WriteString("\n")
	}

	return sb.String()
}

func renderSection(sb *strings.Builder, title string, count int) {
	header := fmt.Sprintf("%s (%d)", title, count)
	sb.WriteString(tableHeaderStyle.Render("  " + header))
	sb.WriteString("\n")
}

func renderProviderKeys(sb *strings.Builder, title string, keys []map[string]any) {
	if len(keys) == 0 {
		return
	}
	renderSection(sb, title, len(keys))
	for i, key := range keys {
		apiKey := getString(key, "api-key")
		prefix := getString(key, "prefix")
		baseURL := getString(key, "base-url")
		info := maskKey(apiKey)
		if prefix != "" {
			info += " (prefix: " + prefix + ")"
		}
		if baseURL != "" {
			info += " → " + baseURL
		}
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, info))
	}
	sb.WriteString("\n")
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

func (m *keysTabModel) setEditInputs(entry accessAPIKeyEntry) {
	m.editInputs[0].SetValue(entry.APIKey)
	m.editInputs[1].SetValue(entry.ExpiresAt)
	if entry.TokenLimit > 0 {
		m.editInputs[2].SetValue(strconv.FormatInt(entry.TokenLimit, 10))
	} else {
		m.editInputs[2].SetValue("")
	}
	m.editingError = ""
}

func (m keysTabModel) currentEditEntry() (accessAPIKeyEntry, error) {
	entry := accessAPIKeyEntry{}
	entry.APIKey = strings.TrimSpace(m.editInputs[0].Value())
	if entry.APIKey == "" {
		return entry, fmt.Errorf("%s", T("key_field_required"))
	}
	entry.ExpiresAt = strings.TrimSpace(m.editInputs[1].Value())
	tokenLimit := strings.TrimSpace(m.editInputs[2].Value())
	if entry.ExpiresAt != "" {
		if _, err := time.Parse(time.RFC3339, entry.ExpiresAt); err != nil {
			return entry, fmt.Errorf("%s", T("key_field_expiry_invalid"))
		}
	}
	if tokenLimit != "" {
		parsed, err := strconv.ParseInt(tokenLimit, 10, 64)
		if err != nil || parsed < 0 {
			return entry, fmt.Errorf("%s", T("key_field_token_invalid"))
		}
		entry.TokenLimit = parsed
	}
	return entry, nil
}

func formatAccessKeyEntry(entry accessAPIKeyEntry) string {
	info := maskKey(entry.APIKey)
	meta := make([]string, 0, 2)
	if entry.ExpiresAt != "" {
		meta = append(meta, "expires-at: "+entry.ExpiresAt)
	}
	if entry.TokenLimit > 0 {
		meta = append(meta, fmt.Sprintf("token-limit: %d", entry.TokenLimit))
	}
	if len(meta) == 0 {
		return info
	}
	return info + " (" + strings.Join(meta, ", ") + ")"
}

func (m *keysTabModel) focusEditField(idx int) {
	if idx < 0 || idx >= len(m.editInputs) {
		return
	}
	for i := range m.editInputs {
		if i == idx {
			m.editInputs[i].Focus()
		} else {
			m.editInputs[i].Blur()
		}
	}
	m.editField = idx
}

func (m *keysTabModel) clearEditingState() {
	m.editing = false
	m.adding = false
	m.editField = 0
	m.editingError = ""
	for i := range m.editInputs {
		m.editInputs[i].Blur()
	}
}

func (m keysTabModel) renderEditForm(sb *strings.Builder) {
	modeLabel := T("new_key_prompt")
	if m.editing {
		modeLabel = T("edit_key_prompt")
	}
	sb.WriteString(helpStyle.Render(strings.TrimSpace(modeLabel)))
	sb.WriteString("\n")
	for i := range m.editInputs {
		sb.WriteString(m.editInputs[i].View())
		sb.WriteString("\n")
	}
	if m.editingError != "" {
		sb.WriteString(errorStyle.Render("✗ " + m.editingError))
		sb.WriteString("\n")
	}
	helpKey := "enter_add"
	if m.editing {
		helpKey = "enter_save_esc"
	}
	sb.WriteString(helpStyle.Render(T(helpKey)))
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render(T("key_form_nav_hint")))
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render(T("key_entry_hint")))
	sb.WriteString("\n")
}

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"

	baml_client "github.com/chris-regnier/gavel/baml_client"
	"github.com/chris-regnier/gavel/internal/config"
)

// Wizard state types
type wizardState int

const (
	stateMainMenu wizardState = iota
	stateCreatePolicy
	stateCreateRule
	stateCreatePersona
	stateCreateConfig
	stateGenerating
	stateResult
	stateError
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		MarginLeft(2)

	itemStyle = lipgloss.NewStyle().
		PaddingLeft(4)

	selectedItemStyle = lipgloss.NewStyle().
		PaddingLeft(2).
		Foreground(lipgloss.Color("#7D56F4"))

	descriptionStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#888888")).
		MarginLeft(4)

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262")).
		MarginTop(1)

	errorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF0000")).
		Bold(true)

	successStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00AA00")).
		Bold(true)
)

// Menu item
type menuItem struct {
	title       string
	description string
	state       wizardState
}

func (i menuItem) FilterValue() string { return i.title }
func (i menuItem) Title() string       { return i.title }
func (i menuItem) Description() string { return i.description }

// Wizard model
type wizardModel struct {
	state       wizardState
	list        list.Model
	textarea    textarea.Model
	textinput   textinput.Model
	category    string
	provider    string
	languages   string
	result      string
	err         error
	width       int
	height      int
}

// Messages
type generationDoneMsg struct {
	result string
	err    error
}

func launchCreateWizard() error {
	p := tea.NewProgram(initialWizardModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func initialWizardModel() wizardModel {
	// Create menu items
	items := []list.Item{
		menuItem{
			title:       "Create Policy",
			description: "Generate an AI policy from natural language description",
			state:       stateCreatePolicy,
		},
		menuItem{
			title:       "Create Rule",
			description: "Generate a regex-based rule from natural language",
			state:       stateCreateRule,
		},
		menuItem{
			title:       "Create Persona",
			description: "Generate a custom analysis persona",
			state:       stateCreatePersona,
		},
		menuItem{
			title:       "Create Full Config",
			description: "Generate a complete gavel configuration",
			state:       stateCreateConfig,
		},
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Gavel Configuration Wizard"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	// Create textarea for descriptions
	ta := textarea.New()
	ta.Placeholder = "Describe what you want to create..."
	ta.SetWidth(80)
	ta.SetHeight(10)

	// Create textinput for simple inputs
	ti := textinput.New()
	ti.Placeholder = "Enter value..."
	ti.Focus()

	return wizardModel{
		state:     stateMainMenu,
		list:      l,
		textarea:  ta,
		textinput: ti,
		category:  "maintainability",
		provider:  "openrouter",
		languages: "any",
	}
}

func (m wizardModel) Init() tea.Cmd {
	return nil
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(msg.Width-4, msg.Height-8)
		m.textarea.SetWidth(msg.Width - 8)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			if m.state != stateMainMenu && m.state != stateGenerating {
				m.state = stateMainMenu
				return m, nil
			}
		case "enter":
			switch m.state {
			case stateMainMenu:
				if i, ok := m.list.SelectedItem().(menuItem); ok {
					m.state = i.state
					m.textarea.SetValue("")
					m.textarea.Focus()
					return m, textarea.Blink
				}
			case stateCreatePolicy, stateCreatePersona, stateCreateConfig:
				if m.textarea.Value() != "" {
					m.state = stateGenerating
					return m, m.generateContent()
				}
			case stateCreateRule:
				// Rule creation has multiple steps - would need more complex flow
				if m.textarea.Value() != "" {
					m.state = stateGenerating
					return m, m.generateRule()
				}
			case stateResult, stateError:
				m.state = stateMainMenu
				return m, nil
			}
		}

	case generationDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateError
		} else {
			m.result = msg.result
			m.state = stateResult
		}
		return m, nil
	}

	// Handle state-specific updates
	switch m.state {
	case stateMainMenu:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	case stateCreatePolicy, stateCreatePersona, stateCreateConfig, stateCreateRule:
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m wizardModel) View() string {
	switch m.state {
	case stateMainMenu:
		return m.viewMainMenu()
	case stateCreatePolicy:
		return m.viewCreatePolicy()
	case stateCreateRule:
		return m.viewCreateRule()
	case stateCreatePersona:
		return m.viewCreatePersona()
	case stateCreateConfig:
		return m.viewCreateConfig()
	case stateGenerating:
		return m.viewGenerating()
	case stateResult:
		return m.viewResult()
	case stateError:
		return m.viewError()
	default:
		return "Unknown state"
	}
}

func (m wizardModel) viewMainMenu() string {
	help := helpStyle.Render("↑/↓: navigate • enter: select • q: quit")
	return fmt.Sprintf("%s\n\n%s\n\n%s",
		titleStyle.Render("🛠️  Gavel Configuration Wizard"),
		m.list.View(),
		help,
	)
}

func (m wizardModel) viewCreatePolicy() string {
	header := titleStyle.Render("Create Policy")
	desc := descriptionStyle.Render("Describe what you want this policy to check for:")
	help := helpStyle.Render("ctrl+enter: submit • esc: back • ctrl+c: quit")

	return fmt.Sprintf("%s\n\n%s\n\n%s\n\n%s",
		header,
		desc,
		m.textarea.View(),
		help,
	)
}

func (m wizardModel) viewCreateRule() string {
	header := titleStyle.Render("Create Rule")
	desc := descriptionStyle.Render("Describe the pattern you want to detect:")
	help := helpStyle.Render("ctrl+enter: submit • esc: back • ctrl+c: quit")

	return fmt.Sprintf("%s\n\n%s\n\n%s\n\n%s",
		header,
		desc,
		m.textarea.View(),
		help,
	)
}

func (m wizardModel) viewCreatePersona() string {
	header := titleStyle.Render("Create Persona")
	desc := descriptionStyle.Render("Describe the expert persona you want to create:")
	help := helpStyle.Render("ctrl+enter: submit • esc: back • ctrl+c: quit")

	return fmt.Sprintf("%s\n\n%s\n\n%s\n\n%s",
		header,
		desc,
		m.textarea.View(),
		help,
	)
}

func (m wizardModel) viewCreateConfig() string {
	header := titleStyle.Render("Create Full Configuration")
	desc := descriptionStyle.Render("Describe your project and what you want to analyze:")
	help := helpStyle.Render("ctrl+enter: submit • esc: back • ctrl+c: quit")

	return fmt.Sprintf("%s\n\n%s\n\n%s\n\n%s",
		header,
		desc,
		m.textarea.View(),
		help,
	)
}

func (m wizardModel) viewGenerating() string {
	return titleStyle.Render("🤖 Generating... please wait")
}

func (m wizardModel) viewResult() string {
	header := successStyle.Render("✅ Generation Complete!")
	content := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(1).
		Width(m.width - 4).
		Render(m.result)
	help := helpStyle.Render("enter/esc: return to menu • ctrl+c: quit")

	return fmt.Sprintf("%s\n\n%s\n\n%s",
		header,
		content,
		help,
	)
}

func (m wizardModel) viewError() string {
	header := errorStyle.Render("❌ Generation Failed")
	errMsg := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF6666")).
		Render(m.err.Error())
	help := helpStyle.Render("enter/esc: return to menu • ctrl+c: quit")

	return fmt.Sprintf("%s\n\n%s\n\n%s",
		header,
		errMsg,
		help,
	)
}

// generateContent handles generation based on current state
func (m wizardModel) generateContent() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		description := m.textarea.Value()

		switch m.state {
		case stateGenerating:
			// This was triggered from policy, persona, or config state
			// We need to check what the previous state was
			// For simplicity, we'll detect from content patterns
			return m.doGenerate(ctx, description)
		}

		return generationDoneMsg{err: fmt.Errorf("unknown generation state")}
	}
}

func (m wizardModel) doGenerate(ctx context.Context, description string) tea.Msg {
	// Determine what to generate based on what the user was doing
	// This is a simplified version - in practice we'd track the target type

	// Default to config generation
	genConfig, err := baml_client.GenerateConfig(ctx, description, "")
	if err != nil {
		return generationDoneMsg{err: fmt.Errorf("generating config: %w", err)}
	}

	cfg := &config.Config{
		Provider: config.ProviderConfig{
			Name: genConfig.Provider.ProviderName,
		},
		Persona:  genConfig.Persona,
		Policies: make(map[string]config.Policy),
	}

	switch genConfig.Provider.ProviderName {
	case "ollama":
		cfg.Provider.Ollama = config.OllamaConfig{
			Model:   genConfig.Provider.Model,
			BaseURL: genConfig.Provider.BaseUrl,
		}
	case "openrouter":
		cfg.Provider.OpenRouter = config.OpenRouterConfig{
			Model: genConfig.Provider.Model,
		}
	case "anthropic":
		cfg.Provider.Anthropic = config.AnthropicConfig{
			Model: genConfig.Provider.Model,
		}
	case "bedrock":
		cfg.Provider.Bedrock = config.BedrockConfig{
			Model:  genConfig.Provider.Model,
			Region: genConfig.Provider.Region,
		}
	case "openai":
		cfg.Provider.OpenAI = config.OpenAIConfig{
			Model: genConfig.Provider.Model,
		}
	}

	for _, p := range genConfig.Policies {
		cfg.Policies[p.Id] = config.Policy{
			Description: p.Description,
			Severity:    p.Severity,
			Instruction: p.Instruction,
			Enabled:     p.Enabled,
		}
	}

	yamlData, err := yaml.Marshal(cfg)
	if err != nil {
		return generationDoneMsg{err: fmt.Errorf("marshaling config: %w", err)}
	}

	return generationDoneMsg{result: string(yamlData)}
}

func (m wizardModel) generateRule() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		description := m.textarea.Value()

		rule, err := baml_client.GenerateRule(ctx, description, m.category, m.languages)
		if err != nil {
			return generationDoneMsg{err: fmt.Errorf("generating rule: %w", err)}
		}

		ruleMap := map[string]interface{}{
			"rules": []map[string]interface{}{{
				"id":          rule.Id,
				"name":        rule.Name,
				"category":    rule.Category,
				"pattern":     rule.Pattern,
				"level":       rule.Level,
				"confidence":  rule.Confidence,
				"message":     rule.Message,
				"explanation": rule.Explanation,
				"remediation": rule.Remediation,
				"source":      rule.Source,
			}},
		}

		if len(rule.Languages) > 0 {
			ruleMap["rules"].([]map[string]interface{})[0]["languages"] = rule.Languages
		}
		if len(rule.Cwe) > 0 {
			ruleMap["rules"].([]map[string]interface{})[0]["cwe"] = rule.Cwe
		}
		if len(rule.Owasp) > 0 {
			ruleMap["rules"].([]map[string]interface{})[0]["owasp"] = rule.Owasp
		}
		if len(rule.References) > 0 {
			ruleMap["rules"].([]map[string]interface{})[0]["references"] = rule.References
		}

		yamlData, err := yaml.Marshal(ruleMap)
		if err != nil {
			return generationDoneMsg{err: fmt.Errorf("marshaling rule: %w", err)}
		}

		return generationDoneMsg{result: string(yamlData)}
	}
}

// saveResult saves the generated content to the appropriate location
func saveResult(content string, configType string) (string, error) {
	switch configType {
	case "policy":
		return saveToFile(content, ".gavel/policies.yaml")
	case "rule":
		return saveToFile(content, ".gavel/rules/generated.yaml")
	case "persona":
		return saveToFile(content, ".gavel/personas.yaml")
	case "config":
		return saveToFile(content, ".gavel/policies.yaml")
	default:
		return "", fmt.Errorf("unknown config type: %s", configType)
	}
}

func saveToFile(content, path string) (string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating directory: %w", err)
	}

	// Check if file exists and append if so
	if _, err := os.Stat(path); err == nil {
		// File exists, append
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return "", fmt.Errorf("opening file: %w", err)
		}
		defer f.Close()

		// Add separator
		if _, err := f.WriteString("\n# --- Generated ---\n"); err != nil {
			return "", fmt.Errorf("writing separator: %w", err)
		}
		if _, err := f.WriteString(content); err != nil {
			return "", fmt.Errorf("writing content: %w", err)
		}
	} else {
		// New file
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("writing file: %w", err)
		}
	}

	return path, nil
}

// string helpers
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

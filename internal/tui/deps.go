package tui

// This file ensures TUI dependencies are included in go.mod
import (
	_ "github.com/alecthomas/chroma/v2"
	_ "github.com/charmbracelet/bubbletea"
	_ "github.com/charmbracelet/glamour"
	_ "github.com/charmbracelet/lipgloss"
)

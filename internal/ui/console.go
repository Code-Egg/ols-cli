package ui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
)

type Console interface {
	Section(title string)
	Info(message string)
	Warn(message string)
	Error(message string)
	Success(message string)
	Bullet(message string)
}

type StyledConsole struct {
	out          io.Writer
	sectionStyle lipgloss.Style
	infoStyle    lipgloss.Style
	warnStyle    lipgloss.Style
	errorStyle   lipgloss.Style
	successStyle lipgloss.Style
	bulletStyle  lipgloss.Style
}

func NewStyledConsole(out io.Writer) StyledConsole {
	return StyledConsole{
		out:          out,
		sectionStyle: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).MarginTop(1),
		infoStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		warnStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
		errorStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		successStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true),
		bulletStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true),
	}
}

func (c StyledConsole) Section(title string) {
	fmt.Fprintln(c.out, c.sectionStyle.Render("▶ "+title))
}

func (c StyledConsole) Info(message string) {
	fmt.Fprintln(c.out, c.infoStyle.Render("ℹ "+message))
}

func (c StyledConsole) Warn(message string) {
	fmt.Fprintln(c.out, c.warnStyle.Render("⚠ "+message))
}

func (c StyledConsole) Error(message string) {
	fmt.Fprintln(c.out, c.errorStyle.Render("✖ "+message))
}

func (c StyledConsole) Success(message string) {
	fmt.Fprintln(c.out, c.successStyle.Render("✔ "+message))
}

func (c StyledConsole) Bullet(message string) {
	fmt.Fprintln(c.out, c.bulletStyle.Render("• ")+message)
}

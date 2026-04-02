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
	out           io.Writer
	sectionStyle  lipgloss.Style
	sectionPrefix lipgloss.Style
	infoStyle     lipgloss.Style
	warnStyle     lipgloss.Style
	errorStyle    lipgloss.Style
	successStyle  lipgloss.Style
	bulletStyle   lipgloss.Style
	bulletText    lipgloss.Style
	okBadge       lipgloss.Style
	warnBadge     lipgloss.Style
	errorBadge    lipgloss.Style
	infoBadge     lipgloss.Style
}

func NewStyledConsole(out io.Writer) StyledConsole {
	return StyledConsole{
		out:           out,
		sectionStyle:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).MarginTop(1),
		sectionPrefix: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")),
		infoStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		warnStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Bold(true),
		errorStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true),
		successStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("120")).Bold(true),
		bulletStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true),
		bulletText:    lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		okBadge:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("28")).Padding(0, 1),
		warnBadge:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("234")).Background(lipgloss.Color("214")).Padding(0, 1),
		errorBadge:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("196")).Padding(0, 1),
		infoBadge:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("63")).Padding(0, 1),
	}
}

func (c StyledConsole) Section(title string) {
	fmt.Fprintln(c.out, c.sectionPrefix.Render("◆")+" "+c.sectionStyle.Render(title))
}

func (c StyledConsole) Info(message string) {
	fmt.Fprintln(c.out, c.infoBadge.Render("INFO")+" "+c.infoStyle.Render(message))
}

func (c StyledConsole) Warn(message string) {
	fmt.Fprintln(c.out, c.warnBadge.Render("WARN")+" "+c.warnStyle.Render(message))
}

func (c StyledConsole) Error(message string) {
	fmt.Fprintln(c.out, c.errorBadge.Render("FAIL")+" "+c.errorStyle.Render(message))
}

func (c StyledConsole) Success(message string) {
	fmt.Fprintln(c.out, c.okBadge.Render("OK")+" "+c.successStyle.Render(message))
}

func (c StyledConsole) Bullet(message string) {
	fmt.Fprintln(c.out, c.bulletStyle.Render("•")+" "+c.bulletText.Render(message))
}

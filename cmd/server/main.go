package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/divizn/ssh-battleships/internal/tui"
)

func main() {
	if _, err := tea.NewProgram(tui.New()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

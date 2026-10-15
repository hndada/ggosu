package main

import (
	"fmt"
	"os"

	"ggosu/internal/menu"
)

// App keeps the existing window.go.main.App binding through Go composition.
// Menu state and implementation live in internal/menu.
type App struct{ menu.App }

func main() {
	app := &App{}
	if err := menu.Run(&app.App, app); err != nil {
		fmt.Fprintln(os.Stderr, "Gosu:", err)
		os.Exit(1)
	}
}

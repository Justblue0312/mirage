package cli

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
)

var useColor = true

func init() {
	// Disable colors if NO_COLOR environment variable is set
	if os.Getenv("NO_COLOR") != "" {
		useColor = false
		return
	}
	// Check if stdout is a terminal
	if !isTTY(os.Stdout.Fd()) {
		useColor = false
	}
}

func isTTY(fd uintptr) bool {
	return term.IsTerminal(int(fd))
}

func success(msg string) {
	if useColor {
		fmt.Printf("%s✓%s %s\n", colorGreen, colorReset, msg)
	} else {
		fmt.Printf("✓ %s\n", msg)
	}
}

func info(msg string) {
	if useColor {
		fmt.Printf("%sℹ%s %s\n", colorCyan, colorReset, msg)
	} else {
		fmt.Printf("ℹ %s\n", msg)
	}
}

func warn(msg string) {
	if useColor {
		fmt.Printf("%s⚠%s %s\n", colorYellow, colorReset, msg)
	} else {
		fmt.Printf("⚠ %s\n", msg)
	}
}

func errMsg(msg string) {
	if useColor {
		fmt.Printf("%s✗%s %s\n", colorRed, colorReset, msg)
	} else {
		fmt.Printf("✗ %s\n", msg)
	}
}

func header(text string) {
	if useColor {
		fmt.Printf("\n%s%s%s\n", colorBold, text, colorReset)
	} else {
		fmt.Printf("\n%s\n", text)
	}
}

func dim(text string) {
	if useColor {
		fmt.Printf("%s%s%s\n", colorDim, text, colorReset)
	} else {
		fmt.Printf("%s\n", text)
	}
}

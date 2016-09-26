package godog

import (
	"fmt"
	"os"
	"strings"
)

// empty struct value takes no space allocation
type void struct{}

// a color code type
type color int

const ansiEscape = "\x1b"

// some ansi colors
const (
	black color = iota + 30
	red
	green
	yellow
	blue
	magenta
	cyan
	white
)

// colorizes foreground s with color c
func cl(s interface{}, c color) string {
	return fmt.Sprintf("%s[%dm%v%s[0m", ansiEscape, c, s, ansiEscape)
}

// colorizes foreground s with bold color c
func bcl(s interface{}, c color) string {
	return fmt.Sprintf("%s[1;%dm%v%s[0m", ansiEscape, c, s, ansiEscape)
}

// repeats a space n times
func s(n int) string {
	return strings.Repeat(" ", n)
}

// checks the error and exits with error status code
func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

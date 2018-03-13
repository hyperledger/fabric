package godog

import (
	"strings"
	"time"

	"github.com/DATA-DOG/godog/colors"
)

// empty struct value takes no space allocation
type void struct{}

var red = colors.Red
var redb = colors.Bold(colors.Red)
var green = colors.Green
var black = colors.Black
var blackb = colors.Bold(colors.Black)
var yellow = colors.Yellow
var cyan = colors.Cyan
var cyanb = colors.Bold(colors.Cyan)
var whiteb = colors.Bold(colors.White)

// repeats a space n times
func s(n int) string {
	return strings.Repeat(" ", n)
}

var timeNowFunc = func() time.Time {
	return time.Now()
}

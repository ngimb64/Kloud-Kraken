package display

import (
	"fmt"
	"time"
)

// Package level variables
const AnsiClear = "\x1b[H\x1b[2J"
const AnsiReset = "\033[0m"


// Clear the terminal display with a sleep prior if specified.
//
// @Parameters
// - sleepTime:  The number of seconds to sleep before clearing the display
//
func ClearScreen(sleepTime int) {
    // If there was a positive amount of sleep time, then sleep
    if sleepTime > 0 {
        time.Sleep(time.Duration(sleepTime) * time.Second)
    }

    // ANSI escape code to clear the screen
    fmt.Print(AnsiClear)
}


// Wraps text with the given color code and returns the colored string.
//
// @Parameters
// - color:  The ANSI color to format the string
// - text:  The content of the colored string
//
// @Returns
// - Formated colorized string
//
func Ctext(color string, text string) string {
    return color + text + AnsiReset
}


// Takes pairs of color and text and applies color to each text portion.
//
// @Parameters
// - pairs:  The pairs of ANSI color code and text it applies to
//
// @Returns
// - The resulting formatted string
//
func CtextMulti(pairs ...string) string {
    var result string

    // Iterate through color/text pairs
    for i := 0; i < len(pairs); i += 2 {
        // Colorize the pair and append it to the result
        result += Ctext(pairs[i], pairs[i+1])
    }

    return result
}


// Wraps text inside bracket like [TEXT], or with sybols like [+], [!], etc.
//
// @Parameters
// - bracketColor:  The ANSI color be assigned to the outer brackets
// - innerColor:  The ANSI color be assigned to the content inside the brackets
// - innerContent:  The content to be written inside the brackets
//
// @Returns
// - The formated colorized message prefix
//
func CtextPrefix(bracketColor string, innerColor string,
                 innerContent string) string {
    return bracketColor + "[" + innerColor + innerContent + bracketColor + "] " + AnsiReset
}

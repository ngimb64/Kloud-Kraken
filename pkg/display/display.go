package display

import (
	"fmt"
	"time"
)

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
    fmt.Print("\x1b[H\x1b[2J")
}

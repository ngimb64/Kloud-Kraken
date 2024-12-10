package display

import (
	"fmt"
	"time"
)

func ClearScreen(sleepTime int) {
	// If there was a positive amount of sleep time, then sleep
	if sleepTime > 0 {
		time.Sleep(time.Duration(sleepTime) * time.Second)
	}

	// ANSI escape code to clear the screen
	fmt.Print("\x1b[H\x1b[2J")
}

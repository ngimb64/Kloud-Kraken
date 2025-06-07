package tui

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pterm/pterm"
)

// Package level variables
const AnsiReset = "\033[0m"


// TUI manages a two-panel display: left=panel1, right=panel2.
type TUI struct {
    area             *pterm.AreaPrinter
    first            bool
    leftPanelBuffer  []string
    LeftPanelCh      chan string
    leftPanelName    string
    maxBuffer        int
    mutx             sync.Mutex
    redrawInterval   time.Duration
    rightColOffset   uint16
    rightPanelBuffer []string
    RightPanelCh     chan string
    rightPanelName   string
    stopCh           chan struct{}
}

// Creates a new TUI instance with given channel buffer sizes.
//
// @Parameters
// - maxBuffer:  The max buffer size for chaneels and pannels
// - leftPanelName:  The name of the header for the left panel
// - redrawInterval:  The duration of time until the display panels are updated
// - rightColOffset:  The offset used to set the position of the right column
//                    (2 for split 50-50, 3 for 33.3-66.6, 4 for 25-75, 5 for 20-80)
// - leftPanelName:  The name of the header for the left panel
//
func NewTUI(maxBuffer int, leftPanelName string, redrawInterval time.Duration,
            rightColOffset uint16, rightPanelName string) *TUI {
    return &TUI{
        first:          true,
        leftPanelBuffer:  make([]string, 0, maxBuffer),
        LeftPanelCh:      make(chan string, maxBuffer),
        leftPanelName:    leftPanelName,
        maxBuffer:        maxBuffer,
        redrawInterval:   redrawInterval,
        rightColOffset:   rightColOffset,
        rightPanelBuffer: make([]string, 0, maxBuffer),
        RightPanelCh:     make(chan string, maxBuffer),
        rightPanelName:   rightPanelName,
        stopCh:           make(chan struct{}),
    }
}

// Runs the continual ticker loop that handles TUI operations.
//
// @Parameters
// - leftPanelHeaderColor:  The color of the left panel header
// - rightPanelHeaderColor:  The color of the right panel header
// - dividerColor:  The color of the divider used to split header and content section
//
func (t *TUI) Start(leftPanelHeaderColor string, rightPanelHeaderColor string,
                    dividerColor string) {
    // Set up ticker for monitoring on intervals
    ticker := time.NewTicker(t.redrawInterval)
    // Stop ticker on local exit
    defer ticker.Stop()

    for {
        select {
        // If there is data in the left panel buffer
        case msg := <-t.LeftPanelCh:
            t.mutx.Lock()
            // Add the message to the left panel buffer slice
            t.leftPanelBuffer = append(t.leftPanelBuffer, msg)
            // Ensure the message does not overflow its column
            t.leftPanelBuffer = t.trimToMax(t.leftPanelBuffer, t.maxBuffer)
            t.mutx.Unlock()

        // If there is data in the right panel buffer
        case msg := <-t.RightPanelCh:
            t.mutx.Lock()
            // Add the message to the right panel buffer slice
            t.rightPanelBuffer = append(t.rightPanelBuffer, msg)
            // Ensure the message does not overflow its column
            t.rightPanelBuffer = t.trimToMax(t.rightPanelBuffer, t.maxBuffer)
            t.mutx.Unlock()

        // If the ticker interval has been reached
        case <-ticker.C:
            t.mutx.Lock()
            // Make a copy of each pannels buffer for rendering output
            bufferLeftCopy := slices.Clone(t.leftPanelBuffer)
            bufferRightCopy := slices.Clone(t.rightPanelBuffer)
            t.mutx.Unlock()

            // If the first ticker occurs
            if t.first {
                // Draw the TUIs static frame
                t.renderStaticFrame(leftPanelHeaderColor, rightPanelHeaderColor,
                                    dividerColor)
                t.first = false
            }

            // Update the content area with data received from buffers
            t.updateContent(bufferLeftCopy, bufferRightCopy)

        // If the stop channel has been closed
        case <-t.stopCh:
            return
        }
    }
}

// Stop signals the TUI to exit its update loop.
func (t *TUI) Stop() {
    close(t.stopCh)
}

// Renders the headers, divider, and dynamic static area where output
// will populate over time.
//
// @Parameters
// - panel1HeaderColor:  The color of the left panel text header
// - panel2HeaderColor:  The color of the right panel text header
// - dividerColor:  The color of the line divider bewteen headers and dynamic text area
//
func (t *TUI) renderStaticFrame(panel1HeaderColor string, panel2HeaderColor string,
                                dividerColor string) {
    // Start with a fresh display
    fmt.Print("\033[2J")

    // Get the terminal display width
    width := pterm.GetTerminalWidth()
    // Calculate column split
    leftW := (width - 1) / int(t.rightColOffset)
    rightW := width - leftW

    // Draw the headers for the first row
    h1 := t.padOrTrim(panel1HeaderColor + t.leftPanelName + AnsiReset, leftW)
    h2 := t.padOrTrim(panel2HeaderColor + t.rightPanelName + AnsiReset, rightW)
    // Move cursor to (row 1, col 1)
    fmt.Printf("\033[1;1H%s%s", h1, h2)

    // Draw banner on the first row
    fmt.Print(dividerColor + strings.Repeat("-", width) + AnsiReset)

    // Move cursor to row 3, col 1
    fmt.Print("\033[3;1H")
    // Start one AreaPrinter that “lives” at row 3, col 1
    area, _ := pterm.DefaultArea.Start()
    t.area = area
}

// Calculates the current terminal width and height, then determines the
// appropriate number of rows and column widths for each panel. It pads or
// trims each line to fit its respective column width and merges the formatted
// lines into a single output. The resulting combined view is written to the
// shared AreaPrinter (t.area).
//
// @Parameters
// - bufferLeft:  The most recent content for the left panel
// - bufferRight:  The most recent content for the right panel
//
func (t *TUI) updateContent(bufferLeft []string, bufferRight []string) {
    // Get the terminal display height and width
    height := pterm.GetTerminalHeight()
    width := pterm.GetTerminalWidth()
    // Compute column widths
    leftW := (width - 1) / int(t.rightColOffset)
    rightW := width - leftW - 1

    // We only have (height−2) rows for content (rows 2..height−1)
    contentRows := max(height - 2, 0)

    // Trim each buffer to at most contentRows lines
    bufferLeft = t.trimToMax(bufferLeft, contentRows)
    bufferRight = t.trimToMax(bufferRight, contentRows)

    lines := make([]string, contentRows)

    // Iterate through slice of content rows
    for row := range contentRows {
        var leftLine, rightLine string

        // If there is a line from bufferLeft for this row, format it
        if row < len(bufferLeft) {
            leftLine = t.padOrTrim(bufferLeft[row], leftW)
        // Otherwise fill with spaces
        } else {
            leftLine = strings.Repeat(" ", leftW)
        }

        // If there is a line from bufferRight for this row, format it
        if row < len(bufferRight) {
            rightLine = t.padOrTrim(bufferRight[row], rightW)
        // Otherwise fill with spaces
        } else {
            rightLine = strings.Repeat(" ", rightW)
        }

        // Combine left and right pane lines into a single full-width line
        lines[row] = leftLine + rightLine
    }

    // Update the single AreaPrinter (t.area) with the joined lines
    t.area.Update(strings.Join(lines, "\n"))
}

// Ensures a string is either padded or trimmed to fit a fixed display width,
// accounting for ANSI color codes which do not consume visible space.
//
// @Parameters
// - value:  The string to format, possibly containing ANSI escape sequences
// - width:  The target visible width for the string in terminal columns
//
// @Returns
// - A new string exactly `width` characters wide in visible length, either
//   padded with spaces or truncated, preserving ANSI formatting
//
func (t *TUI) padOrTrim(value string, width int) string {
    // Get the visable length of string (non ANSI escape codes)
    vis := t.stripAnsiLength(value)
    // If the string fits in buffer with padding space
    if vis < width {
        // Pad to width (ensuring visual width == width)
        return value + strings.Repeat(" ", width-vis)
    }

    // If the string exactly fits, it needs to be cut to allow space
    if vis == width {
        return t.truncateAnsi(value, width - 1) + " "
    }

    // Truncate and ensure 1 space at the end if needed
    truncated := t.truncateAnsi(value, width - 1)
    return truncated + " "
}

// Calculates the visible length of a string, ignoring ANSI escape codes
// used for terminal text formatting (e.g., colors).
//
// @Parameters
// - s:  The string to measure, potentially containing ANSI sequences
//
// @Returns
// - The number of visible characters, excluding ANSI control sequences
//
func (t *TUI) stripAnsiLength(s string) int {
    count := 0
    inAnsi := false

    // Loop over each byte in the string
    for i := range len(s) {
        // Detect the beginning of an ANSI escape sequence (ESC + '[')
        if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
            inAnsi = true
            continue
        }

        // If currently inside an ANSI sequence, look for its end
        if inAnsi {
            // ANSI sequences typically end with a letter (e.g., 'm', 'K', etc.)
            if ('a' <= s[i] && s[i] <= 'z') || ('A' <= s[i] && s[i] <= 'Z') {
                inAnsi = false
            }

            continue
        }

        count++
    }

    return count
}

// Ensures a the passed in string size is limited to its max size and
// any overflow will be discarded.
//
// @Parameters
// - buffer:  The buffer to check size to max value and trim if needed
// - maxSize:  The maximum allowed size of the buffer
//
// @Returns
// - The string buffer trimmed to the max value
//
func (t *TUI) trimToMax(buffer []string, maxSize int) []string {
    // If the buffer is above the max size, trim it
    if len(buffer) > maxSize {
        return buffer[len(buffer)-maxSize:]
    }

    return buffer
}

// Truncates a string to a maximum number of visible characters, preserving
// ANSI formatting and properly closing any open sequences to avoid rendering
// issues.
//
// @Parameters
// - s:  The original string with optional ANSI color codes
// - n:  The desired maximum visible character length
//
// @Returns
// - A new string containing at most `n` visible characters, ending with
//   an ANSI reset code to ensure consistent formatting
//
func (t *TUI) truncateAnsi(s string, n int) string {
    result := ""
    visible := 0
    inAnsi := false

    // Loop over each byte of the string
    for i := range len(s) {
         // If inside an ANSI sequence, append byte and check for end ANSI sequence
        if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
            inAnsi = true
        }

        if inAnsi {
            result += string(s[i])
            // ANSI sequences typically end with a letter (e.g., 'm', 'K', etc.)
            if ('a' <= s[i] && s[i] <= 'z') || ('A' <= s[i] && s[i] <= 'Z') {
                inAnsi = false
            }

            continue
        }

        // If not inside an ANSI sequence, add to result only if limit not reached
        if visible < n {
            result += string(s[i])
            visible++
        }
    }

    return result + AnsiReset
}

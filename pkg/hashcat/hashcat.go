package hashcat

import (
	"bytes"
	"fmt"
	"log"
	"regexp"
	"sort"

	"github.com/ngimb64/Kloud-Kraken/pkg/data"
	"go.uber.org/zap"
)

// Iterate through the parsed charsets and append them to the command options slice
// until an empty charset is met.
//
// @Parameters
// - cmdOptions:  The string slice of command args to be passed into hashcat
//
func AppendCharsets(cmdOptions *[]string, charsets []string) {
    var counter int32 = 1

    // Iterate through hashcat charsets
    for _, charset := range charsets {
        // Exit loop is charset is empty or counter is greater than max charset
        if charset == "" || counter > 4 {
            break
        }

        // Append the formated charset flag and corresponding charset
        *cmdOptions = append(*cmdOptions, fmt.Sprintf("-%d", counter), charset)
        counter += 1
    }
}


// Parses the final section of hashcat output where result statistics reside,
// splits the parsed section by newlines into slice, iterates through split slice
// and trims the data before and after the colon delimiter into key-value variables
// that are mapped to a map. The keys are sorted and iterated over to log the parsed
// output in order established by the keys.
//
// @Parameters
// - output:  Buffer where hashcat output is stored and to be parsed
//
func ParseHashcatOutput(output []byte, delimiter []byte) []any {
    var keys []string
    var logArgs []any
    // Make a map to store parsed data
    outputMap := make(map[string]string)

    // Trim up to the end section with result data
    parsedOutput, err := data.TrimAfterLast(output, delimiter)
    if err != nil {
        log.Fatalf("Error pre-trimming:  %v", err)
    }

    // Split the byte slice into lines base on newlines
    lines := bytes.Split(parsedOutput, []byte("\n"))
    // Compile regular expression to match a period at end variable length
    rePeriodTrim := regexp.MustCompile(`\.*$`)

    // Iterate through slice of byte slice lines
    for _, line := range lines {
        // Find the first occurance of the colon separator
        index := bytes.Index(line, []byte(":"))
        // If the line does not contain the index, skip it
        if index == -1 {
            continue
        }

        // Extract the key/value based on the colon separator
        key := bytes.TrimSpace(line[:index])
        value := bytes.TrimSpace(line[index+1:])

        // If there are any periods at the ent of the string
        if rePeriodTrim.Match(key) {
            key = data.TrimEndChars(key, byte('.'))
        }

        // If there is a $ delimiter on end, trim it
        if bytes.HasSuffix(value, []byte("$")) {
            value = bytes.TrimSuffix(value, []byte("$"))
        }

        keyStr := string(key)
        // Append the key to the keys string slice
        keys = append(keys, keyStr)
        // Store the key and value as strings in map
        outputMap[keyStr] = string(value)
    }

    // Sort the keys by alphabetical order
    sort.Strings(keys)

    // Iterate through the sorted keys
    for _, key := range keys {
        // Add the key/value from output map based on sorted key value
        logArgs = append(logArgs, zap.String(key, outputMap[key]))
    }

    return logArgs
}

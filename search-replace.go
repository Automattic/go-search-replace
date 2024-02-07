package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"sync"
	"unsafe"
)

const (
	searchRe  = `s:\d+:\\\".*?\\\";`
	replaceRe = `s:\d+:\\\"(.*?)\\\";`

	badInputRe   = `\w:\d+:`
	inputRe      = `^[A-Za-z0-9_\-\.:/]+$`
	minInLength  = 4
	minOutLength = 2

	version = "0.0.6-dev"
)

var (
	search  = regexp.MustCompile(searchRe)
	replace = regexp.MustCompile(replaceRe)
	input   = regexp.MustCompile(inputRe)
	bad     = regexp.MustCompile(badInputRe)
)

// Replacement has two fields (both byte slices): "From" & "To"
type Replacement struct {
	From []byte
	To   []byte
}

func main() {
	versionFlag := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("go-search-replace version %s\n", version)
		os.Exit(0)
		return
	}

	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Usage: search-replace <from> <to>")
		os.Exit(1)
		return
	}

	var replacements []*Replacement
	args := os.Args[1:]

	if len(args)%2 > 0 {
		fmt.Fprintln(os.Stderr, "All replacements must have a <from> and <to> value")
		os.Exit(1)
		return
	}

	var from, to string
	for i := 0; i < len(args)/2; i++ {
		from = args[i*2]
		if !validInput(from, minInLength) {
			fmt.Fprintln(os.Stderr, "Invalid <from> URL, minimum length is 4")
			os.Exit(2)
			return
		}

		to = args[(i*2)+1]
		if !validInput(to, minOutLength) {
			fmt.Fprintln(os.Stderr, "Invalid <to>, minimum length is 2")
			os.Exit(3)
			return
		}

		replacements = append(replacements, &Replacement{
			From: []byte(from),
			To:   []byte(to),
		})
	}

	var wg sync.WaitGroup
	lines := make(chan chan []byte, 10)

	wg.Add(1)
	go func() {
		defer wg.Done()

		r := bufio.NewReaderSize(os.Stdin, 2*1024*1024)
		for {
			line, err := r.ReadBytes('\n')

			if err != nil {
				if err == io.EOF {
					if 0 == len(line) {
						break
					}
				} else {
					fmt.Fprintln(os.Stderr, err.Error())
					break
				}
			}

			wg.Add(1)
			ch := make(chan []byte)
			lines <- ch

			go func(line *[]byte) {
				defer wg.Done()
				line = fixLine(line, replacements)
				ch <- *line
			}(&line)
		}
	}()

	go func() {
		wg.Wait()
		close(lines)
	}()

	for line := range lines {
		fmt.Print(unsafeGetString(<-line))
	}
}

var debugMode = false

func Debugf(format string, args ...interface{}) {
    if debugMode {
        fmt.Printf(format, args...)
    }
}

func fixLine(line *[]byte, replacements []*Replacement) *[]byte {
	serializedStringRegexp := regexp.MustCompile(`s:(\d+):\\"`)

	//if !bytes.Contains(*line, []byte("s:")) {
	//	return line
	//}

	startIndex := 0
	for startIndex < len(*line) {
		Debugf("Start of loop, startIndex: %d\n", startIndex)
		match := serializedStringRegexp.FindSubmatchIndex((*line)[startIndex:])
		if match == nil {
			break
		}

		length, err := strconv.Atoi(string((*line)[startIndex+match[2] : startIndex+match[3]]))
		if err != nil {
			startIndex++
			continue
		}
		Debugf("Match found, length: %d\n", length)

		contentStart := startIndex + match[1]
		contentEnd := contentStart + length

		Debugf("Content boundaries, start: %d, end: %d\n", contentStart, contentEnd)

		serializedContent := (*line)[contentStart:contentEnd]
		Debugf("Content before: %s\n", serializedContent)
		updatedContent := replaceInSerializedBytes(serializedContent, replacements)
		Debugf("Content after: %s\n\n", updatedContent)

		// no change, move to the next one
		if bytes.Equal(serializedContent, updatedContent) {
			startIndex = contentEnd + len(`\";`)
			Debugf("No replacements made; skipping to %d: %s\n", startIndex, updatedContent)
			// TODO: fix
			continue
		}

		// Calculate the new length and update the serialized length prefix
		newLength := len(updatedContent)
		newLengthStr := []byte(strconv.Itoa(newLength))
		Debugf("Replaced content new length: %d\n", newLength)
		*line = append((*line)[:startIndex+match[2]], append(newLengthStr, (*line)[startIndex+match[3]:]...)...)
		Debugf("After updating length prefix: %s\n", string(*line))

		// Update the serialized content inline

		//contentEnd = contentStart + newLength // adjust the end index based on the new length -- THIS BREAKS THINGS

		*line = append((*line)[:contentStart], append(updatedContent, (*line)[contentEnd:]...)...)
		Debugf("After updating content: %s\n", string(*line))

		// Adjust startIndex for the next iteration
		startIndex += match[1] + newLength + len(newLengthStr) - len((*line)[startIndex+match[2]:startIndex+match[3]])
		Debugf("New startIndex: %d\n", startIndex)
	}

	Debugf("Doing global replacements: %s\n", string(*line))
	// Catch anything left
	for _, replacement := range replacements {
		*line = bytes.ReplaceAll(*line, replacement.From, replacement.To)
		Debugf("After global replacement (from: %s | to: %s): %s\n", replacement.From, replacement.To, string(*line))
	}

	Debugf("All done: %s\n", string(*line))

	return line
}

func replaceInSerializedBytes(serialized []byte, replacements []*Replacement) []byte {
	for _, replacement := range replacements {
		serialized = bytes.ReplaceAll(serialized, replacement.From, replacement.To)
	}
	return serialized
}

func replaceAndFix(line *[]byte, replacements []*Replacement) *[]byte {
	for _, replacement := range replacements {
		if !bytes.Contains(*line, replacement.From) {
			continue
		}

		// Find/replace from->to
		*line = bytes.Replace(*line, replacement.From, replacement.To, -1)

		// Fix serialized string lengths
		*line = search.ReplaceAllFunc(*line, func(match []byte) []byte {
			// Skip fixing if we didn't replace anything
			if !bytes.Contains(match, replacement.To) {
				return match
			}

			return fix(&match)
		})
	}

	return line
}

func fix(match *[]byte) []byte {
	parts := replace.FindSubmatch(*match)

	if len(parts) != 2 {
		// This looks wrong, don't touch anything
		return *match
	}

	// Get string length - number of escaped characters and avoid double counting escaped \
	length := strconv.Itoa(len(parts[1]) - (bytes.Count(parts[1], []byte(`\`)) - bytes.Count(parts[1], []byte(`\\`))))

	// Allocate enough memory for the string so appending won't resize it
	// length of the string +
	// length of constant characters +
	// number of digits in the "length" component
	replaced := make([]byte, 0, len(parts[1])+8+len(length))

	// Build the string
	replaced = append(replaced, []byte("s:")...)
	replaced = append(replaced, []byte(length)...)
	replaced = append(replaced, ':')
	replaced = append(replaced, []byte("\\\"")...)
	replaced = append(replaced, parts[1]...)
	replaced = append(replaced, []byte("\\\";")...)
	return replaced
}

func validInput(in string, length int) bool {
	if len(in) < length {
		return false
	}

	if !input.MatchString(in) {
		return false
	}

	if bad.MatchString(in) {
		return false
	}

	return true
}

func unsafeGetString(bs []byte) string {
	return *(*string)(unsafe.Pointer(&bs))
}

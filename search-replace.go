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

			if err == io.EOF {
				break
			}

			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				break
			}

			wg.Add(1)
			ch := make(chan []byte)
			lines <- ch

			go func(line *[]byte) {
				defer wg.Done()
				line = replaceAndFix(line, replacements)
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

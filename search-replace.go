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

type EscapedDataDetails struct {
	ContentStartIndex int
	ContentEndIndex   int
	NextPartIndex     int
	CurrentPartIndex  int
	OriginalByteSize  int
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
	if bytes.Contains(*line, []byte("s:")) {
		line = fixSerializedContent(line, replacements)
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

func fixSerializedContent(line *[]byte, replacements []*Replacement) *[]byte {
	index := 0

	var rebuiltLine []byte

	for index < len(*line) {
		Debugf("Start of loop, index: %d\n", index)
		linePart := (*line)[index:]

		details, err := parseEscapedData(linePart)

		if err != nil {
			if err.Error() == "could not find serialized string prefix" && index == 0 {
				return line
			}
			// we've run out of things to parse, so just break out and append the rest
			rebuiltLine = append(rebuiltLine, linePart...)
			break
		}

		rebuiltLine = append(rebuiltLine, (*line)[index:index+details.CurrentPartIndex]...)

		index = index + details.NextPartIndex

		content := linePart[details.ContentStartIndex : details.ContentEndIndex+1]

		updatedContent := replaceInSerializedBytes(content, replacements)

		// php needs the unescaped length, so let's unescape it and measure the length
		contentLength := len(unescapeContent(updatedContent))

		// but if the content never changed, we'll let the error be for safety.
		if bytes.Equal(content, updatedContent) {
			contentLength = details.OriginalByteSize
		}

		// and we rebuild the string
		rebuilt := "s:" + strconv.Itoa(contentLength) + ":\\\"" + string(updatedContent) + "\\\";"

		rebuiltLine = append(rebuiltLine, []byte(rebuilt)...)
	}

	return &rebuiltLine
}

func replaceInSerializedBytes(serialized []byte, replacements []*Replacement) []byte {
	for _, replacement := range replacements {
		serialized = bytes.ReplaceAll(serialized, replacement.From, replacement.To)
	}
	return serialized
}

var serializedStringPrefixRegexp = regexp.MustCompile(`s:(\d+):`)

// Parses escaped data, returning the location details for further parsing
func parseEscapedData(linePart []byte) (*EscapedDataDetails, error) {

	details := EscapedDataDetails{
		ContentStartIndex: 0,
		ContentEndIndex:   0,
		NextPartIndex:     0,
		CurrentPartIndex:  0,
		OriginalByteSize:  0,
	}

	// find starting point in the line
	//TODO: We should first check if we found the string when inside a quote or not.
	// but currently skipping that scenario because it seems unlikely to find it outside.
	match := serializedStringPrefixRegexp.FindSubmatchIndex(linePart)
	if match == nil {
		return nil, fmt.Errorf("could not find serialized string prefix")
	}

	matchedAt := match[0]
	originalBytes := linePart[match[2]:match[3]]

	details.OriginalByteSize, _ = strconv.Atoi(string(originalBytes))

	details.CurrentPartIndex = matchedAt

	// the following assumes escaped double quotes
	//TODO: MySQL can optionally not escape the double quote,
	// but generally sqldumps always include the quotes.
	initialContentIndex := match[3] + 3

	details.ContentStartIndex = initialContentIndex

	currentContentIndex := initialContentIndex

	contentByteCount := 0

	var nextPartIndex int

	backslash := byte('\\')
	semicolon := byte(';')
	quote := byte('"')
	nextPartFound := false

	secondMatch := serializedStringPrefixRegexp.FindSubmatchIndex(linePart[matchedAt+1:])

	maxIndex := len(linePart) - 1

	if secondMatch != nil {
		maxIndex = secondMatch[0] + matchedAt
	}

	// let's find where the content actually ends.
	// it should end when the unescaped value is `";`
	for currentContentIndex < len(linePart) {
		if currentContentIndex+2 > maxIndex {

			// this algorithm SHOULD work, but in cases where the original byte count does not match
			// the actual byte count, it'll error out. We'll add this safeguard here.
			return nil, fmt.Errorf("faulty data, byte count does not match data size")
		}
		char := linePart[currentContentIndex]
		secondChar := linePart[currentContentIndex+1]
		thirdChar := linePart[currentContentIndex+2]
		if char == backslash && contentByteCount < details.OriginalByteSize {
			unescapedBytePair := getUnescapedBytesIfEscaped(linePart[currentContentIndex : currentContentIndex+2])
			// if we get the byte pair without the backslash, it corresponds to a byte
			contentByteCount += len(unescapedBytePair)

			// content index count remains the same.
			currentContentIndex += 2
			continue
		}

		if char == backslash && secondChar == quote && thirdChar == semicolon && contentByteCount >= details.OriginalByteSize {

			// since we've filtered out all the escaped value already, this should be the actual end
			nextPartIndex = currentContentIndex + 3
			details.NextPartIndex = nextPartIndex
			// we're at backslash, so we need to minus 1 to get the index where the content finishes
			details.ContentEndIndex = currentContentIndex - 1
			nextPartFound = true
			break
		}

		contentByteCount++
		currentContentIndex++
	}

	if nextPartFound == false {
		return nil, fmt.Errorf("end of serialized string not found")
	}

	return &details, nil
}

func getUnescapedBytesIfEscaped(charPair []byte) []byte {

	backslash := byte('\\')

	//escapables := []byte{'\\', '\'', '"', 'n', 'r', 't', 'b', 'f', '0'}

	// a map of the second byte to its actual binary presentation

	// if the first byte is not a backslash, we don't need to do anything

	if charPair[0] != backslash {
		return charPair
	}

	unescapedMap := map[byte]byte{
		'\\': '\\',
		'\'': '\'',
		'"':  '"',
		'n':  '\n',
		'r':  '\r',
		't':  '\t',
		'b':  '\b',
		'f':  '\f',
		'0':  '\x00',
	}

	actualByte := unescapedMap[charPair[1]]

	if actualByte != 0 {
		return []byte{actualByte}
	}

	// what if it's not a valid escape? Do nothing - it's considered as already escaped
	return charPair
}

func unescapeContent(escaped []byte) []byte {
	unescapedBytes := make([]byte, 0, len(escaped))
	index := 0

	// only applies to content - do not apply to raw mysql query
	// tested with php -i, mysql client, and mysqldump and mydumper.
	// 1. \" in dump becomes " when inserting a mysql row.
	// 2. \\ in dump becomes \ when inserting a mysql row.
	// 3. \' in dump becomes ' when inserting a mysql row.
	// 4. mysql translates newline into \n when creating a mysqldump. Same applies to carriage return.
	// 5. PHP serialize does not convert the bytes \r or \n into something else - they're as-is.
	// 6. If using single quotes in php, \r and \n does not get converted into bytes - they become literal backslash and letter.
	// Generally, to unescape, we need to do the following:
	// 1. Convert \\ to \
	// 2. Convert \' to '
	// 3. Convert \" to "

	backslash := byte('\\')

	for index < len(escaped) {

		if escaped[index] == backslash {
			unescapedBytePair := getUnescapedBytesIfEscaped(escaped[index : index+2])
			byteLength := len(unescapedBytePair)

			if byteLength == 1 {
				unescapedBytes = append(unescapedBytes, unescapedBytePair...)
				index = index + 2
				continue
			}
		}

		unescapedBytes = append(unescapedBytes, escaped[index])
		index++
	}

	return unescapedBytes
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

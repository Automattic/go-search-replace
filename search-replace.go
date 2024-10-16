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

	version = "0.0.7-dev"
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

type SerializedReplaceResult struct {
	Pre               []byte
	SerializedPortion []byte
	Post              []byte
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

func fixLine(line *[]byte, replacements []*Replacement) *[]byte {
	linePart := *line

	var rebuiltLine []byte

	for len(linePart) > 0 {
		result, err := fixLineWithSerializedData(linePart, replacements)
		if err != nil {
			rebuiltLine = append(rebuiltLine, linePart...)
			break
		}
		rebuiltLine = append(rebuiltLine, result.Pre...)
		rebuiltLine = append(rebuiltLine, result.SerializedPortion...)
		linePart = result.Post
	}

	*line = rebuiltLine

	return line
}

func replaceByPart(part []byte, replacements []*Replacement) []byte {
	for _, replacement := range replacements {
		part = bytes.ReplaceAll(part, replacement.From, replacement.To)
	}
	return part
}

var serializedStringPrefixRegexp = regexp.MustCompile(`s:(\d+):\\"`)

func fixLineWithSerializedData(linePart []byte, replacements []*Replacement) (*SerializedReplaceResult, error) {

	// find starting point in the line
	// We're not checking if we found the serialized string prefix inside a quote or not.
	// Currently skipping that scenario because it seems unlikely to find it outside.
	match := serializedStringPrefixRegexp.FindSubmatchIndex(linePart)
	if match == nil {
		return &SerializedReplaceResult{
			Pre:               replaceByPart(linePart, replacements),
			SerializedPortion: []byte{},
			Post:              []byte{},
		}, nil
	}

	pre := append([]byte{}, linePart[:match[0]]...)

	pre = replaceByPart(pre, replacements)

	if pre == nil {
		pre = []byte{}
	}

	originalBytes := linePart[match[2]:match[3]]

	originalByteSize, _ := strconv.Atoi(string(originalBytes))

	// the following assumes escaped double quotes
	// i.e. s:5:\"x -> we'll need to shift our index from '5' to 'x' - hence shifting by 3
	// MySQL can optionally not escape the double quote,
	// but generally sqldumps always include the quotes.
	contentStartIndex := match[3] + 3

	currentContentIndex := contentStartIndex

	contentByteCount := 0

	contentEndIndex := 0

	var nextSliceIndex int

	backslash := byte('\\')
	semicolon := byte(';')
	quote := byte('"')
	nextSliceFound := false

	maxIndex := len(linePart) - 1

	// let's find where the content actually ends.
	// it should end when the unescaped value is `";`
	for currentContentIndex < len(linePart) {
		if currentContentIndex+2 > maxIndex {

			// this algorithm SHOULD work, but in cases where the original byte count does not match
			// the actual byte count, it'll error out. We'll add this safeguard here.
			return nil, fmt.Errorf("faulty serialized data: out-of-bound index access detected")
		}
		char := linePart[currentContentIndex]
		secondChar := linePart[currentContentIndex+1]
		thirdChar := linePart[currentContentIndex+2]
		if char == backslash && contentByteCount < originalByteSize {
			unescapedBytePair := getUnescapedBytesIfEscaped(linePart[currentContentIndex : currentContentIndex+2])
			// if we get the byte pair without the backslash, it corresponds to a byte
			contentByteCount += len(unescapedBytePair)

			// content index count remains the same.
			currentContentIndex += 2
			continue
		}

		if char == backslash && secondChar == quote && thirdChar == semicolon && contentByteCount >= originalByteSize {

			// we're at backslash

			// index of the beginning of the next slice
			nextSliceIndex = currentContentIndex + 3
			// we're at backslash, so we need to minus 1 to get the index where the content finishes
			contentEndIndex = currentContentIndex - 1
			nextSliceFound = true
			break
		}

		if contentByteCount > originalByteSize {
			return nil, fmt.Errorf("faulty serialized data: calculated byte count does not match given data size")
		}

		contentByteCount++
		currentContentIndex++
	}

	content := append([]byte{}, linePart[contentStartIndex:contentEndIndex+1]...)

	content = replaceByPart(content, replacements)

	contentLength := len(unescapeContent(content))

	// and we rebuild the string
	rebuiltSerializedString := "s:" + strconv.Itoa(contentLength) + ":\\\"" + string(content) + "\\\";"

	if nextSliceFound == false {
		return nil, fmt.Errorf("faulty serialized data: end of serialized data not found")
	}

	result := SerializedReplaceResult{
		Pre:               pre,
		SerializedPortion: []byte(rebuiltSerializedString),
		Post:              linePart[nextSliceIndex:],
	}

	return &result, nil
}

func getUnescapedBytesIfEscaped(charPair []byte) []byte {

	backslash := byte('\\')

	// if the first byte is not a backslash, we don't need to do anything - we'll return the bytes
	// as per the function name, we'll return both bytes, or return one byte if one byte is actually an escape character
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

	// only applies to content of a string - do not apply to raw mysql query
	// tested with php -i, mysql client, and mysqldump and mydumper.
	// 1. mysql translates certain bytes to `\<char>` i.e. `\n`. So these needs unescaping to get the correct byte length. See `getUnescapedBytesIfEscaped`
	// 2. PHP serialize does not convert raw bytes into `\<char>` - they're as-is, so we don't need to take into account of escaped value in byte length calculation.

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

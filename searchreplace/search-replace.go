package searchreplace

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

const (
	searchRe  = `s:\d+:\\\".*?\\\";`
	replaceRe = `s:\d+:\\\"(.*?)\\\";`
)

var (
	search  = regexp.MustCompile(searchRe)
	replace = regexp.MustCompile(replaceRe)
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

func FixLine(line *[]byte, replacements []*Replacement) *[]byte {
	linePart := *line

	var rebuiltLine []byte

	for len(linePart) > 0 {
		result, err := fixLineWithSerializedData(linePart, replacements)
		if err != nil {
			malformedPart, post := recoverFromSerializedParseError(linePart, replacements)
			rebuiltLine = append(rebuiltLine, malformedPart...)
			linePart = post
			continue
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

var serializedStringTerminator = []byte(`\";`)

func recoverFromSerializedParseError(linePart []byte, replacements []*Replacement) ([]byte, []byte) {
	match := serializedStringPrefixRegexp.FindSubmatchIndex(linePart)
	if match == nil {
		return replaceByPart(linePart, replacements), []byte{}
	}

	serializedStart := match[0]
	contentStart := match[1]
	resumeIndex := malformedSerializedResumeIndex(linePart, serializedStart, contentStart)

	rebuiltPart := replaceByPart(linePart[:serializedStart], replacements)
	rebuiltPart = append(rebuiltPart, linePart[serializedStart:resumeIndex]...)

	return rebuiltPart, linePart[resumeIndex:]
}

func malformedSerializedResumeIndex(linePart []byte, serializedStart int, contentStart int) int {
	searchStart := contentStart
	if searchStart < serializedStart+1 {
		searchStart = serializedStart + 1
	}
	if searchStart > len(linePart) {
		return len(linePart)
	}

	resumeIndex := len(linePart)

	if terminatorIndex := bytes.Index(linePart[searchStart:], serializedStringTerminator); terminatorIndex >= 0 {
		resumeIndex = searchStart + terminatorIndex + len(serializedStringTerminator)
	}

	if match := serializedStringPrefixRegexp.FindIndex(linePart[searchStart:]); match != nil {
		prefixIndex := searchStart + match[0]
		if prefixIndex < resumeIndex {
			resumeIndex = prefixIndex
		}
	}

	return resumeIndex
}

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

	originalByteSize, err := strconv.Atoi(string(originalBytes))
	if err != nil {
		return nil, fmt.Errorf("faulty serialized data: invalid declared byte count: %w", err)
	}

	contentStartIndex := match[1]

	currentContentIndex := contentStartIndex

	contentByteCount := 0

	contentEndIndex := 0

	var nextSliceIndex int

	backslash := byte('\\')
	semicolon := byte(';')
	quote := byte('"')
	nextSliceFound := false

	// let's find where the content actually ends.
	// it should end when the unescaped value is `";`
	for currentContentIndex < len(linePart) {
		char := linePart[currentContentIndex]
		if char == backslash && contentByteCount < originalByteSize {
			if currentContentIndex+1 >= len(linePart) {
				return nil, fmt.Errorf("faulty serialized data: incomplete escaped byte pair")
			}

			unescapedBytePair := getUnescapedBytesIfEscaped(linePart[currentContentIndex : currentContentIndex+2])
			// if we get the byte pair without the backslash, it corresponds to a byte
			contentByteCount += len(unescapedBytePair)

			// content index count remains the same.
			currentContentIndex += 2
			continue
		}

		if char == backslash && contentByteCount >= originalByteSize {
			if currentContentIndex+2 >= len(linePart) {
				return nil, fmt.Errorf("faulty serialized data: incomplete serialized data terminator")
			}

			secondChar := linePart[currentContentIndex+1]
			thirdChar := linePart[currentContentIndex+2]

			if secondChar == quote && thirdChar == semicolon {

				// we're at backslash

				// index of the beginning of the next slice
				nextSliceIndex = currentContentIndex + 3
				// we're at backslash, so we need to minus 1 to get the index where the content finishes
				contentEndIndex = currentContentIndex - 1
				nextSliceFound = true
				break
			}
		}

		if contentByteCount > originalByteSize {
			return nil, fmt.Errorf("faulty serialized data: calculated byte count does not match given data size")
		}

		contentByteCount++
		currentContentIndex++
	}

	if nextSliceFound == false {
		return nil, fmt.Errorf("faulty serialized data: end of serialized data not found")
	}

	content := append([]byte{}, linePart[contentStartIndex:contentEndIndex+1]...)

	content = replaceByPart(content, replacements)

	contentLength := len(unescapeContent(content))

	// and we rebuild the string
	rebuiltSerializedString := "s:" + strconv.Itoa(contentLength) + ":\\\"" + string(content) + "\\\";"

	result := SerializedReplaceResult{
		Pre:               pre,
		SerializedPortion: []byte(rebuiltSerializedString),
		Post:              linePart[nextSliceIndex:],
	}

	return &result, nil
}

func getUnescapedBytesIfEscaped(charPair []byte) []byte {

	backslash := byte('\\')

	if len(charPair) < 2 {
		return charPair
	}

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
		// This should actually be '\x00' instead of '0', but golang do strange things
		// like terminating length calculations early, if we put a NULL terminator in an array
		// likely because it thought that the NULL terminator is the end of an array in the memory.
		//
		// This is a workaround and doesn't match the function's name, but right now the function
		// is only being used measuring how many bytes there are, if we unescape the escaped byte representation.
		// Hence, this is a safe workaround for now.
		'0': '0',
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

		if escaped[index] == backslash && index+1 < len(escaped) {
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

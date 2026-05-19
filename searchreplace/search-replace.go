package searchreplace

import (
	"bytes"
	"strconv"
)

type serializedQuoteStyle int

type serializedLengthMode int

const (
	rawSerializedQuote serializedQuoteStyle = iota
	escapedSerializedQuote
)

const (
	sqlSerializedLength serializedLengthMode = iota
	literalSerializedLength
)

type serializedStringToken struct {
	end          int
	contentStart int
	contentEnd   int
	quoteStyle   serializedQuoteStyle
	lengthMode   serializedLengthMode
}

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
	originalLine := *line
	rebuiltLine := make([]byte, 0, len(originalLine))
	searchStart := 0

	for searchStart < len(originalLine) {
		serializedStart := findSerializedStringCandidate(originalLine, searchStart)
		if serializedStart == -1 {
			rebuiltLine = append(rebuiltLine, replaceByPart(originalLine[searchStart:], replacements)...)
			break
		}

		rebuiltLine = append(rebuiltLine, replaceByPart(originalLine[searchStart:serializedStart], replacements)...)

		serializedString, failureEnd, ok := parseSerializedStringAt(originalLine, serializedStart)
		if !ok {
			if failureEnd <= serializedStart {
				failureEnd = serializedStart + 1
			}
			if failureEnd > len(originalLine) {
				failureEnd = len(originalLine)
			}

			rebuiltLine = append(rebuiltLine, originalLine[serializedStart:failureEnd]...)
			searchStart = failureEnd
			continue
		}

		rebuiltLine = appendReplacedSerializedString(rebuiltLine, originalLine, serializedString, replacements)
		searchStart = serializedString.end
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

func malformedSerializedFailureEnd(data []byte, serializedStart int, serializedString serializedStringToken) int {
	searchStart := serializedString.contentStart
	if searchStart < serializedStart+1 {
		searchStart = serializedStart + 1
	}
	if searchStart > len(data) {
		return len(data)
	}

	failureEnd := bestEffortSerializedEnd(data, serializedString)
	if nextSerializedStart := findSerializedStringCandidate(data, searchStart); nextSerializedStart != -1 && nextSerializedStart < failureEnd {
		failureEnd = nextSerializedStart
	}

	return failureEnd
}

func getUnescapedBytesIfEscaped(charPair []byte) []byte {
	if len(charPair) < 2 || charPair[0] != '\\' {
		return charPair
	}

	actualByte, ok := sqlEscapedByte(charPair[1])
	if ok {
		return []byte{actualByte}
	}

	return charPair
}

func unescapeContent(escaped []byte) []byte {
	unescapedBytes := make([]byte, 0, len(escaped))
	index := 0

	for index < len(escaped) {
		if len(escaped[index:]) >= 2 && escaped[index] == '\\' {
			actualByte, ok := sqlEscapedByte(escaped[index+1])
			if ok {
				unescapedBytes = append(unescapedBytes, actualByte)
				index += 2
				continue
			}
		}

		unescapedBytes = append(unescapedBytes, escaped[index])
		index++
	}

	return unescapedBytes
}

func replaceAndFix(line *[]byte, replacements []*Replacement) *[]byte {
	return FixLine(line, replacements)
}

func fix(match *[]byte) []byte {
	serializedStart := findSerializedStringCandidate(*match, 0)
	if serializedStart != 0 {
		return *match
	}

	serializedString, _, ok := parseSerializedStringAt(*match, 0)
	if ok && serializedString.end == len(*match) {
		return appendReplacedSerializedString(nil, *match, serializedString, nil)
	}

	serializedString, ok = parseSerializedStringByDelimiter(*match)
	if !ok {
		return *match
	}

	return appendReplacedSerializedString(nil, *match, serializedString, nil)
}

func findSerializedStringCandidate(data []byte, offset int) int {
	for candidateStart := offset; candidateStart+3 < len(data); candidateStart++ {
		if data[candidateStart] != 's' || data[candidateStart+1] != ':' {
			continue
		}

		lengthStart := candidateStart + 2
		if !isDigit(data[lengthStart]) {
			continue
		}

		lengthEnd := lengthStart
		for lengthEnd < len(data) && isDigit(data[lengthEnd]) {
			lengthEnd++
		}

		if lengthEnd >= len(data) || data[lengthEnd] != ':' {
			continue
		}

		quoteStart := lengthEnd + 1
		if quoteStart >= len(data) {
			continue
		}

		if data[quoteStart] == '"' {
			return candidateStart
		}

		if quoteStart+1 < len(data) && data[quoteStart] == '\\' && data[quoteStart+1] == '"' {
			return candidateStart
		}
	}

	return -1
}

func parseSerializedStringAt(data []byte, start int) (serializedStringToken, int, bool) {
	serializedString := serializedStringToken{}
	lengthStart := start + 2
	lengthEnd := lengthStart

	for lengthEnd < len(data) && isDigit(data[lengthEnd]) {
		lengthEnd++
	}

	if lengthStart == lengthEnd || lengthEnd >= len(data) || data[lengthEnd] != ':' {
		return serializedString, start + 1, false
	}

	quoteStart := lengthEnd + 1
	if quoteStart >= len(data) {
		return serializedString, start + 1, false
	}

	if data[quoteStart] == '"' {
		serializedString.quoteStyle = rawSerializedQuote
		serializedString.contentStart = quoteStart + 1
	} else if quoteStart+1 < len(data) && data[quoteStart] == '\\' && data[quoteStart+1] == '"' {
		serializedString.quoteStyle = escapedSerializedQuote
		serializedString.contentStart = quoteStart + 2
	} else {
		return serializedString, start + 1, false
	}

	declaredLength, err := strconv.Atoi(string(data[lengthStart:lengthEnd]))
	if err != nil {
		return serializedString, malformedSerializedFailureEnd(data, start, serializedString), false
	}

	if serializedString.quoteStyle == escapedSerializedQuote {
		serializedString.lengthMode = sqlSerializedLength
		parsedString, _, ok := parseSerializedStringContent(data, serializedString, declaredLength)
		if ok {
			return parsedString, parsedString.end, true
		}

		return parsedString, malformedSerializedFailureEnd(data, start, serializedString), false
	}

	serializedString.lengthMode = sqlSerializedLength
	parsedString, _, ok := parseSerializedStringContent(data, serializedString, declaredLength)
	if ok {
		return parsedString, parsedString.end, true
	}

	serializedString.lengthMode = literalSerializedLength
	parsedString, _, ok = parseSerializedStringContent(data, serializedString, declaredLength)
	if ok {
		return parsedString, parsedString.end, true
	}

	return parsedString, malformedSerializedFailureEnd(data, start, serializedString), false
}

func parseSerializedStringContent(data []byte, serializedString serializedStringToken, declaredLength int) (serializedStringToken, int, bool) {
	contentIndex := serializedString.contentStart
	contentLength := 0
	for contentLength < declaredLength {
		if contentIndex >= len(data) {
			return serializedString, bestEffortSerializedEnd(data, serializedString), false
		}

		contentIndex += encodedByteLength(data[contentIndex:], serializedString.lengthMode)
		contentLength++
	}

	serializedString.contentEnd = contentIndex
	if serializedCloseMatches(data, serializedString) {
		serializedString.end = contentIndex + serializedCloseLength(serializedString.quoteStyle)
		return serializedString, serializedString.end, true
	}

	return serializedString, bestEffortSerializedEnd(data, serializedString), false
}

func parseSerializedStringByDelimiter(data []byte) (serializedStringToken, bool) {
	serializedString := serializedStringToken{}
	lengthStart := 2
	lengthEnd := lengthStart

	for lengthEnd < len(data) && isDigit(data[lengthEnd]) {
		lengthEnd++
	}

	if lengthStart == lengthEnd || lengthEnd >= len(data) || data[lengthEnd] != ':' {
		return serializedString, false
	}

	quoteStart := lengthEnd + 1
	if quoteStart >= len(data) {
		return serializedString, false
	}

	if data[quoteStart] == '"' {
		serializedString.quoteStyle = rawSerializedQuote
		serializedString.lengthMode = literalSerializedLength
		serializedString.contentStart = quoteStart + 1
	} else if quoteStart+1 < len(data) && data[quoteStart] == '\\' && data[quoteStart+1] == '"' {
		serializedString.quoteStyle = escapedSerializedQuote
		serializedString.lengthMode = sqlSerializedLength
		serializedString.contentStart = quoteStart + 2
	} else {
		return serializedString, false
	}

	serializedString.end = bestEffortSerializedEnd(data, serializedString)
	if serializedString.end != len(data) {
		return serializedString, false
	}

	serializedString.contentEnd = serializedString.end - serializedCloseLength(serializedString.quoteStyle)
	return serializedString, true
}

func appendReplacedSerializedString(rebuilt []byte, source []byte, serializedString serializedStringToken, replacements []*Replacement) []byte {
	content := replaceByPart(source[serializedString.contentStart:serializedString.contentEnd], replacements)

	rebuilt = append(rebuilt, 's', ':')
	rebuilt = strconv.AppendInt(rebuilt, int64(countEncodedBytes(content, serializedString.lengthMode)), 10)
	rebuilt = append(rebuilt, ':')

	if serializedString.quoteStyle == escapedSerializedQuote {
		rebuilt = append(rebuilt, '\\')
	}
	rebuilt = append(rebuilt, '"')
	rebuilt = append(rebuilt, content...)
	if serializedString.quoteStyle == escapedSerializedQuote {
		rebuilt = append(rebuilt, '\\')
	}
	rebuilt = append(rebuilt, '"', ';')

	return rebuilt
}

func serializedCloseMatches(data []byte, serializedString serializedStringToken) bool {
	contentEnd := serializedString.contentEnd
	if serializedString.quoteStyle == escapedSerializedQuote {
		return contentEnd+2 < len(data) && data[contentEnd] == '\\' && data[contentEnd+1] == '"' && data[contentEnd+2] == ';'
	}

	return contentEnd+1 < len(data) && data[contentEnd] == '"' && data[contentEnd+1] == ';'
}

func serializedCloseLength(quoteStyle serializedQuoteStyle) int {
	if quoteStyle == escapedSerializedQuote {
		return 3
	}

	return 2
}

func bestEffortSerializedEnd(data []byte, serializedString serializedStringToken) int {
	closeDelimiter := []byte{'"', ';'}
	if serializedString.quoteStyle == escapedSerializedQuote {
		closeDelimiter = []byte{'\\', '"', ';'}
	}

	closeStart := bytes.Index(data[serializedString.contentStart:], closeDelimiter)
	if closeStart == -1 {
		return len(data)
	}

	return serializedString.contentStart + closeStart + len(closeDelimiter)
}

func countEncodedBytes(content []byte, lengthMode serializedLengthMode) int {
	contentLength := 0
	for contentIndex := 0; contentIndex < len(content); {
		contentIndex += encodedByteLength(content[contentIndex:], lengthMode)
		contentLength++
	}

	return contentLength
}

func encodedByteLength(content []byte, lengthMode serializedLengthMode) int {
	if lengthMode == literalSerializedLength {
		return 1
	}

	return encodedSQLByteLength(content)
}

func encodedSQLByteLength(content []byte) int {
	if len(content) >= 2 && content[0] == '\\' && sqlEscapeRepresentsSingleByte(content[1]) {
		return 2
	}

	return 1
}

func sqlEscapeRepresentsSingleByte(value byte) bool {
	_, ok := sqlEscapedByte(value)
	return ok
}

func sqlEscapedByte(value byte) (byte, bool) {
	switch value {
	case '\\':
		return '\\', true
	case '\'':
		return '\'', true
	case '"':
		return '"', true
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case 't':
		return '\t', true
	case 'b':
		return '\b', true
	case 'f':
		return '\f', true
	case '0':
		return '0', true
	case 'Z':
		return 'Z', true
	default:
		return 0, false
	}
}

func isDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

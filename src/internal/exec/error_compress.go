package exec

import (
	"regexp"
	"strings"
)

// compressError reduces a Python/Deephaven traceback to the minimum needed to
// diagnose and fix the error: the user's file/line reference, the exception
// type+message, and the root cause. Everything else (Java traces, DH library
// frames, eval preamble, wrapper frames) is stripped.
//
// The full traceback is preserved when verbose is true.
// CompressError is the exported entry point for error compression.
func CompressError(errorText string, filename string, verbose bool) string {
	return compressError(errorText, filename, verbose)
}

func compressError(errorText string, filename string, verbose bool) string {
	if verbose || errorText == "" {
		return errorText
	}

	lines := strings.Split(strings.TrimRight(errorText, "\n"), "\n")

	// Split into traceback blocks (each starting with "Traceback (most recent call last):")
	blocks := splitTracebackBlocks(lines)
	if len(blocks) == 0 {
		return stripJavaTraces(errorText)
	}

	// Find the block with the best (most relevant) exception
	bestBlock := -1
	var bestExc exceptionInfo
	for i, block := range blocks {
		exc := extractException(block)
		if exc.priority > bestExc.priority {
			bestExc = exc
			bestBlock = i
		}
	}

	if bestBlock < 0 || bestExc.line == "" {
		return stripJavaTraces(errorText)
	}

	// Extract user frames from the best block only
	userFrames := extractUserFrames(blocks[bestBlock], filename)

	var result []string
	for _, f := range userFrames {
		result = append(result, f)
	}
	result = append(result, bestExc.line)
	if bestExc.causedBy != "" {
		result = append(result, bestExc.causedBy)
	}

	return strings.Join(result, "\n")
}

// splitTracebackBlocks splits lines into groups, each starting at a
// "Traceback (most recent call last):" line. Lines before the first
// Traceback header are discarded.
func splitTracebackBlocks(lines []string) [][]string {
	var blocks [][]string
	var current []string

	for _, line := range lines {
		if strings.TrimSpace(line) == "Traceback (most recent call last):" {
			if len(current) > 0 {
				blocks = append(blocks, current)
			}
			current = []string{line}
			continue
		}
		if current != nil {
			current = append(current, line)
		}
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}
	return blocks
}

var (
	javaTraceLine = regexp.MustCompile(`^\s+at [\w.$]+[\w.$()\s]*\([\w.]+:\d+\)`)
	// Matches "  File "<string>", line 5" or "  File "foo.py", line 5"
	fileFrameRe = regexp.MustCompile(`^\s+File "([^"]+)", line (\d+)`)
	// Matches Java fully-qualified exception: "pkg.ExcName: message"
	javaExcClass = regexp.MustCompile(`([\w.]*\.)(\w+(?:Exception|Error)): (.+)`)
)

// extractUserFrames finds frame references pointing to user code within a
// single traceback block. Strips DH library frames, wrapper frames, and the
// Traceback header. Returns lines like:
//
//	File "basic_error.py", line 3
func extractUserFrames(block []string, filename string) []string {
	var frames []string
	for _, line := range block {
		m := fileFrameRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		file := m[1]

		// Skip DH library frames
		if strings.Contains(file, "deephaven/") || strings.Contains(file, "dist-packages/") {
			continue
		}

		// Skip wrapper <string> frames (high line numbers from the generated wrapper)
		if file == "<string>" {
			lineNo := 0
			parseDigits(m[2], &lineNo)
			if lineNo > 15 {
				continue // wrapper frame
			}
			// User code frame in exec'd string — replace with filename
			if filename != "" {
				line = strings.Replace(line, `"<string>"`, `"`+filename+`"`, 1)
			}
		}

		// Strip ", in <module>" suffix — it's always <module> for user scripts
		line = strings.Replace(line, ", in <module>", "", 1)

		frames = append(frames, strings.TrimRight(line, " "))
	}

	return frames
}

type exceptionInfo struct {
	line     string // e.g. "deephaven.dherror.DHError: table update operation failed."
	causedBy string // e.g. "  Caused by: NoSuchColumnException: ..."
	priority int    // higher = more relevant
}

// extractException finds the exception line in a traceback block and returns
// classified info. Priority: DHError(10) > Python errors(3) > RuntimeError
// wrapping Java(5) > SyntaxError(1).
func extractException(block []string) exceptionInfo {
	var best exceptionInfo

	for _, line := range block {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		if !strings.Contains(line, "Error") && !strings.Contains(line, "Exception") {
			continue
		}
		if !strings.Contains(line, ":") {
			continue
		}

		info := classifyException(line)
		if info.priority > best.priority {
			best = info
		}
	}

	return best
}

func classifyException(line string) exceptionInfo {
	info := exceptionInfo{line: line, priority: 1}

	// SyntaxError — low priority (often from eval preamble, but still valid
	// when it's the only/best error)
	if strings.HasPrefix(line, "SyntaxError:") {
		info.priority = 1
		return info
	}

	// DHError — highest priority, extract root cause
	if strings.HasPrefix(line, "deephaven.dherror.DHError:") {
		info.priority = 10
		if idx := strings.Index(line, " : RuntimeError: "); idx > 0 {
			dhMsg := line[len("deephaven.dherror.DHError: "):idx]
			rest := line[idx+len(" : RuntimeError: "):]
			if m := javaExcClass.FindStringSubmatch("RuntimeError: " + rest); m != nil {
				info.line = "deephaven.dherror.DHError: " + dhMsg
				info.causedBy = "  Caused by: " + m[2] + ": " + m[3]
			}
		}
		return info
	}

	// Standalone RuntimeError with Java exception — simplify class name
	if strings.Contains(line, "RuntimeError: io.deephaven.") {
		info.priority = 2
		if m := javaExcClass.FindStringSubmatch(line); m != nil {
			info.line = m[2] + ": " + m[3]
		}
		return info
	}

	// Java exception directly (NoSuchColumnException, etc.)
	if m := javaExcClass.FindStringSubmatch(line); m != nil && strings.Contains(line, "io.deephaven.") {
		info.priority = 2
		info.line = m[2] + ": " + m[3]
		return info
	}

	// Other Python exception (NameError, TypeError, etc.)
	info.priority = 3
	return info
}

// parseDigits is a minimal helper to parse an int from a digit string.
func parseDigits(s string, v *int) {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	*v = n
}

// stripJavaTraces removes only Java stack trace lines, as a minimal fallback.
func stripJavaTraces(text string) string {
	lines := strings.Split(text, "\n")
	var result []string
	for _, line := range lines {
		if !javaTraceLine.MatchString(line) {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

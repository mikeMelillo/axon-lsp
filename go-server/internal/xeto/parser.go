package xeto

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/mikeMelillo/axon-lsp/go-server/internal/lexer"
)

func ParseFile(path string) map[string]ParsedFunction {
	content, err := os.ReadFile(path)
	if err != nil {
		return map[string]ParsedFunction{}
	}
	return ParseURIContent(fileURI(path), string(content))
}

func fileURI(path string) string {
	return "file://" + filepath.ToSlash(path)
}

type EmbeddedAxonRegion struct {
	Text      string
	StartLine int
	EndLine   int
}

type ParsedFunction struct {
	Name          string
	Doc           string
	ArgsStr       string
	Params        []string
	ParamTypes    map[string]string
	ParamDefaults map[string]string
	ReturnType    string
	URI           string
	StartLine     int
	StartChar     int
	EndLine       int
	EndChar       int
	Embedded      *EmbeddedAxonRegion
}

var funcDeclPattern = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*:\s*Func\b`)
var specDeclPattern = regexp.MustCompile(`^([A-Z][A-Za-z0-9_]*)\s*:`)

// ParseSpecNames returns top-level Xeto spec names declared in source.
func ParseSpecNames(content string) map[string]struct{} {
	names := map[string]struct{}{}
	for _, line := range strings.Split(lexer.MaskComments(content).Text, "\n") {
		if match := specDeclPattern.FindStringSubmatch(line); match != nil {
			names[match[1]] = struct{}{}
		}
	}
	return names
}

func ParseURIContent(uri, content string) map[string]ParsedFunction {
	masked := lexer.MaskComments(content)
	lines := strings.Split(masked.Text, "\n")
	originalLines := strings.Split(content, "\n")
	found := make(map[string]ParsedFunction)
	inFuncs := false
	funcsDepth := 0
	pendingDoc := []string{}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		originalLine := originalLines[i]
		trimmed := strings.TrimSpace(line)
		originalTrimmed := strings.TrimSpace(originalLine)
		if trimmed == "" && strings.HasPrefix(originalTrimmed, "//") {
			pendingDoc = append(pendingDoc, strings.TrimSpace(strings.TrimPrefix(originalTrimmed, "//")))
			continue
		}
		if trimmed == "" {
			if len(pendingDoc) > 0 {
				pendingDoc = append(pendingDoc, "")
			}
			continue
		}

		if !inFuncs {
			if strings.Contains(line, "+Funcs") {
				inFuncs = true
				funcsDepth = countBraceDelta(line)
				if funcsDepth <= 0 {
					funcsDepth = 1
				}
			}
			pendingDoc = nil
			continue
		}

		if match := funcDeclPattern.FindStringSubmatch(line); match != nil {
			name := match[1]
			nameChar := strings.Index(line, name)
			originalBlock, maskedBlock, endLine, endChar := captureFuncBlock(lines, originalLines, i)
			params, paramTypes, paramDefaults, argsStr, returnType, embedded := parseFuncSignature(originalBlock, maskedBlock, i)
			doc := strings.TrimSpace(strings.Join(cleanDocLines(pendingDoc), "\n"))
			found[name] = ParsedFunction{
				Name:          name,
				Doc:           doc,
				ArgsStr:       argsStr,
				Params:        params,
				ParamTypes:    paramTypes,
				ParamDefaults: paramDefaults,
				ReturnType:    returnType,
				URI:           uri,
				StartLine:     i,
				StartChar:     max(nameChar, 0),
				EndLine:       endLine,
				EndChar:       endChar,
				Embedded:      embedded,
			}
			pendingDoc = nil
			i = endLine
			funcsDepth += countBraceDelta(maskedBlock)
			continue
		}

		funcsDepth += countBraceDelta(line)
		if funcsDepth <= 0 {
			inFuncs = false
			funcsDepth = 0
		}
		pendingDoc = nil
	}

	return found
}

func FindEmbeddedAxonRegion(uri, content string, line, character int) (*ParsedFunction, *EmbeddedAxonRegion, bool) {
	parsed := ParseURIContent(uri, content)
	for _, fn := range parsed {
		if fn.Embedded == nil {
			continue
		}
		region := fn.Embedded
		if line < region.StartLine || line > region.EndLine {
			continue
		}
		if line == region.StartLine && character < 0 {
			continue
		}
		fnCopy := fn
		return &fnCopy, region, true
	}
	if region, ok := fallbackEmbeddedRegion(content, line); ok {
		return nil, region, true
	}
	return nil, nil, false
}

func fallbackEmbeddedRegion(content string, targetLine int) (*EmbeddedAxonRegion, bool) {
	lines := strings.Split(lexer.MaskComments(content).Text, "\n")
	originalLines := strings.Split(content, "\n")
	start := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "<axon:---") {
			start = i + 1
			continue
		}
		if start >= 0 && strings.Contains(trimmed, "--->") {
			end := i - 1
			if targetLine >= start && targetLine <= end {
				return &EmbeddedAxonRegion{Text: strings.Join(originalLines[start:i], "\n"), StartLine: start, EndLine: end}, true
			}
			start = -1
		}
	}
	return nil, false
}

func captureFuncBlock(lines, originalLines []string, start int) (string, string, int, int) {
	var originalBuilder strings.Builder
	var maskedBuilder strings.Builder
	for i := start; i < len(lines); i++ {
		line := lines[i]
		if i > start {
			originalBuilder.WriteByte('\n')
			maskedBuilder.WriteByte('\n')
		}
		originalBuilder.WriteString(originalLines[i])
		maskedBuilder.WriteString(line)
		structural := structuralText(maskedBuilder.String())
		depth := 0
		for end, c := range structural {
			if c == '{' {
				depth++
			} else if c == '}' {
				depth--
				if depth == 0 {
					lineStart := strings.LastIndex(structural[:end], "\n") + 1
					return originalBuilder.String()[:end+1], maskedBuilder.String()[:end+1], i, end - lineStart + 1
				}
			}
		}
	}
	last := len(lines) - 1
	return originalBuilder.String(), maskedBuilder.String(), last, len(lines[last])
}

func parseFuncSignature(originalBlock, maskedBlock string, blockStartLine int) ([]string, map[string]string, map[string]string, string, string, *EmbeddedAxonRegion) {
	structural := structuralText(maskedBlock)
	bodyStart := strings.Index(structural, "{")
	bodyEnd := strings.LastIndex(structural, "}")
	if bodyStart < 0 || bodyEnd <= bodyStart {
		return nil, nil, nil, "()", "", nil
	}
	maskedBody := maskedBlock[bodyStart+1 : bodyEnd]
	originalBody := originalBlock[bodyStart+1 : bodyEnd]
	bodyLine := blockStartLine + strings.Count(maskedBlock[:bodyStart+1], "\n")
	lines := strings.Split(maskedBody, "\n")
	originalLines := strings.Split(originalBody, "\n")
	inAxon := false
	params := []string{}
	paramTypes := map[string]string{}
	paramDefaults := map[string]string{}
	signatureLines := []string{}
	argLabels := []string{}
	returnType := ""
	bodyLines := []string{}
	bodyStartLine := -1
	bodyEndLine := -1
	for idx, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		originalLine := originalLines[idx]
		if line == "" {
			if inAxon {
				bodyLines = append(bodyLines, originalLine)
			}
			continue
		}
		if strings.HasPrefix(line, "<axon:---") {
			inAxon = true
			bodyStartLine = bodyLine + idx + 1
			continue
		}
		if inAxon {
			if strings.HasPrefix(line, "--->") {
				inAxon = false
				bodyEndLine = bodyLine + idx - 1
				continue
			}
			bodyLines = append(bodyLines, originalLine)
			continue
		}
		signatureLines = append(signatureLines, rawLine)
	}
	for _, token := range splitSignatureTokens(strings.Join(signatureLines, "\n")) {
		name, value, ok := parseSlot(token)
		if !ok {
			continue
		}
		typeName, metadata := splitSlotMetadata(value)
		// A scalar value following a type is Xeto's own val default, not
		// part of the type name. Only this slot's explicit value is used.
		var literalDefault string
		if quote := strings.IndexByte(typeName, '"'); quote >= 0 {
			literalDefault = strings.TrimSpace(typeName[quote:])
			typeName = strings.TrimSpace(typeName[:quote])
		}
		for _, meta := range splitSignatureTokens(metadata) {
			if key, text, ok := parseSlot(meta); ok && key == "val" && literalDefault == "" {
				literalDefault = text
			}
		}
		for _, meta := range splitSignatureTokens(metadata) {
			if strings.TrimSpace(meta) == "maybe" && !strings.HasSuffix(typeName, "?") {
				typeName += "?"
			}
		}
		if name == "returns" {
			returnType = typeName
			continue
		}
		params = append(params, name)
		paramTypes[name] = typeName
		label := name + ": " + typeName
		if decoded, err := strconv.Unquote(literalDefault); literalDefault != "" && err == nil {
			// Normalize only known scalar encodings. Other defaults retain
			// their typed Xeto spelling for display; none are evaluated.
			text := typeName + " " + literalDefault
			switch strings.TrimPrefix(typeName, "sys::") {
			case "Bool":
				if decoded == "true" || decoded == "false" {
					text = decoded
				}
			case "Str":
				text = strconv.Quote(decoded)
			case "Number", "Int", "Float":
				text = typeName + " " + literalDefault
				if scalarNumberDefault.MatchString(decoded) {
					text = decoded
				}
			}
			paramDefaults[name] = text
		}
		if metadata != "" {
			for _, meta := range splitSignatureTokens(metadata) {
				key, source, ok := parseSlot(meta)
				if ok && key == "axon" {
					if decoded, err := strconv.Unquote(source); err == nil {
						paramDefaults[name] = decoded
					}
				}
			}
		}
		if text, found := paramDefaults[name]; found {
			label += " = " + text
		}
		argLabels = append(argLabels, label)
	}
	argsStr := "()"
	if len(argLabels) > 0 {
		argsStr = "(" + strings.Join(argLabels, ", ") + ")"
	}
	var region *EmbeddedAxonRegion
	if bodyStartLine >= 0 {
		if bodyEndLine < bodyStartLine {
			bodyEndLine = bodyStartLine + len(bodyLines) - 1
		}
		region = &EmbeddedAxonRegion{Text: strings.Join(bodyLines, "\n"), StartLine: bodyStartLine, EndLine: bodyEndLine}
	}
	return params, paramTypes, paramDefaults, argsStr, returnType, region
}

func splitSignatureTokens(line string) []string {
	result := []string{}
	start, depth := 0, 0
	var quote byte
	escaped := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' || c == '`' {
			quote = c
			continue
		}
		switch c {
		case '<', '{', '[', '(':
			depth++
		case '>', '}', ']', ')':
			depth--
		case ',', '\n':
			if depth == 0 {
				if part := strings.TrimSpace(line[start:i]); part != "" {
					result = append(result, part)
				}
				start = i + 1
			}
		}
	}
	if part := strings.TrimSpace(line[start:]); part != "" {
		result = append(result, part)
	}
	return result
}

// Metadata is not part of the declared type, including for structured types.
func splitSlotMetadata(value string) (string, string) {
	depth := 0
	var quote byte
	escaped := false
	for i := 0; i < len(value); i++ {
		c := value[i]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' || c == '`' {
			quote = c
			continue
		}
		switch c {
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		case '<':
			if depth == 0 {
				// A scalar default may follow the metadata block.
				level := 1
				var quoted byte
				for j := i + 1; j < len(value); j++ {
					c := value[j]
					if quoted != 0 {
						if c == '\\' {
							j++
							continue
						}
						if c == quoted {
							quoted = 0
						}
						continue
					}
					if c == '"' || c == '\'' || c == '`' {
						quoted = c
						continue
					}
					if c == '<' {
						level++
					}
					if c == '>' {
						level--
					}
					if level == 0 {
						return strings.TrimSpace(value[:i] + " " + value[j+1:]), strings.TrimSpace(value[i+1 : j])
					}
				}
				return strings.TrimSpace(value[:i]), strings.TrimSpace(value[i+1:])
			}
		}
	}
	return strings.TrimSpace(value), ""
}

func parseSlot(token string) (string, string, bool) {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	name := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if !regexpSlotName.MatchString(name) || value == "" {
		return "", "", false
	}
	return name, value, true
}

var regexpSlotName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var scalarNumberDefault = regexp.MustCompile(`^-?[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

func cleanDocLines(lines []string) []string {
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" && len(cleaned) > 0 && cleaned[len(cleaned)-1] == "" {
			continue
		}
		cleaned = append(cleaned, line)
	}
	for len(cleaned) > 0 && cleaned[0] == "" {
		cleaned = cleaned[1:]
	}
	for len(cleaned) > 0 && cleaned[len(cleaned)-1] == "" {
		cleaned = cleaned[:len(cleaned)-1]
	}
	return cleaned
}

func countBraceDelta(content string) int {
	content = structuralText(content)
	return strings.Count(content, "{") - strings.Count(content, "}")
}

// Preserve offsets while hiding strings and metadata from structural brace scans.
func structuralText(content string) string {
	masked := []byte(content)
	var quote byte
	escaped, inline := false, false
	depth := 0
	for i := 0; i < len(content); i++ {
		c := content[i]
		hide := quote != 0 || depth > 0
		if inline {
			if quote != 0 {
				if escaped {
					escaped = false
				} else if c == '\\' {
					escaped = true
				} else if c == quote {
					quote = 0
				}
			} else if c == '"' || c == '\'' || c == '`' {
				quote = c
			} else if strings.HasPrefix(content[i:], "--->") {
				inline = false
			}
		} else if quote != 0 {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
		} else if c == '"' || c == '\'' || c == '`' {
			quote, hide = c, true
		} else if c == '<' {
			depth++
			hide = true
		} else if c == '>' && depth > 0 {
			depth--
		} else if depth > 0 && strings.HasPrefix(content[i:], "---") {
			inline = true
		}
		if hide && c != '\n' {
			masked[i] = ' '
		}
	}
	return string(masked)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

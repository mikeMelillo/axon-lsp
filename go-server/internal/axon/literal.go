package axon

import "strings"

// LiteralType classifies a literal without treating dates and times as numbers.
// TokenNumber includes temporal literals as well as numeric values with units.
func LiteralType(literal *LiteralExpr) (string, bool) {
	switch literal.Kind {
	case TokenNumber:
		value := literal.Value
		if end := scanDateOrTime(value, 0); end == len(value) && end > 0 {
			if strings.Contains(value, "T") {
				return "DateTime", true
			}
			if strings.Contains(value, ":") {
				return "Time", true
			}
			if len(value) == 10 {
				return "Date", true
			}
			return "DateSpan", true
		}
		return "Number", true
	case TokenReference:
		return "Ref", true
	case TokenString:
		if strings.HasPrefix(literal.Value, "`") {
			return "Uri", true
		}
		return "Str", true
	case TokenKeyword:
		switch literal.Value {
		case "true", "false":
			return "Bool", true
		case "null":
			return "None", true
		case "na", "m":
			return "Marker", true
		}
	}
	return "", false
}

func IsRefLiteral(value string) bool {
	if len(value) < 2 || value[0] != '@' {
		return false
	}
	for i := 1; i < len(value); i++ {
		if !IsRefChar(value[i]) {
			return false
		}
	}
	return true
}

func RefLiteralAt(line string, character int) bool {
	_, _, ok := RefLiteralRangeAt(line, character)
	return ok
}

func RefLiteralRangeAt(line string, character int) (int, int, bool) {
	if character < 0 || character > len(line) || len(line) == 0 {
		return 0, 0, false
	}
	scan := character
	if scan == len(line) {
		scan--
	}
	for start := scan; start >= 0; start-- {
		if line[start] != '@' {
			if !IsRefChar(line[start]) {
				break
			}
			continue
		}
		end := start + 1
		for end < len(line) && IsRefChar(line[end]) {
			end++
		}
		return start, end, end > start+1 && character <= end
	}
	return 0, 0, false
}

// IsRefChar reports whether ch is allowed in a Haystack Ref identifier.
// Keep this aligned with Haxall's xeto.Ref.isIdChar.
func IsRefChar(ch byte) bool {
	return ch == '_' || ch == '-' || ch == '.' || ch == ':' || ch == '~' ||
		ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9'
}

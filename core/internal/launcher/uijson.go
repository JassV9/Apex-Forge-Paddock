package launcher

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

var nameField = regexp.MustCompile(`"name"\s*:\s*"((?:[^"\\]|\\.)*)"`)

// readUIName returns the "name" of a ui_car.json or ui_track.json, or "" when the file is
// missing or has none. Mod files are often not valid JSON: a byte order mark, Windows-1252
// text, raw tabs and line breaks inside strings, trailing commas. They are repaired first, and
// when that is not enough the first "name" field is picked out by hand.
func readUIName(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := repairJSON(toUTF8(raw))
	var doc struct {
		Name string `json:"name"`
	}
	if json.Unmarshal([]byte(text), &doc) == nil {
		return strings.TrimSpace(doc.Name)
	}
	match := nameField.FindStringSubmatch(text)
	if match == nil {
		return ""
	}
	var name string
	if json.Unmarshal([]byte(`"`+match[1]+`"`), &name) != nil {
		return strings.TrimSpace(match[1])
	}
	return strings.TrimSpace(name)
}

// toUTF8 drops a byte order mark and reads text that is not UTF-8 as Latin-1, which is what
// most of those files are close to.
func toUTF8(raw []byte) string {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if utf8.Valid(raw) {
		return string(raw)
	}
	runes := make([]rune, len(raw))
	for i, b := range raw {
		runes[i] = rune(b)
	}
	return string(runes)
}

// repairJSON escapes control characters inside strings and drops commas before } or ].
func repairJSON(text string) string {
	var out strings.Builder
	inString, escaped := false, false
	for i, r := range text {
		switch {
		case inString && escaped:
			escaped = false
		case inString && r == '\\':
			escaped = true
		case r == '"':
			inString = !inString
		case inString && r < 0x20:
			out.WriteString(` `)
			continue
		case !inString && r == ',' && closesNext(text[i+1:]):
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// closesNext reports whether the next non-space character closes an object or array.
func closesNext(rest string) bool {
	trimmed := strings.TrimLeft(rest, " \t\r\n")
	return strings.HasPrefix(trimmed, "}") || strings.HasPrefix(trimmed, "]")
}

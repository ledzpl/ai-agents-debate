package commandutil

import (
	"strings"
	"unicode"
)

// Parse splits a command line into command and argument tail.
func Parse(line string, aliases map[string]string) (command string, arg string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", ""
	}

	splitAt := strings.IndexFunc(line, unicode.IsSpace)
	if splitAt == -1 {
		return normalize(line, aliases), ""
	}
	cmd := normalize(line[:splitAt], aliases)
	return cmd, strings.TrimSpace(line[splitAt+1:])
}

func normalize(cmd string, aliases map[string]string) string {
	if len(aliases) == 0 {
		return cmd
	}
	if normalized, ok := aliases[cmd]; ok {
		return normalized
	}
	return cmd
}

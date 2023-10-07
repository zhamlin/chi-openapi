package internal

import (
	"strconv"
	"strings"
)

// TrimString removes extra lines and new spaces from each
// line in the provided str
func TrimString(str string) string {
	strLines := strings.Split(str, "\n")
	for i, line := range strLines {
		line = strings.Trim(line, "\n")
		line = strings.TrimSpace(line)
		strLines[i] = line
	}
	str = strings.Join(strLines, "\n")
	return strings.Trim(str, "\n")
}

func BoolFromString(str string) (bool, error) {
	return strconv.ParseBool(str)
}

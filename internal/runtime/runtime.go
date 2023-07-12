package runtime

import (
	"fmt"
	"path"
	"runtime"
	"strings"
)

func GetCaller(skip int) string {
	// skip this func
	skip += 1

	maxChecks := skip + 5
	for skip < maxChecks {
		if _, file, line, ok := runtime.Caller(skip); ok {
			inPackge := strings.Contains(file, "chi-openapi")
			inLocalTestFile := inPackge && strings.Contains(file, "_test")
			if !inPackge || inLocalTestFile {
				trimmedFile := path.Base(file)
				return fmt.Sprintf("%s:%d", trimmedFile, line)
			}
			// TODO: add more information about the calling function
			// f := runtime.FuncForPC(p)
		}
		skip++
	}
	return ""
}

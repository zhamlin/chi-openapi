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
			inPackge := strings.Contains(file, "chi-openapi/pkg") ||
				strings.Contains(file, "chi-openapi/internal")
			inLocalTestFile := inPackge && strings.Contains(file, "_test")
			if !inPackge || inLocalTestFile {
				base, name := path.Split(file)
				trimmedBase := path.Base(base)
				return fmt.Sprintf("%s:%d", path.Join(trimmedBase, name), line)
			}
		}
		skip++
	}
	return ""
}

package windows

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

var (
	versionRe   = regexp.MustCompile(`(?i)^\[Version\][ ]*$`)
	classGUIDRe = regexp.MustCompile(`(?i)^ClassGuid[ ]*=[ ]*(.+)$`)
)

// ParseDriverClassGUID retrieves the ClassGUID which is needed for the Windows registry entries.
func ParseDriverClassGUID(driverName, infPath string) (classGUID string, err error) {
	file, err := os.Open(infPath)
	if err != nil {
		return "", fmt.Errorf("Failed to open driver %s inf %s: %w", driverName, infPath, err)
	}

	defer file.Close()

	classGUID = matchClassGUID(file)
	if classGUID == "" {
		return "", fmt.Errorf("Failed to parse driver %s classGuid %s", driverName, infPath)
	}

	return
}

func matchClassGUID(r io.Reader) (classGUID string) {
	versionFlag := false

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !versionFlag {
			if versionRe.MatchString(line) {
				versionFlag = true
			}

			continue
		}

		matches := classGUIDRe.FindStringSubmatch(line)
		if len(matches) > 1 {
			classGUID = strings.TrimSpace(matches[1])
			if classGUID != "" {
				return classGUID
			}
		}
	}

	return ""
}

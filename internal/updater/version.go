package updater

import (
	"strconv"
	"strings"
)

// CompareVersions compares two SemVer-like version strings (e.g. "0.2.0" vs "0.1.0-alpha").
// Returns 1 if v1 > v2, -1 if v1 < v2, and 0 if v1 == v2.
func CompareVersions(v1, v2 string) int {
	v1Clean := strings.TrimPrefix(strings.TrimSpace(v1), "v")
	v2Clean := strings.TrimPrefix(strings.TrimSpace(v2), "v")

	parts1, pre1 := splitVersionAndPre(v1Clean)
	parts2, pre2 := splitVersionAndPre(v2Clean)

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		num1 := 0
		if i < len(parts1) {
			num1, _ = strconv.Atoi(parts1[i])
		}
		num2 := 0
		if i < len(parts2) {
			num2, _ = strconv.Atoi(parts2[i])
		}

		if num1 > num2 {
			return 1
		}
		if num1 < num2 {
			return -1
		}
	}

	// If numeric segments match: release version (empty pre) > pre-release version
	if pre1 == "" && pre2 != "" {
		return 1
	}
	if pre1 != "" && pre2 == "" {
		return -1
	}
	if pre1 > pre2 {
		return 1
	}
	if pre1 < pre2 {
		return -1
	}

	return 0
}

func splitVersionAndPre(v string) ([]string, string) {
	idx := strings.Index(v, "-")
	if idx == -1 {
		return strings.Split(v, "."), ""
	}
	numPart := v[:idx]
	prePart := v[idx+1:]
	return strings.Split(numPart, "."), prePart
}

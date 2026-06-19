package service

import (
	"os/exec"
	"regexp"
)

var singboxVersionPattern = regexp.MustCompile(`v?(\d+\.\d+\.\d+(?:[-+][A-Za-z0-9.-]+)?)`)
var exactSingboxVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][A-Za-z0-9.-]+)?$`)

func isSingboxVersion(s string) bool {
	return exactSingboxVersionPattern.MatchString(s)
}

func readSingboxVersion(binaryPath string) string {
	output, err := exec.Command(binaryPath, "version").CombinedOutput()
	if err != nil {
		return ""
	}

	match := singboxVersionPattern.FindStringSubmatch(string(output))
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

package utils

import (
	"regexp"

	"github.com/AS203038/looking-glass/pkg/errs"
)

var (
	saneASPathRegex = regexp.MustCompile(`[0-9_]+\$?`)
)

func SanitizeASPathRegex(aspath string) (string, error) {
	if len(aspath) == 0 {
		return "", errs.ASPathEmpty
	}
	if len(aspath) > 30 {
		return "", errs.ASPathTooLong
	}
	if !saneASPathRegex.MatchString(aspath) {
		return "", errs.ASPathMalformed
	}
	if aspath[0] != '_' {
		aspath = "_" + aspath
	}
	if aspath[len(aspath)-1] != '_' && aspath[len(aspath)-1] != '$' {
		aspath = aspath + "$"
	}
	return aspath, nil
}

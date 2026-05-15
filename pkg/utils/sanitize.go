package utils

import (
	"regexp"

	"github.com/AS203038/looking-glass/pkg/errs"
)

// saneASPathRegex is the allow-list applied to AS-path regexes:
// ASCII digits, underscores, and an optional trailing "$" anchor.
var saneASPathRegex = regexp.MustCompile(`[0-9_]+\$?`)

// SanitizeASPathRegex validates and normalises an AS-path regex,
// adding leading and trailing word-boundary anchors when missing.
// Returns [errs.ASPathEmpty], [errs.ASPathTooLong], or
// [errs.ASPathMalformed] on invalid input.
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

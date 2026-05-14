package utils

import (
	"regexp"

	"github.com/AS203038/looking-glass/pkg/errs"
)

// saneASPathRegex is the allow-list applied to user-supplied
// AS-path regexes. The bracket class is intentionally tight: only
// ASCII digits and underscores are admitted, plus an optional
// trailing "$" anchor. Anything else (parentheses, alternation
// operators, repetition, character classes) is rejected outright so
// that AS-path filters cannot be weaponised for ReDoS or for
// shell-quoting tricks once interpolated into a vendor command.
var (
	saneASPathRegex = regexp.MustCompile(`[0-9_]+\$?`)
)

// SanitizeASPathRegex validates and normalises an operator-supplied
// AS-path regex before it is rendered into a vendor command.
//
// Rules:
//
//   - Length must be in (0, 30]. Anything longer is rejected with
//     [errs.ASPathTooLong]; empty input returns [errs.ASPathEmpty].
//   - Only ASCII digits, underscores, and an optional trailing "$"
//     anchor are permitted; violations return [errs.ASPathMalformed].
//   - A leading "_" boundary anchor is prepended when missing so
//     that "203038" does not also match "1203038".
//   - A trailing "_" or "$" boundary anchor is appended when
//     missing, with the same intent.
//
// The returned string is safe to interpolate verbatim into router
// commands that accept Cisco-style AS-path regular expressions.
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

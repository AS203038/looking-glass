package utils

import (
	"regexp"
	"strconv"

	"github.com/AS203038/looking-glass/pkg/errs"
)

// saneASPathRegex matches a non-empty run of ASCII digits and
// underscores, optionally terminated by a single literal "$".
var saneASPathRegex = regexp.MustCompile(`^[0-9_]+\$?$`)

// SanitizeASPathRegex validates an AS-path regex and returns it with
// a leading "_" and a trailing "_" or "$" anchor. Returns
// [errs.ASPathEmpty], [errs.ASPathTooLong], or [errs.ASPathMalformed]
// on invalid input.
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

// SanitizeBGPCommunity returns the canonical "ASN:VALUE" string for
// the RFC 1997 standard community (asn, value). Both fields must be
// in 0..65535. Returns [errs.CommunityMalformed] on any range failure.
func SanitizeBGPCommunity(asn, value int32) (string, error) {
	if asn < 0 || asn > 0xFFFF {
		return "", errs.CommunityMalformed
	}
	if value < 0 || value > 0xFFFF {
		return "", errs.CommunityMalformed
	}
	return strconv.FormatInt(int64(asn), 10) + ":" +
		strconv.FormatInt(int64(value), 10), nil
}

// SanitizeBGPLargeCommunity returns the canonical
// "GLOBAL:LOCAL1:LOCAL2" string for the RFC 8092 large community
// (global, local1, local2).
func SanitizeBGPLargeCommunity(global, local1, local2 uint32) string {
	return strconv.FormatUint(uint64(global), 10) + ":" +
		strconv.FormatUint(uint64(local1), 10) + ":" +
		strconv.FormatUint(uint64(local2), 10)
}

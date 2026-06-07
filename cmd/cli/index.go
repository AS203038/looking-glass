package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"
)

// LookingGlassIndex is the on-the-wire representation of public_index.yaml.
type LookingGlassIndex struct {
	LookingGlasses []LookingGlass `yaml:"index"`
}

// LookingGlass is one entry in the public index.
type LookingGlass struct {
	ASN  string `yaml:"asn"  json:"asn"`
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url"  json:"url"`
}

func getCacheDir() string {
	cdir, err := os.UserCacheDir()
	if err != nil {
		cdir = os.TempDir()
	}
	return filepath.Join(cdir, "looking-glass")
}

// fetchIndex GETs the public index URL configured by --index / LG_INDEX_URL.
// It caches the result in ~/.cache/looking-glass/public_index.yaml for 30 days.
func fetchIndex(ctx context.Context) (*LookingGlassIndex, error) {
	cdir := getCacheDir()
	cachePath := filepath.Join(cdir, "public_index.yaml")

	var cachedData []byte
	if !opts.Update {
		if st, err := os.Stat(cachePath); err == nil {
			if time.Since(st.ModTime()) < 30*24*time.Hour {
				cachedData, err = os.ReadFile(cachePath)
				if err == nil {
					verbosef("loaded index from cache (%s)", cachePath)
					var idx LookingGlassIndex
					if err := yaml.Unmarshal(cachedData, &idx); err == nil {
						return &idx, nil
					}
				}
			}
		}
	}

	verbosef("fetching index from %s", opts.IndexURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.IndexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build index request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// Fallback to cache on network error if available
		if fallback, err2 := os.ReadFile(cachePath); err2 == nil {
			verbosef("fetch failed (%v), falling back to stale cache", err)
			var idx LookingGlassIndex
			if yaml.Unmarshal(fallback, &idx) == nil {
				return &idx, nil
			}
		}
		return nil, fmt.Errorf("fetch index: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch index: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}

	var idx LookingGlassIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}

	if err := os.MkdirAll(cdir, 0755); err == nil {
		_ = os.WriteFile(cachePath, data, 0644)
	}

	return &idx, nil
}

// resolveInstance turns a user-supplied instance string into a LookingGlass.
// Accepts a full URL, public-index name, or ASN (with/without "AS" prefix).
func resolveInstance(ctx context.Context, arg string) (*LookingGlass, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return nil, fmt.Errorf("instance argument is required")
	}

	if u, err := url.Parse(arg); err == nil && u.Scheme != "" && u.Host != "" {
		return &LookingGlass{Name: arg, URL: arg}, nil
	}

	idxCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	idx, err := fetchIndex(idxCtx)
	if err != nil {
		return nil, err
	}

	needle := strings.ToLower(arg)
	needleASN := strings.TrimPrefix(needle, "as")
	for _, lg := range idx.LookingGlasses {
		lgName := strings.ToLower(lg.Name)
		lgASN := strings.ToLower(lg.ASN)
		if lgName == needle || lgASN == needle || lgASN == needleASN {
			return &lg, nil
		}
	}

	// Try fuzzy matching
	var closestName string
	bestDist := -1
	for _, lg := range idx.LookingGlasses {
		dName := levenshtein(needle, strings.ToLower(lg.Name))
		dASN := levenshtein(needle, strings.ToLower(lg.ASN))
		d := dName
		if dASN < d {
			d = dASN
		}
		if bestDist == -1 || d < bestDist {
			bestDist = d
			closestName = lg.Name
		}
	}

	if bestDist != -1 && bestDist <= 3 {
		return nil, fmt.Errorf("instance %q not found in index (did you mean %q?)", arg, closestName)
	}
	return nil, fmt.Errorf("instance %q not found in index (try `lg-cli instances` or pass a full URL)", arg)
}

func levenshtein(s, t string) int {
	if len(s) == 0 {
		return len(t)
	}
	if len(t) == 0 {
		return len(s)
	}

	d := make([][]int, len(s)+1)
	for i := range d {
		d[i] = make([]int, len(t)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}

	for j := 1; j <= len(t); j++ {
		for i := 1; i <= len(s); i++ {
			cost := 1
			if s[i-1] == t[j-1] {
				cost = 0
			}
			min := d[i-1][j] + 1
			if d[i][j-1]+1 < min {
				min = d[i][j-1] + 1
			}
			if d[i-1][j-1]+cost < min {
				min = d[i-1][j-1] + cost
			}
			d[i][j] = min
		}
	}
	return d[len(s)][len(t)]
}

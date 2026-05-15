package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
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

// fetchIndex GETs the public index URL configured by --index / LG_INDEX_URL.
func fetchIndex(ctx context.Context) (*LookingGlassIndex, error) {
	verbosef("fetching index from %s", opts.IndexURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.IndexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build index request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch index: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch index: HTTP %d", resp.StatusCode)
	}
	var idx LookingGlassIndex
	if err := yaml.NewDecoder(resp.Body).Decode(&idx); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
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
	return nil, fmt.Errorf("instance %q not found in index (try `lg-cli instances` or pass a full URL)", arg)
}

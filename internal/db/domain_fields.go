package db

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var metadataKeyRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// ParseLegacyTags converts stored tag text into a normalized tag slice.
// Commas, newlines, tabs, and repeated whitespace are treated as separators.
func ParseLegacyTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	replacer := strings.NewReplacer(",", " ", "\n", " ", "\r", " ", "\t", " ", ";", " ")
	raw = replacer.Replace(raw)
	parts := strings.Fields(raw)
	return NormalizeTags(parts)
}

// NormalizeTags trims, de-duplicates, and preserves stable order.
func NormalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}

	result := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, tag)
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// JoinTags returns the canonical comma-separated representation stored in legacy tag text fields.
func JoinTags(tags []string) string {
	tags = NormalizeTags(tags)
	if len(tags) == 0 {
		return ""
	}
	return strings.Join(tags, ", ")
}

// ValidateAndNormalizeMetadata normalizes metadata keys and values.
// Keys are lower-cased, trimmed, and must use [A-Za-z0-9._-].
func ValidateAndNormalizeMetadata(in map[string]string) (map[string]string, error) {
	if len(in) == 0 {
		return nil, nil
	}

	keys := make([]string, 0, len(in))
	normalized := make(map[string]string, len(in))
	for key, value := range in {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "" {
			continue
		}
		if !metadataKeyRE.MatchString(key) {
			return nil, fmt.Errorf("invalid metadata key %q: use letters, numbers, dot, dash, underscore", key)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := normalized[key]; !exists {
			keys = append(keys, key)
		}
		normalized[key] = value
	}

	if len(normalized) == 0 {
		return nil, nil
	}

	slices.Sort(keys)
	out := make(map[string]string, len(normalized))
	for _, key := range keys {
		out[key] = normalized[key]
	}
	return out, nil
}

// MetadataSearchText flattens metadata into stable text form for search/export.
func MetadataSearchText(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}

	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, metadata[key]))
	}
	return strings.Join(parts, "; ")
}

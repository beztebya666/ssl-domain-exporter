package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"ssl-domain-exporter/internal/config"
	"ssl-domain-exporter/internal/db"
)

type domainInput struct {
	Name    string
	HasName bool

	Tags        []string
	HasTags     bool
	Metadata    map[string]string
	HasMetadata bool

	CustomCAPEM    string
	HasCustomCAPEM bool
	CheckMode      string
	HasCheckMode   bool
	DNSServers     string
	HasDNSServers  bool

	Interval    int
	HasInterval bool
	Port        int
	HasPort     bool

	FolderID    *int64
	HasFolderID bool
	Enabled     bool
	HasEnabled  bool
}

type createDomainRequest struct {
	Name        string          `json:"name"`
	Domain      string          `json:"domain"`
	Tags        json.RawMessage `json:"tags"`
	Metadata    json.RawMessage `json:"metadata"`
	CustomCAPEM string          `json:"custom_ca_pem"`
	CheckMode   string          `json:"check_mode"`
	DNSServers  string          `json:"dns_servers"`
	Interval    int             `json:"check_interval"`
	Port        int             `json:"port"`
	FolderID    *int64          `json:"folder_id"`
	Enabled     *bool           `json:"enabled"`
}

type updateDomainRequest struct {
	Name        *string          `json:"name"`
	Domain      *string          `json:"domain"`
	Tags        *json.RawMessage `json:"tags"`
	Metadata    *json.RawMessage `json:"metadata"`
	CustomCAPEM *string          `json:"custom_ca_pem"`
	CheckMode   *string          `json:"check_mode"`
	DNSServers  *string          `json:"dns_servers"`
	Enabled     *bool            `json:"enabled"`
	Interval    *int             `json:"check_interval"`
	Port        *int             `json:"port"`
	FolderID    *int64           `json:"folder_id"`
}

type importDomainsRequest struct {
	Mode          string           `json:"mode"`
	DryRun        bool             `json:"dry_run"`
	TriggerChecks bool             `json:"trigger_checks"`
	Defaults      map[string]any   `json:"defaults"`
	Domains       []map[string]any `json:"domains"`
}

type importSummary struct {
	Total   int `json:"total"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

type importDomainResult struct {
	Index  int        `json:"index"`
	Name   string     `json:"name,omitempty"`
	Action string     `json:"action"`
	Error  string     `json:"error,omitempty"`
	Domain *db.Domain `json:"domain,omitempty"`
}

type importDomainsResponse struct {
	Mode    string               `json:"mode"`
	DryRun  bool                 `json:"dry_run"`
	Summary importSummary        `json:"summary"`
	Results []importDomainResult `json:"results"`
}

func buildCreateInput(req createDomainRequest, cfg *config.Config) (domainInput, error) {
	in := domainInput{
		Name:        normalizeDomain(firstNonEmpty(req.Name, req.Domain)),
		CustomCAPEM: strings.TrimSpace(req.CustomCAPEM),
		DNSServers:  strings.TrimSpace(req.DNSServers),
		Interval:    req.Interval,
		Port:        req.Port,
		FolderID:    sanitizeFolderID(req.FolderID),
	}
	if in.Name == "" {
		return domainInput{}, fmt.Errorf("name is required")
	}
	in.HasName = true

	tags, _, err := parseTagsJSON(req.Tags)
	if err != nil {
		return domainInput{}, err
	}
	in.Tags = tags
	in.HasTags = len(req.Tags) > 0

	metadata, _, err := parseMetadataJSON(req.Metadata)
	if err != nil {
		return domainInput{}, err
	}
	in.Metadata = metadata
	in.HasMetadata = len(req.Metadata) > 0

	if strings.TrimSpace(req.CheckMode) != "" {
		in.CheckMode = config.ValidateCheckMode(req.CheckMode)
	} else {
		in.CheckMode = config.ValidateCheckMode(cfg.Domains.DefaultCheckMode)
	}
	in.HasCheckMode = true
	if req.Enabled != nil {
		in.Enabled = *req.Enabled
		in.HasEnabled = true
	}

	return in, nil
}

func buildUpdateInput(req updateDomainRequest) (domainInput, error) {
	in := domainInput{}
	if req.Name != nil || req.Domain != nil {
		in.Name = normalizeDomain(firstNonEmpty(derefString(req.Name), derefString(req.Domain)))
		if in.Name == "" {
			return domainInput{}, fmt.Errorf("name is required")
		}
		in.HasName = true
	}

	if req.Tags != nil {
		tags, _, err := parseTagsJSON(*req.Tags)
		if err != nil {
			return domainInput{}, err
		}
		in.Tags = tags
		in.HasTags = true
	}

	if req.Metadata != nil {
		metadata, _, err := parseMetadataJSON(*req.Metadata)
		if err != nil {
			return domainInput{}, err
		}
		in.Metadata = metadata
		in.HasMetadata = true
	}

	if req.CustomCAPEM != nil {
		in.CustomCAPEM = strings.TrimSpace(*req.CustomCAPEM)
		in.HasCustomCAPEM = true
	}
	if req.CheckMode != nil {
		in.CheckMode = config.ValidateCheckMode(*req.CheckMode)
		in.HasCheckMode = true
	}
	if req.DNSServers != nil {
		in.DNSServers = strings.TrimSpace(*req.DNSServers)
		in.HasDNSServers = true
	}
	if req.Enabled != nil {
		in.Enabled = *req.Enabled
		in.HasEnabled = true
	}
	if req.Interval != nil {
		in.Interval = *req.Interval
		in.HasInterval = true
	}
	if req.Port != nil {
		in.Port = *req.Port
		in.HasPort = true
	}
	if req.FolderID != nil {
		in.FolderID = sanitizeFolderID(req.FolderID)
		in.HasFolderID = true
	}

	return in, nil
}

func parseImportRequest(req importDomainsRequest) (domainInput, []domainInput, string, error) {
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "create_only"
	}
	if mode != "create_only" && mode != "upsert" {
		return domainInput{}, nil, "", fmt.Errorf("mode must be create_only or upsert")
	}

	defaults, err := parseImportMap(req.Defaults, false)
	if err != nil {
		return domainInput{}, nil, "", fmt.Errorf("defaults: %w", err)
	}

	items := make([]domainInput, 0, len(req.Domains))
	for idx, raw := range req.Domains {
		item, err := parseImportMap(raw, true)
		if err != nil {
			return domainInput{}, nil, "", fmt.Errorf("domains[%d]: %w", idx, err)
		}
		if item.Name == "" {
			return domainInput{}, nil, "", fmt.Errorf("domains[%d]: name/domain is required", idx)
		}
		item = mergeImportDefaults(defaults, item)
		items = append(items, item)
	}

	return defaults, items, mode, nil
}

func parseImportMap(raw map[string]any, requireName bool) (domainInput, error) {
	var in domainInput
	extraMetadata := map[string]string{}

	for key, value := range raw {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		switch normalizedKey {
		case "name", "domain":
			s, err := asString(value)
			if err != nil {
				return domainInput{}, fmt.Errorf("%s: %w", key, err)
			}
			if s != "" {
				in.Name = normalizeDomain(s)
			}
		case "tags":
			tags, err := parseTagsAny(value)
			if err != nil {
				return domainInput{}, fmt.Errorf("tags: %w", err)
			}
			in.Tags = tags
			in.HasTags = true
		case "metadata":
			metadata, err := parseMetadataAny(value)
			if err != nil {
				return domainInput{}, fmt.Errorf("metadata: %w", err)
			}
			in.Metadata = metadata
			in.HasMetadata = true
		case "custom_ca_pem":
			s, err := asString(value)
			if err != nil {
				return domainInput{}, fmt.Errorf("custom_ca_pem: %w", err)
			}
			in.CustomCAPEM = strings.TrimSpace(s)
			in.HasCustomCAPEM = true
		case "check_mode":
			s, err := asString(value)
			if err != nil {
				return domainInput{}, fmt.Errorf("check_mode: %w", err)
			}
			in.CheckMode = config.ValidateCheckMode(s)
			in.HasCheckMode = true
		case "dns_servers":
			s, err := asString(value)
			if err != nil {
				return domainInput{}, fmt.Errorf("dns_servers: %w", err)
			}
			in.DNSServers = strings.TrimSpace(s)
			in.HasDNSServers = true
		case "check_interval":
			n, err := asInt(value)
			if err != nil {
				return domainInput{}, fmt.Errorf("check_interval: %w", err)
			}
			in.Interval = n
			in.HasInterval = true
		case "port":
			n, err := asInt(value)
			if err != nil {
				return domainInput{}, fmt.Errorf("port: %w", err)
			}
			in.Port = n
			in.HasPort = true
		case "folder_id":
			if value == nil {
				in.FolderID = nil
				in.HasFolderID = true
				continue
			}
			n, err := asInt64(value)
			if err != nil {
				return domainInput{}, fmt.Errorf("folder_id: %w", err)
			}
			in.FolderID = sanitizeFolderID(&n)
			in.HasFolderID = true
		case "enabled":
			b, err := asBool(value)
			if err != nil {
				return domainInput{}, fmt.Errorf("enabled: %w", err)
			}
			in.Enabled = b
			in.HasEnabled = true
		default:
			s, err := stringifyImportValue(value)
			if err != nil {
				return domainInput{}, fmt.Errorf("%s: %w", key, err)
			}
			extraMetadata[normalizedKey] = s
		}
	}

	if requireName && in.Name == "" {
		return domainInput{}, fmt.Errorf("name/domain is required")
	}
	if len(extraMetadata) > 0 {
		if in.Metadata == nil {
			in.Metadata = map[string]string{}
		}
		for key, value := range extraMetadata {
			if _, exists := in.Metadata[key]; !exists {
				in.Metadata[key] = value
			}
		}
		in.HasMetadata = true
	}

	return in, nil
}

func mergeImportDefaults(defaults, item domainInput) domainInput {
	out := item
	if !out.HasTags && defaults.HasTags {
		out.Tags = append([]string(nil), defaults.Tags...)
		out.HasTags = true
	} else if out.HasTags && defaults.HasTags {
		out.Tags = db.NormalizeTags(append(append([]string(nil), defaults.Tags...), out.Tags...))
	}

	if defaults.HasMetadata {
		merged := make(map[string]string, len(defaults.Metadata)+len(out.Metadata))
		for key, value := range defaults.Metadata {
			merged[key] = value
		}
		for key, value := range out.Metadata {
			merged[key] = value
		}
		if len(merged) > 0 {
			out.Metadata = merged
			out.HasMetadata = out.HasMetadata || defaults.HasMetadata
		}
	}

	if !out.HasCustomCAPEM && defaults.HasCustomCAPEM {
		out.CustomCAPEM = defaults.CustomCAPEM
		out.HasCustomCAPEM = true
	}
	if !out.HasCheckMode && defaults.HasCheckMode {
		out.CheckMode = defaults.CheckMode
		out.HasCheckMode = true
	}
	if !out.HasDNSServers && defaults.HasDNSServers {
		out.DNSServers = defaults.DNSServers
		out.HasDNSServers = true
	}
	if !out.HasInterval && defaults.HasInterval {
		out.Interval = defaults.Interval
		out.HasInterval = true
	}
	if !out.HasPort && defaults.HasPort {
		out.Port = defaults.Port
		out.HasPort = true
	}
	if !out.HasFolderID && defaults.HasFolderID {
		out.FolderID = defaults.FolderID
		out.HasFolderID = true
	}
	if !out.HasEnabled && defaults.HasEnabled {
		out.Enabled = defaults.Enabled
		out.HasEnabled = true
	}
	return out
}

func parseTagsJSON(raw json.RawMessage) ([]string, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	if string(raw) == "null" {
		return nil, true, nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return db.ParseLegacyTags(single), true, nil
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return db.NormalizeTags(list), true, nil
	}
	return nil, false, fmt.Errorf("tags must be a string, array of strings, or null")
}

func parseMetadataJSON(raw json.RawMessage) (map[string]string, bool, error) {
	if len(raw) == 0 {
		return nil, false, nil
	}
	if string(raw) == "null" {
		return nil, true, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false, fmt.Errorf("metadata must be an object or null")
	}
	metadata, err := parseMetadataAny(obj)
	if err != nil {
		return nil, false, err
	}
	return metadata, true, nil
}

func parseTagsAny(value any) ([]string, error) {
	switch v := value.(type) {
	case nil:
		return nil, nil
	case string:
		return db.ParseLegacyTags(v), nil
	case []string:
		return db.NormalizeTags(v), nil
	case []any:
		tags := make([]string, 0, len(v))
		for _, item := range v {
			s, err := asString(item)
			if err != nil {
				return nil, err
			}
			tags = append(tags, s)
		}
		return db.NormalizeTags(tags), nil
	default:
		return nil, fmt.Errorf("must be a string, array, or null")
	}
}

func parseMetadataAny(value any) (map[string]string, error) {
	if value == nil {
		return nil, nil
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be an object or null")
	}

	metadata := make(map[string]string, len(obj))
	for key, raw := range obj {
		strValue, err := stringifyImportValue(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		metadata[key] = strValue
	}
	return db.ValidateAndNormalizeMetadata(metadata)
}

func stringifyImportValue(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case string:
		return strings.TrimSpace(v), nil
	case bool:
		return strconv.FormatBool(v), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case json.Number:
		return v.String(), nil
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", v), nil
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v), nil
	case map[string]any, []any:
		buf, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(buf), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

func asString(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v), nil
	case json.Number:
		return v.String(), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("must be a string")
	}
}

func asInt(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case json.Number:
		n, err := v.Int64()
		return int(n), err
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		return n, err
	default:
		return 0, fmt.Errorf("must be a number")
	}
}

func asInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case json.Number:
		return v.Int64()
	case string:
		return strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	default:
		return 0, fmt.Errorf("must be a number")
	}
}

func asBool(value any) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		return config.ParseBool(v), nil
	default:
		return false, fmt.Errorf("must be a boolean")
	}
}

func sanitizeFolderID(folderID *int64) *int64 {
	if folderID == nil || *folderID <= 0 {
		return nil
	}
	return folderID
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

package db

import (
	"fmt"
	"sort"
	"strings"
)

const (
	DomainSourceManual           = "manual"
	DomainSourceKubernetesSecret = "kubernetes_" + "secret" //nolint:gosec // Inventory source identifier, not a credential.
	DomainSourceF5Certificate    = "f5_certificate"
)

func NormalizeSourceType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", DomainSourceManual:
		return DomainSourceManual
	case DomainSourceKubernetesSecret:
		return DomainSourceKubernetesSecret
	case DomainSourceF5Certificate:
		return DomainSourceF5Certificate
	default:
		return DomainSourceManual
	}
}

func ValidateAndNormalizeSourceRef(sourceType string, sourceRef map[string]string) (map[string]string, error) {
	sourceType = NormalizeSourceType(sourceType)
	if len(sourceRef) == 0 {
		sourceRef = map[string]string{}
	}

	normalized := make(map[string]string, len(sourceRef))
	for key, value := range sourceRef {
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		normalized[key] = value
	}

	switch sourceType {
	case DomainSourceManual:
		return map[string]string{}, nil
	case DomainSourceKubernetesSecret:
		if normalized["namespace"] == "" {
			return nil, fmt.Errorf("source_ref.namespace is required for kubernetes_secret sources")
		}
		if normalized["secret_name"] == "" {
			return nil, fmt.Errorf("source_ref.secret_name is required for kubernetes_secret sources")
		}
	case DomainSourceF5Certificate:
		if normalized["partition"] == "" {
			return nil, fmt.Errorf("source_ref.partition is required for f5_certificate sources")
		}
		if normalized["certificate_name"] == "" {
			return nil, fmt.Errorf("source_ref.certificate_name is required for f5_certificate sources")
		}
	default:
		return nil, fmt.Errorf("unsupported source_type %q", sourceType)
	}

	return normalized, nil
}

func SourceDisplayName(sourceType string, sourceRef map[string]string) string {
	sourceType = NormalizeSourceType(sourceType)
	switch sourceType {
	case DomainSourceKubernetesSecret:
		namespace := strings.TrimSpace(sourceRef["namespace"])
		secretName := strings.TrimSpace(sourceRef["secret_name"])
		if namespace != "" && secretName != "" {
			return fmt.Sprintf("k8s:%s/%s", namespace, secretName)
		}
	case DomainSourceF5Certificate:
		partition := strings.TrimSpace(sourceRef["partition"])
		certificateName := strings.TrimSpace(sourceRef["certificate_name"])
		if partition != "" && certificateName != "" {
			return fmt.Sprintf("f5:%s/%s", partition, certificateName)
		}
	}
	return ""
}

func CloneSourceRef(sourceRef map[string]string) map[string]string {
	if len(sourceRef) == 0 {
		return map[string]string{}
	}
	clone := make(map[string]string, len(sourceRef))
	for key, value := range sourceRef {
		clone[key] = value
	}
	return clone
}

func SourceRefSearchText(sourceRef map[string]string) string {
	if len(sourceRef) == 0 {
		return ""
	}
	keys := make([]string, 0, len(sourceRef))
	for key := range sourceRef {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, sourceRef[key]))
	}
	return strings.Join(parts, " ")
}

package validation

import (
	"fmt"
	"regexp"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	v1alpha1 "github.com/rockswe/K8s-PodConfigMapController/api/v1alpha1"
)

const (
	// MaxConfigMapNameLength is the maximum length for a ConfigMap name
	MaxConfigMapNameLength = 253
	// MaxConfigMapDataKeyLength is the maximum length for a ConfigMap data key
	MaxConfigMapDataKeyLength = 253
	// MaxConfigMapDataValueLength is the maximum length for a ConfigMap data value
	MaxConfigMapDataValueLength = 1048576 // 1MB
)

var (
	// configMapNameRegex validates ConfigMap names
	configMapNameRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
)

// ValidatePodConfigMapConfig validates a PodConfigMapConfig resource
func ValidatePodConfigMapConfig(pcmc *v1alpha1.PodConfigMapConfig) error {
	if pcmc == nil {
		return fmt.Errorf("PodConfigMapConfig cannot be nil")
	}

	// Validate metadata
	if err := validateMetadata(pcmc.Name, pcmc.Namespace); err != nil {
		return fmt.Errorf("invalid metadata: %w", err)
	}

	// Validate spec
	if err := validatePodConfigMapConfigSpec(&pcmc.Spec); err != nil {
		return fmt.Errorf("invalid spec: %w", err)
	}

	return nil
}

// validateMetadata validates the metadata fields
func validateMetadata(name, namespace string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	if namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}

	// Validate name format
	if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return fmt.Errorf("invalid name format: %v", errs)
	}

	if errs := validation.IsDNS1123Label(namespace); len(errs) > 0 {
		return fmt.Errorf("invalid namespace format: %v", errs)
	}

	return nil
}

// validatePodConfigMapConfigSpec validates the spec fields
func validatePodConfigMapConfigSpec(spec *v1alpha1.PodConfigMapConfigSpec) error {
	if spec == nil {
		return fmt.Errorf("spec cannot be nil")
	}

	// Validate labels to include
	for _, label := range spec.LabelsToInclude {
		if err := validateLabelKey(label); err != nil {
			return fmt.Errorf("invalid label key %q: %w", label, err)
		}
	}

	// Validate annotations to include
	for _, annotation := range spec.AnnotationsToInclude {
		if err := validateAnnotationKey(annotation); err != nil {
			return fmt.Errorf("invalid annotation key %q: %w", annotation, err)
		}
	}

	// Validate pod selector
	if spec.PodSelector != nil {
		if err := validateLabelSelector(spec.PodSelector.MatchLabels); err != nil {
			return fmt.Errorf("invalid pod selector: %w", err)
		}
	}

	return nil
}

// validateLabelKey validates a Kubernetes label key
func validateLabelKey(key string) error {
	if key == "" {
		return fmt.Errorf("label key cannot be empty")
	}

	if errs := validation.IsQualifiedName(key); len(errs) > 0 {
		return fmt.Errorf("invalid label key format: %v", errs)
	}

	return nil
}

// validateAnnotationKey validates a Kubernetes annotation key
func validateAnnotationKey(key string) error {
	if key == "" {
		return fmt.Errorf("annotation key cannot be empty")
	}

	if errs := validation.IsQualifiedName(key); len(errs) > 0 {
		return fmt.Errorf("invalid annotation key format: %v", errs)
	}

	return nil
}

// validateLabelSelector validates label selector match labels
func validateLabelSelector(matchLabels map[string]string) error {
	for key, value := range matchLabels {
		if err := validateLabelKey(key); err != nil {
			return fmt.Errorf("invalid selector key %q: %w", key, err)
		}

		if errs := validation.IsValidLabelValue(value); len(errs) > 0 {
			return fmt.Errorf("invalid selector value %q for key %q: %v", value, key, errs)
		}
	}

	return nil
}

// ValidateConfigMapName validates a generated ConfigMap name
func ValidateConfigMapName(name string) error {
	if name == "" {
		return fmt.Errorf("ConfigMap name cannot be empty")
	}

	if len(name) > MaxConfigMapNameLength {
		return fmt.Errorf("ConfigMap name too long: %d > %d", len(name), MaxConfigMapNameLength)
	}

	if !configMapNameRegex.MatchString(name) {
		return fmt.Errorf("invalid ConfigMap name format: %s", name)
	}

	return nil
}

// ValidateConfigMapData validates ConfigMap data
func ValidateConfigMapData(data map[string]string) error {
	if data == nil {
		return nil
	}

	for key, value := range data {
		if err := validateConfigMapDataKey(key); err != nil {
			return fmt.Errorf("invalid data key %q: %w", key, err)
		}

		if err := validateConfigMapDataValue(value); err != nil {
			return fmt.Errorf("invalid data value for key %q: %w", key, err)
		}
	}

	return nil
}

// validateConfigMapDataKey validates a ConfigMap data key
func validateConfigMapDataKey(key string) error {
	if key == "" {
		return fmt.Errorf("data key cannot be empty")
	}

	if len(key) > MaxConfigMapDataKeyLength {
		return fmt.Errorf("data key too long: %d > %d", len(key), MaxConfigMapDataKeyLength)
	}

	// ConfigMap data keys must be valid as environment variable names
	if !isValidConfigMapKey(key) {
		return fmt.Errorf("invalid data key format: %s", key)
	}

	return nil
}

// validateConfigMapDataValue validates a ConfigMap data value
func validateConfigMapDataValue(value string) error {
	if len(value) > MaxConfigMapDataValueLength {
		return fmt.Errorf("data value too long: %d > %d", len(value), MaxConfigMapDataValueLength)
	}

	return nil
}

// isValidConfigMapKey checks if a key is valid for ConfigMap data
func isValidConfigMapKey(key string) bool {
	// ConfigMap keys should be valid as environment variable names
	// Allow alphanumeric characters, underscores, and dots
	for _, char := range key {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_' || char == '.') {
			return false
		}
	}

	// Key cannot start with a digit
	if len(key) > 0 && key[0] >= '0' && key[0] <= '9' {
		return false
	}

	return true
}

// ValidatePod validates a Pod for ConfigMap generation
func ValidatePod(pod *v1.Pod) error {
	if pod == nil {
		return fmt.Errorf("pod cannot be nil")
	}

	if pod.Name == "" {
		return fmt.Errorf("pod name cannot be empty")
	}

	if pod.Namespace == "" {
		return fmt.Errorf("pod namespace cannot be empty")
	}

	// Validate pod is in a valid phase for ConfigMap generation
	if !isValidPodPhase(pod.Status.Phase) {
		return fmt.Errorf("invalid pod phase for ConfigMap generation: %s", pod.Status.Phase)
	}

	return nil
}

// isValidPodPhase checks if a pod phase is valid for ConfigMap generation
func isValidPodPhase(phase v1.PodPhase) bool {
	switch phase {
	case v1.PodPending, v1.PodRunning, v1.PodSucceeded, v1.PodFailed:
		return true
	default:
		return false
	}
}

// SanitizeConfigMapName sanitizes a ConfigMap name to ensure it's valid
func SanitizeConfigMapName(name string) string {
	// Convert to lowercase
	name = strings.ToLower(name)

	// Replace invalid characters with hyphens
	sanitized := make([]rune, 0, len(name))
	for _, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			sanitized = append(sanitized, char)
		} else {
			sanitized = append(sanitized, '-')
		}
	}

	result := string(sanitized)

	// Ensure it doesn't start or end with a hyphen
	result = strings.Trim(result, "-")

	// Ensure it's not empty
	if result == "" {
		result = "configmap"
	}

	// Truncate if too long
	if len(result) > MaxConfigMapNameLength {
		result = result[:MaxConfigMapNameLength]
		result = strings.TrimRight(result, "-")
	}

	return result
}

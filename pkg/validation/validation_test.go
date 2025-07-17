package validation

import (
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/rockswe/K8s-PodConfigMapController/api/v1alpha1"
)

func TestValidateConfigMapName(t *testing.T) {
	tests := []struct {
		name        string
		configName  string
		shouldError bool
	}{
		{
			name:        "valid name",
			configName:  "pod-test-pod-from-test-pcmc-cfg",
			shouldError: false,
		},
		{
			name:        "valid name with numbers",
			configName:  "pod-test-pod-123-from-test-pcmc-456-cfg",
			shouldError: false,
		},
		{
			name:        "empty name",
			configName:  "",
			shouldError: true,
		},
		{
			name:        "name with uppercase",
			configName:  "Pod-Test-Pod-From-Test-Pcmc-Cfg",
			shouldError: true,
		},
		{
			name:        "name starting with hyphen",
			configName:  "-pod-test-pod-from-test-pcmc-cfg",
			shouldError: true,
		},
		{
			name:        "name ending with hyphen",
			configName:  "pod-test-pod-from-test-pcmc-cfg-",
			shouldError: true,
		},
		{
			name:        "name with underscores",
			configName:  "pod_test_pod_from_test_pcmc_cfg",
			shouldError: true,
		},
		{
			name:        "name with dots",
			configName:  "pod.test.pod.from.test.pcmc.cfg",
			shouldError: true,
		},
		{
			name:        "name too long",
			configName:  strings.Repeat("a", MaxConfigMapNameLength+1),
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfigMapName(tt.configName)
			if tt.shouldError && err == nil {
				t.Errorf("ValidateConfigMapName() expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("ValidateConfigMapName() unexpected error: %v", err)
			}
		})
	}
}

func TestValidateConfigMapData(t *testing.T) {
	tests := []struct {
		name        string
		data        map[string]string
		shouldError bool
	}{
		{
			name: "valid data",
			data: map[string]string{
				"podName":             "test-pod",
				"namespace":           "default",
				"label_app":           "test-app",
				"annotation_build.id": "12345",
			},
			shouldError: false,
		},
		{
			name:        "nil data",
			data:        nil,
			shouldError: false,
		},
		{
			name:        "empty data",
			data:        map[string]string{},
			shouldError: false,
		},
		{
			name: "empty key",
			data: map[string]string{
				"": "value",
			},
			shouldError: true,
		},
		{
			name: "key too long",
			data: map[string]string{
				strings.Repeat("a", MaxConfigMapDataKeyLength+1): "value",
			},
			shouldError: true,
		},
		{
			name: "value too long",
			data: map[string]string{
				"key": strings.Repeat("a", MaxConfigMapDataValueLength+1),
			},
			shouldError: true,
		},
		{
			name: "key with invalid characters",
			data: map[string]string{
				"key-with-hyphens": "value",
			},
			shouldError: true,
		},
		{
			name: "key starting with number",
			data: map[string]string{
				"1key": "value",
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfigMapData(tt.data)
			if tt.shouldError && err == nil {
				t.Errorf("ValidateConfigMapData() expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("ValidateConfigMapData() unexpected error: %v", err)
			}
		})
	}
}

func TestValidatePod(t *testing.T) {
	tests := []struct {
		name        string
		pod         *v1.Pod
		shouldError bool
	}{
		{
			name: "valid pod",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
				},
			},
			shouldError: false,
		},
		{
			name:        "nil pod",
			pod:         nil,
			shouldError: true,
		},
		{
			name: "empty name",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "",
					Namespace: "default",
				},
			},
			shouldError: true,
		},
		{
			name: "empty namespace",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "",
				},
			},
			shouldError: true,
		},
		{
			name: "pending phase",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Status: v1.PodStatus{
					Phase: v1.PodPending,
				},
			},
			shouldError: false,
		},
		{
			name: "failed phase",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Status: v1.PodStatus{
					Phase: v1.PodFailed,
				},
			},
			shouldError: false,
		},
		{
			name: "unknown phase",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Status: v1.PodStatus{
					Phase: v1.PodPhase("unknown"),
				},
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePod(tt.pod)
			if tt.shouldError && err == nil {
				t.Errorf("ValidatePod() expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("ValidatePod() unexpected error: %v", err)
			}
		})
	}
}

func TestValidatePodConfigMapConfig(t *testing.T) {
	tests := []struct {
		name        string
		pcmc        *v1alpha1.PodConfigMapConfig
		shouldError bool
	}{
		{
			name: "valid PCMC",
			pcmc: &v1alpha1.PodConfigMapConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pcmc",
					Namespace: "default",
				},
				Spec: v1alpha1.PodConfigMapConfigSpec{
					LabelsToInclude:      []string{"app", "version"},
					AnnotationsToInclude: []string{"build.id", "commit.sha"},
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test-app",
						},
					},
				},
			},
			shouldError: false,
		},
		{
			name: "valid PCMC with no selector",
			pcmc: &v1alpha1.PodConfigMapConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pcmc",
					Namespace: "default",
				},
				Spec: v1alpha1.PodConfigMapConfigSpec{
					LabelsToInclude:      []string{"app"},
					AnnotationsToInclude: []string{"build.id"},
				},
			},
			shouldError: false,
		},
		{
			name:        "nil PCMC",
			pcmc:        nil,
			shouldError: true,
		},
		{
			name: "empty name",
			pcmc: &v1alpha1.PodConfigMapConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "",
					Namespace: "default",
				},
			},
			shouldError: true,
		},
		{
			name: "empty namespace",
			pcmc: &v1alpha1.PodConfigMapConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pcmc",
					Namespace: "",
				},
			},
			shouldError: true,
		},
		{
			name: "invalid label key",
			pcmc: &v1alpha1.PodConfigMapConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pcmc",
					Namespace: "default",
				},
				Spec: v1alpha1.PodConfigMapConfigSpec{
					LabelsToInclude: []string{""},
				},
			},
			shouldError: true,
		},
		{
			name: "invalid annotation key",
			pcmc: &v1alpha1.PodConfigMapConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pcmc",
					Namespace: "default",
				},
				Spec: v1alpha1.PodConfigMapConfigSpec{
					AnnotationsToInclude: []string{""},
				},
			},
			shouldError: true,
		},
		{
			name: "invalid selector value",
			pcmc: &v1alpha1.PodConfigMapConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pcmc",
					Namespace: "default",
				},
				Spec: v1alpha1.PodConfigMapConfigSpec{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": strings.Repeat("a", 64), // Too long
						},
					},
				},
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePodConfigMapConfig(tt.pcmc)
			if tt.shouldError && err == nil {
				t.Errorf("ValidatePodConfigMapConfig() expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("ValidatePodConfigMapConfig() unexpected error: %v", err)
			}
		})
	}
}

func TestSanitizeConfigMapName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid name",
			input:    "pod-test-pod-from-test-pcmc-cfg",
			expected: "pod-test-pod-from-test-pcmc-cfg",
		},
		{
			name:     "uppercase letters",
			input:    "Pod-Test-Pod-From-Test-Pcmc-Cfg",
			expected: "pod-test-pod-from-test-pcmc-cfg",
		},
		{
			name:     "mixed case with numbers",
			input:    "Pod-Test-Pod-123-From-Test-Pcmc-456-Cfg",
			expected: "pod-test-pod-123-from-test-pcmc-456-cfg",
		},
		{
			name:     "invalid characters",
			input:    "pod_test.pod@from#test$pcmc%cfg",
			expected: "pod-test-pod-from-test-pcmc-cfg",
		},
		{
			name:     "leading and trailing hyphens",
			input:    "-pod-test-pod-from-test-pcmc-cfg-",
			expected: "pod-test-pod-from-test-pcmc-cfg",
		},
		{
			name:     "multiple consecutive hyphens",
			input:    "pod--test--pod--from--test--pcmc--cfg",
			expected: "pod--test--pod--from--test--pcmc--cfg",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "configmap",
		},
		{
			name:     "only invalid characters",
			input:    "@#$%^&*()",
			expected: "configmap",
		},
		{
			name:     "only hyphens",
			input:    "---",
			expected: "configmap",
		},
		{
			name:     "very long name",
			input:    strings.Repeat("a", MaxConfigMapNameLength+100),
			expected: strings.Repeat("a", MaxConfigMapNameLength),
		},
		{
			name:     "long name ending with hyphen",
			input:    strings.Repeat("a", MaxConfigMapNameLength-1) + "-",
			expected: strings.Repeat("a", MaxConfigMapNameLength-1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeConfigMapName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeConfigMapName() = %v, want %v", result, tt.expected)
			}

			// Verify the result is valid
			if err := ValidateConfigMapName(result); err != nil {
				t.Errorf("SanitizeConfigMapName() produced invalid name: %v", err)
			}
		})
	}
}

func TestValidateLabelKey(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		shouldError bool
	}{
		{
			name:        "valid simple key",
			key:         "app",
			shouldError: false,
		},
		{
			name:        "valid key with domain",
			key:         "example.com/app",
			shouldError: false,
		},
		{
			name:        "valid key with hyphens",
			key:         "my-app-name",
			shouldError: false,
		},
		{
			name:        "empty key",
			key:         "",
			shouldError: true,
		},
		{
			name:        "key too long",
			key:         strings.Repeat("a", 300),
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLabelKey(tt.key)
			if tt.shouldError && err == nil {
				t.Errorf("validateLabelKey() expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("validateLabelKey() unexpected error: %v", err)
			}
		})
	}
}

func TestValidateAnnotationKey(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		shouldError bool
	}{
		{
			name:        "valid simple key",
			key:         "build.id",
			shouldError: false,
		},
		{
			name:        "valid key with domain",
			key:         "example.com/build.id",
			shouldError: false,
		},
		{
			name:        "valid key with hyphens and underscores",
			key:         "my-build_id",
			shouldError: false,
		},
		{
			name:        "empty key",
			key:         "",
			shouldError: true,
		},
		{
			name:        "key too long",
			key:         strings.Repeat("a", 300),
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAnnotationKey(tt.key)
			if tt.shouldError && err == nil {
				t.Errorf("validateAnnotationKey() expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("validateAnnotationKey() unexpected error: %v", err)
			}
		})
	}
}

func TestIsValidConfigMapKey(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected bool
	}{
		{
			name:     "valid alphanumeric key",
			key:      "podName",
			expected: true,
		},
		{
			name:     "valid key with underscores",
			key:      "pod_name",
			expected: true,
		},
		{
			name:     "valid key with dots",
			key:      "label.app",
			expected: true,
		},
		{
			name:     "valid mixed key",
			key:      "label_app.version",
			expected: true,
		},
		{
			name:     "invalid key with hyphens",
			key:      "pod-name",
			expected: false,
		},
		{
			name:     "invalid key starting with number",
			key:      "1podName",
			expected: false,
		},
		{
			name:     "invalid key with special characters",
			key:      "pod@name",
			expected: false,
		},
		{
			name:     "empty key",
			key:      "",
			expected: true, // Empty key passes character validation but fails elsewhere
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidConfigMapKey(tt.key)
			if result != tt.expected {
				t.Errorf("isValidConfigMapKey() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Benchmark tests
func BenchmarkValidateConfigMapName(b *testing.B) {
	name := "pod-test-pod-from-test-pcmc-cfg"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ValidateConfigMapName(name)
	}
}

func BenchmarkValidateConfigMapData(b *testing.B) {
	data := map[string]string{
		"podName":             "test-pod",
		"namespace":           "default",
		"label_app":           "test-app",
		"annotation_build.id": "12345",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ValidateConfigMapData(data)
	}
}

func BenchmarkSanitizeConfigMapName(b *testing.B) {
	name := "Pod_Test.Pod@From#Test$Pcmc%Cfg"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		SanitizeConfigMapName(name)
	}
}

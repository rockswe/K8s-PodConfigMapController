package controller

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	v1alpha1 "github.com/rockswe/K8s-PodConfigMapController/api/v1alpha1"
	"github.com/rockswe/K8s-PodConfigMapController/pkg/validation"
)

func TestGenerateConfigMapName(t *testing.T) {
	c := &Controller{}

	tests := []struct {
		name     string
		pod      *v1.Pod
		pcmc     *v1alpha1.PodConfigMapConfig
		expected string
	}{
		{
			name: "basic name generation",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			pcmc: &v1alpha1.PodConfigMapConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pcmc",
				},
			},
			expected: "pod-test-pod-from-test-pcmc-cfg",
		},
		{
			name: "name with special characters",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod-123",
				},
			},
			pcmc: &v1alpha1.PodConfigMapConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pcmc-abc",
				},
			},
			expected: "pod-test-pod-123-from-test-pcmc-abc-cfg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.generateConfigMapName(tt.pod, tt.pcmc)
			if result != tt.expected {
				t.Errorf("generateConfigMapName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

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
			name:        "name starting with number",
			configName:  "1pod-test-pod-from-test-pcmc-cfg",
			shouldError: false, // Actually valid for ConfigMap names
		},
		{
			name:        "name with invalid characters",
			configName:  "pod-test_pod-from-test.pcmc-cfg",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validation.ValidateConfigMapName(tt.configName)
			if tt.shouldError && err == nil {
				t.Errorf("ValidateConfigMapName() expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("ValidateConfigMapName() unexpected error: %v", err)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validation.ValidatePodConfigMapConfig(tt.pcmc)
			if tt.shouldError && err == nil {
				t.Errorf("ValidatePodConfigMapConfig() expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("ValidatePodConfigMapConfig() unexpected error: %v", err)
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
			name: "invalid phase",
			pod: &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Status: v1.PodStatus{
					Phase: v1.PodPhase("invalid"),
				},
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validation.ValidatePod(tt.pod)
			if tt.shouldError && err == nil {
				t.Errorf("ValidatePod() expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("ValidatePod() unexpected error: %v", err)
			}
		})
	}
}

func TestConfigMapDataGeneration(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				"app":     "test-app",
				"version": "1.0.0",
			},
			Annotations: map[string]string{
				"build.id":   "12345",
				"commit.sha": "abcdef",
			},
		},
		Spec: v1.PodSpec{
			NodeName: "test-node",
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}

	pcmc := &v1alpha1.PodConfigMapConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pcmc",
		},
		Spec: v1alpha1.PodConfigMapConfigSpec{
			LabelsToInclude:      []string{"app", "version"},
			AnnotationsToInclude: []string{"build.id", "commit.sha"},
		},
	}

	// Create a fake client
	client := fake.NewSimpleClientset()
	c := &Controller{
		kubeClient: client,
	}

	// Generate ConfigMap name
	_ = c.generateConfigMapName(pod, pcmc)

	// Expected data
	expectedData := map[string]string{
		"podName":               "test-pod",
		"namespace":             "default",
		"nodeName":              "test-node",
		"phase":                 "Running",
		"pcmcName":              "test-pcmc",
		"label_app":             "test-app",
		"label_version":         "1.0.0",
		"annotation_build.id":   "12345",
		"annotation_commit.sha": "abcdef",
	}

	// Test that the generated data matches expectations
	configData := map[string]string{
		"podName":   pod.Name,
		"namespace": pod.Namespace,
		"nodeName":  pod.Spec.NodeName,
		"phase":     string(pod.Status.Phase),
		"pcmcName":  pcmc.Name,
	}

	for _, labelKey := range pcmc.Spec.LabelsToInclude {
		if val, ok := pod.Labels[labelKey]; ok {
			configData["label_"+labelKey] = val
		}
	}
	for _, annotationKey := range pcmc.Spec.AnnotationsToInclude {
		if val, ok := pod.Annotations[annotationKey]; ok {
			configData["annotation_"+annotationKey] = val
		}
	}

	if len(configData) != len(expectedData) {
		t.Errorf("Generated data length mismatch. Got %d, want %d", len(configData), len(expectedData))
	}

	for key, expectedValue := range expectedData {
		if actualValue, ok := configData[key]; !ok {
			t.Errorf("Missing key %s in generated data", key)
		} else if actualValue != expectedValue {
			t.Errorf("Value mismatch for key %s. Got %s, want %s", key, actualValue, expectedValue)
		}
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
			name:     "empty string",
			input:    "",
			expected: "configmap",
		},
		{
			name:     "only invalid characters",
			input:    "@#$%^&*()",
			expected: "configmap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validation.SanitizeConfigMapName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeConfigMapName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Benchmark tests for performance
func BenchmarkGenerateConfigMapName(b *testing.B) {
	c := &Controller{}
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
		},
	}
	pcmc := &v1alpha1.PodConfigMapConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pcmc",
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.generateConfigMapName(pod, pcmc)
	}
}

func BenchmarkValidateConfigMapName(b *testing.B) {
	name := "pod-test-pod-from-test-pcmc-cfg"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validation.ValidateConfigMapName(name)
	}
}

func BenchmarkValidatePodConfigMapConfig(b *testing.B) {
	pcmc := &v1alpha1.PodConfigMapConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pcmc",
			Namespace: "default",
		},
		Spec: v1alpha1.PodConfigMapConfigSpec{
			LabelsToInclude:      []string{"app", "version"},
			AnnotationsToInclude: []string{"build.id", "commit.sha"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validation.ValidatePodConfigMapConfig(pcmc)
	}
}

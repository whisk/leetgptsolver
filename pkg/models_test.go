package leetgptsolver

import (
	"reflect"
	"testing"
)

func TestParseModel(t *testing.T) {
	tests := []struct {
		input          string
		expectedModel  string
		expectedParams string
		expectError    bool
	}{
		{
			input:          "model-name",
			expectedModel:  "model-name",
			expectedParams: "",
			expectError:    false,
		},
		{
			input:          `model-name@{"key1":"value1","key2":42,"key3":3.14}`,
			expectedModel:  "model-name",
			expectedParams: `{"key1":"value1","key2":42,"key3":3.14}`,
			expectError:    false,
		},
		{
			input:          "",
			expectedModel:  "",
			expectedParams: "",
			expectError:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			model, params, err := ParseModelName(test.input)

			if (err != nil) != test.expectError {
				t.Errorf("expected error: %v, got: %v", test.expectError, err)
			}

			if model != test.expectedModel {
				t.Errorf("expected model: %s, got: %s", test.expectedModel, model)
			}

			if !reflect.DeepEqual(params, test.expectedParams) {
				t.Errorf("expected params: %#v, got: %#v", test.expectedParams, params)
			}
		})
	}
}

func TestGuessModelVendor(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		expected int
	}{
		{name: "openai gpt", model: "gpt-5-mini", expected: MODEL_VENDOR_OPENAI},
		{name: "openai o3", model: "o3-mini", expected: MODEL_VENDOR_OPENAI},
		{name: "google gemini", model: "gemini-2.5-pro", expected: MODEL_VENDOR_GOOGLE},
		{name: "anthropic claude", model: "claude-sonnet-4-5", expected: MODEL_VENDOR_ANTHROPIC},
		{name: "deepseek", model: "deepseek-reasoner", expected: MODEL_VENDOR_DEEPSEEK},
		{name: "xai grok", model: "grok-3-latest", expected: MODEL_VENDOR_XAI},
		{name: "unknown", model: "my-custom-model", expected: MODEL_VENDOR_UNKNOWN},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := GuessModelVendor(test.model); got != test.expected {
				t.Errorf("expected vendor: %d, got: %d", test.expected, got)
			}
		})
	}
}

func TestParseModelVendor(t *testing.T) {
	tests := []struct {
		name        string
		vendor      string
		expected    int
		expectError bool
	}{
		{name: "empty", vendor: "", expected: MODEL_VENDOR_UNKNOWN, expectError: false},
		{name: "openai", vendor: "openai", expected: MODEL_VENDOR_OPENAI, expectError: false},
		{name: "vertex alias", vendor: "vertexai", expected: MODEL_VENDOR_GOOGLE, expectError: false},
		{name: "anthropic alias", vendor: "claude", expected: MODEL_VENDOR_ANTHROPIC, expectError: false},
		{name: "unknown", vendor: "other", expected: MODEL_VENDOR_UNKNOWN, expectError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseModelVendor(test.vendor)
			if (err != nil) != test.expectError {
				t.Errorf("expected error: %v, got: %v", test.expectError, err)
			}
			if got != test.expected {
				t.Errorf("expected vendor: %d, got: %d", test.expected, got)
			}
		})
	}
}

func TestResolveModelVendor(t *testing.T) {
	modelVendor, err := ResolveModelVendor("my-custom-model", "")
	if err == nil {
		t.Fatal("expected error for unknown model without vendor")
	}
	if modelVendor != MODEL_VENDOR_UNKNOWN {
		t.Fatalf("expected unknown model vendor, got %d", modelVendor)
	}

	modelVendor, err = ResolveModelVendor("my-custom-model", "openai")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if modelVendor != MODEL_VENDOR_OPENAI {
		t.Fatalf("expected openai model vendor, got %d", modelVendor)
	}
}

package leetgptsolver

import (
	"reflect"
	"testing"
)

func TestParseModel(t *testing.T) {
	tests := []struct {
		input          string
		expectedModel  string
		expectedParams map[string]any
		expectError    bool
	}{
		{
			input:         "model-name",
			expectedModel: "model-name",
			expectedParams: map[string]any{},
			expectError:   false,
		},
		{
			input:         "model-name:key1=value1;key2=42;key3=3.14",
			expectedModel: "model-name",
			expectedParams: map[string]any{
				"key1": "value1",
				"key2": 42,
				"key3": float32(3.14),
			},
			expectError: false,
		},
		{
			input:         "model-name:key1=value1;key2=3x",
			expectedModel: "model-name",
			expectedParams: map[string]any{
				"key1": "value1",
				"key2": "3x",
			},
			expectError: false,
		},
		{
			input:         "model-name:key1=value1;key2",
			expectedModel: "model-name",
			expectedParams: map[string]any{
				"key1": "value1",
			},
			expectError: false,
		},
		{
			input:         "",
			expectedModel: "",
			expectedParams: map[string]any{},
			expectError:   false,
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
				t.Errorf("expected params: %v, got: %v", test.expectedParams, params)
			}
		})
	}
}

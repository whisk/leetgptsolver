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

package shared

import (
	"strings"
	"testing"
)

func TestStringSlice_Value(t *testing.T) {
	tests := []struct {
		name     string
		slice    StringSlice
		expected string
	}{
		{
			name:     "empty slice",
			slice:    StringSlice{},
			expected: "[]",
		},
		{
			name:     "nil slice",
			slice:    nil,
			expected: "[]",
		},
		{
			name:     "single item",
			slice:    StringSlice{"item1"},
			expected: `["item1"]`,
		},
		{
			name:     "multiple items",
			slice:    StringSlice{"item1", "item2", "item3"},
			expected: `["item1","item2","item3"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.slice.Value()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			str, ok := result.([]byte)
			if !ok {
				s, ok := result.(string)
				if !ok {
					t.Fatalf("expected string or []byte, got %T", result)
				}
				str = []byte(s)
			}
			if string(str) != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, string(str))
			}
		})
	}
}

func TestStringSlice_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected StringSlice
		wantErr  bool
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: nil,
		},
		{
			name:     "byte slice",
			input:    []byte(`["a","b","c"]`),
			expected: StringSlice{"a", "b", "c"},
		},
		{
			name:     "string",
			input:    `["x","y"]`,
			expected: StringSlice{"x", "y"},
		},
		{
			name:     "empty array",
			input:    "[]",
			expected: StringSlice{},
		},
		{
			name:    "invalid type",
			input:   123,
			wantErr: true,
		},
		{
			name:    "invalid json",
			input:   "not json",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s StringSlice
			err := s.Scan(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(s) != len(tt.expected) {
				t.Fatalf("expected len %d, got %d", len(tt.expected), len(s))
			}
			for i := range s {
				if s[i] != tt.expected[i] {
					t.Errorf("index %d: expected %s, got %s", i, tt.expected[i], s[i])
				}
			}
		})
	}
}

func TestNewID(t *testing.T) {
	tests := []struct {
		prefix string
	}{
		{prefix: "user_"},
		{prefix: "agent_"},
		{prefix: "key_"},
		{prefix: ""},
	}

	for _, tt := range tests {
		t.Run("prefix_"+tt.prefix, func(t *testing.T) {
			id := NewID(tt.prefix)
			if !strings.HasPrefix(id, tt.prefix) {
				t.Errorf("expected ID to start with '%s', got '%s'", tt.prefix, id)
			}
			expectedLen := len(tt.prefix) + 32
			if len(id) != expectedLen {
				t.Errorf("expected length %d, got %d", expectedLen, len(id))
			}
		})
	}

	id1 := NewID("test_")
	id2 := NewID("test_")
	if id1 == id2 {
		t.Error("expected unique IDs, got duplicates")
	}
}

func TestScope_String(t *testing.T) {
	tests := []struct {
		scope    Scope
		expected string
	}{
		{ScopeProfile, "profile"},
		{ScopeEmail, "email"},
		{ScopeRealtime, "realtime"},
		{ScopeHistory, "history"},
		{ScopePreferences, "preferences"},
	}

	for _, tt := range tests {
		t.Run(string(tt.scope), func(t *testing.T) {
			if tt.scope.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, tt.scope.String())
			}
		})
	}
}

func TestAgentCategory(t *testing.T) {
	categories := []AgentCategory{
		AgentCategoryProductivity,
		AgentCategoryEducation,
		AgentCategoryCreative,
		AgentCategoryDeveloper,
		AgentCategoryAssistant,
		AgentCategoryResearch,
		AgentCategoryEntertainment,
		AgentCategoryHealth,
		AgentCategoryFinance,
	}

	for _, cat := range categories {
		if string(cat) == "" {
			t.Errorf("category should not be empty")
		}
	}
}

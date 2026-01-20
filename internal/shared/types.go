package shared

import (
	"crypto/rand"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	if len(s) == 0 {
		return "[]", nil
	}
	return json.Marshal(s)
}

func (s *StringSlice) Scan(value any) error {
	if value == nil {
		*s = nil
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into StringSlice", value)
	}

	return json.Unmarshal(bytes, s)
}

func NewID(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return prefix + hex.EncodeToString(b)
}

type Scope string

const (
	ScopeProfile     Scope = "profile"
	ScopeEmail       Scope = "email"
	ScopeRealtime    Scope = "realtime"
	ScopeHistory     Scope = "history"
	ScopePreferences Scope = "preferences"
	ScopeVision      Scope = "vision"
)

func (s Scope) String() string {
	return string(s)
}

type AgentCategory string

const (
	AgentCategoryProductivity  AgentCategory = "productivity"
	AgentCategoryEducation     AgentCategory = "education"
	AgentCategoryCreative      AgentCategory = "creative"
	AgentCategoryDeveloper     AgentCategory = "developer"
	AgentCategoryAssistant     AgentCategory = "assistant"
	AgentCategoryResearch      AgentCategory = "research"
	AgentCategoryEntertainment AgentCategory = "entertainment"
	AgentCategoryHealth        AgentCategory = "health"
	AgentCategoryFinance       AgentCategory = "finance"
)

// Package flagmint provides the Go SDK for the Flagmint feature flag service.
package flagmint

import (
	"encoding/json"
	"errors"
)

// ErrFlagNotFound is returned by [FeatureFlags.JSON] when the requested key
// does not exist in the flag set.
var ErrFlagNotFound = errors.New("flagmint: flag not found")

// FlagType enumerates the supported flag value types.
type FlagType string

const (
	FlagTypeBoolean FlagType = "boolean"
	FlagTypeString  FlagType = "string"
	FlagTypeNumber  FlagType = "number"
	FlagTypeJSON    FlagType = "json"
)

// FeatureFlags holds the evaluated feature flags returned by the server.
// Use the type-safe getter methods (Bool, String, Float64, JSON) to retrieve
// individual flag values. The zero value is valid and returns fallbacks for all
// keys.
type FeatureFlags struct {
	values map[string]any
}

// NewFeatureFlags wraps the raw server payload in a type-safe [FeatureFlags].
func NewFeatureFlags(values map[string]any) FeatureFlags {
	return FeatureFlags{values: values}
}

// Has reports whether a flag with the given key exists in the set.
func (f FeatureFlags) Has(key string) bool {
	if f.values == nil {
		return false
	}
	_, ok := f.values[key]
	return ok
}

// Len returns the number of flags in the set.
func (f FeatureFlags) Len() int {
	if f.values == nil {
		return 0
	}
	return len(f.values)
}

// Bool retrieves a boolean flag value.
// Returns fallback when the key is absent or the stored value is not a bool.
func (f FeatureFlags) Bool(key string, fallback bool) bool {
	if f.values == nil {
		return fallback
	}
	val, ok := f.values[key]
	if !ok {
		return fallback
	}
	v, ok := val.(bool)
	if !ok {
		return fallback
	}
	return v
}

// String retrieves a string flag value.
// Returns fallback when the key is absent or the stored value is not a string.
func (f FeatureFlags) String(key string, fallback string) string {
	if f.values == nil {
		return fallback
	}
	val, ok := f.values[key]
	if !ok {
		return fallback
	}
	v, ok := val.(string)
	if !ok {
		return fallback
	}
	return v
}

// Float64 retrieves a numeric flag value.
// Returns fallback when the key is absent or the stored value is not a float64.
func (f FeatureFlags) Float64(key string, fallback float64) float64 {
	if f.values == nil {
		return fallback
	}
	val, ok := f.values[key]
	if !ok {
		return fallback
	}
	v, ok := val.(float64)
	if !ok {
		return fallback
	}
	return v
}

// Get returns the raw value for a flag key and whether it was found.
func (f FeatureFlags) Get(key string) (any, bool) {
	if f.values == nil {
		return nil, false
	}
	v, ok := f.values[key]
	return v, ok
}

// Clone returns a shallow copy of the flag set.
// The copy has its own map so the caller cannot mutate the original values.
func (f FeatureFlags) Clone() FeatureFlags {
	if f.values == nil {
		return FeatureFlags{}
	}
	m := make(map[string]any, len(f.values))
	for k, v := range f.values {
		m[k] = v
	}
	return FeatureFlags{values: m}
}

// JSON unmarshals a complex flag configuration into target (must be a pointer).
// Returns [ErrFlagNotFound] when the key is absent, or a marshalling error
// when the stored value cannot be encoded/decoded into target.
func (f FeatureFlags) JSON(key string, target any) error {
	if f.values == nil {
		return ErrFlagNotFound
	}
	val, ok := f.values[key]
	if !ok {
		return ErrFlagNotFound
	}
	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

// ToMap returns a shallow copy of all flag values as a plain map.
// This is useful for external cache adapters that need to serialise flags.
func (f FeatureFlags) ToMap() map[string]any {
	if f.values == nil {
		return map[string]any{}
	}
	m := make(map[string]any, len(f.values))
	for k, v := range f.values {
		m[k] = v
	}
	return m
}

// MarshalJSON implements [json.Marshaler] so that [FeatureFlags] can be
// serialised directly by external cache adapters.
func (f FeatureFlags) MarshalJSON() ([]byte, error) {
	if f.values == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(f.values)
}

// UnmarshalJSON implements [json.Unmarshaler] so that [FeatureFlags] can be
// deserialised by external cache adapters.
func (f *FeatureFlags) UnmarshalJSON(data []byte) error {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	f.values = m
	return nil
}

// EvaluationContext is the user/org context sent to the server for evaluation.
// Mirrors EvaluationContextT from the JS SDK.
type EvaluationContext struct {
	Kind         string         `json:"kind"` // "user", "organization", "multi"
	Key          string         `json:"key"`
	Attributes   map[string]any `json:"attributes,omitempty"`
	User         *ContextEntity `json:"user,omitempty"`         // for kind="multi"
	Organization *ContextEntity `json:"organization,omitempty"` // for kind="multi"
}

// MarshalJSON flattens attributes to the top level for API compatibility.
func (ec EvaluationContext) MarshalJSON() ([]byte, error) {
	// Build the output map
	out := make(map[string]any)

	out["kind"] = ec.Kind
	if ec.Key != "" {
		out["key"] = ec.Key
	}

	// Flatten attributes to top level
	for k, v := range ec.Attributes {
		out[k] = v
	}

	// Include nested contexts for multi-kind
	if ec.User != nil {
		out["user"] = ec.User
	}
	if ec.Organization != nil {
		out["organization"] = ec.Organization
	}

	return json.Marshal(out)
}

// ContextEntity represents a single entity within a multi-kind context.
type ContextEntity struct {
	Kind       string         `json:"kind,omitempty"`
	Key        string         `json:"key"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// MarshalJSON flattens attributes to the top level for API compatibility.
func (ce ContextEntity) MarshalJSON() ([]byte, error) {
	// Build the output map
	out := make(map[string]any)

	if ce.Kind != "" {
		out["kind"] = ce.Kind
	}
	out["key"] = ce.Key

	// Flatten attributes to top level
	for k, v := range ce.Attributes {
		out[k] = v
	}

	return json.Marshal(out)
}

// This file contains types for the generate.baml functions
// Generated manually following BAML client patterns

package types

import (
	"fmt"
	"reflect"

	baml "github.com/boundaryml/baml/engine/language_client_go/pkg"
	"github.com/boundaryml/baml/engine/language_client_go/pkg/cffi"
)

// GeneratedPolicy represents a policy created by AI generation
type GeneratedPolicy struct {
	Id          string `json:"id"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Instruction string `json:"instruction"`
	Enabled     bool   `json:"enabled"`
}

func (c *GeneratedPolicy) Decode(holder *cffi.CFFIValueClass, typeMap baml.TypeMap) {
	typeName := holder.Name
	if typeName.Namespace != cffi.CFFITypeNamespace_TYPES {
		panic(fmt.Sprintf("expected cffi.CFFITypeNamespace_TYPES, got %s", string(typeName.Namespace.String())))
	}
	if typeName.Name != "GeneratedPolicy" {
		panic(fmt.Sprintf("expected GeneratedPolicy, got %s", typeName.Name))
	}

	for _, field := range holder.Fields {
		key := field.Key
		valueHolder := field.Value
		switch key {
		case "id":
			c.Id = baml.Decode(valueHolder).Interface().(string)
		case "description":
			c.Description = baml.Decode(valueHolder).Interface().(string)
		case "severity":
			c.Severity = baml.Decode(valueHolder).Interface().(string)
		case "instruction":
			c.Instruction = baml.Decode(valueHolder).Interface().(string)
		case "enabled":
			c.Enabled = baml.Decode(valueHolder).Interface().(bool)
		default:
			panic(fmt.Sprintf("unexpected field: %s in class GeneratedPolicy", key))
		}
	}
}

func (c GeneratedPolicy) Encode() (*cffi.HostValue, error) {
	fields := map[string]any{}
	fields["id"] = c.Id
	fields["description"] = c.Description
	fields["severity"] = c.Severity
	fields["instruction"] = c.Instruction
	fields["enabled"] = c.Enabled
	return baml.EncodeClass("GeneratedPolicy", fields, nil)
}

func (c GeneratedPolicy) BamlTypeName() string {
	return "GeneratedPolicy"
}

// GeneratedRule represents a rule created by AI generation
type GeneratedRule struct {
	Id          string   `json:"id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Pattern     string   `json:"pattern"`
	Languages   []string `json:"languages"`
	Level       string   `json:"level"`
	Confidence  float64  `json:"confidence"`
	Message     string   `json:"message"`
	Explanation string   `json:"explanation"`
	Remediation string   `json:"remediation"`
	Source      string   `json:"source"`
	Cwe         []string `json:"cwe"`
	Owasp       []string `json:"owasp"`
	References  []string `json:"references"`
}

func (c *GeneratedRule) Decode(holder *cffi.CFFIValueClass, typeMap baml.TypeMap) {
	typeName := holder.Name
	if typeName.Namespace != cffi.CFFITypeNamespace_TYPES {
		panic(fmt.Sprintf("expected cffi.CFFITypeNamespace_TYPES, got %s", string(typeName.Namespace.String())))
	}
	if typeName.Name != "GeneratedRule" {
		panic(fmt.Sprintf("expected GeneratedRule, got %s", typeName.Name))
	}

	for _, field := range holder.Fields {
		key := field.Key
		valueHolder := field.Value
		switch key {
		case "id":
			c.Id = baml.Decode(valueHolder).Interface().(string)
		case "name":
			c.Name = baml.Decode(valueHolder).Interface().(string)
		case "category":
			c.Category = baml.Decode(valueHolder).Interface().(string)
		case "pattern":
			c.Pattern = baml.Decode(valueHolder).Interface().(string)
		case "languages":
			c.Languages = decodeStringSlice(valueHolder)
		case "level":
			c.Level = baml.Decode(valueHolder).Interface().(string)
		case "confidence":
			c.Confidence = baml.Decode(valueHolder).Float()
		case "message":
			c.Message = baml.Decode(valueHolder).Interface().(string)
		case "explanation":
			c.Explanation = baml.Decode(valueHolder).Interface().(string)
		case "remediation":
			c.Remediation = baml.Decode(valueHolder).Interface().(string)
		case "source":
			c.Source = baml.Decode(valueHolder).Interface().(string)
		case "cwe":
			c.Cwe = decodeStringSlice(valueHolder)
		case "owasp":
			c.Owasp = decodeStringSlice(valueHolder)
		case "references":
			c.References = decodeStringSlice(valueHolder)
		default:
			panic(fmt.Sprintf("unexpected field: %s in class GeneratedRule", key))
		}
	}
}

func (c GeneratedRule) Encode() (*cffi.HostValue, error) {
	fields := map[string]any{}
	fields["id"] = c.Id
	fields["name"] = c.Name
	fields["category"] = c.Category
	fields["pattern"] = c.Pattern
	fields["languages"] = c.Languages
	fields["level"] = c.Level
	fields["confidence"] = c.Confidence
	fields["message"] = c.Message
	fields["explanation"] = c.Explanation
	fields["remediation"] = c.Remediation
	fields["source"] = c.Source
	fields["cwe"] = c.Cwe
	fields["owasp"] = c.Owasp
	fields["references"] = c.References
	return baml.EncodeClass("GeneratedRule", fields, nil)
}

func (c GeneratedRule) BamlTypeName() string {
	return "GeneratedRule"
}

// GeneratedPersona represents a persona created by AI generation
type GeneratedPersona struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	SystemPrompt string `json:"system_prompt"`
}

func (c *GeneratedPersona) Decode(holder *cffi.CFFIValueClass, typeMap baml.TypeMap) {
	typeName := holder.Name
	if typeName.Namespace != cffi.CFFITypeNamespace_TYPES {
		panic(fmt.Sprintf("expected cffi.CFFITypeNamespace_TYPES, got %s", string(typeName.Namespace.String())))
	}
	if typeName.Name != "GeneratedPersona" {
		panic(fmt.Sprintf("expected GeneratedPersona, got %s", typeName.Name))
	}

	for _, field := range holder.Fields {
		key := field.Key
		valueHolder := field.Value
		switch key {
		case "name":
			c.Name = baml.Decode(valueHolder).Interface().(string)
		case "display_name":
			c.DisplayName = baml.Decode(valueHolder).Interface().(string)
		case "system_prompt":
			c.SystemPrompt = baml.Decode(valueHolder).Interface().(string)
		default:
			panic(fmt.Sprintf("unexpected field: %s in class GeneratedPersona", key))
		}
	}
}

func (c GeneratedPersona) Encode() (*cffi.HostValue, error) {
	fields := map[string]any{}
	fields["name"] = c.Name
	fields["display_name"] = c.DisplayName
	fields["system_prompt"] = c.SystemPrompt
	return baml.EncodeClass("GeneratedPersona", fields, nil)
}

func (c GeneratedPersona) BamlTypeName() string {
	return "GeneratedPersona"
}

// GeneratedProviderConfig represents provider config created by AI generation
type GeneratedProviderConfig struct {
	ProviderName string `json:"provider_name"`
	Model        string `json:"model"`
	BaseUrl      string `json:"base_url"`
	Region       string `json:"region"`
}

func (c *GeneratedProviderConfig) Decode(holder *cffi.CFFIValueClass, typeMap baml.TypeMap) {
	typeName := holder.Name
	if typeName.Namespace != cffi.CFFITypeNamespace_TYPES {
		panic(fmt.Sprintf("expected cffi.CFFITypeNamespace_TYPES, got %s", string(typeName.Namespace.String())))
	}
	if typeName.Name != "GeneratedProviderConfig" {
		panic(fmt.Sprintf("expected GeneratedProviderConfig, got %s", typeName.Name))
	}

	for _, field := range holder.Fields {
		key := field.Key
		valueHolder := field.Value
		switch key {
		case "provider_name":
			c.ProviderName = baml.Decode(valueHolder).Interface().(string)
		case "model":
			c.Model = baml.Decode(valueHolder).Interface().(string)
		case "base_url":
			c.BaseUrl = baml.Decode(valueHolder).Interface().(string)
		case "region":
			c.Region = baml.Decode(valueHolder).Interface().(string)
		default:
			panic(fmt.Sprintf("unexpected field: %s in class GeneratedProviderConfig", key))
		}
	}
}

func (c GeneratedProviderConfig) Encode() (*cffi.HostValue, error) {
	fields := map[string]any{}
	fields["provider_name"] = c.ProviderName
	fields["model"] = c.Model
	fields["base_url"] = c.BaseUrl
	fields["region"] = c.Region
	return baml.EncodeClass("GeneratedProviderConfig", fields, nil)
}

func (c GeneratedProviderConfig) BamlTypeName() string {
	return "GeneratedProviderConfig"
}

// GeneratedConfig represents a complete config created by AI generation
type GeneratedConfig struct {
	Provider GeneratedProviderConfig `json:"provider"`
	Persona  string                  `json:"persona"`
	Policies []GeneratedPolicy       `json:"policies"`
}

func (c *GeneratedConfig) Decode(holder *cffi.CFFIValueClass, typeMap baml.TypeMap) {
	typeName := holder.Name
	if typeName.Namespace != cffi.CFFITypeNamespace_TYPES {
		panic(fmt.Sprintf("expected cffi.CFFITypeNamespace_TYPES, got %s", string(typeName.Namespace.String())))
	}
	if typeName.Name != "GeneratedConfig" {
		panic(fmt.Sprintf("expected GeneratedConfig, got %s", typeName.Name))
	}

	for _, field := range holder.Fields {
		key := field.Key
		valueHolder := field.Value
		switch key {
		case "provider":
			// Decode nested class
			decoded := baml.Decode(valueHolder)
			if classVal, ok := decoded.Interface().(*cffi.CFFIValueClass); ok {
				c.Provider.Decode(classVal, typeMap)
			}
		case "persona":
			c.Persona = baml.Decode(valueHolder).Interface().(string)
		case "policies":
			c.Policies = decodePolicySlice(valueHolder)
		default:
			panic(fmt.Sprintf("unexpected field: %s in class GeneratedConfig", key))
		}
	}
}

func (c GeneratedConfig) Encode() (*cffi.HostValue, error) {
	fields := map[string]any{}
	fields["provider"] = c.Provider
	fields["persona"] = c.Persona
	fields["policies"] = c.Policies
	return baml.EncodeClass("GeneratedConfig", fields, nil)
}

func (c GeneratedConfig) BamlTypeName() string {
	return "GeneratedConfig"
}

// Helper function to decode string slices from BAML values
func decodeStringSlice(valueHolder *cffi.CFFIValueHolder) []string {
	decoded := baml.Decode(valueHolder)

	// Handle nil case
	if !decoded.IsValid() || decoded.IsNil() {
		return nil
	}

	// Try to cast as []interface{} and convert to []string
	if arr, ok := decoded.Interface().([]interface{}); ok {
		result := make([]string, len(arr))
		for i, item := range arr {
			if s, ok := item.(string); ok {
				result[i] = s
			}
		}
		return result
	}

	return nil
}

func decodePolicySlice(valueHolder *cffi.CFFIValueHolder) []GeneratedPolicy {
	decoded := baml.Decode(valueHolder)

	// Handle nil case
	if !decoded.IsValid() || decoded.IsNil() {
		return nil
	}

	// Try to get as slice using reflection
	v := reflect.ValueOf(decoded.Interface())
	if v.Kind() == reflect.Slice {
		result := make([]GeneratedPolicy, v.Len())
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i).Interface()
			if classVal, ok := elem.(*cffi.CFFIValueClass); ok {
				result[i].Decode(classVal, baml.TypeMap{})
			}
		}
		return result
	}

	return nil
}

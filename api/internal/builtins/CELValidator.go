// Copyright 2025 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package builtins

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/yaml"
	k8syaml "sigs.k8s.io/yaml"
)

// CELValidatorPlugin validates resources using Common Expression Language (CEL) expressions.
type CELValidatorPlugin struct {
	h *resmap.PluginHelpers

	// Validations contains the CEL validation rules
	Validations []CELValidation `json:"validations,omitempty" yaml:"validations,omitempty"`

	// FieldSpecs specifies which resources to validate (optional)
	FieldSpecs []types.FieldSpec `json:"fieldSpecs,omitempty" yaml:"fieldSpecs,omitempty"`
}

// CELValidation represents a single CEL validation rule
type CELValidation struct {
	// Expression is the CEL expression to evaluate
	Expression string `json:"expression" yaml:"expression"`

	// Message is the error message to display when validation fails
	Message string `json:"message,omitempty" yaml:"message,omitempty"`

	// ResourceSelector filters which resources this validation applies to
	ResourceSelector *ResourceSelector `json:"resourceSelector,omitempty" yaml:"resourceSelector,omitempty"`
}

// ResourceSelector selects resources by various criteria
type ResourceSelector struct {
	// APIVersion filters by API version
	APIVersion string `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`

	// Kind filters by resource kind
	Kind string `json:"kind,omitempty" yaml:"kind,omitempty"`

	// Name filters by resource name
	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	// Namespace filters by resource namespace
	Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`

	// LabelSelector filters by labels using CEL expression
	LabelSelector string `json:"labelSelector,omitempty" yaml:"labelSelector,omitempty"`
}

// Config configures the plugin from a byte slice
func (p *CELValidatorPlugin) Config(h *resmap.PluginHelpers, c []byte) error {
	p.h = h
	return k8syaml.Unmarshal(c, p)
}

// Transform validates resources using CEL expressions
func (p *CELValidatorPlugin) Transform(m resmap.ResMap) error {
	// Create CEL environment once
	env, err := p.createCELEnvironment()
	if err != nil {
		return fmt.Errorf("failed to create CEL environment: %w", err)
	}

	// Compile all expressions upfront
	compiledValidations := make([]compiledValidation, len(p.Validations))
	for i, v := range p.Validations {
		compiled, err := p.compileValidation(env, v)
		if err != nil {
			return fmt.Errorf("failed to compile validation %d: %w", i, err)
		}
		compiledValidations[i] = compiled
	}

	// Validate each resource
	for _, r := range m.Resources() {
		if err := p.validateResource(r, compiledValidations); err != nil {
			return err
		}
	}

	return nil
}

type compiledValidation struct {
	validation CELValidation
	program    cel.Program
	selector   cel.Program // Optional, for label selector
}

func (p *CELValidatorPlugin) createCELEnvironment() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("object", cel.DynType),
		cel.Variable("oldObject", cel.DynType),
		cel.Variable("request", cel.DynType),
		cel.Variable("params", cel.DynType),
		cel.Variable("namespaceObject", cel.DynType),
		cel.Variable("variables", cel.DynType),
	)
}

func (p *CELValidatorPlugin) compileValidation(env *cel.Env, v CELValidation) (compiledValidation, error) {
	ast, issues := env.Compile(v.Expression)
	if issues != nil && issues.Err() != nil {
		return compiledValidation{}, fmt.Errorf("CEL compilation error in expression '%s': %w", v.Expression, issues.Err())
	}

	prg, err := env.Program(ast)
	if err != nil {
		return compiledValidation{}, fmt.Errorf("CEL program error in expression '%s': %w", v.Expression, err)
	}

	result := compiledValidation{
		validation: v,
		program:    prg,
	}

	// Compile label selector if present
	if v.ResourceSelector != nil && v.ResourceSelector.LabelSelector != "" {
		ast, issues := env.Compile(v.ResourceSelector.LabelSelector)
		if issues != nil && issues.Err() != nil {
			return compiledValidation{}, fmt.Errorf("CEL compilation error in label selector '%s': %w", v.ResourceSelector.LabelSelector, issues.Err())
		}

		prg, err := env.Program(ast)
		if err != nil {
			return compiledValidation{}, fmt.Errorf("CEL program error in label selector '%s': %w", v.ResourceSelector.LabelSelector, err)
		}
		result.selector = prg
	}

	return result, nil
}

func (p *CELValidatorPlugin) validateResource(r *resource.Resource, validations []compiledValidation) error {
	// Convert resource to map for CEL evaluation
	resourceMap, err := r.Map()
	if err != nil {
		return fmt.Errorf("failed to convert resource to map: %w", err)
	}

	// Get resource metadata
	meta, err := r.GetMeta()
	if err != nil {
		return fmt.Errorf("failed to get resource metadata: %w", err)
	}

	for _, cv := range validations {
		// Check if this validation applies to this resource
		if !p.shouldValidate(meta, resourceMap, cv) {
			continue
		}

		// Prepare CEL variables
		variables := map[string]interface{}{
			"object":          resourceMap,
			"oldObject":       map[string]interface{}{}, // Empty for creation
			"request":         map[string]interface{}{}, // Could be populated with request context
			"params":          map[string]interface{}{}, // Could be populated with parameters
			"namespaceObject": map[string]interface{}{}, // Could be populated with namespace object
			"variables":       map[string]interface{}{}, // Could be populated with custom variables
		}

		// Evaluate the CEL expression
		out, _, err := cv.program.Eval(variables)
		if err != nil {
			return fmt.Errorf("CEL evaluation error for resource %s/%s: %w", meta.Kind, meta.Name, err)
		}

		// Check if validation passed (expression should return true for valid resources)
		valid, ok := out.Value().(bool)
		if !ok {
			return fmt.Errorf("CEL expression must return a boolean value for resource %s/%s", meta.Kind, meta.Name)
		}

		if !valid {
			message := cv.validation.Message
			if message == "" {
				message = fmt.Sprintf("CEL validation failed: %s", cv.validation.Expression)
			}
			return fmt.Errorf("validation failed for resource %s/%s: %s", meta.Kind, meta.Name, message)
		}
	}

	// Add validated-by label to track that this resource was validated
	labels := r.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["validated-by"] = "cel-validator"
	if err := r.SetLabels(labels); err != nil {
		return fmt.Errorf("failed to set validated-by label: %w", err)
	}

	return nil
}

func (p *CELValidatorPlugin) shouldValidate(meta yaml.ResourceMeta, resourceMap map[string]interface{}, cv compiledValidation) bool {
	if cv.validation.ResourceSelector == nil {
		// No selector means validate all resources
		return true
	}

	sel := cv.validation.ResourceSelector

	// Check API version
	if sel.APIVersion != "" && sel.APIVersion != meta.APIVersion {
		return false
	}

	// Check kind
	if sel.Kind != "" && sel.Kind != meta.Kind {
		return false
	}

	// Check name
	if sel.Name != "" && sel.Name != meta.Name {
		return false
	}

	// Check namespace
	if sel.Namespace != "" && sel.Namespace != meta.Namespace {
		return false
	}

	// Check label selector using CEL
	if cv.selector != nil {
		labels := meta.Labels
		if labels == nil {
			labels = make(map[string]string)
		}

		variables := map[string]interface{}{
			"object": resourceMap,
		}

		out, _, err := cv.selector.Eval(variables)
		if err != nil {
			// If selector evaluation fails, skip this resource
			return false
		}

		matches, ok := out.Value().(bool)
		if !ok || !matches {
			return false
		}
	}

	return true
}

// NewCELValidatorPlugin creates a new CELValidatorPlugin instance
func NewCELValidatorPlugin() resmap.TransformerPlugin {
	return &CELValidatorPlugin{}
}
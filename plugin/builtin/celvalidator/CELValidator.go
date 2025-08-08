// Copyright 2025 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

// Package main implements the CELValidator transformer plugin.
package main

import (
	"sigs.k8s.io/kustomize/api/internal/builtins"
	"sigs.k8s.io/kustomize/api/resmap"
)

// main is required for the plugin to be loadable.
func main() {}

// NewCELValidatorPlugin returns a new CELValidator plugin.
func NewCELValidatorPlugin() resmap.TransformerPlugin {
	return builtins.NewCELValidatorPlugin()
}
// Copyright 2025 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package builtins_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/api/internal/builtins"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

func TestCELValidator_ValidResource(t *testing.T) {
	validator := builtins.NewCELValidatorPlugin()
	
	config := `
validations:
  - expression: "object.spec.replicas >= 1 && object.spec.replicas <= 10"
    message: "Replicas must be between 1 and 10"
    resourceSelector:
      kind: Deployment
`

	err := validator.Config(resmap.NewPluginHelpers(nil, nil, filesys.MakeFsInMemory()), []byte(config))
	require.NoError(t, err)

	rm := resmap.New()
	r := resource.MustParse(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: 3
`)
	rm.Append(r)

	err = validator.Transform(rm)
	assert.NoError(t, err)

	// Check that validated-by label was added
	labels := r.GetLabels()
	assert.Equal(t, "cel-validator", labels["validated-by"])
}

func TestCELValidator_InvalidResource(t *testing.T) {
	validator := builtins.NewCELValidatorPlugin()
	
	config := `
validations:
  - expression: "object.spec.replicas >= 1 && object.spec.replicas <= 10"
    message: "Replicas must be between 1 and 10"
    resourceSelector:
      kind: Deployment
`

	err := validator.Config(resmap.NewPluginHelpers(nil, nil, filesys.MakeFsInMemory()), []byte(config))
	require.NoError(t, err)

	rm := resmap.New()
	r := resource.MustParse(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: 15
`)
	rm.Append(r)

	err = validator.Transform(rm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Replicas must be between 1 and 10")
}

func TestCELValidator_MultipleValidations(t *testing.T) {
	validator := builtins.NewCELValidatorPlugin()
	
	config := `
validations:
  - expression: "has(object.metadata.labels)"
    message: "Resource must have labels"
  - expression: "'app' in object.metadata.labels"
    message: "Resource must have 'app' label"
  - expression: "object.metadata.labels['app'] != ''"
    message: "App label must not be empty"
`

	err := validator.Config(resmap.NewPluginHelpers(nil, nil, filesys.MakeFsInMemory()), []byte(config))
	require.NoError(t, err)

	rm := resmap.New()
	r := resource.MustParse(`
apiVersion: v1
kind: Service
metadata:
  name: test-service
  labels:
    app: myapp
    tier: frontend
spec:
  ports:
  - port: 80
`)
	rm.Append(r)

	err = validator.Transform(rm)
	assert.NoError(t, err)
}

func TestCELValidator_ResourceSelector(t *testing.T) {
	validator := builtins.NewCELValidatorPlugin()
	
	config := `
validations:
  - expression: "object.spec.type == 'ClusterIP'"
    message: "Only ClusterIP services are allowed"
    resourceSelector:
      kind: Service
      namespace: production
`

	err := validator.Config(resmap.NewPluginHelpers(nil, nil, filesys.MakeFsInMemory()), []byte(config))
	require.NoError(t, err)

	rm := resmap.New()
	
	// This should be validated (matches selector)
	r1 := resource.MustParse(`
apiVersion: v1
kind: Service
metadata:
  name: test-service
  namespace: production
spec:
  type: ClusterIP
`)
	rm.Append(r1)

	// This should NOT be validated (different namespace)
	r2 := resource.MustParse(`
apiVersion: v1
kind: Service
metadata:
  name: test-service
  namespace: development
spec:
  type: LoadBalancer
`)
	rm.Append(r2)

	// This should NOT be validated (different kind)
	r3 := resource.MustParse(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
  namespace: production
spec:
  replicas: 1
`)
	rm.Append(r3)

	err = validator.Transform(rm)
	assert.NoError(t, err)

	// Only r1 should have the validated-by label
	assert.Equal(t, "cel-validator", r1.GetLabels()["validated-by"])
	assert.Empty(t, r2.GetLabels()["validated-by"])
	assert.Empty(t, r3.GetLabels()["validated-by"])
}

func TestCELValidator_ContainerValidation(t *testing.T) {
	validator := builtins.NewCELValidatorPlugin()
	
	config := `
validations:
  - expression: |
      object.spec.template.spec.containers.all(container,
        has(container.resources) &&
        has(container.resources.limits) &&
        has(container.resources.limits.memory) &&
        has(container.resources.limits.cpu)
      )
    message: "All containers must have CPU and memory limits"
    resourceSelector:
      kind: Deployment
`

	err := validator.Config(resmap.NewPluginHelpers(nil, nil, filesys.MakeFsInMemory()), []byte(config))
	require.NoError(t, err)

	rm := resmap.New()
	r := resource.MustParse(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: app
        image: nginx:latest
        resources:
          limits:
            cpu: "500m"
            memory: "256Mi"
          requests:
            cpu: "250m"
            memory: "128Mi"
      - name: sidecar
        image: busybox:latest
        resources:
          limits:
            cpu: "100m"
            memory: "64Mi"
`)
	rm.Append(r)

	err = validator.Transform(rm)
	assert.NoError(t, err)
}

func TestCELValidator_InvalidExpression(t *testing.T) {
	validator := builtins.NewCELValidatorPlugin()
	
	config := `
validations:
  - expression: "this is not valid CEL"
    message: "Invalid expression"
`

	err := validator.Config(resmap.NewPluginHelpers(nil, nil, filesys.MakeFsInMemory()), []byte(config))
	require.NoError(t, err)

	rm := resmap.New()
	r := resource.MustParse(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
data:
  key: value
`)
	rm.Append(r)

	err = validator.Transform(rm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "compilation error")
}

func TestCELValidator_SecurityPolicyValidation(t *testing.T) {
	validator := builtins.NewCELValidatorPlugin()
	
	config := `
validations:
  - expression: |
      !has(object.spec.template.spec.securityContext) ||
      !has(object.spec.template.spec.securityContext.runAsUser) ||
      object.spec.template.spec.securityContext.runAsUser != 0
    message: "Containers must not run as root (UID 0)"
    resourceSelector:
      kind: Deployment
  - expression: |
      object.spec.template.spec.containers.all(container,
        !has(container.securityContext) ||
        !has(container.securityContext.privileged) ||
        container.securityContext.privileged == false
      )
    message: "Containers must not run in privileged mode"
    resourceSelector:
      kind: Deployment
`

	err := validator.Config(resmap.NewPluginHelpers(nil, nil, filesys.MakeFsInMemory()), []byte(config))
	require.NoError(t, err)

	rm := resmap.New()
	r := resource.MustParse(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: secure-deployment
spec:
  replicas: 1
  template:
    spec:
      securityContext:
        runAsUser: 1000
        runAsGroup: 3000
        fsGroup: 2000
      containers:
      - name: app
        image: nginx:latest
        securityContext:
          privileged: false
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
`)
	rm.Append(r)

	err = validator.Transform(rm)
	assert.NoError(t, err)
}

func TestCELValidator_NetworkPolicyValidation(t *testing.T) {
	validator := builtins.NewCELValidatorPlugin()
	
	config := `
validations:
  - expression: |
      has(object.spec.podSelector) &&
      has(object.spec.policyTypes) &&
      object.spec.policyTypes.exists(t, t == 'Ingress')
    message: "NetworkPolicy must define ingress rules"
    resourceSelector:
      kind: NetworkPolicy
`

	err := validator.Config(resmap.NewPluginHelpers(nil, nil, filesys.MakeFsInMemory()), []byte(config))
	require.NoError(t, err)

	rm := resmap.New()
	r := resource.MustParse(`
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: test-netpol
spec:
  podSelector:
    matchLabels:
      app: web
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: backend
`)
	rm.Append(r)

	err = validator.Transform(rm)
	assert.NoError(t, err)
}
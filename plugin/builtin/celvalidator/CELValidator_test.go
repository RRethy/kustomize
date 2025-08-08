// Copyright 2025 The Kubernetes Authors.
// SPDX-License-Identifier: Apache-2.0

package main_test

import (
	"testing"

	kusttest_test "sigs.k8s.io/kustomize/api/testutils/kusttest"
)

func TestCELValidator_ValidResource(t *testing.T) {
	th := kusttest_test.MakeEnhancedHarness(t).
		PrepBuiltin("CELValidator")
	defer th.Reset()

	rm := th.LoadAndRunTransformer(`
apiVersion: builtin
kind: CELValidator
metadata:
  name: celvalidator
validations:
  - expression: "object.spec.replicas >= 1 && object.spec.replicas <= 10"
    message: "Replicas must be between 1 and 10"
    resourceSelector:
      kind: Deployment
`, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: 3
`)

	expected := `
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    validated-by: cel-validator
  name: test-deployment
spec:
  replicas: 3
`

	th.AssertActualEqualsExpectedNoIdAnnotations(rm, expected)
}

func TestCELValidator_InvalidResource(t *testing.T) {
	th := kusttest_test.MakeEnhancedHarness(t).
		PrepBuiltin("CELValidator")
	defer th.Reset()

	err := th.ErrorFromLoadAndRunTransformer(`
apiVersion: builtin
kind: CELValidator
metadata:
  name: celvalidator
validations:
  - expression: "object.spec.replicas >= 1 && object.spec.replicas <= 10"
    message: "Replicas must be between 1 and 10"
    resourceSelector:
      kind: Deployment
`, `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  replicas: 15
`)

	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !containsString(err.Error(), "Replicas must be between 1 and 10") {
		t.Fatalf("expected error message about replicas, got: %v", err)
	}
}

func TestCELValidator_MultipleValidations(t *testing.T) {
	th := kusttest_test.MakeEnhancedHarness(t).
		PrepBuiltin("CELValidator")
	defer th.Reset()

	rm := th.LoadAndRunTransformer(`
apiVersion: builtin
kind: CELValidator
metadata:
  name: celvalidator
validations:
  - expression: "has(object.metadata.labels)"
    message: "Resource must have labels"
  - expression: "'app' in object.metadata.labels"
    message: "Resource must have 'app' label"
  - expression: "object.metadata.labels['app'] != ''"
    message: "App label must not be empty"
`, `
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

	expected := `
apiVersion: v1
kind: Service
metadata:
  labels:
    app: myapp
    tier: frontend
    validated-by: cel-validator
  name: test-service
spec:
  ports:
  - port: 80
`

	th.AssertActualEqualsExpectedNoIdAnnotations(rm, expected)
}

func TestCELValidator_ResourceSelector(t *testing.T) {
	th := kusttest_test.MakeEnhancedHarness(t).
		PrepBuiltin("CELValidator")
	defer th.Reset()

	rm := th.LoadAndRunTransformer(`
apiVersion: builtin
kind: CELValidator
metadata:
  name: celvalidator
validations:
  - expression: "object.spec.type == 'ClusterIP'"
    message: "Only ClusterIP services are allowed"
    resourceSelector:
      kind: Service
      namespace: production
`, `
apiVersion: v1
kind: Service
metadata:
  name: test-service
  namespace: production
spec:
  type: ClusterIP
---
apiVersion: v1
kind: Service
metadata:
  name: test-service2
  namespace: development
spec:
  type: LoadBalancer
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
  namespace: production
spec:
  replicas: 1
`)

	expected := `
apiVersion: v1
kind: Service
metadata:
  labels:
    validated-by: cel-validator
  name: test-service
  namespace: production
spec:
  type: ClusterIP
---
apiVersion: v1
kind: Service
metadata:
  name: test-service2
  namespace: development
spec:
  type: LoadBalancer
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
  namespace: production
spec:
  replicas: 1
`

	th.AssertActualEqualsExpectedNoIdAnnotations(rm, expected)
}

func TestCELValidator_ContainerValidation(t *testing.T) {
	th := kusttest_test.MakeEnhancedHarness(t).
		PrepBuiltin("CELValidator")
	defer th.Reset()

	rm := th.LoadAndRunTransformer(`
apiVersion: builtin
kind: CELValidator
metadata:
  name: celvalidator
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
`, `
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

	expected := `
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    validated-by: cel-validator
  name: test-deployment
spec:
  replicas: 2
  template:
    spec:
      containers:
      - image: nginx:latest
        name: app
        resources:
          limits:
            cpu: "500m"
            memory: "256Mi"
          requests:
            cpu: "250m"
            memory: "128Mi"
      - image: busybox:latest
        name: sidecar
        resources:
          limits:
            cpu: "100m"
            memory: "64Mi"
`

	th.AssertActualEqualsExpectedNoIdAnnotations(rm, expected)
}

func TestCELValidator_SecurityPolicyValidation(t *testing.T) {
	th := kusttest_test.MakeEnhancedHarness(t).
		PrepBuiltin("CELValidator")
	defer th.Reset()

	rm := th.LoadAndRunTransformer(`
apiVersion: builtin
kind: CELValidator
metadata:
  name: celvalidator
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
`, `
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

	expected := `
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    validated-by: cel-validator
  name: secure-deployment
spec:
  replicas: 1
  template:
    spec:
      containers:
      - image: nginx:latest
        name: app
        securityContext:
          allowPrivilegeEscalation: false
          privileged: false
          readOnlyRootFilesystem: true
      securityContext:
        fsGroup: 2000
        runAsGroup: 3000
        runAsUser: 1000
`

	th.AssertActualEqualsExpectedNoIdAnnotations(rm, expected)
}

func TestCELValidator_NetworkPolicyValidation(t *testing.T) {
	th := kusttest_test.MakeEnhancedHarness(t).
		PrepBuiltin("CELValidator")
	defer th.Reset()

	rm := th.LoadAndRunTransformer(`
apiVersion: builtin
kind: CELValidator
metadata:
  name: celvalidator
validations:
  - expression: |
      has(object.spec.podSelector) &&
      has(object.spec.policyTypes) &&
      object.spec.policyTypes.exists(t, t == 'Ingress')
    message: "NetworkPolicy must define ingress rules"
    resourceSelector:
      kind: NetworkPolicy
`, `
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

	expected := `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  labels:
    validated-by: cel-validator
  name: test-netpol
spec:
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: backend
  podSelector:
    matchLabels:
      app: web
  policyTypes:
  - Ingress
  - Egress
`

	th.AssertActualEqualsExpectedNoIdAnnotations(rm, expected)
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(s)] != "" && s != "" && substr != "" && 
		len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && 
		(s[:len(substr)] == substr || containsString(s[1:], substr))))
}
# CEL Validator

This example demonstrates how to use the CEL (Common Expression Language) validator to validate Kubernetes resources during the kustomize build process.

## What is CEL?

CEL (Common Expression Language) is a non-Turing complete language designed for simplicity, speed, and safety. It's used in Kubernetes for validation and policy enforcement. The CEL validator in kustomize allows you to write validation rules using CEL expressions that are evaluated against your resources.

## Example Usage

### Directory Structure

```
celvalidator/
├── kustomization.yaml
├── deployment.yaml
├── service.yaml
└── cel-validator.yaml
```

### cel-validator.yaml

Create a CEL validator configuration file:

```yaml
apiVersion: builtin
kind: CELValidator
metadata:
  name: resource-validator
validations:
  # Ensure deployments have reasonable replica counts
  - expression: "object.spec.replicas >= 1 && object.spec.replicas <= 10"
    message: "Deployment replicas must be between 1 and 10"
    resourceSelector:
      kind: Deployment

  # Ensure all resources have required labels
  - expression: "has(object.metadata.labels) && 'app' in object.metadata.labels"
    message: "All resources must have an 'app' label"

  # Ensure services use ClusterIP type in production
  - expression: "!has(object.metadata.namespace) || object.metadata.namespace != 'production' || object.spec.type == 'ClusterIP'"
    message: "Services in production namespace must use ClusterIP type"
    resourceSelector:
      kind: Service

  # Ensure containers have resource limits
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

  # Security: Ensure containers don't run as root
  - expression: |
      !has(object.spec.template.spec.securityContext) ||
      !has(object.spec.template.spec.securityContext.runAsUser) ||
      object.spec.template.spec.securityContext.runAsUser != 0
    message: "Containers must not run as root (UID 0)"
    resourceSelector:
      kind: Deployment

  # Security: Ensure containers are not privileged
  - expression: |
      object.spec.template.spec.containers.all(container,
        !has(container.securityContext) ||
        !has(container.securityContext.privileged) ||
        container.securityContext.privileged == false
      )
    message: "Containers must not run in privileged mode"
    resourceSelector:
      kind: Deployment
```

### deployment.yaml

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  labels:
    app: myapp
spec:
  replicas: 3
  selector:
    matchLabels:
      app: myapp
  template:
    metadata:
      labels:
        app: myapp
    spec:
      securityContext:
        runAsUser: 1000
        runAsGroup: 3000
        fsGroup: 2000
      containers:
      - name: app
        image: myapp:latest
        resources:
          limits:
            cpu: "500m"
            memory: "256Mi"
          requests:
            cpu: "250m"
            memory: "128Mi"
        securityContext:
          privileged: false
          readOnlyRootFilesystem: true
          allowPrivilegeEscalation: false
```

### service.yaml

```yaml
apiVersion: v1
kind: Service
metadata:
  name: myapp
  labels:
    app: myapp
spec:
  type: ClusterIP
  selector:
    app: myapp
  ports:
  - port: 80
    targetPort: 8080
```

### kustomization.yaml

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- deployment.yaml
- service.yaml

validators:
- cel-validator.yaml
```

## Running the Validation

```bash
kustomize build .
```

If all validations pass, the resources will be built successfully with a `validated-by: cel-validator` label added to each validated resource.

If any validation fails, kustomize will return an error with the specific validation message.

## CEL Expression Reference

### Common CEL Functions

- `has(field)` - Check if a field exists
- `field in object` - Check if a key exists in a map
- `list.all(item, predicate)` - Check if all items in a list match a predicate
- `list.exists(item, predicate)` - Check if any item in a list matches a predicate
- `list.filter(item, predicate)` - Filter a list based on a predicate
- `string.matches(regex)` - Check if a string matches a regex pattern

### Available Variables

- `object` - The current resource being validated
- `oldObject` - The previous version of the resource (for updates)
- `request` - Request context (if available)
- `params` - Additional parameters (if configured)
- `namespaceObject` - The namespace object (if applicable)
- `variables` - Custom variables (if configured)

### Resource Selector Options

The `resourceSelector` field supports:
- `apiVersion` - Filter by API version
- `kind` - Filter by resource kind
- `name` - Filter by resource name
- `namespace` - Filter by namespace
- `labelSelector` - CEL expression to filter by labels

## Advanced Examples

### Network Policy Validation

```yaml
validations:
  - expression: |
      has(object.spec.podSelector) &&
      has(object.spec.policyTypes) &&
      object.spec.policyTypes.exists(t, t == 'Ingress')
    message: "NetworkPolicy must define ingress rules"
    resourceSelector:
      kind: NetworkPolicy
```

### ConfigMap Size Validation

```yaml
validations:
  - expression: |
      !has(object.data) || 
      object.data.all(key, size(object.data[key]) <= 1048576)
    message: "ConfigMap values must not exceed 1MB"
    resourceSelector:
      kind: ConfigMap
```

### Label Format Validation

```yaml
validations:
  - expression: |
      has(object.metadata.labels) &&
      object.metadata.labels.all(key, key.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$'))
    message: "Label keys must be valid DNS labels"
```

## Benefits

1. **Type Safety**: CEL is strongly typed and validates expressions at compile time
2. **Performance**: CEL expressions are compiled once and evaluated efficiently
3. **Security**: CEL is non-Turing complete, preventing infinite loops and resource exhaustion
4. **Flexibility**: Complex validation logic can be expressed concisely
5. **Kubernetes Native**: CEL is used throughout Kubernetes for validation and policy

## Troubleshooting

If you encounter compilation errors:
- Check that your CEL expressions are syntactically valid
- Ensure field paths in expressions match your resource structure
- Use `has()` to check for optional fields before accessing them

If validations aren't triggering:
- Verify your `resourceSelector` matches the intended resources
- Check that the validator is properly referenced in `kustomization.yaml`
- Ensure the CEL validator plugin is available in your kustomize build
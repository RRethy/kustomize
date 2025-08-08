# CEL Validator Demo

This example demonstrates the CEL validator catching common policy violations.

## Files

- **good-deployment.yaml**: ✅ Passes all validations
  - Has required labels (app, team)
  - Replicas within limit (3)
  - Runs as non-root (UID 1000)
  - Memory under 1Gi (512Mi)

- **bad-deployment.yaml**: ❌ Fails multiple validations
  - Missing 'team' label
  - Too many replicas (10 > 5)
  - No securityContext (runs as root)
  - Memory over limit (2Gi > 1Gi)

- **service.yaml**: Mixed
  - frontend-service: ✅ ClusterIP type
  - external-service: ❌ LoadBalancer type not allowed

## Expected Validation Errors

When you run `kustomize build .`, you should see errors like:

```
❌ Resources must have 'app' and 'team' labels for tracking
   Failed on: Deployment/bad-app

❌ Deployments must have between 1 and 5 replicas for cost control
   Failed on: Deployment/bad-app (has 10 replicas)

❌ Container memory limits must be <= 1Gi (use Mi units or exactly 1Gi)
   Failed on: Deployment/bad-app

❌ Containers must run as non-root user (UID >= 1000)
   Failed on: Deployment/bad-app

❌ Only ClusterIP services allowed (no LoadBalancer or NodePort)
   Failed on: Service/external-service
```

## Testing Individual Resources

To test just the good deployment:
```bash
kustomize build . | grep -A 20 "name: good-app"
```

## Validation Rules

| Rule | Description | Applies To |
|------|-------------|------------|
| Replica Limits | 1-5 replicas max | Deployments |
| Required Labels | Must have 'app' and 'team' | All resources |
| Memory Limits | Max 1Gi per container | Deployments |
| Service Types | Only ClusterIP allowed | Services |
| Security | No root containers (UID >= 1000) | Deployments |

## Customizing Rules

Edit `validator.yaml` to:
- Change limits (e.g., allow more replicas)
- Add new required labels
- Enforce different security policies
- Add namespace-specific rules

Example: Allow NodePort services in 'public' namespace:
```yaml
- expression: |
    object.spec.type == 'ClusterIP' ||
    (object.metadata.namespace == 'public' && object.spec.type == 'NodePort')
  message: "Services must be ClusterIP (NodePort allowed in 'public' namespace)"
  resourceSelector:
    kind: Service
```
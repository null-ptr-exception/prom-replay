# Design Spec: VictoriaMetrics ServiceAccount Flexibility

Enable flexible ServiceAccount configuration for the VictoriaMetrics pod in the `prom-replay` Helm chart. This allows users to provide an existing ServiceAccount, add annotations, or customize the name of the chart-managed ServiceAccount.

## Problem
The current Helm chart automatically creates and assigns a ServiceAccount to the VictoriaMetrics pod only if `scrapeConfig` is enabled. Users cannot:
1. Provide an existing ServiceAccount.
2. Add annotations (e.g., for IAM roles) to the ServiceAccount.
3. Use a custom name for the ServiceAccount.

## Proposed Changes

### 1. `values.yaml`
Add a new `serviceAccount` block under `victoriametrics`.

```yaml
victoriametrics:
  serviceAccount:
    # Specifies whether a service account should be created
    create: true
    # Annotations to add to the service account
    annotations: {}
    # The name of the service account to use.
    # If not set and create is true, a name is generated using the fullname template
    name: ""
```

### 2. `_helpers.tpl`
Add a helper template to resolve the ServiceAccount name for VictoriaMetrics.

```template
{{/*
Create the name of the service account to use for VictoriaMetrics
*/}}
{{- define "prom-replay.victoriametrics.serviceAccountName" -}}
{{- if .Values.victoriametrics.serviceAccount.create -}}
    {{- default (printf "%s-victoriametrics" (include "prom-replay.fullname" .)) .Values.victoriametrics.serviceAccount.name -}}
{{- else -}}
    {{- default "default" .Values.victoriametrics.serviceAccount.name -}}
{{- end -}}
{{- end -}}
```

### 3. `templates/victoriametrics-deployment.yaml`
Update the `StatefulSet` to use the helper for `serviceAccountName`.

```yaml
spec:
  template:
    spec:
      serviceAccountName: {{ include "prom-replay.victoriametrics.serviceAccountName" . }}
```

### 4. `templates/victoriametrics-rbac.yaml`
Update the `ServiceAccount` and `RoleBinding`/`ClusterRoleBinding` resources.

- Only create the `ServiceAccount` if `victoriametrics.serviceAccount.create` is `true`.
- Use `victoriametrics.serviceAccount.annotations` when creating the `ServiceAccount`.
- Update bindings to use the name resolved by the helper.

## Compatibility & Migration
- **Backward Compatibility**: If `serviceAccount.create` is not specified, it should default to `true` if `scrapeConfig` is enabled to maintain current behavior. (Note: I will set `create: true` as the default in `values.yaml`).
- **RBAC**: RBAC resources will still be created if `scrapeConfig` is enabled, even if using an existing ServiceAccount.

## Testing Strategy
1. **Default behavior**: Verify SA is created and assigned when `create: true`.
2. **Existing SA**: Verify SA is NOT created and the custom name is assigned to the pod when `create: false` and `name: "my-sa"`.
3. **Annotations**: Verify annotations are applied to the SA.
4. **RBAC integration**: Verify RoleBindings point to the correct SA name in all scenarios.

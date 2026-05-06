# VictoriaMetrics ServiceAccount Flexibility Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable flexible ServiceAccount configuration for VictoriaMetrics pods, allowing for existing accounts, custom names, and annotations.

**Architecture:** Implement the standard Helm `serviceAccount` pattern by adding a structured configuration block in `values.yaml`, a helper template for name resolution, and updating the deployment and RBAC templates.

**Tech Stack:** Helm, Kubernetes RBAC, `helm-unittest`

---

### Task 1: Update `values.yaml`

**Files:**
- Modify: `charts/prom-replay/values.yaml`

- [ ] **Step 1: Add the `serviceAccount` block under `victoriametrics`**

```yaml
victoriametrics:
  # ... existing values ...
  serviceAccount:
    # Specifies whether a service account should be created
    create: true
    # Annotations to add to the service account
    annotations: {}
    # The name of the service account to use.
    # If not set and create is true, a name is generated using the fullname template
    name: ""
```

- [ ] **Step 2: Commit**

```bash
git add charts/prom-replay/values.yaml
git commit -m "feat(helm): add serviceAccount config for victoriametrics"
```

---

### Task 2: Add Helper Template

**Files:**
- Modify: `charts/prom-replay/templates/_helpers.tpl`

- [ ] **Step 1: Add `prom-replay.victoriametrics.serviceAccountName` helper**

Add this to the end of `charts/prom-replay/templates/_helpers.tpl`:

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

- [ ] **Step 2: Commit**

```bash
git add charts/prom-replay/templates/_helpers.tpl
git commit -m "feat(helm): add SA name helper for victoriametrics"
```

---

### Task 3: Update Deployment Template

**Files:**
- Modify: `charts/prom-replay/templates/victoriametrics-deployment.yaml`

- [ ] **Step 1: Update `serviceAccountName` to use the new helper**

Change:
```yaml
      {{- if .Values.victoriametrics.scrapeConfig }}
      serviceAccountName: {{ include "prom-replay.fullname" . }}-victoriametrics
      {{- end }}
```
To:
```yaml
      serviceAccountName: {{ include "prom-replay.victoriametrics.serviceAccountName" . }}
```

- [ ] **Step 2: Commit**

```bash
git add charts/prom-replay/templates/victoriametrics-deployment.yaml
git commit -m "feat(helm): use dynamic serviceAccountName in victoriametrics deployment"
```

---

### Task 4: Update RBAC Template

**Files:**
- Modify: `charts/prom-replay/templates/victoriametrics-rbac.yaml`

- [ ] **Step 1: Decouple ServiceAccount creation from `scrapeConfig`**

Refactor the file to create the SA based on `.Values.victoriametrics.serviceAccount.create` and update RoleBindings to use the helper.

```yaml
{{- if .Values.victoriametrics.serviceAccount.create }}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "prom-replay.victoriametrics.serviceAccountName" . }}
  labels:
    {{- include "prom-replay.labels" . | nindent 4 }}
    app.kubernetes.io/component: victoriametrics
  {{- with .Values.victoriametrics.serviceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
---
{{- end }}
{{- if .Values.victoriametrics.scrapeConfig }}
{{- if .Values.victoriametrics.rbac.clusterWide }}
# ... ClusterRole ...
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
# ...
subjects:
  - kind: ServiceAccount
    name: {{ include "prom-replay.victoriametrics.serviceAccountName" . }}
    namespace: {{ .Release.Namespace }}
---
{{- else }}
# ... Role ...
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
# ...
subjects:
  - kind: ServiceAccount
    name: {{ include "prom-replay.victoriametrics.serviceAccountName" . }}
    namespace: {{ .Release.Namespace }}
{{- end }}
{{- end }}
```

- [ ] **Step 2: Commit**

```bash
git add charts/prom-replay/templates/victoriametrics-rbac.yaml
git commit -m "feat(helm): update victoriametrics rbac to support flexible SA"
```

---

### Task 5: Verify with Unit Tests

**Files:**
- Modify: `charts/prom-replay/tests/victoriametrics_test.yaml`
- Modify: `charts/prom-replay/tests/victoriametrics-rbac_test.yaml`

- [ ] **Step 1: Update Deployment tests**

Update `charts/prom-replay/tests/victoriametrics_test.yaml` to reflect that `serviceAccountName` is now set by default (since `create: true`).

```yaml
  - it: should set serviceAccountName by default
    asserts:
      - equal:
          path: spec.template.spec.serviceAccountName
          value: RELEASE-NAME-prom-replay-victoriametrics

  - it: should use custom serviceAccountName when provided
    set:
      victoriametrics.serviceAccount.create: false
      victoriametrics.serviceAccount.name: custom-sa
    asserts:
      - equal:
          path: spec.template.spec.serviceAccountName
          value: custom-sa
```

- [ ] **Step 2: Update RBAC tests**

Update `charts/prom-replay/tests/victoriametrics-rbac_test.yaml` to verify SA creation and annotations.

```yaml
  - it: should create ServiceAccount by default
    asserts:
      - isKind:
          of: ServiceAccount

  - it: should not create ServiceAccount when create is false
    set:
      victoriametrics.serviceAccount.create: false
    asserts:
      - notExists:
          kind: ServiceAccount

  - it: should add annotations to ServiceAccount
    set:
      victoriametrics.serviceAccount.annotations:
        eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/vm-role
    asserts:
      - equal:
          path: metadata.annotations["eks.amazonaws.com/role-arn"]
          value: arn:aws:iam::123456789012:role/vm-role
```

- [ ] **Step 3: Run unit tests**

Run: `helm unittest charts/prom-replay`
Expected: All tests PASS.

- [ ] **Step 4: Commit tests**

```bash
git add charts/prom-replay/tests/
git commit -m "test(helm): add tests for victoriametrics serviceAccount"
```

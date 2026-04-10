#!/usr/bin/env bash
# deploy-poc.sh — derives deploy values from the existing cluster release and
# prints the commands to run. Nothing is executed automatically.
#
# Iteration workflow:
#   1. Make code changes
#   2. git tag -f v0.0.0-poc.otelgateway && git push origin v0.0.0-poc.otelgateway -f
#   3. Wait for CI to publish images (check: Actions → release workflow)
#   4. Run this script and copy the Deploy section
set -euo pipefail

VERSION=0.0.1-poc.otelgateway

# ── 1. Locate the existing release ───────────────────────────────────────────
RELEASE=$(helm list -A -o json | \
  jq -r '.[] | select(.name | test("fleet-intelligence")) | .name' | head -1)
NAMESPACE=$(helm list -A -o json | \
  jq -r '.[] | select(.name | test("fleet-intelligence")) | .namespace' | head -1)

echo "Found release: ${RELEASE} in namespace: ${NAMESPACE}" >&2

# ── 2. Pull existing Helm values ──────────────────────────────────────────────
EXISTING_VALUES=$(helm get values ${RELEASE} -n ${NAMESPACE} -o json)

# otelGateway.enroll.endpoint is the full enrollment URL passed to sakauth.
# Try the gateway value first, fall back to constructing from enroll.endpoint.
GW_ENROLL_ENDPOINT=$(echo "${EXISTING_VALUES}" | jq -r '.otelGateway.enroll.endpoint // empty')
BASE_ENDPOINT=$(echo "${EXISTING_VALUES}" | jq -r '.enroll.endpoint // empty')

if [[ -n "${GW_ENROLL_ENDPOINT}" ]]; then
  ENROLL_ENDPOINT="${GW_ENROLL_ENDPOINT%/}"
elif [[ -n "${BASE_ENDPOINT}" ]]; then
  ENROLL_ENDPOINT="${BASE_ENDPOINT%/}/api/v1/health/enroll"
else
  echo "ERROR: could not determine enroll endpoint — set otelGateway.enroll.endpoint" >&2
  exit 1
fi

# BACKEND_ENDPOINT is the base URL passed to the otlphttp exporter.
# The exporter appends /metrics and /logs to this base.
GW_BACKEND_ENDPOINT=$(echo "${EXISTING_VALUES}" | jq -r '.otelGateway.backendEndpoint // empty')
if [[ -n "${GW_BACKEND_ENDPOINT}" ]]; then
  BACKEND_ENDPOINT="${GW_BACKEND_ENDPOINT%/}"
elif [[ -n "${BASE_ENDPOINT}" ]]; then
  BACKEND_ENDPOINT="${BASE_ENDPOINT%/}/api/v1/health"
else
  echo "ERROR: could not determine backend endpoint — set otelGateway.backendEndpoint" >&2
  exit 1
fi

echo "Enroll endpoint:  ${ENROLL_ENDPOINT}" >&2
echo "Backend endpoint: ${BACKEND_ENDPOINT}" >&2

# ── 3. Extract the SAK ────────────────────────────────────────────────────────
# Try four locations in order:
#   1. otelGateway.enroll.existingSecret (explicit external secret)
#   2. <release>-otel-gateway-sak         (secret created by the PoC chart)
#   3. enroll.tokenSecretName             (bare-metal agent secret)
#   4. enroll.tokenValue                  (inline value in Helm values)
TOKEN_SECRET=$(echo "${EXISTING_VALUES}" | jq -r '.enroll.tokenSecretName // empty')
TOKEN_KEY=$(echo "${EXISTING_VALUES}"    | jq -r '.enroll.tokenSecretKey // "token"')
GW_EXISTING_SECRET=$(echo "${EXISTING_VALUES}" | jq -r '.otelGateway.enroll.existingSecret // empty')
GW_SECRET="${RELEASE}-otel-gateway-sak"

SAK=""

if [[ -n "${GW_EXISTING_SECRET}" ]]; then
  echo "Looking for SAK in otelGateway.enroll.existingSecret: ${GW_EXISTING_SECRET}" >&2
  SAK=$(kubectl get secret "${GW_EXISTING_SECRET}" -n "${NAMESPACE}" \
    -o jsonpath="{.data.sak-token}" 2>/dev/null | base64 -d || true)
fi

if [[ -z "${SAK}" ]]; then
  echo "Looking for SAK in gateway secret: ${GW_SECRET}" >&2
  SAK=$(kubectl get secret "${GW_SECRET}" -n "${NAMESPACE}" \
    -o jsonpath="{.data.sak-token}" 2>/dev/null | base64 -d || true)
fi

if [[ -z "${SAK}" ]] && [[ -n "${TOKEN_SECRET}" ]]; then
  echo "Looking for SAK in enroll.tokenSecretName: ${TOKEN_SECRET}" >&2
  SAK=$(kubectl get secret "${TOKEN_SECRET}" -n "${NAMESPACE}" \
    -o jsonpath="{.data.${TOKEN_KEY}}" 2>/dev/null | base64 -d || true)
fi

if [[ -z "${SAK}" ]]; then
  SAK=$(echo "${EXISTING_VALUES}" | jq -r '.enroll.tokenValue // empty')
fi

if [[ -z "${SAK}" ]]; then
  echo "ERROR: could not find SAK in any known location — set SAK=nvapi-... manually" >&2
  exit 1
fi

echo "SAK: ${SAK:0:12}... (truncated)" >&2
echo "" >&2

# ── 4. Capture current state for rollback ────────────────────────────────────
# Read the tag from the actually running pod image — more reliable than Helm
# values since image.tag is often left empty (defaulting to chart appVersion).
CURRENT_IMAGE=$(kubectl get daemonset "${RELEASE}" -n "${NAMESPACE}" \
  -o jsonpath='{.spec.template.spec.containers[?(@.name=="fleet-intelligence-agent")].image}')
CURRENT_IMAGE_TAG="${CURRENT_IMAGE##*:}"

CURRENT_HELM_REVISION=$(helm list -A -o json | \
  jq -r --arg r "${RELEASE}" '.[] | select(.name==$r) | .revision')

# Capture current user-supplied values for explicit rollback
CURRENT_DEPLOY_ENV=$(echo "${EXISTING_VALUES}"     | jq -r '.deployEnv // "prod"')
CURRENT_OTELGW_ENABLED=$(echo "${EXISTING_VALUES}" | jq -r '.otelGateway.enabled // "false"')

# Secret created by the PoC chart — must be cleaned up on explicit rollback
POC_SECRET="${RELEASE}-otel-gateway-sak"

echo "Current running image:  ${CURRENT_IMAGE}" >&2
echo "Current image tag:      ${CURRENT_IMAGE_TAG}" >&2
echo "Current Helm revision:  ${CURRENT_HELM_REVISION}" >&2
echo "PoC secret to clean up: ${POC_SECRET}" >&2
echo "" >&2

# ── 5. Print commands ─────────────────────────────────────────────────────────
echo "# ── Verify images ───────────────────────────────────────────────────────"
echo "docker manifest inspect ghcr.io/nvidia/fleet-intelligence-agent:${VERSION}"
echo "docker manifest inspect ghcr.io/nvidia/fleetint-otelcol:${VERSION}"
echo ""
echo "# ── Deploy ──────────────────────────────────────────────────────────────"
echo "# Note: enroll.enabled is not set — in gateway mode agents need no enrollment."
echo "# imagePullPolicy=Always is set so the same PoC tag always pulls the latest image."
echo "helm upgrade --install ${RELEASE} ./deployments/helm/fleet-intelligence-agent \\"
echo "  --namespace ${NAMESPACE} \\"
echo "  --set image.tag=${VERSION} \\"
echo "  --set image.pullPolicy=Always \\"
echo "  --set otelGateway.enabled=true \\"
echo "  --set otelGateway.image.tag=${VERSION} \\"
echo "  --set otelGateway.image.pullPolicy=Always \\"
echo "  --set otelGateway.enroll.endpoint=${ENROLL_ENDPOINT} \\"
echo "  --set otelGateway.enroll.sakToken=${SAK} \\"
echo "  --set otelGateway.backendEndpoint=${BACKEND_ENDPOINT} \\"
echo "  --set deployEnv=stg"
echo ""
echo "# Helm upgrade alone won't restart pods if only the image content changed."
echo "# Force a rollout so nodes pull the new image behind the same tag."
echo "kubectl rollout restart deployment/${RELEASE}-otel-gateway -n ${NAMESPACE}"
echo "kubectl rollout restart daemonset/${RELEASE} -n ${NAMESPACE}"
echo ""
echo "# ── Verify gateway ──────────────────────────────────────────────────────"
echo "kubectl rollout status deployment/${RELEASE}-otel-gateway -n ${NAMESPACE}"
echo "kubectl logs deployment/${RELEASE}-otel-gateway -n ${NAMESPACE} --tail=50"
echo ""
echo "# ── Verify agents ───────────────────────────────────────────────────────"
echo "kubectl rollout status daemonset/${RELEASE} -n ${NAMESPACE}"
echo "POD=\$(kubectl get pods -n ${NAMESPACE} \\"
echo "  -l app.kubernetes.io/name=fleet-intelligence-agent,app.kubernetes.io/component!=otel-gateway \\"
echo "  -o jsonpath='{.items[0].metadata.name}')"
echo "kubectl logs -n ${NAMESPACE} \${POD} -c fleet-intelligence-agent --tail=30"
echo ""
echo "# ── Gateway telemetry (confirm data is flowing, zero failures) ───────────"
echo "kubectl port-forward -n ${NAMESPACE} deployment/${RELEASE}-otel-gateway 8888:8888 &"
echo "sleep 2 && curl -s http://localhost:8888/metrics | grep -E 'otelcol_exporter_sent|otelcol_exporter_send_failed'"
echo "kill %1"
echo ""
echo "# ── Rollback ─────────────────────────────────────────────────────────────"
echo "# Option A: Helm rollback to revision ${CURRENT_HELM_REVISION} (fastest)"
echo "# Helm will restore all resources to their previous state, including"
echo "# deleting the gateway Deployment, Service, ConfigMap, and SAK Secret."
echo "helm rollback ${RELEASE} ${CURRENT_HELM_REVISION} -n ${NAMESPACE}"
echo ""
echo "# Option B: Explicit re-deploy (use if Helm history was pruned)"
echo "# First delete the SAK secret created by the PoC chart, then re-deploy."
echo "kubectl delete secret ${POC_SECRET} -n ${NAMESPACE} --ignore-not-found"
echo "helm upgrade ${RELEASE} ./deployments/helm/fleet-intelligence-agent \\"
echo "  --namespace ${NAMESPACE} \\"
echo "  --set image.repository=${CURRENT_IMAGE%:*} \\"
echo "  --set image.tag=${CURRENT_IMAGE_TAG} \\"
echo "  --set otelGateway.enabled=${CURRENT_OTELGW_ENABLED} \\"
echo "  --set deployEnv=${CURRENT_DEPLOY_ENV}"

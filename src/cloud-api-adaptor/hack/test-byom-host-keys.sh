#!/usr/bin/env bash
# Ensure Homebrew binaries are available (macOS)
export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"
# test-byom-host-keys.sh
#
# Quick integration test for the byom-host-keys branch changes.
# Uses Docker containers as a stand-in for the BYOM VM pool.
#
# What this tests:
#   1. (create mode)    Chart renders a byom-ssh-host-keys Secret and mounts it
#                       when providerSecrets.byom.hostKeys is populated.
#   2. (reference mode) Chart renders the mount using an externally-supplied
#                       secret name when existingByomHostKeySecretName is set.
#   3. (no host keys)   Neither Secret nor mount appears when hostKeys is absent.
#   4. (runtime)        Docker containers act as the BYOM VM pool; the allowlist
#                       accepts known containers and rejects a rogue one.
#
# Prerequisites:
#   - helm        (>= 3.x)          install: https://helm.sh/docs/intro/install/
#   - docker      (for the runtime pool test, test 4)
#   - ssh-keygen, ssh-keyscan, ssh  (standard OpenSSH tools)
#   - python3     (stdlib yaml only, for the leak-check assertion)
#
# Usage:
#   chmod +x hack/test-byom-host-keys.sh
#   ./hack/test-byom-host-keys.sh
#
#   # skip the runtime / cluster portion (tests 1-3 only):
#   SKIP_RUNTIME=true ./hack/test-byom-host-keys.sh

set -euo pipefail

# Run from the repo root:
#   cd /path/to/cloud-api-adaptor
#   ./src/cloud-api-adaptor/hack/test-byom-host-keys.sh
CHART_DIR="${CHART_DIR:-src/cloud-api-adaptor/install/charts/peerpods}"
SKIP_RUNTIME="${SKIP_RUNTIME:-false}"
NAMESPACE="${NAMESPACE:-confidential-containers}"
POOL_SIZE="${POOL_SIZE:-2}"          # number of Docker containers acting as pool VMs
SSH_PORT_BASE="${SSH_PORT_BASE:-2220}" # host port mapped to container :22, incremented per container

# ── colours ──────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
pass() { echo -e "${GREEN}PASS${NC} $*"; }
fail() { echo -e "${RED}FAIL${NC} $*"; exit 1; }
info() { echo -e "${YELLOW}INFO${NC} $*"; }

# ── preflight ─────────────────────────────────────────────────────────────────
check_cmd() { command -v "$1" &>/dev/null || { echo "ERROR: '$1' not found – please install it first."; exit 1; }; }
check_cmd helm
check_cmd ssh-keygen
check_cmd ssh-keyscan
if [[ "$SKIP_RUNTIME" != "true" ]]; then
  check_cmd docker
  check_cmd ssh
fi

# Ensure helm sub-chart dependencies are present (they are never committed).
# For local file deps, package them; for the OCI kata-deploy, create a stub.
CHARTS_CACHE="${CHART_DIR}/charts"
mkdir -p "$CHARTS_CACHE"
if [[ ! -f "${CHARTS_CACHE}/peerpodctrl-0.4.0.tgz" ]]; then
  info "Packaging peerpodctrl sub-chart…"
  helm package src/peerpod-ctrl/chart -d "$CHARTS_CACHE" >/dev/null
fi
if [[ ! -f "${CHARTS_CACHE}/peerpods-webhook-0.4.0.tgz" ]]; then
  info "Packaging peerpods-webhook sub-chart…"
  helm package src/webhook/chart -d "$CHARTS_CACHE" >/dev/null
fi
if [[ ! -f "${CHARTS_CACHE}/kata-deploy-0.0.0-dev.tgz" ]]; then
  info "Creating kata-deploy stub sub-chart (OCI, not needed for render tests)…"
  _STUB=$(mktemp -d)
  printf 'apiVersion: v2\nname: kata-deploy\nversion: 0.0.0-dev\n' > "$_STUB/Chart.yaml"
  helm package "$_STUB" -d "$CHARTS_CACHE" >/dev/null
  rm -rf "$_STUB"
fi

# ── temp workspace ────────────────────────────────────────────────────────────
TMPDIR_WORK=$(mktemp -d)
trap 'cleanup' EXIT

cleanup() {
  info "Cleaning up…"
  # stop/remove any pool containers we started
  for cname in "${POOL_CONTAINERS[@]:-}"; do
    docker rm -f "$cname" &>/dev/null || true
  done
  rm -rf "$TMPDIR_WORK"
}

POOL_CONTAINERS=()

# ─────────────────────────────────────────────────────────────────────────────
# 1. HELM RENDER TESTS (no cluster needed)
# ─────────────────────────────────────────────────────────────────────────────
info "=== Test 1: create mode with hostKeys ==="

# Generate a throw-away key pair for the test
ssh-keygen -q -t ed25519 -N "" -f "$TMPDIR_WORK/id_rsa"

# Fake host keys
HOST_KEY_VM1="ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBFakeHostKey1== vm1"
HOST_KEY_VM2="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFakeHostKey2== vm2"

# Write a values override file so multi-line SSH key content is handled safely
# (helm --set does not cope well with newlines in key material).
cat > "$TMPDIR_WORK/values_create.yaml" <<VALEOF
provider: byom
secrets:
  mode: "create"
providerSecrets:
  byom:
    id_rsa: |
$(sed 's/^/      /' "$TMPDIR_WORK/id_rsa")
    id_rsa_pub: "$(cat "$TMPDIR_WORK/id_rsa.pub")"
    hostKeys:
      vm1_ed25519.pub: "${HOST_KEY_VM1}"
      vm2_ed25519.pub: "${HOST_KEY_VM2}"
providerConfigs:
  VM_POOL_IPS: "1.2.3.4"
  SSH_USERNAME: "peerpod"
VALEOF

helm template test-release "$CHART_DIR" \
  --values "$TMPDIR_WORK/values_create.yaml" \
  --namespace "$NAMESPACE" \
  > "$TMPDIR_WORK/render_create.yaml"

# Secret must exist
grep -q "name: byom-ssh-host-keys" "$TMPDIR_WORK/render_create.yaml" \
  && pass "byom-ssh-host-keys Secret rendered" \
  || fail "byom-ssh-host-keys Secret NOT found in create-mode render"

# Volume + volumeMount must reference the secret
grep -q "secretName: byom-ssh-host-keys" "$TMPDIR_WORK/render_create.yaml" \
  && pass "byom-ssh-host-keys volume rendered" \
  || fail "byom-ssh-host-keys volume NOT found in create-mode render"

grep -q "mountPath: /etc/byom/ssh-host-keys" "$TMPDIR_WORK/render_create.yaml" \
  && pass "byom-ssh-host-keys volumeMount rendered" \
  || fail "byom-ssh-host-keys volumeMount NOT found in create-mode render"

# SSH_HOST_KEY_ALLOWLIST_DIR must be injected automatically
grep -q "SSH_HOST_KEY_ALLOWLIST_DIR" "$TMPDIR_WORK/render_create.yaml" \
  && pass "SSH_HOST_KEY_ALLOWLIST_DIR injected in configmap" \
  || fail "SSH_HOST_KEY_ALLOWLIST_DIR NOT found in create-mode render"

# host key content must appear in the Secret
grep -q "FakeHostKey1" "$TMPDIR_WORK/render_create.yaml" \
  && pass "host key content present in Secret" \
  || fail "host key content NOT found in Secret"

# hostKeys must NOT leak into peer-pods-secret as env vars.
# Extract only the peer-pods-secret YAML block and check it has no 'hostKeys' key.
PEER_PODS_SECRET_BLOCK=$(awk '/^---/{doc=""} {doc=doc"\n"$0} /name: peer-pods-secret/{found=doc} END{print found}' "$TMPDIR_WORK/render_create.yaml")
if echo "$PEER_PODS_SECRET_BLOCK" | grep -q "^  hostKeys:"; then
  fail "hostKeys leaked into peer-pods-secret!"
else
  pass "hostKeys excluded from peer-pods-secret"
fi

# ─────────────────────────────────────────────────────────────────────────────
info "=== Test 2: reference mode ==="

cat > "$TMPDIR_WORK/values_reference.yaml" <<VALEOF
provider: byom
secrets:
  mode: "reference"
  existingSecretName: "my-byom-creds"
  existingSshKeySecretName: "my-byom-ssh"
  existingByomHostKeySecretName: "my-byom-host-keys"
providerConfigs:
  VM_POOL_IPS: "1.2.3.4"
  SSH_USERNAME: "peerpod"
VALEOF

helm template test-release "$CHART_DIR" \
  --values "$TMPDIR_WORK/values_reference.yaml" \
  --namespace "$NAMESPACE" \
  > "$TMPDIR_WORK/render_reference.yaml"

# Chart must NOT create the Secret in reference mode (it already exists)
grep "name: byom-ssh-host-keys" "$TMPDIR_WORK/render_reference.yaml" \
  && fail "byom-ssh-host-keys Secret unexpectedly created in reference mode" \
  || pass "byom-ssh-host-keys Secret not created in reference mode (correct)"

# Volume must reference the user-supplied name
grep -q "secretName: my-byom-host-keys" "$TMPDIR_WORK/render_reference.yaml" \
  && pass "reference-mode volume uses user-supplied secret name" \
  || fail "reference-mode volume does NOT use user-supplied secret name"

grep -q "SSH_HOST_KEY_ALLOWLIST_DIR" "$TMPDIR_WORK/render_reference.yaml" \
  && pass "SSH_HOST_KEY_ALLOWLIST_DIR injected in reference mode" \
  || fail "SSH_HOST_KEY_ALLOWLIST_DIR NOT found in reference-mode render"

# ─────────────────────────────────────────────────────────────────────────────
info "=== Test 3: no host keys configured ==="

cat > "$TMPDIR_WORK/values_nokeys.yaml" <<VALEOF
provider: byom
secrets:
  mode: "create"
providerSecrets:
  byom:
    id_rsa: |
$(sed 's/^/      /' "$TMPDIR_WORK/id_rsa")
    id_rsa_pub: "$(cat "$TMPDIR_WORK/id_rsa.pub")"
providerConfigs:
  VM_POOL_IPS: "1.2.3.4"
  SSH_USERNAME: "peerpod"
VALEOF

helm template test-release "$CHART_DIR" \
  --values "$TMPDIR_WORK/values_nokeys.yaml" \
  --namespace "$NAMESPACE" \
  > "$TMPDIR_WORK/render_nokeys.yaml"

grep "byom-ssh-host-keys" "$TMPDIR_WORK/render_nokeys.yaml" \
  && fail "byom-ssh-host-keys Secret/volume appeared when no hostKeys configured" \
  || pass "No byom-ssh-host-keys resource when hostKeys absent (correct)"

grep "SSH_HOST_KEY_ALLOWLIST_DIR" "$TMPDIR_WORK/render_nokeys.yaml" \
  && fail "SSH_HOST_KEY_ALLOWLIST_DIR injected when no hostKeys configured" \
  || pass "SSH_HOST_KEY_ALLOWLIST_DIR absent when hostKeys not configured (correct)"

# ─────────────────────────────────────────────────────────────────────────────
# 2. RUNTIME TEST – Docker containers as BYOM pool
#
# Uses a small inline Go program to call util.CreateSSHClient /
# util.SendFileViaSFTP — the exact code path the BYOM provider uses —
# against a real sshd container.  The key format, fingerprint hashing, and
# allowlist callback are all exercised through the production Go code, not
# through the ssh(1) CLI.
# ─────────────────────────────────────────────────────────────────────────────
if [[ "$SKIP_RUNTIME" == "true" ]]; then
  info "Skipping runtime test (SKIP_RUNTIME=true)"
  echo ""
  info "All helm-render tests passed."
  exit 0
fi

check_cmd go
check_cmd docker

info "=== Test 4: runtime allowlist test via Go provider code ==="

# Use a minimal openssh-server container.  It generates its own host keys on
# first start and accepts pubkey auth for the configured user.
CONTAINER_IMAGE="lscr.io/linuxserver/openssh-server:latest"
CNAME_POOL="byom-pool-vm-1"
CNAME_ROGUE="byom-rogue-vm"
HOST_PORT=$(( SSH_PORT_BASE ))
ROGUE_PORT=$(( SSH_PORT_BASE + 1 ))

# ── start pool container ──────────────────────────────────────────────────────
info "Starting pool container $CNAME_POOL (127.0.0.1:$HOST_PORT)…"
docker rm -f "$CNAME_POOL" &>/dev/null || true
docker run -d \
  --name "$CNAME_POOL" \
  -e "PUBLIC_KEY=$(cat "$TMPDIR_WORK/id_rsa.pub")" \
  -e USER_NAME=peerpod \
  -e PUID=1000 -e PGID=1000 \
  -p "${HOST_PORT}:2222" \
  "$CONTAINER_IMAGE" > /dev/null
POOL_CONTAINERS+=("$CNAME_POOL")

# ── start rogue container (fresh host keys, not in allowlist) ─────────────────
info "Starting rogue container $CNAME_ROGUE (127.0.0.1:$ROGUE_PORT)…"
docker rm -f "$CNAME_ROGUE" &>/dev/null || true
docker run -d \
  --name "$CNAME_ROGUE" \
  -e "PUBLIC_KEY=$(cat "$TMPDIR_WORK/id_rsa.pub")" \
  -e USER_NAME=peerpod \
  -e PUID=1000 -e PGID=1000 \
  -p "${ROGUE_PORT}:2222" \
  "$CONTAINER_IMAGE" > /dev/null
POOL_CONTAINERS+=("$CNAME_ROGUE")

# ── wait for sshd on both containers ─────────────────────────────────────────
wait_sshd() {
  local port=$1 name=$2
  info "Waiting for sshd on $name:$port…"
  for _ in $(seq 1 30); do
    if ssh-keyscan -p "$port" -T 1 127.0.0.1 2>/dev/null | grep -q "ssh-"; then
      return 0
    fi
    sleep 1
  done
  fail "sshd on $name never became ready"
}
wait_sshd "$HOST_PORT"  "$CNAME_POOL"
wait_sshd "$ROGUE_PORT" "$CNAME_ROGUE"

# ── collect pool container's host key in authorized_keys format ───────────────
# ssh-keyscan emits "ip key-type base64" — strip the leading address to get
# the bare authorized_keys format that loadAllowedKeys() expects.
ALLOWLIST_DIR="$TMPDIR_WORK/allowlist"
mkdir -p "$ALLOWLIST_DIR"
ssh-keyscan -p "$HOST_PORT" -T 3 127.0.0.1 2>/dev/null \
  | sed 's/^\[127\.0\.0\.1\]:[0-9]* //' \
  > "$ALLOWLIST_DIR/pool_vm.pub"
[[ -s "$ALLOWLIST_DIR/pool_vm.pub" ]] || fail "Failed to scan host key from pool container"
info "Collected pool host key: $(cat "$ALLOWLIST_DIR/pool_vm.pub" | awk '{print $1, substr($2,1,20)"…"}')"

# ── write inline Go integration test ─────────────────────────────────────────
# This program lives inside the cloud-providers module and calls the exact
# same util.CreateSSHClient + util.SendFileViaSFTP functions that the BYOM
# provider uses at runtime.
GO_TEST_DIR="$TMPDIR_WORK/gotest"
mkdir -p "$GO_TEST_DIR"

# Symlink into the module so we can import it without a replace directive
MODULE_ROOT="$(pwd)/src/cloud-providers"

cat > "$GO_TEST_DIR/allowlist_integration_test.go" <<GOEOF
//go:build integration

package util_test

import (
	"os"
	"testing"
	"time"

	util "github.com/confidential-containers/cloud-api-adaptor/src/cloud-providers/util"
)

// TestAllowlistIntegration_PoolAccepted verifies that util.CreateSSHClient
// with an allowlist that contains the pool container's host key can
// successfully establish an SFTP connection and write a file.
func TestAllowlistIntegration_PoolAccepted(t *testing.T) {
	privKey := os.Getenv("TEST_SSH_PRIV_KEY")
	allowlistDir := os.Getenv("TEST_ALLOWLIST_DIR")
	addr := os.Getenv("TEST_POOL_ADDR") // e.g. "127.0.0.1:2220"
	if privKey == "" || allowlistDir == "" || addr == "" {
		t.Skip("TEST_SSH_PRIV_KEY / TEST_ALLOWLIST_DIR / TEST_POOL_ADDR not set")
	}

	cfg := &util.SSHConfig{
		PrivateKey:          privKey,
		Username:            "peerpod",
		Timeout:             10 * time.Second,
		HostKeyAllowlistDir: allowlistDir,
		EnableSFTP:          true,
	}
	client, err := util.CreateSSHClient(cfg)
	if err != nil {
		t.Fatalf("CreateSSHClient failed: %v", err)
	}

	// Attempt an SFTP write — mirrors exactly what provider.go does for userdata
	err = util.SendFileViaSFTP(addr, client, "/tmp/byom-test-userdata", []byte("#cloud-config\n"))
	if err != nil {
		t.Fatalf("SendFileViaSFTP to pool container failed: %v", err)
	}
	t.Log("SFTP write to allowlisted pool container succeeded")
}

// TestAllowlistIntegration_RogueRejected verifies that a container whose
// host key is NOT in the allowlist is refused at the Go SSH handshake level.
func TestAllowlistIntegration_RogueRejected(t *testing.T) {
	privKey := os.Getenv("TEST_SSH_PRIV_KEY")
	allowlistDir := os.Getenv("TEST_ALLOWLIST_DIR")
	addr := os.Getenv("TEST_ROGUE_ADDR") // e.g. "127.0.0.1:2221"
	if privKey == "" || allowlistDir == "" || addr == "" {
		t.Skip("TEST_SSH_PRIV_KEY / TEST_ALLOWLIST_DIR / TEST_ROGUE_ADDR not set")
	}

	cfg := &util.SSHConfig{
		PrivateKey:          privKey,
		Username:            "peerpod",
		Timeout:             10 * time.Second,
		HostKeyAllowlistDir: allowlistDir,
		EnableSFTP:          true,
	}
	client, err := util.CreateSSHClient(cfg)
	if err != nil {
		t.Fatalf("CreateSSHClient failed: %v", err)
	}

	err = util.SendFileViaSFTP(addr, client, "/tmp/byom-test-userdata", []byte("#cloud-config\n"))
	if err == nil {
		t.Fatal("Expected SendFileViaSFTP to rogue container to fail, but it succeeded")
	}
	t.Logf("Rogue container correctly rejected: %v", err)
}
GOEOF

# Copy the test into the util package source tree so it can import internal symbols
cp "$GO_TEST_DIR/allowlist_integration_test.go" \
   "$MODULE_ROOT/util/allowlist_integration_test.go"

# Clean up the test file on exit (don't litter the source tree)
trap 'rm -f "$MODULE_ROOT/util/allowlist_integration_test.go"; cleanup' EXIT

# ── run the Go integration tests ──────────────────────────────────────────────
info "Running Go integration tests against Docker containers…"
(
  cd "$MODULE_ROOT"
  TEST_SSH_PRIV_KEY="$(cat "$TMPDIR_WORK/id_rsa")" \
  TEST_ALLOWLIST_DIR="$ALLOWLIST_DIR" \
  TEST_POOL_ADDR="127.0.0.1:${HOST_PORT}" \
  TEST_ROGUE_ADDR="127.0.0.1:${ROGUE_PORT}" \
  go test -v -tags integration -run "TestAllowlistIntegration" ./util/ 2>&1
) && pass "Go integration tests passed" || fail "Go integration tests FAILED"

# ─────────────────────────────────────────────────────────────────────────────
echo ""
info "All tests passed ✓"

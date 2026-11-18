#!/usr/bin/env bash

set -euo pipefail

readonly PROFILE="rfc5612-test-only-v1"
readonly CHANNEL_ID="pactway-pq3-demo"
readonly FABRIC_VERSION="3.1.5"
readonly GO_VERSION="1.27.4"
readonly DOCKER_NAMESPACE="localhost/pactway-pq3"
readonly CHAINCODE_NAME="pq3asset"
readonly CHAINCODE_VERSION="1.0"
readonly CHAINCODE_SEQUENCE="1"
readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
readonly ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel)"
readonly DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
readonly STATE_ROOT="$DATA_HOME/pactway-pq3"
readonly REPO_KEY="$(printf '%s' "$ROOT" | sha256sum | cut -c1-12)"
readonly STATE_DIR="$STATE_ROOT/$REPO_KEY"
readonly ACTIVE_FILE="$STATE_DIR/active-run"

CHAINCODE_ID=""

fail() {
	printf 'pq3: %s\n' "$*" >&2
	exit 1
}

record_duration() {
	local name="$1" started="$2"
	printf '%s_seconds=%d\n' "$name" "$((SECONDS - started))" \
		>> "$RUN_DIR/evidence/timings.txt"
}

require_isolated_profile() {
	[[ "${PACTWAY_HYBRID_PROFILE:-}" == "$PROFILE" ]] ||
		fail "set PACTWAY_HYBRID_PROFILE=$PROFILE explicitly"
	[[ "${PACTWAY_ISOLATED_DEVELOPMENT:-}" == "true" ]] ||
		fail "set PACTWAY_ISOLATED_DEVELOPMENT=true explicitly"
}

check_prerequisites() {
	command -v go >/dev/null || fail "Go $GO_VERSION is required"
	command -v make >/dev/null || fail "GNU make is required"
	command -v docker >/dev/null || fail "Docker CLI is required"
	[[ "$(go env GOVERSION)" == "go$GO_VERSION" ]] ||
		fail "Go $GO_VERSION is required; found $(go env GOVERSION)"
	docker info >/dev/null 2>&1 || fail "Docker Desktop is not ready"
	docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 is required"
}

check_linux_data_filesystem() {
	mkdir -p -- "$STATE_DIR"
	local filesystem
	filesystem="$(stat -f -c '%T' "$STATE_DIR")"
	case "$filesystem" in
		9p|drvfs|ntfs3|cifs)
			fail "private material must be stored on the WSL/Linux filesystem, not $filesystem"
			;;
	esac
}

source_tree_digest() {
	git -C "$ROOT" ls-files -co --exclude-standard -z |
		sort -z |
		xargs -0 -r sha256sum |
		sha256sum |
		cut -d ' ' -f 1
}

build_runtime_images() {
	local source_commit tree_digest revision_tag dirty source_dirty
	source_commit="$(git -C "$ROOT" rev-parse HEAD)"
	tree_digest="$(source_tree_digest)"
	revision_tag="pq3-${source_commit:0:7}-${tree_digest:0:12}"
	if [[ -n "$(git -C "$ROOT" status --porcelain --untracked-files=all)" ]]; then
		source_dirty=true
	else
		source_dirty=false
	fi

	make -C "$ROOT" peer-docker orderer-docker \
		DOCKER_NS="$DOCKER_NAMESPACE" \
		UBUNTU_VER=24.04 \
		FABRIC_VER="$FABRIC_VERSION" \
		GO_VER="$GO_VERSION" \
		EXTRA_VERSION="$revision_tag"

	IMAGE_TAG="$FABRIC_VERSION-snapshot-$revision_tag"
	docker image tag "$DOCKER_NAMESPACE/fabric-peer:$FABRIC_VERSION" \
		"$DOCKER_NAMESPACE/fabric-peer:$IMAGE_TAG"
	docker image tag "$DOCKER_NAMESPACE/fabric-orderer:$FABRIC_VERSION" \
		"$DOCKER_NAMESPACE/fabric-orderer:$IMAGE_TAG"

	mkdir -p -- "$ROOT/build/pq3"
	go -C "$ROOT" build -mod=vendor -trimpath -buildvcs=false \
		-o "$ROOT/build/pq3/chaincode" ./integration/pq3
	go -C "$ROOT" build -mod=vendor -trimpath -buildvcs=false \
		-o "$ROOT/build/pq3/osnadmin" ./cmd/osnadmin
	go -C "$ROOT" build -mod=vendor -trimpath -buildvcs=false \
		-o "$ROOT/build/pq3/pq3-verify" ./cmd/pq3-verify
	mkdir -p -- "$RUN_DIR/ccaas"
	cp -- "$ROOT/build/pq3/chaincode" "$RUN_DIR/ccaas/chaincode"
	mkdir -p -- "$RUN_DIR/tools"
	cp -- "$ROOT/build/pq3/osnadmin" "$RUN_DIR/tools/osnadmin"
	cp -- "$ROOT/build/pq3/pq3-verify" "$RUN_DIR/tools/pq3-verify"

	CC_IMAGE="$DOCKER_NAMESPACE/ccaas:$IMAGE_TAG"
	TOOLS_IMAGE="$DOCKER_NAMESPACE/tools:$IMAGE_TAG"
	PEER_IMAGE="$DOCKER_NAMESPACE/fabric-peer:$IMAGE_TAG"
	ORDERER_IMAGE="$DOCKER_NAMESPACE/fabric-orderer:$IMAGE_TAG"
	docker build --pull \
		--file "$SCRIPT_DIR/ccaas.Dockerfile" \
		--tag "$CC_IMAGE" \
		"$RUN_DIR/ccaas"
	build_tools_image

	printf '%s\n' "$IMAGE_TAG" > "$RUN_DIR/image-tag"
	printf '%s\n' "$source_commit" > "$RUN_DIR/source-commit"
	printf '%s\n' "$tree_digest" > "$RUN_DIR/source-tree-sha256"
	printf '%s\n' "$source_dirty" > "$RUN_DIR/source-dirty"
	printf '%s\n' "$CC_IMAGE" > "$RUN_DIR/ccaas-image"
	printf '%s\n' "$TOOLS_IMAGE" > "$RUN_DIR/tools-image"

	mkdir -p -- "$RUN_DIR/evidence"
	cat > "$RUN_DIR/evidence/network-manifest.json" <<EOF
{
  "source_commit": "$source_commit",
  "source_tree_sha256": "$tree_digest",
  "source_worktree_dirty": $source_dirty,
  "fabric_version": "$FABRIC_VERSION",
  "go_version": "$GO_VERSION",
  "hybrid_profile": "$PROFILE",
  "isolated_development": true,
  "channel_id": "$CHANNEL_ID",
  "channel_capability": "PactwayHybridCertificateV1",
  "tls_transport": "classical TLS",
  "docker_network_internal": true,
  "host_ports_published": false,
  "peer_image": "$PEER_IMAGE",
  "peer_image_id": "$(docker image inspect --format '{{.Id}}' "$PEER_IMAGE")",
  "orderer_image": "$ORDERER_IMAGE",
  "orderer_image_id": "$(docker image inspect --format '{{.Id}}' "$ORDERER_IMAGE")",
  "chaincode_image": "$CC_IMAGE",
  "chaincode_image_id": "$(docker image inspect --format '{{.Id}}' "$CC_IMAGE")",
  "admin_tools_image": "$TOOLS_IMAGE",
  "admin_tools_image_id": "$(docker image inspect --format '{{.Id}}' "$TOOLS_IMAGE")",
  "compose_sha256": "$(sha256sum "$SCRIPT_DIR/compose.yaml" | cut -d ' ' -f 1)",
  "orderer_config_sha256": "$(sha256sum "$ROOT/sampleconfig/orderer.yaml" | cut -d ' ' -f 1)"
}
EOF
}

build_tools_image() {
	docker build --pull \
		--file "$SCRIPT_DIR/tools.Dockerfile" \
		--tag "$TOOLS_IMAGE" \
		"$RUN_DIR/tools"
}

load_active() {
	[[ -f "$ACTIVE_FILE" ]] || fail "no active PQ3 run; use fresh first"
	local run_id
	run_id="$(<"$ACTIVE_FILE")"
	[[ "$run_id" =~ ^run-[0-9]+-[0-9]+-[0-9]+$ ]] ||
		fail "invalid active run identifier"
	RUN_ID="$run_id"
	RUN_DIR="$STATE_DIR/$RUN_ID"
	[[ -d "$RUN_DIR" && -f "$RUN_DIR/.pq3-run" ]] ||
		fail "active run directory or marker is missing"
	PROJECT="pq3-$REPO_KEY-${RUN_ID#run-}"
	grep -Fxq "project=$PROJECT" "$RUN_DIR/.pq3-run" ||
		fail "active run marker does not match its project"

	if [[ -f "$RUN_DIR/image-tag" ]]; then
		IMAGE_TAG="$(<"$RUN_DIR/image-tag")"
		[[ "$IMAGE_TAG" =~ ^[A-Za-z0-9._-]+$ ]] || fail "invalid image tag in active run"
	else
		IMAGE_TAG=""
	fi
	[[ -f "$RUN_DIR/ccaas-image" ]] && CC_IMAGE="$(<"$RUN_DIR/ccaas-image")" || CC_IMAGE=""
	[[ -f "$RUN_DIR/chaincode-id" ]] && CHAINCODE_ID="$(<"$RUN_DIR/chaincode-id")" || CHAINCODE_ID=""
	export PQ3_RUN_DIR="$RUN_DIR"
	export PQ3_MATERIAL_DIR="$RUN_DIR/material"
	export PQ3_IMAGE_TAG="$IMAGE_TAG"
	export PQ3_CHAINCODE_ID="$CHAINCODE_ID"
	COMPOSE_FILE="$RUN_DIR/config/compose.yaml"
	if [[ -f "$COMPOSE_FILE" ]]; then
		PROJECT="$PROJECT" docker compose --profile cli --project-name "$PROJECT" \
			--file "$COMPOSE_FILE" config --quiet >/dev/null ||
			fail "active Compose configuration is invalid"
	fi
}

compose() {
	[[ -f "$COMPOSE_FILE" ]] || fail "run configuration is incomplete; reset this run"
	docker compose --profile cli --project-name "$PROJECT" --file "$COMPOSE_FILE" "$@"
}

run_cli() {
	local service="$1"
	local identity="$2"
	shift 2
	compose run --rm --no-deps -e "CORE_PEER_MSPCONFIGPATH=/run/msp/$identity" "$service" "$@"
}

wait_healthy() {
	local deadline=$((SECONDS + 180))
	local services=(orderer peer-org1 peer-org2)
	[[ -n "$CHAINCODE_ID" ]] && services+=(ccaas)
	while (( SECONDS < deadline )); do
		local all_healthy=true service container_id status
		for service in "${services[@]}"; do
			container_id="$(compose ps -q "$service" 2>/dev/null || true)"
			if [[ -z "$container_id" ]]; then
				all_healthy=false
				break
			fi
			status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_id" 2>/dev/null || true)"
			[[ "$status" == "healthy" ]] || { all_healthy=false; break; }
		done
		[[ "$all_healthy" == true ]] && return 0
		sleep 2
	done
	compose ps >&2 || true
	fail "network did not become healthy within 180 seconds"
}

fresh() {
	local started=$SECONDS
	require_isolated_profile
	check_prerequisites
	check_linux_data_filesystem
	[[ ! -e "$ACTIVE_FILE" ]] || fail "an active run exists; stop it and use reset before fresh"

	RUN_ID="run-$(date -u +%Y%m%d%H%M%S)-$$-$RANDOM"
	RUN_DIR="$STATE_DIR/$RUN_ID"
	PROJECT="pq3-$REPO_KEY-${RUN_ID#run-}"
	mkdir -m 700 -- "$RUN_DIR"
	printf 'schema=1\nproject=%s\n' "$PROJECT" > "$RUN_DIR/.pq3-run"
	printf '%s\n' "$RUN_ID" > "$ACTIVE_FILE"

	if ! build_runtime_images; then
		fail "build failed; this run is retained for diagnosis and can be removed with reset"
	fi
	if ! PACTWAY_HYBRID_PROFILE="$PROFILE" PACTWAY_ISOLATED_DEVELOPMENT=true \
		go -C "$ROOT" run ./cmd/pactway-pq-gen \
			-output="$RUN_DIR/material" \
			-configtx-dir="$ROOT/sampleconfig" \
			-fabric-profile=SampleAppChannelEtcdRaft \
			-channel-id="$CHANNEL_ID" \
			-orderer-host=orderer0.ordererorg.pactway.local \
			-orderer-port=7050 \
			-hybrid-profile="$PROFILE"; then
		fail "identity/genesis generation failed; this run is retained as secret material and can be removed with reset"
	fi

	mkdir -m 700 -- "$RUN_DIR/config"
	cp -- "$SCRIPT_DIR/compose.yaml" "$RUN_DIR/config/compose.yaml"
	cp -- "$ROOT/sampleconfig/orderer.yaml" "$RUN_DIR/config/orderer.yaml"
	COMPOSE_FILE="$RUN_DIR/config/compose.yaml"
	export PQ3_RUN_DIR="$RUN_DIR"
	export PQ3_MATERIAL_DIR="$RUN_DIR/material"
	export PQ3_IMAGE_TAG="$IMAGE_TAG"
	export PQ3_CHAINCODE_ID=""
	compose up --detach --wait --wait-timeout 180 orderer peer-org1 peer-org2
	wait_healthy
	record_duration fresh_total "$started"
	printf 'PQ3 network started. Run join to activate both peers on %s.\n' "$CHANNEL_ID"
	printf 'Private material is stored under %s/material/private; do not publish or reuse it.\n' "$RUN_DIR"
}

join_channel() {
	load_active
	local started=$SECONDS
	require_isolated_profile
	wait_healthy
	compose ps

	local orderer_channels
	orderer_channels="$(compose run --rm --no-deps cli-orderer-admin channel list \
		-o orderer0.ordererorg.pactway.local:9443 \
		--ca-file /run/public/organizations/OrdererOrg/msp/tlscacerts/tls-ca.pem \
		--client-cert /run/orderer-tls/client.crt \
		--client-key /run/orderer-tls/client.key)"
	if ! grep -Fq "\"name\": \"$CHANNEL_ID\"" <<<"$orderer_channels"; then
		compose run --rm --no-deps cli-orderer-admin channel join \
			--channelID "$CHANNEL_ID" \
			--config-block /run/public/genesis.block \
			-o orderer0.ordererorg.pactway.local:9443 \
			--ca-file /run/public/organizations/OrdererOrg/msp/tlscacerts/tls-ca.pem \
			--client-cert /run/orderer-tls/client.crt \
			--client-key /run/orderer-tls/client.key
	fi
	orderer_channels="$(compose run --rm --no-deps cli-orderer-admin channel list \
		-o orderer0.ordererorg.pactway.local:9443 \
		--ca-file /run/public/organizations/OrdererOrg/msp/tlscacerts/tls-ca.pem \
		--client-cert /run/orderer-tls/client.crt \
		--client-key /run/orderer-tls/client.key)"
	grep -Fq "\"name\": \"$CHANNEL_ID\"" <<<"$orderer_channels" ||
		fail "orderer did not report membership in $CHANNEL_ID after join"

	for service in cli-org1 cli-org2; do
		local output
		output="$(run_cli "$service" admin peer channel list)"
		if ! grep -Fq "$CHANNEL_ID" <<<"$output"; then
			run_cli "$service" admin peer channel join -b /run/public/genesis.block
			output="$(run_cli "$service" admin peer channel list)"
		fi
		grep -Fq "$CHANNEL_ID" <<<"$output" ||
			fail "$service did not report membership in $CHANNEL_ID"
	done
	printf '%s\n' "$orderer_channels" > "$RUN_DIR/evidence/orderer-channels.json"
	record_duration channel_join "$started"
	printf 'Both peers report membership in %s.\n' "$CHANNEL_ID"
}

deploy_chaincode() {
	load_active
	local started=$SECONDS
	require_isolated_profile
	wait_healthy
	run_cli cli-org1 admin peer channel list | grep -Fq "$CHANNEL_ID" ||
		fail "Org1 peer has not joined $CHANNEL_ID; run join first"
	run_cli cli-org2 admin peer channel list | grep -Fq "$CHANNEL_ID" ||
		fail "Org2 peer has not joined $CHANNEL_ID; run join first"

	local package_dir="$RUN_DIR/package"
	local public_dir="$PQ3_MATERIAL_DIR/public"
	mkdir -m 700 -p -- "$package_dir/code"
	mkdir -p -- "$public_dir"
	cat > "$package_dir/metadata.json" <<EOF
{"type":"ccaas","label":"pq3asset_1.0"}
EOF
	cat > "$package_dir/code/connection.json" <<'EOF'
{
  "address": "pq3-chaincode:9999",
  "dial_timeout": "10s",
  "tls_required": false
}
EOF
	tar -czf "$package_dir/code.tar.gz" -C "$package_dir/code" connection.json
	tar -czf "$public_dir/pq3asset.tar.gz" -C "$package_dir" metadata.json code.tar.gz

	local package_id installed
	package_id="$(run_cli cli-org1 admin peer lifecycle chaincode calculatepackageid \
		/run/public/pq3asset.tar.gz)"
	[[ "$package_id" == pq3asset_1.0:* ]] ||
		fail "unexpected CCAAS package identifier: $package_id"
	for service in cli-org1 cli-org2; do
		installed="$(run_cli "$service" admin peer lifecycle chaincode queryinstalled)"
		if ! grep -Fq "$package_id" <<<"$installed"; then
			run_cli "$service" admin peer lifecycle chaincode install /run/public/pq3asset.tar.gz
		fi
	done

	local policy="AND('Org1MSP.peer','Org2MSP.peer')"
	for service in cli-org1 cli-org2; do
		run_cli "$service" admin peer lifecycle chaincode approveformyorg \
			-o orderer0.ordererorg.pactway.local:7050 --tls --cafile /run/public/organizations/OrdererOrg/msp/tlscacerts/tls-ca.pem \
			--channelID "$CHANNEL_ID" --name "$CHAINCODE_NAME" \
			--version "$CHAINCODE_VERSION" --package-id "$package_id" \
			--sequence "$CHAINCODE_SEQUENCE" --signature-policy "$policy"
	done
	run_cli cli-org1 admin peer lifecycle chaincode commit \
		-o orderer0.ordererorg.pactway.local:7050 --tls --cafile /run/public/organizations/OrdererOrg/msp/tlscacerts/tls-ca.pem \
		--channelID "$CHANNEL_ID" --name "$CHAINCODE_NAME" \
		--version "$CHAINCODE_VERSION" --sequence "$CHAINCODE_SEQUENCE" \
		--signature-policy "$policy" \
		--peerAddresses peer0.org1.pactway.local:7051 \
		--tlsRootCertFiles /run/public/organizations/Org1/msp/tlscacerts/tls-ca.pem \
		--peerAddresses peer0.org2.pactway.local:8051 \
		--tlsRootCertFiles /run/public/organizations/Org2/msp/tlscacerts/tls-ca.pem

	local committed
	committed="$(run_cli cli-org1 admin peer lifecycle chaincode querycommitted \
		--channelID "$CHANNEL_ID" --name "$CHAINCODE_NAME")"
	grep -Fq "Version: $CHAINCODE_VERSION" <<<"$committed" ||
		fail "committed definition did not report version $CHAINCODE_VERSION"
	grep -Fq "Sequence: $CHAINCODE_SEQUENCE" <<<"$committed" ||
		fail "committed definition did not report sequence $CHAINCODE_SEQUENCE"

	printf '%s\n' "$package_id" > "$RUN_DIR/chaincode-id"
	CHAINCODE_ID="$package_id"
	export PQ3_CHAINCODE_ID="$CHAINCODE_ID"
	compose up --detach --wait --wait-timeout 60 ccaas
	wait_healthy
	printf '%s\n' "$committed" > "$RUN_DIR/evidence/chaincode-definition.txt"
	record_duration chaincode_deploy "$started"
	printf 'Committed %s v%s with endorsement policy %s.\n' \
		"$CHAINCODE_NAME" "$CHAINCODE_VERSION" "$policy"
}

invoke_asset() {
	load_active
	local started=$SECONDS
	[[ -n "$CHAINCODE_ID" ]] || fail "chaincode is not deployed; run deploy first"
	require_isolated_profile
	wait_healthy

	local orderer_tls=/run/public/organizations/OrdererOrg/msp/tlscacerts/tls-ca.pem
	local org1_tls=/run/public/organizations/Org1/msp/tlscacerts/tls-ca.pem
	local org2_tls=/run/public/organizations/Org2/msp/tlscacerts/tls-ca.pem
	local args='{"function":"InitAsset","Args":["pq3-asset","initial-value"]}'
	run_cli cli-org1 client peer chaincode invoke \
		-o orderer0.ordererorg.pactway.local:7050 --tls --cafile "$orderer_tls" \
		-C "$CHANNEL_ID" -n "$CHAINCODE_NAME" -c "$args" \
		--peerAddresses peer0.org1.pactway.local:7051 --tlsRootCertFiles "$org1_tls" \
		--peerAddresses peer0.org2.pactway.local:8051 --tlsRootCertFiles "$org2_tls" \
		--waitForEvent --waitForEventTimeout 60s
	local org1_value org2_value
	org1_value="$(run_cli cli-org1 client peer chaincode query \
		-C "$CHANNEL_ID" -n "$CHAINCODE_NAME" \
		-c '{"function":"Get","Args":["pq3-asset"]}')"
	org2_value="$(run_cli cli-org2 client peer chaincode query \
		-C "$CHANNEL_ID" -n "$CHAINCODE_NAME" \
		-c '{"function":"Get","Args":["pq3-asset"]}')"
	[[ "$org1_value" == *initial-value* ]] ||
		fail "Org1 query returned unexpected asset value: $org1_value"
	[[ "$org2_value" == *initial-value* ]] ||
		fail "Org2 query returned unexpected asset value: $org2_value"
	printf 'Both peers read the committed asset value: %s\n' "$org1_value"
	verify_evidence
	record_duration invoke_and_verify "$started"
}

reject_missing_endorsement() {
	load_active
	[[ -n "$CHAINCODE_ID" ]] || fail "chaincode is not deployed; run deploy first"
	require_isolated_profile
	wait_healthy

	local before after invoke_output invoke_status verification_output
	before="$(run_cli cli-org1 client peer chaincode query \
		-C "$CHANNEL_ID" -n "$CHAINCODE_NAME" \
		-c '{"function":"Get","Args":["pq3-asset"]}')"
	if invoke_output="$(run_cli cli-org1 client peer chaincode invoke \
		-o orderer0.ordererorg.pactway.local:7050 --tls \
		--cafile /run/public/organizations/OrdererOrg/msp/tlscacerts/tls-ca.pem \
		-C "$CHANNEL_ID" -n "$CHAINCODE_NAME" \
		-c '{"function":"Set","Args":["pq3-asset","must-not-commit"]}' \
		--peerAddresses peer0.org1.pactway.local:7051 \
		--tlsRootCertFiles /run/public/organizations/Org1/msp/tlscacerts/tls-ca.pem \
		--waitForEvent --waitForEventTimeout 60s 2>&1)"; then
		invoke_status=0
	else
		invoke_status=$?
	fi
	after="$(run_cli cli-org1 client peer chaincode query \
		-C "$CHANNEL_ID" -n "$CHAINCODE_NAME" \
		-c '{"function":"Get","Args":["pq3-asset"]}')"
	[[ "$before" == "$after" ]] ||
		fail "single-organization endorsement changed committed state: before=$before after=$after"
	if verification_output="$(verify_evidence missing-endorsement-verification.txt 2>&1)"; then
		fail "evidence verifier accepted a transaction missing Org2MSP endorsement"
	fi
	grep -Fq "missing endorsement from Org2MSP" <<<"$verification_output" ||
		fail "missing-endorsement evidence failed for an unexpected reason: $verification_output"

	{
		printf 'single_endorser_invoke_exit_code=%s\n' "$invoke_status"
		printf 'single_endorser_invoke_output=%s\n' "$invoke_output"
		printf 'state_before=%s\n' "$before"
		printf 'state_after=%s\n' "$after"
		printf 'verifier_rejection=%s\n' "$verification_output"
	} > "$RUN_DIR/evidence/missing-endorsement.txt"
	printf 'Rejected single-org endorsement; both the state check and byte-level verifier confirmed Org2MSP was required.\n'
}

assert_persisted_network_state() {
	local stage="$1" channels value
	wait_healthy
	for service in cli-org1 cli-org2; do
		channels="$(run_cli "$service" admin peer channel list)"
		grep -Fq "$CHANNEL_ID" <<<"$channels" ||
			fail "$service lost $CHANNEL_ID after $stage"
	done
	local orderer_channels
	orderer_channels="$(compose run --rm --no-deps cli-orderer-admin channel list \
		-o orderer0.ordererorg.pactway.local:9443 \
		--ca-file /run/public/organizations/OrdererOrg/msp/tlscacerts/tls-ca.pem \
		--client-cert /run/orderer-tls/client.crt \
		--client-key /run/orderer-tls/client.key)"
	grep -Fq "\"name\": \"$CHANNEL_ID\"" <<<"$orderer_channels" ||
		fail "orderer lost $CHANNEL_ID after $stage"
	for service in cli-org1 cli-org2; do
		value="$(run_cli "$service" client peer chaincode query \
			-C "$CHANNEL_ID" -n "$CHAINCODE_NAME" \
			-c '{"function":"Get","Args":["pq3-asset"]}')"
		[[ "$value" == *initial-value* ]] ||
			fail "$service returned unexpected state after $stage: $value"
	done
	printf '%s: channel active and state preserved (%s)\n' "$stage" "$value"
}

exercise_recovery() {
	load_active
	local started=$SECONDS
	require_isolated_profile
	[[ -n "$CHAINCODE_ID" ]] || fail "chaincode is not deployed; run deploy first"
	assert_persisted_network_state "before lifecycle cycle"
	stop_network
	start_network
	assert_persisted_network_state "start"
	restart_network
	assert_persisted_network_state "restart"
	resume_network
	assert_persisted_network_state "resume"
	{
		printf 'run_id=%s\n' "$RUN_ID"
		printf 'channel_id=%s\n' "$CHANNEL_ID"
		printf 'chaincode_id=%s\n' "$CHAINCODE_ID"
		printf 'lifecycle=stop,start,restart,resume\n'
		printf 'both_peer_state=initial-value\n'
	} > "$RUN_DIR/evidence/recovery.txt"
	record_duration recovery_cycle "$started"
}

verify_evidence() {
	load_active
	require_isolated_profile
	[[ -n "$CHAINCODE_ID" ]] || fail "chaincode is not deployed; run deploy first"
	local report_name="${1:-transaction-verification.json}"
	[[ "$report_name" =~ ^[A-Za-z0-9._-]+$ ]] || fail "invalid evidence report name"
	mkdir -p -- "$RUN_DIR/evidence"
	go -C "$ROOT" build -mod=vendor -trimpath -buildvcs=false \
		-o "$ROOT/build/pq3/pq3-verify" ./cmd/pq3-verify
	cp -- "$ROOT/build/pq3/pq3-verify" "$RUN_DIR/tools/pq3-verify"
	TOOLS_IMAGE="$DOCKER_NAMESPACE/tools:$IMAGE_TAG"
	build_tools_image
	compose run --rm --no-deps \
		--volume "$RUN_DIR/evidence:/run/evidence" \
		-e CORE_PEER_MSPCONFIGPATH=/run/msp/admin cli-org1 \
		peer channel fetch newest /run/evidence/latest.block \
		-c "$CHANNEL_ID" -o orderer0.ordererorg.pactway.local:7050 \
		--tls --cafile /run/public/organizations/OrdererOrg/msp/tlscacerts/tls-ca.pem
	compose run --rm --no-deps \
		--volume "$RUN_DIR/evidence:/run/evidence" \
		-e PACTWAY_HYBRID_PROFILE="$PROFILE" \
		-e PACTWAY_ISOLATED_DEVELOPMENT=true \
		--entrypoint /usr/local/bin/pq3-verify cli-orderer-admin \
		--config-block /run/public/genesis.block \
		--block /run/evidence/latest.block \
		--require-chaincode "$CHAINCODE_NAME" \
		--require-orgs Org1MSP,Org2MSP |
		tee "$RUN_DIR/evidence/$report_name"
}

show_status() {
	load_active
	compose ps
}

stop_network() {
	load_active
	compose stop
}

start_network() {
	load_active
	compose start
	wait_healthy
}

restart_network() {
	load_active
	compose restart
	wait_healthy
}

resume_network() {
	load_active
	local services=(orderer peer-org1 peer-org2)
	[[ -n "$CHAINCODE_ID" ]] && services+=(ccaas)
	compose up --detach --wait --wait-timeout 180 --pull never "${services[@]}"
	wait_healthy
}

reset_network() {
	load_active
	[[ "$RUN_DIR" == "$STATE_DIR/$RUN_ID" && "$RUN_DIR" != "$STATE_ROOT" ]] ||
		fail "refusing to reset a path outside the PQ3 run directory"
	[[ "$(dirname -- "$RUN_DIR")" == "$STATE_DIR" ]] ||
		fail "refusing to reset a non-child PQ3 run directory"
	if [[ -f "$COMPOSE_FILE" && -n "$IMAGE_TAG" ]]; then
		compose down --volumes --remove-orphans
	fi
	rm -rf -- "$RUN_DIR"
	rm -f -- "$ACTIVE_FILE"
	printf 'Reset only PQ3 run %s and its Compose project volumes.\n' "$RUN_ID"
}

usage() {
	cat <<'EOF'
Usage: integration/pq3/pq3.sh COMMAND

Commands:
  fresh    Build source-pinned local images, generate fresh keys/genesis, and start nodes
  join     Join Org1 and Org2 peers to the generated application channel
  deploy   Install, approve, and commit the CCAAS chaincode with two-org policy
  invoke   Submit a two-peer-endorsed transaction and verify state at both peers
  negative-endorsement  Verify a single-org endorsement cannot update state
  recovery   Exercise stop/start/restart/resume while preserving ledger state
  verify   Fetch and cryptographically verify the latest transaction block
  health   Show node health and wait for node health checks
  status   Show Compose service status
  stop     Stop this run without deleting credentials, configuration, or ledgers
  start    Start stopped containers without regenerating identities or genesis
  restart  Restart containers while preserving credentials and ledgers
  resume   Reconcile/start this run from its saved Compose configuration
  reset    Remove only this run's containers, network, volumes, and generated material
EOF
}

command="${1:-}"
case "$command" in
	fresh)
		fresh
		;;
	join)
		shift
		join_channel
		;;
	deploy)
		deploy_chaincode
		;;
	invoke)
		shift
		invoke_asset
		;;
	negative-endorsement)
		reject_missing_endorsement
		;;
	recovery)
		exercise_recovery
		;;
	verify)
		shift
		verify_evidence
		;;
	health)
		shift
		load_active
		wait_healthy
		compose ps
		;;
	status)
		show_status
		;;
	stop)
		stop_network
		;;
	start)
		start_network
		;;
	restart)
		restart_network
		;;
	resume)
		resume_network
		;;
	reset)
		reset_network
		;;
	*)
		usage
		[[ -z "$command" ]] || exit 2
		;;
esac

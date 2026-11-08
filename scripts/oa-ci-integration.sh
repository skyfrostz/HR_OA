#!/usr/bin/env bash
set -euo pipefail
set +x
umask 077
cd "$(dirname "${BASH_SOURCE[0]}")/.."

project="hr-oa-ci-$(openssl rand -hex 6)"
export OA_CI_DB_PASSWORD OA_CI_DB_ROOT_PASSWORD OA_CI_JWT_SECRET OA_CI_SEED_PASSWORD
OA_CI_DB_PASSWORD="$(openssl rand -hex 24)"
OA_CI_DB_ROOT_PASSWORD="$(openssl rand -hex 24)"
OA_CI_JWT_SECRET="$(openssl rand -hex 32)"
OA_CI_SEED_PASSWORD="$(openssl rand -hex 20)"
export OA_CI_API_PORT=""
compose_files=(--file deploy/oa/integration.compose.yaml)
if [[ "${1:-}" == --mock-providers ]]; then
    shift
    read -r OA_CI_API_PORT OA_CI_MOCK_PORT OA_CI_WEB_PORT < <(python3 - <<'PY'
import socket
with socket.socket() as api, socket.socket() as mock, socket.socket() as web:
    for server in (api, mock, web):
        server.bind(("127.0.0.1", 0))
    print(api.getsockname()[1], mock.getsockname()[1], web.getsockname()[1])
PY
    )
    export OA_CI_MOCK_PORT OA_CI_WEB_PORT OA_MOCK_WECOM_SECRET OA_MOCK_OSS_SECRET OA_MOCK_CONTROL
    OA_MOCK_WECOM_SECRET="$(openssl rand -hex 20)"
    OA_MOCK_OSS_SECRET="$(openssl rand -hex 20)"
    OA_MOCK_CONTROL="$(openssl rand -hex 20)"
    compose_files+=(--file deploy/oa/integration-mock.compose.yaml)
fi
if [[ $# -ne 0 ]]; then
    printf '%s\n' 'Usage: oa-ci-integration.sh [--mock-providers]' >&2
    exit 2
fi

compose() {
    docker compose --env-file /dev/null --project-name "$project" \
        "${compose_files[@]}" "$@"
}

cleanup() {
    local status=$?
    trap - EXIT
    if [[ "$status" -ne 0 ]]; then
        compose ps >&2 || true
        compose logs --no-color --tail 50 api | rg -o 'OA initialization failed|OA configuration invalid|invalid OA_[A-Z_]+|stage=[a-z-]+|panic: [A-Za-z /_-]+|[A-Za-z0-9./_-]+\.go:[0-9]+' >&2 || true
    fi
    if ! compose down >&2; then
        exit 1
    fi
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
compose config --quiet
if [[ -n "${OA_CI_PINNED_API_IMAGE:-}" || -n "${OA_CI_PINNED_WEB_IMAGE:-}" ]]; then
    [[ -n "$OA_CI_API_PORT" && "${OA_CI_PINNED_API_IMAGE:-}" =~ ^sha256:[0-9a-f]{64}$ \
        && "${OA_CI_PINNED_WEB_IMAGE:-}" =~ ^sha256:[0-9a-f]{64}$ ]]
    docker image tag "$OA_CI_PINNED_API_IMAGE" "$project-api:latest"
    docker image tag "$OA_CI_PINNED_WEB_IMAGE" "$project-web:latest"
    compose up -d --no-build --wait --wait-timeout 180
    for service in api web; do
        container_id="$(compose ps -q "$service")"
        actual_image="$(docker inspect --format '{{.Image}}' "$container_id")"
        if [[ "$service" == api ]]; then
            [[ "$actual_image" == "$OA_CI_PINNED_API_IMAGE" ]]
        else
            [[ "$actual_image" == "$OA_CI_PINNED_WEB_IMAGE" ]]
        fi
    done
else
    compose up -d --build --wait --wait-timeout 180
fi
address="$(compose port api 8888)"
if [[ ! "$address" =~ ^127\.0\.0\.1:[0-9]+$ ]]; then
    printf '%s\n' 'CI API is not bound to an isolated loopback port.' >&2
    exit 2
fi
export OA_SEED_PASSWORD="$OA_CI_SEED_PASSWORD"
export OA_CI_PROJECT="$project"
browser_args=()
if [[ -n "${OA_CI_BROWSER_ARTIFACTS:-}" ]]; then
    browser_args+=(--artifacts "$OA_CI_BROWSER_ARTIFACTS")
fi
run_org_browser() {
    local web="$1"
    local org_args=()
    if [[ -n "${OA_CI_ORG_CONTRACT_CASE:-}" ]]; then
        org_args+=(--case "$OA_CI_ORG_CONTRACT_CASE")
    fi
    "$OA_CI_BROWSER_PYTHON" -B tests/oa_org_contract_browser.py --base "http://$web" \
        --runtime "${OA_CI_ORG_CONTRACT_RUNTIME:?Provide the source digest under test}" \
        --artifacts "${OA_CI_ORG_CONTRACT_ARTIFACTS:?Provide a local artifact directory}" --prepare-isolated \
        ${org_args[@]+"${org_args[@]}"}
}
if [[ "${OA_CI_BROWSER_ONLY:-0}" == 1 ]]; then
    : "${OA_CI_BROWSER_PYTHON:?Provide a Playwright Python interpreter}"
    : "${OA_CI_WEB_PORT:?Use --mock-providers for the isolated browser project}"
    if [[ -n "${OA_CI_ORG_CONTRACT_ARTIFACTS:-}" ]]; then
        run_org_browser "127.0.0.1:$OA_CI_WEB_PORT"
        exit 0
    fi
    "$OA_CI_BROWSER_PYTHON" -B tests/oa_m6_browser.py --base "http://127.0.0.1:$OA_CI_WEB_PORT" --mock --prepare-mock \
        --section "${OA_CI_BROWSER_SECTION:-all}" ${browser_args[@]+"${browser_args[@]}"}
    exit 0
fi
python3 -B tests/oa_ci_integration.py --base "http://$address"
if [[ -n "$OA_CI_API_PORT" ]]; then
    [[ "$address" == "127.0.0.1:$OA_CI_API_PORT" ]]
    web="$(compose port web 8080)"
    [[ "$web" == "127.0.0.1:$OA_CI_WEB_PORT" ]]
    python3 -B tests/oa_m6_providers.py --base "http://$web" --mock "http://127.0.0.1:$OA_CI_MOCK_PORT"
    if [[ -n "${OA_CI_BROWSER_PYTHON:-}" ]]; then
        browser_status=0
        "$OA_CI_BROWSER_PYTHON" -B tests/oa_m6_browser.py --base "http://$web" --mock ${browser_args[@]+"${browser_args[@]}"} || browser_status=$?
        if [[ -n "${OA_CI_UI_UX_ARTIFACTS:-}" ]]; then
            "$OA_CI_BROWSER_PYTHON" -B tests/oa_ui_ux_regression.py --base "http://$web" --artifacts "$OA_CI_UI_UX_ARTIFACTS" || browser_status=$?
        fi
        if [[ -n "${OA_CI_ORG_CONTRACT_ARTIFACTS:-}" ]]; then
            run_org_browser "$web" || browser_status=$?
        fi
        if (( browser_status )); then
            exit "$browser_status"
        fi
    fi
fi

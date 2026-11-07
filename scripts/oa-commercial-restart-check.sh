#!/usr/bin/env bash
set -euo pipefail
set +x
cd "$(dirname "${BASH_SOURCE[0]}")/.."

restart_process() {
    local service="$1" id before count health
    id="$(docker compose ps -q "$service")"
    [[ -n "$id" ]]
    [[ "$(docker inspect --format '{{.HostConfig.RestartPolicy.Name}}' "$id")" == "unless-stopped" ]]
    before="$(docker inspect --format '{{.RestartCount}}' "$id")"
    # Signal the process from inside its exact local container. A Docker stop
    # would mark it manually stopped and bypass the policy under test.
    docker exec "$id" sh -c 'kill -TERM 1' || true
    for ((attempt = 0; attempt < 60; attempt++)); do
        read -r count health <<< "$(docker inspect --format '{{.RestartCount}} {{.State.Health.Status}}' "$id")"
        if [[ "$count" -gt "$before" && "$health" == healthy ]]; then
            [[ "$(docker compose ps -q "$service")" == "$id" ]]
            printf 'PASS %s automatically restarted (count %s to %s) and is healthy\n' "$service" "$before" "$count"
            return 0
        fi
        sleep 1
    done
    printf 'FAIL %s did not recover via Docker restart policy\n' "$service" >&2
    docker compose start --wait --wait-timeout 120 "$service" >&2
    return 1
}

for service in api web; do
    restart_process "$service"
done
for path in oa.html h5.html api/oa/v1/health; do
    curl --noproxy '*' --fail --silent --output /dev/null "http://127.0.0.1:8080/$path"
done
set -a
source ./.env
set +a
python3 -B tests/oa_api.py verify --state tests/results/oa-state.json

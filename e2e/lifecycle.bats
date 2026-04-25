#!/usr/bin/env bats

load setup.bash

@test "replay manager is healthy" {
    result=$(api_get /healthz)
    [ "$result" = "ok" ]
}

@test "inject test metrics into VictoriaMetrics" {
    run inject_test_metrics
    [ "$status" -eq 0 ]
}

@test "verify test metrics exist in VM" {
    sleep 2
    result=$(curl -sf "$(vm_url)/api/v1/query?query=e2e_test_gauge" | jq -r '.status')
    [ "$result" = "success" ]

    count=$(curl -sf "$(vm_url)/api/v1/query?query=count(e2e_test_gauge)" | jq -r '.data.result[0].value[1]')
    [ "$count" -ge 1 ]
}

@test "create a run via export" {
    now=$(date +%s)
    past=$((now - 600))
    start=$(date -u -d "@$past" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -r "$past" +%Y-%m-%dT%H:%M:%SZ)
    end=$(date -u -d "@$now" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -r "$now" +%Y-%m-%dT%H:%M:%SZ)

    result=$(api_post /runs -d "{\"start\":\"${start}\",\"end\":\"${end}\",\"labels\":{\"test\":\"e2e\"}}")
    echo "$result"

    run_id=$(echo "$result" | jq -r '.run_id')
    [ -n "$run_id" ]
    [ "$run_id" != "null" ]

    # Save run_id for subsequent tests
    echo "$run_id" > /tmp/prom-replay-e2e-run-id
}

@test "list runs shows the created run" {
    run_id=$(cat /tmp/prom-replay-e2e-run-id)

    result=$(api_get /runs)
    echo "$result"

    found=$(echo "$result" | jq -r ".[] | select(.run_id == \"$run_id\") | .run_id")
    [ "$found" = "$run_id" ]

    loaded=$(echo "$result" | jq -r ".[] | select(.run_id == \"$run_id\") | .loaded")
    [ "$loaded" = "false" ]
}

@test "load the run into VictoriaMetrics" {
    run_id=$(cat /tmp/prom-replay-e2e-run-id)

    result=$(api_post "/runs/${run_id}/load")
    echo "$result"

    status_val=$(echo "$result" | jq -r '.status')
    [ "$status_val" = "loaded" ]
}

@test "verify run_id label exists in VM after load" {
    run_id=$(cat /tmp/prom-replay-e2e-run-id)
    sleep 2

    result=$(curl -sf "$(vm_url)/api/v1/label/run_id/values")
    echo "$result"

    found=$(echo "$result" | jq -r ".data[] | select(. == \"$run_id\")")
    [ "$found" = "$run_id" ]
}

@test "query loaded metrics by run_id" {
    run_id=$(cat /tmp/prom-replay-e2e-run-id)

    result=$(curl -sf "$(vm_url)/api/v1/query?query=e2e_test_gauge%7Brun_id%3D%22${run_id}%22%7D")
    echo "$result"

    count=$(echo "$result" | jq -r '.data.result | length')
    [ "$count" -ge 1 ]
}

@test "list runs shows loaded=true" {
    run_id=$(cat /tmp/prom-replay-e2e-run-id)

    result=$(api_get /runs)
    loaded=$(echo "$result" | jq -r ".[] | select(.run_id == \"$run_id\") | .loaded")
    [ "$loaded" = "true" ]
}

@test "loading again is idempotent" {
    run_id=$(cat /tmp/prom-replay-e2e-run-id)

    result=$(api_post "/runs/${run_id}/load")
    echo "$result"

    status_val=$(echo "$result" | jq -r '.status')
    [ "$status_val" = "already loaded" ]
}

@test "unload the run from VictoriaMetrics" {
    run_id=$(cat /tmp/prom-replay-e2e-run-id)

    result=$(api_delete "/runs/${run_id}/load")
    echo "$result"

    status_val=$(echo "$result" | jq -r '.status')
    [ "$status_val" = "unloaded" ]
}

@test "run_id no longer queryable after unload" {
    run_id=$(cat /tmp/prom-replay-e2e-run-id)
    sleep 2

    result=$(curl -sf "$(vm_url)/api/v1/query?query=e2e_test_gauge%7Brun_id%3D%22${run_id}%22%7D")
    count=$(echo "$result" | jq -r '.data.result | length')
    [ "$count" -eq 0 ]
}

@test "run still exists in MinIO after unload" {
    run_id=$(cat /tmp/prom-replay-e2e-run-id)

    result=$(api_get /runs)
    found=$(echo "$result" | jq -r ".[] | select(.run_id == \"$run_id\") | .run_id")
    [ "$found" = "$run_id" ]

    loaded=$(echo "$result" | jq -r ".[] | select(.run_id == \"$run_id\") | .loaded")
    [ "$loaded" = "false" ]
}

@test "delete the run entirely" {
    run_id=$(cat /tmp/prom-replay-e2e-run-id)

    result=$(api_delete "/runs/${run_id}")
    echo "$result"

    status_val=$(echo "$result" | jq -r '.status')
    [ "$status_val" = "deleted" ]
}

@test "run no longer appears in list after delete" {
    run_id=$(cat /tmp/prom-replay-e2e-run-id)

    result=$(api_get /runs)
    found=$(echo "$result" | jq -r ".[] | select(.run_id == \"$run_id\") | .run_id")
    [ -z "$found" ]

    rm -f /tmp/prom-replay-e2e-run-id
}

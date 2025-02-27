load helpers_pushpull

function setup_file() {
    # Verify prerequisites are available
    if ! verify_prerequisites; then
        exit 1
    fi
    # Download test data to folder common for the entire suite, not just this file
    skopeo --insecure-policy copy --format=oci docker://ghcr.io/project-zot/golang:1.20 oci:${TEST_DATA_DIR}/golang:1.20
    # Setup zot server
    local zot_root_dir=${BATS_FILE_TMPDIR}/zot
    local zot_config_file=${BATS_FILE_TMPDIR}/zot_config.json
    local oci_data_dir=${BATS_FILE_TMPDIR}/oci
    mkdir -p ${zot_root_dir}
    mkdir -p ${oci_data_dir}
    cat > ${zot_config_file}<<EOF
{
    "distSpecVersion": "1.1.0",
    "storage": {
        "rootDirectory": "${zot_root_dir}",
        "dedupe": false,
        "gc": true,
        "gcInterval": "30s"
    },
    "http": {
        "address": "0.0.0.0",
        "port": "8080"
    },
    "log": {
        "level": "debug"
    }
}
EOF
    git -C ${BATS_FILE_TMPDIR} clone https://github.com/project-zot/helm-charts.git
    setup_zot_file_level ${zot_config_file}
    wait_zot_reachable "http://127.0.0.1:8080/v2/_catalog"
}

function teardown_file() {
    local zot_root_dir=${BATS_FILE_TMPDIR}/zot
    local oci_data_dir=${BATS_FILE_TMPDIR}/oci
    teardown_zot_file_level
    rm -rf ${zot_root_dir}
    rm -rf ${oci_data_dir}
}

@test "push image - dedupe not running" {
    start=`date +%s`
    run skopeo --insecure-policy copy --dest-tls-verify=false \
        oci:${TEST_DATA_DIR}/golang:1.20 \
        docker://127.0.0.1:8080/golang:1.20
    [ "$status" -eq 0 ]
    end=`date +%s`

    runtime=$((end-start))
    echo "push image exec time: $runtime sec" >&3

    run curl http://127.0.0.1:8080/v2/_catalog
    [ "$status" -eq 0 ]
    [ $(echo "${lines[-1]}" | jq '.repositories[]') = '"golang"' ]
    run curl http://127.0.0.1:8080/v2/golang/tags/list
    [ "$status" -eq 0 ]
    [ $(echo "${lines[-1]}" | jq '.tags[]') = '"1.20"' ]
}

@test "pull image - dedupe not running" {
    local oci_data_dir=${BATS_FILE_TMPDIR}/oci
    start=`date +%s`
    run skopeo --insecure-policy copy --src-tls-verify=false \
        docker://127.0.0.1:8080/golang:1.20 \
        oci:${oci_data_dir}/golang:1.20
    [ "$status" -eq 0 ]
    end=`date +%s`

    runtime=$((end-start))
    echo "pull image exec time: $runtime sec" >&3
    run cat ${BATS_FILE_TMPDIR}/oci/golang/index.json
    [ "$status" -eq 0 ]
    [ $(echo "${lines[-1]}" | jq '.manifests[].annotations."org.opencontainers.image.ref.name"') = '"1.20"' ]
}

@test "push 50 images with dedupe disabled" {
    for i in {1..50}
    do
        run skopeo --insecure-policy copy --dest-tls-verify=false \
            oci:${TEST_DATA_DIR}/golang:1.20 \
            docker://127.0.0.1:8080/golang${i}:1.20
        [ "$status" -eq 0 ]
    done
}

@test "restart zot with dedupe enabled" {
    local zot_config_file=${BATS_FILE_TMPDIR}/zot_config.json

    # stop server
    teardown_zot_file_level

    # enable dedupe
    sed -i 's/false/true/g' ${zot_config_file}

    setup_zot_file_level ${zot_config_file}
    wait_zot_reachable "http://127.0.0.1:8080/v2/_catalog"
    # deduping will now run in background (task scheduler) while we push images, shouldn't interfere
}

@test "push image - dedupe running" {
    start=`date +%s`
    run skopeo --insecure-policy copy --dest-tls-verify=false \
        oci:${TEST_DATA_DIR}/golang:1.20 \
        docker://127.0.0.1:8080/dedupe/golang:1.20
    [ "$status" -eq 0 ]
    end=`date +%s`

    runtime=$((end-start))
    echo "push image exec time: $runtime sec" >&3
}

@test "pull image - dedupe running" {
    local oci_data_dir=${BATS_FILE_TMPDIR}/oci

    mkdir -p ${oci_data_dir}/dedupe/    

    start=`date +%s`
    run skopeo --insecure-policy copy --src-tls-verify=false \
        docker://127.0.0.1:8080/dedupe/golang:1.20 \
        oci:${oci_data_dir}/dedupe/golang:1.20
    [ "$status" -eq 0 ]
    end=`date +%s`
    runtime=$((end-start))

    echo "pull image exec time: $runtime sec" >&3
}

@test "pull deduped image - dedupe running" {
    local oci_data_dir=${BATS_FILE_TMPDIR}/oci

    mkdir -p ${oci_data_dir}/dedupe/    

    start=`date +%s`
    run skopeo --insecure-policy copy --src-tls-verify=false \
        docker://127.0.0.1:8080/golang2:1.20 \
        oci:${oci_data_dir}/dedupe/golang2:1.20
    [ "$status" -eq 0 ]
    end=`date +%s`
    runtime=$((end-start))

    echo "pull image exec time: $runtime sec" >&3
}

@test "push image index - dedupe running" {
    # --multi-arch below pushes an image index (containing many images) instead
    # of an image manifest (single image)
    start=`date +%s`
    run skopeo --insecure-policy copy --format=oci --dest-tls-verify=false --multi-arch=all \
        docker://public.ecr.aws/docker/library/busybox:latest \
        docker://127.0.0.1:8080/busybox:latest
    [ "$status" -eq 0 ]
    end=`date +%s`
    runtime=$((end-start))

    echo "push image index exec time: $runtime sec" >&3
    run curl http://127.0.0.1:8080/v2/_catalog
    [ "$status" -eq 0 ]
    [ $(echo "${lines[-1]}" | jq '.repositories[0]') = '"busybox"' ]
    run curl http://127.0.0.1:8080/v2/busybox/tags/list
    [ "$status" -eq 0 ]
    [ $(echo "${lines[-1]}" | jq '.tags[]') = '"latest"' ]
}

@test "pull image index - dedupe running" {
    local oci_data_dir=${BATS_FILE_TMPDIR}/oci
    start=`date +%s`
    run skopeo --insecure-policy copy --src-tls-verify=false --multi-arch=all \
        docker://127.0.0.1:8080/busybox:latest \
        oci:${oci_data_dir}/busybox:latest
    [ "$status" -eq 0 ]
    end=`date +%s`
    runtime=$((end-start))

    echo "pull image index exec time: $runtime sec" >&3
    run cat ${BATS_FILE_TMPDIR}/oci/busybox/index.json
    [ "$status" -eq 0 ]
    [ $(echo "${lines[-1]}" | jq '.manifests[].annotations."org.opencontainers.image.ref.name"') = '"latest"' ]
    run skopeo --insecure-policy --override-arch=arm64 --override-os=linux copy --src-tls-verify=false --multi-arch=all \
        docker://127.0.0.1:8080/busybox:latest \
        oci:${oci_data_dir}/busybox:latest
    [ "$status" -eq 0 ]
    run cat ${BATS_FILE_TMPDIR}/oci/busybox/index.json
    [ "$status" -eq 0 ]
    [ $(echo "${lines[-1]}" | jq '.manifests[].annotations."org.opencontainers.image.ref.name"') = '"latest"' ]
    run curl -X DELETE http://127.0.0.1:8080/v2/busybox/manifests/latest
    [ "$status" -eq 0 ]
}

@test "push oras artifact - dedupe running" {
    echo "{\"name\":\"foo\",\"value\":\"bar\"}" > config.json
    echo "hello world" > artifact.txt
    start=`date +%s`
    run oras push --plain-http 127.0.0.1:8080/hello-artifact:v2 \
        --config config.json:application/vnd.acme.rocket.config.v1+json artifact.txt:text/plain -d -v
    [ "$status" -eq 0 ]
    end=`date +%s`
    runtime=$((end-start))

    echo "push oras artifact exec time: $runtime sec" >&3
    rm -f artifact.txt
    rm -f config.json
}

@test "pull oras artifact - dedupe running" {
    start=`date +%s`
    run oras pull --plain-http 127.0.0.1:8080/hello-artifact:v2 -d -v
    [ "$status" -eq 0 ]
    end=`date +%s`
    runtime=$((end-start))

    echo "pull oras artifact exec time: $runtime sec" >&3
    grep -q "hello world" artifact.txt
    rm -f artifact.txt
}

@test "attach oras artifacts - dedupe running" {
    # attach signature
    echo "{\"artifact\": \"\", \"signature\": \"pat hancock\"}" > signature.json
    start=`date +%s`
    run oras attach --plain-http 127.0.0.1:8080/golang:1.20 --image-spec v1.1-image --artifact-type 'signature/example' ./signature.json:application/json
    [ "$status" -eq 0 ]
    end=`date +%s`
    runtime=$((end-start))

    echo "attach signature exec time: $runtime sec" >&3
    # attach sbom
    echo "{\"version\": \"0.0.0.0\", \"artifact\": \"'127.0.0.1:8080/golang:1.20'\", \"contents\": \"good\"}" > sbom.json
    start=`date +%s`
    run oras attach --plain-http 127.0.0.1:8080/golang:1.20 --image-spec v1.1-image --artifact-type 'sbom/example' ./sbom.json:application/json
    [ "$status" -eq 0 ]
    end=`date +%s`
    runtime=$((end-start))

    echo "attach sbom exec time: $runtime sec" >&3
}

@test "discover oras artifacts - dedupe running" {
    start=`date +%s`
    run oras discover --plain-http -o json 127.0.0.1:8080/golang:1.20
    [ "$status" -eq 0 ]
    end=`date +%s`
    runtime=$((end-start))

    echo "discover oras artifacts exec time: $runtime sec" >&3
    [ $(echo "$output" | jq -r ".manifests | length") -eq 2 ]
}

@test "push helm chart - dedupe running" {
    run helm package ${BATS_FILE_TMPDIR}/helm-charts/charts/zot
    [ "$status" -eq 0 ]
    local chart_version=$(awk '/version/{printf $2}' ${BATS_FILE_TMPDIR}/helm-charts/charts/zot/Chart.yaml)
    start=`date +%s`
    run helm push zot-${chart_version}.tgz oci://localhost:8080/zot-chart
    [ "$status" -eq 0 ]
    end=`date +%s`
    runtime=$((end-start))

    echo "helm push exec time: $runtime sec" >&3
}

@test "pull helm chart - dedupe running" {
    local chart_version=$(awk '/version/{printf $2}' ${BATS_FILE_TMPDIR}/helm-charts/charts/zot/Chart.yaml)
    start=`date +%s`
    run helm pull oci://localhost:8080/zot-chart/zot --version ${chart_version}
    [ "$status" -eq 0 ]
    end=`date +%s`
    runtime=$((end-start))

    echo "helm pull exec time: $runtime sec" >&3
}

@test "push image with regclient - dedupe running" {
    run regctl registry set localhost:8080 --tls disabled
    [ "$status" -eq 0 ]
    start=`date +%s`
    run regctl image copy ocidir://${TEST_DATA_DIR}/golang:1.20 localhost:8080/test-regclient
    [ "$status" -eq 0 ]
    end=`date +%s`
    runtime=$((end-start))

    echo "regclient push exec time: $runtime" >&3
}

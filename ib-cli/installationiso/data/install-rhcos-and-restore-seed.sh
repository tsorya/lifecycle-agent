#!/bin/bash

set -e # Halt on error

seed_image=${1:-$SEED_IMAGE}
seed_version=${2:-$SEED_VERSION}
installation_disk=${3:-$INSTALLATION_DISK}
lca_image=${4:-$LCA_IMAGE}
extra_partition_start=${5:-$EXTRA_PARTITION_START}
extra_partition_number=5
extra_partition_label=varlibcontainers


authfile=${AUTH_FILE:-"/var/tmp/backup-secret.json"}
pull_secret=${PULL_SECRET_FILE:-"/var/tmp/pull-secret.json"}


additional_flags=""
if [ -n "${PRECACHE_DISABLED}" ]; then
    additional_flags="${additional_flags} --precache-disabled"
fi

if [ -n "${PRECACHE_BEST_EFFORT}" ]; then
    additional_flags="${additional_flags} --precache-best-effort"
fi

if [ -n "${SKIP_SHUTDOWN}" ]; then
    additional_flags="${additional_flags} --skip-shutdown"
fi

if [[ ! "$extra_partition_start" == "use_directory" ]]; then
    additional_flags="${additional_flags} --create-extra-partition"
fi

podman run --privileged --security-opt label=type:unconfined_t --rm --pid=host --authfile "${authfile}" -v /:/host --entrypoint /usr/local/bin/lca-cli "${lca_image}" ibi --seed-image "${seed_image}" --authfile "${authfile}" --seed-version "${seed_version}" --pullSecretFile "${pull_secret}" ${additional_flags} --installation-disk "${installation_disk}" --extra-partition-start "${extra_partition_start}" --extra-partition-number "${extra_partition_number}" --extra-partition-label "${extra_partition_label}"

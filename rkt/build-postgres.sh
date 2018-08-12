#!/usr/bin/env bash
set -e

if [ "$EUID" -ne 0 ]; then
    echo "This script uses functionality which requires root privileges"
    exit 1
fi

# Start the build from alpine-os
newcontainer=$(buildah --debug from quay.io/coreos/alpine-sh)

__version=$(printf "r%s.%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)")

# Based on alpine
buildah --debug config --arch amd64 ${newcontainer}
buildah --debug config --os linux ${newcontainer}

# Install postgres
buildah --debug run ${newcontainer} -- apk update
buildah --debug run ${newcontainer} -- apk add postgresql

buildah --debug copy ${newcontainer} ./postgres/run.sh /bin/run.sh
buildah --debug copy ${newcontainer} ./postgres/postgresql.auto.conf /var/lib/postgres/
buildah --debug copy ${newcontainer} ./postgres/postgresql.conf /var/lib/postgres/

buildah --debug config --port 5432 ${newcontainer}
buildah --debug config --entrypoint /bin/run.sh ${newcontainer}
buildah --debug config --user postgres ${newcontainer}

# Add a mount points
buildah --debug config --volume data /var/lib/postgres/data
buildah --debug config --volume sock /var/run/postgresql

buildah --debug config --created-by "Marius Orcsik <marius@littr.me>" ${newcontainer}
buildah --debug config --author "lpbm" ${newcontainer}

image="littr-me-postgres-${__version}-linux-amd64"
# Save the image
buildah --debug inspect ${newcontainer}
buildah --debug commit --rm --signature-policy ./insecureAcceptAnything.json --squash ${newcontainer} ${image}
buildah --debug push --format oci --signature-policy ./insecureAcceptAnything.json "${image}" "oci-archive:./${image}.oci"

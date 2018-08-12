#!/usr/bin/env bash
set -e

# Start the build from alpine-os
newcontainer=$(buildah --debug --registries-conf ./registries.toml from quay.io/coreos/alpine-sh )

__version=$(printf "r%s.%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)")

# Based on alpine
buildah --debug config --arch amd64 ${newcontainer}
buildah --debug config --os linux ${newcontainer}

# Install postgres
buildah --debug run ${newcontainer} -- apk update
buildah --debug run ${newcontainer} -- apk add nginx

buildah --debug run ${newcontainer} -- bash -c 'echo "nginx on OCI alpine os image, built using Buildah" > /usr/share/nginx/html/index.html'
buildah --debug config --port 80 --entrypoint /usr/sbin/nginx ${newcontainer}

buildah --debug run copy ${newcontainer} ../nginx/nginx.conf /etc/nginx/nginx.conf

buildah --debug config --created-by "Marius Orcsik <marius@littr.me>" ${newcontainer}
buildah --debug config --author "lpbm" ${newcontainer}

image="littr-me-nginx-${__version}-linux-amd64"
# Save the image
buildah --debug inspect ${newcontainer}
buildah --debug commit --rm --signature-policy ./insecureAcceptAnything.json --squash ${newcontainer} ${image}
buildah --debug push --format oci --signature-policy ./insecureAcceptAnything.json "${image}" "oci-archive:./${image}.oci"

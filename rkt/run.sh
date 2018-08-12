#!/usr/bin/env bash

__version=$(printf "r%s.%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)")

test -f littr-me-postgres-${__version}-linux-amd64.aci || ./build-postgres.sh
test -f littr-me-nginx-${__version}-linux-amd64.aci || ./build-nginx.sh
test -f littr-me-app-${__version}-linux-amd64.aci || ./build-app.sh

#rkt --debug --insecure-options=image \
#    run \
#    littr-me-postgres-${__version}-linux-amd64.aci --volume=data,kind=host,source=/tmp/data \
#    littr-me-nginx-${__version}-linux-amd64.aci --net=host \
#    littr-me-app-${__version}-linux-amd64.aci
rkt run --port=http:3000 \
   littr-me-app-${__version}-linux-amd64.oci

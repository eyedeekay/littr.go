#!/usr/bin/env bash
#!/usr/bin/env bash
set -e

if [ "$EUID" -ne 0 ]; then
    echo "This script uses functionality which requires root privileges"
    exit 1
fi

# Start the build with an empty container
newcontainer=$(buildah --debug from scratch)

__version=$(printf "r%s.%s" "$(git rev-list --count HEAD)" "$(git rev-parse --short HEAD)")

# Based on alpine
buildah --debug config --arch amd64 ${newcontainer}
buildah --debug config --os linux ${newcontainer}

IFS=$'\r\n'
GLOBIGNORE='*'
__env=($(<../.env))
for i in ${__env[@]}; do
    name=${i%=*}
    quot=${i#*=}
    value=${quot//\"}
    buildah --debug config --env "${name}=${value}" ${newcontainer}
done

if [ test -x ../littr ]; then
    cd .. && make littr && cd rkt/
fi

buildah --debug copy ${newcontainer} ../littr /bin/app
buildah --debug copy ${newcontainer} ../assets /assets
buildah --debug copy ${newcontainer} ../templates /templates
buildah --debug config --entrypoint "/bin/app" ${newcontainer}

# Add a port for http traffic over port 3000
buildah --debug config --port 3000 ${newcontainer}

buildah --debug config --workingdir / ${newcontainer}

buildah --debug config --created-by "Marius Orcsik <marius@littr.me>" ${newcontainer}
buildah --debug config --author "lpbm" ${newcontainer}

image="littr-me-app-${__version}-linux-amd64"
# Save the image
buildah --debug inspect ${newcontainer}
buildah --debug commit --rm --signature-policy ./insecureAcceptAnything.json --squash ${newcontainer} ${image}
buildah --debug push --format oci --signature-policy ./insecureAcceptAnything.json "${image}" "oci-archive:./${image}.oci"

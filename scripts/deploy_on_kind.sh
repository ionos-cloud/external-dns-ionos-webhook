#!/usr/bin/env bash

set -e # exit on first error

# local registry
LOCAL_REGISTRY_PORT=5001
LOCAL_REGISTRY_NAME=kind-registry
LOCAL_REGISTRY_RUNNING=$(docker ps -a | grep -q $LOCAL_REGISTRY_NAME && echo "true" || echo "false")

# docker
IMAGE_REGISTRY=localhost:$LOCAL_REGISTRY_PORT
IMAGE_NAME=external-dns-ionos-webhook
IMAGE=$IMAGE_REGISTRY/$IMAGE_NAME

#kind
KIND_CLUSTER_NAME=external-dns
KIND_CLUSTER_CONFIG=./deployments/kind/cluster.yaml
KIND_CLUSTER_RUNNING=$(kind get clusters -q | grep -q $KIND_CLUSTER_NAME && echo "true" || echo "false")
KIND_CLUSTER_WAIT=60s

## helm
HELM_CHART=external-dns/external-dns
HELM_RELEASE_NAME=external-dns-ionos
HELM_VALUES_FILE=deployments/helm/local-kind-values.yaml


# if there is a clean up argument, delete the kind cluster and local registry
if [ "$1" = "clean" ]; then
    printf "Cleaning up...\n"
    if [ "$KIND_CLUSTER_RUNNING" = "true" ]; then
        printf "Deleting kind cluster...\n"
        kind delete cluster --name "$KIND_CLUSTER_NAME"
    fi
    if [ "$LOCAL_REGISTRY_RUNNING" = "true" ]; then
        printf "Deleting local registry...\n"
        docker rm -f "$LOCAL_REGISTRY_NAME"
    fi
    exit 0
fi

# if there is a helm-delete argument, delete the helm release
if [ "$1" = "helm-delete" ]; then
    printf "Deleting helm release...\n"
    helm delete $HELM_RELEASE_NAME
    exit 0
fi

printf "KIND_CLUSTER_RUNNING: %s\n" "$KIND_CLUSTER_RUNNING"
printf "LOCAL_REGISTRY_RUNNING: %s\n" "$LOCAL_REGISTRY_RUNNING"

# run local registry if not running
if [ "$LOCAL_REGISTRY_RUNNING" = "false" ]; then
    printf "Starting local registry...\n"
    docker run -d --restart=always -p "127.0.0.1:$LOCAL_REGISTRY_PORT:5000" --name "$LOCAL_REGISTRY_NAME" registry:2
fi

# run kind cluster if not running
if [ "$KIND_CLUSTER_RUNNING" = "false" ]; then
    printf "Starting kind cluster...\n"
    kind create cluster  --name $KIND_CLUSTER_NAME --config $KIND_CLUSTER_CONFIG
    kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml
    sleep $KIND_CLUSTER_WAIT
    docker network connect "kind" "$LOCAL_REGISTRY_NAME"
    kubectl apply -f ./deployments/kind/local-registry-configmap.yaml
    printf "Installing dns mock server...\n"
    helm upgrade --install --create-namespace --namespace mockserver --set app.serverPort=1080 --set app.logLevel=INFO mockserver mockserver/mockserver
    sleep $KIND_CLUSTER_WAIT
    kubectl port-forward svc/mockserver -n mockserver 1080:1080 &
    sleep 20

    # set expectation for mock server
    RESPONSE_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X PUT -H "Content-Type: application/json" http://localhost:1080/mockserver/expectation -d @scripts/expectation-payload.json)
    if [ "$RESPONSE_CODE" -eq "201" ]; then
      echo "Created expectation on the mock server successfully"
    else
      echo "Failed to create mock server expectation with code $RESPONSE_CODE"
    fi

    pkill -f "kubectl port-forward"
fi

printf "Building binary...\n"
make build

printf "Building image...\n"
make docker-build

printf "Pushing image...\n"
make docker-push

helm upgrade $HELM_RELEASE_NAME $HELM_CHART -f $HELM_VALUES_FILE --install

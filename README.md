# ExternalDNS - IONOS Webhook

ExternalDNS is a Kubernetes controller for automatically managing
Domain Name System (DNS) records for Kubernetes services by using different DNS providers.
By default, Kubernetes manages DNS records internally,
but ExternalDNS takes this functionality a step further by delegating the management of DNS records to an external DNS
provider such as IONOS.
Therefore, the IONOS webhook allows to manage your
IONOS domains inside your kubernetes cluster with [ExternalDNS](https://github.com/kubernetes-sigs/external-dns). 

 For detailed technical instructions on how the IONOS webhook is deployed using [ExternalDNS for Kubernetes](https://kubernetes-sigs.github.io/external-dns/) helm repo, see [deployment instructions](#kubernetes-deployment).

## Providers

1. **IONOS Core:** 
   * Official Page: [https://www.ionos.com/hosting/free-dns](https://www.ionos.com/hosting/free-dns)
   * API docs: [https://developer.hosting.ionos.com/docs/dns](https://developer.hosting.ionos.com/docs/dns)
2. **IONOS Cloud:** 
   * Official Page: [https://cloud.ionos.com/network/cloud-dns](https://cloud.ionos.com/network/cloud-dns)
   * API docs: [https://api.ionos.com/docs/dns/v1/](https://api.ionos.com/docs/dns/v1/)

> [!NOTE]  
> The provider is automatically detected based on the credentials.

## Authentication Methods:

1. **IONOS Core:** Only API Key authentication is supported
2. **IONOS Cloud:** Both username/password and token authentication are supported. The username/password method has the advantage of not requiring the user to intervene periodically. If a token is used, it falls under the responsibility of the user to renew the token periodically (IONOS Cloud tokens can have a maximum ttl of 365 days). Regardless of the method used, it is highly recommended to scope the privileges to the DNS management only. This can be done by creating a new IAM user under your main contract, and scoping the privileges to "Access and manage DNS". More details on how to create a bot user can be found [here](https://github.com/ionos-cloud/cert-manager-webhook-ionos-cloud/blob/main/docs/create-bot-user.md)

> [!IMPORTANT]  
> It is not recommended to use the credentials of the root/Admin account. 


## Kubernetes Deployment

The IONOS webhook is provided as a regular Open Container Initiative (OCI) image released in
the [GitHub container registry](https://github.com/ionos-cloud/external-dns-ionos-webhook/pkgs/container/external-dns-ionos-webhook).
The deployment can be performed in every way Kubernetes supports.
The following example shows the deployment as
a [sidecar container](https://kubernetes.io/docs/concepts/workloads/pods/#workload-resources-for-managing-pods) in the
ExternalDNS pod
using the [charts for ExternalDNS](https://github.com/kubernetes-sigs/external-dns/tree/master/charts/external-dns).

```shell
helm repo add external-dns https://kubernetes-sigs.github.io/external-dns/

kubectl create secret generic ionos-credentials --from-literal=api-key='<EXAMPLE_PLEASE_REPLACE>'
# or, depending on the provider and the authentication method
# only one of two commands in needed
kubectl create secret generic ionos-credentials --from-literal=username='<BOT_USERNAME>' --from-literal=password='<BOT_PASSWORD>'

# create the helm values file
cat <<EOF > external-dns-ionos-values.yaml
image:
  tag: v0.19.0

# -- ExternalDNS Log level.
logLevel: debug # reduce in production

# -- if true, ExternalDNS will run in a namespaced scope (Role and Rolebinding will be namespaced too).
namespaced: false

# -- Kubernetes resources to monitor for DNS entries.
sources:
  - ingress
  - service
  - crd

provider:
  name: webhook
  webhook:
    image:
      repository: ghcr.io/ionos-cloud/external-dns-ionos-webhook
      tag: v0.14.0
      pullPolicy: IfNotPresent
    env:
    - name: LOG_LEVEL
      value: debug # reduce in production
    - name: IONOS_API_KEY
      valueFrom:
        secretKeyRef:
          name: ionos-credentials
          key: api-key
    # this is only valid for IONOS Cloud, if the username/password method is used.
    - name: IONOS_USERNAME
      valueFrom:
        secretKeyRef:
          name: ionos-credentials
          key: username
    # this is only valid for IONOS Cloud, if the username/password method is used.
    - name: IONOS_PASSWORD
      valueFrom:
        secretKeyRef:
          name: ionos-credentials
          key: password
    # The webhook server listens on localhost by default. Otherwise, you can set SERVER_HOST.
    - name: SERVER_PORT
      value: "8888" # default and recommended port for exposing webhook provider EPs
    # The exposed server listens on all interfaces (0.0.0.0) by default. Otherwise, you can set METRICS_HOST.
    - name: METRICS_PORT
      value: "8080" # default and recommended port for exposing metrics and health EPs
    - name: IONOS_DEBUG
      value: "false" # change to "true" if you want see details of the http requests
    - name: IONOS_TOKEN_TTL
      value: "6000h" # allows configuring the refresh interval for the IONOS Cloud token. Defaults to 31536000s (1 year).
    - name: DRY_RUN
      value: "true" # set to "false" when you want to allow making changes to your DNS resources
EOF

# install external-dns with helm
helm upgrade external-dns-ionos external-dns/external-dns --version 1.19.0 -f external-dns-ionos-values.yaml --install
```

### namespaced mode

Currently, the rbac created for a namespaced deployment is not sufficient for the ExternalDNS to work.
In order to get ExternalDNS running in a namespaced mode, you need to create the necessary cluster-role-(binding) resources manually:

```shell
# don't forget to adjust the namespace for the service account in the rbac-for-namespaced.yaml file, if you are using a different namespace than 'default'
kubectl apply -f deployments/rbac-for-namespaced.yaml
```

In the helm chart configuration you then can skip the rbac configuration, so in the helm values file you set:

```yaml
namespaced: true

rbac:
  create: false
```

See [here](./cmd/webhook/init/configuration/configuration.go) for all available configuration options of the IONOS webhook.

## Verify the image resource integrity

All official webhooks provided by IONOS are signed using [Cosign](https://docs.sigstore.dev/cosign/overview/).
The Cosign public key can be found in the [cosign.pub](./cosign.pub) file.

Note: Due to the early development stage of the webhook, the image is not yet signed
by [sigstores transparency log](https://github.com/sigstore/rekor).

```shell
export RELEASE_VERSION=latest
cosign verify --insecure-ignore-tlog --key cosign.pub ghcr.io/ionos-cloud/external-dns-ionos-webhook:$RELEASE_VERSION
```

### Metrics

The Go runtime metrics are exposed via the `/metrics` endpoint, and the health check is available on the `/healthz` endpoint. Both endpoints are served on port 8080 by default.

## Development

The basic development tasks are provided by make. Run `make help` to see the available targets.

### Local deployment

The webhook can be deployed locally with a kind cluster. As a prerequisite, you need to install:

- [Docker](https://docs.docker.com/get-docker/),
- [Helm](https://helm.sh/ ) with the repos:

 ```shell
  helm repo add external-dns https://kubernetes-sigs.github.io/external-dns
  helm repo add mockserver https://www.mock-server.com
  helm repo update
  ```

- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)

```shell
# setup the kind cluster and deploy external-dns with ionos webhook and a dns mockserver
./scripts/deploy_on_kind.sh

# check if the webhook is running
kubectl get pods -l app.kubernetes.io/name=external-dns -o wide

# trigger a DNS change e.g. with annotating the ingress controller service
kubectl -n ingress-nginx annotate service  ingress-nginx-controller "external-dns.alpha.kubernetes.io/internal-hostname=nginx.internal.example.org." 
 
# cleanup
./scripts/deploy_on_kind.sh clean
```

### Local acceptance tests

The acceptance tests are run against a kind cluster with ExternalDNS and the webhook deployed.
The DNS mock server is used to verify the DNS changes. The following diagram shows the test setup:

```mermaid
flowchart LR
subgraph local-machine
  T[<h3>acceptance-test with hurl</h3><ul><li>create HTTP requests</li><li>check HTTP responses</li></ul>] -- 1. create expectations --> M
  T -- 2. create annotations/ingress --> K
  T -- 3. verify expectations --> M

  subgraph k8s kind
    E("external-dns") -. checks .-> K[k8s resources]
    E -. apply record changes .-> M[dns-mockserver]
  end
end

```

For running the acceptance tests locally you need to install [hurl](https://hurl.dev/).
To check the test run execution, see the [Hurl files](./test/hurl).
To view the test reports, see the `./build/reports/hurl` directory.

```shell
scripts/acceptance-tests.sh 
```


# setup external dns with ionos webhook and a dns mockserver

## ionos core

```shell
kubectl create namespace external-dns-ionoscore
kubectl-ns external-dns-ionoscore 
kubectl create secret generic ionos-credentials --from-literal=api-key=<your api key>
helm upgrade external-dns-ionoscore bitnami/external-dns -f external-dns-ionoscore-values.yaml --install 
# deploy kuard 
kubectl run --restart=Never --image=gcr.io/kuar-demo/kuard-amd64:blue kuard --port 8080
# kubectl port-forward kuard 8080:8080
kubectl expose pod kuard --type=LoadBalancer
# activate external dns with service annotation
kubectl annotate --overwrite service kuard "external-dns.alpha.kubernetes.io/hostname=kuard.test-dns-public-0002.info"
``` 

## ionos cloud

```shell
kubectl create namespace external-dns-ionocloud
kubectl-ns external-dns-ionoscloud
kubectl create secret generic ionos-cloud-credentials --from-literal=api-key=<your api key>
helm upgrade external-dns-ionoscloud bitnami/external-dns -f external-dns-ionoscloud-values.yaml --install 
# deploy kuard 
kubectl run --restart=Never --image=gcr.io/kuar-demo/kuard-amd64:green kuard --port 8080
# kubectl port-forward kuard 8080:8080
kubectl expose pod kuard --type=LoadBalancer
# activate external dns with service annotation
kubectl annotate --overwrite service kuard "external-dns.alpha.kubernetes.io/hostname=kuard.demo-ionos.cloud"
``` 



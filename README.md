# Pagerduty Operator

## About
The PagerDuty operator is used to automate integrating Openshift Dedicated clusters with Pagerduty that are provisioned via https://cloud.redhat.com/.

This operator runs on [Hive](https://github.com/openshift/hive) and watches for new cluster deployments. Hive is an API driven OpenShift cluster providing OpenShift Dedicated provisioning and management.

## How the PagerDuty Opertor works

* PagerDuty's reconcile function watches for the `installed` field of the `ClusterDeployment` CRD and waits for the cluster to finish installation. It also sees if `api.openshift.com/noalerts` label is set on the `ClusterDeployment` of the new cluster being provisioned. 
  * The `api.openshift.com/noalerts` label is used to disable alerts from the provisioned cluster. This label is typically used on test clusters that do not require immediate attention as a result of critical issues or outages. Therefore, PagerDuty does not continue its actions if it finds this label in the new cluster's `ClusterDeployment`.
* Once the `installed` field becomes true, PagerDuty creates a secret which contains the integration key required to communicate with PagerDuty Web application.
* The PagerDuty operator then creates [syncset](https://github.com/openshift/hive/blob/master/config/crds/hive_v1_syncset.yaml) with the relevant information for hive to send the PagerDuty secret to the newly provisioned cluster .
* This syncset is used by hive to deploy the pagerduty secret to the provisioned cluster so that Openshift SRE can be alerted in case of issues on the cluster.
* Generally, the pagerduty secret is deployed under the `openshift-monitoring` namespace and named `pd-secret` on the new cluster.

## Development

### Set up local openshift cluster

For example install [minishift](https://github.com/minishift/minishift) as described in its readme.


### Deploy dependencies

Create hive CRDs. To do so, clone [hive repo](https://github.com/openshift/hive/) and run

```terminal
$ oc apply -f config/crds
```

deploy namespace, role, etc from pagerduty-operator

```terminal
$ oc apply -f manifests/01-namespace.yaml
$ oc apply -f manifests/02-role.yaml
$ oc apply -f manifests/03-service_account.yaml
$ oc apply -f manifests/04-role_binding.yaml
```


Create secret with pagerduty api key, for example using a [trial account](https://www.pagerduty.com/free-trial/). You can then create an API key at https://<your-account>.pagerduty.com/api_keys. Also, you need to create the ID of you escalation policy. You can get this by clicking on your policy at https://<your-account>.pagerduty.com/escalation_policies#. The ID will afterwards be visible in the URL behind the `#` character.
Following is an example secret to adjust and apply with `oc apply -f <filename>`.

```yaml
apiVersion: v1
data:
  ACKNOWLEDGE_TIMEOUT: MjE2MDA=
  ESCALATION_POLICY: MTIzNA== #echo -n <escalation-policy-id> | base64
  PAGERDUTY_API_KEY: bXktYXBpLWtleQ== #echo -n <pagerduty-api-key> | base64
  RESOLVE_TIMEOUT: MA==
  SERVICE_PREFIX: b3Nk
kind: Secret
metadata:
  name: pagerduty-api-key
  namespace: pagerduty-operator
type: Opaque
```

### Option 1: Run pagerduty-operator outside of OpenShift

```terminal
$ export OPERATOR_NAME=pagerduty-operator
$ go run cmd/manager/main.go
```

Create namespace `pagerduty-operator`.

```
$ oc create namespace pagerduty-operator
```

Continue to `Create ClusterDeployment`.

### Option 2: Run local built operator in minishift

Build local code modifications and push image to your own quay.io account.

```terminal
$ make docker-build
[...]
Successfully tagged quay.io/<userid>/pagerduty-operator:v0.1.129-057ffd29

$ docker tag quay.io/<userid>/pagerduty-operator:v0.1.129-057ffd29 Successfully tagged quay.io/<userid>/pagerduty-operator:latest
$ docker login quay.io
$ docker push quay.io/<userid>/pagerduty-operator:latest
```

#### Generate secret with quay.io creds

* visit account page https://quay.io/user/<userid>?tab=settings
* click _generate encrypted password_
* Re-enter password
* download `<userid>-secret.yml`
* Deploy quay.io secret

```terminal
$ oc project pagerduty-operator
$ oc apply -f ~/Downloads/<userid>-secret.yml -n pagerduty-operator
```

#### Deploy pagerduty-operator from custom repo

Create a copy of `manifests/05-operator.yaml` and modify it use your image from quay.io

```yaml
...
      imagePullSecrets:
        - name: <userid>-pull-secret
      containers:
        - name: pagerduty-operator
          image: quay.io/<userid>/pagerduty-operator
...
```

Deploy modified operator manifest

```terminal
$ oc apply -f path/to/modified/operator.yaml 
```

### Create ClusterDeployment

`pagerduty-operator` doesn't start reconciling clusters until `status.installed` is set to `true`. To be able to set this variable via `oc edit` without actually deploying a cluster to AWS, the ClusterDeployment CRD needs to be adjusted.

```terminal
$ oc edit crd clusterdeployments.hive.openshift.io
```

Remove `subsesource` part:

```
spec:
  [...]
  subresources: ## delete me
    status: {}  ## delete me
[...]
```

Create ClusterDeployment.

```terminal
$ oc create namespace fake-cluster-namespace
$ oc apply -f hack/clusterdeployment/fake-clusterdeployment.yml
```

If present, set `status.installed` to true.

```terminal
$ oc edit clusterdeployment fake-cluster -n fake-cluster-namespace
```

### Delete ClusterDeployment

To trigger `pagerduty-operator` to remove the service in pagerduty, delete the clusterdeployment.

```terminal
$ oc delete clusterdeployment fake-cluster -n fake-cluster-namespace
```

You may need to remove dangling finalizers from the `clusterdeployment` object.

```terminal
$ oc edit clusterdeployment fake-cluster -n fake-cluster-namespace
```

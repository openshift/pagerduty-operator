# Pagerduty Operator

- [Pagerduty Operator](#pagerduty-operator)
  - [About](#about)
  - [How the PagerDuty Operator works](#how-the-pagerduty-operator-works)
  - [Development](#development)
    - [Set up local openshift cluster](#set-up-local-openshift-cluster)
    - [Deploy dependencies](#deploy-dependencies)
    - [Option 1: Run pagerduty-operator outside of OpenShift](#option-1-run-pagerduty-operator-outside-of-openshift)
    - [Option 2: Run local built operator in CRC](#option-2-run-local-built-operator-in-crc)
      - [Generate secret with quay.io creds](#generate-secret-with-quayio-creds)
      - [Deploy pagerduty-operator from custom repo](#deploy-pagerduty-operator-from-custom-repo)
    - [Create PagerDutyIntegration](#create-pagerdutyintegration)
    - [Create ClusterDeployment](#create-clusterdeployment)
    - [Delete ClusterDeployment](#delete-clusterdeployment)

## About
The PagerDuty operator is used to automate integrating Openshift Dedicated clusters with Pagerduty that are provisioned via https://cloud.redhat.com/.

This operator runs on [Hive](https://github.com/openshift/hive) and watches for new cluster deployments. Hive is an API driven OpenShift cluster providing OpenShift Dedicated provisioning and management.

## How the PagerDuty Operator works

* The PagerDutyIntegration controller watches for changes to PagerDutyIntegration CRs, and also for changes to appropriately labeled ClusterDeployment CRs (and ConfigMap/Secret/SyncSet resources owned by such a ClusterDeployment).
* For each PagerDutyIntegration CR, it will get a list of matching ClusterDeployments that have the `spec.installed` field set to true.
* For each of these ClusterDeployments, PagerDuty creates a secret which contains the integration key required to communicate with PagerDuty Web application.
* The PagerDuty operator then creates [syncset](https://github.com/openshift/hive/blob/master/config/crds/hive.openshift.io_syncsets.yaml) with the relevant information for hive to send the PagerDuty secret to the newly provisioned cluster .
* This syncset is used by hive to deploy the pagerduty secret to the provisioned cluster so that the relevant SRE team get notified of alerts on the cluster.
* The pagerduty secret is deployed to the coordinates specified in the `spec.targetSecretRef` field of the PagerDutyIntegration CR.

## Development

### Set up local openshift cluster

For example install [crc](http://crc.dev/crc/getting_started/getting_started/introducing/) as described in its documentation.


### Deploy dependencies

Create hive CRDs. To do so, clone [hive repo](https://github.com/openshift/hive/) and run

```terminal
oc apply -f config/crds
```

deploy namespace, role, etc from pagerduty-operator

```terminal
oc apply -f manifests/01-namespace.yaml
oc apply -f manifests/02-role.yaml
oc apply -f manifests/03-service_account.yaml
oc apply -f manifests/04-role_binding.yaml
oc apply -f deploy/crds/pagerduty.openshift.io_pagerdutyintegrations.yaml
```


Create secret with pagerduty api key, for example using a [trial account](https://www.pagerduty.com/free-trial/). You can then create an API key at https://{your-account}.pagerduty.com/api_keys.
Following is an example secret to adjust and apply with `oc apply -f <filename>`.

```yaml
apiVersion: v1
data:
  PAGERDUTY_API_KEY: bXktYXBpLWtleQ== #echo -n <pagerduty-api-key> | base64
kind: Secret
metadata:
  name: pagerduty-api-key
  namespace: pagerduty-operator
type: Opaque
```

### Option 1: Run pagerduty-operator outside of OpenShift

```terminal
export OPERATOR_NAME=pagerduty-operator
go run main.go
```

In an other terminal create namespace `pagerduty-operator` if needed:
```
oc create namespace pagerduty-operator
```

Continue to [Create PagerDutyIntegration](#create-pagerdutyintegration).

### Option 2: Run local built operator in CRC

Build local code modifications and push image to your own quay.io account.

```terminal
$ ALLOW_DIRTY_CHECKOUT=true IMAGE_REPOSITORY=${USER_ID} make docker-build
[...]
Successfully tagged quay.io/<userid>/pagerduty-operator:latest

$ podman login quay.io
$ podman push quay.io/<userid>/pagerduty-operator:latest
```

#### Generate secret with quay.io creds

* visit account page https://quay.io/user/{userid}?tab=settings
* click _generate encrypted password_
* Re-enter password
* download `<userid>-secret.yml`
* Deploy quay.io secret

```terminal
oc project pagerduty-operator
oc apply -f ~/Downloads/<userid>-secret.yml -n pagerduty-operator
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
oc apply -f path/to/modified/operator.yaml
```

**Note:**  In some cases, the `pagerduty-operator` pod in the `pagerduty-operator` namespace doesn't start with the following error:

```terminal 
Warning  FailedScheduling  3m5s  default-scheduler  0/1 nodes are available: 1 Insufficient memory. preemption: 0/1 nodes are available: 1 No preemption victims found for incoming pod.
```

To remedy this, lower the requested resources in the `manifests/05-operator.yaml` deployment (e.g. lower memory from 2G to 0.5G).

### Create the `PagerDutyIntegration` object

There's an example at
`deploy-extras/pagerduty_v1alpha1_pagerdutyintegration_cr.yaml` that
you can edit and apply to your cluster.

You'll need to use a valid escalation policy ID from your PagerDuty account. You
can get this by clicking on your policy at
https://{your-account}.pagerduty.com/escalation_policies#. The ID will be
visible in the URL after the `#` character.

### Create the `ClusterDeployment` object

`pagerduty-operator` doesn't start reconciling clusters until `spec.installed` is set to `true`.

You can create a dummy ClusterDeployment by copying a real one from an active hive

```terminal
real-hive$ oc get cd -n <namespace> <cdname> -o yaml > /tmp/fake-clusterdeployment.yaml

...

$ oc create namespace <namespace>
$ oc apply -f /tmp/fake-clusterdeployment.yaml
```

Perform the following modifications:
```terminal
$ oc edit clusterdeployment <cdname> -n <namespace>
```

- Add the following finalizer (i.e. `metadata.finalizers`):
  ```
  - pd.managed.openshift.io/example-pagerdutyintegration
  ```
  (the suffix comes from the `PagerDutyIntegration` name)
- Add the following label:  
  ```
  api.openshift.com/test: "true"
  ```
  (this is used by the `ClusterDeployment` selector in the default [`PagerDutyIntegration`](deploy-extras/pagerduty_v1alpha1_pagerdutyintegration_cr.yaml))
- Set `spec.installed` to true.



### Delete the `ClusterDeployment` object

To trigger `pagerduty-operator` to remove the service in pagerduty, delete the clusterdeployment.

oc delete clusterdeployment fake-cluster -n fake-cluster-namespace
```

You may need to remove dangling finalizers from the `clusterdeployment` object.

```terminal
oc edit clusterdeployment fake-cluster -n fake-cluster-namespace
```

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

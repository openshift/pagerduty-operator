FROM registry.svc.ci.openshift.org/openshift/release:golang-1.10 AS builder
COPY . /go/src/github.com/openshift/pagerduty-operator
WORKDIR /go/src/github.com/openshift/pagerduty-operator
RUN make gobuild

FROM registry.svc.ci.openshift.org/openshift/origin-v4.0:base

WORKDIR /root/

COPY --from=builder /go/src/github.com/openshift/pagerduty-operator/build/_output/bin/pagerduty-operator /usr/local/bin/pagerduty-operator
LABEL io.openshift.managed.name="pagerduty-operator" \
      io.openshift.managed.description="Operator PagerDuty Services and Alertmanager integration keys"

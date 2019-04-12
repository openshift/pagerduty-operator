FROM registry.svc.ci.openshift.org/openshift/release:golang-1.10 AS builder
ENV OPERATOR_PATH=/go/src/github.com/openshift/pagerduty-operator
COPY . ${OPERATOR_PATH}
WORKDIR ${OPERATOR_PATH}
RUN make gobuild

FROM registry.svc.ci.openshift.org/openshift/origin-v4.0:base

ENV OPERATOR_PATH=/go/src/github.com/openshift/pagerduty-operator \
    OPERATOR_BIN=pagerduty-operator

WORKDIR /root/

COPY --from=builder ${OPERATOR_PATH}/build/_output/bin/${OPERATOR_BIN} /usr/local/bin/${OPERATOR_BIN}
LABEL io.openshift.managed.name="pagerduty-operator" \
      io.openshift.managed.description="Operator PagerDuty Services and Alertmanager integration keys"

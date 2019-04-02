FROM alpine:3.8

RUN apk --no-cache add ca-certificates

ENV OPERATOR=/usr/local/bin/pagerduty-operator \
    USER_UID=1001 \
    USER_NAME=pagerduty-operator

# install operator binary
COPY build/_output/bin/pagerduty-operator ${OPERATOR}

COPY build/bin /usr/local/bin
RUN  /usr/local/bin/user_setup

ENTRYPOINT ["/usr/local/bin/entrypoint"]

USER ${USER_UID}

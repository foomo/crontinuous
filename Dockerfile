FROM alpine:latest

ENV SHELL="/bin/ash"
COPY bin/crontinuous-linux-amd64 /usr/sbin/crontinuous

RUN apk add --update --no-cache curl && \
    rm -rf /var/cache/apk/*

VOLUME /etc/crontab
ENTRYPOINT ["/usr/sbin/crontinuous"]

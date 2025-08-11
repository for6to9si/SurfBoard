FROM alpine:latest

ARG BINARY_PATH

COPY build/${BINARY_PATH} /usr/local/bin/surfboard

RUN chmod +x /usr/local/bin/surfboard && \
    apk add --no-cache jq iptables curl

CMD ["/usr/local/bin/surfboard"]
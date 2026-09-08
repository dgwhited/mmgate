FROM alpine:3.24

RUN apk --no-cache add ca-certificates

# Run unprivileged. mmgate binds :8080 (>1024), so it needs no capabilities.
RUN addgroup -S -g 10001 mmgate \
 && adduser  -S -u 10001 -G mmgate -H -s /sbin/nologin mmgate

COPY mmgate /usr/local/bin/mmgate

USER 10001:10001

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1

ENTRYPOINT ["mmgate"]
CMD ["--config", "/etc/mmgate/config.yaml"]

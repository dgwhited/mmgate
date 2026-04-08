FROM alpine:3.21
RUN apk --no-cache add ca-certificates
COPY mmgate /usr/local/bin/mmgate
EXPOSE 8080
ENTRYPOINT ["mmgate"]
CMD ["--config", "/etc/mmgate/config.yaml"]

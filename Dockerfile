FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /mmgate .

FROM alpine:3.21
RUN apk --no-cache add ca-certificates
COPY --from=build /mmgate /usr/local/bin/mmgate
EXPOSE 8080
ENTRYPOINT ["mmgate"]
CMD ["--config", "/etc/mmgate/config.yaml"]

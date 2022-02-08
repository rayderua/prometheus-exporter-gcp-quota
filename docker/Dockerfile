FROM --platform=linux/amd64 golang:1.17-alpine as builder
WORKDIR /app
ADD . /app
RUN apk --no-cache add ca-certificates
RUN go mod download
RUN BUILD_DATE=$(date +%F-%T) CGO_ENABLED=0 GOOS=linux \
    go build -o /prometheus-exporter-gcp-quota \
    -ldflags  "-s -w -extldflags \"-static\" -X main.BuildDate=$BUILD_DATE" .

FROM --platform=linux/amd64 scratch as scratch
WORKDIR /app
COPY --from=builder /prometheus-exporter-gcp-quota /app/prometheus-exporter-gcp-quota
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

USER 65534:65534
EXPOSE 9593
ENTRYPOINT [ "/app/prometheus-exporter-gcp-quota" ]


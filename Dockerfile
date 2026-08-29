# syntax=docker/dockerfile:1.6
FROM golang:1.22-bookworm AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG REVISION=unknown
ARG BRANCH=unknown
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
    -ldflags "-s -w -X github.com/prometheus/common/version.Version=${VERSION} -X github.com/prometheus/common/version.Revision=${REVISION} -X github.com/prometheus/common/version.Branch=${BRANCH}" \
    -o /out/beat-exporter .

FROM gcr.io/distroless/static-debian12:nonroot
LABEL maintainer="mohamedhabas11/beat-exporter"
COPY --from=builder /out/beat-exporter /bin/beat-exporter
USER nonroot:nonroot
EXPOSE 9479
ENTRYPOINT ["/bin/beat-exporter"]
# For alpine alternative, use alpine:3.20 and add ca-certificates:
# FROM alpine:3.20
# RUN apk --no-cache add ca-certificates
# COPY --from=builder /out/beat-exporter /bin/beat-exporter
# USER nobody

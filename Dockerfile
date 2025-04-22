FROM golang:1.24 AS builder

ARG VERSION
ENV PKG=github.com/resmoio/kubernetes-event-exporter/pkg

ADD . /app
WORKDIR /app
RUN CGO_ENABLED=0 GOOS=linux GO11MODULE=on go build -ldflags="-s -w -X ${PKG}/version.Version=${VERSION}" -a -o /main .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder --chown=nonroot:nonroot /main /kubernetes-event-exporter

# https://github.com/GoogleContainerTools/distroless/blob/main/base/base.bzl#L8C1-L9C1
USER 65532

ENTRYPOINT ["/kubernetes-event-exporter"]

FROM golang:1.19 AS builder
ENV GOPROXY="https://goproxy.cn"
WORKDIR /app
COPY go.mod /app/go.mod
COPY go.sum /app/go.sun
RUN go mod download

ADD . /app
RUN CGO_ENABLED=0 GOOS=linux GO11MODULE=on go build -a -o /main .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder --chown=nonroot:nonroot /main /kubernetes-event-exporter

USER nonroot

ENTRYPOINT ["/kubernetes-event-exporter"]

FROM golang:1.19 AS builder

ADD . /app
WORKDIR /app
RUN CGO_ENABLED=0 GOOS=linux GO11MODULE=on go build -a -o /main .

FROM ubuntu:20.04
RUN apt-get update && apt-get install gettext-base curl ca-certificates -y --no-install-recommends \
    && \
    apt-get autoremove \
    && \
    apt-get clean \
    && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /main /kubernetes-event-exporter

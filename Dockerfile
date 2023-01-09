FROM golang:1.19.4-alpine3.17 AS build

WORKDIR /app

# Install VIPS
RUN apk add --update --no-cache vips-dev libwebp-dev libheif-dev build-base

# Compile Go
COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY *.go ./

RUN go build -o /proxy proxy.go

## Deploy
FROM alpine:3.17

WORKDIR /

EXPOSE 6743

COPY --from=build /proxy /proxy

RUN apk add --update --no-cache vips-dev libwebp-dev libheif-dev  && \
    addgroup nonroot && adduser --shell /sbin/nologin --disabled-password  --no-create-home --ingroup nonroot nonroot
USER nonroot:nonroot

ENTRYPOINT ["/proxy"]

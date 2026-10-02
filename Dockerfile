# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X github.com/Raindancer118/nak-api/cmd.Version=$VERSION" -o /out/nak .

FROM alpine:3.22
# poppler-utils: pdftotext gives better text from Moodle PDFs than the Go fallback
RUN apk add --no-cache ca-certificates tzdata poppler-utils \
 && adduser -S -D -H -u 10001 -h /data -s /sbin/nologin nak \
 && install -d -o nak -m 0700 /data
COPY --from=build /out/nak /usr/local/bin/nak
ENV NAK_DATA_DIR=/data \
    NAK_WEB_ADDR=0.0.0.0:8080 \
    NAK_TZ=Europe/Berlin \
    TZ=Europe/Berlin
USER nak
WORKDIR /data
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["nak", "healthcheck"]
ENTRYPOINT ["nak"]
CMD ["serve"]

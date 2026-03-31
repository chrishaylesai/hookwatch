FROM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.23-alpine AS builder
WORKDIR /src
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN apk add --no-cache ca-certificates tzdata
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/frontend/build ./frontend/build
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
	go build -trimpath -ldflags="-s -w" -o /out/hookwatch ./cmd/hookwatch
RUN mkdir -p /rootfs/data /rootfs/etc/ssl/certs /rootfs/usr/share /rootfs/usr/local/bin && \
	cp /etc/ssl/certs/ca-certificates.crt /rootfs/etc/ssl/certs/ca-certificates.crt && \
	cp -R /usr/share/zoneinfo /rootfs/usr/share/zoneinfo && \
	cp /out/hookwatch /rootfs/usr/local/bin/hookwatch && \
	chown -R 65532:65532 /rootfs/data

FROM scratch
COPY --from=builder /rootfs/ /
VOLUME ["/data"]
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/hookwatch"]
CMD ["--port", "8080", "--data-dir", "/data"]

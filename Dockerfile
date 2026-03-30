FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.23-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/frontend/build ./frontend/build
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /hookwatch ./cmd/hookwatch

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=backend /hookwatch /usr/local/bin/hookwatch
VOLUME /data
EXPOSE 8080
ENTRYPOINT ["hookwatch"]
CMD ["--port", "8080", "--data-dir", "/data"]

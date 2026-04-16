FROM --platform=$BUILDPLATFORM golang:1.26.2-alpine AS builder
ARG TARGETARCH
ARG TARGETVARIANT
ARG BUILD_TAGS=""
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} GOARM=${TARGETVARIANT#v} go build -tags "${BUILD_TAGS}" -a -trimpath -o ollama-gateway .

FROM scratch
COPY --from=builder /app/ollama-gateway /ollama-gateway
COPY ./LICENSE /LICENSE
ENTRYPOINT ["/ollama-gateway"]

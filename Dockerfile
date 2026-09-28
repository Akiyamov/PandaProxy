FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine AS modules
WORKDIR /modules
COPY go.mod go.sum ./
RUN go mod download

FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine AS builder
WORKDIR /app

COPY --from=modules /go/pkg /go/pkg

COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG VERSION
ARG COMMIT=none

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X main.Version=${VERSION:-dev} -X main.Commit=${COMMIT:-none} -X main.BuildDate=$(date -u +'%Y-%m-%dT%H:%M:%SZ')" \
    -o /bin/app .

FROM gcr.io/distroless/static:nonroot

ARG VERSION
ARG COMMIT
ARG SOURCE_URL

LABEL org.opencontainers.image.version="${VERSION}"
LABEL org.opencontainers.image.revision="${COMMIT}"
LABEL org.opencontainers.image.source="${SOURCE_URL}"
LABEL org.opencontainers.image.description="PandaProxy"
LABEL org.opencontainers.image.licenses="GNU General Public License v3.0"

COPY --from=builder /bin/app /app/app

USER 1000

EXPOSE 60394

ENV DISABLE_ENV_FILE=true

CMD ["/app/app"]

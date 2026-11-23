FROM --platform=$BUILDPLATFORM golang:1.26.10-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN test ! -e /src/docs/handoffs && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -buildvcs=false -trimpath -o /out/backoffice ./cmd/backoffice && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -buildvcs=false -trimpath -o /out/dsp ./cmd/dsp
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tini bash curl && rm -rf /var/lib/apt/lists/* && useradd -u 10001 -m app
COPY --from=build /out/ /app/
COPY assets/geo/dbip-country-lite.mmdb /app/geo/country.mmdb
COPY LICENSE THIRD_PARTY_NOTICES.md /app/licenses/
COPY assets/geo/NOTICE.md assets/geo/CC-BY-4.0.txt /app/licenses/geo/
COPY internal/backoffice/assets/*LICENSE internal/backoffice/assets/fonts/*LICENSE /app/licenses/assets/
COPY third_party/ /app/licenses/dependencies/
COPY scripts/install.sh scripts/update.sh scripts/metrics.sh /app/
COPY scripts/container-entrypoint.sh /app/entrypoint.sh
RUN chmod 755 /app/entrypoint.sh && mkdir /data && chown app:app /data
ENV DB_PATH=/data/app.db TOKEN_FILE=/data/internal-token LISTEN_ADDR=0.0.0.0:8080
USER app
VOLUME /data
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=30s CMD curl -fsS http://127.0.0.1:8080/health/ready || exit 1
ENTRYPOINT ["/usr/bin/tini", "--", "/app/entrypoint.sh"]

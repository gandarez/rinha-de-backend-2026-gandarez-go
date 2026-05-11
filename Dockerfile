FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY scripts/ ./scripts/
RUN apk add --no-cache curl bash \
    && bash scripts/download-resources.sh \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
       go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.20
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=build /out/api ./api
COPY --from=build /src/resources ./resources
ENV RESOURCES_DIR=/app/resources \
    PORT=9999
EXPOSE 9999
USER app
ENTRYPOINT ["/app/api"]

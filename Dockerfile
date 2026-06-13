ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG TARGETARCH

FROM --platform=$BUILDPLATFORM node:22-alpine AS web-build
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY VERSION /app/VERSION
COPY web ./
RUN NEXT_PUBLIC_APP_VERSION="$(cat /app/VERSION)" npm run build -- --webpack

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} go build -trimpath -ldflags="-s -w" -o /out/chatgpt2api ./cmd/server

FROM alpine:3.20 AS app
WORKDIR /app
RUN adduser -D -H app && mkdir -p /app/data /app/web_dist && chown -R app:app /app
COPY --from=go-build /out/chatgpt2api /app/chatgpt2api
COPY --from=web-build /app/web/out /app/web_dist
COPY config.json VERSION ./
USER app
EXPOSE 80
ENV CHATGPT2API_ADDR=:80
CMD ["/app/chatgpt2api"]

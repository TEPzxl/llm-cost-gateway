FROM golang:1.26.3-alpine AS build

WORKDIR /src

ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/gateway ./cmd/gateway

FROM alpine:3.20

RUN adduser -D -H appuser
USER appuser

COPY --from=build /out/gateway /gateway

EXPOSE 8080

ENTRYPOINT ["/gateway"]

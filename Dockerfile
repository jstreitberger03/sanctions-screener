FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

RUN LDFLAGS="-s -w -X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildDate=${DATE}'" && \
	CGO_ENABLED=1 go build -ldflags="$LDFLAGS" -o /app/screener ./cmd/screener && \
	CGO_ENABLED=1 go build -ldflags="$LDFLAGS" -o /app/api ./cmd/api

FROM alpine:3.19

RUN apk add --no-cache sqlite-libs ca-certificates

WORKDIR /app
COPY --from=builder /app/screener .
COPY --from=builder /app/api .
COPY --from=builder /app/config ./config
COPY --from=builder /app/data ./data

EXPOSE 8080

ENTRYPOINT ["./screener"]
CMD ["serve", "--port", "8080"]

# To run the standalone API server instead:
#   docker run ... ./api

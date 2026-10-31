# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/migrate ./cmd/migrate
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/prober ./cmd/prober

FROM alpine:3.21
RUN adduser -D -H app
WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /out/migrate /app/migrate
COPY --from=build /out/prober /app/prober
COPY web /app/web
COPY migrations /app/migrations
USER app
ENV PORT=8080 STATIC_DIR=/app/web MIGRATIONS_DIR=/app/migrations
EXPOSE 8080
CMD ["/app/api"]

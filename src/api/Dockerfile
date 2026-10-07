FROM golang:1.26.8-alpine3.24 AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/server

FROM alpine:3.24
RUN apk upgrade --no-cache
WORKDIR /app
COPY --from=build /out/api /app/api
EXPOSE 8080
ENTRYPOINT ["/app/api"]

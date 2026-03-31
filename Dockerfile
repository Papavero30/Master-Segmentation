FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./
COPY data-layer ./data-layer
COPY domain-layer ./domain-layer
COPY helpers ./helpers
COPY presentation-layer ./presentation-layer

RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/brainnav ./

FROM golang:1.25-alpine AS dev
WORKDIR /app
RUN go install github.com/air-verse/air@latest
COPY . .
COPY --from=build /bin/brainnav /bin/brainnav
CMD ["air"]

FROM alpine:3.20 AS app
WORKDIR /app
ENV APP_ENV=production

RUN adduser -D -u 10001 appuser
COPY --from=build /bin/brainnav /app/brainnav
COPY certs ./certs

EXPOSE 8080 8443
USER appuser
ENTRYPOINT ["/app/brainnav"]

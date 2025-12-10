FROM golang:1.25-alpine3.22 AS build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o transformer ./services/transformer

FROM alpine:3.22
WORKDIR /app
COPY --from=build /app/transformer .
CMD ["./transformer"]

FROM golang:1.24.2-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

EXPOSE 6380

RUN go mod download && go mod verify

ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

COPY . .

RUN go build -o velox-server ./cmd/velox-server/main.go

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app .

CMD ["./velox-server"]
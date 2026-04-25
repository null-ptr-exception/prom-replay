FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /replay-manager .

FROM gcr.io/distroless/static-debian12
COPY --from=builder /replay-manager /replay-manager
ENTRYPOINT ["/replay-manager"]

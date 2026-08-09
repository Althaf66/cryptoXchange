# One image definition for all three Go services; SERVICE picks the binary.
FROM golang:1.24-alpine AS build

ARG SERVICE=api
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/app ./cmd/${SERVICE}

FROM alpine:3.20
WORKDIR /app
COPY --from=build /out/app /app/app
ENTRYPOINT ["/app/app"]

# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
# Run unit tests as part of the build - build fails if tests fail.
RUN go test ./...
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/controller ./cmd/controller

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/controller /controller
EXPOSE 7000 7001
ENTRYPOINT ["/controller"]

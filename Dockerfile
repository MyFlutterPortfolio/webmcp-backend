FROM golang:1.26.6-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/webmcp-server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/webmcp-server /webmcp-server
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/webmcp-server"]

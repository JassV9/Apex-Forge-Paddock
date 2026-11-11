FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/paddock ./cmd/paddock
# An empty directory to copy as /data, so the volume mounted there is writable by nonroot.
RUN mkdir /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/paddock /paddock
COPY --from=build --chown=nonroot:nonroot /out/data /data
EXPOSE 8080
ENTRYPOINT ["/paddock"]

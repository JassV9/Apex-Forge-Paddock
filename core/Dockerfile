FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/paddock ./cmd/paddock
# Empty directories to copy as /data and /outbox, so volumes mounted there are writable by
# nonroot.
RUN mkdir /out/data /out/outbox

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/paddock /paddock
COPY --from=build --chown=nonroot:nonroot /out/data /data
COPY --from=build --chown=nonroot:nonroot /out/outbox /outbox
EXPOSE 8080
ENTRYPOINT ["/paddock"]

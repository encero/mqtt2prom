FROM golang:1.18-alpine AS build

ENV CGO_ENABLED=0

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
go build -buildvcs=false -o /out/mqtt2prom .

FROM scratch AS bin-unix

LABEL org.opencontainers.image.source https://github.com/encero/mqtt2prom

COPY --from=build /etc/ssl/certs /etc/ssl/certs
COPY --from=build /out/mqtt2prom /mqtt2prom
CMD ["/mqtt2prom"]




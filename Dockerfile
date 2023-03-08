FROM golang:1.19-bullseye as build

WORKDIR /go/src/dhfind

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /go/bin/dhfind ./cmd

FROM gcr.io/distroless/static-debian11
COPY --from=build /go/bin/dhfind /usr/bin/

ENTRYPOINT ["/usr/bin/dhfind"]
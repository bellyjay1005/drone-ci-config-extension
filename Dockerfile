FROM alpine:3.6 as alpine
RUN apk add -U --no-cache ca-certificates

FROM golang as base
WORKDIR /app

ENV GODEBUG netdns=go
ENV GO111MODULE=on

COPY go.mod .
COPY go.sum .
RUN go mod download

FROM base as test

CMD go test plugin/*.go

FROM base as build

COPY . .
RUN go build -v -a -tags config-check-extension \
  -o /release/linux/amd64/aws-config-check-extension ./cmd/aws-config-check-extension

FROM golang
EXPOSE 5000

ENV GODEBUG netdns=go
ENV GO111MODULE=on

COPY --from=alpine /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /release/linux/amd64/aws-config-check-extension /bin/aws-config-check-extension

ENTRYPOINT ["/bin/aws-config-check-extension"]

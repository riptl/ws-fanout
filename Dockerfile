FROM golang:alpine as builder
ADD . /go/src/github.com/terorie/ws-fanout
RUN apk add git \
 && go get -d -v github.com/terorie/ws-fanout \
 && CGO_ENABLED=0 go install -a \
    -installsuffix cgo \
    -ldflags="-s -w" \
    github.com/terorie/ws-fanout

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/bin/ws-fanout /bin/
ENV PORT=8000
EXPOSE 8000
CMD ["/bin/ws-fanout"]

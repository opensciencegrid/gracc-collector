FROM golang:1.16 AS builder

LABEL name="OSG GRACC Collector"
LABEL build-date=20171102-1623

WORKDIR /go/src/app
COPY . .

RUN go get -d -v ./...
RUN CGO_ENABLED=0 go build -tags=netgo -ldflags="-X main.BUILD=$(date -u +%Y-%m-%dT%H:%M:%SZ)"


FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root
COPY --from=builder /go/src/app/gracc-collector .
ENV GRACC_ADDRESS 0.0.0.0
ENV GRACC_PORT 8080

EXPOSE 8080


CMD ["./gracc-collector"]


FROM golang:1.16

LABEL name="OSG GRACC Collector"
LABEL build-date=20171102-1623

WORKDIR /go/src/app
COPY . .

RUN go get -d -v ./
RUN go install -v ./

ENV GRACC_ADDRESS 0.0.0.0
ENV GRACC_PORT 8080

EXPOSE 8080


CMD ["gracc-collector"]


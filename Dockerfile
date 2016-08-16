FROM centos:7

# setup dev environment
RUN yum -y groupinstall 'Development Tools'
RUN yum -y install rpmdevtools && rpmdev-setuptree

# Install Go
RUN curl -O -s https://storage.googleapis.com/golang/go1.6.3.linux-amd64.tar.gz
RUN echo 'cdde5e08530c0579255d6153b08fdb3b8e47caabbe717bc7bcd7561275a87aeb  go1.6.3.linux-amd64.tar.gz' > go1.6.3.linux-amd64.tar.gz.sha256
RUN sha256sum --check go1.6.3.linux-amd64.tar.gz.sha256
RUN tar -C /usr/local -xzf go1.6.3.linux-amd64.tar.gz
ENV PATH /usr/local/go/bin:$PATH
ENV GOPATH /gopath

# Build & Test
RUN mkdir -p /gopath/src/github.com/opensciencegrid/gracc-collector
ADD . /gopath/src/github.com/opensciencegrid/gracc-collector

WORKDIR /gopath/src/github.com/opensciencegrid/gracc-collector
CMD make test && make rpm
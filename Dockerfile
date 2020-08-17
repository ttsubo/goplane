FROM ubuntu:18.04

MAINTAINER  Toshiki Tsuboi <t.tsubo2000@gmail.com>

# Setup environment varialbles for go
ENV GO_VERSION 1.15
ENV GOPATH  /golang
ENV PATH  /go/bin:$GOPATH/bin:$PATH

# Install go
ADD https://storage.googleapis.com/golang/go$GO_VERSION.linux-amd64.tar.gz /
RUN tar xzvf go$GO_VERSION.linux-amd64.tar.gz


# Install goplane
WORKDIR $GOPATH/bin
RUN apt update \
 && apt install -y \
                   curl \
                   git \
                   vim \
                   build-essential \
                   tcpdump \
                   bridge-utils \
                   net-tools \
                   iproute2 \ 
                   iputils-ping \
 && apt clean \
 && rm -rf /var/lib/apt/lists/*

WORKDIR $GOPATH/src/github.com/ttsubo/goplane/
ENV GO15VENDOREXPERIMENT :1
RUN go get github.com/golang/dep/cmd/dep
ADD . $GOPATH/src/github.com/ttsubo/goplane/
RUN dep ensure
RUN go install
RUN cd vendor/github.com/osrg/gobgp/cmd/gobgp && go install

## docker build --rm -t ubuntu-1804-photoarchive-builder .
## docker run --rm -it -v $(pwd):/PhotoArchive ubuntu-1804-photoarchive-builder
FROM ubuntu:18.04

VOLUME /PhotoArchive

RUN apt-get -y update \
    && apt-get -y upgrade \
    && apt-get -y install git golang-go

WORKDIR /PhotoArchive

RUN go get github.com/xor-gate/goexif2/exif
ENTRYPOINT go build timestampNameImages.go

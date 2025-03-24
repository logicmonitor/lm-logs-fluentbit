FROM gcc:latest

FROM golang:1.21.12 as gobuilder
ENV GOOS=linux\
    GOARCH=amd64
RUN mkdir /go/src/lm
COPY . /go/src/lm
WORKDIR /go/src/lm
ENV CGO_ENABLED=1
RUN apt-get update && \
    apt-get install -y gcc \
    build-essential \
    gcc-aarch64-linux-gnu \
    clang
RUN make
RUN make linux-amd
RUN make linux-arm
RUN chmod -R 444 /go/src/lm/build/out_lm.so

# Add Windows build stage
FROM golang:1.21.12 as gobuilder-windows
ENV GOOS=windows\
    GOARCH=amd64
RUN mkdir /go/src/lm
COPY . /go/src/lm
WORKDIR /go/src/lm
ENV CGO_ENABLED=1
RUN apt-get update && \
    apt-get install -y gcc \
    build-essential \
    gcc-mingw-w64-x86-64 \
    clang
COPY --from=gobuilder /go/src/lm/*.so /go/src/lm/build/
RUN make windows
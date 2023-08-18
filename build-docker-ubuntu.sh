docker run -v .:/build ubuntu:22.04 bash -c '\
  apt-get update &&\
  apt-get install -y build-essential cmake g++ libcurl4-openssl-dev zlib1g-dev &&\
  cmake /build/OcapReplaySaver2 &&\
  make &&\
  cp OcapReplaySaver2_x64.so /build'

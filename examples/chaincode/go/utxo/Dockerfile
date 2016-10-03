from hyperledger/fabric-baseimage

RUN apt-get update && apt-get install pkg-config autoconf libtool -y
RUN cd /tmp && git clone https://github.com/bitcoin/secp256k1.git && cd secp256k1/
WORKDIR /tmp/secp256k1
RUN ./autogen.sh
RUN ./configure --enable-module-recovery
RUN make
RUN ./tests
RUN make install

WORKDIR /tmp
RUN apt-get install libtool libboost1.55-all-dev -y
RUN git clone https://github.com/libbitcoin/libbitcoin-consensus.git
WORKDIR /tmp/libbitcoin-consensus
RUN ./autogen.sh
RUN ./configure
RUN make
RUN make install

# Now SWIG
WORKDIR /tmp
# Need pcre lib for building
RUN apt-get install libpcre3-dev -y
RUN wget http://prdownloads.sourceforge.net/swig/swig-3.0.8.tar.gz && tar xvf swig-3.0.8.tar.gz
WORKDIR /tmp/swig-3.0.8
RUN ./autogen.sh
RUN ./configure
RUN make
RUN make install

# Now add this for SWIG execution requirement to get missing stubs-32.h header file
RUN apt-get install g++-multilib -y

ENV CGO_LDFLAGS="-L/usr/local/lib/ -lbitcoin-consensus -lstdc++"

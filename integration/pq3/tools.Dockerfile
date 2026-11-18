FROM ubuntu:24.04

COPY osnadmin /usr/local/bin/osnadmin
COPY pq3-verify /usr/local/bin/pq3-verify

ENTRYPOINT ["/usr/local/bin/osnadmin"]

FROM scratch
COPY payload/busybox /bin/busybox
RUN ["/bin/busybox", "mkdir", "-p", "/usr/bin", "/sbin", "/usr/sbin"]
RUN ["/bin/busybox", "--install"]
RUN mkdir -p /usr/local/bin
ENV PATH=$PATH:/usr/local/bin

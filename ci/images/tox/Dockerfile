# Copyright IBM Corp. All Rights Reserved.
#
# SPDX-License-Identifier: Apache-2.0

FROM alpine:3.12

ENV TOX_VERSION=3.18.1

RUN apk --no-cache add curl python2
RUN curl https://bootstrap.pypa.io/get-pip.py -o get-pip.py && python get-pip.py
RUN pip install tox==${TOX_VERSION} && rm -rf /root/.cache/pip

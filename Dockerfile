FROM golang:alpine

COPY . /go/src/github.com/automattic/go-search-replace

RUN set -x \
	&& apk add --no-cache --virtual .build-deps git \
	&& cd /go/src/github.com/automattic/go-search-replace \
	&& go build -o /usr/bin/go-search-replace . \
	&& rm -rf /go \
	&& apk del .build-deps

CMD [ "/usr/bin/go-search-replace" ]

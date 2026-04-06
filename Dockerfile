FROM golang:1.26-alpine

RUN apk add --no-cache bash \
	curl \
	git \
	make \
	build-base \
	tini

ENTRYPOINT ["/sbin/tini", "--", "/entrypoint.sh"]
CMD [ "-h" ]

COPY scripts/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ARG TARGETARCH
COPY linux/${TARGETARCH}/admctl /usr/bin/admctl

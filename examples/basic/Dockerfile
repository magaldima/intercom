# build stage
FROM golang:stretch AS build-go

ENV http_proxy http://ftpproxy.blackrock.com:8080
ENV https_proxy http://ftpproxy.blackrock.com:8080
ENV HTTP_PROXY http://ftpproxy.blackrock.com:8080`
ENV HTTPS_PROXY http://ftpproxy.blackrock.com:8080

ENV D=/go/src/github.com/magaldima/intercom
# deps
RUN go get -u github.com/golang/dep/...
COPY ./*.go $D/app/
RUN cd $D/app && dep init -v

# build
RUN go build -o dist
COPY dist /tmp/

# final stage
FROM debian:9.4
WORKDIR /app/server/
COPY --from=build-go /tmp/dist /app/server/
CMD [ "./dist" ]

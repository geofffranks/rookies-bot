FROM golang

COPY . /repo
RUN cd /repo && CGO_ENABLED=0 go build -o /rookies-bot .


FROM alpine
COPY --from=0 /rookies-bot /rookies-bot
RUN apk add --no-cache tzdata
ENV TZ=America/New_York
CMD /rookies-bot --config /data/config.yml bot


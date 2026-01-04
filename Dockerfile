FROM golang

COPY . /repo
RUN cd /repo && CGO_ENABLED=0 go build -o /rookies-bot .


FROM alpine
COPY --from=0 /rookies-bot /rookies-bot
CMD /rookies-bot --config /data/config.yml bot

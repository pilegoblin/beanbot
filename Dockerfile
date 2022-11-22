FROM golang:1.19.3-bullseye

WORKDIR /app

ENV BEANBOT_TOKEN=put_token_here

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY *.go ./

RUN go build -o /beanbot

CMD [ "/beanbot" ]
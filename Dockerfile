FROM golang:latest

WORKDIR /app

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY go.mod .
COPY go.sum .

RUN go build -o beanbot ./cmd/bot

CMD [ "./beanbot" ]
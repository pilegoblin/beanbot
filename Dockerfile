FROM golang:latest

WORKDIR /app

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY go.mod .
COPY go.sum .
COPY .env .

RUN go build -o beanbot ./cmd/bot

CMD [ "./beanbot" ]